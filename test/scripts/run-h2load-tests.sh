#!/bin/bash
# Standalone h2load Performance Testing Script with Resource Monitoring
# This script runs h2load benchmarks against the HTTP gateway and monitors resource usage

set -e

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

# Environment variables
export CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
export COMPOSE_CMD="${COMPOSE_CMD:-podman-compose}"
export PATH="/Library/Frameworks/Python.framework/Versions/3.14/bin:$PATH"

# Get the project root directory
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$TEST_DIR"

print_section "h2load HTTP/2 Performance Testing"
echo ""

# Check for h2load
if ! command -v h2load &> /dev/null; then
    print_error "h2load is not installed"
    echo ""
    echo "Install h2load:"
    echo "  macOS:         brew install nghttp2"
    echo "  Ubuntu/Debian: sudo apt-get install nghttp2-client"
    echo "  Fedora/RHEL:   sudo dnf install nghttp2"
    exit 1
fi

print_success "h2load version: $(h2load --version 2>&1 | head -1)"

# Check if services are running
if ! curl -sf http://localhost:9090/health > /dev/null 2>&1; then
    print_error "Gateway is not running. Start services first with:"
    echo "  cd test && podman-compose up -d"
    exit 1
fi
print_success "Gateway is healthy"

# Function to get container resource usage
get_container_stats() {
    local container_name=$1
    if [ "$CONTAINER_RUNTIME" = "podman" ]; then
        podman stats --no-stream --format "{{.MemUsage}} {{.CPUPerc}}" "$container_name" 2>/dev/null || echo "N/A N/A"
    else
        docker stats --no-stream --format "{{.MemUsage}} {{.CPUPerc}}" "$container_name" 2>/dev/null || echo "N/A N/A"
    fi
}

# Get gateway container name
GATEWAY_CONTAINER=$($COMPOSE_CMD ps -q gateway 2>/dev/null | head -1)
if [ -z "$GATEWAY_CONTAINER" ]; then
    print_error "Cannot find gateway container"
    exit 1
fi
print_success "Gateway container: $GATEWAY_CONTAINER"

echo ""
print_section "Test 1: Low Load - Baseline Performance"
print_info "Target: Default frontend (http://localhost:8080/)"
print_info "Config: 1,000 requests, 10 concurrent clients, 10 max streams"
echo ""

STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
echo "Resources before: $STATS_BEFORE"

h2load -n 1000 -c 10 -m 10 http://localhost:8080/ 2>&1 | tee h2load-low-default.txt

STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
echo ""
echo "Resources after:  $STATS_AFTER"
print_success "Low load test completed"

echo ""
print_section "Test 2: Medium Load - Moderate Traffic"
print_info "Target: API frontend (http://localhost:8081/api)"
print_info "Config: 10,000 requests, 50 concurrent clients, 10 max streams"
echo ""

STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
echo "Resources before: $STATS_BEFORE"

h2load -n 10000 -c 50 -m 10 http://localhost:8081/api 2>&1 | tee h2load-medium-api.txt

STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
echo ""
echo "Resources after:  $STATS_AFTER"
print_success "Medium load test completed"

echo ""
print_section "Test 3: High Load - Heavy Traffic"
print_info "Target: Web frontend (http://localhost:8082/)"
print_info "Config: 50,000 requests, 100 concurrent clients, 10 max streams"
echo ""

STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
echo "Resources before: $STATS_BEFORE"

h2load -n 50000 -c 100 -m 10 http://localhost:8082/ 2>&1 | tee h2load-high-web.txt

STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
echo ""
echo "Resources after:  $STATS_AFTER"
print_success "High load test completed"

echo ""
print_section "Test 4: Stress Test - Maximum Capacity"
print_info "Target: All frontends simultaneously"
print_info "Config: 100,000 requests each, 200 concurrent clients, 10 max streams"
print_info "Monitoring: Real-time CPU and RAM tracking"
echo ""

