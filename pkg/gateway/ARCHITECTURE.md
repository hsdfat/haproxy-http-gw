# HTTP Gateway Architecture

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Your Backend Source                          │
│  (Database / REST API / Service Registry / Config File / etc.)  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         │ Fetch/Watch
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                  BackendProvider Interface                      │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ • Start(ctx, eventChan)  → Watch and send events          │  │
│  │ • GetBackends()          → Get all backends               │  │
│  │ • GetBackend(name)       → Get specific backend           │  │
│  │ • Stop()                 → Cleanup                         │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
│  Example Implementations:                                        │
│  • SimpleProvider    - In-memory, manual updates                │
│  • PollingProvider   - Periodic polling with fetch function     │
│  • RESTProvider      - Poll REST API endpoints                  │
│  • Your Custom Provider - Implement for your source!            │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         │ BackendEvent Channel
                         │ (Add/Update/Delete)
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Backend Manager                             │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ Event Processor:                                           │  │
│  │  • Receives BackendEvent from provider                     │  │
│  │  • Maintains backend state map                             │  │
│  │  • Triggers HAProxy sync on changes                        │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ Periodic Reconciler:                                       │  │
│  │  • Runs every 5 seconds (configurable)                     │  │
│  │  • Ensures HAProxy matches desired state                   │  │
│  │  • Recovers from errors                                    │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         │ HAProxy API Calls
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                   HAProxy API Client                            │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ Configuration API (client-native/v6):                      │  │
│  │  • BackendCreateOrUpdate()  - Create/modify backend        │  │
│  │  • BackendServerCreate()    - Add servers                  │  │
│  │  • Transaction management   - Atomic updates               │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ Runtime Socket API:                                        │  │
│  │  • SetServerAddrAndState()  - Dynamic server updates       │  │
│  │  • ExecuteRaw()             - Direct socket commands       │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
          ┌──────────────┴──────────────┐
          │                             │
          ▼                             ▼
┌──────────────────────┐      ┌──────────────────────┐
│ Configuration API    │      │   Runtime Socket     │
│ /etc/haproxy/        │      │ /var/run/haproxy-    │
│ haproxy.cfg          │      │ runtime-api.sock     │
└──────────┬───────────┘      └──────────┬───────────┘
           │                             │
           │ Structural Changes          │ Dynamic Updates
           │ (Requires Reload)           │ (No Reload)
           │                             │
           └──────────────┬──────────────┘
                          │
                          ▼
           ┌──────────────────────────────┐
           │      HAProxy Process         │
           │  ┌────────────────────────┐  │
           │  │ Frontend (HTTP/2)      │  │
           │  │ :80, :443             │  │
           │  │ ALPN: h2,http/1.1     │  │
           │  └───────────┬────────────┘  │
           │              │                │
           │              ▼                │
           │  ┌────────────────────────┐  │
           │  │ Routing Rules/ACLs     │  │
           │  │ • Host matching        │  │
           │  │ • Path matching        │  │
           │  └───────────┬────────────┘  │
           │              │                │
           │              ▼                │
           │  ┌────────────────────────┐  │
           │  │ Backends (HTTP/2)      │  │
           │  │ • api-backend          │  │
           │  │ • web-backend          │  │
           │  │ • ...                  │  │
           │  └───────────┬────────────┘  │
           └──────────────┼────────────────┘
                          │
                          │ Load Balanced
                          │ HTTP/2 Connections
                          ▼
           ┌──────────────────────────────┐
           │     Backend Servers          │
           │  srv1: 10.0.1.10:8080       │
           │  srv2: 10.0.1.11:8080       │
           │  srv3: 10.0.1.12:8080       │
           └──────────────────────────────┘
```

## Event Flow

### 1. Backend Discovery
```
Database/API → Provider.Start() → Watch for changes
                     │
                     ▼
           Detect backend change
                     │
                     ▼
          Create BackendEvent{
              Type: ADD/UPDATE/DELETE,
              Backend: {name, servers[]}
          }
                     │
                     ▼
           Send to eventChan
```

### 2. Event Processing
```
eventChan → Manager.processEvents()
                     │
                     ▼
          Manager.handleBackendEvent()
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
    Update local           Sync to HAProxy
    backend map           (syncBackendToHAProxy)
```

### 3. HAProxy Synchronization

#### Dynamic Update (No Reload)
```
Manager → Runtime Socket API
    │
    ▼
"set server backend/srv1 addr 10.0.1.10 port 8080"
"set server backend/srv1 state ready"
    │
    ▼
HAProxy Process (immediate effect, no reload)
```

#### Configuration Update (With Reload)
```
Manager → APIStartTransaction()
    │
    ▼
BackendCreateOrUpdate(backend)
    │
    ▼
BackendServerCreate(server1)
BackendServerCreate(server2)
    │
    ▼
APICommitTransaction()
    │
    ▼
APIFinalCommitTransaction()
    │
    ▼
Kill -SIGUSR2 <haproxy-pid>
    │
    ▼
HAProxy Graceful Reload (zero downtime)
```

## Data Flow Example

### Scenario: Add new backend server

```
1. Database Update:
   INSERT INTO servers (backend, name, ip, port)
   VALUES ('api-backend', 'srv3', '10.0.1.12', 8080);

2. Provider Polling:
   DBProvider.fetchFromDB() → Detects new server

3. Event Generation:
   eventChan <- BackendEvent{
       Type: UPDATE,
       Backend: {
           Name: "api-backend",
           Servers: [srv1, srv2, srv3],  // srv3 is new
       }
   }

4. Manager Processing:
   • Updates internal state map
   • Calls syncBackendToHAProxy()

