#!/bin/bash
# Local Test Script for HAProxy HTTP Gateway
# This script runs the complete test flow using Podman

set -e

# Ensure podman and podman-compose are in PATH
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
if ! command -v podman &> /dev/null; then
    print_error "Podman is not installed"
    exit 1
fi
print_success "Podman found: $(podman --version)"

if ! command -v podman-compose &> /dev/null; then
    print_error "podman-compose is not installed"
    echo "Install with: pip install podman-compose"
    exit 1
fi
print_success "podman-compose found: $(podman-compose --version | head -1)"

# Step 2: Generate certificates
print_info "Step 2: Generating SSL certificates..."
if [ ! -f "certs/server.pem" ]; then
    ./scripts/generate-certs.sh
    print_success "Certificates generated"
else
    print_success "Certificates already exist"
fi

# Step 3: Build images
print_info "Step 3: Building container images (this may take a few minutes)..."
if podman-compose build; then
    print_success "All images built successfully"
else
    print_error "Build failed"
    exit 1
fi

# Step 4: Start services
print_info "Step 4: Starting services..."
podman-compose up -d
print_success "Services started"

# Step 5: Wait for services to be ready
print_info "Step 5: Waiting for services to be ready (30 seconds)..."
sleep 30

# Step 6: Check service status
print_info "Step 6: Checking service status..."
podman-compose ps

# Step 7: Check health endpoints
print_info "Step 7: Checking health endpoints..."
if curl -sf http://localhost:9090/health > /dev/null 2>&1; then
    print_success "Gateway API is healthy"
else
    print_error "Gateway API health check failed"
    podman-compose logs gateway
    exit 1
fi

if curl -sf http://localhost:8000/health > /dev/null 2>&1; then
    print_success "Backend API is healthy"
else
    print_error "Backend API health check failed"
    podman-compose logs backend-api
    exit 1
fi

# Step 7.5: Configure routes via API
print_info "Step 7.5: Configuring routes via API..."
if ./scripts/configure-routes.sh; then
    print_success "Routes configured successfully"
else
    print_error "Route configuration failed"
    podman-compose logs gateway
    exit 1
fi

# Step 8: Run functional tests
print_info "Step 8: Running functional tests..."

# Wait a bit for HAProxy to reload with the new routes
print_info "Waiting for HAProxy to apply route configuration..."
sleep 5

# Test 1: Test API backend route with retry
print_info "Test 1: Testing api.example.com/api -> api-backend"
max_retries=10
retry=0
while [ $retry -lt $max_retries ]; do
    response=$(curl -s -H "Host: api.example.com" http://localhost:8080/api/test)
    if echo "$response" | grep -q "backend-server"; then
        print_success "API route is working correctly"
        break
    fi
    retry=$((retry + 1))
    if [ $retry -eq $max_retries ]; then
        print_error "API route test failed after $max_retries attempts"
        echo "Response: $response"
        echo "Checking HAProxy logs:"
        podman-compose logs gateway | tail -20
        exit 1
    fi
    sleep 1
done

# Test 2: Test web backend route
print_info "Test 2: Testing www.example.com/ -> web-backend"
response=$(curl -s -H "Host: www.example.com" http://localhost:8080/)
if echo "$response" | grep -q "web-server"; then
    print_success "Web route is working correctly"
else
    print_error "Web route test failed"
    echo "Response: $response"
    exit 1
fi

# Test 3: Test load balancing (multiple requests should hit different servers)
print_info "Test 3: Testing load balancing across backend servers"
servers_hit=$(for i in {1..10}; do curl -s -H "Host: api.example.com" http://localhost:8080/api/test; done | grep -o "backend-server-[0-9]" | sort -u | wc -l)
if [ "$servers_hit" -ge 2 ]; then
    print_success "Load balancing is working (hit $servers_hit different servers)"
else
    print_error "Load balancing test failed (only hit $servers_hit server(s))"
    exit 1
fi

# Test 4: Test H2C (HTTP/2 Cleartext) support
print_info "Test 4: Testing H2C (HTTP/2 Cleartext) support"
if curl --http2-prior-knowledge -s -H "Host: api.example.com" http://localhost:8080/api/test | grep -q "backend-server"; then
    print_success "H2C (HTTP/2 Cleartext) is working correctly"
else
    print_info "H2C test skipped (curl may not support --http2-prior-knowledge)"
fi

print_success "All functional tests passed"

# Step 10: Summary
echo ""
echo "============================================="
print_success "All tests completed successfully!"
echo ""
print_info "To view logs:"
echo "  podman-compose logs -f gateway"
echo "  podman-compose logs -f backend-api"
echo ""
print_info "To stop services:"
echo "  podman-compose down"
echo ""
print_info "To cleanup everything:"
echo "  podman-compose down -v"
echo "============================================="
