# Dynamic Backend Registration/Deregistration Tests

This directory contains comprehensive test scripts for verifying dynamic backend management in the HAProxy HTTP Gateway.

## Test Scripts

### 1. `test-dynamic-backends-standalone.sh`
Standalone test script that runs against a simple local HAProxy gateway container.

**Use Case:** Quick testing during development without requiring full test infrastructure.

**Features:**
- Tests single and multiple server backends
- Verifies via 3 methods: API, config file, and runtime socket
- Multiple rapid register/deregister cycles
- Concurrent backend registration
- Re-registration with different configurations

### 2. `test-dynamic-backends.sh`
Integrated test script for use within the full test suite (podman-compose).

**Use Case:** Comprehensive testing with backend servers available.

## Prerequisites

### For Standalone Testing
1. HAProxy HTTP Gateway container running
2. Gateway API accessible (default: `http://localhost:9090`)
3. Container runtime (podman or docker)
4. `curl`, `bc`, and `socat` installed

### For Integrated Testing
1. Full test environment running via podman-compose
2. Backend servers available (backend-server-1, backend-server-2, backend-server-3)

## Quick Start - Standalone Test

### Step 1: Start the Gateway

```bash
cd test
podman-compose up -d gateway
```

Or if you have the gateway running elsewhere, just ensure it's accessible.

### Step 2: Run the Standalone Test

```bash
cd test/scripts
./test-dynamic-backends-standalone.sh
```

### Step 3: Customize Configuration (Optional)

You can customize the test via environment variables:

```bash
# Change gateway API URL
export GATEWAY_API=http://localhost:9090

# Change frontend ID to test
export FRONTEND_ID=default

# Change container runtime
export CONTAINER_RUNTIME=podman  # or docker

# Change gateway container name
export GATEWAY_CONTAINER=test-gateway-1

# Run the test
./test-dynamic-backends-standalone.sh
```

## Test Coverage

The standalone test performs **5 test cycles** with multiple verification steps:

### Cycle 1: Single Server Backend
- Register backend with 1 server
- Verify via API, config file, and runtime socket
- Deregister backend
- Verify removal via all 3 methods

**Total: 8 tests**

### Cycle 2: Multiple Server Backend
- Register backend with 3 servers
- Verify via API, config file, and runtime socket
- Deregister backend
- Verify removal via all 3 methods

**Total: 8 tests**

### Cycle 3: Rapid Register/Deregister
- 5 rapid cycles of register → verify → deregister → verify
- Uses API and socket verification (faster than config file)

**Total: 20 tests (4 per cycle × 5 cycles)**

### Cycle 4: Concurrent Multiple Backends
- Register 3 backends concurrently
- Verify all 3 exist (via API and socket)
- Deregister all 3
- Verify all 3 removed (via API and socket)

**Total: 12 tests**

### Cycle 5: Re-registration with Different Configuration
- Register backend with 1 server
- Deregister
- Re-register same backend name with 3 servers
- Verify new configuration
- Cleanup

**Total: 5 tests**

**Grand Total: ~53 verification tests**

## Verification Methods

### 1. API Verification
Queries the Gateway REST API to list backends:
```bash
curl http://localhost:9090/api/frontends/default/backends
```

### 2. Config File Verification
Reads HAProxy configuration file directly from the container:
```bash
podman exec test-gateway-1 cat /etc/haproxy/haproxy.cfg
```

### 3. Runtime Socket Verification
Queries HAProxy runtime socket for live backend status:
```bash
echo 'show backend' | socat stdio /var/run/haproxy-runtime-api.sock
echo 'show servers state <backend>' | socat stdio /var/run/haproxy-runtime-api.sock
```

## Example Output

```
═══════════════════════════════════════════
  Dynamic Backend Registration/Deregistration Tests
═══════════════════════════════════════════

→ Test Configuration:
  Gateway API: http://localhost:9090
  Frontend ID: default
  Container Runtime: podman
  Gateway Container: test-gateway-1

→ Checking prerequisites...
✓ Gateway API is accessible
✓ Gateway container is running

═══════════════════════════════════════════
  Test Cycle 1: Single Server Backend
═══════════════════════════════════════════

--- 1.1: Register backend 'test-backend-1' with single server
→ Registering backend 'test-backend-1' with server 'srv1' (127.0.0.1:8080)...
✓ Backend 'test-backend-1' registered successfully
Response: {"success":true}

--- 1.2: Verify backend exists via API
→ Verifying backend 'test-backend-1' via API (should_exist=true)...
✓ Backend 'test-backend-1' found in API response

--- 1.3: Verify backend exists in config file
→ Verifying backend 'test-backend-1' in HAProxy config file (should_exist=true)...
✓ Backend 'test-backend-1' found in config file
backend test-backend-1
    balance roundrobin
    server srv1 127.0.0.1:8080 check

[... more tests ...]

═══════════════════════════════════════════
  Test Results Summary
═══════════════════════════════════════════

Total Tests:  53
Passed Tests: 53
Failed Tests: 0

Success Rate: 100.00%

═══════════════════════════════════════════
  ✅ ALL TESTS PASSED
═══════════════════════════════════════════
```

## Troubleshooting

### Gateway API not accessible
```bash
# Check if gateway is running
podman ps | grep gateway

# Check gateway logs
podman logs test-gateway-1

# Test API manually
curl http://localhost:9090/health
```

### Container not found
```bash
# List running containers
podman ps

# Update container name
export GATEWAY_CONTAINER=your-gateway-container-name
```

### Socket not accessible
```bash
# Verify socket exists in container
podman exec test-gateway-1 ls -la /var/run/haproxy-runtime-api.sock

# Check if socat is installed in container
podman exec test-gateway-1 which socat
```

## Integration with CI/CD

You can integrate this test into your CI/CD pipeline:

```yaml
# GitHub Actions example
- name: Run Dynamic Backend Tests
  run: |
    cd test/scripts
    ./test-dynamic-backends-standalone.sh
  env:
    GATEWAY_API: http://localhost:9090
    GATEWAY_CONTAINER: gateway
```

## Architecture Notes

The test validates the following architectural components:

1. **API Layer**: REST API endpoints for backend registration/deregistration
2. **Manager Layer**: Backend event processing and state management
3. **HAProxy Integration**: Configuration updates via Data Plane API
4. **Runtime Updates**: Socket-based live configuration changes

This ensures that:
- Backends can be dynamically added/removed without downtime
- Configuration changes persist in HAProxy config file
- Runtime state matches expected configuration
- Multiple concurrent operations are handled correctly
- Re-registration scenarios work properly

## Related Documentation

- [Backend Registration Script](register-backend.sh) - Used by backend services
- [Gateway Manager](../../pkg/gateway/manager.go) - Backend management logic
- [HAProxy API](../../pkg/haproxy/api/) - HAProxy integration
