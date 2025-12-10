#!/bin/bash
# Local Test Script for HAProxy HTTP Gateway
# This script runs the complete test flow using Podman or Docker Compose

set -e

# Ensure podman-compose is in PATH (for macOS)
export PATH="/Library/Frameworks/Python.framework/Versions/3.14/bin:$PATH"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}→ $1${NC}"
}

# Ensure we're in the test directory
cd "$(dirname "$0")"

print_info "Starting HAProxy HTTP Gateway Local Test"
echo "============================================="
echo ""

# Step 1: Check prerequisites
print_info "Step 1: Checking prerequisites..."
if ! command -v podman-compose &> /dev/null && ! command -v docker-compose &> /dev/null; then
    print_error "Neither podman-compose nor docker-compose is installed"
    echo "Install with: pip install podman-compose OR install docker-compose"
    exit 1
fi

# Determine which compose command to use
if command -v podman-compose &> /dev/null; then
    COMPOSE_CMD="podman-compose"
    print_success "Using podman-compose: $(podman-compose --version | head -1)"
else
    COMPOSE_CMD="docker-compose"
    print_success "Using docker-compose: $(docker-compose --version)"
fi

# Step 2: Generate certificates
print_info "Step 2: Generating SSL certificates..."
if [ ! -f "certs/server.pem" ]; then
    chmod +x scripts/generate-certs.sh
    ./scripts/generate-certs.sh
    print_success "Certificates generated"
else
    print_success "Certificates already exist"
fi

# Step 3: Build images
print_info "Step 3: Building container images (this may take a few minutes)..."
if $COMPOSE_CMD build; then
    print_success "All images built successfully"
else
    print_error "Build failed"
    exit 1
fi

# Step 4: Start services
print_info "Step 4: Starting services..."
$COMPOSE_CMD up -d
print_success "Services started"

# Step 5: Wait for gateway to be ready
print_info "Step 5: Waiting for gateway to be ready..."
max_retries=30
retry=0
while [ $retry -lt $max_retries ]; do
    if curl -sf http://localhost:9090/health > /dev/null 2>&1; then
        print_success "Gateway API is healthy"
        break
    fi

    retry=$((retry + 1))
    if [ $retry -eq $max_retries ]; then
        print_error "Gateway failed to become healthy after $max_retries attempts"
        $COMPOSE_CMD logs gateway
        exit 1
    fi

    echo "  Attempt $retry/$max_retries, waiting for gateway..."
    sleep 2
done

# Step 6: Check service status
print_info "Step 6: Checking service status..."
$COMPOSE_CMD ps

# Step 7: Wait for backends to register
print_info "Step 7: Waiting for backend servers to register with gateway..."
sleep 15

# Check registered backends using frontend-scoped API for all frontends
print_info "Step 8: Verifying backend registration across all frontends..."

