## Backend Manager API - Client Library Example

This example demonstrates building an HTTP API server using the HAProxy Gateway Client Library (`pkg/client`).

### Overview

Instead of calling HTTP REST APIs, this example shows how to:
1. Use the Go client library directly
2. Wrap it in your own HTTP API
3. Provide custom endpoints for backend management

### Architecture

```
┌────────────────┐
│  Your API Call │
└───────┬────────┘
        │ HTTP
        ▼
┌────────────────┐
│ Backend Manager│
│  API Server    │  (This Example)
└───────┬────────┘
        │ Go function calls
        ▼
┌────────────────┐
│ Client Library │  (pkg/client)
└───────┬────────┘
        │
        ▼
┌────────────────┐
│FrontendManager │
└────────────────┘
```

### API Endpoints

#### Health Check
```bash
GET /health
```

Response:
```json
{
  "success": true,
  "message": "healthy",
  "data": {"status": "ok"}
}
```

#### Register Backend
```bash
POST /api/backends/register
Content-Type: application/json

{
  "frontend_id": "default",
  "backend": {
    "name": "my-backend",
    "servers": [
      {"name": "server1", "ip": "192.168.1.10", "port": 8080},
      {"name": "server2", "ip": "192.168.1.11", "port": 8080}
    ]
  }
}
```

#### Unregister Backend
```bash
DELETE /api/backends/unregister?frontend_id=default&backend_name=my-backend
```

#### Get Backends
```bash
GET /api/backends?frontend_id=default
```

#### Add Server to Existing Backend
```bash
POST /api/servers/add
Content-Type: application/json

{
  "frontend_id": "default",
  "backend_name": "my-backend",
  "server": {
    "name": "server3",
    "ip": "192.168.1.12",
    "port": 8080
  }
}
```

#### Remove Server from Backend
```bash
POST /api/servers/remove
Content-Type: application/json

{
  "frontend_id": "default",
  "backend_name": "my-backend",
  "server_name": "server2"
}
```

#### Update Server Address
```bash
POST /api/servers/update
Content-Type: application/json

{
  "frontend_id": "default",
  "backend_name": "my-backend",
  "server_name": "server1",
  "new_ip": "192.168.1.99",
  "new_port": 9000
}
```

### Building

```bash
# Build locally
go build -o backend-manager-api ./examples/backend-manager-api/main.go

# Build Docker image
docker build -t backend-manager-api:latest -f examples/backend-manager-api/Dockerfile .
```

### Running

#### Option 1: Standalone (Demo)
```bash
PORT=9091 ./backend-manager-api
```

#### Option 2: Docker
```bash
docker run -p 9091:9091 backend-manager-api:latest
```

### Usage Examples

```bash
# Check health
curl http://localhost:9091/health

# Register a backend
curl -X POST http://localhost:9091/api/backends/register \
  -H "Content-Type: application/json" \
  -d '{
    "frontend_id": "default",
    "backend": {
      "name": "web-backend",
      "servers": [
        {"name": "web-1", "ip": "192.168.1.10", "port": 8080}
      ]
    }
  }'

# Add a server
curl -X POST http://localhost:9091/api/servers/add \
  -H "Content-Type: application/json" \
  -d '{
    "frontend_id": "default",
    "backend_name": "web-backend",
    "server": {
      "name": "web-2",
      "ip": "192.168.1.11",
      "port": 8080
    }
  }'

# Get all backends
curl http://localhost:9091/api/backends?frontend_id=default

# Update server address
curl -X POST http://localhost:9091/api/servers/update \
  -H "Content-Type: application/json" \
  -d '{
    "frontend_id": "default",
    "backend_name": "web-backend",
    "server_name": "web-1",
    "new_ip": "192.168.1.20",
    "new_port": 8080
  }'

# Remove a server
curl -X POST http://localhost:9091/api/servers/remove \
  -H "Content-Type: application/json" \
  -d '{
    "frontend_id": "default",
    "backend_name": "web-backend",
    "server_name": "web-2"
  }'

# Unregister backend
curl -X DELETE "http://localhost:9091/api/backends/unregister?frontend_id=default&backend_name=web-backend"
```

### Integration with Gateway

To use this in production, you need to integrate it with the gateway process:

**Option 1: Embed in Gateway**
```go
// In cmd/http-gateway/main.go
import "github.com/haproxytech/kubernetes-ingress/examples/backend-manager-api"

// After creating frontendManager
c := client.NewClient(frontendManager)
apiServer := backendmanager.NewServer(c, "9091")
go apiServer.Start()
```

**Option 2: gRPC/IPC** (Future Enhancement)
- Expose FrontendManager via gRPC
- This API connects via gRPC client
- Allows running as separate process

### Testing with Dynamic Backend Tests

Once integrated with the gateway, you can test using:

```bash
# Run the comprehensive dynamic backend tests
cd test/scripts
GATEWAY_API=http://localhost:9091 ./test-dynamic-backends-standalone.sh
```

Note: You'll need to adapt the test script's API calls to match these endpoints.

### Code Structure

- `main.go` - HTTP server using client library
- `Dockerfile` - Container image definition
- `README.md` - This file

### Benefits vs Direct HTTP API

| Feature | This Example | Gateway HTTP API |
|---------|--------------|------------------|
| **Implementation** | Uses client library | Direct REST API |
| **Type Safety** | ✅ Compile-time | ❌ Runtime only |
| **Custom Logic** | ✅ Easy to add | ❌ Modify gateway |
| **Deployment** | Separate service | Part of gateway |
| **Use Case** | Custom workflows | Standard operations |

### Next Steps

1. ✅ API structure defined
2. ⏭️ Inject FrontendManager (via embedding or gRPC)
3. ⏭️ Add authentication/authorization
4. ⏭️ Add rate limiting
5. ⏭️ Add audit logging
6. ⏭️ Add metrics/monitoring
