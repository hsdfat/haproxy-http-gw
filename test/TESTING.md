# Testing Guide

## Overview

The HAProxy HTTP Gateway test suite verifies the backend registration architecture, load balancing, and HTTP/2 support.

## Test Architecture

```
┌─────────────┐
│   Gateway   │ ──> Static frontend (haproxy-init.cfg)
│  (Port 8080)│     API Server (Port 9090)
└──────┬──────┘
       │
       │ Backends auto-register on startup
       │
       ├──> backend-server-1 (api-backend)
       ├──> backend-server-2 (api-backend)
       ├──> backend-server-3 (api-backend)
       ├──> web-server-1 (web-backend)
       └──> web-server-2 (web-backend)
```

## Local Testing

### Prerequisites

- **Podman** and **podman-compose** OR **Docker** and **docker-compose**
- **curl** for API testing
- **jq** (optional) for JSON formatting

### Installation

**macOS (Podman)**:
```bash
brew install podman
pip install podman-compose
```

**Linux (Podman)**:
```bash
# Podman is usually pre-installed
pip install podman-compose
```

**Docker**:
```bash
# Install Docker Desktop or Docker Engine
# docker-compose is usually included
```

### Running Tests

```bash
cd test
./run-local-test.sh
```

### Test Steps

The local test script performs the following steps:

1. **Prerequisites Check**: Verifies podman-compose or docker-compose is installed
2. **Certificate Generation**: Creates SSL certificates for HTTPS testing
3. **Build Images**: Builds all container images
4. **Start Services**: Starts the gateway and backend servers
5. **Health Check**: Waits for gateway API to be healthy
6. **Backend Registration**: Waits for backends to auto-register
7. **Functional Tests**:
   - Direct backend access
   - Load balancing across multiple servers
   - Backend registration API
   - HTTP/2 (H2C) support

### Manual Testing

After running the test, you can manually test the system:

```bash
# List registered backends
curl http://localhost:9090/api/backends | jq

# Register a new backend
curl -X POST http://localhost:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "my-backend",
    "servers": [
      {"name": "server1", "ip": "backend-server-1", "port": 9000}
    ]
  }'

# Test load balancing
for i in {1..5}; do
  curl -s http://localhost:8080/ | jq -r '.server'
done

# Add a routing rule
curl -X POST http://localhost:9090/api/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "host": "test.example.com",
    "path": "/api",
    "backend": "my-backend"
  }'

# Test the route
curl -H "Host: test.example.com" http://localhost:8080/api/
```

### Viewing Logs

```bash
# Gateway logs
podman-compose logs -f gateway

# Backend server logs
podman-compose logs -f backend-server-1

# All logs
podman-compose logs -f
```

### Cleanup

```bash
# Stop services
podman-compose down

# Remove all data
podman-compose down -v
```

## GitHub Actions Testing

The GitHub Actions workflow runs automatically on:
- Push to `master`, `main`, or `develop` branches
- Pull requests to `master`, `main`, or `develop` branches
- Manual trigger via `workflow_dispatch`

### Test Workflow

The CI/CD pipeline performs:

1. **Build Phase**:
   - Build all container images
   - Generate SSL certificates

2. **Start Services**:
   - Start gateway
   - Start backend servers (auto-register)

3. **Health Checks**:
   - Gateway API health
   - Backend registration verification

4. **Functional Tests**:
   - Basic functionality tests
   - Load balancing tests
   - HTTP/2 support tests

5. **Performance Tests**:
   - Low concurrency (10 workers, 1000 requests)
   - Medium concurrency (50 workers, 5000 requests)
   - HTTP/2 performance (50 workers, 5000 requests)

6. **Dynamic Backend Tests**:
   - Backend registration via API
   - Backend unregistration via API

### Viewing Results

Test results are available in:
- **GitHub Actions Summary**: View metrics and status
- **Test Artifacts**: Download detailed test results
- **PR Comments**: Automated comment with test summary

## Test Components

### 1. Backend Registration

Backends automatically register on startup using the entrypoint script:

**File**: `test/scripts/backend-entrypoint.sh`

```bash
# Environment variables
BACKEND_NAME=api-backend
SERVER_NAME=backend-server-1
SERVER_IP=backend-server-1
SERVER_PORT=9000
GATEWAY_URL=http://gateway:9090
```

### 2. Registration Script

**File**: `test/scripts/register-backend.sh`

Features:
- Waits for gateway availability
- Retries on failure (30 attempts, 2s delay)
- Validates registration response
- Merges with existing backend servers

### 3. Test Servers

**Backend Servers**: Simple HTTP/2-enabled Go servers that echo request details

**File**: `test/backend/main.go`

