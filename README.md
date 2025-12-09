# HAProxy HTTP Gateway

A dynamic HTTP gateway built on HAProxy with support for multiple frontends, dynamic backend registration, and HTTP/2 (H2C) protocol.

## Features

- **Multi-Frontend Support** - Run multiple independent HTTP frontends on different ports
- **Dynamic Backend Registration** - Backends self-register via API on startup
- **HTTP/2 (H2C)** - HTTP/2 over cleartext support on all frontends
- **Round-Robin Load Balancing** - Automatic traffic distribution across backend servers
- **Zero-Downtime Reloads** - Graceful HAProxy configuration updates
- **RESTful API** - Manage frontends and backends via HTTP API

## Quick Start

### Running Tests

```bash
cd test
./run-local-test.sh
```

This will:
1. Build all container images
2. Start 3 frontends (ports 8080, 8081, 8082)
3. Start 9 backend servers
4. Run functional and performance tests
5. Verify load balancing and HTTP/2 support

### Test Endpoints

- **Default Frontend:** http://localhost:8080
- **API Frontend:** http://localhost:8081 (HTTP/2)
- **Web Frontend:** http://localhost:8082 (HTTP/2)
- **Gateway API:** http://localhost:9090

## Documentation

See [docs/](docs/) directory for comprehensive documentation:

- **[Gateway Architecture](docs/gateway-architecture.md)** - Architecture, components, and design patterns
- **[Gateway API](docs/gateway-api.md)** - API reference and usage examples
- **[Multi-Frontend Setup](docs/multi-frontend-setup.md)** - Multi-frontend configuration and testing
- **[Testing Guide](docs/testing-guide.md)** - Test environment and execution
- **[Test Features](docs/test-features.md)** - Test features and capabilities

## Architecture

```
┌─────────────────────────────────────────┐
│     HAProxy HTTP Gateway                │
├─────────────────────────────────────────┤
│  Frontend: default (8080)               │
│    └─> api-backend (3 servers)          │
│                                         │
│  Frontend: frontend-api (8081, H2C)     │
│    └─> api-backend (3 servers)          │
│                                         │
│  Frontend: frontend-web (8082, H2C)     │
│    └─> web-backend (2 servers)          │
└─────────────────────────────────────────┘
```

## Configuration

- **Frontend Config:** [test/frontend-config-test.yaml](test/frontend-config-test.yaml)
- **Docker Compose:** [test/docker-compose.yml](test/docker-compose.yml)
- **GitHub Actions:** [.github/workflows/gateway-tests.yml](.github/workflows/gateway-tests.yml)

## API Endpoints

### Frontend Management

```bash
# List all frontends
curl http://localhost:9090/api/frontends

# Get frontend details
curl http://localhost:9090/api/frontends/default
```

### Backend Management

```bash
# List backends for a frontend
curl http://localhost:9090/api/frontends/default/backends

# Register a new backend
curl -X POST http://localhost:9090/api/frontends/default/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "new-backend",
    "servers": [
      {"name": "srv1", "ip": "192.168.1.10", "port": 8080}
    ]
  }'

# Unregister a backend
curl -X DELETE http://localhost:9090/api/frontends/default/backends/new-backend
```

## Building

```bash
# Build gateway binary
go build -o http-gateway ./cmd/http-gateway

# Build with Docker
docker build -t http-gateway -f test/Dockerfile .
```

## Testing

### Local Tests
```bash
cd test && ./run-local-test.sh
```

### GitHub Actions
Tests run automatically on push to master/main branches.

## License

Apache License 2.0
