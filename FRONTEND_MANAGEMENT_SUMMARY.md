# Frontend Management - Design Summary

## Quick Overview

This design provides a **configuration-based frontend management system** for HAProxy HTTP Gateway that allows you to:

✅ Configure **multiple frontends** from a YAML file
✅ Each frontend has its **own binding configuration** (IP, port, protocol)
✅ Support **bypass mode** (skip all ACL rules)
✅ Each frontend has its **own default backend**
✅ **Unique frontend identification** for registration/deregistration
✅ **Dynamic backend management** per frontend via API

## Key Features

### 1. Multiple Frontends via Configuration

```yaml
frontends:
  - id: "http-frontend-1"          # Unique ID for API operations
    name: "http-gateway-1"          # HAProxy frontend name
    enabled: true

  - id: "http-frontend-2"
    name: "http-gateway-2"
    enabled: true
```

### 2. Flexible Binding Configuration

Each frontend can bind to different addresses and ports:

```yaml
bindings:
  - address: "0.0.0.0"
    port: 8080
    protocol: "http"
    http2: true

  - address: "0.0.0.0"
    port: 8443
    protocol: "https"
    ssl: true
    alpn: "h2,http/1.1"
    cert_dir: "/etc/haproxy/certs"
```

### 3. Bypass/Passthrough Mode

Frontends can bypass all ACL rules and route directly to default backend:

```yaml
routing:
  bypass_rules: true              # Skip all routing rules
  default_backend: "my-backend"   # Direct all traffic here
```

### 4. Default Backend Per Frontend

Each frontend can have its own default backend:

```yaml
routing:
  bypass_rules: false
  default_backend: "frontend-1-default"
```

### 5. Frontend-Specific Backend Registration

API endpoints to register backends to specific frontends:

```bash
# Register backend to frontend-1
POST /api/frontends/http-frontend-1/backends
{
  "name": "my-backend",
  "servers": [
    {"name": "srv1", "ip": "192.168.1.10", "port": 8080}
  ]
}

# Unregister backend from frontend-1
DELETE /api/frontends/http-frontend-1/backends/my-backend
```

### 6. Frontend-Specific Routing

Add routes to specific frontends:

```bash
# Add route to frontend-1
POST /api/frontends/http-frontend-1/routes
{
  "host": "example.com",
  "path": "/api",
  "backend_name": "my-backend"
}
```

## Architecture Components

### 1. Configuration Layer
- **FrontendConfig**: Main configuration structure
- **FrontendDefinition**: Individual frontend config
- **BindingDefinition**: Network binding config
- **RoutingConfig**: Routing behavior config

### 2. Management Layer
- **FrontendManager**: Manages all frontends
- **ManagedFrontend**: Represents one frontend instance
- Each frontend has its own **BackendManager**

### 3. API Layer
- **EnhancedAPIServer**: REST API for management
- Frontend-specific endpoints
- Backend and route registration per frontend

## API Endpoints

### Frontend Management
```
GET    /api/frontends              # List all frontends
GET    /api/frontends/{id}         # Get frontend details
```

### Backend Management (Per Frontend)
```
POST   /api/frontends/{id}/backends           # Register backend
GET    /api/frontends/{id}/backends           # List backends
DELETE /api/frontends/{id}/backends/{name}    # Unregister backend
```

### Route Management (Per Frontend)
```
POST   /api/frontends/{id}/routes             # Add route
GET    /api/frontends/{id}/routes             # List routes
DELETE /api/frontends/{id}/routes/{route_id}  # Delete route
```

## Example Configurations

### Example 1: Multi-Port HTTP Gateway

```yaml
frontends:
  - id: "public-api"
    name: "public-api-frontend"
    bindings:
      - address: "0.0.0.0"
        port: 80
        protocol: "http"
      - address: "0.0.0.0"
        port: 443
        protocol: "https"
        ssl: true
    routing:
      bypass_rules: false
      default_backend: "public-default"

  - id: "internal-api"
    name: "internal-api-frontend"
    bindings:
      - address: "192.168.1.1"
        port: 8080
        protocol: "http"
    routing:
      bypass_rules: false
      default_backend: "internal-default"
```

### Example 2: Simple Load Balancer (Bypass Mode)

```yaml
frontends:
  - id: "simple-lb"
    name: "simple-loadbalancer"
    bindings:
      - address: "0.0.0.0"
        port: 9000
        protocol: "http"
    routing:
      bypass_rules: true          # No ACL processing
      default_backend: "app-pool" # All traffic goes here
```

### Example 3: Multi-Tenant Setup

