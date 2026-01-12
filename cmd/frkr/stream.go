package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	streamingv1 "github.com/frkr-io/frkr-proto/go/streaming/v1"
	"github.com/spf13/cobra"
)

var streamCmd = &cobra.Command{
	Use:   "stream [stream-id]",
	Short: "Stream messages from a frkr stream",
	Long:  `Stream messages from the specified stream and forward them to a local URL.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		streamID := args[0]
		gatewayAddr, _ := cmd.Flags().GetString("gateway")
		username, _ := cmd.Flags().GetString("username")
		password, _ := cmd.Flags().GetString("password")
		forwardURL, _ := cmd.Flags().GetString("forward-url")
		insecure, _ := cmd.Flags().GetBool("insecure")

		// Build auth header
		authHeader := ""
		if username != "" && password != "" {
			credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
			authHeader = "Basic " + credentials
		} else {
			// Try to load token/creds from auth store
			store, err := loadAuthStore()
			if err == nil && store != nil && store.IsValid() {
				if store.AccessToken != "" {
					authHeader = "Bearer " + store.AccessToken
				} else if store.BasicAuthUsername != "" && store.BasicAuthPassword != "" {
					credentials := base64.StdEncoding.EncodeToString([]byte(store.BasicAuthUsername + ":" + store.BasicAuthPassword))
					authHeader = "Basic " + credentials
				}
			}
		}

		if authHeader == "" {
			return fmt.Errorf("authentication required: provide username/password or login first")
		}

		// Connect to streaming gateway via gRPC
		fmt.Printf("🔌 Connecting to gateway %s...\n", gatewayAddr)
		conn, err := NewGRPCConnection(gatewayAddr, insecure)
		if err != nil {
			return fmt.Errorf("failed to connect to gateway: %w", err)
		}
		defer conn.Close()

		client := streamingv1.NewStreamingServiceClient(conn)
		
		// Open access to the stream
		ctx := context.Background()
		ctx = AuthenticatedContext(ctx, authHeader)

		stream, err := client.OpenStream(ctx, &streamingv1.OpenStreamRequest{
			StreamId: streamID,
		})
		if err != nil {
			return fmt.Errorf("failed to open stream: %w", err)
		}

		forwardTimeout, _ := cmd.Flags().GetInt("forward-timeout")
		maxRetries, _ := cmd.Flags().GetInt("max-retries")

		fmt.Printf("✅ Connected to stream: %s\n", streamID)
		fmt.Printf("📡 Forwarding to: %s\n", forwardURL)
		fmt.Printf("⏱️  Timeout: %ds, Max retries: %d\n", forwardTimeout, maxRetries)
		fmt.Println("Waiting for messages...")

		// Receive loop
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				fmt.Println("Stream closed by server")
				return nil
			}
			if err != nil {
				return fmt.Errorf("error receiving message: %w", err)
			}

			if err := forwardMessageWithRetry(msg, forwardURL, forwardTimeout, maxRetries); err != nil {
				fmt.Fprintf(os.Stderr, "❌ Error forwarding message after retries: %v\n", err)
				continue
			}
		}
	},
}

func forwardMessageWithRetry(msg *streamingv1.StreamMessage, forwardURL string, timeoutSeconds, maxRetries int) error {
	// Build forward URL with path and query
	fullURL := forwardURL + msg.Path
	if len(msg.Query) > 0 {
		var queryParts []string
		for k, v := range msg.Query {
			queryParts = append(queryParts, fmt.Sprintf("%s=%s", k, v))
		}
		fullURL += "?" + strings.Join(queryParts, "&")
	}

	// Create request
	var bodyReader io.Reader
	if msg.Body != "" {
		bodyReader = bytes.NewReader([]byte(msg.Body))
	}

	req, err := http.NewRequest(msg.Method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create forward request: %w", err)
	}

	// Copy headers (excluding Host)
	for k, v := range msg.Headers {
		if strings.ToLower(k) != "host" {
			req.Header.Set(k, v)
		}
	}

	// Retry logic with exponential backoff
	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s...
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			fmt.Fprintf(os.Stderr, "⚠️  Retry %d/%d after %v...\n", attempt, maxRetries, backoff)
			time.Sleep(backoff)
		}

		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			// Don't retry on client errors (4xx)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				fmt.Printf("➡️  %s %s -> %d (client error, not retrying)\n", msg.Method, msg.Path, resp.StatusCode)
				return nil
			}
			// Success or server error (5xx) - log and return success for now
			fmt.Printf("➡️  %s %s -> %d\n", msg.Method, msg.Path, resp.StatusCode)
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
}

func init() {
	streamCmd.Flags().String("gateway", "localhost:8081", "Streaming Gateway Address (host:port)")
	streamCmd.Flags().String("username", "", "Username for basic auth")
	streamCmd.Flags().String("password", "", "Password for basic auth")
	streamCmd.Flags().String("forward-url", "http://localhost:3001", "URL to forward requests to")
	streamCmd.Flags().Int("forward-timeout", 30, "Timeout in seconds for forwarding requests (default: 30)")
	streamCmd.Flags().Int("max-retries", 3, "Maximum number of retries for failed forwards (default: 3)")
	streamCmd.Flags().Bool("insecure", false, "Use insecure connection (no TLS)")

	rootCmd.AddCommand(streamCmd)
}
