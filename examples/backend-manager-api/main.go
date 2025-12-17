// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/haproxytech/kubernetes-ingress/pkg/client"
	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
)

// Server wraps the client library and provides HTTP endpoints
type Server struct {
	client *client.Client
	port   string
}

// RegisterBackendRequest represents the JSON request body for backend registration
type RegisterBackendRequest struct {
	FrontendID string                  `json:"frontend_id"`
	Backend    gateway.Backend         `json:"backend"`
}

// AddServerRequest represents the JSON request for adding a single server
type AddServerRequest struct {
	FrontendID  string                `json:"frontend_id"`
	BackendName string                `json:"backend_name"`
	Server      gateway.BackendServer `json:"server"`
}

// RemoveServerRequest represents the JSON request for removing a server
type RemoveServerRequest struct {
	FrontendID  string `json:"frontend_id"`
	BackendName string `json:"backend_name"`
	ServerName  string `json:"server_name"`
}

// UpdateServerRequest represents the JSON request for updating server address
type UpdateServerRequest struct {
	FrontendID  string `json:"frontend_id"`
	BackendName string `json:"backend_name"`
	ServerName  string `json:"server_name"`
	NewIP       string `json:"new_ip"`
	NewPort     int    `json:"new_port"`
}

// Response represents a standard JSON response
type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func NewServer(c *client.Client, port string) *Server {
	return &Server{
		client: c,
		port:   port,
	}
}

// RegisterBackend handles POST /api/backends/register
func (s *Server) RegisterBackend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req RegisterBackendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if err := s.client.RegisterBackend(req.FrontendID, req.Backend); err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to register backend: %v", err))
		return
	}

	sendSuccess(w, "Backend registered successfully", map[string]string{
		"frontend": req.FrontendID,
		"backend":  req.Backend.Name,
	})
}

// UnregisterBackend handles DELETE /api/backends/{frontendID}/{backendName}
func (s *Server) UnregisterBackend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	frontendID := r.URL.Query().Get("frontend_id")
	backendName := r.URL.Query().Get("backend_name")

	if frontendID == "" || backendName == "" {
		sendError(w, http.StatusBadRequest, "frontend_id and backend_name are required")
		return
	}

	if err := s.client.UnregisterBackend(frontendID, backendName); err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to unregister backend: %v", err))
		return
	}

	sendSuccess(w, "Backend unregistered successfully", map[string]string{
		"frontend": frontendID,
		"backend":  backendName,
	})
}

// GetBackends handles GET /api/backends
func (s *Server) GetBackends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	frontendID := r.URL.Query().Get("frontend_id")
	if frontendID == "" {
		frontendID = "default"
	}

	backends, err := s.client.GetBackends(frontendID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get backends: %v", err))
		return
	}

	sendSuccess(w, "Backends retrieved successfully", backends)
}

// AddServer handles POST /api/servers/add
func (s *Server) AddServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AddServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if err := s.client.AddServer(req.FrontendID, req.BackendName, req.Server); err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add server: %v", err))
		return
	}

	sendSuccess(w, "Server added successfully", map[string]string{
		"frontend": req.FrontendID,
		"backend":  req.BackendName,
		"server":   req.Server.Name,
	})
}

// RemoveServer handles POST /api/servers/remove
func (s *Server) RemoveServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req RemoveServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if err := s.client.RemoveServer(req.FrontendID, req.BackendName, req.ServerName); err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to remove server: %v", err))
		return
	}

	sendSuccess(w, "Server removed successfully", map[string]string{
		"frontend": req.FrontendID,
		"backend":  req.BackendName,
		"server":   req.ServerName,
	})
}

// UpdateServer handles POST /api/servers/update
func (s *Server) UpdateServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req UpdateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if err := s.client.UpdateServerAddress(req.FrontendID, req.BackendName, req.ServerName, req.NewIP, req.NewPort); err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update server: %v", err))
		return
	}

	sendSuccess(w, "Server updated successfully", map[string]interface{}{
		"frontend": req.FrontendID,
		"backend":  req.BackendName,
		"server":   req.ServerName,
		"new_ip":   req.NewIP,
		"new_port": req.NewPort,
	})
}

// Health check endpoint
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	sendSuccess(w, "healthy", map[string]string{"status": "ok"})
}

func (s *Server) Start() error {
	http.HandleFunc("/health", s.Health)
	http.HandleFunc("/api/backends/register", s.RegisterBackend)
	http.HandleFunc("/api/backends/unregister", s.UnregisterBackend)
	http.HandleFunc("/api/backends", s.GetBackends)
	http.HandleFunc("/api/servers/add", s.AddServer)
	http.HandleFunc("/api/servers/remove", s.RemoveServer)
	http.HandleFunc("/api/servers/update", s.UpdateServer)

	log.Printf("Backend Manager API server starting on port %s", s.port)
	log.Printf("Endpoints:")
	log.Printf("  GET  /health")
	log.Printf("  POST /api/backends/register")
	log.Printf("  DELETE /api/backends/unregister?frontend_id=X&backend_name=Y")
	log.Printf("  GET  /api/backends?frontend_id=X")
	log.Printf("  POST /api/servers/add")
	log.Printf("  POST /api/servers/remove")
	log.Printf("  POST /api/servers/update")

	return http.ListenAndServe(":"+s.port, nil)
}

func sendSuccess(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func sendError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Success: false,
		Message: message,
	})
}

func main() {
	// NOTE: This is a skeleton - you need to inject the FrontendManager
	// In a real implementation, this would be part of the main gateway process
	// or would connect to a running gateway via gRPC/IPC

	log.Println("Backend Manager API Example")
	log.Println("=============================")
	log.Println()
	log.Println("This example demonstrates how to build an HTTP API using the client library")
	log.Println("In a production setup, this would be integrated with the gateway process")
	log.Println()

	port := os.Getenv("PORT")
	if port == "" {
		port = "9091"
	}

	// TODO: Connect to gateway's FrontendManager
	// For now, this is just a skeleton showing the API structure
	log.Fatal("FrontendManager injection not implemented yet")

	// Example of how it would work:
	// var frontendManager *gateway.FrontendManager
	// c := client.NewClient(frontendManager)
	// server := NewServer(c, port)
	// log.Fatal(server.Start())
}
