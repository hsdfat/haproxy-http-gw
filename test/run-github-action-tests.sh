#!/bin/bash
# Complete GitHub Actions Test Runner (Local)
# This script runs all the tests from the GitHub Actions workflow locally

set -e

# Environment variables (matching workflow)
export CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
export COMPOSE_CMD="${COMPOSE_CMD:-podman-compose}"
export GO_VERSION="${GO_VERSION:-1.21}"

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

# Get the project root directory (parent of test directory)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$SCRIPT_DIR"

# Cleanup function (runs on exit, matching workflow's if: always())
cleanup() {
    local exit_code=$?
    if [ -n "$COMPOSE_CMD" ]; then
        echo ""
        print_info "Cleaning up containers..."
        $COMPOSE_CMD down -v || true
    fi
    exit $exit_code
}
trap cleanup EXIT INT TERM

print_section "HTTP Gateway Tests - GitHub Actions Local Runner"
echo ""

# Track start time
TEST_START_TIME=$(date +%s)

# Step 1: Check prerequisites
print_info "Step 1: Checking prerequisites..."

# Check for Go
if ! command -v go &> /dev/null; then
    print_error "Go is not installed"
    echo "Install Go version ${GO_VERSION} or later"
    exit 1
fi
GO_VER=$(go version | awk '{print $3}' | sed 's/go//')
print_success "Go version: $GO_VER"

# Check for container runtime
if [ "$CONTAINER_RUNTIME" = "podman" ]; then
    if ! command -v podman &> /dev/null; then
        print_error "podman is not installed"
        exit 1
    fi
    podman --version
fi

# Check for compose command
if ! command -v $COMPOSE_CMD &> /dev/null; then
    if [ "$COMPOSE_CMD" = "podman-compose" ]; then
        print_info "podman-compose not found, attempting to install..."
        
        # Try pip3 first (common on macOS), then pip
        if command -v pip3 &> /dev/null; then
            pip3 install podman-compose || {
                print_error "Failed to install podman-compose with pip3"
                echo "Try manually: pip3 install podman-compose"
                echo "Or install Python pip first: python3 -m ensurepip --upgrade"
                exit 1
            }
        elif command -v pip &> /dev/null; then
            pip install podman-compose || {
                print_error "Failed to install podman-compose with pip"
                echo "Try manually: pip install podman-compose"
                echo "Or install Python pip first: python3 -m ensurepip --upgrade"
                exit 1
            }
        else
            print_error "podman-compose is not installed and pip is not available"
            echo "Install options:"
            echo "  - pip3 install podman-compose"
            echo "  - pip install podman-compose"
            echo "  - Install Python pip first: python3 -m ensurepip --upgrade"
            exit 1
        fi
        print_success "podman-compose installed successfully"
    else
        print_error "$COMPOSE_CMD is not installed"
        exit 1
    fi
fi
print_success "Using $COMPOSE_CMD: $($COMPOSE_CMD --version | head -1)"

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

# Check for yamllint (for config validation)
if ! command -v yamllint &> /dev/null; then
    print_info "yamllint not found, attempting to install..."
    
    # Try pip3 first (common on macOS), then pip, then brew (macOS)
    if command -v pip3 &> /dev/null; then
        pip3 install yamllint || {
            print_error "Failed to install yamllint with pip3"
            echo "Try manually: pip3 install yamllint"
            echo "Or on macOS: brew install yamllint"
            exit 1
        }
    elif command -v pip &> /dev/null; then
        pip install yamllint || {
            print_error "Failed to install yamllint with pip"
            echo "Try manually: pip install yamllint"
            echo "Or on macOS: brew install yamllint"
            exit 1
        }
    elif command -v brew &> /dev/null; then
        print_info "Trying to install yamllint via Homebrew..."
        brew install yamllint || {
            print_error "Failed to install yamllint with brew"
            echo "Try manually: brew install yamllint"
            echo "Or install Python pip first: python3 -m ensurepip --upgrade"
            exit 1
        }
    else
        print_error "yamllint is not installed and no package manager found"
        echo "Install options:"
        echo "  - pip3 install yamllint"
        echo "  - pip install yamllint"
        echo "  - brew install yamllint (macOS)"
        echo "  - apt-get install yamllint (Ubuntu/Debian)"
        exit 1
    fi
    print_success "yamllint installed successfully"
