package main_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	pb "github.com/ramonberrutti/grpc-streaming-example/protogen"
)

const (
	charlesDickens      = "Charles Dickens"
	charlesDickensBooks = 89
)

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

var benchBooks []*pb.Book

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
