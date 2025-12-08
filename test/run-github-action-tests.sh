#!/bin/bash
# Complete GitHub Actions Test Runner (Local)
# This script runs all the tests from the GitHub Actions workflow locally

set -e

# Ensure podman-compose is in PATH (for macOS)
export PATH="/Library/Frameworks/Python.framework/Versions/3.14/bin:$PATH"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

print_section() {
    echo -e "${BLUE}═══════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════${NC}"
}

# Ensure we're in the test directory
cd "$(dirname "$0")"

print_section "HTTP Gateway Tests - GitHub Actions Local Runner"
echo ""

# Step 1: Check prerequisites
print_info "Step 1: Checking prerequisites..."
if ! command -v podman-compose &> /dev/null && ! command -v docker-compose &> /dev/null; then
    print_error "Neither podman-compose nor docker-compose is installed"
    echo "Install with: pip install podman-compose OR install docker-compose"
    exit 1
fi

# Check for required tools
if ! command -v jq &> /dev/null; then
    print_error "jq is not installed (required for parsing JSON)"
    echo "Install with: brew install jq (macOS) or apt-get install jq (Ubuntu)"
    exit 1
fi

if ! command -v bc &> /dev/null; then
    print_error "bc is not installed (required for calculations)"
    echo "Install with: brew install bc (macOS) or apt-get install bc (Ubuntu)"
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
print_section "Generate SSL Certificates"
if [ ! -f "certs/server.pem" ]; then
    chmod +x scripts/generate-certs.sh
    ./scripts/generate-certs.sh
    print_success "Certificates generated"
else
    print_success "Certificates already exist"
fi

# Step 3: Build images
print_section "Build Test Environment"
if $COMPOSE_CMD build; then
    print_success "All images built successfully"
else
    print_error "Build failed"
    exit 1
fi

# Step 4: Start services
print_section "Start Test Environment"
$COMPOSE_CMD up -d
print_success "Services started"

# Step 5: Wait for gateway to be ready
print_section "Check Service Health"
print_info "Checking gateway API health..."
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

    echo "  Attempt $retry/$max_retries: Gateway API not ready yet..."
    sleep 2
done

# Step 6: Check service status
print_info "Checking service status..."
$COMPOSE_CMD ps

# Step 7: Wait for backends to register
print_section "Wait for Backend Registration"
MAX_ATTEMPTS=30
ATTEMPT=0
API_BACKEND_FOUND=false
WEB_BACKEND_FOUND=false

