# GitHub CI HAProxy Timeout Fix

## Problem
The GitHub CI test workflow was failing with HAProxy runtime socket timeout errors:
```
Starting HAProxy HTTP Gateway...
Waiting for HAProxy runtime socket...
Waiting for HAProxy runtime socket...
[repeated many times]
time="2025-12-05T13:35:25Z" level=warning msg="StopSignal SIGUSR1 failed to stop container http-gateway in 10 seconds, resorting to SIGKILL"
```

**Root Cause**: The entrypoint script was waiting for HAProxy's runtime socket, but HAProxy itself was never started.

## Solution Overview

The fix implements proper HAProxy startup sequence:
1. Start HAProxy process in master-worker mode
2. Wait for runtime socket to be created (with timeout)
3. Start the http-gateway application

## Files Modified

### 1. test/haproxy-init.cfg (NEW)
Created minimal HAProxy configuration for initial startup:
- Uses `/tmp/haproxy-gateway` for sockets (user-writable directory)
- Configures master-worker mode
- Creates runtime API socket
- Provides minimal placeholder frontend/backend

### 2. test/scripts/gateway-entrypoint.sh
**Before**: Only waited for socket (which never appeared)
```bash
until [ -S /var/run/haproxy-runtime-api.sock ]; do
    echo "Waiting for HAProxy runtime socket..."
    sleep 1
done
```

**After**: Starts HAProxy, then waits for socket with timeout
```bash
# Start HAProxy
/usr/local/sbin/haproxy -f /etc/haproxy/haproxy.cfg -W -db &

# Wait for socket with 30-second timeout
until [ -S "$ACTUAL_SOCKET" ]; do
    RETRY_COUNT=$((RETRY_COUNT + 1))
    if [ $RETRY_COUNT -ge $MAX_RETRIES ]; then
        echo "Error: HAProxy runtime socket not available"
        exit 1
    fi
    sleep 1
done

# Export socket path for application
export HAPROXY_RUNTIME_SOCKET="$ACTUAL_SOCKET"
exec /usr/local/bin/http-gateway
```

### 3. test/Dockerfile.gateway
Added copying of initial HAProxy config:
```dockerfile
COPY test/haproxy-init.cfg /etc/haproxy/haproxy.cfg
COPY test/scripts/gateway-entrypoint.sh /entrypoint.sh
```

### 4. test/docker-compose.yml
Removed `/var/run` volume mount to avoid permission conflicts:
```yaml
volumes:
  - ./certs:/etc/haproxy/certs:ro
  - haproxy-config:/tmp/haproxy-gateway
  # Removed: - haproxy-runtime:/var/run
```

### 5. cmd/http-gateway/main.go
Added environment variable support for socket path:
```go
// Get runtime socket from environment or use default
runtimeSocket := os.Getenv("HAPROXY_RUNTIME_SOCKET")
if runtimeSocket == "" {
    runtimeSocket = "/var/run/haproxy-runtime-api.sock"
}
logger.Infof("Using HAProxy runtime socket: %s", runtimeSocket)
```

## Technical Details

### Socket Path Strategy
- **HAProxy creates socket at**: `/tmp/haproxy-gateway/haproxy-runtime-api.sock`
- **Application expects socket at**: `/var/run/haproxy-runtime-api.sock`
- **Solution**: Export `HAPROXY_RUNTIME_SOCKET` environment variable in entrypoint

### Permission Handling
- `/var/run` is a symlink to `/run` owned by root
- `/tmp/haproxy-gateway` is owned by `haproxy` user (created in Dockerfile)
- Entrypoint attempts symlink creation but gracefully handles permission denied

### Timeout Protection
- Maximum 30 retries (30 seconds) waiting for socket
- Provides diagnostic output on failure
- Prevents infinite loops that caused CI timeout

## Verification

Local test results:
```
✅ HAProxy starts: [NOTICE] (2) : Loading success.
✅ Socket created: haproxy-runtime-api.sock (< 2 seconds)
✅ API functional: echo 'show info' | socat - socket
✅ Container stable: No restart loops
```

## CI Compatibility

This fix aligns with how the original HAProxy Ingress Controller project handles HAProxy:
- Uses S6 overlay for process supervision in production
- Our test environment simplified: direct process start
- Socket path configurable via environment (production-ready pattern)
- HAProxy config structure matches project conventions

## Additional Notes

1. **Certs Directory**: Set permissions to 755 for HAProxy to read SSL certs:
   ```bash
   chmod -R 755 test/certs
   ```

2. **Application Errors**: The http-gateway application may show configuration errors (missing backends, SSL certs) - these are expected and separate from the infrastructure fix.

3. **Docker vs Podman**: Solution works with both container runtimes.

## Testing

To test locally:
```bash
cd test
chmod -R 755 certs
podman-compose down -v
podman-compose build
podman-compose up -d

# Verify
podman ps | grep gateway
podman logs http-gateway | head -20
podman exec http-gateway ps aux | grep haproxy
```

Expected output:
- Container running and healthy
- "HAProxy is ready" message in logs within 2 seconds
- HAProxy master and worker processes visible
- Runtime socket exists at `/tmp/haproxy-gateway/haproxy-runtime-api.sock`
