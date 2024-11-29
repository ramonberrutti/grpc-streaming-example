package main_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	pb "github.com/ramonberrutti/grpc-streaming-example/protogen"
)

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
