#!/bin/bash
set -e
cd test

# Re-register backends with multiple servers to fix load balancing
echo "=== Re-registering Backends with Multiple Servers ==="
./scripts/register-all-backends.sh
sleep 2

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
  echo "functional_tests_passed=true"
else
  echo "functional_tests_passed=false"
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

# Function to get container stats
get_container_stats() {
  local container_name=$1
  if command -v podman &> /dev/null; then
    podman stats --no-stream --format "{{.MemUsage}} {{.CPUPerc}}" "$container_name" 2>/dev/null || echo "N/A N/A"
  elif command -v docker &> /dev/null; then
    docker stats --no-stream --format "{{.MemUsage}} {{.CPUPerc}}" "$container_name" 2>/dev/null || echo "N/A N/A"
  else
    echo "N/A N/A"
  fi
}

GATEWAY_CONTAINER="http-gateway"

# Performance tests - Low concurrency (8 workers, 2,000 requests - all frontends in parallel)
echo "=== Running Performance Tests - Low (All Frontends in Parallel) ==="
STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
echo "Resource usage before test - Memory: $(echo $STATS_BEFORE | awk '{print $1}'), CPU: $(echo $STATS_BEFORE | awk '{print $2}')"

# Run all 3 frontends simultaneously in background
go run ./client/cmd/perf-client/main.go -url=http://localhost:8080 -http2 -c=8 -n=2000 > perf-low-default.txt 2>&1 &
PID_DEFAULT=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8081 -http2 -c=8 -n=2000 > perf-low-api.txt 2>&1 &
PID_API=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8082 -http2 -c=8 -n=2000 > perf-low-web.txt 2>&1 &
PID_WEB=$!

# Wait for all tests to complete
wait $PID_DEFAULT $PID_API $PID_WEB
STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
echo "✓ All low concurrency tests completed"
echo "Resource usage after test - Memory: $(echo $STATS_AFTER | awk '{print $1}'), CPU: $(echo $STATS_AFTER | awk '{print $2}')"

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
  echo "perf_low_passed=true"
  echo "perf_low_default_rps=${PERF_LOW_DEFAULT_RPS}"
  echo "perf_low_api_rps=${PERF_LOW_API_RPS}"
  echo "perf_low_web_rps=${PERF_LOW_WEB_RPS}"
else
  echo "perf_low_passed=false"
  exit 1
fi

# Performance tests - Medium concurrency (16 workers, 20,000 requests - all frontends in parallel)
echo "=== Running Performance Tests - Medium (All Frontends in Parallel) ==="
STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
echo "Resource usage before test - Memory: $(echo $STATS_BEFORE | awk '{print $1}'), CPU: $(echo $STATS_BEFORE | awk '{print $2}')"

# Run all 3 frontends simultaneously in background
go run ./client/cmd/perf-client/main.go -url=http://localhost:8080 -http2 -c=16 -n=20000 > perf-medium-default.txt 2>&1 &
PID_DEFAULT=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8081 -http2 -c=16 -n=20000 > perf-medium-api.txt 2>&1 &
PID_API=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8082 -http2 -c=16 -n=20000 > perf-medium-web.txt 2>&1 &
PID_WEB=$!

# Wait for all tests to complete
wait $PID_DEFAULT $PID_API $PID_WEB
STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
echo "✓ All medium concurrency tests completed"
echo "Resource usage after test - Memory: $(echo $STATS_AFTER | awk '{print $1}'), CPU: $(echo $STATS_AFTER | awk '{print $2}')"

# Display results
echo "=== Default Frontend Results ==="
cat perf-medium-default.txt
echo "=== API Frontend Results ==="
cat perf-medium-api.txt
echo "=== Web Frontend Results ==="
cat perf-medium-web.txt

# Validate results
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
  echo "perf_medium_passed=true"
  echo "perf_medium_default_rps=${PERF_MEDIUM_DEFAULT_RPS}"
  echo "perf_medium_api_rps=${PERF_MEDIUM_API_RPS}"
  echo "perf_medium_web_rps=${PERF_MEDIUM_WEB_RPS}"
else
  echo "perf_medium_passed=false"
  exit 1
fi

# Performance tests - High concurrency (32 workers, 100,000 requests - all frontends in parallel)
echo "=== Running Performance Tests - High (All Frontends in Parallel) ==="