Endpoints:
- `GET /health` - Health check
- `GET /*` - Echo server info (server name, timestamp, headers)

## Troubleshooting

### Backends Not Registering

**Symptoms**: Test fails at "Verifying backend registration"

**Check**:
```bash
# View gateway logs
podman-compose logs gateway

# View backend server logs
podman-compose logs backend-server-1

# Check if backend servers are running
podman-compose ps
```

**Common causes**:
- Gateway not ready when backends start
- Network issues between containers
- Environment variables not set correctly

**Fix**:
```bash
# Restart backend servers
podman-compose restart backend-server-1 backend-server-2 backend-server-3

# Manually register a backend
curl -X POST http://localhost:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "api-backend",
    "servers": [
      {"name": "server1", "ip": "backend-server-1", "port": 9000}
    ]
  }'
```

### Load Balancing Not Working

**Symptoms**: All requests go to the same server

**Check**:
```bash
# Test load balancing
for i in {1..10}; do
  curl -s http://localhost:8080/ | jq -r '.server'
done
```

**Common causes**:
- Only one backend server registered
- HAProxy configuration issue

**Fix**:
```bash
# Check registered backends
curl http://localhost:9090/api/backends | jq

# Verify all backend servers are healthy
curl http://localhost:8080/ | jq
```

### H2C Test Fails

**Symptoms**: HTTP/2 test is skipped or fails

**Check**:
```bash
# Test if curl supports HTTP/2
curl --version | grep HTTP2

# Test H2C manually
curl --http2-prior-knowledge -v http://localhost:8080/
```

**Common causes**:
- Curl not compiled with HTTP/2 support
- HAProxy H2C not configured correctly

**Fix**:
- Upgrade curl: `brew upgrade curl` (macOS)
- Check HAProxy config: `haproxy.cfg` should have `proto h2` on bind lines

### Gateway Health Check Fails

**Symptoms**: Gateway API not responding

**Check**:
```bash
# Check if gateway is running
podman-compose ps gateway

# View gateway logs
podman-compose logs gateway

# Try health endpoint
curl -v http://localhost:9090/health
```

**Common causes**:
- Gateway failed to start
- Port conflict
- HAProxy configuration error

**Fix**:
```bash
# Restart gateway
podman-compose restart gateway

# Check for port conflicts
lsof -i :8080
lsof -i :9090

# Rebuild gateway
podman-compose build gateway
podman-compose up -d gateway
```

## Performance Benchmarks

Expected performance metrics (GitHub Actions, Ubuntu runner):

| Test | Concurrency | Requests | Expected RPS | Success Rate |
|------|------------|----------|--------------|--------------|
| Low | 10 workers | 1,000 | ~800-1,000 | ≥99% |
| Medium | 50 workers | 5,000 | ~2,000-3,000 | ≥98% |
| HTTP/2 | 50 workers | 5,000 | ~2,500-4,000 | ≥98% |

**Note**: Performance varies based on hardware and system load.

## Environment Variables

### Gateway

```bash
LOG_LEVEL=debug              # Logging level (debug, info, warn, error)
HAPROXY_RUNTIME_SOCKET=/var/run/haproxy-runtime-api.sock
```

### Backend Servers

```bash
SERVER_NAME=backend-server-1  # Unique server identifier
SERVER_IP=backend-server-1    # Server IP/hostname
SERVER_PORT=9000              # Server port
BACKEND_NAME=api-backend      # Backend group to join
GATEWAY_URL=http://gateway:9090  # Gateway API URL
```

## Adding New Tests

### Local Test

Edit `test/run-local-test.sh`:

```bash
# Test X: Description
print_info "Test X: Testing feature X"
RESPONSE=$(curl -sf http://localhost:8080/feature)
if echo "$RESPONSE" | grep -q "expected"; then
    print_success "Feature X is working"
else
    print_error "Feature X failed"
    exit 1
fi
```

### GitHub Actions

Edit `.github/workflows/gateway-tests.yml`:

```yaml
- name: Test feature X
  id: feature-x-test
  run: |
    cd test
    echo "Testing feature X..."
    # Add test commands
```

## Best Practices

1. **Always check health** before running tests
2. **Wait for backends** to register (15-30 seconds)
3. **Use retries** for flaky network operations
4. **View logs** when tests fail
5. **Clean up** after tests (`podman-compose down -v`)
6. **Use jq** for JSON parsing when available
7. **Test both HTTP/1.1 and HTTP/2** protocols
8. **Verify load balancing** with multiple requests

## Related Documentation

- [Backend Registration Architecture](../BACKEND_REGISTRATION.md)
- [HTTP/2 Support](HTTP2_SUPPORT.md)
- [Quickstart Guide](QUICKSTART.md)
