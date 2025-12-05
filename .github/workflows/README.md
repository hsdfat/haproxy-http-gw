# GitHub Actions Workflows

## HTTP Gateway Tests Workflow

**File:** [gateway-tests.yml](gateway-tests.yml)

Automated test workflow for the HTTP Gateway feature with comprehensive functional and performance testing.

### Triggers

The workflow runs on:

- **Push** to `master`, `main`, or `develop` branches (when gateway-related files change)
- **Pull Requests** to `master`, `main`, or `develop` (when gateway-related files change)
- **Manual trigger** via GitHub UI (workflow_dispatch)

### Monitored Paths

The workflow only runs when these paths are modified:
- `pkg/gateway/**` - Gateway implementation
- `cmd/http-gateway/**` - Gateway binary
- `test/**` - Test system
- `.github/workflows/gateway-tests.yml` - Workflow itself

### Test Stages

#### 1. Environment Setup
- ✅ Checkout code
- ✅ Set up Docker Buildx
- ✅ Cache Docker layers for faster builds
- ✅ Generate SSL certificates
- ✅ Build all Docker images in parallel

#### 2. Service Startup
- ✅ Start all services via docker-compose
- ✅ Wait 30 seconds for initialization
- ✅ Health check gateway (HTTP endpoint)
- ✅ Health check backend API

#### 3. Functional Tests

Runs comprehensive functional test suite:
- Basic HTTP requests
- HTTP/2 protocol support
- Load balancing verification
- Path-based routing
- Host-based routing
- Health checks

**Pass Criteria:**
- All 6 functional tests must pass
- Success rate: 100%

**Fail Criteria:**
- Any test fails
- Workflow exits with error code 1

#### 4. Performance Tests

##### Test 1: Low Concurrency
- **Configuration:** 10 workers, 1000 requests
- **Protocol:** HTTP/1.1
- **Pass Criteria:** Success rate ≥ 99%
- **Metrics Captured:** RPS, latency, success rate

##### Test 2: Medium Concurrency
- **Configuration:** 50 workers, 5000 requests
- **Protocol:** HTTP/1.1
- **Pass Criteria:** Success rate ≥ 98%
- **Metrics Captured:** RPS, latency, success rate

##### Test 3: HTTP/2 Performance
- **Configuration:** 50 workers, 5000 requests
- **Protocol:** HTTP/2 (h2c)
- **Pass Criteria:** Success rate ≥ 98%
- **Metrics Captured:** RPS, latency, success rate
- **Comparison:** Should show improvement over HTTP/1.1

#### 5. Dynamic Backend Test

Tests dynamic backend discovery and updates:
- Add new backend via REST API
- Verify gateway picks up changes
- Confirm backend is available

**Pass Criteria:**
- Backend successfully added
- Gateway routes traffic to new backend

#### 6. Results Collection

- Upload test result artifacts (retained for 30 days)
- Collect Docker container logs
- Capture Docker stats
- Generate test summary

#### 7. Cleanup

- Stop all services
- Remove Docker volumes
- Clean up environment

### Pass/Fail Criteria

#### Overall Pass Criteria

All of the following must be true:
- ✅ All functional tests pass (6/6)
- ✅ Low concurrency performance: Success rate ≥ 99%
- ✅ Medium concurrency performance: Success rate ≥ 98%
- ✅ HTTP/2 performance: Success rate ≥ 98%
- ✅ Dynamic backend update successful
- ✅ No service crashes or errors

#### Failure Conditions

The workflow fails if:
- ❌ Any functional test fails
- ❌ Performance test success rate below threshold
- ❌ Service fails to start or become healthy
- ❌ Backend API not responding
- ❌ Dynamic backend update fails

### Output and Reporting

#### 1. GitHub Step Summary

Automatically generated summary visible in the workflow run:

```markdown
# HTTP Gateway Test Results

## Test Summary

| Test Category | Status | Details |
|--------------|--------|---------|
| Functional Tests | ✅ PASS | All tests passed |
| Performance (Low) | ✅ PASS | RPS: 850.32 |
| Performance (Medium) | ✅ PASS | RPS: 1523.45 |
| HTTP/2 Performance | ✅ PASS | RPS: 1876.23 |
| Dynamic Backend | ✅ PASS | Backend updates working |

## Performance Metrics

| Configuration | Requests/sec |
|--------------|--------------|
| 10 workers, HTTP/1.1 | 850.32 |
| 50 workers, HTTP/1.1 | 1523.45 |
| 50 workers, HTTP/2 | 1876.23 |
```

#### 2. Pull Request Comment

For PRs, automatically posts a comment with results:

```markdown
## ✅ HTTP Gateway Test Results

**Status:** All tests passed!

### Test Results

| Test | Result |
|------|--------|
| Functional Tests | ✅ |
| Performance (10 workers) | ✅ |
| Performance (50 workers) | ✅ |
| HTTP/2 Performance | ✅ |
| Dynamic Backend Updates | ✅ |

### Performance Metrics
...
```

#### 3. Artifacts

Test results uploaded as artifacts (30-day retention):
- `functional-results.txt` - Full functional test output
- `perf-low-results.txt` - Low concurrency performance results
- `perf-medium-results.txt` - Medium concurrency performance results
- `perf-http2-results.txt` - HTTP/2 performance results

