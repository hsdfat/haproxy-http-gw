# Testing Guide for HTTP Gateway

## Overview

This document describes the automated testing strategy for the HTTP Gateway feature.

## Test Automation

### GitHub Actions Workflow

**Status:** ![Gateway Tests](https://github.com/YOUR_ORG/haproxy-http-gw/workflows/HTTP%20Gateway%20Tests/badge.svg)

The HTTP Gateway has a fully automated test workflow that runs on:
- Push to main branches
- Pull requests
- Manual trigger

### Test Coverage

```
┌─────────────────────────────────────────┐
│        HTTP Gateway Test Coverage       │
├─────────────────────────────────────────┤
│ Functional Tests              │    6/6  │
│ Performance Tests             │    3/3  │
│ Dynamic Backend Tests         │    1/1  │
│ Integration Tests             │   10/10 │
├─────────────────────────────────────────┤
│ Total Coverage                │   100%  │
└─────────────────────────────────────────┘
```

## Test Pyramid

```
        ┌───────────┐
       ╱   E2E (1)   ╲      Dynamic Backend Integration
      ╱───────────────╲
     ╱   Integration   ╲    Functional Tests (6)
    ╱      (10)         ╲
   ╱─────────────────────╲
  ╱   Performance (3)     ╲  Load & HTTP/2 Tests
 ╱─────────────────────────╲
└───────────────────────────┘
```

## Test Types

### 1. Functional Tests (6 tests)

Tests core gateway functionality:

| # | Test | Description | Pass Criteria |
|---|------|-------------|---------------|
| 1 | Basic HTTP | Simple HTTP request/response | Status 200, valid response |
| 2 | HTTP/2 Support | Protocol negotiation | Protocol = HTTP/2 |
| 3 | Load Balancing | Traffic distribution | Multiple servers receive traffic |
| 4 | Path Routing | Path-based routing | Correct backend for path |
| 5 | Host Routing | Host-based routing | Correct backend for host |
| 6 | Health Check | Service health | Status 200 |

**Execution Time:** ~8 seconds
**Pass Rate Required:** 100% (6/6 tests must pass)

### 2. Performance Tests (3 tests)

Tests gateway performance under load:

| # | Test | Workers | Requests | Protocol | Pass Criteria |
|---|------|---------|----------|----------|---------------|
| 1 | Low Load | 10 | 1,000 | HTTP/1.1 | Success ≥ 99% |
| 2 | Medium Load | 50 | 5,000 | HTTP/1.1 | Success ≥ 98% |
| 3 | HTTP/2 Load | 50 | 5,000 | HTTP/2 | Success ≥ 98% |

**Execution Time:** ~12-15 seconds total
**Metrics Captured:**
- Requests per second (RPS)
- Average latency
- Min/max latency
- Success rate

### 3. Dynamic Backend Test (1 test)

Tests real-time backend discovery:

| Test | Description | Pass Criteria |
|------|-------------|---------------|
| Backend Update | Add backend via API | Backend routable within 5s |

**Execution Time:** ~8 seconds

### 4. Integration Tests (10 tests)

End-to-end integration scenarios:

1. ✅ Service startup and health
2. ✅ Certificate generation
3. ✅ Docker image builds
4. ✅ Network connectivity
5. ✅ Backend API operations
6. ✅ Gateway configuration
7. ✅ HAProxy integration
8. ✅ Backend server responses
9. ✅ Log collection
10. ✅ Cleanup and teardown

## Pass/Fail Criteria

### ✅ Overall Pass Conditions

**ALL** of the following must be true:

```
✓ Functional Tests     → 6/6 passing (100%)
✓ Performance Low      → Success rate ≥ 99%
✓ Performance Medium   → Success rate ≥ 98%
✓ Performance HTTP/2   → Success rate ≥ 98%
✓ Dynamic Backend      → Update successful
✓ All services healthy → No crashes or errors
```

### ❌ Failure Conditions

**ANY** of the following causes failure:

```
✗ Any functional test fails
✗ Performance success rate below threshold
✗ Service fails to start within 60s
✗ Backend API not responding
✗ Dynamic update takes > 10s
✗ Memory/CPU limits exceeded
```

## Test Results Verification

### Automated Verification

The workflow automatically verifies:

1. **Test Execution:** All tests complete without errors
2. **Success Rates:** Meet or exceed thresholds
3. **Service Health:** All services remain healthy
4. **Resource Usage:** Within acceptable limits
5. **Logs:** No critical errors or warnings

### Manual Verification

For critical releases, manually verify:

```bash
# 1. Check workflow status
gh workflow view "HTTP Gateway Tests"

# 2. Download and review artifacts
gh run download <run-id>

# 3. Review logs
cat functional-results.txt
cat perf-medium-results.txt

# 4. Verify metrics
grep "Requests/sec" perf-*.txt
grep "Successful" perf-*.txt
```

## Performance Benchmarks

### Expected Results (GitHub Actions)

| Environment | Workers | Protocol | Expected RPS | Success Rate |
|------------|---------|----------|--------------|--------------|
| GitHub Actions (2 CPU) | 10 | HTTP/1.1 | 400-1000 | ≥ 99% |
| GitHub Actions (2 CPU) | 50 | HTTP/1.1 | 800-2000 | ≥ 98% |
| GitHub Actions (2 CPU) | 50 | HTTP/2 | 1000-2500 | ≥ 98% |

### Expected Results (Local Dev)

| Environment | Workers | Protocol | Expected RPS | Success Rate |
|------------|---------|----------|--------------|--------------|
| Local (4 CPU, 8GB) | 10 | HTTP/1.1 | 500-1200 | ≥ 99% |
| Local (4 CPU, 8GB) | 50 | HTTP/1.1 | 1000-2500 | ≥ 99% |
| Local (4 CPU, 8GB) | 100 | HTTP/2 | 2000-4000 | ≥ 98% |

## Test Reports

### Workflow Summary

Available in GitHub Actions UI:

- Overall status (Pass/Fail)
- Individual test results
- Performance metrics table
- Execution time per stage
- Resource usage stats

### Artifacts

Downloaded from workflow runs:

- `functional-results.txt` - Detailed functional test output
- `perf-low-results.txt` - Low concurrency results
- `perf-medium-results.txt` - Medium concurrency results
- `perf-http2-results.txt` - HTTP/2 performance results

### Pull Request Comments

Automatically posted on PRs:

```markdown
## ✅ HTTP Gateway Test Results

**Status:** All tests passed!

### Test Results
[Table with all test statuses]

### Performance Metrics
[Table with RPS metrics]
```

## Running Tests Locally

### Quick Test

```bash
cd test
make test-quick
```

### Full Test Suite

```bash
cd test
make test
```

### Individual Tests

```bash
# Functional only
make test-functional

# Performance only
make test-perf

# Specific performance test
docker compose run --rm test-client /perf-client -c=50 -n=5000
```

## Debugging Failed Tests

### Step 1: Identify Failure

```bash
# View workflow logs
gh run view <run-id>

# Download artifacts
gh run download <run-id>
```

### Step 2: Reproduce Locally

```bash
cd test
make setup
make test
```

### Step 3: Check Logs

```bash
# Gateway logs
docker compose logs gateway

# Backend API logs
docker compose logs backend-api

# All logs
make logs
```

### Step 4: Verify Services

```bash
# Service status
docker compose ps

# Health checks
curl http://localhost:8080/health
curl http://localhost:8000/health

# Backend list
curl http://localhost:8000/backends | jq
```

### Step 5: Test Manually

```bash
# Manual request
curl -H "Host: api.example.com" http://localhost:8080/api/test

# Check response
curl -v http://localhost:8080/api/test | jq
```

## Test Maintenance

### Updating Thresholds

If environment performance changes:

**File:** `.github/workflows/gateway-tests.yml`

```yaml
# Change success rate threshold
if (( $(echo "$SUCCESS_RATE >= 99.0" | bc -l) )); then  # Change 99.0
```

### Adding New Tests

1. Add test function to test client
2. Update workflow to run new test
3. Define pass/fail criteria
4. Document in this guide

### Modifying Test Parameters

```yaml
# Change concurrency
-c=50  # Change this value

# Change request count
-n=5000  # Change this value

# Change duration
-d=30s  # Change this value
```

## CI/CD Integration

### Branch Protection

Recommended settings:

- ✅ Require status checks to pass
- ✅ Require "HTTP Gateway Tests" to pass
- ✅ Require up-to-date branches
- ❌ Allow force push (disabled)

### Merge Conditions

Before merging, ensure:

1. ✅ All tests pass
2. ✅ No test failures in last 3 runs
3. ✅ Performance within acceptable range
4. ✅ No critical warnings in logs
5. ✅ Code reviewed and approved

### Deployment Gates

Use test results as deployment gates:

```yaml
deploy:
  needs: test
  if: needs.test.result == 'success'
  steps:
    - name: Deploy to production
      run: ./deploy.sh
```

## Test Metrics and Trends

### Track Over Time

Monitor these metrics:

- Test success rate (should be ~100%)
- Performance RPS (track trends)
- Test execution time (watch for increases)
- Service startup time (should be consistent)
- Resource usage (CPU/memory)

### Performance Regression Detection

Alert if:

- RPS drops > 20% from baseline
- Success rate drops below 98%
- Test execution time increases > 50%
- Any test becomes consistently flaky

## Best Practices

### For Developers

1. ✅ Run tests locally before pushing
2. ✅ Fix failing tests immediately
3. ✅ Don't bypass test failures
4. ✅ Review test logs for warnings
5. ✅ Keep test documentation updated

### For Reviewers

1. ✅ Verify tests pass in PR
2. ✅ Check performance metrics
3. ✅ Review test artifacts if suspicious
4. ✅ Ensure tests cover new features
5. ✅ Validate test modifications

### For Operators

1. ✅ Monitor test success trends
2. ✅ Investigate flaky tests
3. ✅ Update thresholds as needed
4. ✅ Archive test results
5. ✅ Track performance baselines

## Getting Help

### Test System Issues

- **Documentation:** [test/README.md](../test/README.md)
- **Quick Start:** [test/QUICKSTART.md](../test/QUICKSTART.md)
- **Workflow Docs:** [workflows/README.md](workflows/README.md)

### Common Problems

- **Tests timeout:** Increase workflow timeout
- **Flaky tests:** Check resource constraints
- **Low performance:** Review runner specs
- **Service crashes:** Check logs and resources

## Summary

The HTTP Gateway test system provides:

- ✅ **Comprehensive Coverage:** Functional, performance, and integration tests
- ✅ **Automated Verification:** Clear pass/fail criteria
- ✅ **Detailed Reporting:** Metrics, logs, and artifacts
- ✅ **CI/CD Integration:** Branch protection and deployment gates
- ✅ **Easy Debugging:** Local reproduction and detailed logs

All tests must pass before code can be merged to main branches.
