package main_test

import (
	"encoding/csv"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/jszwec/csvutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type book struct {
	Title           string
	Author          string
	PublicationYear int
	Genre           string
}

var bookStoreData = []book{}
var bookStoreOnce sync.Once

func TestMain(m *testing.M) {
	// Initialize book store
	bookStoreOnce.Do(func() {
		f, err := os.Open("data/fake_books.csv")
		if err != nil {
			panic(err)
		}
		defer f.Close()

		dec, err := csvutil.NewDecoder(csv.NewReader(f))
		if err != nil {
			panic(err)
		}

		if err := dec.Decode(&bookStoreData); err != nil {
			panic(err)
		}
	})

	// Run tests
	m.Run()
}

// newServer creates a new gRPC server and returns a client connection to it.
func newServer(t testing.TB, f func(s grpc.ServiceRegistrar)) grpc.ClientConnInterface {
	t.Helper()

	s := grpc.NewServer()
	f(s)

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() {
		s.Stop()
		l.Close()
	})

	go func() {
		err := s.Serve(l)
		if err != nil && err != grpc.ErrServerStopped {
			t.Errorf("server exited with error: %v", err)
		}
	}()

	conn, err := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create client connection: %v", err)
	}

	return conn
}
