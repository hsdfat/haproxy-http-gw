#!/bin/bash
# Configure Gateway Routes
# This script adds routing rules to the gateway after it starts
# Supports multiple frontends

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

print_section() {
    echo -e "${BLUE}═══════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════${NC}"
}

# Gateway API endpoint
GATEWAY_API="${GATEWAY_API:-http://localhost:9090}"

print_section "Configuring Gateway Routes for All Frontends"
echo ""
print_info "Gateway API: $GATEWAY_API"

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

# Check if bypass_rules is enabled for frontends
print_info "Checking frontend routing configuration..."

# Check if all frontends have bypass_rules enabled
BYPASS_ENABLED=true
for frontend_id in default frontend-api frontend-web; do
    FRONTEND_INFO=$(curl -sf "$GATEWAY_API/api/frontends/$frontend_id" 2>/dev/null || echo "{}")
    if ! echo "$FRONTEND_INFO" | grep -q '"bypass_rules":true'; then
        BYPASS_ENABLED=false
        break
    fi
done

if [ "$BYPASS_ENABLED" = true ]; then
    print_success "All frontends have bypass_rules enabled - routing via default_backend only"
    print_info "Skipping ACL route configuration (not needed with bypass_rules=true)"
    echo ""
    print_success "All routes configured successfully for all frontends"
    exit 0
fi

print_info "Bypass mode not enabled - configuring ACL routing rules..."

# Function to add a route to a frontend
add_route() {
    local frontend_id=$1
    local host=$2
    local path=$3
    local backend=$4

    print_info "Adding route to '$frontend_id': $host$path -> $backend"
    response=$(curl -sf -X POST "$GATEWAY_API/api/frontends/$frontend_id/routes" \
        -H "Content-Type: application/json" \
        -d "{\"host\":\"$host\",\"path\":\"$path\",\"backend_name\":\"$backend\"}" 2>&1)

    if echo "$response" | grep -q '"success":true'; then
        print_success "Route added successfully"
        return 0
    else
        print_error "Failed to add route"
        echo "Response: $response"
        return 1
    fi
}

# Configure routes for default frontend
print_section "Frontend: default (port 8080)"
add_route "default" "127.0.0.1" "/healthz" "api-backend"
add_route "default" "127.0.0.1" "/" "web-backend"

echo ""
print_info "Routes for 'default' frontend:"
curl -s "$GATEWAY_API/api/frontends/default/routes" | python3 -m json.tool 2>/dev/null || curl -s "$GATEWAY_API/api/frontends/default/routes"

# Configure routes for frontend-api
echo ""
print_section "Frontend: frontend-api (port 8081)"
add_route "frontend-api" "" "/api" "api-v2-backend"
add_route "frontend-api" "" "/" "api-v2-backend"

echo ""
print_info "Routes for 'frontend-api' frontend:"
curl -s "$GATEWAY_API/api/frontends/frontend-api/routes" | python3 -m json.tool 2>/dev/null || curl -s "$GATEWAY_API/api/frontends/frontend-api/routes"

# Configure routes for frontend-web
echo ""
print_section "Frontend: frontend-web (port 8082)"
add_route "frontend-web" "" "/" "web-v2-backend"
add_route "frontend-web" "" "/static" "web-v2-backend"

echo ""
print_info "Routes for 'frontend-web' frontend:"
curl -s "$GATEWAY_API/api/frontends/frontend-web/routes" | python3 -m json.tool 2>/dev/null || curl -s "$GATEWAY_API/api/frontends/frontend-web/routes"

echo ""
print_success "All routes configured successfully for all frontends"
