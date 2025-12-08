# Frontend Management Design

## Overview

This document describes the design for a configurable frontend management system for the HAProxy HTTP Gateway. The design allows for multiple frontends, each with its own configuration, binding options, routing rules, and backend associations.

## Goals

1. **Configurability**: Support multiple frontends via configuration file
2. **Flexible Binding**: Each frontend can bind to different addresses/ports
3. **Default Bypass**: Support bypass/passthrough mode for frontends
4. **Default Backend**: Each frontend can have its own default backend
5. **Frontend Identification**: Unique identification for registration/deregistration
6. **Backend Lists**: Dynamic backend registration per frontend

## Architecture

### Configuration Structure

```yaml
# frontend-config.yaml
frontends:
  - id: "http-frontend-1"
    name: "http-gateway-1"
    enabled: true
    mode: "http"

    # Binding configuration
    bindings:
      - address: "0.0.0.0"
        port: 8080
        protocol: "http"
        http2: true
        ipv6: false
      - address: "0.0.0.0"
        port: 8443
        protocol: "https"
        http2: true
        ssl: true
        alpn: "h2,http/1.1"
        cert_dir: "/etc/haproxy/certs"
        ipv6: false

    # Routing configuration
    routing:
      bypass_rules: false  # If true, skip all ACL/routing rules
      default_backend: "default-backend-1"

    # Additional options
    options:
      max_connections: 10000
      timeout_client: "30s"
      http_connection_mode: "http-keep-alive"

  - id: "http-frontend-2"
    name: "http-gateway-2"
    enabled: true
    mode: "http"

    bindings:
      - address: "0.0.0.0"
        port: 9080
        protocol: "http"
        http2: true
        ipv6: false

    routing:
      bypass_rules: true  # Passthrough mode - no ACL processing
      default_backend: "default-backend-2"

    options:
      max_connections: 5000
      timeout_client: "60s"

  - id: "tcp-frontend-1"
    name: "tcp-gateway-1"
    enabled: true
    mode: "tcp"

    bindings:
      - address: "0.0.0.0"
        port: 5000
        protocol: "tcp"

    routing:
      default_backend: "tcp-backend-1"
```

### Data Structures

#### Go Configuration Types

```go
package gateway

import "time"

// FrontendConfig represents the complete frontend configuration
type FrontendConfig struct {
    Frontends []FrontendDefinition `yaml:"frontends" json:"frontends"`
}

// FrontendDefinition defines a single frontend
type FrontendDefinition struct {
    ID       string              `yaml:"id" json:"id"`             // Unique identifier
    Name     string              `yaml:"name" json:"name"`         // HAProxy frontend name
    Enabled  bool                `yaml:"enabled" json:"enabled"`   // Enable/disable frontend
    Mode     string              `yaml:"mode" json:"mode"`         // "http" or "tcp"
    Bindings []BindingDefinition `yaml:"bindings" json:"bindings"` // Bind configurations
    Routing  RoutingConfig       `yaml:"routing" json:"routing"`   // Routing rules
    Options  FrontendOptions     `yaml:"options" json:"options"`   // Additional options
}

// BindingDefinition defines how a frontend binds to network
type BindingDefinition struct {
    Address  string `yaml:"address" json:"address"`   // IP address (0.0.0.0, ::, etc.)
    Port     int    `yaml:"port" json:"port"`         // Port number
    Protocol string `yaml:"protocol" json:"protocol"` // "http", "https", "tcp"
    HTTP2    bool   `yaml:"http2" json:"http2"`       // Enable HTTP/2
    SSL      bool   `yaml:"ssl" json:"ssl"`           // Enable SSL/TLS
    ALPN     string `yaml:"alpn" json:"alpn"`         // ALPN string for HTTP/2
    CertDir  string `yaml:"cert_dir" json:"cert_dir"` // Certificate directory
    IPv6     bool   `yaml:"ipv6" json:"ipv6"`         // Enable IPv6
}

// RoutingConfig defines routing behavior
type RoutingConfig struct {
    BypassRules    bool   `yaml:"bypass_rules" json:"bypass_rules"`       // Skip ACL processing
    DefaultBackend string `yaml:"default_backend" json:"default_backend"` // Default backend name
}

// FrontendOptions holds additional frontend options
type FrontendOptions struct {
    MaxConnections     int           `yaml:"max_connections" json:"max_connections"`
    TimeoutClient      time.Duration `yaml:"timeout_client" json:"timeout_client"`
    HTTPConnectionMode string        `yaml:"http_connection_mode" json:"http_connection_mode"`
}
```

