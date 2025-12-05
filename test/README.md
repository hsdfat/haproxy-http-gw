# HTTP Gateway Test System

Complete test system for the HTTP/2 Gateway with functional and performance testing capabilities.

## Overview

This test system provides:

1. **Docker Compose Environment** - Complete local test infrastructure
2. **Mock Backend Servers** - HTTP/2-enabled backend services
3. **Backend API Provider** - REST API for dynamic backend discovery
4. **Functional Test Client** - Automated functional tests
5. **Performance Test Client** - Load and performance testing

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Test Environment                         │
│                                                              │
│  ┌────────────┐      ┌─────────────────────────────────┐   │
│  │   Test     │      │      HTTP Gateway               │   │
│  │  Client    │─────→│  (HAProxy + Gateway Controller) │   │
│  └────────────┘      └──────────┬──────────────────────┘   │
│                                  │                           │
│                                  ↓                           │
│                      ┌───────────────────────┐              │
│                      │   Backend API         │              │
│                      │  (REST Provider)      │              │
│                      └───────────────────────┘              │
│                                  │                           │
│                                  ↓                           │
│              ┌──────────────────────────────────┐           │
│              │      Backend Servers             │           │
│              │  • backend-server-1 (HTTP/2)     │           │
│              │  • backend-server-2 (HTTP/2)     │           │
│              │  • backend-server-3 (HTTP/2)     │           │
│              │  • web-server-1 (HTTP/2)         │           │
│              │  • web-server-2 (HTTP/2)         │           │
│              └──────────────────────────────────┘           │
└──────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

**Option 1: Using Podman (Recommended)**
- Podman
- podman-compose (install with `pip install podman-compose`)
- OpenSSL (for certificate generation)

**Option 2: Using Docker**
- Docker
- Docker Compose
- OpenSSL (for certificate generation)

### Setup

1. **Generate SSL Certificates**

```bash
cd test
./scripts/generate-certs.sh
```

2. **Start the Test Environment**

**Using Podman (Default):**
```bash
podman-compose up -d
# OR using make
make up
```

**Using Docker:**
```bash
docker-compose up -d
# OR using make
make CONTAINER_RUNTIME=docker up
```

3. **Verify Services are Running**

**Using Podman:**
```bash
podman-compose ps
# OR using make
make CONTAINER_RUNTIME=podman ps
```

**Using Docker:**
```bash
docker-compose ps
```

Expected output:
```
NAME                 STATUS              PORTS
backend-api          Up                  0.0.0.0:8000->8000/tcp
backend-server-1     Up
backend-server-2     Up
backend-server-3     Up
web-server-1         Up
web-server-2         Up
http-gateway         Up                  0.0.0.0:8080->8080/tcp, 0.0.0.0:8443->8443/tcp
```

### Running Tests

#### Option 1: Using Make (Recommended - Works with both Podman and Docker)

```bash
# Run all tests (uses podman by default)
make test

# Run functional tests only
make test-functional

# Run performance tests only
make test-perf

# Quick smoke test
make test-quick

# Use Docker instead of Podman
make CONTAINER_RUNTIME=docker test
```

#### Option 2: Run All Tests in Container

**Using Podman:**
```bash
podman-compose run --rm test-client /usr/local/bin/run-tests.sh
```

**Using Docker:**
```bash
docker-compose run --rm test-client /usr/local/bin/run-tests.sh
```

#### Option 3: Run Individual Tests

**Functional Tests:**

*Using Podman:*
```bash
podman-compose run --rm test-client /test-client -gateway=http://gateway:8080 -verbose
```

*Using Docker:*
```bash
docker-compose run --rm test-client /test-client -gateway=http://gateway:8080 -verbose
```

**Performance Tests:**

*Using Podman:*
```bash
# Quick test (10 workers, 1000 requests)
podman-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=10 -n=1000

# Load test (100 workers, 10000 requests)
podman-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=100 -n=10000

# Duration-based test (20 workers, 30 seconds)
podman-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=20 -d=30s
```

*Using Docker:*
```bash
# Quick test (10 workers, 1000 requests)
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=10 -n=1000

# Load test (100 workers, 10000 requests)
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=100 -n=10000

# Duration-based test (20 workers, 30 seconds)
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=20 -d=30s
```

### Manual Testing

Test from your local machine:

