package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Redefine locally to avoid cross-module build issues in testcontainers
type StreamMessage struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Body    string            `json:"body,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
}

func main() {
	http.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		streamID := r.URL.Query().Get("stream_id")
		if streamID == "" {
			http.Error(w, "Missing stream_id", http.StatusBadRequest)
			return
		}

		log.Printf("Client connected to stream: %s", streamID)

		// Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Flush headers
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		// Send a few test messages
		for i := 0; i < 3; i++ {
			msg := StreamMessage{
				Method: "POST",
				Path:   "/webhook",
				Body:   fmt.Sprintf(`{"test": "message %d"}`, i),
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			}

			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Error marshalling message: %v", err)
				continue
			}

			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			time.Sleep(500 * time.Millisecond)
		}

		// Keep connection open for a bit
		time.Sleep(2 * time.Second)
	})

	log.Println("Mock Gateway running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
