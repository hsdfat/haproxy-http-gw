package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type Server struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type Backend struct {
	Name    string   `json:"name"`
	Servers []Server `json:"servers"`
}

type BackendsResponse struct {
	Backends []Backend `json:"backends"`
}

type BackendStore struct {
	mu       sync.RWMutex
	backends map[string]Backend
}

func NewBackendStore() *BackendStore {
	return &BackendStore{
		backends: make(map[string]Backend),
	}
}

func (s *BackendStore) GetAll() []Backend {
	s.mu.RLock()
	defer s.mu.RUnlock()

	backends := make([]Backend, 0, len(s.backends))
	for _, backend := range s.backends {
		backends = append(backends, backend)
	}
	return backends
}

func (s *BackendStore) Get(name string) (Backend, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	backend, ok := s.backends[name]
	return backend, ok
}

func (s *BackendStore) Set(backend Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.backends[backend.Name] = backend
}

func (s *BackendStore) Delete(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.backends, name)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	store := NewBackendStore()

	// Initialize with default backends
	store.Set(Backend{
		Name: "api-backend",
		Servers: []Server{
			{Name: "backend-server-1", IP: "backend-server-1", Port: 9000},
			{Name: "backend-server-2", IP: "backend-server-2", Port: 9000},
		},
	})

	store.Set(Backend{
		Name: "web-backend",
		Servers: []Server{
			{Name: "web-server-1", IP: "web-server-1", Port: 9000},
			{Name: "web-server-2", IP: "web-server-2", Port: 9000},
		},
	})

	mux := http.NewServeMux()

	// GET /backends - List all backends
	mux.HandleFunc("GET /backends", func(w http.ResponseWriter, r *http.Request) {
		backends := store.GetAll()
		response := BackendsResponse{Backends: backends}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

		log.Printf("GET /backends - returned %d backends", len(backends))
	})

	// GET /backends/{name} - Get specific backend
	mux.HandleFunc("GET /backends/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		backend, ok := store.Get(name)

		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "backend not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(backend)

		log.Printf("GET /backends/%s - found", name)
	})

	// POST /backends - Create or update backend
	mux.HandleFunc("POST /backends", func(w http.ResponseWriter, r *http.Request) {
		var backend Backend
		if err := json.NewDecoder(r.Body).Decode(&backend); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		store.Set(backend)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(backend)

		log.Printf("POST /backends - created/updated backend '%s' with %d servers", backend.Name, len(backend.Servers))
	})

	// DELETE /backends/{name} - Delete backend
	mux.HandleFunc("DELETE /backends/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		store.Delete(name)

		w.WriteHeader(http.StatusNoContent)

		log.Printf("DELETE /backends/%s - deleted", name)
	})

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Backend API server starting on port %s", port)
	log.Printf("Endpoints:")
	log.Printf("  GET    /backends")
	log.Printf("  GET    /backends/{name}")
	log.Printf("  POST   /backends")
	log.Printf("  DELETE /backends/{name}")
	log.Printf("  GET    /health")

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