# Start resource monitoring in background
MONITOR_PID=""
if [ "$CONTAINER_RUNTIME" = "podman" ]; then
    (while true; do
        echo "$(date +%s),$(podman stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.NetIO}},{{.BlockIO}}' "$GATEWAY_CONTAINER" 2>/dev/null)"
        sleep 1
    done) > h2load-stress-monitor.csv 2>&1 &
    MONITOR_PID=$!
else
    (while true; do
        echo "$(date +%s),$(docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.NetIO}},{{.BlockIO}}' "$GATEWAY_CONTAINER" 2>/dev/null)"
        sleep 1
    done) > h2load-stress-monitor.csv 2>&1 &
    MONITOR_PID=$!
fi

echo "Started resource monitoring (PID: $MONITOR_PID)"
echo "Timestamp,Name,CPU%,Memory,NetIO,BlockIO" > h2load-stress-monitor-header.csv

# Run stress test on all frontends simultaneously
print_info "Launching stress tests..."
h2load -n 100000 -c 200 -m 10 http://localhost:8080/ > h2load-stress-default.txt 2>&1 &
PID_DEFAULT=$!
echo "  - Default frontend test started (PID: $PID_DEFAULT)"

h2load -n 100000 -c 200 -m 10 http://localhost:8081/api > h2load-stress-api.txt 2>&1 &
PID_API=$!
echo "  - API frontend test started (PID: $PID_API)"

h2load -n 100000 -c 200 -m 10 http://localhost:8082/ > h2load-stress-web.txt 2>&1 &
PID_WEB=$!
echo "  - Web frontend test started (PID: $PID_WEB)"

echo ""
print_info "Waiting for stress tests to complete..."

# Wait for all tests to complete
wait $PID_DEFAULT
print_success "Default frontend stress test completed"

wait $PID_API
print_success "API frontend stress test completed"

wait $PID_WEB
print_success "Web frontend stress test completed"

# Stop monitoring
if [ -n "$MONITOR_PID" ]; then
    kill $MONITOR_PID 2>/dev/null || true
    wait $MONITOR_PID 2>/dev/null || true
fi
print_success "Resource monitoring stopped"

echo ""
print_section "Test Results Summary"
echo ""

# Display key metrics from each test
echo "### Low Load Test (Default Frontend)"
grep -E "requests:|finished in|req/s|time for request:|min/avg/max/stdev" h2load-low-default.txt | head -10
echo ""

echo "### Medium Load Test (API Frontend)"
grep -E "requests:|finished in|req/s|time for request:|min/avg/max/stdev" h2load-medium-api.txt | head -10
echo ""

echo "### High Load Test (Web Frontend)"
grep -E "requests:|finished in|req/s|time for request:|min/avg/max/stdev" h2load-high-web.txt | head -10
echo ""

echo "### Stress Test Results"
for frontend in default api web; do
    echo "#### ${frontend^} Frontend"
    grep -E "requests:|finished in|req/s|time for request:|min/avg/max/stdev" h2load-stress-${frontend}.txt | head -10
    echo ""
done

echo "### Resource Usage During Stress Test"
echo "Sample of resource monitoring data (last 20 entries):"
echo ""
cat h2load-stress-monitor-header.csv
cat h2load-stress-monitor.csv | tail -20
echo ""

print_section "Test Artifacts"
echo ""
echo "Detailed results saved to:"
echo "  - h2load-low-default.txt      (Low load baseline)"
echo "  - h2load-medium-api.txt       (Medium load test)"
echo "  - h2load-high-web.txt         (High load test)"
echo "  - h2load-stress-default.txt   (Stress test - default)"
echo "  - h2load-stress-api.txt       (Stress test - API)"
echo "  - h2load-stress-web.txt       (Stress test - web)"
echo "  - h2load-stress-monitor.csv   (Resource monitoring)"
echo ""

print_success "All h2load tests completed successfully!"