### Frontend Manager

#### Manager Structure

```go
package gateway

import (
    "context"
    "fmt"
    "sync"

    "github.com/haproxytech/client-native/v6/models"
    "github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
)

// FrontendManager manages multiple frontends
type FrontendManager struct {
    haproxyClient api.HAProxyClient
    config        FrontendConfig
    frontends     map[string]*ManagedFrontend
    mu            sync.RWMutex
}

// ManagedFrontend represents a managed frontend instance
type ManagedFrontend struct {
    Definition FrontendDefinition
    Manager    *BackendManager  // Each frontend has its own backend manager
    Routes     map[string]Route // Routing rules for this frontend
}

// Route represents a routing rule in a frontend
type Route struct {
    ID          string   // Unique route ID
    Host        string   // Host pattern
    Path        string   // Path pattern
    BackendName string   // Target backend
    FrontendID  string   // Frontend this route belongs to
}

// NewFrontendManager creates a new frontend manager
func NewFrontendManager(haproxyClient api.HAProxyClient, config FrontendConfig) *FrontendManager {
    return &FrontendManager{
        haproxyClient: haproxyClient,
        config:        config,
        frontends:     make(map[string]*ManagedFrontend),
    }
}

// Start initializes all configured frontends
func (fm *FrontendManager) Start(ctx context.Context) error {
    fm.mu.Lock()
    defer fm.mu.Unlock()

    for _, frontendDef := range fm.config.Frontends {
        if !frontendDef.Enabled {
            logger.Infof("Skipping disabled frontend: %s", frontendDef.ID)
            continue
        }

        if err := fm.startFrontend(ctx, frontendDef); err != nil {
            return fmt.Errorf("failed to start frontend %s: %w", frontendDef.ID, err)
        }
    }

    return nil
}

// startFrontend initializes a single frontend
func (fm *FrontendManager) startFrontend(ctx context.Context, def FrontendDefinition) error {
    logger.Infof("Starting frontend: %s (name=%s)", def.ID, def.Name)

    // Create backend manager for this frontend
    backendMgr := NewManager(ManagerConfig{
        HAProxyClient: fm.haproxyClient,
        Provider:      nil, // Provider can be set later or passed in
        SyncPeriod:    5 * time.Second,
    })

    // Create managed frontend
    mf := &ManagedFrontend{
        Definition: def,
        Manager:    backendMgr,
        Routes:     make(map[string]Route),
    }

    // Configure HAProxy frontend
    if err := fm.configureFrontend(def); err != nil {
        return fmt.Errorf("failed to configure frontend: %w", err)
    }

    // Start backend manager
    if err := backendMgr.Start(ctx); err != nil {
        return fmt.Errorf("failed to start backend manager: %w", err)
    }

    fm.frontends[def.ID] = mf
    logger.Infof("Frontend %s started successfully", def.ID)

    return nil
}

// configureFrontend creates HAProxy frontend configuration
func (fm *FrontendManager) configureFrontend(def FrontendDefinition) error {
    if err := fm.haproxyClient.APIStartTransaction(); err != nil {
        return err
    }
    defer fm.haproxyClient.APIDisposeTransaction()

    // Create frontend
    frontend := models.FrontendBase{
        Name:           def.Name,
        Mode:           def.Mode,
        DefaultBackend: def.Routing.DefaultBackend,
    }

    if def.Options.HTTPConnectionMode != "" {
        frontend.HTTPConnectionMode = def.Options.HTTPConnectionMode
    }

    if def.Options.MaxConnections > 0 {
        frontend.Maxconn = int64(def.Options.MaxConnections)
    }

    // Create or update frontend
    if err := fm.haproxyClient.FrontendCreate(frontend); err != nil {
        if err := fm.haproxyClient.FrontendEdit(frontend); err != nil {
            logger.Debugf("Frontend edit failed: %v", err)
        }
    }

    // Configure bindings
    for i, binding := range def.Bindings {
        if err := fm.createBinding(def.Name, binding, i); err != nil {
            logger.Errorf("Failed to create binding %d for frontend %s: %v", i, def.Name, err)
        }
    }

    return fm.haproxyClient.APIFinalCommitTransaction()
}

// createBinding creates a single bind configuration
func (fm *FrontendManager) createBinding(frontendName string, binding BindingDefinition, index int) error {
    bind := models.Bind{
        BindParams: models.BindParams{
            Name: fmt.Sprintf("bind-%d", index),
        },
        Address: fmt.Sprintf("%s:%d", binding.Address, binding.Port),
    }

    // Configure SSL
    if binding.SSL {
        bind.BindParams.Ssl = true
        if binding.ALPN != "" {
            bind.BindParams.Alpn = binding.ALPN
        }
        if binding.CertDir != "" {
            bind.SslCertificate = binding.CertDir
        }
    }

    // Configure HTTP/2
    if binding.HTTP2 && binding.Protocol == "http" {
        bind.BindParams.Proto = "h2"
    }

    // Configure IPv6
    if binding.IPv6 {
        bind.BindParams.V4v6 = true
    }

    return fm.haproxyClient.FrontendBindCreate(frontendName, bind)
}

// RegisterBackend registers a backend with a specific frontend
func (fm *FrontendManager) RegisterBackend(frontendID string, backend Backend) error {
    fm.mu.RLock()
    mf, exists := fm.frontends[frontendID]
    fm.mu.RUnlock()

    if !exists {
        return fmt.Errorf("frontend %s not found", frontendID)
    }

    return mf.Manager.RegisterBackend(backend)
}

// UnregisterBackend removes a backend from a specific frontend
func (fm *FrontendManager) UnregisterBackend(frontendID string, backendName string) error {
    fm.mu.RLock()
    mf, exists := fm.frontends[frontendID]
    fm.mu.RUnlock()

    if !exists {
        return fmt.Errorf("frontend %s not found", frontendID)
    }

    return mf.Manager.UnregisterBackend(backendName)
}

// AddRoute adds a routing rule to a specific frontend
func (fm *FrontendManager) AddRoute(frontendID string, route Route) error {
    fm.mu.Lock()
    defer fm.mu.Unlock()

    mf, exists := fm.frontends[frontendID]
    if !exists {
        return fmt.Errorf("frontend %s not found", frontendID)
    }

    // Check if bypass mode is enabled
    if mf.Definition.Routing.BypassRules {
        return fmt.Errorf("frontend %s has bypass_rules enabled, routing not allowed", frontendID)
    }

    // Add route configuration to HAProxy
    if err := fm.addRouteToHAProxy(mf.Definition.Name, route); err != nil {
        return err
    }

    // Store route
    mf.Routes[route.ID] = route

    logger.Infof("Route %s added to frontend %s", route.ID, frontendID)
    return nil
}

// addRouteToHAProxy configures routing in HAProxy
func (fm *FrontendManager) addRouteToHAProxy(frontendName string, route Route) error {
    // Implementation similar to HTTPGateway.AddBackendRoute
    // ... (ACL and backend switching rule creation)
    return nil
}

// GetFrontend returns a managed frontend by ID
func (fm *FrontendManager) GetFrontend(frontendID string) (*ManagedFrontend, error) {
    fm.mu.RLock()
    defer fm.mu.RUnlock()

    mf, exists := fm.frontends[frontendID]
    if !exists {
        return nil, fmt.Errorf("frontend %s not found", frontendID)
    }

    return mf, nil
}

// ListFrontends returns all managed frontends
func (fm *FrontendManager) ListFrontends() map[string]*ManagedFrontend {
    fm.mu.RLock()
    defer fm.mu.RUnlock()

    result := make(map[string]*ManagedFrontend, len(fm.frontends))
    for k, v := range fm.frontends {
        result[k] = v
    }
    return result
}

// Stop stops all frontends
func (fm *FrontendManager) Stop() error {
    fm.mu.Lock()
    defer fm.mu.Unlock()

    for id, mf := range fm.frontends {
        if err := mf.Manager.Stop(); err != nil {
            logger.Errorf("Error stopping frontend %s: %v", id, err)
        }
    }

    return nil
}
```