```yaml
frontends:
  - id: "tenant-a"
    name: "tenant-a-gateway"
    bindings:
      - port: 8080
    routing:
      default_backend: "tenant-a-backends"

  - id: "tenant-b"
    name: "tenant-b-gateway"
    bindings:
      - port: 8081
    routing:
      default_backend: "tenant-b-backends"
```

## Usage Workflow

### 1. Start Gateway with Configuration

```bash
./http-gateway --frontend-config=/etc/haproxy/frontends.yaml
```

### 2. Register Backends to Frontend

```bash
curl -X POST http://localhost:6060/api/frontends/public-api/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-backend",
    "servers": [
      {"name": "api1", "ip": "10.0.1.10", "port": 8080},
      {"name": "api2", "ip": "10.0.1.11", "port": 8080}
    ]
  }'
```

### 3. Add Routes to Frontend

```bash
curl -X POST http://localhost:6060/api/frontends/public-api/routes \
  -H "Content-Type: application/json" \
  -d '{
    "host": "api.example.com",
    "path": "/v1",
    "backend_name": "api-backend"
  }'
```

### 4. List Frontends

```bash
curl http://localhost:6060/api/frontends
```

## Benefits

1. **Multi-Frontend Support**: Run multiple independent gateways in one process
2. **Configuration-Driven**: No code changes needed for new frontends
3. **Frontend Isolation**: Each frontend manages its own backends/routes
4. **Dynamic Updates**: Add/remove backends without restart
5. **Bypass Mode**: Simple passthrough for basic load balancing
6. **Flexible Binding**: Different IPs, ports, protocols per frontend
7. **API-First Design**: All operations available via REST API

## Implementation Status

**Status**: Design Phase ✏️

### Implementation Phases

#### Phase 1: Configuration Infrastructure ✏️
**Goal**: Build flexible configuration system with interface support

- **ConfigProvider Interface**: Support multiple config sources (YAML, flags, env vars)
- **Configuration Types**: Define all structs with validation
- **Configuration Loaders**: YAML, flags-to-config converter
- **Configuration Registry**: Try providers in order

**Key Feature**: Interface-based design enables getting configuration from any source

#### Phase 2: HAProxy Integration 📋
**Goal**: Integrate frontend configuration with HAProxy

- FrontendManager core implementation
- Binding configuration (HTTP/HTTPS/TCP, HTTP/2, IPv6)
- Backend Manager integration per frontend
- Configuration validation

#### Phase 3: API Layer 📋
**Goal**: Expose frontend management via REST API

- Enhanced API server
- Frontend-scoped backend registration
- Frontend-scoped route management
- API documentation

#### Phase 4: Default Bypass Rules 🔮
**Goal**: Implement bypass mode (Future Enhancement)

**Status**: Configuration exists but **not implemented yet**

- `bypass_rules: true` will be parsed but not enforced
- Full implementation deferred until core features are stable
- Will skip all ACL processing when implemented
- Performance optimization opportunity

**When to Implement Phase 4**:
- After core frontend management is stable
- When use cases for bypass mode are validated
- When performance requirements are clear

## Files to Create/Modify

### New Files (Phase 1)
- [pkg/gateway/config.go](pkg/gateway/config.go) - Configuration types and structs
- [pkg/gateway/config_provider.go](pkg/gateway/config_provider.go) - ConfigProvider interface
- [pkg/gateway/config_yaml.go](pkg/gateway/config_yaml.go) - YAML config loader
- [pkg/gateway/config_flags.go](pkg/gateway/config_flags.go) - Flags to config converter
- [pkg/gateway/config_test.go](pkg/gateway/config_test.go) - Configuration tests

### New Files (Phase 2)
- [pkg/gateway/frontend_manager.go](pkg/gateway/frontend_manager.go) - Frontend manager implementation

### New Files (Phase 3)
- [pkg/gateway/enhanced_api.go](pkg/gateway/enhanced_api.go) - Enhanced API server

### Modified Files
- [main.go](main.go) - Add frontend config flag
- [pkg/utils/flags.go](pkg/utils/flags.go) - Add configuration flags
- [cmd/http-gateway/main.go](cmd/http-gateway/main.go) - Use FrontendManager

### Configuration Files
- `/etc/haproxy-gateway/frontends.yaml` - Frontend configuration

## See Also

- [FRONTEND_MANAGEMENT_DESIGN.md](FRONTEND_MANAGEMENT_DESIGN.md) - Complete design specification
- [BACKEND_REGISTRATION.md](BACKEND_REGISTRATION.md) - Backend registration details
- [GATEWAY_FEATURES.md](GATEWAY_FEATURES.md) - Gateway feature overview
