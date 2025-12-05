package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Response struct {
	Server    string            `json:"server"`
	Timestamp time.Time         `json:"timestamp"`
	Path      string            `json:"path"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	Protocol  string            `json:"protocol"`
}

func main() {
	serverName := os.Getenv("SERVER_NAME")
	if serverName == "" {
		serverName = "unknown-server"
	}

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "9000"
	}

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Echo endpoint - returns request details
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		headers := make(map[string]string)
		for name, values := range r.Header {
			if len(values) > 0 {
				headers[name] = values[0]
			}
		}

		response := Response{
			Server:    serverName,
			Timestamp: time.Now(),
			Path:      r.URL.Path,
			Method:    r.Method,
			Headers:   headers,
			Protocol:  r.Proto,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend-Server", serverName)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
		}

		log.Printf("[%s] %s %s from %s", serverName, r.Method, r.URL.Path, r.RemoteAddr)
	})

	// Create HTTP/2 server with h2c (HTTP/2 cleartext) support
	h2s := &http2.Server{}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: h2c.NewHandler(mux, h2s),
	}

	log.Printf("Starting backend server '%s' on port %s with h2c (HTTP/2 cleartext) support", serverName, port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