### Enhanced API Server

#### API Endpoints

```go
package gateway

// Enhanced API server to support multiple frontends

type EnhancedAPIServer struct {
    frontendManager *FrontendManager
    server          *http.Server
    mu              sync.RWMutex
}

// NewEnhancedAPIServer creates API server for frontend management
func NewEnhancedAPIServer(frontendMgr *FrontendManager, port int) *EnhancedAPIServer {
    api := &EnhancedAPIServer{
        frontendManager: frontendMgr,
    }

    mux := http.NewServeMux()

    // Frontend management
    mux.HandleFunc("GET /api/frontends", api.listFrontends)
    mux.HandleFunc("GET /api/frontends/{id}", api.getFrontend)

    // Backend registration (per frontend)
    mux.HandleFunc("POST /api/frontends/{id}/backends", api.registerBackend)
    mux.HandleFunc("GET /api/frontends/{id}/backends", api.listBackends)
    mux.HandleFunc("DELETE /api/frontends/{id}/backends/{name}", api.unregisterBackend)

    // Route management (per frontend)
    mux.HandleFunc("POST /api/frontends/{id}/routes", api.addRoute)
    mux.HandleFunc("GET /api/frontends/{id}/routes", api.listRoutes)
    mux.HandleFunc("DELETE /api/frontends/{id}/routes/{route_id}", api.deleteRoute)

    // Health check
    mux.HandleFunc("GET /health", api.health)

    api.server = &http.Server{
        Addr:    fmt.Sprintf(":%d", port),
        Handler: mux,
    }

    return api
}

// API Request/Response Types

type BackendRegistrationRequest struct {
    FrontendID string                 `json:"frontend_id"`
    Name       string                 `json:"name"`
    Servers    []BackendServerRequest `json:"servers"`
}

type RouteRequest struct {
    Host        string `json:"host"`
    Path        string `json:"path"`
    BackendName string `json:"backend_name"`
}

type FrontendResponse struct {
    ID       string              `json:"id"`
    Name     string              `json:"name"`
    Enabled  bool                `json:"enabled"`
    Mode     string              `json:"mode"`
    Bindings []BindingDefinition `json:"bindings"`
    Routing  RoutingConfig       `json:"routing"`
}
```