```bash
# HTTP request
curl -H "Host: api.example.com" http://localhost:8080/api/test

# HTTPS request (HTTP/2)
curl -k --http2 -H "Host: api.example.com" https://localhost:8443/api/test

# Check HTTP/2 protocol
curl -k --http2 -v https://localhost:8443/api/test 2>&1 | grep "HTTP/2"

# Test load balancing (multiple requests)
for i in {1..10}; do
  curl -s -H "Host: api.example.com" http://localhost:8080/api/test | jq -r '.server'
done
```

## Components

### 1. HTTP Gateway (Port 8080, 8443)

The main gateway service that:
- Accepts HTTP/HTTPS requests
- Routes based on Host header and path
- Load balances across backend servers
- Supports HTTP/2

**Configuration:**
- HTTP Port: 8080
- HTTPS Port: 8443
- Admin/Stats: 9090
- Backend discovery: REST API (http://backend-api:8000)

### 2. Backend API (Port 8000)

REST API service for dynamic backend discovery.

**Endpoints:**

```bash
# List all backends
curl http://localhost:8000/backends

# Get specific backend
curl http://localhost:8000/backends/api-backend

# Create/update backend
curl -X POST http://localhost:8000/backends -H "Content-Type: application/json" -d '{
  "name": "my-backend",
  "servers": [
    {"name": "srv1", "ip": "10.0.0.1", "port": 8080},
    {"name": "srv2", "ip": "10.0.0.2", "port": 8080}
  ]
}'

# Delete backend
curl -X DELETE http://localhost:8000/backends/my-backend
```

**Default Backends:**

- `api-backend`: backend-server-1, backend-server-2
- `web-backend`: web-server-1, web-server-2

### 3. Backend Servers

Mock HTTP/2-enabled backend servers that:
- Echo request details (method, path, headers, protocol)
- Identify themselves in responses
- Support HTTP/2

**Testing a backend directly:**

```bash
docker exec backend-server-1 curl http://localhost:9000/test
```

### 4. Test Clients

#### Functional Test Client

Automated tests covering:
- Basic HTTP requests
- HTTP/2 support
- Load balancing
- Path routing
- Host-based routing
- Health checks

**Usage:**

```bash
/test-client [options]

Options:
  -gateway string       Gateway HTTP URL (default: http://localhost:8080)
  -gateway-https string Gateway HTTPS URL (default: https://localhost:8443)
  -host string          Host header to use (default: api.example.com)
  -verbose              Enable verbose output
```

**Example:**

```bash
docker-compose run --rm test-client /test-client \
  -gateway=http://gateway:8080 \
  -gateway-https=https://gateway:8443 \
  -host=api.example.com \
  -verbose
```

#### Performance Test Client

Load testing tool with configurable concurrency and request counts.

**Usage:**

```bash
/perf-client [options]

Options:
  -url string     Gateway URL (default: http://localhost:8080)
  -host string    Host header (default: api.example.com)
  -path string    Request path (default: /api/test)
  -c int          Concurrency (number of workers) (default: 10)
  -n int          Total number of requests (default: 1000)
  -d duration     Test duration (overrides -n)
  -http2          Use HTTP/2
```

**Examples:**

```bash
# Basic load test
/perf-client -url=http://gateway:8080 -c=50 -n=10000

# Duration-based test
/perf-client -url=http://gateway:8080 -c=20 -d=1m

# HTTP/2 performance test
/perf-client -url=https://gateway:8443 -http2 -c=100 -n=50000

# Specific path
/perf-client -url=http://gateway:8080 -path=/api/users -c=30 -n=5000
```

**Output Metrics:**
- Total requests
- Success/failure rate
- Requests per second
- Latency statistics (min, max, average)

## Test Scenarios

### Scenario 1: Basic Functional Testing

**Using Make (works with both Podman and Docker):**
```bash
# Setup and start environment
make setup

# Run functional tests
make test-functional

# Check results
# All tests should pass with ✓ PASS status
```

**Using Podman directly:**
```bash
# Start environment
podman-compose up -d

# Run functional tests
podman-compose run --rm test-client /test-client -verbose

# Check results
# All tests should pass with ✓ PASS status
```

**Using Docker directly:**
```bash
# Start environment
docker-compose up -d

# Run functional tests
docker-compose run --rm test-client /test-client -verbose

# Check results
# All tests should pass with ✓ PASS status
```

### Scenario 2: Dynamic Backend Updates

```bash
# Add a new backend server
curl -X POST http://localhost:8000/backends -H "Content-Type: application/json" -d '{
  "name": "dynamic-backend",
  "servers": [
    {"name": "backend-server-3", "ip": "backend-server-3", "port": 9000}
  ]
}'

# Verify backend was added
curl http://localhost:8000/backends

# The gateway should automatically pick up the new backend
# Test requests will now be routed to the new backend
```

### Scenario 3: Load Balancing Verification

```bash
# Make multiple requests and check distribution
for i in {1..20}; do
  curl -s -H "Host: api.example.com" http://localhost:8080/api/test | jq -r '.server'
done | sort | uniq -c

# Expected output shows distribution across servers:
#   10 backend-server-1
#   10 backend-server-2
```

### Scenario 4: HTTP/2 Performance Testing

**Using Podman:**
```bash
# Test HTTP/2 performance
podman-compose run --rm test-client /perf-client \
  -url=https://gateway:8443 \
  -http2 \
  -c=100 \
  -n=50000

# Compare with HTTP/1.1
podman-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=100 \
  -n=50000

# HTTP/2 should show better throughput and lower latency
```

**Using Docker:**
```bash
# Test HTTP/2 performance
docker-compose run --rm test-client /perf-client \
  -url=https://gateway:8443 \
  -http2 \
  -c=100 \
  -n=50000

# Compare with HTTP/1.1
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=100 \
  -n=50000

# HTTP/2 should show better throughput and lower latency
```

### Scenario 5: High Concurrency Testing

**Using Podman:**
```bash
# Stress test with high concurrency
podman-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=200 \
  -d=2m

# Monitor gateway performance
podman stats http-gateway
```

**Using Docker:**
```bash
# Stress test with high concurrency
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=200 \
  -d=2m

# Monitor gateway performance
docker stats http-gateway
```

## Monitoring and Debugging

### View Gateway Logs

**Using Make:**
```bash
make logs
```

**Using Podman:**
```bash
podman-compose logs -f gateway
```

**Using Docker:**
```bash
docker-compose logs -f gateway
```

### View Backend API Logs

**Using Podman:**
```bash
podman-compose logs -f backend-api
```

**Using Docker:**
```bash
docker-compose logs -f backend-api
```

### View Backend Server Logs

**Using Podman:**
```bash
podman-compose logs -f backend-server-1 backend-server-2 backend-server-3
```

**Using Docker:**
```bash
docker-compose logs -f backend-server-1 backend-server-2 backend-server-3
```

### Check HAProxy Stats

**Using Podman:**
```bash
# Via runtime socket
podman exec http-gateway sh -c "echo 'show stat' | socat - /var/run/haproxy-runtime-api.sock"

# Check backends
podman exec http-gateway sh -c "echo 'show backend' | socat - /var/run/haproxy-runtime-api.sock"

# Check servers
podman exec http-gateway sh -c "echo 'show servers state' | socat - /var/run/haproxy-runtime-api.sock"
```

**Using Docker:**
```bash
# Via runtime socket
docker exec http-gateway sh -c "echo 'show stat' | socat - /var/run/haproxy-runtime-api.sock"

# Check backends
docker exec http-gateway sh -c "echo 'show backend' | socat - /var/run/haproxy-runtime-api.sock"

# Check servers
docker exec http-gateway sh -c "echo 'show servers state' | socat - /var/run/haproxy-runtime-api.sock"
```

### Access HAProxy Stats Page

If stats are enabled, access at: http://localhost:9090/stats

## Cleanup

**Using Make (Recommended):**
```bash
# Stop services and remove volumes
make clean

# Complete cleanup and rebuild
make reset
```

**Using Podman:**
```bash
# Stop all services
podman-compose down

# Remove volumes
podman-compose down -v

# Clean up everything including images
podman-compose down -v --rmi all
```

**Using Docker:**
```bash
# Stop all services
docker-compose down

# Remove volumes
docker-compose down -v

# Clean up everything including images
docker-compose down -v --rmi all
```

## Customization

### Adding More Backend Servers

Edit [docker-compose.yml](docker-compose.yml) and add:

```yaml
backend-server-4:
  build:
    context: ..
    dockerfile: test/Dockerfile.backend
  container_name: backend-server-4
  environment:
    - SERVER_NAME=backend-server-4
    - SERVER_PORT=9000
    - ENABLE_HTTP2=true
  networks:
    - gateway-net
```

Then update the backend via API:

```bash
curl -X POST http://localhost:8000/backends -H "Content-Type: application/json" -d '{
  "name": "api-backend",
  "servers": [
    {"name": "backend-server-1", "ip": "backend-server-1", "port": 9000},
    {"name": "backend-server-2", "ip": "backend-server-2", "port": 9000},
    {"name": "backend-server-4", "ip": "backend-server-4", "port": 9000}
  ]
}'
```

### Configuring Gateway Settings

Edit environment variables in [docker-compose.yml](docker-compose.yml):

```yaml
gateway:
  environment:
    - LOG_LEVEL=debug          # trace, debug, info, warning, error
    - BACKEND_API_URL=http://backend-api:8000
    - SYNC_PERIOD=5s           # Backend sync period
```

## Troubleshooting

### Gateway fails to start

**Using Podman:**
```bash
# Check logs
podman-compose logs gateway

# Verify HAProxy is running
podman exec http-gateway ps aux | grep haproxy

# Check runtime socket
podman exec http-gateway ls -la /var/run/haproxy-runtime-api.sock
```

**Using Docker:**
```bash
# Check logs
docker-compose logs gateway

# Verify HAProxy is running
docker exec http-gateway ps aux | grep haproxy

# Check runtime socket
docker exec http-gateway ls -la /var/run/haproxy-runtime-api.sock
```

### Tests fail with connection errors

**Using Podman:**
```bash
# Verify all services are running
podman-compose ps

# Check network connectivity
podman-compose exec test-client ping gateway
podman-compose exec test-client ping backend-api

# Wait longer for services to be ready
podman-compose run --rm test-client sleep 10 && /usr/local/bin/run-tests.sh
```

**Using Docker:**
```bash
# Verify all services are running
docker-compose ps

# Check network connectivity
docker-compose exec test-client ping gateway
docker-compose exec test-client ping backend-api

# Wait longer for services to be ready
docker-compose run --rm test-client sleep 10 && /usr/local/bin/run-tests.sh
```

### Backend API not updating

**Using Podman:**
```bash
# Check backend API logs
podman-compose logs backend-api

# Verify backend data
curl http://localhost:8000/backends | jq

# Restart backend API
podman-compose restart backend-api
```

**Using Docker:**
```bash
# Check backend API logs
docker-compose logs backend-api

# Verify backend data
curl http://localhost:8000/backends | jq

# Restart backend API
docker-compose restart backend-api
```

### Performance tests showing low throughput

**Using Podman:**
```bash
# Monitor resource usage
podman stats

# Reduce concurrency
podman-compose run --rm test-client /perf-client -c=10 -n=1000
```

**Using Docker:**
```bash
# Increase Docker resources (CPU/Memory)
# Check Docker Desktop settings

# Monitor resource usage
docker stats

# Reduce concurrency
docker-compose run --rm test-client /perf-client -c=10 -n=1000
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test Gateway

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Generate certificates
        run: |
          cd test
          ./scripts/generate-certs.sh

      - name: Start test environment
        run: docker-compose -f test/docker-compose.yml up -d

      - name: Wait for services
        run: sleep 30

      - name: Run functional tests
        run: docker-compose -f test/docker-compose.yml run --rm test-client /test-client -verbose

      - name: Run performance tests
        run: docker-compose -f test/docker-compose.yml run --rm test-client /perf-client -c=50 -n=5000

      - name: Cleanup
        if: always()
        run: docker-compose -f test/docker-compose.yml down -v
```

## Performance Benchmarks

Expected performance on a typical development machine (4 CPU, 8GB RAM):

| Test Type | Concurrency | Requests | RPS | Avg Latency | Success Rate |
|-----------|-------------|----------|-----|-------------|--------------|
| Light     | 10          | 1,000    | ~500-1000 | 10-20ms | >99% |
| Medium    | 50          | 5,000    | ~1000-2000 | 20-50ms | >99% |
| Heavy     | 100         | 10,000   | ~1500-3000 | 30-70ms | >98% |
| Sustained | 20          | 30s      | ~500-1500 | 10-40ms | >99% |

HTTP/2 typically shows 10-30% better performance compared to HTTP/1.1 under high load.

## License

Apache License 2.0
