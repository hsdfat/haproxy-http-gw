#!/bin/bash
# Standalone Dynamic Backend Registration/Deregistration Test Script
# Runs against a simple local HAProxy HTTP Gateway container
# Tests backend server list changes with multiple register/deregister cycles
# Verifies changes via HAProxy config file and runtime socket

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
GATEWAY_API="${GATEWAY_API:-http://localhost:9090}"
FRONTEND_ID="${FRONTEND_ID:-default}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
GATEWAY_CONTAINER="${GATEWAY_CONTAINER:-test-gateway-1}"

# Note: Backend deletion is a two-phase process in HAProxy gateway:
# 1. BackendDelete() marks backend as unused
# 2. BackendDeleteAllUnnecessary() (called during APIFinalCommitTransaction) actually removes it
# We trigger a transaction after deletion to complete the cleanup process

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

print_subsection() {
    echo -e "${CYAN}--- $1${NC}"
}

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Function to verify backend via API
verify_backend_via_api() {
    local backend_name=$1
    local should_exist=$2  # "true" or "false"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    print_info "Verifying backend '$backend_name' via API (should_exist=$should_exist)..."

    RESPONSE=$(curl -sf "$GATEWAY_API/api/frontends/$FRONTEND_ID/backends" 2>/dev/null || echo '{"success":false}')

    if [ "$should_exist" = "true" ]; then
        if echo "$RESPONSE" | grep -q "\"$backend_name\""; then
            print_success "Backend '$backend_name' found in API response"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        else
            print_error "Backend '$backend_name' NOT found in API response (expected to exist)"
            echo "Response: $RESPONSE"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            return 1
        fi
    else
        if echo "$RESPONSE" | grep -q "\"$backend_name\""; then
            print_error "Backend '$backend_name' found in API response (expected NOT to exist)"
            echo "Response: $RESPONSE"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            return 1
        else
            print_success "Backend '$backend_name' NOT found in API response (as expected)"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        fi
    fi
}

# Function to verify backend via HAProxy config file
verify_backend_via_config() {
    local backend_name=$1
    local should_exist=$2  # "true" or "false"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    print_info "Verifying backend '$backend_name' in HAProxy config file (should_exist=$should_exist)..."

    # Get HAProxy config from container
    CONFIG=$($CONTAINER_RUNTIME exec $GATEWAY_CONTAINER cat /etc/haproxy/haproxy.cfg 2>/dev/null || echo "")

    if [ -z "$CONFIG" ]; then
        print_error "Failed to read HAProxy config file"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi

    if [ "$should_exist" = "true" ]; then
        if echo "$CONFIG" | grep -q "backend $backend_name"; then
            print_success "Backend '$backend_name' found in config file"
            # Show the backend section
            echo "$CONFIG" | sed -n "/^backend $backend_name/,/^backend\|^frontend\|^listen\|^$/p" | head -20
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        else
            print_error "Backend '$backend_name' NOT found in config file (expected to exist)"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            return 1
        fi
    else
        if echo "$CONFIG" | grep -q "backend $backend_name"; then
            print_error "Backend '$backend_name' found in config file (expected NOT to exist)"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            return 1
        else
            print_success "Backend '$backend_name' NOT found in config file (as expected)"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        fi
    fi
}