### API Usage Examples

#### List Frontends
```bash
curl http://localhost:6060/api/frontends
```

Response:
```json
{
  "success": true,
  "frontends": [
    {
      "id": "http-frontend-1",
      "name": "http-gateway-1",
      "enabled": true,
      "mode": "http",
      "bindings": [
        {
          "address": "0.0.0.0",
          "port": 8080,
          "protocol": "http",
          "http2": true
        }
      ],
      "routing": {
        "bypass_rules": false,
        "default_backend": "default-backend-1"
      }
    }
  ]
}
```

#### Register Backend to Frontend
```bash
curl -X POST http://localhost:6060/api/frontends/http-frontend-1/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-backend",
    "servers": [
      {
        "name": "server1",
        "ip": "192.168.1.10",
        "port": 8080
      }
    ]
  }'
```

#### Add Route to Frontend
```bash
curl -X POST http://localhost:6060/api/frontends/http-frontend-1/routes \
  -H "Content-Type: application/json" \
  -d '{
    "host": "example.com",
    "path": "/api",
    "backend_name": "my-backend"
  }'
```

## Implementation Plan

### Phase 1: Configuration Infrastructure with Interface Support
**Goal**: Build flexible configuration system that supports multiple config sources

1. **Create Configuration Interfaces**
   ```go
   // ConfigProvider interface allows different config sources
   type ConfigProvider interface {
       LoadConfig() (*FrontendConfig, error)
       ValidateConfig(*FrontendConfig) error
   }

   // YAMLConfigProvider implements config from YAML file
   type YAMLConfigProvider struct {
       FilePath string
   }

   // EnvConfigProvider implements config from environment variables
   type EnvConfigProvider struct {}

   // FlagConfigProvider implements config from command-line flags (backward compat)
   type FlagConfigProvider struct {
       OSArgs utils.OSArgs
   }
   ```

