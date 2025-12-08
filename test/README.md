# HTTP Gateway Test System

Complete test infrastructure for the HAProxy HTTP/HTTP2 Gateway with automated testing, performance benchmarking, and dynamic backend management.

## Quick Start

### Prerequisites

- Docker/Podman with compose
- 8GB+ RAM available
- Ports 8000, 8080, 8443, 9090 available

### 5-Minute Setup

```bash
cd test
make setup  # Generate certs, build images, start services
make test   # Run all tests
```

Expected output:
```
✓ Functional Tests: PASS (6/6)
✓ Performance Tests: PASS (>99% success rate)
✓ All systems operational
```

### Manual Testing

```bash
# Basic HTTP request
curl -H "Host: api.example.com" http://localhost:8080/api/test

# HTTP/2 request
curl -k --http2 -H "Host: api.example.com" https://localhost:8443/api/test

# Test load balancing
for i in {1..10}; do
  curl -s http://localhost:8080/ | jq -r '.server'
done
```

## Architecture

```
┌─────────────┐
│Test Client  │
└──────┬──────┘
       │
       ↓
┌──────────────────┐         ┌─────────────────┐
│  HTTP Gateway    │ ← REST ←│  Backend API    │
│  :8080 :8443     │   API   │  :8000          │
└────────┬─────────┘         └─────────────────┘
         │
         ├─→ backend-server-1 (api-backend)
         ├─→ backend-server-2 (api-backend)
         ├─→ backend-server-3 (api-backend)
         ├─→ web-server-1 (web-backend)
         └─→ web-server-2 (web-backend)
```

**Features:**
- **HTTP/HTTP2 Gateway** - HAProxy with HTTP/1.1 and HTTP/2 (H2C) support
- **Backend API** - REST API for dynamic backend discovery
- **Mock Backends** - HTTP/2-enabled test servers with auto-registration
- **Test Clients** - Functional and performance testing tools
- **Auto-Registration** - Backends self-register using IP auto-detection

## Components

### 1. HTTP Gateway (Ports 8080, 8443, 9090)

Main gateway service providing:
- HTTP/1.1 and HTTP/2 protocol support
- Path and host-based routing
- Round-robin load balancing
- Health monitoring
- Admin API (port 9090)

**Configuration:**
- HTTP Port: 8080
- HTTPS Port: 8443
- Admin/API Port: 9090
- Stats available at: http://localhost:9090/stats

### 2. Backend API (Port 8000)

REST API for dynamic backend management:

```bash
# List backends
curl http://localhost:8000/backends | jq

# Register backend
curl -X POST http://localhost:8000/backends -H "Content-Type: application/json" -d '{
  "name": "my-backend",
  "servers": [{"name": "srv1", "ip": "10.0.0.1", "port": 8080}]
}'

# Delete backend
curl -X DELETE http://localhost:8000/backends/my-backend
```

Default backends:
- `api-backend`: backend-server-1, backend-server-2, backend-server-3
- `web-backend`: web-server-1, web-server-2

### 3. Backend Servers

Mock HTTP/2-enabled servers that:
- Auto-detect their IP address from network interface
- Self-register with gateway on startup
- Echo request details (method, path, headers, protocol)
- Support both HTTP/1.1 and HTTP/2

**IP Auto-Detection:**
Servers automatically detect their container IP using:
1. `eth0` interface IP (primary method)
2. `hostname -i` (fallback)
3. Hostname (last resort)
4. Explicit `SERVER_IP` env var (override)

See [Protocol & Features](FEATURES.md) for HTTP/2 details.

### 4. Test Clients

#### Functional Test Client
```bash
docker-compose run --rm test-client /test-client -gateway=http://gateway:8080 -verbose
```

Tests:
- Basic HTTP requests
- HTTP/2 support
- Load balancing
- Path/host routing
- Health checks

#### Performance Test Client
```bash
# 50 concurrent workers, 5000 requests
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=50 -n=5000

# HTTP/2 performance
docker-compose run --rm test-client /perf-client -url=https://gateway:8443 -http2 -c=50 -n=5000
```

