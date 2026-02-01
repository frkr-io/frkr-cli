package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	streamingv1 "github.com/frkr-io/frkr-proto/go/streaming/v1"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var streamCmd = &cobra.Command{
	Use:   "stream [stream-id]",
	Short: "Stream messages from a frkr stream",
	Long:  `Stream messages from the specified stream and forward them to a local URL or port.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		gatewayAddr, _ := cmd.Flags().GetString("gateway")
		username, _ := cmd.Flags().GetString("username")
		password, _ := cmd.Flags().GetString("password")
		oauthMode, _ := cmd.Flags().GetBool("oauth")
		forwardURL, _ := cmd.Flags().GetString("forward-url")
		port, _ := cmd.Flags().GetInt("port")
		insecure, _ := cmd.Flags().GetBool("insecure")

		// Handle --port flag (mutually exclusive with --forward-url when explicitly set)
		forwardURLChanged := cmd.Flags().Changed("forward-url")
		portChanged := cmd.Flags().Changed("port")

		if portChanged && forwardURLChanged {
			return fmt.Errorf("--port and --forward-url are mutually exclusive")
		}

		if portChanged && port > 0 {
			forwardURL = fmt.Sprintf("http://localhost:%d", port)
		}

		// Mutually exclusive auth flags
		if oauthMode && (username != "" || password != "") {
			return fmt.Errorf("--oauth flag is mutually exclusive with --username/--password")
		}

		// Initialize Store
		store, err := loadAuthStore()
		if err != nil || store == nil {
			store = &AuthTokenStore{}
		}

		// PERSISTENCE LOGIC overrides
		if username != "" && password != "" {
			// Explicit Basic Auth -> Overwrite Store
			fmt.Printf("Using provided Basic Auth credentials for user: %s\n", username)
			store.ClearOIDC()
			store.BasicAuthUsername = username
			store.BasicAuthPassword = password
			if err := saveAuthStore(store); err != nil {
				return fmt.Errorf("failed to save credentials: %w", err)
			}
		} else if oauthMode {
			// Explicit OAuth -> Clear Basic Auth in Store
			// Logic handled by AuthManager? No, better do it here to ensure "Last Write Wins" semantics
			if store.BasicAuthUsername != "" {
				fmt.Println("Switching to OAuth mode (clearing Basic Auth)...")
				store.ClearBasicAuth()
				if err := saveAuthStore(store); err != nil {
					return fmt.Errorf("failed to save auth state: %w", err)
				}
			}
		}

		// AUTH MANAGER LOGIC
		authManager := NewAuthManager(store)
		
		// Get Header (Force OAuth if flag set, otherwise infer from store)
		// If explicit Basic Auth flags were passed, they are now in the store, so GetAuthHeader will pick them up
		// unless we force OIDC. But wait, if we passed basic auth flags, we want basic auth.
		// If we passed --oauth, we want OAuth. 
		// If we passed NOTHING, we want whatever is in store.

		// If username/password flags were set, we definitely don't want to force OAuth.
		// If oauth flag was set, we force OAuth.
		
		ctx := context.Background()
		authHeader, err := authManager.GetAuthHeader(ctx, true, oauthMode)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Connect to streaming gateway via gRPC
		fmt.Printf("🔌 Connecting to gateway %s...\n", gatewayAddr)
		conn, err := NewGRPCConnection(gatewayAddr, insecure)
		if err != nil {
			return fmt.Errorf("failed to connect to gateway: %w", err)
		}
		defer conn.Close()

		client := streamingv1.NewStreamingServiceClient(conn)

		// Create authenticated context
		ctx = AuthenticatedContext(ctx, authHeader)
		
		// Determine stream ID
		var streamID string
		if len(args) > 0 {
			streamID = args[0]
		} else {
			// List available streams and prompt user
			streamID, err = selectStream(ctx, client)
			if err != nil {
				return fmt.Errorf("failed to select stream: %w", err)
			}
		}

		// Parse replay flags
		fromStr, _ := cmd.Flags().GetString("from")
		toStr, _ := cmd.Flags().GetString("to")

		var fromTimestamp *timestamppb.Timestamp
		if fromStr != "" {
			t, err := parseTimestampOrDuration(fromStr)
			if err != nil {
				return fmt.Errorf("invalid --from value: %w", err)
			}
			fromTimestamp = timestamppb.New(t)
		}

		var toTimestamp *timestamppb.Timestamp
		if toStr != "" {
			t, err := parseTimestampOrDuration(toStr)
			if err != nil {
				return fmt.Errorf("invalid --to value: %w", err)
			}
			toTimestamp = timestamppb.New(t)
		}

		// Validate Replay Range
		if fromTimestamp != nil && toTimestamp != nil {
			if !toTimestamp.AsTime().After(fromTimestamp.AsTime()) {
				return fmt.Errorf("invalid time range: --to must be after --from")
			}
		}

		stream, err := client.OpenStream(ctx, &streamingv1.OpenStreamRequest{
			StreamId:   streamID,
			ReplayFrom: fromTimestamp,
			ReplayTo:   toTimestamp,
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

// selectStream lists available streams and prompts user to select one
func selectStream(ctx context.Context, client streamingv1.StreamingServiceClient) (string, error) {
	resp, err := client.ListStreams(ctx, &streamingv1.ListStreamsRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to list streams: %w", err)
	}

	if len(resp.Streams) == 0 {
		return "", fmt.Errorf("no streams available")
	}

	fmt.Println("\n📋 Available streams:")
	for i, s := range resp.Streams {
		desc := s.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("  %d. %s - %s\n", i+1, s.Name, desc)
	}

	fmt.Print("\nSelect a stream (1-", len(resp.Streams), "): ")
	var selection int
	_, err = fmt.Scanf("%d", &selection)
	if err != nil || selection < 1 || selection > len(resp.Streams) {
		return "", fmt.Errorf("invalid selection")
	}

	return resp.Streams[selection-1].Name, nil
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

// parseTimestampOrDuration parses a string as either RFC3339 timestamp or a duration subtraction from now
func parseTimestampOrDuration(value string) (time.Time, error) {
	// Try parsing as duration first (e.g., "1h", "10m")
	if d, err := time.ParseDuration(value); err == nil {
		return time.Now().Add(-d), nil
	}
	// Try parsing as RFC3339
	return time.Parse(time.RFC3339, value)
}

func init() {
	streamCmd.Flags().String("gateway", "localhost:8081", "Streaming Gateway Address (host:port)")
	streamCmd.Flags().String("username", "", "Username for basic auth")
	streamCmd.Flags().String("password", "", "Password for basic auth")
	streamCmd.Flags().String("forward-url", "http://localhost:3001", "URL to forward requests to")
	streamCmd.Flags().Int("port", 0, "Local port to forward to (alternative to --forward-url)")
	streamCmd.Flags().Int("forward-timeout", 30, "Timeout in seconds for forwarding requests (default: 30)")
	streamCmd.Flags().Int("max-retries", 3, "Maximum number of retries for failed forwards (default: 3)")
	streamCmd.Flags().Bool("insecure", false, "Use insecure connection (no TLS)")
	streamCmd.Flags().String("from", "", "Start replay from timestamp (RFC3339) or duration relative to now (e.g. 1h)")
	streamCmd.Flags().String("to", "", "End replay at timestamp (RFC3339)")
	streamCmd.Flags().Bool("oauth", false, "Force OIDC authentication (ignores/clears basic auth)")

	rootCmd.AddCommand(streamCmd)
}

