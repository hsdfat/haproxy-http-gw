# HTTP Gateway Test System - Quick Start Guide

Get up and running with the HTTP Gateway test system in 5 minutes.

## Prerequisites

- Docker and Docker Compose installed
- 8GB+ RAM available
- Ports 8000, 8080, 8443 available

## 1. Setup (First Time Only)

```bash
cd test
make setup
```

This will:
- Generate SSL certificates
- Build all Docker images
- Start all services
- Wait for everything to be ready

Expected output:
```
✓ Certificates generated
✓ Building images...
✓ Starting services...
✓ Services ready!
```

## 2. Verify Services

```bash
docker-compose ps
```

You should see all services running:
- `gateway` - HTTP Gateway (ports 8080, 8443)
- `backend-api` - Backend provider (port 8000)
- `backend-server-1`, `backend-server-2`, `backend-server-3` - Backend servers
- `web-server-1`, `web-server-2` - Web servers

## 3. Run Tests

### Quick Smoke Test

```bash
make test-quick
```

### Full Functional Tests

```bash
make test-functional
```

### Full Performance Tests

```bash
make test-perf
```

### All Tests

```bash
make test
```

## 4. Manual Testing

### Basic HTTP Request

```bash
curl -H "Host: api.example.com" http://localhost:8080/api/test
```

Expected response:
```json
{
  "server": "backend-server-1",
  "timestamp": "2025-12-05T10:00:00Z",
  "path": "/api/test",
  "method": "GET",
  "protocol": "HTTP/1.1"
}
```

### HTTP/2 Request

```bash
curl -k --http2 -H "Host: api.example.com" https://localhost:8443/api/test
```

### Test Load Balancing

```bash
for i in {1..10}; do
  curl -s -H "Host: api.example.com" http://localhost:8080/api/test | jq -r '.server'
done
```

You should see requests distributed across `backend-server-1` and `backend-server-2`.

## 5. Dynamic Backend Management

### List Backends

```bash
curl http://localhost:8000/backends | jq
```

### Add New Backend

```bash
curl -X POST http://localhost:8000/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "new-backend",
    "servers": [
      {"name": "backend-server-3", "ip": "backend-server-3", "port": 9000}
    ]
  }'
```

### Update Existing Backend

```bash
curl -X POST http://localhost:8000/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-backend",
    "servers": [
      {"name": "backend-server-1", "ip": "backend-server-1", "port": 9000},
      {"name": "backend-server-2", "ip": "backend-server-2", "port": 9000},
      {"name": "backend-server-3", "ip": "backend-server-3", "port": 9000}
    ]
  }'
```

### Delete Backend

```bash
curl -X DELETE http://localhost:8000/backends/new-backend
```

## 6. View Logs

### All Services

```bash
make logs
```

### Specific Service

```bash
docker-compose logs -f gateway
docker-compose logs -f backend-api
docker-compose logs -f backend-server-1
```

## 7. Common Commands

| Command | Description |
|---------|-------------|
| `make up` | Start services |
| `make down` | Stop services |
| `make logs` | View logs |
| `make test` | Run all tests |
| `make clean` | Stop and remove volumes |
| `make reset` | Complete cleanup and rebuild |
| `make help` | Show all available commands |

## 8. Performance Testing Examples

### Quick Test (10 concurrent, 1000 requests)

```bash
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=10 -n=1000
```

### Load Test (100 concurrent, 10000 requests)

```bash
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=100 -n=10000
```

### Duration Test (20 concurrent, 30 seconds)

```bash
docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=20 -d=30s
```

### HTTP/2 Performance Test

```bash
docker-compose run --rm test-client /perf-client -url=https://gateway:8443 -http2 -c=50 -n=5000
```

## 9. Troubleshooting

### Services won't start

```bash
# Check Docker resources
docker info

# Check logs
docker-compose logs

# Reset everything
make reset
```

### Tests fail

```bash
# Verify services are healthy
docker-compose ps

# Wait longer for startup
sleep 30 && make test

# Check specific service
docker-compose logs gateway
```

### Port conflicts

```bash
# Check what's using the ports
lsof -i :8080
lsof -i :8443
lsof -i :8000

# Stop conflicting services or change ports in docker-compose.yml
```

## 10. Cleanup

### Stop Services (Keep Data)

```bash
make down
```

### Complete Cleanup

```bash
make clean
```

### Full Reset

```bash
make reset
```

## Next Steps

- Read the full [README.md](README.md) for detailed documentation
- Explore the [test client source code](client/cmd/)
- Customize backends in [docker-compose.yml](docker-compose.yml)
- Integrate tests into your CI/CD pipeline

## Example Test Session

```bash
# 1. Setup
cd test
make setup

# 2. Verify
curl -H "Host: api.example.com" http://localhost:8080/api/test

# 3. Run tests
make test-quick

# 4. Add a backend server
curl -X POST http://localhost:8000/backends -H "Content-Type: application/json" -d '{
  "name": "test-backend",
  "servers": [{"name": "srv1", "ip": "backend-server-1", "port": 9000}]
}'

# 5. Performance test
docker-compose run --rm test-client /perf-client -c=50 -n=5000

# 6. View results and logs
make logs

# 7. Cleanup
make clean
```

## Success Indicators

After setup, you should see:

✓ All Docker containers running
✓ Gateway responding on ports 8080 and 8443
✓ Backend API responding on port 8000
✓ Functional tests passing (6/6 tests)
✓ Performance tests completing with >99% success rate
✓ Load balancing working across multiple backend servers

## Getting Help

If you encounter issues:

1. Check the [README.md](README.md) troubleshooting section
2. Review service logs: `make logs`
3. Verify Docker resources are sufficient
4. Try a complete reset: `make reset`

## Performance Expectations

On a typical development machine:

- **Functional Tests**: Complete in 5-10 seconds
- **Quick Perf Test**: ~2 seconds, ~500-1000 req/s
- **Medium Perf Test**: ~5 seconds, ~1000-2000 req/s
- **Heavy Perf Test**: ~10 seconds, ~1500-3000 req/s

HTTP/2 typically shows 10-30% better performance than HTTP/1.1.
