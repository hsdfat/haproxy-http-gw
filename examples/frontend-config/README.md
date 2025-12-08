# Frontend Configuration Examples

This directory contains example frontend configuration files for the HAProxy HTTP Gateway.

## Available Examples

### 1. Single HTTP Frontend
**File**: [single-http-frontend.yaml](single-http-frontend.yaml)

Simple single frontend configuration with HTTP only.

```bash
./http-gateway --frontend-config=examples/frontend-config/single-http-frontend.yaml
```

### 2. HTTP and HTTPS Frontend
**File**: [http-https-frontend.yaml](http-https-frontend.yaml)

Single frontend with both HTTP (port 80) and HTTPS (port 443) bindings.

```bash
./http-gateway --frontend-config=examples/frontend-config/http-https-frontend.yaml
```

### 3. Multiple Frontends
**File**: [multiple-frontends.yaml](multiple-frontends.yaml)

Multiple frontends for different purposes:
- Public API (ports 8080, 8443)
- Internal API (port 9080)
- Admin (port 9090)

```bash
./http-gateway --frontend-config=examples/frontend-config/multiple-frontends.yaml
```

### 4. Bypass Mode Frontend
**File**: [bypass-mode-frontend.yaml](bypass-mode-frontend.yaml)

Demonstrates `bypass_rules` configuration (Phase 4 feature).

**Note**: `bypass_rules: true` is currently parsed but NOT enforced. This is a Phase 4 feature.

```bash
./http-gateway --frontend-config=examples/frontend-config/bypass-mode-frontend.yaml
```

### 5. Multi-Tenant Setup
**File**: [multi-tenant.yaml](multi-tenant.yaml)

Separate frontends for multiple tenants, each on different ports.

```bash
./http-gateway --frontend-config=examples/frontend-config/multi-tenant.yaml
```

## Configuration Structure

```yaml
frontends:
  - id: "unique-id"              # Unique identifier for API operations
    name: "haproxy-name"         # HAProxy frontend name
    enabled: true                # Enable/disable frontend
    mode: "http"                 # "http" or "tcp"

    bindings:
      - address: "0.0.0.0"       # Bind address
        port: 8080               # Port number
        protocol: "http"         # "http", "https", or "tcp"
        http2: true              # Enable HTTP/2
        ssl: false               # Enable SSL/TLS
        alpn: "h2,http/1.1"      # ALPN protocols (for HTTPS)
        cert_dir: "/path/certs"  # Certificate directory
        ipv6: false              # Enable IPv6

    routing:
      bypass_rules: false        # Skip ACL processing (Phase 4)
      default_backend: "backend" # Default backend name

    options:
      max_connections: 10000     # Max concurrent connections
      timeout_client: "30s"      # Client timeout
      http_connection_mode: "http-keep-alive"
```

## Using the API

After starting the gateway with a frontend config, you can use the API to manage backends and routes.

### List Frontends

```bash
curl http://localhost:6060/api/frontends
```

### Register Backend to Frontend

```bash
curl -X POST http://localhost:6060/api/frontends/public-api/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-backend",
    "servers": [
      {"name": "srv1", "ip": "10.0.1.10", "port": 8080},
      {"name": "srv2", "ip": "10.0.1.11", "port": 8080}
    ]
  }'
```

### Add Route to Frontend

```bash
curl -X POST http://localhost:6060/api/frontends/public-api/routes \
  -H "Content-Type: application/json" \
  -d '{
    "host": "api.example.com",
    "path": "/v1",
    "backend_name": "api-backend"
  }'
```

### List Backends for Frontend

```bash
curl http://localhost:6060/api/frontends/public-api/backends
```

### List Routes for Frontend

```bash
curl http://localhost:6060/api/frontends/public-api/routes
```

## Validation

The configuration is validated on load. Common validation errors:

- **Duplicate IDs**: Each frontend must have a unique `id`
- **Duplicate Names**: Each frontend must have a unique `name`
- **Port Conflicts**: Same address:port cannot be used by multiple frontends
- **Invalid Mode**: Must be "http" or "tcp"
- **Missing Required Fields**: `id`, `name`, `bindings`, `default_backend` are required

## Backward Compatibility

If no `--frontend-config` is specified, the gateway creates a default frontend from command-line flags:

```bash
# Old way (still works)
./http-gateway --http-bind-port=8080 --https-bind-port=8443

# Equivalent to:
./http-gateway --frontend-config=<generated-from-flags>
```

## Testing Configuration

To test your configuration without starting the gateway:

```bash
# Validate YAML syntax
yamllint examples/frontend-config/your-config.yaml

# Test loading (gateway will exit after validation if --test flag exists)
./http-gateway --frontend-config=examples/frontend-config/your-config.yaml --test
```

## See Also

- [FRONTEND_MANAGEMENT_DESIGN.md](../../FRONTEND_MANAGEMENT_DESIGN.md) - Complete design specification
- [FRONTEND_MANAGEMENT_SUMMARY.md](../../FRONTEND_MANAGEMENT_SUMMARY.md) - Quick start guide
- [PHASE1_CONFIG_INTERFACE.md](../../PHASE1_CONFIG_INTERFACE.md) - Phase 1 implementation details