fi

# Step 2: Run unit tests
print_section "Unit Tests"
cd "$PROJECT_ROOT"

# Run configuration tests
print_info "Running configuration unit tests..."
cd pkg/gateway
if go test -v -run TestFrontendConfig 2>&1 | tee ../../config-test-results.txt; then
    if grep -q "PASS" ../../config-test-results.txt; then
        print_success "Configuration tests passed"
    else
        print_error "Configuration tests failed"
        exit 1
    fi
else
    print_error "Configuration tests failed"
    exit 1
fi

# Run all gateway unit tests
print_info "Running all gateway unit tests..."
if go test -v 2>&1 | tee ../../gateway-test-results.txt; then
    if grep -q "PASS" ../../gateway-test-results.txt; then
        print_success "All gateway tests passed"
    else
        print_error "Gateway tests failed"
        exit 1
    fi
else
    print_error "Gateway tests failed"
    exit 1
fi

# Generate test coverage
print_info "Generating test coverage..."
go test -coverprofile=coverage.out
go tool cover -func=coverage.out > coverage.txt
echo "Test Coverage:"
cat coverage.txt | tail -1

cd "$SCRIPT_DIR"

# Step 3: Validate configurations
print_section "Validate Configurations"
cd "$PROJECT_ROOT"

# Validate YAML syntax
print_info "Validating YAML syntax..."
FAILED=0
for file in examples/frontend-config/*.yaml; do
    if [ -f "$file" ]; then
        echo "Checking $file..."
        if yamllint -d relaxed "$file" 2>&1; then
            print_success "$(basename $file) - Valid YAML"
        else
            print_error "$(basename $file) - Invalid YAML"
            FAILED=$((FAILED + 1))
        fi
    fi
done

if [ $FAILED -eq 0 ]; then
    print_success "All configuration files are valid"
else
    print_error "$FAILED configuration file(s) failed validation"
    exit 1
fi

# Verify example files exist
print_info "Verifying all example files exist..."
REQUIRED_FILES=(
    "examples/frontend-config/single-http-frontend.yaml"
    "examples/frontend-config/http-https-frontend.yaml"
    "examples/frontend-config/multiple-frontends.yaml"
    "examples/frontend-config/bypass-mode-frontend.yaml"
    "examples/frontend-config/multi-tenant.yaml"
    "examples/frontend-config/README.md"
)

MISSING=0
for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        print_success "$file exists"
    else
        print_error "$file missing"
        MISSING=$((MISSING + 1))
    fi
done

if [ $MISSING -gt 0 ]; then
    print_error "$MISSING required file(s) missing"
    exit 1
fi

print_success "All required files exist"
cd "$SCRIPT_DIR"

# Step 4: Setup & Deploy Environment
print_section "Setup & Deploy Environment"
cd "$SCRIPT_DIR"

# Generate certificates
print_info "Generating SSL certificates..."
if [ ! -f "certs/server.pem" ]; then
    chmod +x scripts/generate-certs.sh
    ./scripts/generate-certs.sh
    print_success "Certificates generated"
else
    print_success "Certificates already exist"
fi

# Build images
print_info "Building test environment..."
if $COMPOSE_CMD build; then
    print_success "All images built successfully"
else
    print_error "Build failed"
    exit 1
fi

# Start services
print_info "Starting services..."
$COMPOSE_CMD up -d
print_success "Services started"

# Step 5: Wait for gateway to be ready (with timeout)
print_section "Check Service Health"
print_info "Checking gateway API health..."
HEALTH_TIMEOUT=$((SECONDS + 120))  # 2 minute timeout
HEALTH_CHECK_PASSED=false

for i in {1..30}; do
    if [ $SECONDS -ge $HEALTH_TIMEOUT ]; then
        print_error "Health check timed out after 2 minutes"
        break
    fi

    if curl -sf http://localhost:9090/health > /dev/null 2>&1; then
        print_success "Gateway API is healthy"
        HEALTH_CHECK_PASSED=true
        break
    fi
    echo "Attempt $i/30: Gateway API not ready yet..."
    sleep 2
done

if [ "$HEALTH_CHECK_PASSED" != "true" ]; then
    print_error "Gateway API failed to become healthy"
    echo ""
    echo "=== Gateway Logs ==="
    $COMPOSE_CMD logs gateway
    echo ""
    echo "=== Container Status ==="
    $CONTAINER_RUNTIME ps -a
    exit 1
fi

# Step 6: Wait for backend registration (with timeout)
print_section "Wait for Backend Registration"
MAX_ATTEMPTS=30
ATTEMPT=0
BACKEND_TIMEOUT=$((SECONDS + 180))  # 3 minute timeout
API_BACKEND_FOUND=false
WEB_BACKEND_FOUND=false
API_V2_BACKEND_FOUND=false
WEB_V2_BACKEND_FOUND=false

while [ $ATTEMPT -lt $MAX_ATTEMPTS ] && [ $SECONDS -lt $BACKEND_TIMEOUT ]; do
    BACKENDS_DEFAULT=$(curl -sf http://localhost:9090/api/frontends/default/backends 2>/dev/null || echo "")
    if echo "$BACKENDS_DEFAULT" | grep -q "api-backend"; then API_BACKEND_FOUND=true; fi
    if echo "$BACKENDS_DEFAULT" | grep -q "web-backend"; then WEB_BACKEND_FOUND=true; fi

    BACKENDS_API=$(curl -sf http://localhost:9090/api/frontends/frontend-api/backends 2>/dev/null || echo "")
    if echo "$BACKENDS_API" | grep -q "api-v2-backend"; then API_V2_BACKEND_FOUND=true; fi

    BACKENDS_WEB=$(curl -sf http://localhost:9090/api/frontends/frontend-web/backends 2>/dev/null || echo "")
    if echo "$BACKENDS_WEB" | grep -q "web-v2-backend"; then WEB_V2_BACKEND_FOUND=true; fi

    if [ "$API_BACKEND_FOUND" = true ] && [ "$WEB_BACKEND_FOUND" = true ] && [ "$API_V2_BACKEND_FOUND" = true ] && [ "$WEB_V2_BACKEND_FOUND" = true ]; then
        print_success "All backends registered successfully"
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    echo "Attempt $ATTEMPT/$MAX_ATTEMPTS: Waiting for backends..."
    sleep 2
done

if [ "$API_BACKEND_FOUND" != true ] || [ "$WEB_BACKEND_FOUND" != true ] || [ "$API_V2_BACKEND_FOUND" != true ] || [ "$WEB_V2_BACKEND_FOUND" != true ]; then
    print_error "Some backends failed to register"
    echo "Backend Registration Status:"
    echo "  - api-backend (default): $API_BACKEND_FOUND"
    echo "  - web-backend (default): $WEB_BACKEND_FOUND"
    echo "  - api-v2-backend (frontend-api): $API_V2_BACKEND_FOUND"
    echo "  - web-v2-backend (frontend-web): $WEB_V2_BACKEND_FOUND"
    echo ""
    echo "=== Gateway Logs ==="
    $COMPOSE_CMD logs gateway
    echo ""
    echo "=== Backend Server Logs ==="
    $COMPOSE_CMD logs backend-server-1 backend-server-2 backend-server-3 || true
    $COMPOSE_CMD logs api-v2-server-1 api-v2-server-2 || true
    $COMPOSE_CMD logs web-v2-server-1 web-v2-server-2 || true
    exit 1
fi

# Configure routing rules
print_info "Configuring routing rules..."
chmod +x scripts/configure-routes.sh
./scripts/configure-routes.sh

# Verify backends are serving traffic
print_info "Verifying backends are serving traffic..."
sleep 5
for i in {1..20}; do
    RESP=$(curl -s http://localhost:8080/ || echo "")
    if echo "$RESP" | grep -q "backend-server"; then
        print_success "Services are ready"
        break
    fi
    if [ $i -eq 20 ]; then
        print_error "Services not serving traffic"
        $COMPOSE_CMD logs gateway
        exit 1
    fi
    sleep 3
done

# Step 7: Run All Tests
print_section "Run All Tests"
cd "$SCRIPT_DIR"

# Functional tests
echo "=== Running Functional Tests ==="
go run ./client/cmd/test-client/main.go -gateway=http://localhost:8080 -verbose > functional-results-default.txt 2>&1
cat functional-results-default.txt
DEFAULT_PASSED=$(grep -q "Failed: 0" functional-results-default.txt && echo "true" || echo "false")

go run ./client/cmd/test-client/main.go -gateway=http://localhost:8081 -verbose > functional-results-api.txt 2>&1
cat functional-results-api.txt
API_PASSED=$(grep -q "Failed: 0" functional-results-api.txt && echo "true" || echo "false")

go run ./client/cmd/test-client/main.go -gateway=http://localhost:8082 -verbose > functional-results-web.txt 2>&1
cat functional-results-web.txt
WEB_PASSED=$(grep -q "Failed: 0" functional-results-web.txt && echo "true" || echo "false")

if [ "$DEFAULT_PASSED" = "true" ] && [ "$API_PASSED" = "true" ] && [ "$WEB_PASSED" = "true" ]; then
    FUNCTIONAL_TESTS_PASSED=true
    print_success "All functional tests passed"
else
    FUNCTIONAL_TESTS_PASSED=false
    print_error "Some functional tests failed"
    echo "=== FUNCTIONAL TEST FAILURES ==="
    echo "Test Status:"
    echo "  - Default frontend (8080): $DEFAULT_PASSED"
    echo "  - API frontend (8081): $API_PASSED"
    echo "  - Web frontend (8082): $WEB_PASSED"
    echo ""
    echo "Failed test details:"
    grep -i "failed\|error" functional-results-*.txt || true
    exit 1
fi

# Performance tests - Low concurrency (8 workers, 2,000 requests - all frontends in parallel)
echo "=== Running Performance Tests - Low (All Frontends in Parallel) ==="

# Run all 3 frontends simultaneously in background
go run ./client/cmd/perf-client/main.go -url=http://localhost:8080 -http2 -c=8 -n=2000 > perf-low-default.txt 2>&1 &
PID_DEFAULT=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8081 -http2 -c=8 -n=2000 > perf-low-api.txt 2>&1 &
PID_API=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8082 -http2 -c=8 -n=2000 > perf-low-web.txt 2>&1 &
PID_WEB=$!

# Wait for all tests to complete
wait $PID_DEFAULT $PID_API $PID_WEB
print_success "All low concurrency tests completed"

# Display results
echo "=== Default Frontend Results ==="
cat perf-low-default.txt
echo "=== API Frontend Results ==="
cat perf-low-api.txt
echo "=== Web Frontend Results ==="
cat perf-low-web.txt

# Validate results
PERF_LOW_DEFAULT_PASSED=false
PERF_LOW_DEFAULT_RPS=""
if grep -q "Successful:" perf-low-default.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-low-default.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-low-default.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 99.0" | bc -l) )); then
        PERF_LOW_DEFAULT_PASSED=true
        PERF_LOW_DEFAULT_RPS=$RPS
    fi
fi

PERF_LOW_API_PASSED=false
PERF_LOW_API_RPS=""
if grep -q "Successful:" perf-low-api.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-low-api.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-low-api.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 99.0" | bc -l) )); then
        PERF_LOW_API_PASSED=true
        PERF_LOW_API_RPS=$RPS
    fi
fi

PERF_LOW_WEB_PASSED=false
PERF_LOW_WEB_RPS=""
if grep -q "Successful:" perf-low-web.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-low-web.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-low-web.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 99.0" | bc -l) )); then
        PERF_LOW_WEB_PASSED=true
        PERF_LOW_WEB_RPS=$RPS
    fi
fi

if [ "$PERF_LOW_DEFAULT_PASSED" = "true" ] && [ "$PERF_LOW_API_PASSED" = "true" ] && [ "$PERF_LOW_WEB_PASSED" = "true" ]; then
    PERF_LOW_PASSED=true
    print_success "All low concurrency performance tests passed"
else
    PERF_LOW_PASSED=false
    print_error "Some low concurrency performance tests failed"
    exit 1
fi

# Performance tests - Medium concurrency (16 workers, 20,000 requests - all frontends in parallel)
echo "=== Running Performance Tests - Medium (All Frontends in Parallel) ==="

# Run all 3 frontends simultaneously in background
go run ./client/cmd/perf-client/main.go -url=http://localhost:8080 -http2 -c=16 -n=20000 > perf-medium-default.txt 2>&1 &
PID_DEFAULT=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8081 -http2 -c=16 -n=20000 > perf-medium-api.txt 2>&1 &
PID_API=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8082 -http2 -c=16 -n=20000 > perf-medium-web.txt 2>&1 &
PID_WEB=$!

# Wait for all tests to complete
wait $PID_DEFAULT $PID_API $PID_WEB
print_success "All medium concurrency tests completed"

# Display and validate results for default frontend
cat perf-medium-default.txt
PERF_MEDIUM_DEFAULT_PASSED=false
PERF_MEDIUM_DEFAULT_RPS=""
if grep -q "Successful:" perf-medium-default.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-medium-default.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-medium-default.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 98.0" | bc -l) )); then
        PERF_MEDIUM_DEFAULT_PASSED=true
        PERF_MEDIUM_DEFAULT_RPS=$RPS
    fi
fi

# Display and validate results for API frontend
cat perf-medium-api.txt
PERF_MEDIUM_API_PASSED=false
PERF_MEDIUM_API_RPS=""
if grep -q "Successful:" perf-medium-api.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-medium-api.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-medium-api.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 98.0" | bc -l) )); then
        PERF_MEDIUM_API_PASSED=true
        PERF_MEDIUM_API_RPS=$RPS
    fi
fi

# Display and validate results for web frontend
cat perf-medium-web.txt
PERF_MEDIUM_WEB_PASSED=false
PERF_MEDIUM_WEB_RPS=""
if grep -q "Successful:" perf-medium-web.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-medium-web.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-medium-web.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 98.0" | bc -l) )); then
        PERF_MEDIUM_WEB_PASSED=true
        PERF_MEDIUM_WEB_RPS=$RPS
    fi
fi

if [ "$PERF_MEDIUM_DEFAULT_PASSED" = "true" ] && [ "$PERF_MEDIUM_API_PASSED" = "true" ] && [ "$PERF_MEDIUM_WEB_PASSED" = "true" ]; then
    PERF_MEDIUM_PASSED=true
    print_success "All medium concurrency performance tests passed"
else
    PERF_MEDIUM_PASSED=false
    print_error "Some medium concurrency performance tests failed"
    exit 1
fi

# Performance tests - High concurrency (32 workers, 100,000 requests - all frontends in parallel)
echo "=== Running Performance Tests - High (All Frontends in Parallel) ==="

# Run all 3 frontends simultaneously in background
go run ./client/cmd/perf-client/main.go -url=http://localhost:8080 -http2 -c=32 -n=100000 > perf-high-default.txt 2>&1 &
PID_DEFAULT=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8081 -http2 -c=32 -n=100000 > perf-high-api.txt 2>&1 &
PID_API=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8082 -http2 -c=32 -n=100000 > perf-high-web.txt 2>&1 &
PID_WEB=$!

# Wait for all tests to complete
wait $PID_DEFAULT $PID_API $PID_WEB
print_success "All high concurrency tests completed"

# Display and validate results for default frontend
cat perf-high-default.txt
PERF_HIGH_DEFAULT_PASSED=false
PERF_HIGH_DEFAULT_RPS=""
if grep -q "Successful:" perf-high-default.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-high-default.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-high-default.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 95.0" | bc -l) )); then
        PERF_HIGH_DEFAULT_PASSED=true
        PERF_HIGH_DEFAULT_RPS=$RPS
    fi
fi

# Display and validate results for API frontend
cat perf-high-api.txt
PERF_HIGH_API_PASSED=false
PERF_HIGH_API_RPS=""
if grep -q "Successful:" perf-high-api.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-high-api.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-high-api.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 95.0" | bc -l) )); then
        PERF_HIGH_API_PASSED=true
        PERF_HIGH_API_RPS=$RPS
    fi
fi

# Display and validate results for web frontend
cat perf-high-web.txt
PERF_HIGH_WEB_PASSED=false
PERF_HIGH_WEB_RPS=""
if grep -q "Successful:" perf-high-web.txt; then
    SUCCESS_RATE=$(grep "Successful:" perf-high-web.txt | awk '{print $3}' | tr -d '()%')
    RPS=$(grep "Requests/sec:" perf-high-web.txt | awk '{print $2}')
    if (( $(echo "$SUCCESS_RATE >= 95.0" | bc -l) )); then
        PERF_HIGH_WEB_PASSED=true
        PERF_HIGH_WEB_RPS=$RPS
    fi
fi

if [ "$PERF_HIGH_DEFAULT_PASSED" = "true" ] && [ "$PERF_HIGH_API_PASSED" = "true" ] && [ "$PERF_HIGH_WEB_PASSED" = "true" ]; then
    PERF_HTTP2_PASSED=true
    print_success "All high concurrency performance tests passed"
else
    PERF_HTTP2_PASSED=false
    print_error "Some high concurrency performance tests failed"
    exit 1
fi

# Dynamic backend test
echo "=== Testing Dynamic Backend Updates ==="
RESPONSE=$(curl -sf -X POST http://localhost:9090/api/frontends/default/backends \
    -H "Content-Type: application/json" \
    -d '{"name": "dynamic-test-backend", "servers": [{"name": "test-srv", "ip": "backend-server-3", "port": 9000}]}')
if echo "$RESPONSE" | grep -q '"success":true'; then
    sleep 5
    BACKENDS=$(curl -sf http://localhost:9090/api/frontends/default/backends)
    if echo "$BACKENDS" | grep -q "dynamic-test-backend"; then
        UNREG_RESPONSE=$(curl -sf -X DELETE http://localhost:9090/api/frontends/default/backends/dynamic-test-backend)
        if echo "$UNREG_RESPONSE" | grep -q '"success":true'; then
            DYNAMIC_BACKEND_PASSED=true
            print_success "Dynamic backend test passed"
        else
            DYNAMIC_BACKEND_PASSED=false
            print_error "Dynamic backend unregistration failed"
            exit 1
        fi
    else
        DYNAMIC_BACKEND_PASSED=false
        print_error "Dynamic backend not found after registration"
        exit 1
    fi
else
    DYNAMIC_BACKEND_PASSED=false
    print_error "Dynamic backend registration failed"
    exit 1
fi

# Step 8: Report Results & Cleanup
print_section "Report Results & Cleanup"

# View logs
echo "=== Gateway Logs ==="
$COMPOSE_CMD logs gateway || true
echo ""
echo "=== Backend Logs ==="
$COMPOSE_CMD logs backend-server-1 backend-server-2 backend-server-3 || true
$COMPOSE_CMD logs api-v2-server-1 api-v2-server-2 || true
$COMPOSE_CMD logs web-v2-server-1 web-v2-server-2 || true

# Collect stats
echo "=== Container Stats ==="
if [ "$CONTAINER_RUNTIME" = "podman" ]; then
    podman stats --no-stream || true
else
    docker stats --no-stream || true
fi

# Calculate overall status
if [ "$FUNCTIONAL_TESTS_PASSED" = "true" ] && [ "$PERF_LOW_PASSED" = "true" ] && [ "$PERF_MEDIUM_PASSED" = "true" ] && [ "$PERF_HTTP2_PASSED" = "true" ] && [ "$DYNAMIC_BACKEND_PASSED" = "true" ]; then
    OVERALL_STATUS="✅ ALL TESTS PASSED"
    OVERALL_EMOJI="🎉"
else
    OVERALL_STATUS="❌ SOME TESTS FAILED"
    OVERALL_EMOJI="⚠️"
fi

# Generate comprehensive summary
echo ""
echo "# $OVERALL_EMOJI HTTP Gateway Test Results"
echo ""
echo "> **Overall Status:** $OVERALL_STATUS"
echo ""

echo "## 📊 Test Summary"
echo ""
echo "| Test Category | Status | Details |"
echo "|--------------|:------:|---------|"

# Functional tests
if [ "$FUNCTIONAL_TESTS_PASSED" = "true" ]; then
    echo "| **Functional Tests** | ✅ PASS | All frontends passed |"
else
    echo "| **Functional Tests** | ❌ FAIL | Some tests failed - check artifacts |"
fi

# HTTP/2 (H2C) Performance test - low
if [ "$PERF_LOW_PASSED" = "true" ]; then
    echo "| **HTTP/2 (H2C) - Low** | ✅ PASS | 8 workers, 2,000 requests (all frontends) |"
else
    echo "| **HTTP/2 (H2C) - Low** | ❌ FAIL | Success rate < 99% |"
fi

# HTTP/2 (H2C) Performance test - medium
if [ "$PERF_MEDIUM_PASSED" = "true" ]; then
    echo "| **HTTP/2 (H2C) - Medium** | ✅ PASS | 16 workers, 20,000 requests (all frontends) |"
else
    echo "| **HTTP/2 (H2C) - Medium** | ❌ FAIL | Success rate < 98% |"
fi

# HTTP/2 (H2C) Performance test - high
if [ "$PERF_HTTP2_PASSED" = "true" ]; then
    echo "| **HTTP/2 (H2C) - High** | ✅ PASS | 32 workers, 100,000 requests (all frontends) |"
else
    echo "| **HTTP/2 (H2C) - High** | ❌ FAIL | Success rate < 95% |"
fi

# Dynamic backend test
if [ "$DYNAMIC_BACKEND_PASSED" = "true" ]; then
    echo "| **Dynamic Backend** | ✅ PASS | Registration & unregistration working |"
else
    echo "| **Dynamic Backend** | ❌ FAIL | Backend updates not working |"
fi

echo ""
echo "## 🚀 HTTP/2 (H2C) Performance Metrics"
echo ""
echo "| Test Level | Frontend | Concurrency | Total Requests | Requests/sec | Status |"
echo "|-----------|:--------:|:-----------:|:--------------:|:------------:|:------:|"

# Low concurrency - all frontends
if [ -n "$PERF_LOW_DEFAULT_RPS" ]; then
    echo "| Low | Default | 8 | 2,000 | **${PERF_LOW_DEFAULT_RPS}** | ✅ |"
else
    echo "| Low | Default | 8 | 2,000 | N/A | ❌ |"
fi

if [ -n "$PERF_LOW_API_RPS" ]; then
    echo "| Low | API | 8 | 2,000 | **${PERF_LOW_API_RPS}** | ✅ |"
else
    echo "| Low | API | 8 | 2,000 | N/A | ❌ |"
fi

if [ -n "$PERF_LOW_WEB_RPS" ]; then
    echo "| Low | Web | 8 | 2,000 | **${PERF_LOW_WEB_RPS}** | ✅ |"
else
    echo "| Low | Web | 8 | 2,000 | N/A | ❌ |"
fi

# Medium concurrency - all frontends
if [ -n "$PERF_MEDIUM_DEFAULT_RPS" ]; then
    echo "| Medium | Default | 16 | 20,000 | **${PERF_MEDIUM_DEFAULT_RPS}** | ✅ |"
else
    echo "| Medium | Default | 16 | 20,000 | N/A | ❌ |"
fi

if [ -n "$PERF_MEDIUM_API_RPS" ]; then
    echo "| Medium | API | 16 | 20,000 | **${PERF_MEDIUM_API_RPS}** | ✅ |"
else
    echo "| Medium | API | 16 | 20,000 | N/A | ❌ |"
fi

if [ -n "$PERF_MEDIUM_WEB_RPS" ]; then
    echo "| Medium | Web | 16 | 20,000 | **${PERF_MEDIUM_WEB_RPS}** | ✅ |"
else
    echo "| Medium | Web | 16 | 20,000 | N/A | ❌ |"
fi

# High concurrency - all frontends
if [ -n "$PERF_HIGH_DEFAULT_RPS" ]; then
    echo "| High | Default | 32 | 100,000 | **${PERF_HIGH_DEFAULT_RPS}** | ✅ |"
else
    echo "| High | Default | 32 | 100,000 | N/A | ❌ |"
fi

if [ -n "$PERF_HIGH_API_RPS" ]; then
    echo "| High | API | 32 | 100,000 | **${PERF_HIGH_API_RPS}** | ✅ |"
else
    echo "| High | API | 32 | 100,000 | N/A | ❌ |"
fi

if [ -n "$PERF_HIGH_WEB_RPS" ]; then
    echo "| High | Web | 32 | 100,000 | **${PERF_HIGH_WEB_RPS}** | ✅ |"
else
    echo "| High | Web | 32 | 100,000 | N/A | ❌ |"
fi

echo ""
echo "> **Note:** All performance tests use HTTP/2 over cleartext (H2C) protocol."

echo ""
echo "## 📦 Test Artifacts"
echo ""
echo "Detailed test results are available:"
echo "- \`functional-results-default.txt\` - Default frontend functional tests"
echo "- \`functional-results-api.txt\` - Frontend-API functional tests"
echo "- \`functional-results-web.txt\` - Frontend-Web functional tests"
echo "- \`perf-low-default.txt\` - Low concurrency (default frontend)"
echo "- \`perf-low-api.txt\` - Low concurrency (frontend-api)"
echo "- \`perf-low-web.txt\` - Low concurrency (frontend-web)"
echo "- \`perf-medium-default.txt\` - Medium concurrency (default frontend)"
echo "- \`perf-medium-api.txt\` - Medium concurrency (frontend-api)"
echo "- \`perf-medium-web.txt\` - Medium concurrency (frontend-web)"
echo "- \`perf-high-default.txt\` - High concurrency (default frontend)"
echo "- \`perf-high-api.txt\` - High concurrency (frontend-api)"
echo "- \`perf-high-web.txt\` - High concurrency (frontend-web)"

echo ""
echo "## ℹ️ Environment"
echo ""
echo "- **Container Runtime:** $CONTAINER_RUNTIME"
echo "- **Compose Command:** $COMPOSE_CMD"
echo "- **Go Version:** $GO_VER"

# Calculate test duration
TEST_END_TIME=$(date +%s)
TEST_DURATION=$((TEST_END_TIME - TEST_START_TIME))
TEST_DURATION_MIN=$((TEST_DURATION / 60))
TEST_DURATION_SEC=$((TEST_DURATION % 60))

echo ""
echo "## ⏱️ Test Duration"
echo ""
echo "- **Total Time:** ${TEST_DURATION_MIN}m ${TEST_DURATION_SEC}s"

# Cleanup
print_info "Cleaning up..."
$COMPOSE_CMD down -v || true

# Final status
if [ "$FUNCTIONAL_TESTS_PASSED" = "true" ] && [ "$PERF_LOW_PASSED" = "true" ] && [ "$PERF_MEDIUM_PASSED" = "true" ] && [ "$PERF_HTTP2_PASSED" = "true" ] && [ "$DYNAMIC_BACKEND_PASSED" = "true" ]; then
    print_section "$OVERALL_STATUS"
    exit 0
else
    print_section "$OVERALL_STATUS"
    exit 1
fi
