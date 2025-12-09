# HTTP Gateway with HTTP/2 Support

This package provides an event-driven HTTP gateway that forwards requests to dynamic backends using HAProxy with HTTP/2 support.

## Features

- **Event-Driven Architecture**: Backends are discovered and updated via events through a channel-based system
- **HTTP/2 Support**: Full HTTP/2 support with ALPN negotiation (`h2,http/1.1`)
- **Dynamic Backend Management**: Add, update, and remove backends without restarting
- **Interface-Based Provider**: Implement custom backend discovery from any source
- **Zero-Downtime Updates**: HAProxy graceful reloads ensure no dropped connections
- **Runtime API**: Updates server addresses dynamically without full reloads when possible

## Architecture

```
┌─────────────────────────┐
│   Backend Provider      │  (Your Implementation)
│   - REST API            │
│   - Database            │
│   - Service Registry    │
└───────────┬─────────────┘
            │ Events (Add/Update/Delete)
            ▼
┌─────────────────────────┐
│   Backend Manager       │
│   - Event Processing    │
│   - State Management    │
│   - HAProxy Sync        │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│   HAProxy API Client    │
│   - Configuration API   │
│   - Runtime Socket      │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│   HAProxy Process       │
│   - HTTP/2 Frontend     │
│   - Dynamic Backends    │
│   - Load Balancing      │
└─────────────────────────┘
```

## Quick Start

### 1. Implement the Backend Provider Interface

```go
package main

import (
    "context"
    "github.com/haproxytech/kubernetes-ingress/pkg/gateway"
)

type MyProvider struct {
    // Your fields
}

func (p *MyProvider) Start(ctx context.Context, eventChan chan<- gateway.BackendEvent) error {
    // Watch your backend source and send events
    go func() {
        for {
            // When you detect a backend change:
            eventChan <- gateway.BackendEvent{
                Type: gateway.BackendEventAdd,
                Backend: gateway.Backend{
                    Name: "my-backend",
                    Servers: []gateway.BackendServer{
                        {Name: "srv1", IP: "10.0.0.1", Port: 8080},
                        {Name: "srv2", IP: "10.0.0.2", Port: 8080},
                    },
                },
            }
        }
    }()
    return nil
}

func (p *MyProvider) Stop() error { return nil }
func (p *MyProvider) GetBackends() ([]gateway.Backend, error) { /* ... */ }
func (p *MyProvider) GetBackend(name string) (*gateway.Backend, error) { /* ... */ }
```

### 2. Create and Start the Gateway

```go
package main

import (
    "context"
    "github.com/haproxytech/kubernetes-ingress/pkg/gateway"
    "github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
)

func main() {
    // Initialize HAProxy client
    haproxyClient, _ := api.New(
        "/tmp/haproxy-gateway",
        "/etc/haproxy/haproxy.cfg",
        "/usr/local/sbin/haproxy",
        "/var/run/haproxy-runtime-api.sock",
    )

    // Create your backend provider
    provider := &MyProvider{}

    // Create backend manager
    manager := gateway.NewManager(gateway.ManagerConfig{
        HAProxyClient: haproxyClient,
        Provider:      provider,
        SyncPeriod:    5 * time.Second,
        EventChanSize: 100,
    })

    // Create HTTP gateway with HTTP/2
    gw := gateway.NewHTTPGateway(haproxyClient, manager, gateway.GatewayConfig{
        FrontendName:   "http-gateway",
        HTTPPort:       80,
        HTTPSPort:      443,
        HTTPSEnabled:   true,
        SSLCertDir:     "/etc/haproxy/certs",
        EnableHTTP2:    true,
        ALPN:           "h2,http/1.1",
        DefaultBackend: "my-backend",
    })

    // Start the gateway
    ctx := context.Background()
    gw.Start(ctx)

    // Add routing rules
    gw.AddBackendRoute("api.example.com", "/api", "api-backend")
    gw.AddBackendRoute("www.example.com", "/", "web-backend")

    // Keep running...
}
```

## Backend Provider Interface

```go
type BackendProvider interface {
    // Start begins watching for backend changes and sends events to the channel
    Start(ctx context.Context, eventChan chan<- BackendEvent) error

    // Stop stops the backend provider
    Stop() error

    // GetBackends returns the current list of all backends
    GetBackends() ([]Backend, error)

    // GetBackend returns a specific backend by name
    GetBackend(name string) (*Backend, error)
}
```

## Event Types

```go
type BackendEvent struct {
    Type    BackendEventType  // ADD, UPDATE, DELETE
    Backend Backend           // Backend data
}

const (
    BackendEventAdd    BackendEventType = "ADD"
    BackendEventUpdate BackendEventType = "UPDATE"
    BackendEventDelete BackendEventType = "DELETE"
)
```

## Backend Structure

```go
type Backend struct {
    Name    string          // Backend name
    Servers []BackendServer // List of servers
}

type BackendServer struct {
    Name string // Server name/identifier
    IP   string // IP address
    Port int    // Port number
}
```

## Example Implementations

### 1. Simple Provider (In-Memory)

```go
provider := examples.NewSimpleProvider()

// Add backends manually
provider.AddBackend(gateway.Backend{
    Name: "api-backend",
    Servers: []gateway.BackendServer{
        {Name: "api-1", IP: "10.0.1.10", Port: 8080},
        {Name: "api-2", IP: "10.0.1.11", Port: 8080},
    },
})
```

### 2. Polling Provider (Periodic Fetch)

```go
provider := examples.NewPollingProvider(10*time.Second, func() ([]gateway.Backend, error) {
    // Fetch from your source (database, API, etc.)
    return fetchBackendsFromDatabase()
})
```

