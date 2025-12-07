#!/bin/bash
# Backend Server Entrypoint
# This script registers the backend with the gateway and then starts the server

set -e

# Configuration from environment
GATEWAY_URL="${GATEWAY_URL:-http://gateway:9090}"
BACKEND_NAME="${BACKEND_NAME:-api-backend}"
SERVER_NAME="${SERVER_NAME:-$(hostname)}"
SERVER_PORT="${SERVER_PORT:-9000}"

# Auto-detect IP address if not provided
if [ -z "$SERVER_IP" ]; then
    # Try to get IP from eth0 interface (common in Docker/Podman)
    SERVER_IP=$(ip addr show eth0 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 || true)

    # Fallback to hostname -i if eth0 not found
    if [ -z "$SERVER_IP" ]; then
        SERVER_IP=$(hostname -i 2>/dev/null | awk '{print $1}' || true)
    fi

    # Last resort: use hostname
    if [ -z "$SERVER_IP" ]; then
        SERVER_IP=$(hostname)
    fi
fi

echo "========================================="
echo "Backend Server Starting"
echo "========================================="
echo "Server Name: $SERVER_NAME"
echo "Detected IP: $SERVER_IP"
echo "Server Port: $SERVER_PORT"
echo "Backend Name: $BACKEND_NAME"
echo "Gateway URL: $GATEWAY_URL"
echo "========================================="
echo ""

# Start the backend server in the background
echo "Starting backend server..."
"$@" &
SERVER_PID=$!

# Wait a moment for server to start
sleep 2

# Register with gateway
echo "Registering with gateway..."
BACKEND_NAME="$BACKEND_NAME" \
SERVER_NAME="$SERVER_NAME" \
SERVER_IP="$SERVER_IP" \
SERVER_PORT="$SERVER_PORT" \
GATEWAY_URL="$GATEWAY_URL" \
MAX_RETRIES=30 \
/register-backend.sh || {
    echo "Warning: Failed to register with gateway, but continuing..."
}

echo ""
echo "Backend server is running and registered!"
echo "========================================="

# Wait for server process
wait $SERVER_PID
