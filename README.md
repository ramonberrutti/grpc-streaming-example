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

each time that the server send a message (stream.Send()) to the client, need to
encode the message and send it over the network. By sending multiple documents 
at once, we reduce the number of times that the server needs to encode and send 
the message, which results in better performance not only for the server but also
for the client.

In the above example, the batch size is set to 10, but the client can adjust the
batch size based on the network conditions and the size of the documents.


### Use bidirectional streaming to control the flow



