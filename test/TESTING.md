# Testing Guide

Comprehensive testing guide for the HAProxy HTTP Gateway test system, including local testing, CI/CD integration, and troubleshooting.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Local Testing](#local-testing)
3. [GitHub Actions CI/CD](#github-actions-cicd)
4. [Manual Testing](#manual-testing)
5. [Performance Testing](#performance-testing)
6. [Troubleshooting](#troubleshooting)
7. [Best Practices](#best-practices)

---

## Quick Start

### Simple Test (Recommended for First-Time Users)

```bash
cd test
./run-local-test.sh
```

**What it does:**
- ✅ Checks prerequisites
- ✅ Generates SSL certificates
- ✅ Builds all images
- ✅ Starts services
- ✅ Waits for backends to register
- ✅ Runs basic functional tests

**Expected output:**
```
✓ All functional tests passed!
```

### Full Test Suite (Mirrors GitHub Actions)

```bash
cd test
./run-github-action-tests.sh
```

**What it does:**
1. Functional tests (basic functionality)
2. Performance test - Low concurrency (10 workers, 1000 requests)
3. Performance test - Medium concurrency (50 workers, 5000 requests)
4. HTTP/2 performance test (50 workers, 5000 requests)
5. Dynamic backend tests (register/unregister)

---

## Local Testing

### Prerequisites

**macOS:**
```bash
brew install jq bc podman
pip3 install podman-compose
```

**Ubuntu/Debian:**
```bash
sudo apt-get install jq bc
pip3 install podman-compose
```

**Docker Alternative:**
```bash
# Install Docker Desktop (includes docker-compose)
# Works on macOS, Windows, Linux
```

### Using Make Commands

```bash
# First-time setup
make setup

# Run all tests
make test

# Run specific tests
make test-functional   # Functional tests only
make test-perf         # Performance tests only
make test-quick        # Quick smoke test

# View logs
make logs

# Cleanup
make clean             # Stop services
make reset             # Complete reset and rebuild
```

### Using Scripts Directly

#### Simple Local Test

```bash
cd test
./run-local-test.sh
```

**Test Coverage:**
- Direct backend access
- Load balancing across multiple servers
- Backend registration API
- HTTP/2 (H2C) support

#### Full GitHub Actions Test

```bash
cd test
./run-github-action-tests.sh
```

**Test Coverage:**
- All functional tests
- Performance tests (3 concurrency levels)
- HTTP/2 performance
- Dynamic backend management

### Container Runtime Options

**Using Podman (Default):**
```bash
make test
# or
podman-compose up -d
podman-compose run --rm test-client /test-client -verbose
```

**Using Docker:**
```bash
make CONTAINER_RUNTIME=docker test
# or
docker-compose up -d
docker-compose run --rm test-client /test-client -verbose
```

---

## GitHub Actions CI/CD

### When Tests Run

The GitHub Actions workflow (`.github/workflows/gateway-tests.yml`) runs automatically on:
- Push to `master`, `main`, or `develop` branches
- Pull requests to `master`, `main`, or `develop` branches
- Manual trigger via `workflow_dispatch`

### Test Workflow

**Phases:**

1. **Build Phase**
   - Build all container images
   - Generate SSL certificates

2. **Start Services**
   - Start gateway
   - Start backend servers (auto-register)

3. **Health Checks**
   - Gateway API health verification
   - Backend registration verification

4. **Functional Tests**
   - Basic functionality tests
   - Load balancing tests
   - HTTP/2 support tests

5. **Performance Tests**
   - Low concurrency (10 workers, 1000 requests)
   - Medium concurrency (50 workers, 5000 requests)
   - HTTP/2 performance (50 workers, 5000 requests)

6. **Dynamic Backend Tests**
   - Backend registration via API
   - Backend unregistration via API

### Success Criteria

| Test Type | Success Rate | Notes |
|-----------|-------------|-------|
| Functional Tests | 100% pass | All 6 tests must pass |
| Performance - Low | ≥99% | 10 workers, 1000 requests |
| Performance - Medium | ≥98% | 50 workers, 5000 requests |
| Performance - HTTP/2 | ≥98% | 50 workers, 5000 requests |
| Dynamic Backends | 100% pass | Register/unregister operations |

### Viewing Results

**GitHub Actions Summary:**
- View metrics and status
- Test duration and success rate
- RPS (requests per second) metrics

**Test Artifacts:**
- `functional-results.txt` - Functional test output
- `perf-low-results.txt` - Low concurrency performance
- `perf-medium-results.txt` - Medium concurrency performance
- `perf-http2-results.txt` - HTTP/2 performance

**PR Comments:**
Automated comment with test summary on pull requests

### Running Locally Before Push

Test your changes locally to avoid CI/CD failures:

```bash
cd test
./run-github-action-tests.sh
```

Fix any failures before pushing to GitHub.

---

## Manual Testing

### Development Workflow

#### 1. Start Services

```bash
cd test
podman-compose up -d
```

Wait 15-30 seconds for services to initialize.

#### 2. Verify Health

```bash
# Gateway health
curl http://localhost:9090/health

# Expected: {"status":"healthy"}

# Registered backends
curl http://localhost:9090/api/backends | jq

# Expected: Lists api-backend and web-backend
```

#### 3. Configure Routes

```bash
./scripts/configure-routes.sh
```

Adds:
- `api.example.com/api` → `api-backend`
- `www.example.com/` → `web-backend`

#### 4. Run Tests

**Basic HTTP test:**
```bash
curl http://localhost:8080/
```

**With Host header:**
```bash
curl -H "Host: api.example.com" http://localhost:8080/api/test
```

**Load balancing test:**
```bash
for i in {1..10}; do
  curl -s http://localhost:8080/ | jq -r '.server'
done
```

Expected: Requests distributed across backend-server-1 and backend-server-2

**HTTP/2 test (H2C):**
```bash
curl --http2-prior-knowledge http://localhost:8080/
```

**HTTP/2 test (HTTPS):**
```bash
curl -k --http2 https://localhost:8443/
```

#### 5. View Logs

```bash
# All logs
podman-compose logs -f

# Gateway logs
podman-compose logs -f gateway

# Backend logs
podman-compose logs -f backend-server-1

# Backend API logs
podman-compose logs -f backend-api
```

#### 6. Debug HAProxy

```bash
# HAProxy stats
echo "show stat" | podman exec -i http-gateway socat - /var/run/haproxy-runtime-api.sock

# Backend status
echo "show servers state" | podman exec -i http-gateway socat - /var/run/haproxy-runtime-api.sock

# Current config
echo "show config" | podman exec -i http-gateway socat - /var/run/haproxy-runtime-api.sock
```

#### 7. Cleanup

```bash
# Stop services
podman-compose down

# Stop and remove volumes
podman-compose down -v
```

### Backend Registration Testing

**List backends:**
```bash
curl http://localhost:9090/api/backends | jq
```

**Register new backend:**
```bash
curl -X POST http://localhost:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "my-backend",
    "servers": [
      {"name": "server1", "ip": "backend-server-1", "port": 9000}
    ]
  }'
```

**Delete backend:**
```bash
curl -X DELETE http://localhost:9090/api/backends/my-backend
```

**Add routing rule:**
```bash
curl -X POST http://localhost:9090/api/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "host": "test.example.com",
    "path": "/api",
    "backend": "my-backend"
  }'
```

**Test the route:**
```bash
curl -H "Host: test.example.com" http://localhost:8080/api/
```

---

## Performance Testing

### Using Test Clients

#### Functional Test Client

```bash
docker-compose run --rm test-client /test-client \
  -gateway=http://gateway:8080 \
  -gateway-https=https://gateway:8443 \
  -host=api.example.com \
  -verbose
```

**Tests:**
- Basic HTTP requests
- HTTP/2 support
- Load balancing
- Path/host routing
- Health checks

#### Performance Test Client

**Basic load test:**
```bash
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=50 \
  -n=5000
```

**Duration-based test:**
```bash
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=20 \
  -d=1m
```

**HTTP/2 performance:**
```bash
docker-compose run --rm test-client /perf-client \
  -url=https://gateway:8443 \
  -http2 \
  -c=50 \
  -n=5000
```

**Specific path:**
```bash
docker-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -path=/api/users \
  -c=30 \
  -n=5000
```

### Performance Benchmarks

Expected performance on typical hardware:

| Environment | Concurrency | Requests | RPS | Avg Latency | Success Rate |
|------------|-------------|----------|-----|-------------|--------------|
| GitHub Actions | 10 | 1,000 | 800-1,000 | 10-20ms | >99% |
| GitHub Actions | 50 | 5,000 | 2,000-3,000 | 20-50ms | >98% |
| GitHub Actions HTTP/2 | 50 | 5,000 | 2,500-4,000 | 20-40ms | >98% |
| MacBook Pro M1 | 50 | 5,000 | 4,000-6,000 | 10-25ms | >99% |
| MacBook Pro M1 HTTP/2 | 50 | 5,000 | 5,000-8,000 | 8-20ms | >99% |

**Note:** Performance varies based on:
- CPU cores and speed
- Available memory
- System load
- Container runtime (Podman vs Docker)

### Monitoring Performance

```bash
# Monitor resource usage
podman stats

# Monitor specific service
podman stats http-gateway

# View gateway logs
podman-compose logs -f gateway
```

---

## Troubleshooting

### Services Not Starting

**Symptoms:** Services fail to start or crash

**Diagnostics:**
```bash
# Check Docker/Podman resources
podman info  # or docker info

# View logs
podman-compose logs

# Check service status
podman-compose ps
```

**Solutions:**
```bash
# Reset everything
make reset

# Check for port conflicts
lsof -i :8080
lsof -i :8443
lsof -i :9090
lsof -i :8000

# Increase Docker Desktop resources (if using Docker)
# Docker Desktop → Settings → Resources
```

### Backends Not Registering

**Symptoms:** Test fails at "Verifying backend registration"

**Diagnostics:**
```bash
# View gateway logs
podman-compose logs gateway

# View backend server logs
podman-compose logs backend-server-1

# Check if backend servers are running
podman-compose ps

# Check registered backends
curl http://localhost:9090/api/backends | jq
```

**Common Causes:**
- Gateway not ready when backends start
- Network issues between containers
- Environment variables not set correctly

**Solutions:**
```bash
# Restart backend servers
podman-compose restart backend-server-1 backend-server-2 backend-server-3

# Manually register a backend
curl -X POST http://localhost:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "api-backend",
    "servers": [{"name": "server1", "ip": "backend-server-1", "port": 9000}]
  }'

# Wait longer before running tests
sleep 30 && make test
```

### Load Balancing Not Working

**Symptoms:** All requests go to the same server

**Diagnostics:**
```bash
# Test distribution
for i in {1..20}; do
  curl -s http://localhost:8080/ | jq -r '.server'
done | sort | uniq -c

# Should show even distribution:
#   10 backend-server-1
#   10 backend-server-2

# Check registered backends
curl http://localhost:9090/api/backends | jq
```

**Common Causes:**
- Only one backend server registered
- HAProxy configuration issue

**Solutions:**
```bash
# Check backend count
curl http://localhost:9090/api/backends | jq '.backends["api-backend"].Servers | length'

# Verify all backend servers are healthy
curl http://localhost:8080/ | jq

# Restart services
make reset && make setup
```

### HTTP/2 Tests Fail

**Symptoms:** HTTP/2 test is skipped or fails

**Diagnostics:**
```bash
# Test if curl supports HTTP/2
curl --version | grep HTTP2

# Test H2C manually
curl --http2-prior-knowledge -v http://localhost:8080/

# Test HTTPS HTTP/2
curl -k --http2 -v https://localhost:8443/ 2>&1 | grep "HTTP/2"

# View backend logs for protocol
curl -s http://localhost:8080/ | jq '.protocol'
# Should show "HTTP/2.0" or "HTTP/1.1"
```

**Common Causes:**
- Curl not compiled with HTTP/2 support
- HAProxy H2C not configured correctly

**Solutions:**
```bash
# Upgrade curl
brew upgrade curl  # macOS
sudo apt-get install curl  # Ubuntu/Debian

# Check HAProxy config
podman exec http-gateway cat /etc/haproxy/haproxy.cfg | grep "option http-use-htx"

# Rebuild gateway
make reset && make setup
```

### Gateway Health Check Fails

**Symptoms:** Gateway API not responding

**Diagnostics:**
```bash
# Check if gateway is running
podman-compose ps gateway

# View gateway logs
podman-compose logs gateway

# Try health endpoint
curl -v http://localhost:9090/health

# Check for port conflicts
lsof -i :9090
```

**Common Causes:**
- Gateway failed to start
- Port conflict
- HAProxy configuration error

**Solutions:**
```bash
# Restart gateway
podman-compose restart gateway

# Rebuild gateway
podman-compose build gateway
podman-compose up -d gateway

# Check HAProxy process
podman exec http-gateway ps aux | grep haproxy

# Verify runtime socket
podman exec http-gateway ls -la /var/run/haproxy-runtime-api.sock
```

### Performance Tests Failing

**Symptoms:** Success rate < 98%

**Diagnostics:**
```bash
# Check system resources
top

# Monitor container stats
podman stats

# View gateway logs for errors
podman-compose logs gateway | grep -i error
```

**Common Causes:**
- System under heavy load
- Not enough memory
- Too many concurrent operations

**Solutions:**
```bash
# Reduce concurrency
podman-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=5 \
  -n=100

# Increase Docker/Podman resources

# Close other applications

# Run tests during off-peak hours
```

### Test Client Not Found

**Symptoms:** `missing services [test-client]`

**Cause:** Test client service uses a profile

**Solution:**
```bash
# Use --profile testing
podman-compose --profile testing run --rm test-client /test-client \
  -gateway=http://gateway:8080 \
  -verbose

# Or use make commands
make test
```

---

## Best Practices

### Before Testing

1. **Always check health** before running tests
2. **Wait for backends** to register (15-30 seconds)
3. **Configure routes** if testing host-based routing
4. **Check system resources** are sufficient

### During Testing

1. **Use retries** for flaky network operations
2. **View logs** when tests fail
3. **Monitor resources** during performance tests
4. **Test both HTTP/1.1 and HTTP/2** protocols

### After Testing

1. **Clean up** after tests (`podman-compose down -v`)
2. **Review results** in test artifact files
3. **Fix failures** before pushing to GitHub
4. **Document** any new test scenarios

### General Practices

1. **Use jq** for JSON parsing when available
2. **Verify load balancing** with multiple requests
3. **Test edge cases** (errors, timeouts, failures)
4. **Keep services updated** (`make reset` periodically)
5. **Run full suite** before major changes

## Environment Variables

### Gateway
```bash
LOG_LEVEL=debug                           # Logging level (trace, debug, info, warning, error)
HAPROXY_RUNTIME_SOCKET=/var/run/haproxy-runtime-api.sock
```

### Backend Servers
```bash
SERVER_NAME=backend-server-1              # Unique server identifier
SERVER_IP=                                # Auto-detected from eth0 (or set explicitly)
SERVER_PORT=9000                          # Server port
BACKEND_NAME=api-backend                  # Backend group to join
GATEWAY_URL=http://gateway:9090           # Gateway API URL
ENABLE_HTTP2=true                         # Enable HTTP/2
```

## Adding New Tests

### Local Test Script

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

### GitHub Actions Workflow

Edit `.github/workflows/gateway-tests.yml`:

```yaml
- name: Test feature X
  id: feature-x-test
  run: |
    cd test
    echo "Testing feature X..."
    # Add test commands
```

## Related Documentation

- [README.md](README.md) - Main test system documentation
- [FEATURES.md](FEATURES.md) - HTTP/2 support and IP auto-detection
- [client/README.md](client/README.md) - Test client documentation
- [../GATEWAY_FEATURES.md](../GATEWAY_FEATURES.md) - Complete gateway features
- [../BACKEND_REGISTRATION.md](../BACKEND_REGISTRATION.md) - Backend registration architecture

## Getting Help

If you encounter issues:

1. Check the troubleshooting section above
2. Review service logs: `make logs`
3. Verify Docker/Podman resources are sufficient
4. Try a complete reset: `make reset`
5. Run tests locally before pushing to GitHub
6. Open an issue on GitHub with logs and error messages

## License

Apache License 2.0
