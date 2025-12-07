# Local Testing Guide - GitHub Actions Verification

This guide shows you how to run all GitHub Actions tests locally before pushing to GitHub.

## Quick Start (Simple Test)

The simplest way to verify your setup is working:

```bash
cd test
./run-local-test.sh
```

This script:
- ✅ Checks prerequisites
- ✅ Generates SSL certificates
- ✅ Builds all images
- ✅ Starts services
- ✅ Waits for backends to register
- ✅ Runs basic functional tests

**Expected output:**
```
✓ All functional tests passed!
```

## Full GitHub Actions Test Suite

To run ALL tests exactly as they run in GitHub Actions:

```bash
cd test
./run-github-action-tests.sh
```

This comprehensive script runs:
1. **Functional Tests** - Basic gateway functionality
2. **Performance Tests** - Low concurrency (10 workers, 1000 requests)
3. **Performance Tests** - Medium concurrency (50 workers, 5000 requests)
4. **HTTP/2 Performance** - HTTP/2 protocol test (50 workers, 5000 requests)
5. **Dynamic Backend Tests** - Backend registration/unregistration

### Prerequisites

Install these tools before running the full test suite:

**macOS:**
```bash
brew install jq bc podman
pip3 install podman-compose
```

**Ubuntu/Debian:**
```bash
sudo apt-get install jq bc
pip3 install podman-compose
```

### Understanding Test Results

The script outputs a summary table like this:

```
| Test Category | Status | Details |
|--------------|--------|---------|
| Functional Tests | ✅ PASS | All tests passed |
| Performance (Low) | ✅ PASS | RPS: 1234 |
| Performance (Medium) | ✅ PASS | RPS: 2345 |
| HTTP/2 Performance | ✅ PASS | RPS: 3456 |
| Dynamic Backend | ✅ PASS | Backend updates working |
```

### Performance Benchmarks

Expected performance on different hardware:

| Environment | Low (10c) | Medium (50c) | HTTP/2 (50c) |
|------------|-----------|--------------|--------------|
| GitHub Actions (Ubuntu) | ~800-1000 RPS | ~2000-3000 RPS | ~2500-4000 RPS |
| MacBook Pro M1 | ~1500-2500 RPS | ~4000-6000 RPS | ~5000-8000 RPS |
| AWS t3.medium | ~600-900 RPS | ~1500-2500 RPS | ~2000-3500 RPS |

**Note:** Performance varies based on:
- CPU cores and speed
- Available memory
- System load
- Container runtime (Podman vs Docker)

## Manual Testing Workflow

For development, you can keep services running and test manually:

### 1. Start Services

```bash
cd test
podman-compose up -d
```

### 2. Wait for Services (15-30 seconds)

```bash
# Check if gateway is ready
curl http://localhost:9090/health

# Check registered backends
curl http://localhost:9090/api/backends | jq
```

### 3. Configure Routes (Required for routing tests)

```bash
./scripts/configure-routes.sh
```

This adds:
- `api.example.com/api` → `api-backend`
- `www.example.com/` → `web-backend`

### 4. Run Individual Tests

**Basic HTTP test:**
```bash
curl http://localhost:8080/
```

**With Host header:**
```bash
curl -H "Host: api.example.com" http://localhost:8080/api/test
```

**Load balancing test:**
```bash
for i in {1..10}; do
  curl -s http://localhost:8080/ | jq -r '.server'
done
```

**HTTP/2 test:**
```bash
curl --http2-prior-knowledge http://localhost:8080/
```

### 5. Run Performance Tests

**Low concurrency:**
```bash
podman-compose --profile testing run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=10 \
  -n=1000
```

**Medium concurrency:**
```bash
podman-compose --profile testing run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=50 \
  -n=5000
```

**HTTP/2 performance:**
```bash
podman-compose --profile testing run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -http2 \
  -c=50 \
  -n=5000
```

### 6. View Logs

```bash
# Gateway logs
podman-compose logs -f gateway

# Backend logs
podman-compose logs -f backend-server-1

# All logs
podman-compose logs -f
```

### 7. Cleanup

```bash
# Stop services
podman-compose down

# Remove all data (volumes)
podman-compose down -v
```

## Troubleshooting

### "No backends available" Error

**Symptom:** Requests return 503 with "No backends available"

**Causes:**
1. Routes not configured
2. Backends not registered yet
3. Backend servers not running

**Fix:**
```bash
# Check if backends are registered
curl http://localhost:9090/api/backends | jq

# Configure routes
./scripts/configure-routes.sh

# Restart backend servers if needed
podman-compose restart backend-server-1 backend-server-2 backend-server-3
```

### Gateway Not Starting

**Symptom:** Health check fails

**Check:**
```bash
# View gateway logs
podman-compose logs gateway

# Check if port is available
lsof -i :8080
lsof -i :9090
```

**Fix:**
```bash
# Rebuild and restart
podman-compose down
podman-compose build gateway
podman-compose up -d
```

### Performance Tests Failing

**Symptom:** Success rate < 98%

**Common causes:**
1. System under heavy load
2. Not enough memory
3. Too many concurrent operations

**Fix:**
```bash
# Check system resources
top

# Reduce concurrency
podman-compose run --rm test-client /perf-client \
  -url=http://gateway:8080 \
  -c=5 \
  -n=100
```

### Test Client Not Found

**Symptom:** `missing services [test-client]`

**Cause:** Test client service uses a profile

**Fix:** Use `--profile testing`:
```bash
podman-compose --profile testing run --rm test-client /test-client \
  -gateway=http://gateway:8080 \
  -verbose
```

## Comparing with GitHub Actions

The GitHub Actions workflow (`.github/workflows/gateway-tests.yml`) performs these steps:

1. ✅ Install dependencies (`jq`, `bc`, `podman-compose`)
2. ✅ Generate SSL certificates
3. ✅ Build container images
4. ✅ Start services
5. ✅ Health checks
6. ✅ Wait for backend registration
7. ✅ Run functional tests
8. ✅ Run performance tests (3 variants)
9. ✅ Test dynamic backend updates
10. ✅ Generate summary report

The `run-github-action-tests.sh` script mirrors these exact steps locally.

## Test Artifacts

After running tests, you'll find these result files:

```
test/
├── functional-results.txt      # Functional test output
├── perf-low-results.txt        # Low concurrency performance
├── perf-medium-results.txt     # Medium concurrency performance
└── perf-http2-results.txt      # HTTP/2 performance
```

View them anytime:
```bash
cat functional-results.txt
cat perf-low-results.txt
```

## Next Steps

1. **Run simple test first**: `./run-local-test.sh`
2. **If that passes**, run full suite: `./run-github-action-tests.sh`
3. **Fix any failures** before pushing to GitHub
4. **Push your changes** - GitHub Actions will run the same tests

## Related Documentation

- [TESTING.md](TESTING.md) - Complete testing documentation
- [QUICKSTART.md](QUICKSTART.md) - Quick start guide
- [HTTP2_SUPPORT.md](HTTP2_SUPPORT.md) - HTTP/2 details
- [../BACKEND_REGISTRATION.md](../BACKEND_REGISTRATION.md) - Backend architecture
