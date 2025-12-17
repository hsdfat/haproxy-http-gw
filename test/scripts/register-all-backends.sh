#!/bin/bash
# Register all backend servers with the gateway
# This script waits for all backend containers to be ready, then registers them as multi-server backends

set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:9090}"
MAX_RETRIES=30
RETRY_DELAY=2

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}→ $1${NC}"
}

# Wait for gateway
print_info "Waiting for gateway..."
for i in $(seq 1 $MAX_RETRIES); do
    if curl -sf "$GATEWAY_URL/health" > /dev/null 2>&1; then
        print_success "Gateway is ready"
        break
    fi
    sleep $RETRY_DELAY
done

# Get IPs of backend servers (using docker/podman network)
print_info "Discovering backend server IPs..."

# Function to get container IP
get_container_ip() {
    local container_name=$1
    podman inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container_name" 2>/dev/null || \
    docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container_name" 2>/dev/null || \
    echo ""
}

# Get IPs
BACKEND_1_IP=$(get_container_ip "backend-server-1")
BACKEND_2_IP=$(get_container_ip "backend-server-2")
BACKEND_3_IP=$(get_container_ip "backend-server-3")
WEB_1_IP=$(get_container_ip "web-server-1")
WEB_2_IP=$(get_container_ip "web-server-2")
API_V2_1_IP=$(get_container_ip "api-v2-server-1")
API_V2_2_IP=$(get_container_ip "api-v2-server-2")
WEB_V2_1_IP=$(get_container_ip "web-v2-server-1")
WEB_V2_2_IP=$(get_container_ip "web-v2-server-2")

print_info "Backend Server IPs:"
echo "  backend-server-1: $BACKEND_1_IP"
echo "  backend-server-2: $BACKEND_2_IP"
echo "  backend-server-3: $BACKEND_3_IP"
echo "  web-server-1: $WEB_1_IP"
echo "  web-server-2: $WEB_2_IP"
echo "  api-v2-server-1: $API_V2_1_IP"
echo "  api-v2-server-2: $API_V2_2_IP"
echo "  web-v2-server-1: $WEB_V2_1_IP"
echo "  web-v2-server-2: $WEB_V2_2_IP"

# Wait a bit for servers to start
sleep 3

# Register api-backend with 3 servers (for default frontend)
print_info "Registering api-backend with 3 servers..."
curl -sf -X POST "$GATEWAY_URL/api/frontends/default/backends" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"api-backend\",
        \"servers\": [
            {\"name\": \"backend-server-1\", \"ip\": \"$BACKEND_1_IP\", \"port\": 9000},
            {\"name\": \"backend-server-2\", \"ip\": \"$BACKEND_2_IP\", \"port\": 9000},
            {\"name\": \"backend-server-3\", \"ip\": \"$BACKEND_3_IP\", \"port\": 9000}
        ]
    }" > /dev/null && print_success "api-backend registered with 3 servers"

# Register web-backend with 2 servers (for default frontend)
print_info "Registering web-backend with 2 servers..."
curl -sf -X POST "$GATEWAY_URL/api/frontends/default/backends" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"web-backend\",
        \"servers\": [
            {\"name\": \"web-server-1\", \"ip\": \"$WEB_1_IP\", \"port\": 9000},
            {\"name\": \"web-server-2\", \"ip\": \"$WEB_2_IP\", \"port\": 9000}
        ]
    }" > /dev/null && print_success "web-backend registered with 2 servers"

# Register api-v2-backend with 2 servers (for frontend-api)
print_info "Registering api-v2-backend with 2 servers..."
curl -sf -X POST "$GATEWAY_URL/api/frontends/frontend-api/backends" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"api-v2-backend\",
        \"servers\": [
            {\"name\": \"api-v2-server-1\", \"ip\": \"$API_V2_1_IP\", \"port\": 9000},
            {\"name\": \"api-v2-server-2\", \"ip\": \"$API_V2_2_IP\", \"port\": 9000}
        ]
    }" > /dev/null && print_success "api-v2-backend registered with 2 servers"

# Register web-v2-backend with 2 servers (for frontend-web)
print_info "Registering web-v2-backend with 2 servers..."
curl -sf -X POST "$GATEWAY_URL/api/frontends/frontend-web/backends" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"web-v2-backend\",
        \"servers\": [
            {\"name\": \"web-v2-server-1\", \"ip\": \"$WEB_V2_1_IP\", \"port\": 9000},
            {\"name\": \"web-v2-server-2\", \"ip\": \"$WEB_V2_2_IP\", \"port\": 9000}
        ]
    }" > /dev/null && print_success "web-v2-backend registered with 2 servers"

print_success "All backends registered successfully!"