5. HAProxy API:
   transaction := APIStartTransaction()
   BackendServerCreate("api-backend", Server{
       Name: "srv3",
       IP: "10.0.1.12",
       Port: 8080,
       Alpn: "h2,http/1.1",
       Proto: "h2",
   })
   APICommitTransaction()

6. HAProxy Reload:
   SIGUSR2 → Graceful reload
   New worker starts with srv3
   Old worker drains connections
   Zero downtime transition

7. Traffic Flow:
   Client → HTTP/2 Request → HAProxy Frontend
                                   ↓
                          Round-robin to srv1/srv2/srv3
                                   ↓
                          HTTP/2 Connection to backend
```

## HTTP/2 Configuration Details

### Frontend Configuration
```haproxy
frontend http-gateway
    # HTTP/2 on HTTPS
    bind :443 ssl crt /etc/haproxy/certs alpn h2,http/1.1

    # Plain HTTP (upgradeable to h2c)
    bind :80

    mode http
    http-connection-mode http-keep-alive

    # Routing rules
    use_backend api-backend if { hdr(host) -i api.example.com }
    use_backend web-backend if { hdr(host) -i www.example.com }

    default_backend api-backend
```

### Backend Configuration
```haproxy
backend api-backend
    mode http
    balance roundrobin

    # HTTP/2 to backend servers
    server srv1 10.0.1.10:8080 check alpn h2,http/1.1 proto h2
    server srv2 10.0.1.11:8080 check alpn h2,http/1.1 proto h2
    server srv3 10.0.1.12:8080 check alpn h2,http/1.1 proto h2
```

### ALPN Negotiation
```
Client                    HAProxy                   Backend
  │                          │                         │
  │ ClientHello (ALPN: h2)   │                         │
  ├─────────────────────────→│                         │
  │                          │                         │
  │ ServerHello (ALPN: h2)   │                         │
  │←─────────────────────────┤                         │
  │                          │                         │
  │   HTTP/2 Connection      │  ClientHello (ALPN:h2) │
  │←────────────────────────→│────────────────────────→│
  │                          │                         │
  │                          │  ServerHello (ALPN:h2) │
  │                          │←────────────────────────┤
  │                          │                         │
  │                          │   HTTP/2 Connection     │
  │                          │←───────────────────────→│
```

## Component Interaction Timeline

```
Time →
Provider        Manager         HAProxy API      HAProxy Process
   │               │                │                  │
   ├─Start()───────→               │                  │
   │               ├─Start()────────→                 │
   │               │                ├─Configure───────→│
   │               │                │                  │
   ├─Poll DB───────┐               │                  │
   │               │                │                  │
   ├─Backend Change│               │                  │
   │               │                │                  │
   ├─Event────────→│                │                  │
   │               ├─Handle Event───┐                 │
   │               │                │                  │
   │               ├─Transaction────→                 │
   │               │                ├─Update Config──→│
   │               │                │                  │
   │               │                ├─Commit─────────→│
   │               │                │                  ├─Reload
   │               │                │                  │
   │               │←───Success─────┤                  │
   │               │                │                  │
   ├─Poll DB───────┐               │                  │
   │ (10s later)   │                │                  │
   │               │                │                  │
   ├─No Change─────┘               │                  │
   │               │                │                  │
   │               ├─Reconcile──────→                 │
   │               │ (5s interval)  │                  │
   │               │                ├─Check State────→│
   │               │                │                  │
   │               │←───OK──────────┤                  │
   │               │                │                  │
   ▼               ▼                ▼                  ▼
```

## Interface Contract

### Provider → Manager
```go
// Provider sends events through channel
eventChan <- BackendEvent{
    Type: BackendEventAdd,
    Backend: Backend{
        Name: "my-backend",
        Servers: []BackendServer{...},
    },
}
```

### Manager → HAProxy API
```go
// Manager calls HAProxy API methods
haproxyClient.APIStartTransaction()
haproxyClient.BackendCreateOrUpdate(backend)
haproxyClient.BackendServerCreate(backendName, server)
haproxyClient.APICommitTransaction()
haproxyClient.APIFinalCommitTransaction()
```

### HAProxy API → HAProxy Process
```
# Runtime Socket (Unix domain socket)
/var/run/haproxy-runtime-api.sock

# Configuration File
/etc/haproxy/haproxy.cfg

# Process Signal
kill -SIGUSR2 <pid>
```

## Scalability and Performance

### Event Processing
- **Async**: Non-blocking event channel
- **Buffered**: Configurable channel size (default: 100)
- **Batched**: Multiple events processed in one sync cycle

### HAProxy Updates
- **Runtime Updates**: Microseconds (no reload)
- **Config Updates**: ~1 second (graceful reload)
- **Zero Downtime**: Seamless transitions

### Resource Usage
- **Memory**: O(n) where n = number of backends
- **CPU**: Minimal (event-driven, not polling)
- **Network**: Only when backends change

## Error Handling

```
Provider Error → Logged, retry on next poll
    │
Manager Event Error → Logged, state preserved
    │
HAProxy API Error → Transaction rollback, previous state restored
    │
HAProxy Reload Error → Old process continues, alert logged
```

## Summary

This architecture provides:
1. **Flexible Backend Discovery**: Implement provider for any source
2. **Event-Driven Updates**: Real-time backend changes
3. **HTTP/2 Support**: Full HTTP/2 throughout the stack
4. **No Kubernetes**: Standalone operation
5. **Zero Downtime**: Graceful reloads and runtime updates
6. **Fault Tolerant**: Error recovery and reconciliation
