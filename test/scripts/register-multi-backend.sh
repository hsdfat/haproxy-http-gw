#!/bin/bash
# Multi-Server Backend Registration Script
# This script registers a backend with multiple servers at once

set -e

# Configuration
GATEWAY_URL="${GATEWAY_URL:-http://gateway:9090}"
FRONTEND_ID="${FRONTEND_ID:-default}"
BACKEND_NAME="${BACKEND_NAME:-}"
SERVERS_JSON="${SERVERS_JSON:-}"  # JSON array of servers
MAX_RETRIES="${MAX_RETRIES:-30}"
RETRY_DELAY="${RETRY_DELAY:-2}"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

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
    exit 1
fi

if [ -z "$SERVERS_JSON" ]; then
    print_error "SERVERS_JSON environment variable is required"
    exit 1
fi

print_info "Multi-Server Backend Registration Configuration:"
echo "  Gateway URL: $GATEWAY_URL"
echo "  Frontend ID: $FRONTEND_ID"
echo "  Backend Name: $BACKEND_NAME"
echo "  Servers: $SERVERS_JSON"
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

# Register backend with multiple servers
print_info "Registering backend '$BACKEND_NAME' with multiple servers to frontend '$FRONTEND_ID'..."

PAYLOAD=$(jq -n \
    --arg name "$BACKEND_NAME" \
    --argjson servers "$SERVERS_JSON" \
    '{name: $name, servers: $servers}')

RESPONSE=$(curl -sf -X POST "$GATEWAY_URL/api/frontends/$FRONTEND_ID/backends" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD" || echo "")

# Check response
if echo "$RESPONSE" | grep -q '"success":true'; then
    print_success "Backend '$BACKEND_NAME' registered successfully with multiple servers!"
    SERVER_COUNT=$(echo "$SERVERS_JSON" | jq 'length')
    print_info "$SERVER_COUNT servers registered to backend '$BACKEND_NAME'"
else
    print_error "Failed to register backend"
    echo "Payload: $PAYLOAD"
    echo "Response: $RESPONSE"
    exit 1
fi
