# HAProxy HTTP Gateway Client Library

This package provides a programmatic Go client for interacting with the HAProxy HTTP Gateway without using HTTP API calls.

## Features

- **Direct Backend Management**: Register and unregister backends programmatically
- **Route Management**: Add and remove routing rules
- **Type-Safe API**: Strongly typed Go interfaces
- **No HTTP Overhead**: Direct function calls to the gateway manager
- **Thread-Safe**: Safe for concurrent use

## Usage

### Basic Example

```go
package main

import (
    "fmt"
    "github.com/haproxytech/kubernetes-ingress/pkg/client"
    "github.com/haproxytech/kubernetes-ingress/pkg/gateway"
)

func main() {
    // Get the frontend manager from your gateway instance
    var frontendManager *gateway.FrontendManager
    // ... initialize frontend manager ...

    // Create client
    c := client.NewClient(frontendManager)

    // Register a backend
    backend := gateway.Backend{
        Name: "my-backend",
        Servers: []gateway.BackendServer{
            {
                Name: "server1",
                IP:   "192.168.1.10",
                Port: 8080,
            },
            {
                Name: "server2",
                IP:   "192.168.1.11",
                Port: 8080,
            },
        },
    }

    err := c.RegisterBackend("default", backend)
    if err != nil {
        fmt.Printf("Failed to register backend: %v\n", err)
        return
    }

    fmt.Println("Backend registered successfully")

    // Get all backends
    backends, err := c.GetBackends("default")
    if err != nil {
        fmt.Printf("Failed to get backends: %v\n", err)
        return
    }

    for name, backend := range backends {
        fmt.Printf("Backend: %s, Servers: %d\n", name, len(backend.Servers))
    }

    // Unregister backend
    err = c.UnregisterBackend("default", "my-backend")
    if err != nil {
        fmt.Printf("Failed to unregister backend: %v\n", err)
        return
    }

    fmt.Println("Backend unregistered successfully")
}
```

### Route Management Example

```go
// Add a route
route := gateway.Route{
    ID:          "api-route",
    Host:        "api.example.com",
    Path:        "/v1",
    BackendName: "api-backend",
    FrontendID:  "default",
}

err := c.AddRoute("default", route)
if err != nil {
    fmt.Printf("Failed to add route: %v\n", err)
}

// Get all routes
routes, err := c.GetRoutes("default")
if err != nil {
    fmt.Printf("Failed to get routes: %v\n", err)
}

// Remove a route
err = c.RemoveRoute("default", "api-route")
if err != nil {
    fmt.Printf("Failed to remove route: %v\n", err)
}
```

### Multi-Frontend Example

```go
// List all frontends
frontends := c.GetFrontends()
for _, frontend := range frontends {
    fmt.Printf("Frontend: %s (%s)\n", frontend.ID, frontend.Name)
}

// Register backend to specific frontend
err := c.RegisterBackend("frontend-api", gateway.Backend{
    Name: "api-backend",
    Servers: []gateway.BackendServer{
        {Name: "api-1", IP: "10.0.1.10", Port: 8080},
        {Name: "api-2", IP: "10.0.1.11", Port: 8080},
    },
})
```

## API Reference

### Client Methods

#### `RegisterBackend(frontendID string, backend Backend) error`
Registers a backend with the specified frontend.

**Parameters:**
- `frontendID`: The ID of the frontend to register the backend with
- `backend`: Backend definition including name and servers

**Returns:** Error if registration fails

#### `UnregisterBackend(frontendID string, backendName string) error`
Removes a backend from the specified frontend.

**Parameters:**
- `frontendID`: The ID of the frontend
- `backendName`: Name of the backend to remove

**Returns:** Error if unregistration fails

#### `GetBackends(frontendID string) (map[string]Backend, error)`
Returns all backends registered to the specified frontend.

**Parameters:**
- `frontendID`: The ID of the frontend

**Returns:** Map of backend names to Backend objects, or error

#### `AddRoute(frontendID string, route Route) error`
Adds a routing rule to the specified frontend.

**Parameters:**
- `frontendID`: The ID of the frontend
- `route`: Route definition

**Returns:** Error if route addition fails

#### `RemoveRoute(frontendID string, routeID string) error`
Removes a routing rule from the specified frontend.

**Parameters:**
- `frontendID`: The ID of the frontend
- `routeID`: ID of the route to remove

**Returns:** Error if route removal fails

#### `GetRoutes(frontendID string) (map[string]Route, error)`
Returns all routes for the specified frontend.

**Parameters:**
- `frontendID`: The ID of the frontend

**Returns:** Map of route IDs to Route objects, or error

#### `GetFrontends() []FrontendDefinition`
Returns all configured frontends.

**Returns:** Array of frontend definitions

## Data Structures

### Backend
```go
type Backend struct {
    Name    string
    Servers []BackendServer
}
```

### BackendServer
```go
type BackendServer struct {
    Name string
    IP   string
    Port int
}
```

### Route
```go
type Route struct {
    ID          string
    Host        string
    Path        string
    BackendName string
    FrontendID  string
}
```

## Comparison: Client Library vs HTTP API

| Feature | Client Library | HTTP API |
|---------|---------------|----------|
| **Type Safety** | ✅ Compile-time type checking | ❌ Runtime validation only |
| **Performance** | ✅ Direct function calls | ❌ HTTP overhead |
| **Error Handling** | ✅ Go error types | ❌ HTTP status codes + JSON |
| **Deployment** | ❌ Same process only | ✅ Remote access |
| **Authentication** | ✅ Implicit (in-process) | ❌ Requires auth mechanism |
| **Use Case** | Internal/embedded | External/remote clients |

## Use Cases

### When to Use the Client Library

1. **Embedded Applications**: When the gateway runs in the same process
2. **Testing**: Unit and integration tests
3. **Performance Critical**: When HTTP overhead is unacceptable
4. **Type Safety**: When compile-time type checking is required
5. **Internal Tools**: CLI tools that link against the gateway library

### When to Use the HTTP API

1. **Remote Access**: Accessing gateway from different processes/hosts
2. **Language Agnostic**: Clients written in other languages
3. **Microservices**: Distributed architectures
4. **Web Frontends**: Browser-based management interfaces

## Testing

The package includes comprehensive tests:

```bash
# Run unit tests
go test ./pkg/client/...

# Run integration tests (requires running gateway)
go test ./test/client/... -v

# Skip integration tests
go test ./test/client/... -short
```

## Thread Safety

The client library is thread-safe and can be used concurrently from multiple goroutines. All operations are protected by the underlying frontend manager's mutex.

## Error Handling

All client methods return Go errors. Common error scenarios:

- **Frontend not found**: `frontend <id> not found`
- **Backend not found**: `backend <name> not found`
- **Invalid configuration**: Validation errors from HAProxy client

Example:
```go
err := c.RegisterBackend("default", backend)
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        // Handle not found error
    } else {
        // Handle other errors
    }
}
```

## Future Enhancements

- [ ] gRPC service for remote access
- [ ] Event subscriptions (watch for backend changes)
- [ ] Batch operations (register multiple backends at once)
- [ ] Health check integration
- [ ] Metrics and observability
