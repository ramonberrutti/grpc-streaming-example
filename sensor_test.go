package main_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/ramonberrutti/grpc-streaming-example/protogen"
)

type sensorService struct {
	pb.UnimplementedSensorServer

	sensors map[string]*sensor
}

type serverStream struct {
	s           *sensorService         // Service
	stream      pb.Sensor_WatchServer  // Stream
	sendCh      chan *pb.WatchResponse // Control channel
	sensorCh    chan sensorData        // Data channel
	sensorWatch map[string]int         // Map of sensor id to watch id
}

func (ss *serverStream) close() {
	for sensor, id := range ss.sensorWatch {
		ss.s.sensors[sensor].cancel(id)
	}

	close(ss.sendCh)
	close(ss.sensorCh)
}

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
			if req.CreateRequest == nil {
				break
			}

			// check if sensor exists
			sensorId := req.CreateRequest.SensorId
			sensor, ok := ss.s.sensors[sensorId]
			if !ok {
				ss.sendCh <- &pb.WatchResponse{
					SensorId:   sensorId,
					Canceleted: true,
					Error:      "sensor not found",
				}
				break
			}

			// check if sensor is already being watched
			if _, ok := ss.sensorWatch[sensorId]; ok {
				ss.sendCh <- &pb.WatchResponse{
					SensorId:   sensorId,
					Canceleted: true,
					Error:      "sensor already being watched",
				}
				break
			}

			// create a channel to send data to the client
			id := sensor.watch(ss.sensorCh)
			ss.sensorWatch[sensorId] = id

			// send created message
			ss.sendCh <- &pb.WatchResponse{
				SensorId: sensorId,
				Created:  true,
			}

		case *pb.WatchRequest_CancelRequest:
			if req.CancelRequest == nil {
				break
			}

			// check if we are watching the sensor
			sensorId := req.CancelRequest.SensorId
			id, ok := ss.sensorWatch[sensorId]
			if !ok {
				ss.sendCh <- &pb.WatchResponse{
					SensorId:   sensorId,
					Canceleted: true,
					Error:      "sensor not being watched",
				}
				break
			}

			// cancel the watch
			ss.s.sensors[sensorId].cancel(id)
			delete(ss.sensorWatch, sensorId)

			ss.sendCh <- &pb.WatchResponse{
				SensorId:   sensorId,
				Canceleted: true,
			}

		case *pb.WatchRequest_NowRequest:
			if req.NowRequest == nil {
				break
			}

			// check if sensor exists
			sensorId := req.NowRequest.SensorId
			sensor, ok := ss.s.sensors[sensorId]
			if !ok {
				ss.sendCh <- &pb.WatchResponse{
					SensorId: sensorId,
					Error:    "sensor not found",
				}
				break
			}

			// send current value
			ss.sendCh <- &pb.WatchResponse{
				SensorId:  sensorId,
				Timestamp: timestamppb.Now(),
				Value:     int32(sensor.read()),
			}
		}
	}
}

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

func (s *sensorService) Watch(stream pb.Sensor_WatchServer) error {
	ss := &serverStream{
		s:           s,
		stream:      stream,
		sendCh:      make(chan *pb.WatchResponse, 2), // Control channel
		sensorCh:    make(chan sensorData, 16),       // Data channel
		sensorWatch: make(map[string]int),
	}

	go ss.sendLoop()
	return ss.recvLoop()
}

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

func createRequest(t testing.TB, stream pb.Sensor_WatchClient, sensorID string) {
	t.Helper()
	err := stream.Send(&pb.WatchRequest{
		Request: &pb.WatchRequest_CreateRequest{
			CreateRequest: &pb.WatchCreateRequest{
				SensorId: sensorID,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to send create request: %v", err)
	}
}

func cancelRequest(t testing.TB, stream pb.Sensor_WatchClient, sensorID string) {
	t.Helper()
	err := stream.Send(&pb.WatchRequest{
		Request: &pb.WatchRequest_CancelRequest{
			CancelRequest: &pb.WatchCancelRequest{
				SensorId: sensorID,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to send cancel request: %v", err)
	}
}

func nowRequest(t testing.TB, stream pb.Sensor_WatchClient, sensorID string) {
	t.Helper()
	err := stream.Send(&pb.WatchRequest{
		Request: &pb.WatchRequest_NowRequest{
			NowRequest: &pb.WatchNowRequest{
				SensorId: sensorID,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to send now request: %v", err)
	}
}

func waitUntilCreated(t testing.TB, response chan *pb.WatchResponse, sensorID string) {
	t.Helper()
	for {
		resp, ok := <-response
		if !ok {
			t.Fatalf("failed to receive response")
		}

		if resp.GetSensorId() == sensorID && resp.GetCreated() {
			break
		}
	}
}

func waitUntilCanceled(t testing.TB, response chan *pb.WatchResponse, sensorID string) {
	t.Helper()
	for {
		resp, ok := <-response
		if !ok {
			t.Fatalf("failed to receive response")
		}

		if resp.GetSensorId() == sensorID && resp.GetCanceleted() {
			break
		}
	}
}

func waitForSensorData(t testing.TB, response chan *pb.WatchResponse, sensorID string) {
	t.Helper()
	for {
		resp, ok := <-response
		if !ok {
			t.Fatalf("failed to receive response")
		}

		if resp.GetSensorId() == sensorID && resp.GetTimestamp() != nil {
			break
		}
	}
}

func waitForNoSensorData(t testing.TB, response chan *pb.WatchResponse, sensorID string, timeout time.Duration) {
	tick := time.NewTimer(timeout)
	t.Helper()
	for {
		select {
		case <-tick.C:
			return

		case resp, ok := <-response:
			if !ok {
				t.Fatalf("failed to receive response")
			}

			if resp.GetSensorId() == sensorID && resp.GetTimestamp() != nil {
				t.Fatalf("unexpected sensor data")
			}
		}
	}
}

type sensor struct {
	mu sync.RWMutex

	id  string
	val int

	nextID   int
	watchers map[int]chan<- sensorData
	ticker   *time.Ticker
	done     chan struct{}
}

type sensorData struct {
	id   string
	val  int
	time time.Time
}

func newSensor(id string) *sensor {
	ticker := time.NewTicker(1 * time.Second)
	initialVal := 50

	s := &sensor{
		id:       id,
		val:      initialVal,
		watchers: make(map[int]chan<- sensorData),
		ticker:   ticker,
		done:     make(chan struct{}),
	}

	go func() {
		for {
			select {
			case t := <-ticker.C:
				s.mu.Lock()
				if s.val >= initialVal+10 {
					s.val = initialVal
				}
				s.val += 1
				s.mu.Unlock()

				s.mu.RLock()
				for _, ch := range s.watchers {
					select {
					case ch <- sensorData{
						id:   s.id,
						val:  s.val,
						time: t,
					}:
					default:
					}
				}
				s.mu.RUnlock()
			case <-s.done:
				return
			}

		}
	}()
	return s
}

func (s *sensor) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.watchers {
		delete(s.watchers, id)
	}
	s.ticker.Stop()
	close(s.done)
}

func (s *sensor) read() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.val
}

func (s *sensor) watch(ch chan<- sensorData) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	s.watchers[s.nextID] = ch
	return s.nextID
}

func (s *sensor) cancel(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.watchers, id)
}

func newSensors() map[string]*sensor {
	return map[string]*sensor{
		"temp":  newSensor("temp"),
		"pres":  newSensor("pres"),
		"hum":   newSensor("hum"),
		"light": newSensor("light"),
	}
}
