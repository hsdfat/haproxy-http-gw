#!/bin/bash

set -e

echo "==================================="
echo "HTTP Gateway Test Suite"
echo "==================================="
echo ""

GATEWAY_URL="${GATEWAY_URL:-http://gateway:8080}"
GATEWAY_HTTPS_URL="${GATEWAY_HTTPS_URL:-https://gateway:8443}"

echo "Gateway HTTP URL:  $GATEWAY_URL"
echo "Gateway HTTPS URL: $GATEWAY_HTTPS_URL"
echo ""

# Wait for gateway to be ready
echo "Waiting for gateway to be ready..."
MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -sf "$GATEWAY_URL/health" > /dev/null 2>&1; then
        echo "Gateway is ready!"
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "Attempt $RETRY_COUNT/$MAX_RETRIES..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "ERROR: Gateway did not become ready in time"
    exit 1
fi

echo ""
echo "==================================="
echo "Running Functional Tests"
echo "==================================="
/test-client -gateway="$GATEWAY_URL" -gateway-https="$GATEWAY_HTTPS_URL" -verbose

echo ""
echo "==================================="
echo "Running Performance Tests"
echo "==================================="

echo ""
echo "Test 1: Low concurrency (10 workers, 1000 requests)"
/perf-client -url="$GATEWAY_URL" -c=10 -n=1000

echo ""
echo "Test 2: Medium concurrency (50 workers, 5000 requests)"
/perf-client -url="$GATEWAY_URL" -c=50 -n=5000

echo ""
echo "Test 3: High concurrency (100 workers, 10000 requests)"
/perf-client -url="$GATEWAY_URL" -c=100 -n=10000

echo ""
echo "Test 4: Duration-based test (20 workers, 30 seconds)"
/perf-client -url="$GATEWAY_URL" -c=20 -d=30s

echo ""
echo "==================================="
echo "All Tests Completed Successfully!"
echo "==================================="