# Start continuous resource monitoring in background
MONITOR_PID=""
if command -v podman &> /dev/null; then
  (while true; do
    podman stats --no-stream --format "{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.NetIO}},{{.BlockIO}}" "$GATEWAY_CONTAINER" 2>/dev/null || echo "$GATEWAY_CONTAINER,N/A,N/A,N/A,N/A"
    sleep 1
  done) > perf-high-monitor.csv 2>&1 &
  MONITOR_PID=$!
elif command -v docker &> /dev/null; then
  (while true; do
    docker stats --no-stream --format "{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.NetIO}},{{.BlockIO}}" "$GATEWAY_CONTAINER" 2>/dev/null || echo "$GATEWAY_CONTAINER,N/A,N/A,N/A,N/A"
    sleep 1
  done) > perf-high-monitor.csv 2>&1 &
  MONITOR_PID=$!
fi

STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
echo "Resource usage before test - Memory: $(echo $STATS_BEFORE | awk '{print $1}'), CPU: $(echo $STATS_BEFORE | awk '{print $2}')"

# Run all 3 frontends simultaneously in background
go run ./client/cmd/perf-client/main.go -url=http://localhost:8080 -http2 -c=32 -n=100000 > perf-high-default.txt 2>&1 &
PID_DEFAULT=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8081 -http2 -c=32 -n=100000 > perf-high-api.txt 2>&1 &
PID_API=$!
go run ./client/cmd/perf-client/main.go -url=http://localhost:8082 -http2 -c=32 -n=100000 > perf-high-web.txt 2>&1 &
PID_WEB=$!

# Wait for all tests to complete
wait $PID_DEFAULT $PID_API $PID_WEB

# Stop monitoring
if [ -n "$MONITOR_PID" ]; then
  kill $MONITOR_PID 2>/dev/null || true
  wait $MONITOR_PID 2>/dev/null || true
fi

STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
echo "✓ All high concurrency tests completed"
echo "Resource usage after test - Memory: $(echo $STATS_AFTER | awk '{print $1}'), CPU: $(echo $STATS_AFTER | awk '{print $2}')"

# Display resource monitoring summary
if [ -f perf-high-monitor.csv ]; then
  echo "=== Resource Monitoring During High Concurrency Test (Last 10 samples) ==="
  tail -10 perf-high-monitor.csv
fi

# Display results
echo "=== Default Frontend Results ==="
cat perf-high-default.txt
echo "=== API Frontend Results ==="
cat perf-high-api.txt
echo "=== Web Frontend Results ==="
cat perf-high-web.txt

# Validate results
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
  echo "perf_http2_passed=true"
  echo "perf_high_default_rps=${PERF_HIGH_DEFAULT_RPS}"
  echo "perf_high_api_rps=${PERF_HIGH_API_RPS}"
  echo "perf_high_web_rps=${PERF_HIGH_WEB_RPS}"
else
  echo "perf_http2_passed=false"
  exit 1
fi

