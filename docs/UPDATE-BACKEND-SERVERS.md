# Updating Backend Server Lists

## Overview

To update the server list for an existing backend, simply **re-register** the backend with the same name but different servers. The gateway will update the backend configuration automatically.

## How It Works

The `RegisterBackend` function is **idempotent** - it creates or updates:

```go
// In manager.go line 279
m.backends[backend.Name] = &backend  // Overwrites if exists, creates if new
```

## Methods

### Method 1: Using Client Library (Recommended)

```go
// Initial registration
backend := gateway.Backend{
    Name: "my-backend",
    Servers: []gateway.BackendServer{
        {Name: "server1", IP: "192.168.1.10", Port: 8080},
        {Name: "server2", IP: "192.168.1.11", Port: 8080},
    },
}
client.RegisterBackend("default", backend)

// Update: Add a new server
backend.Servers = append(backend.Servers, gateway.BackendServer{
    Name: "server3",
    IP:   "192.168.1.12",
    Port: 8080,
})
client.RegisterBackend("default", backend)  // Updates existing backend

// Update: Remove a server
backend.Servers = []gateway.BackendServer{
    {Name: "server1", IP: "192.168.1.10", Port: 8080},
    // server2 and server3 removed
}
client.RegisterBackend("default", backend)  // Updates with new list

// Update: Change server IPs/ports
backend.Servers = []gateway.BackendServer{
    {Name: "server1", IP: "192.168.1.20", Port: 9000},  // Changed IP and port
}
client.RegisterBackend("default", backend)  // Updates configuration
```

### Method 2: Using HTTP API

```bash
# Initial registration
curl -X POST http://localhost:9090/api/frontends/default/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-backend",
    "servers": [
      {"name": "server1", "ip": "192.168.1.10", "port": 8080},
      {"name": "server2", "ip": "192.168.1.11", "port": 8080}
    ]
  }'

# Update: Add server3
curl -X POST http://localhost:9090/api/frontends/default/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-backend",
    "servers": [
      {"name": "server1", "ip": "192.168.1.10", "port": 8080},
      {"name": "server2", "ip": "192.168.1.11", "port": 8080},
      {"name": "server3", "ip": "192.168.1.12", "port": 8080}
    ]
  }'

# Update: Change server IPs
curl -X POST http://localhost:9090/api/frontends/default/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-backend",
    "servers": [
      {"name": "server1", "ip": "192.168.1.20", "port": 9000},
      {"name": "server2", "ip": "192.168.1.21", "port": 9000}
    ]
  }'
```

## Complete Example

```go
package main

import (
    "fmt"
    "time"
    "github.com/haproxytech/kubernetes-ingress/pkg/client"
    "github.com/haproxytech/kubernetes-ingress/pkg/gateway"
)

func main() {
    // Assume client is already created
    var c *client.Client
    frontendID := "default"

    // Step 1: Initial registration with 2 servers
    fmt.Println("Step 1: Register backend with 2 servers")
    backend := gateway.Backend{
        Name: "my-backend",
        Servers: []gateway.BackendServer{
            {Name: "server1", IP: "192.168.1.10", Port: 8080},
            {Name: "server2", IP: "192.168.1.11", Port: 8080},
        },
    }
    c.RegisterBackend(frontendID, backend)

    time.Sleep(2 * time.Second)

    // Verify
    backends, _ := c.GetBackends(frontendID)
    fmt.Printf("Server count: %d\n", len(backends["my-backend"].Servers))
    // Output: Server count: 2

    // Step 2: Scale up - add server3
    fmt.Println("\nStep 2: Add server3")
    backend.Servers = append(backend.Servers, gateway.BackendServer{
        Name: "server3",
        IP:   "192.168.1.12",
        Port: 8080,
    })
    c.RegisterBackend(frontendID, backend)

    time.Sleep(2 * time.Second)

    backends, _ = c.GetBackends(frontendID)
    fmt.Printf("Server count: %d\n", len(backends["my-backend"].Servers))
    // Output: Server count: 3

    // Step 3: Scale down - remove server2
    fmt.Println("\nStep 3: Remove server2")
    backend.Servers = []gateway.BackendServer{
        {Name: "server1", IP: "192.168.1.10", Port: 8080},
        {Name: "server3", IP: "192.168.1.12", Port: 8080},
    }
    c.RegisterBackend(frontendID, backend)

    time.Sleep(2 * time.Second)

    backends, _ = c.GetBackends(frontendID)
    fmt.Printf("Server count: %d\n", len(backends["my-backend"].Servers))
    // Output: Server count: 2

    // Step 4: Update server IPs (e.g., after pod recreation)
    fmt.Println("\nStep 4: Update server IPs")
    backend.Servers = []gateway.BackendServer{
        {Name: "server1", IP: "192.168.1.20", Port: 8080},  // New IP
        {Name: "server3", IP: "192.168.1.22", Port: 8080},  // New IP
    }
    c.RegisterBackend(frontendID, backend)

    time.Sleep(2 * time.Second)

    backends, _ = c.GetBackends(frontendID)
    for _, srv := range backends["my-backend"].Servers {
        fmt.Printf("  %s: %s:%d\n", srv.Name, srv.IP, srv.Port)
    }
    // Output:
    //   server1: 192.168.1.20:8080
    //   server3: 192.168.1.22:8080
}
```