### Verification Job

Separate `verify` job that:
- Depends on main `test` job
- Checks overall test status
- Provides final pass/fail determination
- Always runs (even if tests fail)

### Performance Expectations

Based on GitHub Actions runners (2 CPU, 7GB RAM):

| Test | Expected RPS | Expected Success Rate |
|------|--------------|---------------------|
| Low Concurrency | 400-1000 | ≥ 99% |
| Medium Concurrency | 800-2000 | ≥ 98% |
| HTTP/2 | 1000-2500 | ≥ 98% |

### Troubleshooting

#### Workflow Fails at Service Startup

**Issue:** Services fail to become healthy

**Solutions:**
- Check Docker image build logs
- Verify certificate generation
- Increase wait time in workflow
- Check service dependencies

#### Performance Tests Fail

**Issue:** Success rate below threshold

**Possible Causes:**
- GitHub runner resource constraints
- Network issues
- Backend server issues
- Gateway configuration problems

**Solutions:**
- Review service logs in workflow output
- Check Docker stats
- Verify backend servers are responding
- Review HAProxy configuration

#### Dynamic Backend Test Fails

**Issue:** Backend not added or not accessible

**Solutions:**
- Check backend API logs
- Verify REST API endpoint
- Ensure gateway is syncing backends
- Check backend sync period

### Local Testing

Test the workflow behavior locally:

```bash
# Run the same tests locally
cd test
make setup
make test

# Or manually:
docker-compose up -d
docker-compose run --rm test-client /test-client -verbose
docker-compose run --rm test-client /perf-client -c=10 -n=1000
docker-compose down -v
```

### Customization

#### Change Performance Thresholds

Edit the workflow file:

```yaml
# Change from 99% to 95%
if (( $(echo "$SUCCESS_RATE >= 95.0" | bc -l) )); then
```

#### Add More Test Scenarios

Add new steps:

```yaml
- name: Run custom test
  run: |
    cd test
    docker-compose run --rm test-client /perf-client -c=100 -d=60s
```

#### Modify Concurrency Levels

Change test parameters:

```yaml
# Change from 50 to 100 workers
docker-compose run --rm test-client /perf-client -c=100 -n=10000
```

### Monitoring and Metrics

The workflow captures:

- **Test Execution Time:** Total time for all tests
- **Requests per Second:** For each performance test
- **Success Rate:** Percentage of successful requests
- **Service Startup Time:** Time to become healthy
- **Resource Usage:** Docker container stats

### Best Practices

1. **Always Review Logs:** Check workflow logs for any warnings or errors
2. **Monitor Performance Trends:** Track RPS metrics over time
3. **Investigate Flaky Tests:** If tests fail intermittently, investigate root cause
4. **Update Thresholds:** Adjust based on environment capabilities
5. **Keep Dependencies Updated:** Regularly update Docker images and Go modules

### Integration with CI/CD

This workflow integrates with:

- **Branch Protection:** Require tests to pass before merging
- **Status Checks:** Shows pass/fail status on PRs
- **Code Reviews:** Test results help reviewers
- **Deployment Gates:** Can be used as deployment prerequisite

### Required GitHub Actions Permissions

```yaml
permissions:
  contents: read      # Checkout code
  issues: write       # Comment on PRs
  pull-requests: write # Comment on PRs
```

### Environment Variables

```yaml
DOCKER_BUILDKIT: 1              # Enable BuildKit
COMPOSE_DOCKER_CLI_BUILD: 1     # Use BuildKit with Compose
```

### Timeout

- **Workflow Timeout:** 30 minutes
- **Individual Test Timeouts:** Varies per test
- **Service Startup Timeout:** 30 seconds

### Caching

Docker layer caching improves build times:

- **Cache Key:** `${{ runner.os }}-buildx-${{ github.sha }}`
- **Restore Keys:** Previous builds
- **Cache Location:** `/tmp/.buildx-cache`

### Manual Trigger

Run workflow manually from GitHub UI:

1. Go to **Actions** tab
2. Select **HTTP Gateway Tests** workflow
3. Click **Run workflow** button
4. Select branch
5. Click **Run workflow**

### Example Workflow Run

```
✅ Checkout code
✅ Set up Docker Buildx
✅ Generate SSL certificates
✅ Build test environment (2m 15s)
✅ Start test environment (35s)
✅ Check service health (5s)
✅ Run functional tests (8s)
✅ Run performance tests - Low concurrency (3s)
✅ Run performance tests - Medium concurrency (6s)
✅ Run HTTP/2 performance test (6s)
✅ Test dynamic backend updates (8s)
✅ Upload test results
✅ Generate test summary
✅ Comment PR with results
✅ Verify test results

Total time: 3m 45s
Status: Success ✅
```

## Related Documentation

- [Test System Documentation](../../test/README.md)
- [Quick Start Guide](../../test/QUICKSTART.md)
- [HTTP/2 Support](../../test/HTTP2_SUPPORT.md)
- [Gateway Implementation](../../GATEWAY_IMPLEMENTATION.md)
