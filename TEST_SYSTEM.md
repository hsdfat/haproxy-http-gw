# HTTP Gateway Test System Documentation

Complete test infrastructure for the HTTP/2 Gateway system with functional and performance testing capabilities.

## Overview

This test system provides a complete Docker-based environment for testing the HTTP Gateway feature, including:

- **Automated Test Environment**: Docker Compose setup with all dependencies
- **Mock Services**: Backend servers and API provider for dynamic backend discovery
- **Test Clients**: Functional and performance testing tools
- **CI/CD Ready**: Easy integration into automated pipelines

## Quick Links

- **[Quick Start Guide](test/QUICKSTART.md)** - Get running in 5 minutes
- **[Full Documentation](test/README.md)** - Complete reference and advanced usage
- **[Gateway Implementation](GATEWAY_IMPLEMENTATION.md)** - Feature documentation
- **[Architecture Details](pkg/gateway/ARCHITECTURE.md)** - System architecture

## Directory Structure

```
test/
├── README.md                    # Full documentation
├── QUICKSTART.md               # Quick start guide
├── Makefile                    # Convenient make targets
├── docker-compose.yml          # Test environment definition
├── Dockerfile.gateway          # Gateway container
├── Dockerfile.backend          # Backend server container
├── Dockerfile.backend-api      # Backend API container
├── Dockerfile.test-client      # Test client container
├── backend/                    # Mock backend server
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── backend-api/                # REST API provider
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── client/                     # Test clients
│   ├── cmd/
│   │   ├── test-client/       # Functional tests
│   │   │   └── main.go
│   │   └── perf-client/       # Performance tests
│   │       └── main.go
│   ├── go.mod
│   └── go.sum
├── scripts/                    # Helper scripts
│   ├── gateway-entrypoint.sh
│   ├── run-tests.sh
│   └── generate-certs.sh
└── certs/                      # Generated SSL certificates
    ├── ca.crt
    ├── ca.key
    ├── server.crt
    ├── server.key
    └── server.pem
```

## Components

### 1. HTTP Gateway Service

The main gateway service running HAProxy with the HTTP/2 gateway controller.

**Features:**
- HTTP/2 support with ALPN negotiation
- Dynamic backend discovery via REST API
- Host and path-based routing
- Load balancing with health checks
- Zero-downtime configuration updates

**Ports:**
- 8080: HTTP
- 8443: HTTPS
- 9090: Stats/Admin (optional)

### 2. Backend API Service

REST API service that provides backend information to the gateway.

**Endpoints:**
- `GET /backends` - List all backends
- `GET /backends/{name}` - Get specific backend
- `POST /backends` - Create/update backend
- `DELETE /backends/{name}` - Delete backend
- `GET /health` - Health check

**Port:** 8000

### 3. Backend Servers

Mock HTTP/2-enabled backend servers that echo request information.

**Features:**
- HTTP/2 support
- Echo server (returns request details)
- Identifies itself in responses
- Health check endpoint

**Instances:**
- backend-server-1, backend-server-2, backend-server-3 (api-backend)
- web-server-1, web-server-2 (web-backend)

### 4. Test Clients

#### Functional Test Client

Automated test suite covering:
- ✓ Basic HTTP requests
- ✓ HTTP/2 protocol support
- ✓ Load balancing verification
- ✓ Path-based routing
- ✓ Host-based routing
- ✓ Health checks

#### Performance Test Client

Load testing tool with:
- Configurable concurrency
- Request count or duration-based tests
- Real-time progress reporting
- Detailed performance metrics
- HTTP/1.1 and HTTP/2 support

## Usage

### Quick Start (5 minutes)

```bash
cd test
make setup      # Generate certs, build, and start
make test-quick # Run smoke tests
```

### Full Test Suite

```bash
make test       # Run all tests (functional + performance)
```

### Individual Test Types

```bash
make test-functional  # Functional tests only
make test-perf       # Performance tests only
```

### Manual Testing

```bash
# Basic request
curl -H "Host: api.example.com" http://localhost:8080/api/test

# HTTP/2 request
curl -k --http2 -H "Host: api.example.com" https://localhost:8443/api/test

# Test load balancing
for i in {1..10}; do
  curl -s -H "Host: api.example.com" http://localhost:8080/api/test | jq -r '.server'
done
```

### Performance Testing

```bash
# Quick test (10 workers, 1000 requests)
docker-compose run --rm test-client /perf-client -c=10 -n=1000

# Load test (100 workers, 10000 requests)
docker-compose run --rm test-client /perf-client -c=100 -n=10000

# Duration test (20 workers, 30 seconds)
docker-compose run --rm test-client /perf-client -c=20 -d=30s

# HTTP/2 test
docker-compose run --rm test-client /perf-client -url=https://gateway:8443 -http2 -c=50 -n=5000
```

## Test Scenarios

### Scenario 1: Basic Functionality

Tests that the gateway correctly routes requests to backends.

```bash
make test-functional
```

### Scenario 2: Dynamic Backend Updates

Tests that the gateway picks up backend changes in real-time.

```bash
# Add new backend
curl -X POST http://localhost:8000/backends -H "Content-Type: application/json" -d '{
  "name": "dynamic-backend",
  "servers": [{"name": "srv1", "ip": "backend-server-3", "port": 9000}]
}'

# Verify gateway routes to new backend
# Gateway automatically detects and configures the new backend
```

