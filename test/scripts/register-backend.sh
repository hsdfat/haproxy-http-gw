#!/bin/bash
# Backend Registration Script
# This script is used by backend servers to register themselves with the gateway

set -e

# Configuration
GATEWAY_URL="${GATEWAY_URL:-http://gateway:9090}"
FRONTEND_ID="${FRONTEND_ID:-default}"  # Frontend ID to register backend with
BACKEND_NAME="${BACKEND_NAME:-}"
SERVER_NAME="${SERVER_NAME:-$(hostname)}"
SERVER_IP="${SERVER_IP:-$(hostname)}"
SERVER_PORT="${SERVER_PORT:-9000}"
MAX_RETRIES="${MAX_RETRIES:-30}"
RETRY_DELAY="${RETRY_DELAY:-2}"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}→ $1${NC}"
}

# Validate required parameters
if [ -z "$BACKEND_NAME" ]; then
    print_error "BACKEND_NAME environment variable is required"
    echo "Usage: BACKEND_NAME=my-backend SERVER_NAME=server1 SERVER_IP=192.168.1.10 SERVER_PORT=9000 $0"
    exit 1
fi

print_info "Backend Registration Configuration:"
echo "  Gateway URL: $GATEWAY_URL"
echo "  Frontend ID: $FRONTEND_ID"
echo "  Backend Name: $BACKEND_NAME"
echo "  Server Name: $SERVER_NAME"
echo "  Server IP: $SERVER_IP"
echo "  Server Port: $SERVER_PORT"
echo ""

# Wait for gateway to be available
print_info "Waiting for gateway to be available..."
for i in $(seq 1 $MAX_RETRIES); do
    if curl -sf "$GATEWAY_URL/health" > /dev/null 2>&1; then
        print_success "Gateway is available"
        break
    fi

    if [ $i -eq $MAX_RETRIES ]; then
        print_error "Gateway did not become available after $MAX_RETRIES attempts"
        exit 1
    fi

    echo "  Attempt $i/$MAX_RETRIES failed, retrying in ${RETRY_DELAY}s..."
    sleep $RETRY_DELAY
done

# Register backend with frontend-scoped API
print_info "Registering backend '$BACKEND_NAME' with server '$SERVER_NAME' to frontend '$FRONTEND_ID'..."

# Register new backend with single server using frontend-scoped API
RESPONSE=$(curl -sf -X POST "$GATEWAY_URL/api/frontends/$FRONTEND_ID/backends" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$BACKEND_NAME\",\"servers\":[{\"name\":\"$SERVER_NAME\",\"ip\":\"$SERVER_IP\",\"port\":$SERVER_PORT}]}" || echo "")

# Check response
if echo "$RESPONSE" | grep -q '"success":true'; then
    print_success "Backend '$BACKEND_NAME' registered successfully!"
    print_info "Server '$SERVER_NAME' ($SERVER_IP:$SERVER_PORT) is now part of backend '$BACKEND_NAME'"
else
    print_error "Failed to register backend"
    echo "Response: $RESPONSE"
    exit 1
fi

# Keep script running to maintain registration (optional)
if [ "${KEEP_ALIVE:-false}" = "true" ]; then
    print_info "Running in keep-alive mode (send SIGTERM to exit)..."
    trap 'print_info "Received shutdown signal"; exit 0' TERM INT
    while true; do
        sleep 60
        # Could add periodic health check here
    done
fi
