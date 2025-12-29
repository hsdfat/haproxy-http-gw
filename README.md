# HAProxy HTTP Gateway

A dynamic HTTP gateway built on HAProxy with support for multiple frontends, dynamic backend registration, and HTTP/2 (H2C) protocol.

## Features

- **Multi-Frontend Support** - Run multiple independent HTTP frontends on different ports
- **Dynamic Backend Registration** - Backends self-register via API on startup
- **Runtime-First Updates** - Socket-based updates for instant changes without HAProxy reloads
- **HTTP/2 (H2C)** - HTTP/2 over cleartext support on all frontends
- **Round-Robin Load Balancing** - Automatic traffic distribution across backend servers
- **Zero-Downtime Reloads** - Graceful HAProxy configuration updates when necessary
- **RESTful API** - Manage frontends and backends via HTTP API
- **Runtime Socket API** - Direct access to HAProxy runtime socket information
- **Path Statistics & Rate Limiting** - Track per-path metrics and protect against abuse with stick tables (standalone deployments only)

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

### Stick Tables (Path Statistics & Rate Limiting)

**Note**: Stick tables currently work with standalone HAProxy deployments only. Container-based deployments require API implementation.

- **[Stick Tables Documentation](STICK_TABLES_IMPLEMENTATION.md)** - Complete implementation guide, configuration, and usage

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

## Performance: Runtime-First Updates

The gateway uses a **runtime-first strategy** for backend updates to minimize HAProxy reloads:

### How It Works

1. **Server Slot Pre-allocation**: Each backend gets 42 pre-allocated server slots by default
2. **Runtime Socket Updates**: Changes within available slots use HAProxy socket commands (instant, no reload)
3. **Automatic Fallback**: Config reload only when slots exhausted or runtime update fails

### Performance Benefits

```bash
# Example: 5 frontends with dynamic backends
# Initial registration:  5 reloads (one per new backend)
# Subsequent updates:    0 reloads (all via runtime socket)

# Single backend update pattern:
# Initial: 1 reload (creates 42 slots)
# Add servers (within 42): 0 reloads
# Update IPs: 0 reloads
# Remove servers: 0 reloads
```

### Verification

Updates via runtime socket do NOT modify the config file - they only change HAProxy's in-memory state:

```bash
# Check runtime state via API
curl http://localhost:9090/api/runtime/backends/api-backend/servers

# Config file shows original values
# Runtime socket shows updated values
# Traffic uses runtime state (not config file)
```

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

### Runtime Socket API

Query HAProxy runtime socket information directly:

```bash
# List all backends with runtime state
curl http://localhost:9090/api/runtime/backends

# Show server connections and stats for a backend
curl http://localhost:9090/api/runtime/backends/api-backend/servers

# Show detailed backend stats
curl http://localhost:9090/api/runtime/backends/api-backend/stats

# Execute custom socket commands (read-only: show/get commands)
curl -X POST http://localhost:9090/api/runtime/command \
  -H "Content-Type: application/json" \
  -d '{"command": "show servers state api-backend"}'
```

**Security Note**: Only read-only socket commands (`show`, `get`) are allowed via the API.

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