# Function to verify backend via HAProxy runtime socket
verify_backend_via_socket() {
    local backend_name=$1
    local should_exist=$2  # "true" or "false"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    print_info "Verifying backend '$backend_name' via HAProxy runtime socket (should_exist=$should_exist)..."

    # Query HAProxy socket for backend info
    # Try both common socket locations
    SOCKET_OUTPUT=$($CONTAINER_RUNTIME exec $GATEWAY_CONTAINER sh -c "echo 'show backend' | socat stdio /tmp/haproxy-gateway/haproxy-runtime-api.sock" 2>/dev/null || \
                    $CONTAINER_RUNTIME exec $GATEWAY_CONTAINER sh -c "echo 'show backend' | socat stdio /var/run/haproxy-runtime-api.sock" 2>/dev/null || echo "")

    if [ -z "$SOCKET_OUTPUT" ]; then
        print_error "Failed to query HAProxy runtime socket"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi

    if [ "$should_exist" = "true" ]; then
        if echo "$SOCKET_OUTPUT" | grep -q "$backend_name"; then
            print_success "Backend '$backend_name' found in runtime socket output"
            # Show server details
            print_info "Querying servers in backend '$backend_name'..."
            SERVER_OUTPUT=$($CONTAINER_RUNTIME exec $GATEWAY_CONTAINER sh -c "echo 'show servers state $backend_name' | socat stdio /tmp/haproxy-gateway/haproxy-runtime-api.sock" 2>/dev/null || \
                            $CONTAINER_RUNTIME exec $GATEWAY_CONTAINER sh -c "echo 'show servers state $backend_name' | socat stdio /var/run/haproxy-runtime-api.sock" 2>/dev/null || echo "")
            echo "$SERVER_OUTPUT" | head -10
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        else
            print_error "Backend '$backend_name' NOT found in runtime socket (expected to exist)"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            return 1
        fi
    else
        if echo "$SOCKET_OUTPUT" | grep -q "$backend_name"; then
            print_error "Backend '$backend_name' found in runtime socket (expected NOT to exist)"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            return 1
        else
            print_success "Backend '$backend_name' NOT found in runtime socket (as expected)"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        fi
    fi
}

# Function to register a backend
register_backend() {
    local backend_name=$1
    local server_name=$2
    local server_ip=$3
    local server_port=$4

    print_info "Registering backend '$backend_name' with server '$server_name' ($server_ip:$server_port)..."

    RESPONSE=$(curl -sf -X POST "$GATEWAY_API/api/frontends/$FRONTEND_ID/backends" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$backend_name\",\"servers\":[{\"name\":\"$server_name\",\"ip\":\"$server_ip\",\"port\":$server_port}]}" 2>/dev/null || echo '{"success":false}')

    if echo "$RESPONSE" | grep -q '"success":true'; then
        print_success "Backend '$backend_name' registered successfully"
        echo "Response: $RESPONSE"
        return 0
    else
        print_error "Failed to register backend '$backend_name'"
        echo "Response: $RESPONSE"
        return 1
    fi
}

# Function to register a backend with multiple servers
register_backend_multi_server() {
    local backend_name=$1
    shift
    local servers_json="["
    local first=true

    for server_def in "$@"; do
        IFS=':' read -r server_name server_ip server_port <<< "$server_def"
        if [ "$first" = false ]; then
            servers_json+=","
        fi
        servers_json+="{\"name\":\"$server_name\",\"ip\":\"$server_ip\",\"port\":$server_port}"
        first=false
    done
    servers_json+="]"

    print_info "Registering backend '$backend_name' with ${#@} servers..."

    RESPONSE=$(curl -sf -X POST "$GATEWAY_API/api/frontends/$FRONTEND_ID/backends" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$backend_name\",\"servers\":$servers_json}" 2>/dev/null || echo '{"success":false}')

    if echo "$RESPONSE" | grep -q '"success":true'; then
        print_success "Backend '$backend_name' registered with multiple servers"
        echo "Response: $RESPONSE"
        return 0
    else
        print_error "Failed to register backend '$backend_name'"
        echo "Response: $RESPONSE"
        return 1
    fi
}

# Function to deregister a backend
deregister_backend() {
    local backend_name=$1

    print_info "Deregistering backend '$backend_name'..."

    RESPONSE=$(curl -sf -X DELETE "$GATEWAY_API/api/frontends/$FRONTEND_ID/backends/$backend_name" 2>/dev/null || echo '{"success":false}')

    if echo "$RESPONSE" | grep -q '"success":true'; then
        print_success "Backend '$backend_name' deregistered successfully"
        echo "Response: $RESPONSE"

        # Trigger a transaction to commit the deletion
        # This is the normal flow - deletion completes during the next transaction
        print_info "Triggering transaction to commit deletion..."
        DUMMY_NAME="dummy-trigger-$(date +%s)"
        curl -sf -X POST "$GATEWAY_API/api/frontends/$FRONTEND_ID/backends" \
            -H "Content-Type: application/json" \
            -d "{\"name\":\"$DUMMY_NAME\",\"servers\":[{\"name\":\"dummy\",\"ip\":\"127.0.0.1\",\"port\":1}]}" >/dev/null 2>&1
        sleep 1
        curl -sf -X DELETE "$GATEWAY_API/api/frontends/$FRONTEND_ID/backends/$DUMMY_NAME" >/dev/null 2>&1
        sleep 2  # Give time for transaction to complete

        return 0
    else
        print_error "Failed to deregister backend '$backend_name'"
        echo "Response: $RESPONSE"
        return 1
    fi
}

