#!/bin/sh

set -e

echo "Starting HAProxy HTTP Gateway..."

# Ensure config exists
if [ ! -f /etc/haproxy/haproxy.cfg ]; then
    echo "Error: HAProxy config not found at /etc/haproxy/haproxy.cfg"
    exit 1
fi

# Start HAProxy in master-worker mode with daemon
echo "Starting HAProxy..."
/usr/local/sbin/haproxy -f /etc/haproxy/haproxy.cfg -W -db &
HAPROXY_PID=$!
echo "HAProxy started with PID: $HAPROXY_PID"

# Wait for HAProxy runtime socket to be available
echo "Waiting for HAProxy runtime socket..."
ACTUAL_SOCKET="/tmp/haproxy-gateway/haproxy-runtime-api.sock"
EXPECTED_SOCKET="/var/run/haproxy-runtime-api.sock"
RETRY_COUNT=0
MAX_RETRIES=30
until [ -S "$ACTUAL_SOCKET" ]; do
    RETRY_COUNT=$((RETRY_COUNT + 1))
    if [ $RETRY_COUNT -ge $MAX_RETRIES ]; then
        echo "Error: HAProxy runtime socket not available after ${MAX_RETRIES} seconds"
        echo "HAProxy process status:"
        ps aux | grep haproxy || true
        echo "Socket directory contents:"
        ls -la /tmp/haproxy-gateway/ || true
        exit 1
    fi
    echo "Waiting for HAProxy runtime socket... (attempt $RETRY_COUNT/$MAX_RETRIES)"
    sleep 1
done

echo "HAProxy runtime socket is ready at $ACTUAL_SOCKET"

# Try to create symlink for http-gateway application compatibility (may fail due to permissions)
if ln -sf "$ACTUAL_SOCKET" "$EXPECTED_SOCKET" 2>/dev/null; then
    echo "Created symlink: $EXPECTED_SOCKET -> $ACTUAL_SOCKET"
else
    echo "Warning: Could not create symlink at $EXPECTED_SOCKET (permission denied)"
    echo "Using socket directly at $ACTUAL_SOCKET"
fi

echo "HAProxy is ready, starting HTTP Gateway application..."

# Start the HTTP Gateway (this will keep running in foreground)
# Export the actual socket path so the application can use it
export HAPROXY_RUNTIME_SOCKET="$ACTUAL_SOCKET"
exec /usr/local/bin/http-gateway
