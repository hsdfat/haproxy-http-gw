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

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// APIServer provides HTTP API for gateway configuration
type APIServer struct {
	gateway *HTTPGateway
	server  *http.Server
	routes  map[string]RouteConfig
	mu      sync.RWMutex
}

// RouteConfig represents a routing rule
type RouteConfig struct {
	Host    string `json:"host"`
	Path    string `json:"path"`
	Backend string `json:"backend"`
}

// RouteResponse represents the API response for route operations
type RouteResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Route   *RouteConfig `json:"route,omitempty"`
	Routes  []RouteConfig `json:"routes,omitempty"`
}

// NewAPIServer creates a new API server for gateway configuration
func NewAPIServer(gateway *HTTPGateway, port int) *APIServer {
	api := &APIServer{
		gateway: gateway,
		routes:  make(map[string]RouteConfig),
	}

	mux := http.NewServeMux()

	// Route management endpoints
	mux.HandleFunc("POST /api/routes", api.addRoute)
	mux.HandleFunc("GET /api/routes", api.listRoutes)
	mux.HandleFunc("DELETE /api/routes", api.deleteRoute)

	// Health endpoint
	mux.HandleFunc("GET /health", api.health)

	api.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return api
}

// Start starts the API server
func (a *APIServer) Start() error {
	logger.Infof("Starting Gateway API server on %s", a.server.Addr)
	return a.server.ListenAndServe()
}

// Stop stops the API server
func (a *APIServer) Stop() error {
	logger.Info("Stopping Gateway API server")
	return a.server.Close()
}

// addRoute handles POST /api/routes
func (a *APIServer) addRoute(w http.ResponseWriter, r *http.Request) {
	var route RouteConfig
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		a.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate input
	if route.Backend == "" {
		a.sendError(w, http.StatusBadRequest, "Backend is required")
		return
	}
	if route.Host == "" && route.Path == "" {
		a.sendError(w, http.StatusBadRequest, "Either host or path must be specified")
		return
	}

	// Serialize route additions to prevent concurrent HAProxy configuration changes
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add route to gateway
	if err := a.gateway.AddBackendRoute(route.Host, route.Path, route.Backend); err != nil {
		a.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add route: %v", err))
		return
	}

	// Store route in memory
	key := fmt.Sprintf("%s:%s", route.Host, route.Path)
	a.routes[key] = route

	logger.Infof("Route added via API: %s%s -> %s", route.Host, route.Path, route.Backend)

	a.sendSuccess(w, "Route added successfully", &route)
}

// listRoutes handles GET /api/routes
func (a *APIServer) listRoutes(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	routes := make([]RouteConfig, 0, len(a.routes))
	for _, route := range a.routes {
		routes = append(routes, route)
	}
	a.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RouteResponse{
		Success: true,
		Routes:  routes,
	})
}

// deleteRoute handles DELETE /api/routes?host=X&path=Y
func (a *APIServer) deleteRoute(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	path := r.URL.Query().Get("path")

	if host == "" && path == "" {
		a.sendError(w, http.StatusBadRequest, "Either host or path query parameter is required")
		return
	}

	key := fmt.Sprintf("%s:%s", host, path)

	a.mu.Lock()
	delete(a.routes, key)
	a.mu.Unlock()

	// Note: Actual HAProxy route deletion would require additional implementation
	logger.Infof("Route deleted via API: %s%s", host, path)

	a.sendSuccess(w, "Route deleted successfully", nil)
}

// health handles GET /health
func (a *APIServer) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// sendSuccess sends a success response
func (a *APIServer) sendSuccess(w http.ResponseWriter, message string, route *RouteConfig) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RouteResponse{
		Success: true,
		Message: message,
		Route:   route,
	})
}

// sendError sends an error response
func (a *APIServer) sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(RouteResponse{
		Success: false,
		Message: message,
	})
}
