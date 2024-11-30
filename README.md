DESCLAIMER: This is an article I wrote for [dev.to](https://dev.to/). All the code is as simple as possible to make it easier to understand. The code is not production-ready and should not be used as is.

# gRPC Streaming the Right Way

## Introduction

gRPC streaming allows protobuf's message to be streamed from client to server, server to client, or bidirectionally. 
This is a powerful feature that can be used to build real-time applications such as chat applications, real-time monitoring dashboards, and more.

In this article, we will explore how to use gRPC streaming to right way.

## Prerequisites

- Basic knowledge of gRPC
- Basic knowledge of Go programming language (The sample code is written in Go, but the concept can be applied to other languages as well)


## Good Practices

Let's check to good practices to use gRPC streaming:

### Use unary request for unary request

One common mistake is to use streaming for unary request. 
For example, consider the following gRPC service definition:

```protobuf
service MyService {
  rpc GetSomething (SomethingRequest) returns (stream SomethingResponse) {}
}
```

If the client only needs to send one request and receive one response, 
there is no need to use streaming. Instead, we can define the service as follows:

```protobuf
service MyService {
  rpc GetSomething (SomethingRequest) returns (SomethingResponse) {}
}
```

By using streaming for unary request, we are adding unnecessary complexity
to the code, which can make it harder to understand and maintain and not 
gaining any benefits from using streaming.

Golang code example comparing unary request and streaming request:

Unary request:
```go
type sometringUnary struct {
	pb.UnimplementedSomethingUnaryServer
}

func (s *sometringUnary) GetSomething(ctx context.Context, req *pb.SomethingRequest) (*pb.SomethingResponse, error) {
	return &pb.SomethingResponse{
		Message: "Hello " + req.Name,
	}, nil
}

func TestSomethingUnary(t *testing.T) {
	conn := newServer(t, func(s grpc.ServiceRegistrar) {
		pb.RegisterSomethingUnaryServer(s, &sometringUnary{})
	})

	client := pb.NewSomethingUnaryClient(conn)

	response, err := client.GetSomething(
		context.Background(),
		&pb.SomethingRequest{
			Name: "test",
		},
	)
	if err != nil {
		t.Fatalf("failed to get something: %v", err)
	}

	if response.Message != "Hello test" {
		t.Errorf("unexpected response: %v", response.Message)
	}
}
```

Streaming unary request:
```go
type sometringStream struct {
	pb.UnimplementedSomethingStreamServer
}

func (s *sometringStream) GetSomething(req *pb.SomethingRequest, stream pb.SomethingStream_GetSomethingServer) error {
	if err := stream.Send(&pb.SomethingResponse{
		Message: "Hello " + req.Name,
	}); err != nil {
		return err
	}

	return nil
}

func TestSomethingStream(t *testing.T) {
	conn := newServer(t, func(s grpc.ServiceRegistrar) {
		pb.RegisterSomethingStreamServer(s, &sometringStream{})
	})

	client := pb.NewSomethingStreamClient(conn)

	stream, err := client.GetSomething(
		context.Background(),
		&pb.SomethingRequest{
			Name: "test",
		},
	)
	if err != nil {
		t.Fatalf("failed to get something stream: %v", err)
	}

	response, err := stream.Recv()
	if err != nil {
		t.Fatalf("failed to receive response: %v", err)
	}

	if response.Message != "Hello test" {
		t.Errorf("unexpected response: %v", response.Message)
	}
}
```

As we can see, the code for unary request is simpler and easier to understand
than the code for streaming request.

### Sending multiple documents at once if we can

Let's compare these two service definitions:

```protobuf
service BookStore {
  rpc ListBooks(ListBooksRequest) returns (stream Book) {}
}

service BookStoreBatch {
  rpc ListBooks(ListBooksRequest) returns (stream ListBooksResponse) {}
}

message ListBooksResponse {
  repeated Book books = 1;
}
```

`BookStore` is streaming one book at a time while `BookStoreBatch` is streaming multiple books at once.

If the client needs to list all books, it is more efficient to use `BookStoreBatch` 
because it reduces the number of round trips between the client and the server.

Let's see the Golang code example for `BookStore` and `BookStoreBatch`:

`BookStore`:
```go
type bookStore struct {
	pb.UnimplementedBookStoreServer
}

func (s *bookStore) ListBooks(req *pb.ListBooksRequest, stream pb.BookStore_ListBooksServer) error {
	for _, b := range bookStoreData {
		if b.Author == req.Author {
			if err := stream.Send(&pb.Book{
				Title:           b.Title,
				Author:          b.Author,
				PublicationYear: int32(b.PublicationYear),
				Genre:           b.Genre,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestBookStore_ListBooks(t *testing.T) {
	conn := newServer(t, func(s grpc.ServiceRegistrar) {
		pb.RegisterBookStoreServer(s, &bookStore{})
	})

	client := pb.NewBookStoreClient(conn)

	stream, err := client.ListBooks(
		context.Background(),
		&pb.ListBooksRequest{
			Author: charlesDickens,
		},
	)
	if err != nil {
		t.Fatalf("failed to list books: %v", err)
	}

	books := []*pb.Book{}
	for {
		book, err := stream.Recv()
		if err != nil {
			break
		}
		books = append(books, book)
	}

	if len(books) != charlesDickensBooks {
		t.Errorf("unexpected number of books: %d", len(books))
	}
}
```

`BookStoreBatch`:
```go
type bookStoreBatch struct {
	pb.UnimplementedBookStoreBatchServer
}

func (s *bookStoreBatch) ListBooks(req *pb.ListBooksRequest, stream pb.BookStoreBatch_ListBooksServer) error {
	const batchSize = 10
	books := make([]*pb.Book, 0, batchSize)
	for _, b := range bookStoreData {
		if b.Author == req.Author {
			books = append(books, &pb.Book{
				Title:           b.Title,
				Author:          b.Author,
				PublicationYear: int32(b.PublicationYear),
				Genre:           b.Genre,
			})

			if len(books) == batchSize {
				if err := stream.Send(&pb.ListBooksResponse{
					Books: books,
				}); err != nil {
					return err
				}
				books = books[:0]
			}
		}
	}

	if len(books) > 0 {
		if err := stream.Send(&pb.ListBooksResponse{
			Books: books,
		}); err != nil {
			return nil
		}
	}

	return nil
}

func TestBookStoreBatch_ListBooks(t *testing.T) {
	conn := newServer(t, func(s grpc.ServiceRegistrar) {
		pb.RegisterBookStoreBatchServer(s, &bookStoreBatch{})
	})

	client := pb.NewBookStoreBatchClient(conn)

	stream, err := client.ListBooks(
		context.Background(),
		&pb.ListBooksRequest{
			Author: charlesDickens,
		},
	)
	if err != nil {
		t.Fatalf("failed to list books: %v", err)
	}

	books := []*pb.Book{}
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		books = append(books, response.Books...)
	}

	if len(books) != charlesDickensBooks {
		t.Errorf("unexpected number of books: %d", len(books))
	}
}
```

From the code above, is not clear which one is better.
Let's run a benchmark to see the difference:

`BookStore` benchmark:
```go
func BenchmarkBookStore_ListBooks(b *testing.B) {
	conn := newServer(b, func(s grpc.ServiceRegistrar) {
		pb.RegisterBookStoreServer(s, &bookStore{})
	})

	client := pb.NewBookStoreClient(conn)

	var benchInerBooks []*pb.Book
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := client.ListBooks(
			context.Background(),
			&pb.ListBooksRequest{
				Author: charlesDickens,
			},
		)
		if err != nil {
			b.Fatalf("failed to list books: %v", err)
		}

		books := []*pb.Book{}
		for {
			book, err := stream.Recv()
			if err != nil {
				break
			}
			books = append(books, book)
		}

		benchInerBooks = books
	}

	benchBooks = benchInerBooks
}
```

`BookStoreBatch` benchmark:
```go
func BenchmarkBookStoreBatch_ListBooks(b *testing.B) {
	conn := newServer(b, func(s grpc.ServiceRegistrar) {
		pb.RegisterBookStoreBatchServer(s, &bookStoreBatch{})
	})

	client := pb.NewBookStoreBatchClient(conn)

	var benchInerBooks []*pb.Book
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := client.ListBooks(
			context.Background(),
			&pb.ListBooksRequest{
				Author: charlesDickens,
			},
		)
		if err != nil {
			b.Fatalf("failed to list books: %v", err)
		}

		books := []*pb.Book{}
		for {
			response, err := stream.Recv()
			if err != nil {
				break
			}
			books = append(books, response.Books...)
		}

		benchInerBooks = books
	}

	benchBooks = benchInerBooks
}
```

Benchmark results:
```
BenchmarkBookStore_ListBooks
BenchmarkBookStore_ListBooks-12                      732           1647454 ns/op           85974 B/op       1989 allocs/op
BenchmarkBookStoreBatch_ListBooks
BenchmarkBookStoreBatch_ListBooks-12                1202            937491 ns/op           61098 B/op        853 allocs/op
```

What an improvement! `BookStoreBatch` is faster than `BookStore` by a factor of 1.75x.

But why is `BookStoreBatch` faster than `BookStore`?

Each time that the server send a message (stream.Send()) to the client, needs to
encode the message and send it over the network. By sending multiple documents 
at once, we reduce the number of times that the server needs to encode and send 
the message, which improve the performance not only for the server but also
for the client that needs to decode the message.

In the above example, the batch size is set to 10, but the client can adjust the
batch size based on the network conditions and the size of the documents.


### Use bidirectional streaming to control the flow

The book store example returns all the books and finishes the stream, if the client
needs to watch for events in real-time (e.g., sensors) the use ofbidirectional
streaming is the right choice.

Bidirectional streaming are a bit tricky because both the client and the server
can send and receive messages at the same time. Hopefully golang makes it easy
to work with concurency like this.

As mentioned before a sensor can be an excellent example of bidirectional streaming.
The watch function allows the client to decide which sensors to watch and request
the current value if is needed.

Let's take a look at the following protobuf definition:

```protobuf
service Sensor {
  rpc Watch(stream WatchRequest) returns (stream WatchResponse) {}
}

message WatchRequest {
  oneof request {
    WatchCreateRequest create_request = 1;
    WatchCancelRequest cancel_request = 2;
    WatchNowRequest now_request = 3;
  }
}

message WatchCreateRequest {
  string sensor_id = 1;
}

message WatchCancelRequest {
  string sensor_id = 1;
}

message WatchNowRequest {
  string sensor_id = 1;
}

message WatchResponse {
  string sensor_id = 1;
  bool created = 2;
  bool canceleted = 3;
  string error = 4;
  google.protobuf.Timestamp timestamp = 5;
  int32 value = 6;
}
```

The request message is not only a stream of messages but also a message that can
contain different types of requests. The oneof directive allows us to define a
field that can contain only one of the specified types.

```WatchCreateRequest create_request``` is used to create a new watch for a sensor.
```WatchCancelRequest cancel_request``` is used to cancel a watch for a sensor.
```WatchNowRequest now_request``` is used to get the current value of a sensor.

The response is a stream of messages that can contain different types of responses.
```string sensor_id``` is the sensor id.
```bool created``` is true if the watch was created successfully.
```bool canceleted``` is true if the watch was canceled successfully or if the creation failed.
```string error``` is the error message if something went wrong.
```google.protobuf.Timestamp timestamp``` is the timestamp of the value.
```int32 value``` is the value of the sensor.


The golang code for the sensor will ignore, but you can found it [here](https://github.com/ramonberrutti/grpc-streaming-example/blob/main/sensor_test.go)

`serverStream` wraps the stream and the sensor data to make it easier to work with.

```go
type serverStream struct {
	s           *sensorService         // Service
	stream      pb.Sensor_WatchServer  // Stream
	sendCh      chan *pb.WatchResponse // Control channel
	sensorCh    chan sensorData        // Data channel
	sensorWatch map[string]int         // Map of sensor id to watch id
}
```

As noted before, the server can send and receive messages at the same time, one 
function will handle the incoming messages and another function will handle the
outgoing messages.

Receiving messages:
```go
func (ss *serverStream) recvLoop() error {
	defer ss.close()
	for {
		req, err := ss.stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		switch req := req.Request.(type) {
		case *pb.WatchRequest_CreateRequest:
            // IGNORE VALIDATION (check the full code)

			// create a channel to send data to the client
			id := sensor.watch(ss.sensorCh)
			ss.sensorWatch[sensorId] = id

			// send created message
			ss.sendCh <- &pb.WatchResponse{
				SensorId: sensorId,
				Created:  true,
			}

		case *pb.WatchRequest_CancelRequest:
            // IGNORE VALIDATION (check the full code)

			// cancel the watch
			ss.s.sensors[sensorId].cancel(id)
			delete(ss.sensorWatch, sensorId)

			ss.sendCh <- &pb.WatchResponse{
				SensorId:   sensorId,
				Canceleted: true,
			}

		case *pb.WatchRequest_NowRequest:
            // IGNORE VALIDATION (check the full code)

			// send current value
			ss.sendCh <- &pb.WatchResponse{
				SensorId:  sensorId,
				Timestamp: timestamppb.Now(),
				Value:     int32(sensor.read()),
			}
		}
	}
}
```

The switch statement is used to handle the different types of requests and decide
what to do with each request. It's important to leave the recvLoop function only
to read and don't send messages to the client for this reason we have the sendLoop
that will read the messages from the control channel and send it to the client.

Sending messages:
```go
func (ss *serverStream) sendLoop() {
	for {
		select {
		case m, ok := <-ss.sendCh:
			if !ok {
				return
			}

			// send message
			if err := ss.stream.Send(m); err != nil {
				return
			}

		case data, ok := <-ss.sensorCh:
			if !ok {
				return
			}

			// send data
			if err := ss.stream.Send(&pb.WatchResponse{
				SensorId:  data.id,
				Timestamp: timestamppb.New(data.time),
				Value:     int32(data.val),
			}); err != nil {
				return
			}

		case <-ss.stream.Context().Done():
			return
		}
	}
}
```

The sendLoop function reads both the control channel and the data channel and sends
the messages to the client. If the stream is closed, the function will return.

Finally, a happy path test for the sensor service:

```go
func TestSensor(t *testing.T) {
	conn := newServer(t, func(s grpc.ServiceRegistrar) {
		pb.RegisterSensorServer(s, &sensorService{
			sensors: newSensors(),
		})
	})

	client := pb.NewSensorClient(conn)

	stream, err := client.Watch(context.Background())
	if err != nil {
		t.Fatalf("failed to watch: %v", err)
	}

	response := make(chan *pb.WatchResponse)
	// Go routine to read from the stream
	go func() {
		defer close(response)
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				return
			}
			response <- resp
		}
	}()

	createRequest(t, stream, "temp")
	waitUntilCreated(t, response, "temp")
	waitForSensorData(t, response, "temp")

	createRequest(t, stream, "pres")
	waitUntilCreated(t, response, "pres")
	waitForSensorData(t, response, "pres")

	waitForSensorData(t, response, "temp")
	waitForSensorData(t, response, "pres")

	// invalid sensor
	createRequest(t, stream, "invalid")
	waitUntilCanceled(t, response, "invalid")

	nowRequest(t, stream, "light")
	waitForSensorData(t, response, "light")
	// Wait for 2 seconds to make sure we don't receive any data for light
	waitForNoSensorData(t, response, "light", 2*time.Second)

	cancelRequest(t, stream, "temp")
	waitUntilCanceled(t, response, "temp")

	waitForSensorData(t, response, "pres")
	// Wait for 2 seconds to make sure we don't receive any data for temp
	waitForNoSensorData(t, response, "temp", 2*time.Second)

	err = stream.CloseSend()
	if err != nil {
		t.Fatalf("failed to close send: %v", err)
	}
}
```

## Challenge Yourself

- Implement a chat application using gRPC streaming.
- Modify the sensor service to send multiple values at once to save round trips.

## Conclusion

gRPC streaming is a powerful feature that can be used to build real-time applications.
The dificulty comes with a lot of benefits, but it's important to use it the right way.

## Stay in touch

If you have any questions or feedback, feel free to reach out to me on [LinkedIn](https://www.linkedin.com/in/ramonberrutti).