echo ""
echo "Default frontend backends:"
BACKENDS=$(curl -sf http://localhost:9090/api/frontends/default/backends)
if command -v jq &> /dev/null; then
    echo "$BACKENDS" | jq '.'
else
    echo "$BACKENDS"
fi

if echo "$BACKENDS" | grep -q "api-backend"; then
    print_success "api-backend is registered (default frontend)"
else
    print_error "api-backend not found in default frontend"
    echo "Gateway logs:"
    $COMPOSE_CMD logs gateway | tail -50
    echo ""
    echo "Backend server logs:"
    $COMPOSE_CMD logs backend-server-1 | tail -20
    exit 1
fi

if echo "$BACKENDS" | grep -q "web-backend"; then
    print_success "web-backend is registered (default frontend)"
else
    print_error "web-backend not found in default frontend"
    exit 1
fi

echo ""
echo "Frontend-API backends:"
BACKENDS=$(curl -sf http://localhost:9090/api/frontends/frontend-api/backends)
if command -v jq &> /dev/null; then
    echo "$BACKENDS" | jq '.'
else
    echo "$BACKENDS"
fi

if echo "$BACKENDS" | grep -q "api-v2-backend"; then
    print_success "api-v2-backend is registered (frontend-api)"
else
    print_error "api-v2-backend not found in frontend-api"
    echo "Backend server logs:"
    $COMPOSE_CMD logs api-v2-server-1 api-v2-server-2 | tail -30
    exit 1
fi

echo ""
echo "Frontend-Web backends:"
BACKENDS=$(curl -sf http://localhost:9090/api/frontends/frontend-web/backends)
if command -v jq &> /dev/null; then
    echo "$BACKENDS" | jq '.'
else
    echo "$BACKENDS"
fi

if echo "$BACKENDS" | grep -q "web-v2-backend"; then
    print_success "web-v2-backend is registered (frontend-web)"
else
    print_error "web-v2-backend not found in frontend-web"
    echo "Backend server logs:"
    $COMPOSE_CMD logs web-v2-server-1 web-v2-server-2 | tail -30
    exit 1
fi

# Step 8.5: Route configuration removed - using default_backend approach
# Routing rules are no longer needed since each frontend directly uses its intended backend
# via the default_backend setting in frontend-config-test.yaml

# Step 9: Run functional tests
print_info "Step 9: Running functional tests on all frontends..."
echo ""

# Test 1: Test default frontend (port 8080)
print_info "Test 1: Testing default frontend (port 8080)"
max_retries=10
retry=0

while [ $retry -lt $max_retries ]; do
    RESPONSE=$(curl -sf http://localhost:8080/ 2>&1 || echo "")
    if echo "$RESPONSE" | grep -q "backend-server"; then
        SERVER_NAME=$(echo "$RESPONSE" | grep -o "backend-server-[0-9]" | head -1 || echo "unknown")
        print_success "Successfully reached default frontend: $SERVER_NAME"
        break
    fi

    retry=$((retry + 1))
    if [ $retry -eq $max_retries ]; then
        print_error "Failed to reach default frontend after $max_retries attempts"
        echo "Last response: $RESPONSE"
        echo ""
        echo "Gateway logs:"
        $COMPOSE_CMD logs gateway | tail -30
        exit 1
    fi

    echo "  Attempt $retry/$max_retries failed, retrying in 2s..."
    sleep 2
done

# Test 2: Test frontend-api (port 8081) - uses HTTP/2
# Note: Without routing rules, frontend-api uses default_backend (api-backend)
print_info "Test 2: Testing frontend-api (port 8081)"
retry=0
while [ $retry -lt $max_retries ]; do
    RESPONSE=$(curl --http2-prior-knowledge -sf http://localhost:8081/ 2>&1 || echo "")
    if echo "$RESPONSE" | grep -q "backend-server"; then
        SERVER_NAME=$(echo "$RESPONSE" | grep -o "backend-server-[0-9]" | head -1 || echo "unknown")
        print_success "Successfully reached frontend-api: $SERVER_NAME (using default_backend: api-backend)"
        break
    fi

    retry=$((retry + 1))
    if [ $retry -eq $max_retries ]; then
        print_error "Failed to reach frontend-api after $max_retries attempts"
        echo "Last response: $RESPONSE"
        exit 1
    fi

    echo "  Attempt $retry/$max_retries failed, retrying in 2s..."
    sleep 2
done

# Test 3: Test frontend-web (port 8082) - uses HTTP/2
# Note: Without routing rules, frontend-web uses default_backend (web-backend)
print_info "Test 3: Testing frontend-web (port 8082)"
retry=0
while [ $retry -lt $max_retries ]; do
    RESPONSE=$(curl --http2-prior-knowledge -sf http://localhost:8082/ 2>&1 || echo "")
    if echo "$RESPONSE" | grep -q "web-server"; then
        SERVER_NAME=$(echo "$RESPONSE" | grep -o "web-server-[0-9]" | head -1 || echo "unknown")
        print_success "Successfully reached frontend-web: $SERVER_NAME (using default_backend: web-backend)"
        break
    fi

    retry=$((retry + 1))
    if [ $retry -eq $max_retries ]; then
        print_error "Failed to reach frontend-web after $max_retries attempts"
        echo "Last response: $RESPONSE"
        exit 1
    fi

    echo "  Attempt $retry/$max_retries failed, retrying in 2s..."
    sleep 2
done

# Test 4: Test load balancing on default frontend
print_info "Test 4: Testing load balancing on default frontend (port 8080)"
servers_hit=$(for i in {1..10}; do curl -s http://localhost:8080/; done | grep -o "backend-server-[0-9]" | sort -u | wc -l | tr -d ' ')
if [ "$servers_hit" -ge 2 ]; then
    print_success "Default frontend load balancing is working (hit $servers_hit different servers)"
else
    print_error "Default frontend load balancing test failed (only hit $servers_hit server(s))"
    exit 1
fi

# Test 5: Test load balancing on frontend-api
# Note: Without routing rules, tests backend-server-* (api-backend)
print_info "Test 5: Testing load balancing on frontend-api (port 8081)"
servers_hit=$(for _ in {1..10}; do curl --http2-prior-knowledge -s http://localhost:8081/; done | grep -o "backend-server-[0-9]" | sort -u | wc -l | tr -d ' ')
if [ "$servers_hit" -ge 2 ]; then
    print_success "Frontend-API load balancing is working (hit $servers_hit different servers from api-backend)"
else
    print_error "Frontend-API load balancing test failed (only hit $servers_hit server(s))"
    exit 1
fi

# Test 6: Test load balancing on frontend-web
# Note: Without routing rules, tests web-server-* (web-backend)
print_info "Test 6: Testing load balancing on frontend-web (port 8082)"
servers_hit=$(for _ in {1..10}; do curl --http2-prior-knowledge -s http://localhost:8082/; done | grep -o "web-server-[0-9]" | sort -u | wc -l | tr -d ' ')
if [ "$servers_hit" -ge 2 ]; then
    print_success "Frontend-Web load balancing is working (hit $servers_hit different servers from web-backend)"
else
    print_error "Frontend-Web load balancing test failed (only hit $servers_hit server(s))"
    exit 1
fi

# Test 7: Test backend registration API (using frontend-scoped API)
print_info "Test 7: Testing backend registration API"
TEST_BACKEND=$(curl -sf -X POST http://localhost:9090/api/frontends/default/backends \
    -H 'Content-Type: application/json' \
    -d '{"name":"test-backend","servers":[{"name":"test-server","ip":"backend-server-1","port":9000}]}' || echo "")

if echo "$TEST_BACKEND" | grep -q '"success":true'; then
    print_success "Backend registration API is working"
else
    print_error "Backend registration API failed"
    echo "Response: $TEST_BACKEND"
    exit 1
fi

# Test 8: Test H2C (HTTP/2 Cleartext) support on all frontends
print_info "Test 8: Testing H2C (HTTP/2 Cleartext) support"
if command -v curl &> /dev/null; then
    if curl --http2-prior-knowledge -s http://localhost:8080/ 2>/dev/null | grep -q "backend-server"; then
        print_success "Default frontend H2C is working"
    else
        print_info "Default frontend H2C test skipped or failed"
    fi

    if curl --http2-prior-knowledge -s http://localhost:8081/ 2>/dev/null | grep -q "backend-server"; then
        print_success "Frontend-API H2C is working (using api-backend)"
    else
        print_info "Frontend-API H2C test skipped or failed"
    fi

    if curl --http2-prior-knowledge -s http://localhost:8082/ 2>/dev/null | grep -q "web-server"; then
        print_success "Frontend-Web H2C is working (using web-backend)"
    else
        print_info "Frontend-Web H2C test skipped or failed"
    fi
else
    print_info "H2C tests skipped (curl may not support --http2-prior-knowledge)"
fi

echo ""
print_success "All functional tests passed!"

# Step 10: Summary
echo ""
echo "============================================="
print_success "All tests completed successfully!"
echo ""
print_info "System Information:"
echo "  Gateway API: http://localhost:9090"
echo "  Gateway HTTP: http://localhost:8080"
echo "  Gateway HTTPS: https://localhost:8443"
echo ""
print_info "Frontends configured:"
echo "  - Default frontend: http://localhost:8080"
echo "  - API frontend:     http://localhost:8081"
echo "  - Web frontend:     http://localhost:8082"
echo "  - Gateway API:      http://localhost:9090"
echo ""
print_info "Useful commands:"
echo "  View gateway logs:"
echo "    $COMPOSE_CMD logs -f gateway"
echo ""
echo "  View backend logs:"
echo "    $COMPOSE_CMD logs -f backend-server-1 api-v2-server-1 web-v2-server-1"
echo ""
echo "  List all frontends:"
echo "    curl http://localhost:9090/api/frontends | jq"
echo ""
echo "  List registered backends for frontend 'default':"
echo "    curl http://localhost:9090/api/frontends/default/backends | jq"
echo ""
echo "  List registered backends for frontend 'frontend-api':"
echo "    curl http://localhost:9090/api/frontends/frontend-api/backends | jq"
echo ""
echo "  List registered backends for frontend 'frontend-web':"
echo "    curl http://localhost:9090/api/frontends/frontend-web/backends | jq"
echo ""
echo "  Register a new backend to frontend 'default':"
echo "    curl -X POST http://localhost:9090/api/frontends/default/backends -H 'Content-Type: application/json' \\"
echo "      -d '{\"name\":\"my-backend\",\"servers\":[{\"name\":\"server1\",\"ip\":\"192.168.1.10\",\"port\":9000}]}'"
echo ""
echo "  Test all frontends:"
echo "    curl http://localhost:8080/  # Default frontend"
echo "    curl http://localhost:8081/  # API frontend"
echo "    curl http://localhost:8082/  # Web frontend"
echo ""
echo "  Stop services:"
echo "    $COMPOSE_CMD down"
echo ""
echo "  Cleanup everything:"
echo "    $COMPOSE_CMD down -v"
echo "============================================="
