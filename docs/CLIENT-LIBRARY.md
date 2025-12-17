# HAProxy HTTP Gateway - Client Library

## Overview

The HAProxy HTTP Gateway now provides a **Go client library** (`pkg/client`) for programmatic backend registration/deregistration without using HTTP API calls.

## Why Use the Client Library?

### Benefits

✅ **Type-Safe**: Compile-time type checking
✅ **Performance**: Direct function calls, no HTTP overhead
✅ **Simplicity**: No need to marshal/unmarshal JSON
✅ **Error Handling**: Native Go errors instead of HTTP status codes
✅ **Thread-Safe**: Safe for concurrent use

### When to Use

- **Embedded applications**: Gateway runs in the same process
- **Testing**: Unit and integration tests
- **CLI tools**: Command-line tools that link against the gateway
- **Performance critical**: When HTTP overhead is unacceptable

## Quick Start

### Installation

```go
import "github.com/haproxytech/kubernetes-ingress/pkg/client"
```

### Basic Usage

```go
// Create client
c := client.NewClient(frontendManager)

// Register a backend
backend := gateway.Backend{
    Name: "my-backend",
    Servers: []gateway.BackendServer{
        {Name: "server1", IP: "192.168.1.10", Port: 8080},
        {Name: "server2", IP: "192.168.1.11", Port: 8080},
    },
}

err := c.RegisterBackend("default", backend)
if err != nil {
    log.Fatal(err)
}

// Unregister a backend
err = c.UnregisterBackend("default", "my-backend")
```

## API Reference

### Client Creation

```go
func NewClient(fm *gateway.FrontendManager) *Client
```

### Backend Management

#### RegisterBackend
```go
func (c *Client) RegisterBackend(frontendID string, backend gateway.Backend) error
```

Registers a backend with the specified frontend.

**Example:**
```go
backend := gateway.Backend{
    Name: "web-backend",
    Servers: []gateway.BackendServer{
        {Name: "web-1", IP: "10.0.1.10", Port: 80},
    },
}
err := c.RegisterBackend("default", backend)
```

#### UnregisterBackend
```go
func (c *Client) UnregisterBackend(frontendID string, backendName string) error
```

Removes a backend from the specified frontend.

**Example:**
```go
err := c.UnregisterBackend("default", "web-backend")
```

#### GetBackends
```go
func (c *Client) GetBackends(frontendID string) (map[string]*gateway.Backend, error)
```

Returns all backends for a frontend.

**Example:**
```go
backends, err := c.GetBackends("default")
for name, backend := range backends {
    fmt.Printf("%s: %d servers\n", name, len(backend.Servers))
}
```

### Route Management

#### AddRoute
```go
func (c *Client) AddRoute(frontendID string, route gateway.Route) error
```

Adds a routing rule.

**Example:**
```go
route := gateway.Route{
    ID:          "api-route",
    Host:        "api.example.com",
    Path:        "/v1",
    BackendName: "api-backend",
}
err := c.AddRoute("default", route)
```

#### DeleteRoute
```go
func (c *Client) DeleteRoute(frontendID string, routeID string) error
```

Removes a routing rule.

#### GetRoutes
```go
func (c *Client) GetRoutes(frontendID string) (map[string]gateway.Route, error)
```

Returns all routes for a frontend.

### Frontend Management

#### ListFrontends
```go
func (c *Client) ListFrontends() map[string]*gateway.ManagedFrontend
```

Returns all configured frontends.

#### GetFrontend
```go
func (c *Client) GetFrontend(frontendID string) (*gateway.ManagedFrontend, error)
```

Returns a specific frontend by ID.

## Complete Example

See [`examples/client-library/main.go`](../examples/client-library/main.go) for a complete working example.

## Comparison: Client Library vs HTTP API

| Feature | Client Library | HTTP API |
|---------|---------------|----------|
| **Access** | In-process only | Remote access |
| **Type Safety** | ✅ Compile-time | ❌ Runtime only |
| **Performance** | ✅ Fast (direct calls) | ❌ HTTP overhead |
| **Language** | Go only | Any language |
| **Authentication** | Not needed | Required |
| **Use Case** | Embedded/Testing | Microservices |

## Testing

### Run Tests

```bash
# Build the library
go build ./pkg/client/...

# Run unit tests
go test ./pkg/client/...

# Run example
go run ./examples/client-library/main.go
```

## Architecture

```
┌─────────────────┐
│  Your App/Test  │
└────────┬────────┘
         │
         │ import
         ▼
┌─────────────────┐
│  Client Library │ (pkg/client/client.go)
└────────┬────────┘
         │
         │ calls
         ▼
┌─────────────────┐
│FrontendManager  │ (pkg/gateway/frontend_manager.go)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   HAProxy API   │
└─────────────────┘
```

## Files Created

- **`pkg/client/client.go`** - Client library implementation
- **`pkg/client/README.md`** - Detailed documentation
- **`test/client/dynamic_backend_test.go`** - Integration tests
- **`examples/client-library/main.go`** - Complete example

## Next Steps

1. ✅ Client library implemented and compiling
2. ⏭️ Add gRPC service for remote access
3. ⏭️ Add event subscriptions (watch backend changes)
4. ⏭️ Add batch operations

## Build Status

```bash
$ go build ./pkg/client/...
✓ Build successful

$ go build ./examples/client-library/...
✓ Build successful
```

The client library is ready to use!