Metrics:
- Requests per second (RPS)
- Latency (min, max, avg, p50, p95, p99)
- Success rate
- Error distribution

## Testing

### Using Make Commands

```bash
make setup          # First-time setup (certs, build, start)
make test           # Run all tests
make test-functional # Functional tests only
make test-perf      # Performance tests only
make test-quick     # Quick smoke test
make logs           # View all logs
make clean          # Stop and cleanup
make reset          # Complete reset and rebuild
```

### Local Testing (Mirrors GitHub Actions)

Run the exact same tests as GitHub Actions CI/CD:

```bash
cd test
./run-github-action-tests.sh
```

This runs:
1. Functional tests (basic functionality)
2. Performance test - Low concurrency (10 workers, 1000 requests)
3. Performance test - Medium concurrency (50 workers, 5000 requests)
4. HTTP/2 performance test (50 workers, 5000 requests)
5. Dynamic backend tests (register/unregister)

**Prerequisites:**
```bash
# macOS
brew install jq bc podman
pip3 install podman-compose

# Ubuntu/Debian
sudo apt-get install jq bc
pip3 install podman-compose
```

### Frontend Management Testing

Test the frontend management feature (multiple frontends, per-frontend backends/routes):

```bash
cd test
./run-frontend-test.sh
```

This runs:
1. **Unit Tests**: Configuration validation, providers, registry
2. **API Tests**: Frontend management endpoints
   - List frontends
   - Get frontend details
   - Frontend statistics
3. **Backend Tests**: Per-frontend backend registration
   - Register backends to specific frontends
   - List backends per frontend
   - Unregister backends
4. **Route Tests**: Per-frontend route management
   - Add routes to frontends
   - List routes per frontend
   - Delete routes
5. **Configuration Validation**: Example YAML files
6. **Documentation Checks**: Required docs exist

**GitHub CI**: Frontend management tests run automatically on push/PR via [.github/workflows/frontend-management-tests.yml](../.github/workflows/frontend-management-tests.yml)

See [Testing Guide](TESTING.md) for detailed testing instructions.

## Development Workflow

### 1. Start Services

```bash
cd test
podman-compose up -d  # or docker-compose
```

### 2. Verify Health

```bash
# Gateway health
curl http://localhost:9090/health

# Registered backends
curl http://localhost:9090/api/backends | jq
```

### 3. Configure Routes

```bash
./scripts/configure-routes.sh
```

Adds:
- `api.example.com/api` → `api-backend`
- `www.example.com/` → `web-backend`

### 4. Manual Testing

```bash
# Test with host header
curl -H "Host: api.example.com" http://localhost:8080/api/test

# Load balancing verification
for i in {1..20}; do
  curl -s http://localhost:8080/ | jq -r '.server'
done | sort | uniq -c

# HTTP/2 test (H2C - cleartext)
curl --http2-prior-knowledge http://localhost:8080/

# HTTP/2 test (HTTPS)
curl -k --http2 https://localhost:8443/
```

### 5. View Logs

```bash
# All logs
podman-compose logs -f

# Specific service
podman-compose logs -f gateway
podman-compose logs -f backend-server-1
podman-compose logs -f backend-api
```

### 6. Debug HAProxy

```bash
# HAProxy stats
echo "show stat" | podman exec -i http-gateway socat - /var/run/haproxy-runtime-api.sock

# Backend status
echo "show servers state" | podman exec -i http-gateway socat - /var/run/haproxy-runtime-api.sock

# Current config
echo "show config" | podman exec -i http-gateway socat - /var/run/haproxy-runtime-api.sock
```

### 7. Cleanup

```bash
podman-compose down    # Stop services
podman-compose down -v # Stop and remove volumes
```

## Performance Benchmarks

Expected performance on typical development hardware:

| Environment | Concurrency | Requests | RPS | Avg Latency | Success Rate |
|------------|-------------|----------|-----|-------------|--------------|
| GitHub Actions | 10 | 1,000 | 800-1,000 | 10-20ms | >99% |
| GitHub Actions | 50 | 5,000 | 2,000-3,000 | 20-50ms | >98% |
| GitHub Actions HTTP/2 | 50 | 5,000 | 2,500-4,000 | 20-40ms | >98% |
| MacBook Pro M1 | 50 | 5,000 | 4,000-6,000 | 10-25ms | >99% |
| MacBook Pro M1 HTTP/2 | 50 | 5,000 | 5,000-8,000 | 8-20ms | >99% |