### 3. REST API Provider

```go
provider := examples.NewRESTBackendProvider("http://api.example.com/backends", 10*time.Second)
```

**Expected REST API JSON format:**

```json
{
  "backends": [
    {
      "name": "api-backend",
      "servers": [
        {"name": "srv1", "ip": "10.0.1.10", "port": 8080},
        {"name": "srv2", "ip": "10.0.1.11", "port": 8080}
      ]
    },
    {
      "name": "web-backend",
      "servers": [
        {"name": "web1", "ip": "10.0.2.10", "port": 80}
      ]
    }
  ]
}
```

## HTTP/2 Configuration

The gateway automatically configures HAProxy for HTTP/2:

### Frontend (Client-Side HTTP/2)

```
frontend http-gateway
    bind :443 ssl crt /etc/haproxy/certs alpn h2,http/1.1
    mode http
    http-connection-mode http-keep-alive
```

### Backend (Server-Side HTTP/2)

```
backend api-backend
    mode http
    balance roundrobin
    server srv1 10.0.1.10:8080 check alpn h2,http/1.1 proto h2
    server srv2 10.0.1.11:8080 check alpn h2,http/1.1 proto h2
```

## Routing Rules

Add routing rules to direct traffic to specific backends:

```go
// Route by hostname
gw.AddBackendRoute("api.example.com", "", "api-backend")

// Route by path
gw.AddBackendRoute("", "/api", "api-backend")

// Route by hostname and path
gw.AddBackendRoute("www.example.com", "/api", "api-backend")
```

## Configuration Options

### Manager Config

```go
type ManagerConfig struct {
    HAProxyClient api.HAProxyClient  // HAProxy API client
    Provider      BackendProvider     // Your backend provider
    SyncPeriod    time.Duration      // Reconciliation period (default: 5s)
    EventChanSize int                // Event channel buffer size (default: 100)
}
```

### Gateway Config

```go
type GatewayConfig struct {
    FrontendName   string  // Frontend name (default: "http-gateway")
    HTTPPort       int     // HTTP port (default: 80)
    HTTPSPort      int     // HTTPS port (default: 443)
    HTTPSEnabled   bool    // Enable HTTPS
    SSLCertDir     string  // SSL certificate directory
    StrictSNI      bool    // Strict SNI matching
    EnableHTTP2    bool    // Enable HTTP/2
    ALPN           string  // ALPN protocols (default: "h2,http/1.1")
    DefaultBackend string  // Default backend name
    IPv4BindAddr   string  // IPv4 bind address (default: "0.0.0.0")
    IPv6BindAddr   string  // IPv6 bind address (default: "::")
}
```

## Direct HAProxy Management (No K8s)

This gateway operates completely independently of Kubernetes:

1. **No K8s Dependencies**: Doesn't require Kubernetes API or resources
2. **Direct HAProxy Control**: Uses HAProxy Client-Native API and Runtime Socket
3. **Custom Backend Sources**: Implement providers for any backend source
4. **Standalone Operation**: Can run as a standalone binary

## Communication Flow

### Backend Updates (No Reload Required)

```
Provider → Event Channel → Manager
                             ↓
                    Runtime Socket API
                             ↓
               "set server backend/srv1 addr 10.0.0.1 port 8080"
                             ↓
                    HAProxy Process (no reload)
```

### Configuration Changes (Reload Required)

```
Provider → Event Channel → Manager
                             ↓
                    Transaction API
                             ↓
               Create/Update Backend Config
                             ↓
                    Commit Transaction
                             ↓
                    SIGUSR2 (graceful reload)
```

## Testing

Run the example:

```bash
go run cmd/http-gateway/main.go
```

Test HTTP/2 with curl:

```bash
# Test HTTP/2
curl -v --http2 https://api.example.com/api

# Check protocol negotiation
curl -v --http2-prior-knowledge http://localhost:8080/
```

## Integration Examples

### Database Provider

```go
type DBProvider struct {
    db *sql.DB
}

func (p *DBProvider) Start(ctx context.Context, eventChan chan<- gateway.BackendEvent) error {
    // Poll database for backend changes
    // Or use database triggers/notifications
}
```

### Consul Provider

```go
type ConsulProvider struct {
    client *consul.Client
}

func (p *ConsulProvider) Start(ctx context.Context, eventChan chan<- gateway.BackendEvent) error {
    // Watch Consul for service changes
    // Send events when services are added/removed
}
```

### etcd Provider

```go
type EtcdProvider struct {
    client *clientv3.Client
}

func (p *EtcdProvider) Start(ctx context.Context, eventChan chan<- gateway.BackendEvent) error {
    // Watch etcd keys for backend definitions
    // Send events on changes
}
```

## Performance Considerations

1. **Event Channel Size**: Increase `EventChanSize` for high-frequency updates
2. **Sync Period**: Adjust `SyncPeriod` based on your reconciliation needs
3. **Runtime Updates**: Server address changes use runtime socket (fast, no reload)
4. **Configuration Changes**: Backend creation requires HAProxy reload (graceful)

## Troubleshooting

### Enable Debug Logging

```go
logger := utils.GetLogger()
logger.SetLevel("debug")
```

### Check HAProxy Status

```bash
# Via runtime socket
echo "show backend" | socat - /var/run/haproxy-runtime-api.sock

# Check servers
echo "show servers state" | socat - /var/run/haproxy-runtime-api.sock
```

### Verify HTTP/2

```bash
# Check ALPN negotiation
openssl s_client -connect localhost:443 -alpn h2,http/1.1
```

## License

Apache License 2.0