# Main test execution
print_section "Dynamic Backend Registration/Deregistration Tests"
echo ""
print_info "Test Configuration:"
echo "  Gateway API: $GATEWAY_API"
echo "  Frontend ID: $FRONTEND_ID"
echo "  Container Runtime: $CONTAINER_RUNTIME"
echo "  Gateway Container: $GATEWAY_CONTAINER"
echo ""

# Check prerequisites
print_info "Checking prerequisites..."
if ! curl -sf "$GATEWAY_API/health" > /dev/null 2>&1; then
    print_error "Gateway API is not accessible at $GATEWAY_API"
    echo ""
    echo "Please ensure the gateway is running. Example:"
    echo "  cd test && podman-compose up -d gateway"
    echo ""
    echo "Or set GATEWAY_API environment variable:"
    echo "  export GATEWAY_API=http://localhost:9090"
    exit 1
fi
print_success "Gateway API is accessible"

# Verify container is running
if ! $CONTAINER_RUNTIME ps | grep -q "$GATEWAY_CONTAINER"; then
    print_error "Gateway container '$GATEWAY_CONTAINER' is not running"
    echo ""
    echo "Available containers:"
    $CONTAINER_RUNTIME ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    echo ""
    echo "You can set GATEWAY_CONTAINER environment variable:"
    echo "  export GATEWAY_CONTAINER=your-container-name"
    exit 1
fi
print_success "Gateway container is running"

echo ""

# ======================================================================
# TEST CYCLE 1: Single Server Backend
# ======================================================================
print_section "Test Cycle 1: Single Server Backend"
echo ""

print_subsection "1.1: Register backend 'test-backend-1' with single server"
register_backend "test-backend-1" "srv1" "127.0.0.1" "8080"
sleep 3

print_subsection "1.2: Verify backend exists via API"
verify_backend_via_api "test-backend-1" "true"

print_subsection "1.3: Verify backend exists in config file"
verify_backend_via_config "test-backend-1" "true"

print_subsection "1.4: Verify backend exists via runtime socket"
verify_backend_via_socket "test-backend-1" "true"

print_subsection "1.5: Deregister backend 'test-backend-1'"
deregister_backend "test-backend-1"
sleep 3

print_subsection "1.6: Verify backend removed via API"
verify_backend_via_api "test-backend-1" "false"

print_subsection "1.7: Verify backend removed from config file"
verify_backend_via_config "test-backend-1" "false"

print_subsection "1.8: Verify backend removed via runtime socket"
verify_backend_via_socket "test-backend-1" "false"

echo ""

# ======================================================================
# TEST CYCLE 2: Multiple Server Backend
# ======================================================================
print_section "Test Cycle 2: Multiple Server Backend"
echo ""

print_subsection "2.1: Register backend 'test-backend-2' with 3 servers"
register_backend_multi_server "test-backend-2" \
    "srv1:127.0.0.1:8080" \
    "srv2:127.0.0.1:8081" \
    "srv3:127.0.0.1:8082"
sleep 3

print_subsection "2.2: Verify backend exists via API"
verify_backend_via_api "test-backend-2" "true"

print_subsection "2.3: Verify backend exists in config file"
verify_backend_via_config "test-backend-2" "true"

print_subsection "2.4: Verify backend exists via runtime socket"
verify_backend_via_socket "test-backend-2" "true"

print_subsection "2.5: Deregister backend 'test-backend-2'"
deregister_backend "test-backend-2"
sleep 3

print_subsection "2.6: Verify backend removed via API"
verify_backend_via_api "test-backend-2" "false"

print_subsection "2.7: Verify backend removed from config file"
verify_backend_via_config "test-backend-2" "false"

print_subsection "2.8: Verify backend removed via runtime socket"
verify_backend_via_socket "test-backend-2" "false"

echo ""

# ======================================================================
# TEST CYCLE 3: Multiple Rapid Register/Deregister Cycles
# ======================================================================
print_section "Test Cycle 3: Rapid Register/Deregister (5 cycles)"
echo ""