2. **Implement Configuration Types**
   - Define all structs: `FrontendConfig`, `FrontendDefinition`, `BindingDefinition`, `RoutingConfig`
   - Add YAML/JSON tags for serialization
   - Add validation methods

3. **Create Configuration Loaders**
   - `YAMLConfigProvider.LoadConfig()` - Load from YAML file
   - `FlagConfigProvider.LoadConfig()` - Convert existing flags to FrontendConfig
   - Config validation and defaults

4. **Add Configuration Registry**
   ```go
   type ConfigRegistry struct {
       providers []ConfigProvider
   }

   // Try providers in order until one succeeds
   func (r *ConfigRegistry) LoadConfig() (*FrontendConfig, error)
   ```

5. **Unit Tests**
   - Test YAML parsing
   - Test configuration validation
   - Test different providers

**Deliverables**:
- `pkg/gateway/config.go` - Configuration types
- `pkg/gateway/config_provider.go` - ConfigProvider interface and implementations
- `pkg/gateway/config_yaml.go` - YAML loader
- `pkg/gateway/config_flags.go` - Flags to config converter
- Tests for all configuration loading

### Phase 2: HAProxy Integration
**Goal**: Integrate frontend configuration with HAProxy

1. **Implement FrontendManager Core**
   - Basic lifecycle: Start, Stop
   - Frontend initialization from config
   - HAProxy frontend creation

2. **Binding Configuration**
   - Convert BindingDefinition to HAProxy bind models
   - Support HTTP, HTTPS, TCP protocols
   - HTTP/2 configuration (H2C and ALPN)
   - IPv6 support

3. **Backend Manager Integration**
   - Create BackendManager instance per frontend
   - Link frontend to its backends
   - Ensure proper lifecycle management

4. **Configuration Validation**
   - Validate frontend names are unique
   - Validate port conflicts
   - Validate backend references

**Deliverables**:
- `pkg/gateway/frontend_manager.go` - FrontendManager implementation
- Integration with existing BackendManager
- HAProxy configuration generation
- Integration tests

### Phase 3: API Layer
**Goal**: Expose frontend management via REST API

1. **Enhanced API Server**
   - Create EnhancedAPIServer struct
   - Support frontend-scoped operations
   - Maintain backward compatibility with existing API

2. **Frontend Endpoints**
   - `GET /api/frontends` - List all frontends
   - `GET /api/frontends/{id}` - Get frontend details

3. **Frontend-Scoped Backend Management**
   - `POST /api/frontends/{id}/backends` - Register backend to frontend
   - `GET /api/frontends/{id}/backends` - List backends for frontend
   - `DELETE /api/frontends/{id}/backends/{name}` - Unregister backend

4. **Frontend-Scoped Route Management**
   - `POST /api/frontends/{id}/routes` - Add route to frontend
   - `GET /api/frontends/{id}/routes` - List routes for frontend
   - `DELETE /api/frontends/{id}/routes/{route_id}` - Delete route

5. **API Documentation**
   - OpenAPI/Swagger spec
   - Example requests/responses
   - Error handling documentation

**Deliverables**:
- `pkg/gateway/enhanced_api.go` - Enhanced API server
- API endpoint implementations
- API tests and examples
- Documentation

### Phase 4: Default Bypass Rules (Future Enhancement)
**Goal**: Implement bypass mode where frontends skip all ACL processing by default

**Current Status**: Configuration support exists (`bypass_rules: true`), but implementation deferred

**When to Implement**:
This feature should be implemented when:
- Core frontend management is stable
- Use cases for bypass mode are validated
- Performance requirements are clear

**What Needs Implementation**:

1. **Bypass Mode Configuration**
   - Already defined in config: `routing.bypass_rules: true`
   - Validation: Ensure bypass frontends have default_backend

