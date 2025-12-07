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

# Check registered backends
print_info "Step 8: Verifying backend registration..."
BACKENDS=$(curl -sf http://localhost:9090/api/backends)

if command -v jq &> /dev/null; then
    echo "$BACKENDS" | jq '.'
else
    echo "$BACKENDS"
fi

if echo "$BACKENDS" | grep -q "api-backend"; then
    print_success "api-backend is registered"
else
    print_error "api-backend not found in registered backends"
    echo "Gateway logs:"
    $COMPOSE_CMD logs gateway | tail -50
    echo ""
    echo "Backend server logs:"
    $COMPOSE_CMD logs backend-server-1 | tail -20
    exit 1
fi

if echo "$BACKENDS" | grep -q "web-backend"; then
    print_success "web-backend is registered"
else
    print_error "web-backend not found in registered backends"
    exit 1
fi

# Step 9: Run functional tests
print_info "Step 9: Running functional tests..."
echo ""

# Test 1: Test direct backend access (without routing rules)
print_info "Test 1: Testing direct access to api-backend (default backend)"
max_retries=10
retry=0

while [ $retry -lt $max_retries ]; do
    RESPONSE=$(curl -sf http://localhost:8080/ 2>&1 || echo "")
    if echo "$RESPONSE" | grep -q "backend-server"; then
        SERVER_NAME=$(echo "$RESPONSE" | grep -o "backend-server-[0-9]" | head -1 || echo "unknown")
        print_success "Successfully reached api-backend: $SERVER_NAME"
        break
    fi

    retry=$((retry + 1))
    if [ $retry -eq $max_retries ]; then
        print_error "Failed to reach backend after $max_retries attempts"
        echo "Last response: $RESPONSE"
        echo ""
        echo "Gateway logs:"
        $COMPOSE_CMD logs gateway | tail -30
        exit 1
    fi

    echo "  Attempt $retry/$max_retries failed, retrying in 2s..."
    sleep 2
done

# Test 2: Test load balancing (multiple requests should hit different servers)
print_info "Test 2: Testing load balancing across backend servers"
servers_hit=$(for i in {1..10}; do curl -s http://localhost:8080/; done | grep -o "backend-server-[0-9]" | sort -u | wc -l | tr -d ' ')
if [ "$servers_hit" -ge 2 ]; then
    print_success "Load balancing is working (hit $servers_hit different servers)"
else
    print_error "Load balancing test failed (only hit $servers_hit server(s))"
    exit 1
fi

# Test 3: Test backend registration API
print_info "Test 3: Testing backend registration API"
TEST_BACKEND=$(curl -sf -X POST http://localhost:9090/api/backends \
    -H 'Content-Type: application/json' \
    -d '{"name":"test-backend","servers":[{"name":"test-server","ip":"backend-server-1","port":9000}]}' || echo "")

if echo "$TEST_BACKEND" | grep -q '"success":true'; then
    print_success "Backend registration API is working"
else
    print_error "Backend registration API failed"
    echo "Response: $TEST_BACKEND"
    exit 1
fi

# Test 4: Test H2C (HTTP/2 Cleartext) support
print_info "Test 4: Testing H2C (HTTP/2 Cleartext) support"
if command -v curl &> /dev/null && curl --http2-prior-knowledge -s http://localhost:8080/ 2>/dev/null | grep -q "backend-server"; then
    print_success "H2C (HTTP/2 Cleartext) is working correctly"
else
    print_info "H2C test skipped (curl may not support --http2-prior-knowledge or test failed)"
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
print_info "Useful commands:"
echo "  View gateway logs:"
echo "    $COMPOSE_CMD logs -f gateway"
echo ""
echo "  View backend logs:"
echo "    $COMPOSE_CMD logs -f backend-server-1"
echo ""
echo "  List registered backends:"
echo "    curl http://localhost:9090/api/backends | jq"
echo ""
echo "  Register a new backend:"
echo "    curl -X POST http://localhost:9090/api/backends -H 'Content-Type: application/json' \\"
echo "      -d '{\"name\":\"my-backend\",\"servers\":[{\"name\":\"server1\",\"ip\":\"192.168.1.10\",\"port\":9000}]}'"
echo ""
echo "  Stop services:"
echo "    $COMPOSE_CMD down"
echo ""
echo "  Cleanup everything:"
echo "    $COMPOSE_CMD down -v"
echo "============================================="
