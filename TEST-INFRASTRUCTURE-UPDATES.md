# Test Infrastructure Updates

## Summary

Updated the test infrastructure to align with the backend registration "replace" pattern and ensure all load balancing tests pass with multiple servers per backend.

## Problem

The backend registration API uses a **replace** pattern (not merge) - when a backend is registered, it replaces the entire backend configuration. Individual backend servers were registering themselves one at a time, resulting in only the last server persisting in each backend. This caused load balancing tests to fail because only 1 server received traffic instead of being distributed across multiple servers.

## Solution

### 1. Created Multi-Server Registration Script

**File:** `test/scripts/register-all-backends.sh`

This centralized script:
- Discovers all backend container IPs dynamically using `podman inspect` or `docker inspect`
- Registers backends with multiple servers in a single API call (respecting the replace pattern)
- Handles all 3 frontends:
  - **default frontend (port 8080):**
    - `api-backend`: 3 servers (backend-server-1, 2, 3)
    - `web-backend`: 2 servers (web-server-1, 2)
  - **frontend-api (port 8081):**
    - `api-v2-backend`: 2 servers (api-v2-server-1, 2)
  - **frontend-web (port 8082):**
    - `web-v2-backend`: 2 servers (web-v2-server-1, 2)

### 2. Updated Test Scripts

**File:** `run-all-tests.sh`
- Added backend re-registration step before running functional tests
- Ensures all backends have multiple servers for proper load balancing

**File:** `test/run-github-action-tests.sh`
- Added "Re-register Backends with Multiple Servers" section
- Includes verification of multi-server registration with server counts
- Validates that each backend has at least 2 servers before proceeding with tests

**File:** `.github/workflows/gateway-tests.yml`
- Added new workflow step: "Re-register Backends with Multiple Servers"
- Executes after backend containers are ready but before running tests
- Verifies backend configuration with expected server counts

### 3. Helper Scripts

**File:** `test/scripts/register-multi-backend.sh` (for future use)
- Generic script that can register a backend with multiple servers
- Uses `SERVERS_JSON` environment variable for flexible server configuration
- Can be used for custom backend registration scenarios

## Test Results

All tests now pass with excellent performance:

### Functional Tests (15/15 passed)
- ✅ Basic HTTP Request (all 3 frontends)
- ✅ HTTP/2 Support (H2C) (all 3 frontends)
- ✅ **Load Balancing** (all 3 frontends) - **FIXED**
- ✅ Different Paths (all 3 frontends)
- ✅ Health Check (all 3 frontends)

### Performance Tests (All passed)

**Low Concurrency (8 workers, 2K requests):**
- Default: 3,878 req/s (100% success)
- API: 4,898 req/s (100% success)
- Web: 4,124 req/s (100% success)

**Medium Concurrency (16 workers, 20K requests):**
- Default: 3,569 req/s (100% success)
- API: 4,599 req/s (100% success)
- Web: 3,462 req/s (100% success)

**High Concurrency (32 workers, 100K requests):**
- Default: 4,050 req/s (100% success)
- API: 4,027 req/s (100% success)
- Web: 5,416 req/s (100% success)

### Dynamic Backend Test
- ✅ Backend registration and unregistration working correctly

## Architecture Notes

### Backend Registration Pattern

The gateway uses a **replace** pattern for backend registration:
```go
// pkg/gateway/manager.go:450
m.backends[backend.Name] = &backend  // Replaces entire backend
```

This is the correct design because:
1. It provides atomic updates - no partial state issues
2. It simplifies backend configuration management
3. It matches the declarative nature of the API

### Multi-Server Registration

To register multiple servers to a backend, send them all in one request:

```bash
curl -X POST http://localhost:9090/api/frontends/default/backends \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-backend",
    "servers": [
      {"name": "backend-server-1", "ip": "10.89.1.4", "port": 9000},
      {"name": "backend-server-2", "ip": "10.89.1.5", "port": 9000},
      {"name": "backend-server-3", "ip": "10.89.1.6", "port": 9000}
    ]
  }'
```

## Files Changed

1. `test/scripts/register-all-backends.sh` - **CREATED**
2. `test/scripts/register-multi-backend.sh` - **CREATED** (optional utility)
3. `run-all-tests.sh` - **UPDATED**
4. `test/run-github-action-tests.sh` - **UPDATED**
5. `.github/workflows/gateway-tests.yml` - **UPDATED**

## Usage

### Local Testing

```bash
# Start containers
cd test
podman-compose up -d

# Wait for gateway health
curl http://localhost:9090/health

# Run tests (includes automatic backend re-registration)
cd ..
./run-all-tests.sh
```

### GitHub Actions

The workflow now automatically:
1. Starts all containers
2. Waits for initial backend registration
3. **Re-registers backends with multiple servers**
4. Verifies multi-server configuration
5. Runs all tests

## Future Improvements

1. **Consider updating docker-compose**: Modify backend containers to not auto-register individually, and instead use a single initialization container that registers all backends at once
2. **Add backend health checks**: Monitor server health and automatically re-register if servers go down
3. **Dynamic scaling**: Support adding/removing servers without replacing the entire backend

## Validation

To verify backend configuration:
```bash
# Check default frontend
curl -s http://localhost:9090/api/frontends/default/backends | jq

# Expected output:
# - api-backend: 3 servers
# - web-backend: 2 servers

# Check other frontends
curl -s http://localhost:9090/api/frontends/frontend-api/backends | jq  # api-v2-backend: 2 servers
curl -s http://localhost:9090/api/frontends/frontend-web/backends | jq  # web-v2-backend: 2 servers
```
