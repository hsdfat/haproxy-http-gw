#!/bin/bash
# Local Test Script for HAProxy HTTP Gateway
# This script runs the complete test flow using Podman

set -e

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
if curl -sf http://localhost:8080/health > /dev/null 2>&1; then
    print_success "Gateway is healthy"
else
    print_error "Gateway health check failed"
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

# Step 8: Run functional tests
print_info "Step 8: Running functional tests..."
if podman-compose run --rm test-client /test-client \
    -gateway=http://gateway:8080 \
    -gateway-https=https://gateway:8443 \
    -verbose; then
    print_success "Functional tests passed"
else
    print_error "Functional tests failed"
    exit 1
fi

# Step 9: Run quick performance test
print_info "Step 9: Running quick performance test..."
if podman-compose run --rm test-client /perf-client \
    -url=http://gateway:8080 \
    -c=10 \
    -n=100; then
    print_success "Performance test completed"
else
    print_error "Performance test failed"
    exit 1
fi

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
