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

	"github.com/google/uuid"
)

// EnhancedAPIServer provides HTTP API for frontend management
type EnhancedAPIServer struct {
	frontendManager *FrontendManager
	server          *http.Server
	mu              sync.RWMutex
}

// RouteRequest represents a route addition request
type RouteRequest struct {
	Host        string `json:"host"`
	Path        string `json:"path"`
	BackendName string `json:"backend_name"`
}

// RouteAPIResponse represents the response for route operations in enhanced API
type RouteAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Route   *Route `json:"route,omitempty"`
}

// FrontendResponse represents a frontend in API responses
type FrontendResponse struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Enabled        bool                `json:"enabled"`
	Mode           string              `json:"mode"`
	Bindings       []BindingDefinition `json:"bindings"`
	Routing        RoutingConfig       `json:"routing"`
	BackendsCount  int                 `json:"backends_count"`
	RoutesCount    int                 `json:"routes_count"`
}

// NewEnhancedAPIServer creates a new API server for frontend management
func NewEnhancedAPIServer(frontendMgr *FrontendManager, port int) *EnhancedAPIServer {
	api := &EnhancedAPIServer{
		frontendManager: frontendMgr,
	}

	mux := http.NewServeMux()

	// Frontend management endpoints
	mux.HandleFunc("GET /api/frontends", api.listFrontends)
	mux.HandleFunc("GET /api/frontends/{id}", api.getFrontend)
	mux.HandleFunc("GET /api/frontends/{id}/stats", api.getFrontendStats)

	// Backend registration endpoints (per frontend)
	mux.HandleFunc("POST /api/frontends/{id}/backends", api.registerBackend)
	mux.HandleFunc("GET /api/frontends/{id}/backends", api.listBackends)
	mux.HandleFunc("DELETE /api/frontends/{id}/backends/{name}", api.unregisterBackend)

	// Route management endpoints (per frontend)
	mux.HandleFunc("POST /api/frontends/{id}/routes", api.addRoute)
	mux.HandleFunc("GET /api/frontends/{id}/routes", api.listRoutes)
	mux.HandleFunc("DELETE /api/frontends/{id}/routes/{route_id}", api.deleteRoute)

	// Health endpoint
	mux.HandleFunc("GET /health", api.health)
	mux.HandleFunc("GET /api/health", api.health)

	api.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return api
}

// Start starts the API server
func (a *EnhancedAPIServer) Start() error {
	logger.Infof("Starting Enhanced Gateway API server on %s", a.server.Addr)
	return a.server.ListenAndServe()
}

// Stop stops the API server
func (a *EnhancedAPIServer) Stop() error {
	logger.Info("Stopping Enhanced Gateway API server")
	return a.server.Close()
}