while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    BACKENDS=$(curl -sf http://localhost:9090/api/frontends/default/backends 2>/dev/null || echo "")

    if echo "$BACKENDS" | grep -q "api-backend"; then
        API_BACKEND_FOUND=true
    fi

    if echo "$BACKENDS" | grep -q "web-backend"; then
        WEB_BACKEND_FOUND=true
    fi

    if [ "$API_BACKEND_FOUND" = true ] && [ "$WEB_BACKEND_FOUND" = true ]; then
        print_success "All backends registered successfully"
        break
    fi

    ATTEMPT=$((ATTEMPT + 1))
    echo "Attempt $ATTEMPT/$MAX_ATTEMPTS: Waiting for backends to register..."
    sleep 2
done

echo ""
echo "Final backend status:"
BACKENDS=$(curl -sf http://localhost:9090/api/frontends/default/backends)
echo "$BACKENDS" | jq '.' || echo "$BACKENDS"

if [ "$API_BACKEND_FOUND" != true ]; then
    print_error "api-backend not found in registered backends"
    echo ""
    echo "Gateway logs:"
    $COMPOSE_CMD logs gateway
    echo ""
    echo "Backend server logs:"
    $COMPOSE_CMD logs backend-server-1 backend-server-2 backend-server-3
    exit 1
fi

if [ "$WEB_BACKEND_FOUND" != true ]; then
    print_error "web-backend not found in registered backends"
    exit 1
fi

print_success "api-backend is registered"
print_success "web-backend is registered"

# Step 7.5: Configure routes
print_section "Configure Routes"
print_info "Configuring gateway routes..."
if ./scripts/configure-routes.sh; then
    print_success "Routes configured successfully"
else
    print_error "Failed to configure routes"
    exit 1
fi

# Step 8: Run functional tests
print_section "Run Functional Tests"
echo "Running functional tests (including H2C support)..."
go run ./client/cmd/test-client/main.go \
    -gateway=http://localhost:8080 \
    -verbose > functional-results.txt 2>&1

cat functional-results.txt

# Check if all tests passed
if grep -q "Failed: 0" functional-results.txt; then
    print_success "All functional tests passed"
    FUNCTIONAL_TESTS_PASSED=true
else
    print_error "Some functional tests failed"
    FUNCTIONAL_TESTS_PASSED=false
fi

# Step 9: Run performance tests - Low concurrency
print_section "Run Performance Tests - Low Concurrency"
echo "Running low concurrency performance test (10 workers, 1000 requests)..."
go run ./client/cmd/perf-client/main.go \
    -url=http://localhost:8080 \
    -c=10 \
    -n=1000 > perf-low-results.txt 2>&1

cat perf-low-results.txt

# Extract and validate metrics
if grep -q "Successful:" perf-low-results.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-low-results.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-low-results.txt | awk '{print $2}')

    echo "Success rate: ${SUCCESS_RATE}%"
    echo "Requests/sec: ${RPS}"

    # Validate success rate >= 99%
    if (( $(echo "$SUCCESS_RATE >= 99.0" | bc -l) )); then
        print_success "Performance test passed (success rate: ${SUCCESS_RATE}%)"
        PERF_LOW_PASSED=true
        PERF_LOW_RPS=$RPS
    else
        print_error "Performance test failed (success rate: ${SUCCESS_RATE}% < 99%)"
        PERF_LOW_PASSED=false
    fi
else
    print_error "Failed to parse performance results"
    PERF_LOW_PASSED=false
fi

# Step 10: Run performance tests - Medium concurrency
print_section "Run Performance Tests - Medium Concurrency"
echo "Running medium concurrency performance test (50 workers, 5000 requests)..."
go run ./client/cmd/perf-client/main.go \
    -url=http://localhost:8080 \
    -c=50 \
    -n=5000 > perf-medium-results.txt 2>&1

cat perf-medium-results.txt

# Extract and validate metrics
if grep -q "Successful:" perf-medium-results.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-medium-results.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-medium-results.txt | awk '{print $2}')

    echo "Success rate: ${SUCCESS_RATE}%"
    echo "Requests/sec: ${RPS}"

    # Validate success rate >= 98%
    if (( $(echo "$SUCCESS_RATE >= 98.0" | bc -l) )); then
        print_success "Performance test passed (success rate: ${SUCCESS_RATE}%)"
        PERF_MEDIUM_PASSED=true
        PERF_MEDIUM_RPS=$RPS
    else
        print_error "Performance test failed (success rate: ${SUCCESS_RATE}% < 98%)"
        PERF_MEDIUM_PASSED=false
    fi
else
    print_error "Failed to parse performance results"
    PERF_MEDIUM_PASSED=false
fi

# Step 11: Run HTTP/2 performance test
print_section "Run HTTP/2 Performance Test"
echo "Running HTTP/2 performance test (50 workers, 5000 requests)..."
go run ./client/cmd/perf-client/main.go \
    -url=http://localhost:8080 \
    -http2 \
    -c=50 \
    -n=5000 > perf-http2-results.txt 2>&1

cat perf-http2-results.txt

# Extract and validate metrics
if grep -q "Successful:" perf-http2-results.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-http2-results.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-http2-results.txt | awk '{print $2}')

    echo "Success rate: ${SUCCESS_RATE}%"
    echo "Requests/sec: ${RPS}"

    # Validate success rate >= 98%
    if (( $(echo "$SUCCESS_RATE >= 98.0" | bc -l) )); then
        print_success "HTTP/2 performance test passed (success rate: ${SUCCESS_RATE}%)"
        PERF_HTTP2_PASSED=true
        PERF_HTTP2_RPS=$RPS
    else
        print_error "HTTP/2 performance test failed (success rate: ${SUCCESS_RATE}% < 98%)"
        PERF_HTTP2_PASSED=false
    fi
else
    print_error "Failed to parse performance results"
    PERF_HTTP2_PASSED=false
fi

# Step 12: Test dynamic backend updates
print_section "Test Dynamic Backend Updates"
echo "Testing dynamic backend registration API..."

# Register a new backend via API using frontend-scoped API
echo "Registering new backend via API to frontend 'default'..."
RESPONSE=$(curl -sf -X POST http://localhost:9090/api/frontends/default/backends \
    -H "Content-Type: application/json" \
    -d '{
      "name": "dynamic-test-backend",
      "servers": [
        {"name": "test-srv", "ip": "backend-server-3", "port": 9000}
      ]
    }')

echo "Response: $RESPONSE"

# Verify response
if echo "$RESPONSE" | grep -q '"success":true'; then
    print_success "Backend registration API successful"
    DYNAMIC_BACKEND_PASSED=true
else
    print_error "Backend registration API failed"
    DYNAMIC_BACKEND_PASSED=false
fi

# Wait for HAProxy to apply changes
sleep 5

