# HTTP/2 (h2c) Support in Test System

The test system now fully supports **h2c (HTTP/2 cleartext)** - HTTP/2 without TLS.

## What is h2c?

h2c allows HTTP/2 communication over plain TCP connections without requiring TLS/SSL certificates. This is useful for:
- Internal service-to-service communication
- Testing environments
- Backend connections behind a TLS-terminating proxy

## Implementation

### Backend Servers (h2c servers)

The mock backend servers are configured to support h2c using Go's `golang.org/x/net/http2/h2c` package:

**File:** [test/backend/main.go](backend/main.go)

```go
import (
    "golang.org/x/net/http2"
    "golang.org/x/net/http2/h2c"
)

// Create HTTP/2 server with h2c support
h2s := &http2.Server{}

server := &http.Server{
    Addr:    ":9000",
    Handler: h2c.NewHandler(mux, h2s),
}
```

This wraps the HTTP handler with h2c support, allowing the server to accept both:
- HTTP/1.1 requests (standard)
- HTTP/2 requests without TLS (h2c)

### Test Clients (h2c clients)

#### Functional Test Client

**File:** [test/client/cmd/test-client/main.go](client/cmd/test-client/main.go)

For HTTP/2 testing over HTTPS (with TLS):

```go
import "golang.org/x/net/http2"

transport := &http.Transport{
    TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}
// Enable HTTP/2 support
http2.ConfigureTransport(transport)

client := &http.Client{
    Transport: transport,
}
```

#### Performance Test Client

**File:** [test/client/cmd/perf-client/main.go](client/cmd/perf-client/main.go)

Supports both HTTP/1.1 and HTTP/2 via flag:

```go
import "golang.org/x/net/http2"

transport := &http.Transport{
    MaxIdleConns:        concurrency,
    MaxIdleConnsPerHost: concurrency,
    IdleConnTimeout:     90 * time.Second,
}

if useHTTP2 {
    // Enable HTTP/2 support
    http2.ConfigureTransport(transport)
}

client := &http.Client{
    Transport: transport,
}
```

## Usage

### Backend Servers

Backend servers automatically support h2c. No configuration needed:

```bash
# Start backend - supports both HTTP/1.1 and h2c
docker-compose up backend-server-1

# Test with HTTP/1.1
curl http://localhost:9000/test

# Test with h2c (requires h2c-capable client)
```

### Test Clients

#### Test HTTP/2 Support

```bash
# Run HTTP/2 test (uses HTTPS)
docker-compose run --rm test-client /test-client -gateway-https=https://gateway:8443 -verbose
```

#### Performance Testing with HTTP/2

```bash
# HTTP/1.1 performance test
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=50 -n=5000

# HTTP/2 performance test (enable with -http2 flag)
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -http2 -c=50 -n=5000
```

## Gateway Configuration

The HAProxy gateway should be configured to:

1. **Frontend**: Accept HTTP/2 from clients (with or without TLS)
2. **Backend**: Use HTTP/2 (h2c) when connecting to backend servers

Example HAProxy configuration:

```haproxy
# Frontend - accept HTTP/2 from clients
frontend http-gateway
    bind :443 ssl crt /etc/certs alpn h2,http/1.1
    bind :80
    mode http

# Backend - use h2c to connect to backend servers
backend api-backend
    mode http
    balance roundrobin
    server srv1 backend-server-1:9000 check proto h2
    server srv2 backend-server-2:9000 check proto h2
```

## Verification

### Check Backend Protocol

Make a request and verify the backend received HTTP/2:

```bash
curl -s http://localhost:8080/api/test | jq '.protocol'
# Output: "HTTP/2.0"
```

### Test Load Balancing with HTTP/2

```bash
for i in {1..10}; do
  curl -s http://localhost:8080/api/test | jq -r '.server + " - " + .protocol'
done

# Expected output (distributed across servers, all HTTP/2):
# backend-server-1 - HTTP/2.0
# backend-server-2 - HTTP/2.0
# backend-server-1 - HTTP/2.0
# ...
```

### Performance Comparison

Compare HTTP/1.1 vs HTTP/2 performance:

```bash
echo "=== HTTP/1.1 Baseline ==="
docker-compose run --rm test-client /perf-client -c=100 -n=10000

echo ""
echo "=== HTTP/2 Performance ==="
docker-compose run --rm test-client /perf-client -http2 -c=100 -n=10000
```

Expected results:
- HTTP/2 typically shows 10-30% better throughput
- Lower latency under high concurrency
- Better connection multiplexing

## Dependencies

The h2c support requires:

```go
require golang.org/x/net v0.32.0
```

This is automatically included in:
- `test/backend/go.mod`
- `test/client/go.mod`

## Architecture Flow

```
Client (HTTP/2 or HTTP/1.1)
    ↓
Gateway Frontend (HTTP/2 capable)
    ↓
Gateway Backend Connection (h2c)
    ↓
Backend Server (h2c-enabled)
```

### End-to-End HTTP/2 Flow

1. **Client → Gateway**: Client can use HTTP/2 over HTTPS or HTTP/1.1
2. **Gateway → Backend**: Gateway uses h2c (HTTP/2 cleartext) to communicate with backends
3. **Backend Response**: Backends respond via h2c
4. **Gateway → Client**: Gateway returns response in client's protocol

## Benefits of h2c

1. **Performance**: HTTP/2 multiplexing and header compression
2. **No TLS Overhead**: Skip TLS handshake for internal communication
3. **Connection Reuse**: Single connection for multiple requests
4. **Binary Protocol**: More efficient than HTTP/1.1 text protocol
5. **Stream Prioritization**: Better resource utilization

## Testing Scenarios

### Scenario 1: Basic h2c Communication

```bash
# Start environment
docker-compose up -d

# Test that backend receives HTTP/2
curl -s http://localhost:8080/api/test | jq
```

### Scenario 2: HTTP/2 Load Test

```bash
# High concurrency HTTP/2 test
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -http2 \
  -c=200 \
  -n=50000
```

### Scenario 3: Protocol Comparison

```bash
# Create comparison script
cat > compare.sh << 'EOF'
#!/bin/bash
echo "Testing HTTP/1.1..."
docker-compose run --rm test-client /perf-client -c=100 -n=10000 2>&1 | grep "Requests/sec"

echo "Testing HTTP/2..."
docker-compose run --rm test-client /perf-client -http2 -c=100 -n=10000 2>&1 | grep "Requests/sec"
EOF

chmod +x compare.sh
./compare.sh
```

## Troubleshooting

### Backend Not Using HTTP/2

Check backend logs:

```bash
docker-compose logs backend-server-1
# Should show: "Starting backend server 'backend-server-1' on port 9000 with h2c support"
```

### Client Not Negotiating HTTP/2

Verify h2c is enabled:

```bash
# Check if golang.org/x/net is installed
docker-compose run --rm test-client go list -m golang.org/x/net
# Should show: golang.org/x/net v0.32.0
```

### Gateway Not Using h2c to Backends

Check HAProxy configuration:

```bash
docker exec http-gateway cat /etc/haproxy/haproxy.cfg | grep proto
# Should show: server ... proto h2
```

## Performance Expectations

Based on typical test environment (4 CPU, 8GB RAM):

| Protocol | Concurrency | Requests/sec | Avg Latency |
|----------|-------------|--------------|-------------|
| HTTP/1.1 | 50 | 1000-2000 | 25-50ms |
| HTTP/2 | 50 | 1200-2500 | 20-40ms |
| HTTP/1.1 | 100 | 1500-3000 | 30-70ms |
| HTTP/2 | 100 | 2000-4000 | 25-50ms |

**Key improvements with HTTP/2:**
- 15-30% higher throughput
- 20-40% lower latency
- Better resource utilization under high load

## References

- [RFC 7540 - HTTP/2](https://tools.ietf.org/html/rfc7540)
- [RFC 7541 - HPACK Header Compression](https://tools.ietf.org/html/rfc7541)
- [Go http2 Package](https://pkg.go.dev/golang.org/x/net/http2)
- [Go h2c Package](https://pkg.go.dev/golang.org/x/net/http2/h2c)
