# Gateway Features & Protocol Support

Complete guide to HTTP/2 support, protocol handling, and IP auto-detection in the HAProxy HTTP Gateway test system.

## Table of Contents

1. [Protocol Support](#protocol-support)
2. [HTTP/2 (H2C) Support](#http2-h2c-support)
3. [IP Auto-Detection](#ip-auto-detection)
4. [Performance Comparison](#performance-comparison)

---

## Protocol Support

The gateway supports both **HTTP/1.1** and **HTTP/2** (H2C - HTTP/2 Cleartext) protocols simultaneously.

### Configuration

#### Frontend Binding

```haproxy
frontend http-gateway
    bind :8080
    bind [::]:8080 v4v6
    mode http
    option http-use-htx
    default_backend api-backend
```

**Key Points:**
- **No `proto h2` on bind**: Allows both HTTP/1.1 and HTTP/2
- **`option http-use-htx`**: Enables HAProxy's internal HTX (HTTP Transaction) representation
- **Client-driven protocol**: The client chooses which protocol to use

### Protocol Selection

#### HTTP/1.1 (Default)

Standard curl requests use HTTP/1.1:

```bash
curl http://localhost:8080/
```

Response headers:
```
< HTTP/1.1 200 OK
```

#### HTTP/2 (H2C - Cleartext)

For HTTP/2 without TLS, use the `--http2-prior-knowledge` flag:

```bash
curl --http2-prior-knowledge http://localhost:8080/
```

Response headers:
```
< HTTP/2 200
```

#### HTTP/2 (HTTPS)

For HTTP/2 over TLS with ALPN:

```bash
curl -k --http2 https://localhost:8443/
```

### Why Both Protocols?

#### HTTP/1.1 Support Required For:
- Standard curl commands and testing
- Legacy clients
- Simple health checks
- Load balancer health probes
- Most existing tools and scripts

#### HTTP/2 Benefits:
- **Multiplexing**: Multiple requests over single connection
- **Header Compression**: HPACK compression reduces overhead
- **Server Push**: Proactive resource delivery
- **Binary Protocol**: More efficient than text-based HTTP/1.1
- **Better Performance**: 15-30% higher throughput, 20-40% lower latency

### Testing Both Protocols

#### HTTP/1.1 Test
```bash
curl -v http://localhost:8080/
# Response shows: HTTP/1.1 200 OK
```

#### HTTP/2 Test
```bash
curl --http2-prior-knowledge -v http://localhost:8080/
# Response shows: HTTP/2 200
```

### Common Issues

#### `<BADREQ>` Errors

**Problem**: HAProxy shows `<BADREQ>` in logs

**Cause**: Frontend configured with `proto h2` forces HTTP/2-only

**Fix**: Remove `proto h2` from bind lines

**Before** (HTTP/2 only):
```haproxy
bind :8080 proto h2  # ❌ Rejects HTTP/1.1 clients
```

**After** (Both protocols):
```haproxy
bind :8080           # ✅ Accepts both protocols
option http-use-htx
```

---

## HTTP/2 (H2C) Support

Complete HTTP/2 Cleartext (h2c) implementation for testing and internal service communication.

### What is H2C?

H2C allows HTTP/2 communication over plain TCP connections without TLS/SSL. This is useful for:
- Internal service-to-service communication
- Testing environments
- Backend connections behind a TLS-terminating proxy
- Development without certificate management overhead

### Architecture Flow

```
Client (HTTP/2 or HTTP/1.1)
    ↓
Gateway Frontend (HTTP/2 capable)
    ↓
Gateway Backend Connection (h2c)
    ↓
Backend Server (h2c-enabled)
```

**End-to-End HTTP/2 Flow:**
1. **Client → Gateway**: Client uses HTTP/2 over HTTPS or HTTP/1.1
2. **Gateway → Backend**: Gateway uses h2c (HTTP/2 cleartext) to backends
3. **Backend Response**: Backends respond via h2c
4. **Gateway → Client**: Gateway returns response in client's protocol

### Implementation

#### Backend Servers

Servers support h2c using Go's `golang.org/x/net/http2/h2c` package:

**File:** `test/backend/main.go`

```go
import (
    "golang.org/x/net/http2"
    "golang.org/x/net/http2/h2c"
)

// Create HTTP/2 server with h2c support
h2s := &http2.Server{}

server := &http.Server{
    Addr:    ":9000",
    Handler: h2c.NewHandler(mux, h2s),  // Wraps handler with h2c
}
```

This allows the server to accept both:
- HTTP/1.1 requests (standard)
- HTTP/2 requests without TLS (h2c)

#### Test Clients

**Functional Test Client** (`test/client/cmd/test-client/main.go`):

```go
import "golang.org/x/net/http2"

transport := &http.Transport{
    TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}
// Enable HTTP/2 support
http2.ConfigureTransport(transport)

client := &http.Client{Transport: transport}
```

**Performance Test Client** (`test/client/cmd/perf-client/main.go`):

```go
transport := &http.Transport{
    MaxIdleConns:        concurrency,
    MaxIdleConnsPerHost: concurrency,
    IdleConnTimeout:     90 * time.Second,
}

if useHTTP2 {
    http2.ConfigureTransport(transport)  // Enable HTTP/2
}

client := &http.Client{Transport: transport}
```

### Usage

#### Backend Servers

Backend servers automatically support h2c. No configuration needed:

```bash
# Start backend - supports both HTTP/1.1 and h2c
docker-compose up backend-server-1

# Test with HTTP/1.1
curl http://localhost:9000/test

# Test with h2c
curl --http2-prior-knowledge http://localhost:9000/test
```

#### Performance Testing with HTTP/2

```bash
# HTTP/1.1 baseline
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=50 \
  -n=5000

# HTTP/2 performance
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -http2 \
  -c=50 \
  -n=5000
```

### Gateway Configuration

**HAProxy Example:**

```haproxy
# Frontend - accept HTTP/2 from clients
frontend http-gateway
    bind :443 ssl crt /etc/certs alpn h2,http/1.1
    bind :80
    mode http
    option http-use-htx

# Backend - use h2c to connect to backend servers
backend api-backend
    mode http
    balance roundrobin
    server srv1 backend-server-1:9000 check proto h2
    server srv2 backend-server-2:9000 check proto h2
```

### Verification

#### Check Backend Protocol

```bash
curl -s http://localhost:8080/api/test | jq '.protocol'
# Output: "HTTP/2.0"
```

#### Test Load Balancing with HTTP/2

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

### Dependencies

The h2c support requires:

```go
require golang.org/x/net v0.32.0
```

Automatically included in:
- `test/backend/go.mod`
- `test/client/go.mod`

### HAProxy Configuration Options

#### HTTP/1.1 Only

```haproxy
frontend http-gateway
    bind :8080
    mode http
    # No HTTP/2 support
```

#### HTTP/1.1 and HTTP/2 (Current)

```haproxy
frontend http-gateway
    bind :8080
    mode http
    option http-use-htx
    # Supports both protocols
```

#### HTTP/2 Only

```haproxy
frontend http-gateway
    bind :8080 proto h2
    mode http
    # HTTP/2 only (will reject HTTP/1.1)
```

#### HTTPS with ALPN

```haproxy
frontend https-gateway
    bind :8443 ssl crt /etc/haproxy/certs alpn h2,http/1.1
    mode http
    # ALPN negotiates protocol over TLS
```

---

## IP Auto-Detection

Backend servers automatically detect their IP address from the container's network interface and register with the gateway.

### Why IP-Based Registration?

#### Benefits

1. **HAProxy Compatibility**: HAProxy backends work better with IP addresses for health checks
2. **DNS Independence**: No reliance on container DNS resolution
3. **Network Transparency**: Uses actual container IP from network interface
4. **Flexibility**: Works across Docker, Podman, and Kubernetes

#### Comparison

**Before (Hostname-based)**:
```yaml
environment:
  - SERVER_IP=backend-server-1  # Uses hostname
```

HAProxy config:
```
server backend-server-1 backend-server-1:9000
```

**After (IP-based)**:
```yaml
environment:
  # SERVER_IP is auto-detected from eth0
```

HAProxy config:
```
server backend-server-1 10.88.0.5:9000  # Actual IP
```

### How It Works

#### Detection Logic

The backend entrypoint script detects IP in the following order:

1. **Check environment variable** (if explicitly set)
2. **Try eth0 interface** (most common in containers)
3. **Fallback to hostname -i**
4. **Last resort: use hostname**

#### Code Implementation

**File:** `test/scripts/backend-entrypoint.sh`

```bash
#!/bin/bash
set -e

# Auto-detect IP address if not provided
if [ -z "$SERVER_IP" ]; then
    # Try to get IP from eth0 interface
    SERVER_IP=$(ip addr show eth0 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 || true)

    # Fallback to hostname -i if eth0 not found
    if [ -z "$SERVER_IP" ]; then
        SERVER_IP=$(hostname -i 2>/dev/null | awk '{print $1}' || true)
    fi

    # Last resort: use hostname
    if [ -z "$SERVER_IP" ]; then
        SERVER_IP=$(hostname)
    fi
fi

echo "Detected IP: $SERVER_IP"
```

### Requirements

#### Package Dependencies

The backend Dockerfile includes `iproute2` for the `ip` command:

```dockerfile
FROM golang:1.24-alpine

# Install tools for backend registration and IP detection
RUN apk add --no-cache curl jq bash iproute2
```

#### Network Configuration

- Container must have network interface (typically `eth0`)
- Network driver must assign IP addresses (default behavior)
- Works with Docker bridge, Podman bridge, and Kubernetes pod networks

### Usage Examples

#### Docker Compose

**Automatic detection (recommended)**:
```yaml
backend-server-1:
  environment:
    - SERVER_NAME=backend-server-1
    # SERVER_IP auto-detected from eth0
    - SERVER_PORT=9000
    - BACKEND_NAME=api-backend
    - GATEWAY_URL=http://gateway:9090
```

**Manual override**:
```yaml
backend-server-1:
  environment:
    - SERVER_NAME=backend-server-1
    - SERVER_IP=10.0.1.10  # Explicitly set
    - SERVER_PORT=9000
    - BACKEND_NAME=api-backend
```

#### Kubernetes

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: backend
    env:
    - name: SERVER_NAME
      valueFrom:
        fieldRef:
          fieldPath: metadata.name
    # SERVER_IP auto-detected from pod network
    - name: SERVER_PORT
      value: "9000"
    - name: BACKEND_NAME
      value: "api-backend"
```

### Verification

#### Check Detected IP

View container logs:

```bash
docker logs backend-server-1 | grep "Detected IP"
```

Output:
```
=========================================
Backend Server Starting
=========================================
Server Name: backend-server-1
Detected IP: 10.88.0.5
Server Port: 9000
Backend Name: api-backend
Gateway URL: http://gateway:9090
=========================================
```

#### Query Gateway

```bash
curl http://localhost:9090/api/backends | jq
```

Output:
```json
{
  "success": true,
  "backends": {
    "api-backend": {
      "Name": "api-backend",
      "Servers": [
        {
          "Name": "backend-server-1",
          "IP": "10.88.0.5",
          "Port": 9000
        }
      ]
    }
  }
}
```

### Troubleshooting

#### IP Not Detected

**Problem**: IP shows as hostname

**Check**:
```bash
docker exec backend-server-1 ip addr show eth0
```

**Solution**:
- Ensure container has network interface
- Verify `iproute2` is installed
- Check entrypoint script permissions

#### Wrong IP Detected

**Problem**: Detected IP is not routable

**Check**:
```bash
# View all network interfaces
docker exec backend-server-1 ip addr show

# Check which interface is used
docker exec backend-server-1 hostname -i
```

**Solution**:
- Explicitly set `SERVER_IP` environment variable
- Use correct network interface (not eth0)
- Check Docker/Podman network configuration

#### Multiple IPs

**Problem**: Container has multiple network interfaces

**Check**:
```bash
docker exec backend-server-1 hostname -i
# Output: 10.88.0.5 172.17.0.3
```

**Solution**:
```bash
# Explicitly set the IP to use
docker run -e SERVER_IP=10.88.0.5 ...
```

### Network Topologies

#### Docker Bridge Network

```
┌─────────────────────────────┐
│   Docker Bridge Network     │
│   (10.88.0.0/16)           │
│                             │
│  ┌──────────────────┐      │
│  │  Gateway         │      │
│  │  10.88.0.2:9090  │      │
│  └──────────────────┘      │
│           ↑                 │
│           │ Registration    │
│  ┌──────────────────┐      │
│  │  Backend 1       │      │
│  │  10.88.0.5:9000  │──────┼─→ Auto-detected
│  └──────────────────┘      │
│  ┌──────────────────┐      │
│  │  Backend 2       │      │
│  │  10.88.0.6:9000  │──────┼─→ Auto-detected
│  └──────────────────┘      │
└─────────────────────────────┘
```

### Best Practices

1. **Let it auto-detect**: Don't set `SERVER_IP` unless necessary
2. **Use consistent naming**: Set `SERVER_NAME` to match container/pod name
3. **Verify in logs**: Always check detected IP in container logs
4. **Test connectivity**: Ensure gateway can reach the detected IP
5. **Monitor registration**: Check gateway API for registered backends

---

## Performance Comparison

### HTTP/1.1 vs HTTP/2

| Protocol | Concurrency | Requests/sec | Avg Latency | Notes |
|----------|-------------|--------------|-------------|-------|
| HTTP/1.1 | 50 | 1,000-2,000 | 25-50ms | Standard performance |
| HTTP/2 | 50 | 1,200-2,500 | 20-40ms | 15-25% improvement |
| HTTP/1.1 | 100 | 1,500-3,000 | 30-70ms | Higher load |
| HTTP/2 | 100 | 2,000-4,000 | 25-50ms | Better under load |

### Key Improvements with HTTP/2

- **15-30% higher throughput**
- **20-40% lower latency**
- **Better resource utilization** under high load
- **Reduced connection overhead** through multiplexing
- **Header compression** reduces bandwidth

### Test Scenarios

#### Scenario 1: Basic H2C Communication

```bash
# Start environment
docker-compose up -d

# Test that backend receives HTTP/2
curl -s http://localhost:8080/api/test | jq
```

#### Scenario 2: HTTP/2 Load Test

```bash
# High concurrency HTTP/2 test
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -http2 \
  -c=200 \
  -n=50000
```

#### Scenario 3: Protocol Comparison

```bash
echo "Testing HTTP/1.1..."
docker-compose run --rm test-client /perf-client -c=100 -n=10000 2>&1 | grep "Requests/sec"

echo "Testing HTTP/2..."
docker-compose run --rm test-client /perf-client -http2 -c=100 -n=10000 2>&1 | grep "Requests/sec"
```

### Monitoring

#### Check Protocol in Use

From HAProxy stats:
```bash
echo "show stat" | socat stdio /var/run/haproxy-runtime-api.sock
```

From logs:
```
# HTTP/1.1 request
192.168.1.10:54321 [...] "GET / HTTP/1.1"

# HTTP/2 request
192.168.1.10:54322 [...] "GET / HTTP/2.0"
```

## References

- [RFC 7540 - HTTP/2](https://tools.ietf.org/html/rfc7540)
- [RFC 7541 - HPACK Header Compression](https://tools.ietf.org/html/rfc7541)
- [RFC 7230 - HTTP/1.1](https://tools.ietf.org/html/rfc7230)
- [Go http2 Package](https://pkg.go.dev/golang.org/x/net/http2)
- [Go h2c Package](https://pkg.go.dev/golang.org/x/net/http2/h2c)
- [HAProxy HTTP/2 Documentation](https://www.haproxy.com/documentation/haproxy-configuration-tutorials/http2/)
- [Docker Networking](https://docs.docker.com/network/)
- [Podman Networking](https://docs.podman.io/en/latest/markdown/podman-network.1.html)