### Scenario 3: Load Balancing

Verifies requests are distributed across backend servers.

```bash
# Make multiple requests and check distribution
for i in {1..20}; do
  curl -s -H "Host: api.example.com" http://localhost:8080/api/test | jq -r '.server'
done | sort | uniq -c

# Should show roughly equal distribution
```

### Scenario 4: Performance Testing

Tests throughput and latency under load.

```bash
make test-perf
```

### Scenario 5: HTTP/2 Performance

Compares HTTP/2 vs HTTP/1.1 performance.

```bash
# HTTP/2
docker-compose run --rm test-client /perf-client -url=https://gateway:8443 -http2 -c=100 -n=10000

# HTTP/1.1
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=100 -n=10000
```

## Monitoring and Debugging

### View Logs

```bash
make logs                              # All services
docker-compose logs -f gateway         # Gateway only
docker-compose logs -f backend-api     # Backend API only
```

### Check HAProxy Status

```bash
# View backends
docker exec http-gateway sh -c "echo 'show backend' | socat - /var/run/haproxy-runtime-api.sock"

# View server states
docker exec http-gateway sh -c "echo 'show servers state' | socat - /var/run/haproxy-runtime-api.sock"

# View statistics
docker exec http-gateway sh -c "echo 'show stat' | socat - /var/run/haproxy-runtime-api.sock"
```

### Monitor Resource Usage

```bash
docker stats
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Gateway Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup test environment
        run: cd test && make setup

      - name: Run functional tests
        run: cd test && make test-functional

      - name: Run performance tests
        run: cd test && make test-perf

      - name: Cleanup
        if: always()
        run: cd test && make clean
```

## Performance Benchmarks

Expected performance on typical development machine (4 CPU, 8GB RAM):

| Test Configuration | Requests/sec | Avg Latency | Success Rate |
|-------------------|--------------|-------------|--------------|
| 10 workers, HTTP/1.1 | 500-1000 | 10-20ms | >99% |
| 50 workers, HTTP/1.1 | 1000-2000 | 20-50ms | >99% |
| 100 workers, HTTP/1.1 | 1500-3000 | 30-70ms | >98% |
| 50 workers, HTTP/2 | 1200-2500 | 15-40ms | >99% |
| 100 workers, HTTP/2 | 2000-4000 | 25-60ms | >99% |

HTTP/2 typically shows 10-30% better performance under high concurrency.

## Common Commands

```bash
make help           # Show all available commands
make setup          # Complete setup (first time)
make up             # Start services
make down           # Stop services
make logs           # View logs
make test           # Run all tests
make test-quick     # Quick smoke test
make clean          # Stop and remove volumes
make reset          # Complete cleanup and rebuild
```

## Troubleshooting

### Services won't start

```bash
# Check logs
docker-compose logs

# Verify Docker resources
docker info

# Reset everything
make reset
```

### Tests fail

```bash
# Wait for services to be fully ready
sleep 30 && make test

# Check individual service health
docker-compose ps
curl http://localhost:8080/health
curl http://localhost:8000/health
```

### Port conflicts

```bash
# Check what's using ports
lsof -i :8080
lsof -i :8443
lsof -i :8000

# Edit docker-compose.yml to use different ports
```

### Low performance

```bash
# Increase Docker resources in Docker Desktop settings
# Reduce test concurrency
docker-compose run --rm test-client /perf-client -c=10 -n=1000
```

## Customization

### Adding Backend Servers

1. Edit `docker-compose.yml` to add new backend service
2. Update backend via API:

```bash
curl -X POST http://localhost:8000/backends -H "Content-Type: application/json" -d '{
  "name": "api-backend",
  "servers": [
    {"name": "backend-server-1", "ip": "backend-server-1", "port": 9000},
    {"name": "backend-server-2", "ip": "backend-server-2", "port": 9000},
    {"name": "new-server", "ip": "new-server", "port": 9000}
  ]
}'
```

### Modifying Test Parameters

Edit the test client commands in `scripts/run-tests.sh` or use custom parameters:

```bash
docker-compose run --rm test-client /perf-client -c=200 -d=5m
```

### Changing Gateway Configuration

Edit environment variables in `docker-compose.yml`:

```yaml
gateway:
  environment:
    - LOG_LEVEL=debug
    - SYNC_PERIOD=10s
```

## Best Practices

1. **Always run `make setup` on first use** to generate certificates and build images
2. **Use `make test-quick` for rapid feedback** during development
3. **Run full tests before committing** with `make test`
4. **Monitor resource usage** with `docker stats` during performance tests
5. **Check logs** when debugging with `make logs`
6. **Clean up regularly** with `make clean` to free resources
7. **Use HTTP/2 for performance tests** to test realistic scenarios

## Resources

- [Quick Start Guide](test/QUICKSTART.md)
- [Full Test Documentation](test/README.md)
- [Gateway Feature Documentation](pkg/gateway/README.md)
- [Architecture Documentation](pkg/gateway/ARCHITECTURE.md)
- [Implementation Guide](GATEWAY_IMPLEMENTATION.md)

## License

Apache License 2.0