## Common Patterns

### Pattern 1: Add a Server

```go
// Get current backend
backends, _ := client.GetBackends("default")
backend := backends["my-backend"]

// Add new server
backend.Servers = append(backend.Servers, gateway.BackendServer{
    Name: "new-server",
    IP:   "192.168.1.100",
    Port: 8080,
})

// Update
client.RegisterBackend("default", *backend)
```

### Pattern 2: Remove a Server

```go
// Get current backend
backends, _ := client.GetBackends("default")
backend := backends["my-backend"]

// Remove server by name
newServers := []gateway.BackendServer{}
for _, srv := range backend.Servers {
    if srv.Name != "server-to-remove" {
        newServers = append(newServers, srv)
    }
}
backend.Servers = newServers

// Update
client.RegisterBackend("default", *backend)
```

### Pattern 3: Update Server IP (e.g., after pod restart)

```go
// Get current backend
backends, _ := client.GetBackends("default")
backend := backends["my-backend"]

// Update specific server
for i := range backend.Servers {
    if backend.Servers[i].Name == "server1" {
        backend.Servers[i].IP = "192.168.1.99"  // New IP
        break
    }
}

// Update
client.RegisterBackend("default", *backend)
```

### Pattern 4: Replace All Servers (for rolling updates)

```go
// Complete replacement
newBackend := gateway.Backend{
    Name: "my-backend",
    Servers: []gateway.BackendServer{
        {Name: "new-server1", IP: "10.0.1.10", Port: 8080},
        {Name: "new-server2", IP: "10.0.1.11", Port: 8080},
        {Name: "new-server3", IP: "10.0.1.12", Port: 8080},
    },
}

client.RegisterBackend("default", newBackend)
```

## Important Notes

### 1. Atomic Updates
The entire server list is replaced atomically. You must provide the **complete** desired server list.

**❌ Wrong:**
```go
// This will replace the entire backend with only server3!
backend := gateway.Backend{
    Name: "my-backend",
    Servers: []gateway.BackendServer{
        {Name: "server3", IP: "192.168.1.12", Port: 8080},
    },
}
client.RegisterBackend("default", backend)
// server1 and server2 are now gone!
```

**✅ Correct:**
```go
// Get current servers first
backends, _ := client.GetBackends("default")
currentBackend := backends["my-backend"]

// Add to existing list
currentBackend.Servers = append(currentBackend.Servers, gateway.BackendServer{
    Name: "server3",
    IP:   "192.168.1.12",
    Port: 8080,
})

// Update with complete list
client.RegisterBackend("default", *currentBackend)
```

### 2. Zero-Downtime Updates
HAProxy handles server list updates gracefully:
- Existing connections to removed servers are allowed to complete
- New connections immediately use the new server list
- No traffic interruption

### 3. Server Names
Server names must be unique within a backend. If you use the same name with different IP/port, the existing server is updated.

## Helper Functions

You can create helper functions for common operations:

```go
// AddServer adds a server to an existing backend
func AddServer(c *client.Client, frontendID, backendName string, server gateway.BackendServer) error {
    backends, err := c.GetBackends(frontendID)
    if err != nil {
        return err
    }

    backend, exists := backends[backendName]
    if !exists {
        return fmt.Errorf("backend %s not found", backendName)
    }

    backend.Servers = append(backend.Servers, server)
    return c.RegisterBackend(frontendID, *backend)
}

// RemoveServer removes a server from an existing backend
func RemoveServer(c *client.Client, frontendID, backendName, serverName string) error {
    backends, err := c.GetBackends(frontendID)
    if err != nil {
        return err
    }

    backend, exists := backends[backendName]
    if !exists {
        return fmt.Errorf("backend %s not found", backendName)
    }

    newServers := []gateway.BackendServer{}
    for _, srv := range backend.Servers {
        if srv.Name != serverName {
            newServers = append(newServers, srv)
        }
    }

    backend.Servers = newServers
    return c.RegisterBackend(frontendID, *backend)
}

// UpdateServerIP updates the IP of a specific server
func UpdateServerIP(c *client.Client, frontendID, backendName, serverName, newIP string) error {
    backends, err := c.GetBackends(frontendID)
    if err != nil {
        return err
    }

    backend, exists := backends[backendName]
    if !exists {
        return fmt.Errorf("backend %s not found", backendName)
    }

    for i := range backend.Servers {
        if backend.Servers[i].Name == serverName {
            backend.Servers[i].IP = newIP
            return c.RegisterBackend(frontendID, *backend)
        }
    }

    return fmt.Errorf("server %s not found in backend %s", serverName, backendName)
}
```

## Summary

**To update servers in a backend:**

1. ✅ Call `RegisterBackend` with the same backend name
2. ✅ Provide the **complete** new server list
3. ✅ The backend configuration is updated atomically
4. ✅ No separate "update" method needed - `RegisterBackend` handles both create and update