**HTTP/2 Benefits:**
- 15-30% higher throughput
- 20-40% lower latency
- Better connection multiplexing
- Reduced overhead

## Troubleshooting

### Services Won't Start

```bash
# Check Docker/Podman resources
docker info  # or podman info

# View logs
podman-compose logs

# Reset everything
make reset
```

### Tests Fail

```bash
# Verify services are healthy
podman-compose ps

# Wait longer for startup
sleep 30 && make test

# Check gateway health
curl http://localhost:9090/health

# View gateway logs
podman-compose logs gateway
```

### Backends Not Registered

```bash
# Check backend logs
podman-compose logs backend-server-1

# View registered backends
curl http://localhost:9090/api/backends | jq

# Manually register
curl -X POST http://localhost:9090/api/backends \
  -H "Content-Type: application/json" \
  -d '{"name": "api-backend", "servers": [{"name": "srv1", "ip": "backend-server-1", "port": 9000}]}'

# Restart backends
podman-compose restart backend-server-1 backend-server-2 backend-server-3
```

### Port Conflicts

```bash
# Check what's using ports
lsof -i :8080
lsof -i :8443
lsof -i :9090
lsof -i :8000

# Change ports in docker-compose.yml or stop conflicting services
```

### Load Balancing Not Working

```bash
# Test distribution
for i in {1..20}; do curl -s http://localhost:8080/ | jq -r '.server'; done | sort | uniq -c

# Should show even distribution:
#   10 backend-server-1
#   10 backend-server-2

# Check backend count
curl http://localhost:9090/api/backends | jq '.backends["api-backend"].Servers | length'
```

### HTTP/2 Not Working

```bash
# Check curl HTTP/2 support
curl --version | grep HTTP2

# Test H2C (HTTP/2 cleartext)
curl --http2-prior-knowledge -v http://localhost:8080/

# Test HTTPS HTTP/2
curl -k --http2 -v https://localhost:8443/ 2>&1 | grep "HTTP/2"

# View backend logs for protocol
curl -s http://localhost:8080/ | jq '.protocol'
# Should show "HTTP/2.0" or "HTTP/1.1"
```

### Performance Issues

```bash
# Check system resources
top
docker stats  # or podman stats

# Reduce concurrency
docker-compose run --rm test-client /perf-client -c=5 -n=100

# Monitor gateway
podman stats http-gateway
```

## CI/CD Integration

### GitHub Actions

The `.github/workflows/gateway-tests.yml` workflow runs automatically on:
- Push to `master`, `main`, or `develop`
- Pull requests
- Manual trigger

**Test Phases:**
1. Build and start services
2. Health verification
3. Functional tests (6 tests)
4. Performance tests (3 variants)
5. Dynamic backend tests
6. Generate test summary

**Artifacts:**
- `functional-results.txt`
- `perf-low-results.txt`
- `perf-medium-results.txt`
- `perf-http2-results.txt`

### Running Locally Before Push

```bash
# Run exact GitHub Actions tests
cd test
./run-github-action-tests.sh

# Fix any failures before pushing
```

## Customization

### Add More Backend Servers

Edit `docker-compose.yml`:

```yaml
backend-server-4:
  build:
    context: ..
    dockerfile: test/Dockerfile.backend
  container_name: backend-server-4
  environment:
    - SERVER_NAME=backend-server-4
    - SERVER_PORT=9000
    - BACKEND_NAME=api-backend
    - GATEWAY_URL=http://gateway:9090
    - ENABLE_HTTP2=true
  networks:
    - gateway-net
```

Then restart:
```bash
podman-compose up -d backend-server-4
```

The server will auto-register with the gateway.

### Configure Gateway Settings

Edit `docker-compose.yml` gateway environment:

```yaml
gateway:
  environment:
    - LOG_LEVEL=debug           # trace, debug, info, warning, error
    - BACKEND_API_URL=http://backend-api:8000
    - SYNC_PERIOD=5s            # Backend sync period
```

### Modify Test Scenarios

Edit test client code:
- **Functional tests:** `test/client/cmd/test-client/main.go`
- **Performance tests:** `test/client/cmd/perf-client/main.go`

Rebuild:
```bash
make build
```

## File Structure

```
test/
├── README.md                      # This file
├── FEATURES.md                    # Protocol support & features
├── TESTING.md                     # Detailed testing guide
├── docker-compose.yml             # Service definitions
├── Makefile                       # Build and test targets
├── haproxy-init.cfg               # HAProxy configuration
├── run-local-test.sh              # Simple local test script
├── run-github-action-tests.sh     # Full CI/CD test script
├── Dockerfile.gateway             # Gateway image
├── Dockerfile.backend             # Backend server image
├── Dockerfile.backend-api         # Backend API image
├── Dockerfile.client              # Test client image
├── scripts/
│   ├── generate-certs.sh          # SSL certificate generation
│   ├── gateway-entrypoint.sh      # Gateway initialization
│   ├── backend-entrypoint.sh      # Backend initialization
│   ├── register-backend.sh        # Backend registration
│   ├── configure-routes.sh        # Route configuration
│   └── run-tests.sh               # Test execution
├── backend/
│   ├── main.go                    # Backend server implementation
│   └── go.mod
├── backend-api/
│   ├── main.go                    # Backend API implementation
│   └── go.mod
└── client/
    ├── cmd/
    │   ├── test-client/           # Functional test client
    │   └── perf-client/           # Performance test client
    ├── README.md                  # Client documentation
    └── go.mod
```

## Environment Variables

### Gateway
```bash
LOG_LEVEL=debug                           # Logging level
HAPROXY_RUNTIME_SOCKET=/var/run/haproxy-runtime-api.sock
```

### Backend Servers
```bash
SERVER_NAME=backend-server-1              # Unique server name
SERVER_IP=                                # Auto-detected from eth0 (or set explicitly)
SERVER_PORT=9000                          # Server port
BACKEND_NAME=api-backend                  # Backend group
GATEWAY_URL=http://gateway:9090           # Gateway API URL
ENABLE_HTTP2=true                         # Enable HTTP/2
```

## Common Make Commands

| Command | Description |
|---------|-------------|
| `make setup` | First-time setup (certs, build, start) |
| `make up` | Start all services |
| `make down` | Stop all services |
| `make ps` | Show service status |
| `make logs` | View all logs |
| `make test` | Run all tests |
| `make test-functional` | Functional tests only |
| `make test-perf` | Performance tests only |
| `make test-quick` | Quick smoke test |
| `make clean` | Stop services and remove volumes |
| `make reset` | Complete cleanup and rebuild |
| `make help` | Show all available commands |

## Additional Documentation

- **[FEATURES.md](FEATURES.md)** - HTTP/2 support, protocol details, IP auto-detection
- **[TESTING.md](TESTING.md)** - Comprehensive testing guide with examples
- **[client/README.md](client/README.md)** - Test client documentation
- **[../GATEWAY_FEATURES.md](../GATEWAY_FEATURES.md)** - Complete gateway feature documentation
- **[../BACKEND_REGISTRATION.md](../BACKEND_REGISTRATION.md)** - Backend registration architecture

## Getting Help

If you encounter issues:

1. Check the troubleshooting section above
2. Review service logs: `make logs`
3. Verify Docker/Podman resources are sufficient
4. Try a complete reset: `make reset`
5. Open an issue on GitHub

## Success Indicators

After setup, you should see:

✓ All Docker containers running (`make ps`)
✓ Gateway responding on ports 8080, 8443, 9090
✓ Backend API responding on port 8000
✓ Backends auto-registered (`curl http://localhost:9090/api/backends | jq`)
✓ Functional tests passing (6/6)
✓ Performance tests with >98% success rate
✓ Load balancing distributing across servers

## License

Apache License 2.0
