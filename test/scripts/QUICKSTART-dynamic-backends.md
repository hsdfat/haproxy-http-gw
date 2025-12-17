# Quick Start: Dynamic Backend Testing

## Run the Test in 3 Steps

### 1. Start Gateway (if not already running)
```bash
cd test
podman-compose up -d gateway
```

Wait for gateway to be ready:
```bash
# Check health
curl http://localhost:9090/health

# Should return: {"status":"healthy"}
```

### 2. Run the Test
```bash
cd scripts
./test-dynamic-backends-standalone.sh
```

### 3. View Results
The test will show real-time progress and final summary:
- ✅ Green checkmarks = passed
- ❌ Red X marks = failed
- Final summary shows success rate

## Expected Output (Summary)
```
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

## What the Test Does

1. **Cycle 1**: Single backend server - register, verify, deregister
2. **Cycle 2**: Multiple backend servers (3) - register, verify, deregister
3. **Cycle 3**: Rapid cycles (5x) - stress test registration/deregistration
4. **Cycle 4**: Concurrent backends - test parallel operations
5. **Cycle 5**: Re-registration - test updating backend configuration

## Verification Methods

Each test cycle verifies backends via:
- **API**: HTTP REST endpoints
- **Config File**: HAProxy configuration file
- **Runtime Socket**: Live HAProxy runtime state

## Customize

```bash
# Different gateway URL
export GATEWAY_API=http://192.168.1.100:9090
./test-dynamic-backends-standalone.sh

# Different frontend
export FRONTEND_ID=frontend-api
./test-dynamic-backends-standalone.sh

# Different container name
export GATEWAY_CONTAINER=my-gateway
./test-dynamic-backends-standalone.sh
```

## Troubleshooting

**Gateway not accessible?**
```bash
podman ps | grep gateway
podman logs test-gateway-1
```

**Container name wrong?**
```bash
podman ps --format "table {{.Names}}\t{{.Status}}"
export GATEWAY_CONTAINER=actual-name
```

**Need full docs?**
See [README-dynamic-backend-tests.md](README-dynamic-backend-tests.md)