2. **HAProxy Configuration Generation**
   ```go
   // When bypass_rules is true:
   // - Do NOT create ACLs
   // - Do NOT create backend switching rules
   // - Configure only default_backend
   // - Optimize for performance (no ACL processing)
   ```

3. **Route API Restrictions**
   - Block route additions to bypass frontends
   - Return clear error: "Frontend has bypass_rules enabled"
   - Document bypass mode behavior

4. **Performance Optimization**
   - Ensure minimal HAProxy config for bypass frontends
   - No ACL evaluation overhead
   - Direct routing to default backend

5. **Validation**
   - Prevent bypass frontends without default_backend
   - Warn when routes exist but bypass is enabled
   - Clear error messages

**Implementation Tasks** (for Phase 4):
```go
// In FrontendManager.configureFrontend()
if def.Routing.BypassRules {
    // Skip ACL and backend switching rule configuration
    // Only configure:
    // 1. Frontend with mode and default_backend
    // 2. Bindings
    // 3. Basic options (timeouts, connections)
    logger.Infof("Frontend %s configured in bypass mode", def.ID)
    return
}
// ... normal ACL/routing configuration ...
```

**Testing Requirements**:
- Verify no ACLs created for bypass frontends
- Verify route API returns error for bypass frontends
- Performance test: compare bypass vs normal mode
- Validate all traffic goes to default_backend

**Deliverables** (Phase 4):
- Bypass mode implementation in FrontendManager
- API route blocking for bypass frontends
- Tests for bypass behavior
- Performance benchmarks
- Documentation updates

**Note**: Until Phase 4 is implemented, setting `bypass_rules: true` in configuration will be parsed but not enforced. The system will still allow routes and create ACLs. Phase 4 implementation will make this flag functional.

## Configuration File Location

The frontend configuration can be loaded from:
- Command-line flag: `--frontend-config=/path/to/config.yaml`
- Environment variable: `FRONTEND_CONFIG_PATH`
- Default location: `/etc/haproxy-gateway/frontends.yaml`

## Benefits

1. **Flexibility**: Support multiple frontends with different characteristics
2. **Isolation**: Each frontend manages its own backends and routes
3. **Scalability**: Can run different services on different ports/addresses
4. **Simplicity**: Configuration-driven approach reduces code complexity
5. **Dynamic Updates**: Backends can be registered/unregistered via API
6. **Bypass Mode**: Support for simple passthrough without ACL processing

## Example Use Cases

### Use Case 1: Multi-Tenant Gateway
```yaml
frontends:
  - id: "tenant-a"
    name: "tenant-a-frontend"
    bindings:
      - address: "0.0.0.0"
        port: 8080
    routing:
      default_backend: "tenant-a-default"

  - id: "tenant-b"
    name: "tenant-b-frontend"
    bindings:
      - address: "0.0.0.0"
        port: 8081
    routing:
      default_backend: "tenant-b-default"
```

### Use Case 2: Protocol Separation
```yaml
frontends:
  - id: "http-frontend"
    mode: "http"
    bindings:
      - port: 80
        protocol: "http"
      - port: 443
        protocol: "https"
        ssl: true

  - id: "tcp-frontend"
    mode: "tcp"
    bindings:
      - port: 5000
        protocol: "tcp"
```

### Use Case 3: Bypass Gateway
```yaml
frontends:
  - id: "simple-lb"
    name: "simple-loadbalancer"
    bindings:
      - port: 9000
    routing:
      bypass_rules: true  # No ACL processing, direct to default backend
      default_backend: "simple-pool"
```

## Migration Path

For existing deployments using the current `HTTPGateway`:

1. **Backward Compatibility**: Keep existing `HTTPGateway` working
2. **Configuration Wrapper**: Create default config from existing flags
3. **Gradual Migration**: Allow mixed mode operation
4. **Feature Flag**: Enable new frontend manager via flag

Example:
```bash
# Old way (still works)
./http-gateway --http-bind-port=8080 --https-bind-port=8443

# New way (using config file)
./http-gateway --frontend-config=/etc/frontends.yaml
```
