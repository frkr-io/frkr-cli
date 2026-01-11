package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
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
		gatewayURL, _ := cmd.Flags().GetString("gateway-url")
		username, _ := cmd.Flags().GetString("username")
		password, _ := cmd.Flags().GetString("password")
		forwardURL, _ := cmd.Flags().GetString("forward-url")

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

		// Connect to streaming gateway
		url := fmt.Sprintf("%s/stream?stream_id=%s", gatewayURL, streamID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to connect to gateway: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("gateway error: %s - %s", resp.Status, string(body))
		}

		forwardTimeout, _ := cmd.Flags().GetInt("forward-timeout")
		maxRetries, _ := cmd.Flags().GetInt("max-retries")

		fmt.Printf("✅ Connected to stream: %s\n", streamID)
		fmt.Printf("📡 Forwarding to: %s\n", forwardURL)
		fmt.Printf("⏱️  Timeout: %ds, Max retries: %d\n", forwardTimeout, maxRetries)
		fmt.Println("Waiting for messages...")

		// Read SSE stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if err := forwardMessageWithRetry(data, forwardURL, forwardTimeout, maxRetries); err != nil {
					fmt.Fprintf(os.Stderr, "❌ Error forwarding message after retries: %v\n", err)
					continue
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stream: %w", err)
		}

		return nil
	},
}

func forwardMessageWithRetry(data, forwardURL string, timeoutSeconds, maxRetries int) error {
	var msg streamingv1.StreamMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

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
			// Server errors could be retried, but we'll consider them handled
			fmt.Printf("➡️  %s %s -> %d\n", msg.Method, msg.Path, resp.StatusCode)
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
}

func init() {
	streamCmd.Flags().String("gateway-url", "http://localhost:8081", "Streaming Gateway URL")
	streamCmd.Flags().String("username", "", "Username for basic auth")
	streamCmd.Flags().String("password", "", "Password for basic auth")
	streamCmd.Flags().String("forward-url", "http://localhost:3001", "URL to forward requests to")
	streamCmd.Flags().Int("forward-timeout", 30, "Timeout in seconds for forwarding requests (default: 30)")
	streamCmd.Flags().Int("max-retries", 3, "Maximum number of retries for failed forwards (default: 3)")

	rootCmd.AddCommand(streamCmd)
}