// listFrontends handles GET /api/frontends
func (a *EnhancedAPIServer) listFrontends(w http.ResponseWriter, r *http.Request) {
	frontends := a.frontendManager.ListFrontends()

	response := make([]FrontendResponse, 0, len(frontends))
	for _, mf := range frontends {
		backends := mf.Manager.GetBackends()
		response = append(response, FrontendResponse{
			ID:            mf.Definition.ID,
			Name:          mf.Definition.Name,
			Enabled:       mf.Definition.Enabled,
			Mode:          mf.Definition.Mode,
			Bindings:      mf.Definition.Bindings,
			Routing:       mf.Definition.Routing,
			BackendsCount: len(backends),
			RoutesCount:   len(mf.Routes),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"frontends": response,
	})
}

// getFrontend handles GET /api/frontends/{id}
func (a *EnhancedAPIServer) getFrontend(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	if frontendID == "" {
		a.sendError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}

	mf, err := a.frontendManager.GetFrontend(frontendID)
	if err != nil {
		a.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	backends := mf.Manager.GetBackends()
	response := FrontendResponse{
		ID:            mf.Definition.ID,
		Name:          mf.Definition.Name,
		Enabled:       mf.Definition.Enabled,
		Mode:          mf.Definition.Mode,
		Bindings:      mf.Definition.Bindings,
		Routing:       mf.Definition.Routing,
		BackendsCount: len(backends),
		RoutesCount:   len(mf.Routes),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"frontend": response,
	})
}

// getFrontendStats handles GET /api/frontends/{id}/stats
func (a *EnhancedAPIServer) getFrontendStats(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	if frontendID == "" {
		a.sendError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}

	mf, err := a.frontendManager.GetFrontend(frontendID)
	if err != nil {
		a.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	stats := mf.GetStats()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// registerBackend handles POST /api/frontends/{id}/backends
func (a *EnhancedAPIServer) registerBackend(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	if frontendID == "" {
		a.sendBackendError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}

	var req BackendRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.sendBackendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate input
	if req.Name == "" {
		a.sendBackendError(w, http.StatusBadRequest, "Backend name is required")
		return
	}
	if len(req.Servers) == 0 {
		a.sendBackendError(w, http.StatusBadRequest, "At least one server is required")
		return
	}

	// Validate servers
	for i, srv := range req.Servers {
		if srv.Name == "" {
			a.sendBackendError(w, http.StatusBadRequest, fmt.Sprintf("Server %d: name is required", i))
			return
		}
		if srv.IP == "" {
			a.sendBackendError(w, http.StatusBadRequest, fmt.Sprintf("Server %s: IP is required", srv.Name))
			return
		}
		if srv.Port <= 0 || srv.Port > 65535 {
			a.sendBackendError(w, http.StatusBadRequest, fmt.Sprintf("Server %s: invalid port %d", srv.Name, srv.Port))
			return
		}
	}

	// Convert to backend format
	servers := make([]BackendServer, len(req.Servers))
	for i, srv := range req.Servers {
		servers[i] = BackendServer{
			Name: srv.Name,
			IP:   srv.IP,
			Port: srv.Port,
		}
	}

	backend := Backend{
		Name:    req.Name,
		Servers: servers,
	}

	// Register backend with the frontend
	if err := a.frontendManager.RegisterBackend(frontendID, backend); err != nil {
		a.sendBackendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to register backend: %v", err))
		return
	}

	logger.Infof("Backend registered via API: %s to frontend %s with %d servers", req.Name, frontendID, len(servers))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(BackendRegistrationResponse{
		Success: true,
		Message: "Backend registered successfully",
		Backend: req.Name,
	})
}

// listBackends handles GET /api/frontends/{id}/backends
func (a *EnhancedAPIServer) listBackends(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	if frontendID == "" {
		a.sendError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}

	backends, err := a.frontendManager.GetBackends(frontendID)
	if err != nil {
		a.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"backends": backends,
	})
}

// unregisterBackend handles DELETE /api/frontends/{id}/backends/{name}
func (a *EnhancedAPIServer) unregisterBackend(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	backendName := r.PathValue("name")

	if frontendID == "" {
		a.sendBackendError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}
	if backendName == "" {
		a.sendBackendError(w, http.StatusBadRequest, "Backend name is required")
		return
	}

	if err := a.frontendManager.UnregisterBackend(frontendID, backendName); err != nil {
		a.sendBackendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to unregister backend: %v", err))
		return
	}

	logger.Infof("Backend unregistered via API: %s from frontend %s", backendName, frontendID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(BackendRegistrationResponse{
		Success: true,
		Message: "Backend unregistered successfully",
		Backend: backendName,
	})
}

// addRoute handles POST /api/frontends/{id}/routes
func (a *EnhancedAPIServer) addRoute(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	if frontendID == "" {
		a.sendRouteError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}

	var req RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.sendRouteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate input
	if req.BackendName == "" {
		a.sendRouteError(w, http.StatusBadRequest, "Backend name is required")
		return
	}
	if req.Host == "" && req.Path == "" {
		a.sendRouteError(w, http.StatusBadRequest, "Either host or path must be specified")
		return
	}

	// Create route with unique ID
	route := Route{
		ID:          uuid.New().String(),
		Host:        req.Host,
		Path:        req.Path,
		BackendName: req.BackendName,
		FrontendID:  frontendID,
	}

	// Add route to frontend
	if err := a.frontendManager.AddRoute(frontendID, route); err != nil {
		a.sendRouteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add route: %v", err))
		return
	}

	logger.Infof("Route added via API to frontend %s: %s%s -> %s", frontendID, req.Host, req.Path, req.BackendName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RouteAPIResponse{
		Success: true,
		Message: "Route added successfully",
		Route:   &route,
	})
}

// listRoutes handles GET /api/frontends/{id}/routes
func (a *EnhancedAPIServer) listRoutes(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	if frontendID == "" {
		a.sendError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}

	routes, err := a.frontendManager.GetRoutes(frontendID)
	if err != nil {
		a.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	// Convert map to slice for JSON response
	routesList := make([]Route, 0, len(routes))
	for _, route := range routes {
		routesList = append(routesList, route)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"routes":  routesList,
	})
}

// deleteRoute handles DELETE /api/frontends/{id}/routes/{route_id}
func (a *EnhancedAPIServer) deleteRoute(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	routeID := r.PathValue("route_id")

	if frontendID == "" {
		a.sendRouteError(w, http.StatusBadRequest, "Frontend ID is required")
		return
	}
	if routeID == "" {
		a.sendRouteError(w, http.StatusBadRequest, "Route ID is required")
		return
	}

	if err := a.frontendManager.DeleteRoute(frontendID, routeID); err != nil {
		a.sendRouteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete route: %v", err))
		return
	}

	logger.Infof("Route deleted via API: %s from frontend %s", routeID, frontendID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RouteAPIResponse{
		Success: true,
		Message: "Route deleted successfully",
	})
}

// health handles GET /health and GET /api/health
func (a *EnhancedAPIServer) health(w http.ResponseWriter, r *http.Request) {
	frontends := a.frontendManager.ListFrontends()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "healthy",
		"frontends_count":  len(frontends),
	})
}

// sendError sends a generic error response
func (a *EnhancedAPIServer) sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": message,
	})
}

// sendBackendError sends an error response for backend operations
func (a *EnhancedAPIServer) sendBackendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(BackendRegistrationResponse{
		Success: false,
		Message: message,
	})
}

// sendRouteError sends an error response for route operations
func (a *EnhancedAPIServer) sendRouteError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(RouteAPIResponse{
		Success: false,
		Message: message,
	})
}
