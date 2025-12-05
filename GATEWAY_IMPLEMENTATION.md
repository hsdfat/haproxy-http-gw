# HTTP Gateway Implementation Summary

## What Was Built

A complete HTTP/2 gateway system that forwards requests to dynamic backends discovered through an event-driven interface, completely independent of Kubernetes.

## File Structure

```
pkg/gateway/
├── provider.go              # Backend provider interface
├── manager.go               # Event-driven backend manager
├── gateway.go               # HTTP/2 gateway implementation
├── README.md                # Complete documentation
└── examples/
    ├── simple_provider.go   # In-memory provider example
    └── rest_provider.go     # REST API provider example

cmd/http-gateway/
└── main.go                  # Complete usage examples
```

## Key Features

### 1. Backend Provider Interface
- Define how backends (IP-name pairs) are discovered
- Event-based updates (Add/Update/Delete)
- Can be implemented for any backend source

### 2. HTTP/2 Support
- Full HTTP/2 via ALPN negotiation (`h2,http/1.1`)
- Client-side HTTP/2 (frontend)
- Server-side HTTP/2 (backend connections)
- Configurable via `ALPN` parameter

### 3. Event-Driven Architecture
- Channel-based event system (similar to K8s sync)
- Non-blocking event processing
- Periodic reconciliation for consistency

### 4. No Kubernetes Dependency
- Direct HAProxy API usage
- Runtime socket for dynamic updates
- Configuration API for structural changes
- Works completely standalone

## Quick Usage

```go
// 1. Create a backend provider
provider := examples.NewSimpleProvider()
provider.AddBackend(gateway.Backend{
    Name: "my-backend",
    Servers: []gateway.BackendServer{
        {Name: "srv1", IP: "10.0.0.1", Port: 8080},
        {Name: "srv2", IP: "10.0.0.2", Port: 8080},
    },
})

// 2. Create manager
manager := gateway.NewManager(gateway.ManagerConfig{
    HAProxyClient: haproxyClient,
    Provider:      provider,
})

// 3. Create HTTP/2 gateway
gw := gateway.NewHTTPGateway(haproxyClient, manager, gateway.GatewayConfig{
    HTTPPort:     80,
    HTTPSPort:    443,
    HTTPSEnabled: true,
    EnableHTTP2:  true,
    ALPN:         "h2,http/1.1",
})

// 4. Start
ctx := context.Background()
gw.Start(ctx)

// 5. Add routes
gw.AddBackendRoute("api.example.com", "/api", "my-backend")
```

## How It Communicates with HAProxy

### Backend Discovery Flow

```
Your Backend Source (DB/API/Registry)
    ↓
BackendProvider Implementation
    ↓
Event Channel (BackendEvent)
    ↓
Backend Manager (Manager)
    ↓
HAProxy API Client
    ↓
├─→ Runtime Socket (/var/run/haproxy-runtime-api.sock)
│   └─→ Dynamic server updates (no reload)
│
└─→ Configuration API (client-native)
    └─→ Backend creation/deletion (graceful reload)
```

### Communication Methods

1. **Runtime Socket** - For dynamic changes (fast):
   ```
   set server backend/srv1 addr 10.0.0.1 port 8080
   set server backend/srv1 state ready
   ```

2. **Configuration API** - For structural changes:
   ```go
   APIStartTransaction()
   BackendCreateOrUpdate(backend)
   BackendServerCreate(server)
   APICommitTransaction()
   APIFinalCommitTransaction()
   ```

3. **Process Signals** - For graceful reload:
   ```
   SIGUSR2 → Graceful reload with zero downtime
   ```

## Provider Interface

```go
type BackendProvider interface {
    // Start watching and send events to channel
    Start(ctx context.Context, eventChan chan<- BackendEvent) error

    // Stop the provider
    Stop() error

    // Get all backends
    GetBackends() ([]Backend, error)

    // Get specific backend
    GetBackend(name string) (*Backend, error)
}
```

## Example Implementations

### 1. Simple Provider (Manual Management)
```go
provider := examples.NewSimpleProvider()
provider.AddBackend(backend)
provider.UpdateBackend(backend)
provider.DeleteBackend(name)
```

### 2. Polling Provider (Periodic Fetch)
```go
provider := examples.NewPollingProvider(10*time.Second, func() ([]Backend, error) {
    // Fetch from database, API, etc.
    return fetchBackends()
})
```

### 3. REST API Provider
```go
provider := examples.NewRESTBackendProvider("http://api.example.com/backends", 10*time.Second)
```

**Expected JSON format:**
```json
{
  "backends": [
    {
      "name": "api-backend",
      "servers": [
        {"name": "srv1", "ip": "10.0.1.10", "port": 8080}
      ]
    }
  ]
}
```

## Testing

Run the examples:
```bash
go run cmd/http-gateway/main.go
```

Test HTTP/2:
```bash
curl -v --http2 https://localhost:8443/
```

Check HAProxy backend:
```bash
echo "show backend" | socat - /var/run/haproxy-runtime-api.sock
```

## Next Steps to Implement Your Own Provider

1. **Implement BackendProvider interface** for your backend source
2. **Start method**: Watch your source and send events to the channel
3. **Event types**:
   - `BackendEventAdd` - New backend discovered
   - `BackendEventUpdate` - Backend servers changed
   - `BackendEventDelete` - Backend removed

Example for database:
```go
type DBProvider struct {
    db *sql.DB
}

func (p *DBProvider) Start(ctx context.Context, eventChan chan<- BackendEvent) error {
    ticker := time.NewTicker(5 * time.Second)
    for {
        select {
        case <-ticker.C:
            backends := p.fetchFromDB()
            for _, backend := range backends {
                eventChan <- BackendEvent{
                    Type: BackendEventUpdate,
                    Backend: backend,
                }
            }
        case <-ctx.Done():
            return nil
        }
    }
}

func (p *DBProvider) fetchFromDB() []Backend {
    rows, _ := p.db.Query("SELECT name, server_ip, server_port FROM backends")
    // Parse and return backends
}
```

## HTTP/2 Configuration Details

### Frontend HTTP/2 (Client-facing)
- **ALPN**: `h2,http/1.1` (negotiates HTTP/2 or fallback to HTTP/1.1)
- **Binding**: SSL-enabled binds with ALPN support
- **Mode**: `http` with `http-keep-alive`

### Backend HTTP/2 (Server connections)
- **Server proto**: `h2` (force HTTP/2 to backend servers)
- **Server ALPN**: `h2,http/1.1` (negotiate with backend)
- **Check ALPN**: Health checks via HTTP/2

### Configuration generated:
```haproxy
frontend http-gateway
    bind :443 ssl crt /etc/certs alpn h2,http/1.1
    mode http
    use_backend api-backend if host_api

backend api-backend
    mode http
    balance roundrobin
    server srv1 10.0.1.10:8080 check alpn h2,http/1.1 proto h2
```

## Benefits

1. **No Kubernetes**: Works standalone without K8s API
2. **Dynamic Backends**: Real-time updates via events
3. **HTTP/2**: Full HTTP/2 support both client and server side
4. **Flexible**: Implement provider for any backend source
5. **Performance**: Runtime socket updates are fast (no reload)
6. **Zero Downtime**: Graceful reloads when needed

## Complete Documentation

See [pkg/gateway/README.md](pkg/gateway/README.md) for full documentation with more examples and troubleshooting.
