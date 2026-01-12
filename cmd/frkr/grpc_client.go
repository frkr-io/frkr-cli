package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// NewGRPCConnection establishes a connection to the gRPC server
func NewGRPCConnection(address string, insecureConn bool) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if insecureConn {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// Load system CA pool
		creds := credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	return conn, nil
}

// AuthenticatedContext adds auth credentials to the context
func AuthenticatedContext(ctx context.Context, authHeader string) context.Context {
	md := metadata.Pairs("authorization", authHeader)
	return metadata.NewOutgoingContext(ctx, md)
}
