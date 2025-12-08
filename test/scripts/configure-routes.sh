#!/bin/bash
# Configure Gateway Routes
# This script adds routing rules to the gateway after it starts

set -e

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

# Gateway API endpoint
GATEWAY_API="${GATEWAY_API:-http://localhost:9090}"
FRONTEND_ID="${FRONTEND_ID:-default}"

print_info "Configuring gateway routes via API at $GATEWAY_API for frontend '$FRONTEND_ID'"

# Wait for API to be ready
print_info "Waiting for Gateway API to be ready..."
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    if curl -sf "$GATEWAY_API/health" > /dev/null 2>&1; then
        print_success "Gateway API is ready"
        break
    fi
    attempt=$((attempt + 1))
    if [ $attempt -eq $max_attempts ]; then
        print_error "Gateway API did not become ready in time"
        exit 1
    fi
    sleep 1
done

# Add route for api.example.com/api -> api-backend
print_info "Adding route: api.example.com/api -> api-backend"
response=$(curl -sf -X POST "$GATEWAY_API/api/frontends/$FRONTEND_ID/routes" \
    -H "Content-Type: application/json" \
    -d '{"host":"127.0.0.1","path":"/healthz","backend":"api-backend"}' 2>&1)

if echo "$response" | grep -q '"success":true'; then
    print_success "API route added successfully"
else
    print_error "Failed to add API route"
    echo "Response: $response"
    exit 1
fi

# Add route for www.example.com/ -> web-backend
print_info "Adding route: www.example.com/ -> web-backend"
response=$(curl -sf -X POST "$GATEWAY_API/api/frontends/$FRONTEND_ID/routes" \
    -H "Content-Type: application/json" \
    -d '{"host":"127.0.0.1","path":"/","backend":"web-backend"}' 2>&1)

if echo "$response" | grep -q '"success":true'; then
    print_success "Web route added successfully"
else
    print_error "Failed to add web route"
    echo "Response: $response"
    exit 1
fi

# List all routes
print_info "Current routes configuration:"
curl -s "$GATEWAY_API/api/frontends/$FRONTEND_ID/routes" | python3 -m json.tool 2>/dev/null || curl -s "$GATEWAY_API/api/frontends/$FRONTEND_ID/routes"

print_success "All routes configured successfully"
