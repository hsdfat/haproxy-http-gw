# HAProxy HTTP Gateway Documentation

This directory contains all documentation for the HAProxy HTTP Gateway project.

## Gateway Documentation

- **[gateway-architecture.md](gateway-architecture.md)** - Gateway architecture, components, and design patterns
- **[gateway-api.md](gateway-api.md)** - Gateway API reference and usage examples
- **[multi-frontend-setup.md](multi-frontend-setup.md)** - Multi-frontend configuration and testing

## Testing Documentation

- **[testing-guide.md](testing-guide.md)** - Testing guide and test environment setup
- **[test-features.md](test-features.md)** - Test features and capabilities

## Quick Links

### Running Tests
```bash
cd test && ./run-local-test.sh
```

### Test Endpoints
- Default Frontend: http://localhost:8080
- API Frontend: http://localhost:8081 (HTTP/2)
- Web Frontend: http://localhost:8082 (HTTP/2)
- Gateway API: http://localhost:9090

### Configuration Files
- Frontend config: [test/frontend-config-test.yaml](../test/frontend-config-test.yaml)
- Docker compose: [test/docker-compose.yml](../test/docker-compose.yml)
- GitHub Actions: [.github/workflows/gateway-tests.yml](../.github/workflows/gateway-tests.yml)

### Key Implementation Files
- Gateway main: [cmd/http-gateway/main.go](../cmd/http-gateway/main.go)
- Frontend manager: [pkg/gateway/frontend_manager.go](../pkg/gateway/frontend_manager.go)
- Gateway manager: [pkg/gateway/manager.go](../pkg/gateway/manager.go)
- HAProxy API client: [pkg/haproxy/api/api.go](../pkg/haproxy/api/api.go)
