package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestStreamCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// 1. Build and Run Mock Gateway using Testcontainers
	absPath, err := filepath.Abs("mock-gateway")
	if err != nil {
		t.Fatal(err)
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    absPath,
			Dockerfile: "Dockerfile",
			KeepImage:  false,
		},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForLog("Mock Gateway running on :8080"),
	}

	gatewayContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start mock gateway: %v", err)
	}
	defer gatewayContainer.Terminate(ctx)

	endpoint, err := gatewayContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatal(err)
	}


	// 2. Start a local server to receive forwarded requests
	receivedMessages := make(chan string, 10)
	
	// Setup listener manually to get the port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	
	forwardPort := listener.Addr().(*net.TCPAddr).Port
	forwardURL := fmt.Sprintf("http://localhost:%d", forwardPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedMessages <- string(body)
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{Handler: mux}
	go func() {
		server.Serve(listener)
	}()
	defer server.Close()

	// 3. Run the frkr CLI
	// We assume we are running from the project root or tests dir.
	// We'll use "go run" to run the CLI to avoid needing a pre-built binary.
	// The path to main should be adjusted.
	// Since this test is in /tests/integration, the repo root is ../../
	
	repoRoot, err := filepath.Abs("../../")
	if err != nil {
		t.Fatal(err)
	}
	
	cmd := exec.Command("go", "run", "./cmd/frkr", "stream", "test-stream",
		"--gateway", endpoint,
		"--username", "user",
		"--password", "pass",
		"--forward-url", forwardURL,
		"--forward-timeout", "5",
		"--insecure",
	)
	cmd.Dir = repoRoot
	
	// Capture output for debugging
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start CLI: %v", err)
	}
	
	// Ensure we kill the process
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// 4. Verify we received messages
	// We expect 3 messages from the mock gateway
	timeout := time.After(10 * time.Second)
	messageCount := 0
	
	for messageCount < 3 {
		select {
		case msg := <-receivedMessages:
			t.Logf("Received forwarded message: %s", msg)
			messageCount++
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received %d/3", messageCount)
		}
	}

	t.Log("Successfully received all messages!")
}
