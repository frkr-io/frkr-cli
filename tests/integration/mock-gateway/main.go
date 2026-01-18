package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	streamingv1 "github.com/frkr-io/frkr-proto/go/streaming/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type server struct {
	streamingv1.UnimplementedStreamingServiceServer
}

func (s *server) OpenStream(req *streamingv1.OpenStreamRequest, stream streamingv1.StreamingService_OpenStreamServer) error {
	log.Printf("Client connected to stream: %s", req.StreamId)

	// Send a few test messages
	for i := 0; i < 3; i++ {
		msg := &streamingv1.StreamMessage{
			Method: "POST",
			Path:   "/webhook",
			Body:   fmt.Sprintf(`{"test": "message %d"}`, i),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TimestampNs: time.Now().UnixNano(),
		}

		if err := stream.Send(msg); err != nil {
			log.Printf("Error sending message: %v", err)
			return err
		}
	}

	return nil
}

func (s *server) ListStreams(ctx context.Context, req *streamingv1.ListStreamsRequest) (*streamingv1.ListStreamsResponse, error) {
	return &streamingv1.ListStreamsResponse{}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	
	s := grpc.NewServer()
	streamingv1.RegisterStreamingServiceServer(s, &server{})
	reflection.Register(s)

	log.Println("Mock Gateway running on :8080")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
