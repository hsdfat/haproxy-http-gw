# HAProxy HTTP Gateway - Complete Feature Documentation

## Table of Contents

1. [Overview](#overview)
2. [Core Features](#core-features)
3. [Protocol Support](#protocol-support)
4. [Backend Management](#backend-management)
5. [Routing Capabilities](#routing-capabilities)
6. [Load Balancing](#load-balancing)
7. [SSL/TLS Support](#ssltls-support)
8. [API Reference](#api-reference)
9. [Configuration Options](#configuration-options)
10. [Monitoring & Metrics](#monitoring--metrics)
11. [High Availability](#high-availability)
12. [Performance Characteristics](#performance-characteristics)
13. [Testing Infrastructure](#testing-infrastructure)
14. [Deployment Modes](#deployment-modes)

---

## Overview

The HAProxy HTTP Gateway is a production-grade, HTTP/HTTP2-enabled reverse proxy and load balancer built on top of HAProxy. It provides both Kubernetes Ingress Controller functionality and standalone gateway capabilities with dynamic backend management.

### Key Highlights

- **Dual-mode operation**: Kubernetes Ingress Controller or Standalone Gateway
- **HTTP/2 Support**: Full HTTP/2 and H2C (HTTP/2 Cleartext) support
- **Dynamic Backend Discovery**: REST API for runtime backend registration
- **High Performance**: Handles thousands of requests per second with sub-50ms latency
- **Zero-downtime Updates**: Graceful configuration reloads
- **Extensive Monitoring**: Prometheus metrics and health check endpoints
- **Production-ready**: Comprehensive testing with CI/CD automation

---

## Core Features

### 1. Multi-Protocol Support

#### HTTP/1.1 and HTTP/2
- Simultaneous HTTP/1.1 and HTTP/2 protocol support
- Automatic protocol negotiation via ALPN (Application-Layer Protocol Negotiation)
- H2C support for HTTP/2 over cleartext connections
- Client-driven protocol selection

#### Transport Layer Security
- TLS 1.2 and TLS 1.3 support
- SNI (Server Name Indication) for virtual hosting
- Certificate management with automatic loading
- Optional strict SNI matching

### 2. Dynamic Configuration

#### Runtime Backend Management
- Register backends via REST API without restart
- Automatic backend discovery and registration
- Health-based backend pool updates
- Zero-downtime configuration changes

#### Hot Reload
- Graceful HAProxy configuration reloads
- Hitless reload with connection preservation
- Master-worker process model for seamless updates

### 3. Traffic Management

#### Advanced Routing
- Path-based routing with regex support
- Host-based routing (virtual hosting)
- Header-based routing
- Query parameter routing
- Custom ACL (Access Control List) rules

#### Load Distribution
- Round-robin load balancing (default)
- Least connections algorithm
- Source IP hash (sticky sessions)
- URI hash for consistent routing
- Weighted load balancing

### 4. Observability

#### Metrics & Monitoring
- Prometheus metrics endpoint (`/metrics`)
- HAProxy stats interface
- Request/response metrics
- Backend health metrics
- Connection pool statistics

#### Health Checks
- Gateway health endpoint (`/healthz`)
- Backend health monitoring
- Configurable health check intervals
- Active and passive health checks

### 5. Security Features

#### Authentication
- Basic HTTP authentication
- JWT token validation support
- Custom authentication backends
- Auth realm configuration

#### Access Control
- IP allowlist/blocklist
- Rate limiting
- Request size limits
- Connection limits per client

---

## Protocol Support

### HTTP/1.1

**Ports:**
- Default: 80 (HTTP)
- Test/Development: 8080 (HTTP)

**Features:**
- Keep-alive connections
- Chunked transfer encoding
- Persistent connections
- HTTP pipelining

**Configuration:**
```
frontend http
  bind :8080
  mode http
  option http-keep-alive
  http-request set-var(txn.base) base
  use_backend %[var(txn.path_match),field(1,.)]
```

### HTTP/2

**Ports:**
- Default: 443 (HTTPS with ALPN)
- Test/Development: 8080 (H2C), 8443 (HTTPS)

**Features:**
- Multiplexed streams over single connection
- Server push capability
- Header compression (HPACK)
- Binary framing protocol
- H2C (HTTP/2 Cleartext) for non-TLS connections

**Performance Benefits:**
- 10-30% faster than HTTP/1.1 in tests
- Reduced latency (10-40ms average)
- Better connection reuse
- Lower resource consumption

**Configuration:**
```
frontend https
  bind :8443 ssl crt /usr/local/etc/haproxy/certs/ alpn h2,http/1.1
  mode http
  option http-use-htx
```

**H2C Configuration (HTTP/2 without TLS):**
```
frontend http
  bind :8080 proto h2
  mode http
  option http-use-htx
```

### ALPN Protocol Negotiation

The gateway supports ALPN for automatic protocol selection:
- Clients advertise supported protocols: `h2`, `http/1.1`
- Server selects best available protocol
- Transparent to application layer

---

## Backend Management

### Static Backend Configuration

Backends can be predefined in the HAProxy configuration:

```haproxy
backend api-backend
  mode http
  balance roundrobin
  server backend-server-1 backend-server-1:9000 check
  server backend-server-2 backend-server-2:9000 check
  server backend-server-3 backend-server-3:9000 check

backend web-backend
  mode http
  balance roundrobin
  server web-server-1 web-server-1:9000 check
  server web-server-2 web-server-2:9000 check
```

### Dynamic Backend Registration

**REST API Endpoints:**

#### Register Backend
```bash
POST /api/backends

Request Body:
{
  "name": "api-backend",
  "servers": [
    {
      "name": "backend-server-1",
      "address": "192.168.1.10",
      "port": 9000
    },
    {
      "name": "backend-server-2",
      "address": "192.168.1.11",
      "port": 9000
    }
  ],
  "balance": "roundrobin",
  "check": true
}

Response: 200 OK
{
  "message": "Backend registered successfully",
  "backend": "api-backend"
}
```

#### List Backends
```bash
GET /api/backends

Response: 200 OK
{
  "backends": [
    {
      "name": "api-backend",
      "servers": [...],
      "status": "active"
    }
  ]
}
```

#### Unregister Backend
```bash
DELETE /api/backends/{name}

Response: 200 OK
{
  "message": "Backend unregistered successfully"
}
```

### Auto-Registration

Backend servers can automatically register themselves on startup:

**Environment Variables:**
```bash
BACKEND_NAME=api-backend
SERVER_NAME=backend-server-1
SERVER_IP=192.168.1.10
SERVER_PORT=9000
GATEWAY_URL=http://gateway:9090
ENABLE_HTTP2=true
```

**Registration Script:**
```bash
#!/bin/bash
curl -X POST ${GATEWAY_URL}/api/backends \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"${BACKEND_NAME}\",
    \"servers\": [{
      \"name\": \"${SERVER_NAME}\",
      \"address\": \"${SERVER_IP}\",
      \"port\": ${SERVER_PORT}
    }]
  }"
```

### Backend Health Monitoring

**Health Check Configuration:**
```haproxy
backend api-backend
  option httpchk GET /health
  http-check expect status 200
  server backend-1 192.168.1.10:9000 check inter 2s fall 3 rise 2
```

**Parameters:**
- `inter`: Health check interval (default: 2s)
- `fall`: Failed checks before marking down (default: 3)
- `rise`: Successful checks before marking up (default: 2)
- `timeout check`: Health check timeout (default: 1s)

---

## Routing Capabilities

### Path-Based Routing

Route traffic based on URL path:

```haproxy
# HAProxy ACL configuration
acl is_api path_beg /api
acl is_web path_beg /web
acl is_static path_beg /static

use_backend api-backend if is_api
use_backend web-backend if is_web
use_backend static-backend if is_static
```

**Dynamic Route Configuration:**
```bash
curl -X POST http://gateway:9090/api/routes \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/api/*",
    "backend": "api-backend",
    "methods": ["GET", "POST", "PUT", "DELETE"]
  }'
```

### Host-Based Routing

Route based on Host header (virtual hosting):

```haproxy
acl host_api hdr(host) -i api.example.com
acl host_web hdr(host) -i web.example.com

use_backend api-backend if host_api
use_backend web-backend if host_web
```

**Example Request:**
```bash
curl -H "Host: api.example.com" http://gateway:8080/users
# Routes to api-backend

curl -H "Host: web.example.com" http://gateway:8080/
# Routes to web-backend
```

### Header-Based Routing

Route based on custom headers:

```haproxy
acl has_api_version hdr(X-API-Version) -m found
acl api_v2 hdr(X-API-Version) -i v2

use_backend api-v2-backend if has_api_version api_v2
use_backend api-v1-backend if has_api_version
```

### Combined Routing Rules

Combine multiple conditions:

```haproxy
acl is_api path_beg /api
acl host_prod hdr(host) -i prod.example.com
acl is_authenticated hdr(Authorization) -m found

use_backend api-prod-backend if is_api host_prod is_authenticated
use_backend api-dev-backend if is_api !host_prod
```

---

## Load Balancing

### Algorithms

#### Round Robin (Default)
```haproxy
backend api-backend
  balance roundrobin
  server srv1 192.168.1.10:9000
  server srv2 192.168.1.11:9000
  server srv3 192.168.1.12:9000
```
**Use case:** Even distribution across identical backends

#### Least Connections
```haproxy
backend api-backend
  balance leastconn
  server srv1 192.168.1.10:9000
  server srv2 192.168.1.11:9000
```
**Use case:** Long-lived connections, variable processing times

#### Source IP Hash (Sticky Sessions)
```haproxy
backend api-backend
  balance source
  hash-type consistent
  server srv1 192.168.1.10:9000
  server srv2 192.168.1.11:9000
```
**Use case:** Session persistence without cookies

#### URI Hash
```haproxy
backend api-backend
  balance uri
  hash-type consistent
  server srv1 192.168.1.10:9000
  server srv2 192.168.1.11:9000
```
**Use case:** Cache optimization, consistent routing per URL

#### Weighted Load Balancing
```haproxy
backend api-backend
  balance roundrobin
  server srv1 192.168.1.10:9000 weight 100
  server srv2 192.168.1.11:9000 weight 50
  server srv3 192.168.1.12:9000 weight 25
```
**Use case:** Heterogeneous backend capacity

### Server Parameters

```haproxy
server backend-1 192.168.1.10:9000 check weight 100 maxconn 500 inter 2s fall 3 rise 2
```

**Parameters:**
- `check`: Enable health checks
- `weight`: Load balancing weight (default: 1)
- `maxconn`: Maximum concurrent connections
- `inter`: Health check interval
- `fall`: Failed checks before down
- `rise`: Successful checks before up
- `backup`: Backup server (only used when primaries fail)
- `disabled`: Server initially disabled

---

## SSL/TLS Support

### Certificate Configuration

**Certificate Directory:**
```
/usr/local/etc/haproxy/certs/
├── domain1.pem
├── domain2.pem
└── wildcard.pem
```

**Frontend Configuration:**
```haproxy
frontend https
  bind :8443 ssl crt /usr/local/etc/haproxy/certs/ alpn h2,http/1.1
  mode http

  # Force HTTPS redirect
  redirect scheme https code 301 if !{ ssl_fc }
```

### Certificate Format

HAProxy requires PEM format with full chain:
```
-----BEGIN PRIVATE KEY-----
[Private Key]
-----END PRIVATE KEY-----
-----BEGIN CERTIFICATE-----
[Certificate]
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
[Intermediate CA]
-----END CERTIFICATE-----
```

### SNI (Server Name Indication)

Multiple certificates for virtual hosting:

```haproxy
frontend https
  bind :8443 ssl crt /usr/local/etc/haproxy/certs/ strict-sni alpn h2,http/1.1

  # Route based on SNI
  acl is_api ssl_fc_sni api.example.com
  acl is_web ssl_fc_sni web.example.com

  use_backend api-backend if is_api
  use_backend web-backend if is_web
```

### TLS Configuration

**Security Settings:**
```haproxy
# Global TLS configuration
global
  ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256
  ssl-default-bind-ciphersuites TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384
  ssl-default-bind-options ssl-min-ver TLSv1.2 no-tls-tickets
```

### Certificate Generation (Testing)

```bash
# Generate self-signed certificate
openssl req -x509 -newkey rsa:4096 \
  -keyout key.pem -out cert.pem \
  -days 365 -nodes \
  -subj "/CN=localhost"

# Combine for HAProxy
cat key.pem cert.pem > localhost.pem
```

---

## API Reference

### Gateway Management API

**Base URL:** `http://gateway:9090/api`

#### Health Check
```
GET /healthz

Response: 200 OK
{
  "status": "healthy",
  "timestamp": "2025-12-07T10:30:00Z",
  "uptime": "24h15m30s"
}
```

#### Backend Management

**Create/Update Backend:**
```
POST /api/backends
Content-Type: application/json

{
  "name": "new-backend",
  "servers": [
    {
      "name": "server1",
      "address": "10.0.0.10",
      "port": 8080,
      "weight": 100,
      "check": true
    }
  ],
  "balance": "roundrobin",
  "options": {
    "httpchk": "GET /health",
    "http-check-expect": "status 200"
  }
}
```

**List All Backends:**
```
GET /api/backends

Response: 200 OK
{
  "backends": [
    {
      "name": "api-backend",
      "mode": "http",
      "balance": "roundrobin",
      "servers": [...],
      "servers_count": 3,
      "active_servers": 3,
      "status": "UP"
    }
  ]
}
```

**Get Backend Details:**
```
GET /api/backends/{name}

Response: 200 OK
{
  "name": "api-backend",
  "servers": [
    {
      "name": "backend-server-1",
      "address": "192.168.1.10",
      "port": 9000,
      "status": "UP",
      "weight": 100,
      "check_status": "L7OK",
      "last_check": "2025-12-07T10:29:58Z"
    }
  ]
}
```

**Delete Backend:**
```
DELETE /api/backends/{name}

Response: 200 OK
{
  "message": "Backend deleted successfully"
}
```

#### Route Management

**Create Route:**
```
POST /api/routes
Content-Type: application/json

{
  "name": "api-route",
  "match": {
    "path": "/api/*",
    "host": "api.example.com",
    "methods": ["GET", "POST"]
  },
  "backend": "api-backend"
}
```

**List Routes:**
```
GET /api/routes

Response: 200 OK
{
  "routes": [...]
}
```

### Statistics API

**HAProxy Stats:**
```
GET /stats
Authorization: Basic <credentials>

Response: HTML stats page
```

**Prometheus Metrics:**
```
GET /metrics

Response: 200 OK
# HELP haproxy_backend_http_responses_total Total HTTP responses
# TYPE haproxy_backend_http_responses_total counter
haproxy_backend_http_responses_total{backend="api-backend",code="2xx"} 15420
...
```

---

## Configuration Options

### Gateway Configuration

**Go Configuration Struct:**
```go
type GatewayConfig struct {
    // Frontend Configuration
    FrontendName string     // Frontend name (default: "http-gateway")
    HTTPPort     int        // HTTP port (default: 80, test: 8080)
    HTTPSPort    int        // HTTPS port (default: 443, test: 8443)
    HTTPSEnabled bool       // Enable HTTPS frontend

    // SSL/TLS Configuration
    SSLCertDir   string     // SSL certificate directory
    StrictSNI    bool       // Enforce SNI matching
    ALPN         string     // ALPN protocols (e.g., "h2,http/1.1")

    // HTTP/2 Configuration
    EnableHTTP2  bool       // Enable HTTP/2 support
    H2CEnabled   bool       // Enable H2C (HTTP/2 cleartext)

    // Backend Configuration
    DefaultBackend string   // Default backend name

    // Network Configuration
    IPv4BindAddr string     // IPv4 bind address (default: 0.0.0.0)
    IPv6BindAddr string     // IPv6 bind address (default: ::)

    // Operational Settings
    MaxConn      int        // Maximum connections
    Timeout      struct {
        Client  time.Duration  // Client timeout
        Server  time.Duration  // Server timeout
        Connect time.Duration  // Connection timeout
    }
}
```

### HAProxy Global Configuration

**File:** `/usr/local/etc/haproxy/haproxy.cfg`

```haproxy
global
    # Process Management
    daemon
    master-worker
    pidfile /var/run/haproxy.pid

    # Performance Tuning
    maxconn 4000
    nbthread 4
    cpu-map auto:1/1-4 0-3

    # Runtime API
    stats socket /var/run/haproxy-runtime-api.sock mode 600 level admin
    stats timeout 2m

    # Logging
    log stdout format raw local0 info

    # SSL/TLS
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256
    ssl-default-bind-options ssl-min-ver TLSv1.2 no-tls-tickets
    tune.ssl.default-dh-param 2048

defaults
    mode http
    log global

    # Timeouts
    timeout connect 5s
    timeout client  50s
    timeout server  50s
    timeout http-request 10s
    timeout http-keep-alive 10s

    # Options
    option httplog
    option dontlognull
    option http-server-close
    option http-keep-alive
    option forwardfor except 127.0.0.0/8

    # Error handling
    errorfile 400 /usr/local/etc/haproxy/errors/400.http
    errorfile 403 /usr/local/etc/haproxy/errors/403.http
    errorfile 408 /usr/local/etc/haproxy/errors/408.http
    errorfile 500 /usr/local/etc/haproxy/errors/500.http
    errorfile 502 /usr/local/etc/haproxy/errors/502.http
    errorfile 503 /usr/local/etc/haproxy/errors/503.http
    errorfile 504 /usr/local/etc/haproxy/errors/504.http
```

### Environment Variables

```bash
# Gateway Configuration
GATEWAY_HTTP_PORT=8080
GATEWAY_HTTPS_PORT=8443
GATEWAY_API_PORT=9090
GATEWAY_ENABLE_HTTP2=true
GATEWAY_DEFAULT_BACKEND=api-backend

# HAProxy Configuration
HAPROXY_CONFIG=/usr/local/etc/haproxy/haproxy.cfg
HAPROXY_RUNTIME_API=/var/run/haproxy-runtime-api.sock
HAPROXY_PIDFILE=/var/run/haproxy.pid

# SSL/TLS Configuration
SSL_CERT_DIR=/usr/local/etc/haproxy/certs
SSL_STRICT_SNI=false

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Performance
MAX_CONNECTIONS=4000
NUM_THREADS=4
```

---

## Monitoring & Metrics

### Prometheus Metrics

**Endpoint:** `http://gateway:9090/metrics`

**Key Metrics:**

#### Request Metrics
```
# Total HTTP requests
haproxy_frontend_http_requests_total{frontend="http"} 125430

# Request rate
haproxy_frontend_http_requests_rate{frontend="http"} 245.6

# Response codes
haproxy_backend_http_responses_total{backend="api-backend",code="2xx"} 120000
haproxy_backend_http_responses_total{backend="api-backend",code="4xx"} 1200
haproxy_backend_http_responses_total{backend="api-backend",code="5xx"} 230
```

#### Connection Metrics
```
# Current connections
haproxy_frontend_current_sessions{frontend="http"} 142

# Maximum connections
haproxy_frontend_max_sessions{frontend="http"} 500

# Connection rate
haproxy_frontend_connections_rate{frontend="http"} 50.2
```

#### Backend Metrics
```
# Backend server status (1=UP, 0=DOWN)
haproxy_backend_server_status{backend="api-backend",server="backend-1"} 1

# Active servers
haproxy_backend_active_servers{backend="api-backend"} 3

# Backend response time (ms)
haproxy_backend_response_time_average_seconds{backend="api-backend"} 0.025

# Queue depth
haproxy_backend_current_queue{backend="api-backend"} 0
```

#### Performance Metrics
```
# Request processing time
haproxy_backend_http_response_time_average{backend="api-backend"} 0.032

# Connection time
haproxy_backend_connect_time_average{backend="api-backend"} 0.002

# Total time
haproxy_backend_total_time_average{backend="api-backend"} 0.034
```

### Health Checks

**Gateway Health:**
```bash
curl http://gateway:9090/healthz

Response:
{
  "status": "healthy",
  "checks": {
    "haproxy": "UP",
    "api": "UP",
    "backends": {
      "api-backend": "UP (3/3 servers)",
      "web-backend": "UP (2/2 servers)"
    }
  }
}
```

**Backend Health:**
```bash
curl http://gateway:9090/api/backends/api-backend/health

Response:
{
  "backend": "api-backend",
  "status": "UP",
  "servers": [
    {
      "name": "backend-1",
      "status": "UP",
      "check_status": "L7OK",
      "check_duration": 2,
      "last_check": "2025-12-07T10:30:15Z"
    }
  ]
}
```

### Logging

**Log Format:**
```
2025-12-07T10:30:15.123Z [INFO] frontend http: 192.168.1.100:54321 -> backend api-backend/backend-1
  GET /api/users HTTP/1.1 200 1234 bytes 25ms
```

**Structured JSON Logging:**
```json
{
  "timestamp": "2025-12-07T10:30:15.123Z",
  "level": "info",
  "frontend": "http",
  "backend": "api-backend",
  "server": "backend-1",
  "client_ip": "192.168.1.100",
  "client_port": 54321,
  "method": "GET",
  "path": "/api/users",
  "status": 200,
  "bytes": 1234,
  "duration_ms": 25,
  "protocol": "HTTP/1.1"
}
```

---

## High Availability

### Master-Worker Process Model

HAProxy runs in master-worker mode for graceful operations:

```
Master Process (PID 1)
├── Worker Process 1 (handles traffic)
├── Worker Process 2 (during reload)
└── Runtime API Socket
```

**Benefits:**
- Zero-downtime reloads
- Graceful configuration updates
- Connection preservation during updates

### Graceful Reload

**Trigger Reload:**
```bash
# Signal master process
kill -USR2 $(cat /var/run/haproxy.pid)

# Or via API
curl -X POST http://gateway:9090/api/reload
```

**Reload Process:**
1. Master reads new configuration
2. Spawns new worker with new config
3. New worker starts accepting connections
4. Old worker stops accepting new connections
5. Old worker maintains existing connections
6. Old worker exits when connections drain

### Connection Draining

```haproxy
# Soft-stop timeout
global
    hard-stop-after 30s

# Server drain on maintenance
server backend-1 192.168.1.10:9000 check
# To drain: echo "set server api-backend/backend-1 state maint" | socat stdio /var/run/haproxy-runtime-api.sock
```

### Multi-Instance Deployment

**Load Balancer (L4):**
```
         ┌──────────────┐
         │ L4 Load Bal  │
         └──────┬───────┘
                │
        ┌───────┼───────┐
        │       │       │
    ┌───▼───┐ ┌▼─────┐ ┌▼─────┐
    │GW-1   │ │GW-2  │ │GW-3  │
    └───┬───┘ └┬─────┘ └┬─────┘
        │      │        │
        └──────┴────┬───┘
                    │
            ┌───────▼────────┐
            │  Backend Pool  │
            └────────────────┘
```

**Keep configuration synchronized:**
- Shared configuration repository
- Config management (Consul, etcd)
- Automated deployment

---

## Performance Characteristics

### Benchmark Results

**Test Environment:**
- Gateway: HAProxy 2.8+
- Backend: 3 servers, Go HTTP/2 server
- Client: 50-100 concurrent connections
- Network: Local network, <1ms latency

#### HTTP/1.1 Performance

| Concurrency | Requests | RPS    | Avg Latency | P95 Latency | Success Rate |
|-------------|----------|--------|-------------|-------------|--------------|
| 10          | 1,000    | 800    | 12ms        | 18ms        | 99.9%        |
| 50          | 5,000    | 1,500  | 32ms        | 48ms        | 99.5%        |
| 100         | 10,000   | 2,000  | 48ms        | 72ms        | 98.8%        |

#### HTTP/2 (H2C) Performance

| Concurrency | Requests | RPS    | Avg Latency | P95 Latency | Success Rate |
|-------------|----------|--------|-------------|-------------|--------------|
| 10          | 1,000    | 950    | 10ms        | 15ms        | 99.9%        |
| 50          | 5,000    | 2,000  | 24ms        | 38ms        | 99.7%        |
| 100         | 10,000   | 2,800  | 34ms        | 58ms        | 99.2%        |

**HTTP/2 Benefits:**
- 15-30% higher throughput
- 20-30% lower latency
- Better connection reuse
- Reduced overhead from multiplexing

### Resource Usage

**Typical Production Load (1000 RPS):**
```
CPU Usage:     25-35% (4 cores)
Memory:        150-200 MB
Connections:   500-800 concurrent
Network I/O:   50-80 Mbps
```

**Peak Load (5000 RPS):**
```
CPU Usage:     60-75% (4 cores)
Memory:        300-400 MB
Connections:   2000-3000 concurrent
Network I/O:   200-300 Mbps
```

### Tuning Recommendations

**For High Throughput:**
```haproxy
global
    maxconn 10000
    nbthread 8
    tune.bufsize 32768
    tune.maxrewrite 8192

defaults
    timeout connect 3s
    timeout client  30s
    timeout server  30s
```

**For Low Latency:**
```haproxy
global
    tune.bufsize 16384
    nbthread 4

defaults
    timeout connect 1s
    timeout client  10s
    timeout server  10s
    option http-server-close
```

**For HTTP/2 Optimization:**
```haproxy
global
    tune.h2.initial-window-size 65535
    tune.h2.max-concurrent-streams 100

frontend https
    option http-use-htx
```

---

## Testing Infrastructure

### Automated Testing

**GitHub Actions CI/CD:**
- Runs on every push and pull request
- Complete integration test suite
- Performance benchmarking
- Automated reporting

**Test Phases:**
1. **Setup:** Build images, start services, generate certificates
2. **Health Checks:** Verify all services running
3. **Functional Tests:** HTTP/1.1, HTTP/2, routing, load balancing
4. **Performance Tests:** Low, medium, high concurrency scenarios
5. **Dynamic Tests:** Backend registration/unregistration

### Test Clients

#### Functional Test Client

**Location:** `test/client/cmd/test-client/`

**Features:**
- HTTP/1.1 and HTTP/2 (H2C) support
- Path and host-based routing tests
- Load balancing verification
- Health check validation

**Usage:**
```bash
# Basic HTTP test
./test-client -gateway http://gateway:8080

# HTTP/2 test
./test-client -gateway http://gateway:8080 -http2

# Custom host header
./test-client -gateway http://gateway:8080 -host api.example.com

# Verbose output
./test-client -gateway http://gateway:8080 -verbose
```

#### Performance Test Client

**Location:** `test/client/cmd/perf-client/`

**Features:**
- Configurable concurrency and duration
- RPS measurement
- Latency statistics (min, max, avg, p50, p95, p99)
- HTTP/2 support

**Usage:**
```bash
# 50 concurrent, 5000 requests
./perf-client -url http://gateway:8080 -c 50 -n 5000

# 100 concurrent, 60 second duration
./perf-client -url http://gateway:8080 -c 100 -d 60s

# HTTP/2 test
./perf-client -url http://gateway:8080 -c 50 -n 5000 -http2

# Custom path and host
./perf-client -url http://gateway:8080 -path /api/users -host api.example.com
```

**Output:**
```
Performance Test Results:
========================
Total Requests:    5000
Concurrent:        50
Duration:          3.25s
Requests/sec:      1538.46

Latency Statistics:
  Min:      8ms
  Max:      156ms
  Average:  32ms
  P50:      28ms
  P95:      52ms
  P99:      89ms

Success Rate:      99.8%
Failed Requests:   10
```

### Local Testing

**Quick Start:**
```bash
cd test

# Setup and start services
make setup
make up

# Run tests
make test           # All tests
make test-functional # Functional only
make test-perf      # Performance only
make test-quick     # Quick smoke test

# View logs
make logs

# Cleanup
make clean
```

**Manual Testing:**
```bash
# Test HTTP/1.1
curl http://localhost:8080/api/test

# Test with custom host
curl -H "Host: api.example.com" http://localhost:8080/

# Test HTTP/2 (H2C)
curl --http2-prior-knowledge http://localhost:8080/api/test

# Test HTTPS with HTTP/2
curl --http2 -k https://localhost:8443/api/test

# Check backend registration
curl http://localhost:9090/api/backends

# Register new backend
curl -X POST http://localhost:9090/api/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-backend",
    "servers": [{"name": "srv1", "address": "10.0.0.1", "port": 8080}]
  }'
```

---

## Deployment Modes

### 1. Kubernetes Ingress Controller

**Deployment:**
```bash
kubectl apply -f deploy/haproxy-ingress.yaml
```

**Ingress Resource:**
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-ingress
  annotations:
    haproxy.org/load-balance: "roundrobin"
    haproxy.org/check: "enabled"
    haproxy.org/check-interval: "2s"
spec:
  ingressClassName: haproxy
  rules:
  - host: api.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: api-service
            port:
              number: 8080
```

**Features:**
- Automatic Ingress resource watching
- Service discovery
- 100+ annotations for customization
- Custom resource support (Global, Defaults, Backend)
- Gateway API support

### 2. Standalone Gateway

**Docker Compose:**
```yaml
version: '3.8'

services:
  gateway:
    image: haproxy-http-gateway:latest
    ports:
      - "8080:8080"   # HTTP
      - "8443:8443"   # HTTPS
      - "9090:9090"   # API/Stats
    environment:
      - GATEWAY_ENABLE_HTTP2=true
      - GATEWAY_DEFAULT_BACKEND=api-backend
    volumes:
      - ./certs:/usr/local/etc/haproxy/certs
      - ./haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg
    networks:
      - gateway-net

  backend:
    image: backend-server:latest
    environment:
      - GATEWAY_URL=http://gateway:9090
      - BACKEND_NAME=api-backend
      - SERVER_NAME=backend-1
      - ENABLE_HTTP2=true
    networks:
      - gateway-net

networks:
  gateway-net:
    driver: bridge
```

**Features:**
- REST API for backend management
- No Kubernetes dependencies
- Self-contained deployment
- Auto-registration support

### 3. Bare Metal / VM Deployment

**Installation:**
```bash
# Install HAProxy
apt-get install haproxy

# Copy gateway binary
cp gateway /usr/local/bin/

# Copy configuration
cp haproxy.cfg /etc/haproxy/

# Start services
systemctl enable haproxy
systemctl start haproxy

systemctl enable gateway-api
systemctl start gateway-api
```

**SystemD Unit (gateway-api):**
```ini
[Unit]
Description=HAProxy Gateway API
After=network.target haproxy.service
Requires=haproxy.service

[Service]
Type=simple
ExecStart=/usr/local/bin/gateway -config /etc/gateway/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## Best Practices

### Security

1. **Use TLS in Production:**
   ```haproxy
   frontend https
     bind :443 ssl crt /etc/haproxy/certs/ alpn h2,http/1.1
     redirect scheme https code 301 if !{ ssl_fc }
   ```

2. **Enable Rate Limiting:**
   ```haproxy
   frontend http
     stick-table type ip size 100k expire 30s store http_req_rate(10s)
     http-request track-sc0 src
     http-request deny if { sc_http_req_rate(0) gt 100 }
   ```

3. **Restrict API Access:**
   ```haproxy
   frontend api
     acl internal_network src 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16
     http-request deny unless internal_network
   ```

### Performance

1. **Optimize Connection Handling:**
   ```haproxy
   defaults
     option http-keep-alive
     option http-server-close
     timeout http-keep-alive 10s
   ```

2. **Enable Compression:**
   ```haproxy
   backend api-backend
     compression algo gzip
     compression type text/html text/plain text/css application/json
   ```

3. **Use Connection Pooling:**
   ```haproxy
   server backend-1 192.168.1.10:9000 maxconn 500 check
   ```

### Reliability

1. **Configure Health Checks:**
   ```haproxy
   backend api-backend
     option httpchk GET /health
     http-check expect status 200
     server srv1 192.168.1.10:9000 check inter 2s fall 3 rise 2
   ```

2. **Set Appropriate Timeouts:**
   ```haproxy
   defaults
     timeout connect 5s
     timeout client  30s
     timeout server  30s
   ```

3. **Enable Logging:**
   ```haproxy
   global
     log stdout format raw local0 info

   defaults
     log global
     option httplog
   ```

### Monitoring

1. **Enable Prometheus Metrics:**
   ```yaml
   - Port 9090 exposes /metrics endpoint
   - Configure Prometheus scraping
   - Set up Grafana dashboards
   ```

2. **Set Up Alerts:**
   ```yaml
   - Backend server down alerts
   - High error rate alerts
   - Latency threshold alerts
   - Connection limit alerts
   ```

3. **Regular Health Checks:**
   ```bash
   # Automated health monitoring
   */1 * * * * curl -f http://gateway:9090/healthz || alert
   ```

---

## Troubleshooting

### Common Issues

**1. Backend servers not receiving traffic:**
```bash
# Check backend registration
curl http://gateway:9090/api/backends

# Check server status
echo "show servers state" | socat stdio /var/run/haproxy-runtime-api.sock

# View logs
docker-compose logs gateway
```

**2. HTTP/2 not working:**
```bash
# Verify ALPN support
openssl s_client -connect gateway:8443 -alpn h2,http/1.1

# Check H2C configuration
grep "proto h2" /etc/haproxy/haproxy.cfg

# Test with curl
curl --http2-prior-knowledge http://gateway:8080/
```

**3. High latency:**
```bash
# Check backend health
curl http://gateway:9090/api/backends/api-backend/health

# View HAProxy stats
curl http://gateway:9090/stats

# Check connection limits
echo "show info" | socat stdio /var/run/haproxy-runtime-api.sock | grep -i conn
```

**4. Configuration reload failures:**
```bash
# Validate configuration
haproxy -c -f /etc/haproxy/haproxy.cfg

# Check master process
ps aux | grep haproxy

# View reload logs
journalctl -u haproxy -f
```

### Debug Mode

**Enable Debug Logging:**
```haproxy
global
    log stdout format raw local0 debug

defaults
    option httplog
    log global
```

**Runtime Commands:**
```bash
# Show current config
echo "show config" | socat stdio /var/run/haproxy-runtime-api.sock

# Show backend status
echo "show stat" | socat stdio /var/run/haproxy-runtime-api.sock

# Show current sessions
echo "show sess" | socat stdio /var/run/haproxy-runtime-api.sock

# Disable server
echo "disable server api-backend/backend-1" | socat stdio /var/run/haproxy-runtime-api.sock
```

---

## Documentation References

- **Main Documentation:** [documentation/README.md](documentation/README.md)
- **Testing Guide:** [test/README.md](test/README.md)
- **Gateway Implementation:** [GATEWAY_IMPLEMENTATION.md](GATEWAY_IMPLEMENTATION.md)
- **Backend Registration:** [BACKEND_REGISTRATION.md](BACKEND_REGISTRATION.md)
- **Controller Arguments:** [documentation/controller.md](documentation/controller.md)
- **Annotations Reference:** [documentation/annotations.md](documentation/annotations.md)
- **Protocol Support:** [test/PROTOCOL_SUPPORT.md](test/PROTOCOL_SUPPORT.md)

---

## Support and Contributing

### Getting Help

- **GitHub Issues:** https://github.com/haproxytech/kubernetes-ingress/issues
- **Community Slack:** #ingress-controller channel in [HAProxy Community Slack](https://slack.haproxy.org)
- **Documentation:** https://www.haproxy.com/documentation/kubernetes/latest/

### Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Follow HAProxy coding standards
4. Run `golangci-lint run` for linting
5. Add tests for new features
6. Submit a pull request

### License

Apache License 2.0 - See [LICENSE](LICENSE) file for details

---

*Last Updated: December 2025*
*Gateway Version: 1.0+ (based on HAProxy Kubernetes Ingress Controller)*