# h2load Performance Tests with CPU/Memory Monitoring
H2LOAD_AVAILABLE=false
if command -v h2load &> /dev/null; then
  H2LOAD_AVAILABLE=true
  echo ""
  echo "=== Running h2load HTTP/2 Performance Tests with Resource Monitoring ==="

  # Function to get container stats
  get_container_stats() {
    local container_name=$1
    if command -v podman &> /dev/null; then
      podman stats --no-stream --format "{{.MemUsage}} {{.CPUPerc}}" "$container_name" 2>/dev/null || echo "N/A N/A"
    elif command -v docker &> /dev/null; then
      docker stats --no-stream --format "{{.MemUsage}} {{.CPUPerc}}" "$container_name" 2>/dev/null || echo "N/A N/A"
    else
      echo "N/A N/A"
    fi
  }

  # Get gateway container name
  GATEWAY_CONTAINER="http-gateway"

  # Test 1: Low load baseline (1000 requests, 10 concurrent clients)
  echo "→ h2load Test 1: Low load baseline (1K req, 10 clients)"
  STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
  h2load -n 1000 -c 10 -m 10 http://localhost:8080/ > h2load-low-default.txt 2>&1 || true
  STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
  echo "  Resource usage - Before: $STATS_BEFORE | After: $STATS_AFTER"

  # Test 2: Medium load (10000 requests, 50 concurrent clients)
  echo "→ h2load Test 2: Medium load (10K req, 50 clients)"
  STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
  h2load -n 10000 -c 50 -m 10 http://localhost:8081/api > h2load-medium-api.txt 2>&1 || true
  STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
  echo "  Resource usage - Before: $STATS_BEFORE | After: $STATS_AFTER"

  # Test 3: High load (50000 requests, 100 concurrent clients)
  echo "→ h2load Test 3: High load (50K req, 100 clients)"
  STATS_BEFORE=$(get_container_stats "$GATEWAY_CONTAINER")
  h2load -n 50000 -c 100 -m 10 http://localhost:8082/ > h2load-high-web.txt 2>&1 || true
  STATS_AFTER=$(get_container_stats "$GATEWAY_CONTAINER")
  echo "  Resource usage - Before: $STATS_BEFORE | After: $STATS_AFTER"

  # Test 4: Stress test with continuous resource monitoring
  echo "→ h2load Test 4: Stress test with resource monitoring (100K req, 200 clients)"

  # Start resource monitoring in background
  MONITOR_PID=""
  if command -v podman &> /dev/null; then
    (while true; do
      podman stats --no-stream --format "{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.NetIO}},{{.BlockIO}}" "$GATEWAY_CONTAINER" 2>/dev/null
      sleep 1
    done) > h2load-stress-monitor.csv 2>&1 &
    MONITOR_PID=$!
  elif command -v docker &> /dev/null; then
    (while true; do
      docker stats --no-stream --format "{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.NetIO}},{{.BlockIO}}" "$GATEWAY_CONTAINER" 2>/dev/null
      sleep 1
    done) > h2load-stress-monitor.csv 2>&1 &
    MONITOR_PID=$!
  fi

  # Run stress tests on all frontends simultaneously
  (h2load -n 100000 -c 200 -m 10 http://localhost:8080/ > h2load-stress-default.txt 2>&1 || true) &
  PID_H2_DEFAULT=$!
  (h2load -n 100000 -c 200 -m 10 http://localhost:8081/api > h2load-stress-api.txt 2>&1 || true) &
  PID_H2_API=$!
  (h2load -n 100000 -c 200 -m 10 http://localhost:8082/ > h2load-stress-web.txt 2>&1 || true) &
  PID_H2_WEB=$!

  # Wait for all tests to complete
  wait $PID_H2_DEFAULT $PID_H2_API $PID_H2_WEB || true

  # Stop monitoring
  if [ -n "$MONITOR_PID" ]; then
    kill $MONITOR_PID 2>/dev/null || true
    wait $MONITOR_PID 2>/dev/null || true
  fi

  echo "✓ h2load tests completed"

  # Display stress test results
  echo ""
  echo "=== h2load Stress Test Results ==="
  if [ -f h2load-stress-default.txt ]; then
    echo "--- Default Frontend ---"
    grep -E "finished in|requests:|status codes:" h2load-stress-default.txt | head -10
  fi
  if [ -f h2load-stress-api.txt ]; then
    echo "--- API Frontend ---"
    grep -E "finished in|requests:|status codes:" h2load-stress-api.txt | head -10
  fi
  if [ -f h2load-stress-web.txt ]; then
    echo "--- Web Frontend ---"
    grep -E "finished in|requests:|status codes:" h2load-stress-web.txt | head -10
  fi

  # Display resource monitoring summary
  if [ -f h2load-stress-monitor.csv ]; then
    echo ""
    echo "=== Resource Monitoring During Stress Test (Last 10 samples) ==="
    echo "Name,CPU%,Memory,NetIO,BlockIO"
    tail -10 h2load-stress-monitor.csv
  fi

  echo "h2load_tests_passed=true"
else
  echo ""
  echo "=== h2load not available, skipping HTTP/2 benchmark tests ==="
  echo "Install with: brew install nghttp2 (macOS) or apt-get install nghttp2-client (Linux)"
  echo "h2load_tests_passed=skipped"
fi

# Dynamic backend test
echo ""
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
      echo "dynamic_backend_passed=true"
    else
      echo "dynamic_backend_passed=false"
      exit 1
    fi
  else
    echo "dynamic_backend_passed=false"
    exit 1
  fi
else
  echo "dynamic_backend_passed=false"
  exit 1
fi

echo ""
echo "=== ALL TESTS PASSED ==="
