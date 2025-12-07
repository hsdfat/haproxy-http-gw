# Backend Registration Architecture

## Overview

The HAProxy HTTP Gateway now uses a **backend registration model** where:

1. **Frontend is statically configured** in `haproxy-init.cfg` at initialization
2. **Backends register dynamically** via REST API when they come alive
3. **Gateway manages backend lifecycle** through the registration API

## Architecture Flow

```
┌─────────────────┐
│   HAProxy Init  │ ──> Frontend statically configured
│   Config File   │     (http-gateway frontend on port 8080)
└─────────────────┘

┌─────────────────┐
│  Backend Server │ ──> Starts up
└────────┬────────┘
         │
         ├──> Registers with Gateway API
         │    POST /api/backends
         │    {
         │      "name": "api-backend",
         │      "servers": [{
         │        "name": "server1",
         │        "ip": "192.168.1.10",
         │        "port": 9000
         │      }]
         │    }
         │
         v
┌─────────────────┐
│  Gateway API    │ ──> Updates HAProxy configuration
│  (Port 9090)    │     Adds backend servers dynamically
└─────────────────┘
```

## Components

### 1. Static Frontend Configuration

**File**: `test/haproxy-init.cfg`

The frontend is now pre-configured in the HAProxy init file:

```haproxy
frontend http-gateway
    bind :8080
    bind [::]:8080 v4v6
    mode http
    option http-use-htx
    default_backend api-backend
```

This means:
- Frontend exists from the start
- No dynamic frontend creation needed
- **Supports both HTTP/1.1 and HTTP/2** (H2C via client request)
- Default backend handles requests until routes are configured

### 2. Backend Registration API

**Endpoint**: `POST /api/backends`

Backends register themselves by sending their information:

```bash
curl -X POST http://gateway:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "api-backend",
    "servers": [
      {
        "name": "server1",
        "ip": "192.168.1.10",
        "port": 9000
      }
    ]
  }'
```

**Response**:
```json
{
  "success": true,
  "message": "Backend registered successfully",
  "backend": "api-backend"
}
```

### 3. Backend Registration Script

**File**: `test/scripts/register-backend.sh`

Backends use this script to auto-register on startup:

```bash
BACKEND_NAME=api-backend \
SERVER_NAME=server1 \
SERVER_IP=192.168.1.10 \  # Optional - auto-detected if not provided
SERVER_PORT=9000 \
GATEWAY_URL=http://gateway:9090 \
./register-backend.sh
```

Features:
- Waits for gateway availability
- Retries on failure (30 attempts, 2s delay)
- Validates registration
- **Auto-detects IP** if not provided (from eth0 or hostname -i)
- Optional keep-alive mode

### 4. Backend Entrypoint

**File**: `test/scripts/backend-entrypoint.sh`

Docker containers use this entrypoint to:
1. **Auto-detect IP address** from network interface (eth0)
2. Start the backend server
3. Auto-register with the gateway
4. Handle graceful shutdown

**IP Detection Logic**:
```bash
# 1. Try eth0 interface (most common)
ip addr show eth0 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1

# 2. Fallback to hostname -i
hostname -i

# 3. Last resort: use hostname
hostname
```

## API Endpoints

### Backend Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/backends` | Register a new backend |
| GET | `/api/backends` | List all registered backends |
| DELETE | `/api/backends/{name}` | Unregister a backend |

### Route Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/routes` | Add a routing rule |
| GET | `/api/routes` | List all routes |
| DELETE | `/api/routes` | Delete a route |

### Health Check

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check endpoint |

## Usage Examples

### Manual Backend Registration

```bash
# Register a backend with multiple servers
curl -X POST http://localhost:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "api-backend",
    "servers": [
      {"name": "server1", "ip": "10.0.1.10", "port": 9000},
      {"name": "server2", "ip": "10.0.1.11", "port": 9000},
      {"name": "server3", "ip": "10.0.1.12", "port": 9000}
    ]
  }'

# List all registered backends
curl http://localhost:9090/api/backends

# Unregister a backend
curl -X DELETE http://localhost:9090/api/backends/api-backend
```

### Add Routing Rules

```bash
# Route api.example.com/api to api-backend
curl -X POST http://localhost:9090/api/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "host": "api.example.com",
    "path": "/api",
    "backend": "api-backend"
  }'

# Route web.example.com to web-backend
curl -X POST http://localhost:9090/api/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "host": "web.example.com",
    "backend": "web-backend"
  }'
```

## Docker Compose Configuration

Backends are configured with environment variables for auto-registration:

```yaml
backend-server-1:
  build:
    context: ..
    dockerfile: test/Dockerfile.backend
  environment:
    - SERVER_NAME=backend-server-1
    # SERVER_IP is auto-detected from eth0 interface
    - SERVER_PORT=9000
    - BACKEND_NAME=api-backend
    - GATEWAY_URL=http://gateway:9090
  depends_on:
    - gateway
```

**Note**: `SERVER_IP` is automatically detected from the container's network interface. You can override it by setting the environment variable if needed.

## Benefits

### 1. Simplified Initialization
- HAProxy starts with valid configuration
- No need to create frontend dynamically
- Faster startup time

### 2. Dynamic Backend Management
- Backends can join/leave at any time
- Auto-registration on startup
- Self-service backend lifecycle

### 3. Separation of Concerns
- Frontend: Statically configured (infrastructure)
- Backends: Dynamically managed (application)
- Routes: API-driven (configuration)

### 4. Better Testing
- Backends register themselves
- Clear separation of initialization steps
- Easier to debug registration issues

## Migration from Old Architecture

**Before**:
- Backends were added in code: `provider.AddBackend(...)`
- Frontend was created dynamically
- Tightly coupled initialization

**After**:
- Backends register via API: `POST /api/backends`
- Frontend is pre-configured
- Loosely coupled, event-driven

## Testing

Run the test suite:

```bash
cd test
./run-local-test.sh
```

The test will:
1. Start the gateway with static frontend
2. Start backend servers (auto-register)
3. Wait for registration
4. Verify backends are registered
5. Test load balancing
6. Test HTTP/2 (H2C) support

## Troubleshooting

### Backend fails to register

Check gateway logs:
```bash
podman-compose logs gateway
```

Check backend logs:
```bash
podman-compose logs backend-server-1
```

### Manual registration

If auto-registration fails, manually register:
```bash
curl -X POST http://localhost:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "api-backend",
    "servers": [
      {"name": "server1", "ip": "backend-server-1", "port": 9000}
    ]
  }'
```

### Check registered backends

```bash
curl http://localhost:9090/api/backends | jq
```

## Future Enhancements

1. **Health Checks**: Auto-deregister unhealthy backends
2. **TTL-based Registration**: Backends must re-register periodically
3. **Service Discovery**: Integrate with Consul/etcd/K8s
4. **Authentication**: Secure the registration API
5. **Webhooks**: Notify on backend changes