for i in {1..5}; do
    print_subsection "3.$i: Cycle $i - Register and verify"

    register_backend "test-backend-rapid-$i" "srv-$i" "127.0.0.1" "808$i"
    sleep 2

    verify_backend_via_api "test-backend-rapid-$i" "true"
    verify_backend_via_socket "test-backend-rapid-$i" "true"

    deregister_backend "test-backend-rapid-$i"
    sleep 2

    verify_backend_via_api "test-backend-rapid-$i" "false"
    verify_backend_via_socket "test-backend-rapid-$i" "false"

    echo ""
done

# ======================================================================
# TEST CYCLE 4: Concurrent Multiple Backends
# ======================================================================
print_section "Test Cycle 4: Concurrent Multiple Backends"
echo ""

print_subsection "4.1: Register 3 backends concurrently"
register_backend "concurrent-backend-1" "srv1" "127.0.0.1" "9001" &
PID1=$!
register_backend "concurrent-backend-2" "srv2" "127.0.0.1" "9002" &
PID2=$!
register_backend "concurrent-backend-3" "srv3" "127.0.0.1" "9003" &
PID3=$!

wait $PID1 $PID2 $PID3
sleep 5

print_subsection "4.2: Verify all 3 backends exist"
verify_backend_via_api "concurrent-backend-1" "true"
verify_backend_via_api "concurrent-backend-2" "true"
verify_backend_via_api "concurrent-backend-3" "true"

verify_backend_via_socket "concurrent-backend-1" "true"
verify_backend_via_socket "concurrent-backend-2" "true"
verify_backend_via_socket "concurrent-backend-3" "true"

print_subsection "4.3: Deregister all 3 backends"
deregister_backend "concurrent-backend-1"
deregister_backend "concurrent-backend-2"
deregister_backend "concurrent-backend-3"
sleep 5

print_subsection "4.4: Verify all 3 backends removed"
verify_backend_via_api "concurrent-backend-1" "false"
verify_backend_via_api "concurrent-backend-2" "false"
verify_backend_via_api "concurrent-backend-3" "false"

verify_backend_via_socket "concurrent-backend-1" "false"
verify_backend_via_socket "concurrent-backend-2" "false"
verify_backend_via_socket "concurrent-backend-3" "false"

echo ""

# ======================================================================
# TEST CYCLE 5: Re-registration with Different Configuration
# ======================================================================
print_section "Test Cycle 5: Re-registration with Different Configuration"
echo ""

print_subsection "5.1: Register backend 'reconfig-backend' with 1 server"
register_backend "reconfig-backend" "srv1" "127.0.0.1" "7000"
sleep 3
verify_backend_via_api "reconfig-backend" "true"

print_subsection "5.2: Deregister backend"
deregister_backend "reconfig-backend"
sleep 3
verify_backend_via_api "reconfig-backend" "false"

print_subsection "5.3: Re-register same backend with different servers (3 servers)"
register_backend_multi_server "reconfig-backend" \
    "srv1:127.0.0.1:7001" \
    "srv2:127.0.0.1:7002" \
    "srv3:127.0.0.1:7003"
sleep 3

print_subsection "5.4: Verify re-registered backend"
verify_backend_via_api "reconfig-backend" "true"
verify_backend_via_config "reconfig-backend" "true"
verify_backend_via_socket "reconfig-backend" "true"

print_subsection "5.5: Cleanup - Deregister backend"
deregister_backend "reconfig-backend"
sleep 3
verify_backend_via_api "reconfig-backend" "false"

echo ""

# ======================================================================
# Final Report
# ======================================================================
print_section "Test Results Summary"
echo ""

echo "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed Tests: $PASSED_TESTS${NC}"
echo -e "${RED}Failed Tests: $FAILED_TESTS${NC}"
echo ""

SUCCESS_RATE=0
if [ $TOTAL_TESTS -gt 0 ]; then
    SUCCESS_RATE=$(echo "scale=2; $PASSED_TESTS * 100 / $TOTAL_TESTS" | bc)
fi

echo "Success Rate: $SUCCESS_RATE%"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    print_section "✅ ALL TESTS PASSED"
    exit 0
else
    print_section "❌ SOME TESTS FAILED"
    exit 1
fi