# Verify backend was added
echo "Verifying backend was added..."
BACKENDS=$(curl -sf http://localhost:9090/api/frontends/default/backends)
if echo "$BACKENDS" | grep -q "dynamic-test-backend"; then
    print_success "Dynamic backend registration successful"
else
    print_error "Dynamic backend not found after registration"
    echo "Backends response: $BACKENDS"
    DYNAMIC_BACKEND_PASSED=false
fi

# Test unregistration
echo "Testing backend unregistration..."
UNREG_RESPONSE=$(curl -sf -X DELETE http://localhost:9090/api/frontends/default/backends/dynamic-test-backend)
if echo "$UNREG_RESPONSE" | grep -q '"success":true'; then
    print_success "Backend unregistration successful"
else
    print_error "Backend unregistration failed"
    echo "Response: $UNREG_RESPONSE"
    DYNAMIC_BACKEND_PASSED=false
fi

# Step 13: Generate test summary
print_section "Test Summary"
echo ""
echo "# HTTP Gateway Test Results"
echo ""
echo "## Test Summary"
echo ""
echo "| Test Category | Status | Details |"
echo "|--------------|--------|---------|"

# Functional tests
if [ "$FUNCTIONAL_TESTS_PASSED" = true ]; then
    echo "| Functional Tests | ✅ PASS | All tests passed |"
else
    echo "| Functional Tests | ❌ FAIL | Some tests failed |"
fi

# Performance test - low
if [ "$PERF_LOW_PASSED" = true ]; then
    echo "| Performance (Low) | ✅ PASS | RPS: $PERF_LOW_RPS |"
else
    echo "| Performance (Low) | ❌ FAIL | Failed |"
fi

# Performance test - medium
if [ "$PERF_MEDIUM_PASSED" = true ]; then
    echo "| Performance (Medium) | ✅ PASS | RPS: $PERF_MEDIUM_RPS |"
else
    echo "| Performance (Medium) | ❌ FAIL | Failed |"
fi

# Performance test - HTTP/2
if [ "$PERF_HTTP2_PASSED" = true ]; then
    echo "| HTTP/2 Performance | ✅ PASS | RPS: $PERF_HTTP2_RPS |"
else
    echo "| HTTP/2 Performance | ❌ FAIL | Failed |"
fi

# Dynamic backend test
if [ "$DYNAMIC_BACKEND_PASSED" = true ]; then
    echo "| Dynamic Backend | ✅ PASS | Backend updates working |"
else
    echo "| Dynamic Backend | ❌ FAIL | Failed |"
fi

echo ""
echo "## Performance Metrics"
echo ""
echo "| Configuration | Requests/sec |"
echo "|--------------|--------------|"
echo "| 10 workers, HTTP/1.1 | ${PERF_LOW_RPS:-N/A} |"
echo "| 50 workers, HTTP/1.1 | ${PERF_MEDIUM_RPS:-N/A} |"
echo "| 50 workers, HTTP/2 | ${PERF_HTTP2_RPS:-N/A} |"

# Check if all tests passed
ALL_PASSED=true
if [ "$FUNCTIONAL_TESTS_PASSED" != true ]; then ALL_PASSED=false; fi
if [ "$PERF_LOW_PASSED" != true ]; then ALL_PASSED=false; fi
if [ "$PERF_MEDIUM_PASSED" != true ]; then ALL_PASSED=false; fi
if [ "$PERF_HTTP2_PASSED" != true ]; then ALL_PASSED=false; fi
if [ "$DYNAMIC_BACKEND_PASSED" != true ]; then ALL_PASSED=false; fi

echo ""
if [ "$ALL_PASSED" = true ]; then
    print_section "✅ All HTTP Gateway tests passed successfully!"
    echo ""
    print_info "Test artifacts saved:"
    echo "  - functional-results.txt"
    echo "  - perf-low-results.txt"
    echo "  - perf-medium-results.txt"
    echo "  - perf-http2-results.txt"
    echo ""
    print_info "Services are still running. Use these commands:"
    echo "  View logs: $COMPOSE_CMD logs -f gateway"
    echo "  Stop services: $COMPOSE_CMD down"
    echo "  Cleanup: $COMPOSE_CMD down -v"
    exit 0
else
    print_section "❌ HTTP Gateway tests failed!"
    echo ""
    print_info "Please review the test results and fix any issues."
    echo ""
    print_info "View logs:"
    echo "  Gateway: $COMPOSE_CMD logs gateway"
    echo "  Backends: $COMPOSE_CMD logs backend-server-1"
    echo ""
    print_info "Stop services: $COMPOSE_CMD down"
    exit 1
fi
