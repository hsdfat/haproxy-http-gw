# h2load Performance Testing

## Overview

The test suite now includes comprehensive HTTP/2 performance testing using `h2load`, a benchmarking tool from the nghttp2 project. These tests measure throughput, latency, and resource consumption under various load conditions.

## Features

- **HTTP/2 Protocol Testing**: Native HTTP/2 benchmarking with h2load
- **Resource Monitoring**: Real-time CPU and RAM usage tracking during tests
- **Multiple Load Levels**: Low, medium, high, and stress test scenarios
- **Multi-Frontend Testing**: Tests all configured frontends simultaneously
- **Automated Installation**: Script auto-detects and installs h2load if missing

## Installation

### macOS (Homebrew)
```bash
brew install nghttp2
```

### Ubuntu/Debian
```bash
sudo apt-get install nghttp2-client
```

### Fedora/RHEL
```bash
sudo dnf install nghttp2
```

## Running Tests

### Integrated with Full Test Suite

The h2load tests are automatically included when running the complete test suite:

```bash
cd test
./run-github-action-tests.sh
```

The script will:
1. Check if h2load is installed
2. Attempt to install it if missing
3. Run all h2load benchmarks with resource monitoring
4. Include results in the final test report

### Standalone h2load Testing

For quick performance testing without the full suite:

```bash
cd test
./scripts/run-h2load-tests.sh
```

## Test Scenarios

### 1. Low Load - Baseline Performance
- **Target**: Default frontend (port 8080)
- **Configuration**: 1,000 requests, 10 concurrent clients
- **Purpose**: Establish baseline performance metrics
- **Resource Tracking**: Before/after snapshot

### 2. Medium Load - Moderate Traffic
- **Target**: API frontend (port 8081)
- **Configuration**: 10,000 requests, 50 concurrent clients
- **Purpose**: Simulate typical production load
- **Resource Tracking**: Before/after snapshot

### 3. High Load - Heavy Traffic
- **Target**: Web frontend (port 8082)
- **Configuration**: 50,000 requests, 100 concurrent clients
- **Purpose**: Test performance under heavy load
- **Resource Tracking**: Before/after snapshot

### 4. Stress Test - Maximum Capacity
- **Target**: All frontends simultaneously
- **Configuration**: 100,000 requests each, 200 concurrent clients
- **Purpose**: Determine maximum throughput and resource limits
- **Resource Tracking**: Continuous monitoring (1-second intervals)

## Resource Monitoring

### Metrics Collected

During stress tests, the following metrics are captured every second:

- **CPU Usage**: Percentage of CPU consumed by gateway container
- **Memory Usage**: Current memory consumption (used/available)
- **Network I/O**: Bytes sent/received
- **Block I/O**: Disk read/write activity
- **Timestamp**: Unix timestamp for each measurement

### Output Files

Resource monitoring data is saved to:
- `h2load-stress-monitor.csv` - Time-series resource usage data

Example format:
```csv
Timestamp,Name,CPU%,Memory,NetIO,BlockIO
1702345678,gateway,45.23%,256MiB / 4GiB,1.2GB / 890MB,0B / 12kB
```

## Test Results

### Output Files

Each test produces detailed output files:

| File | Description |
|------|-------------|
| `h2load-low-default.txt` | Low load test results (1K requests) |
| `h2load-medium-api.txt` | Medium load test results (10K requests) |
| `h2load-high-web.txt` | High load test results (50K requests) |
| `h2load-stress-default.txt` | Stress test - default frontend |
| `h2load-stress-api.txt` | Stress test - API frontend |
| `h2load-stress-web.txt` | Stress test - web frontend |
| `h2load-stress-monitor.csv` | Resource monitoring time-series data |

### Key Metrics

Each test result includes:

- **Total Requests**: Number of requests completed
- **Successful Requests**: Requests with 2xx/3xx status codes
- **Failed Requests**: Requests with errors or timeouts
- **Requests/sec**: Throughput (higher is better)
- **Time for Request**: Latency statistics (min/avg/max/stdev)
- **Request Distribution**: Histogram of response times

### Example Output

```
finished in 5.02s, 199.20 req/s, 45.32KB/s
requests: 1000 total, 1000 started, 1000 done, 1000 succeeded, 0 failed, 0 errored, 0 timeout
status codes: 1000 2xx, 0 3xx, 0 4xx, 0 5xx
traffic: 227.50KB (233000) total, 97.66KB (100000) headers (space savings 31.25%), 97.66KB (100000) data

                     min         max         mean         sd        +/- sd
time for request:     2.45ms     15.32ms      5.02ms      2.13ms    68.40%
time for connect:    12.45ms     45.67ms     28.91ms      8.23ms    72.50%
time to 1st byte:    45.23ms     89.12ms     62.45ms     12.34ms    65.30%
req/s           :      19.92       39.84       29.88        5.67    61.20%
```

## Performance Baselines

### Expected Performance (Local Testing)

Based on typical development environment (M1/M2 Mac or modern Linux):

| Test Level | Requests/sec | Avg Latency | Success Rate |
|-----------|--------------|-------------|--------------|
| Low Load | 200-400 | 20-50ms | 100% |
| Medium Load | 500-1500 | 30-80ms | >99% |
| High Load | 1000-3000 | 50-150ms | >98% |
| Stress Test | 2000-5000 | 100-300ms | >95% |

**Note**: Actual performance varies based on:
- Hardware specifications (CPU, RAM, disk)
- Container runtime (Podman vs Docker)
- System load and available resources
- Network configuration and latency

## Interpreting Results

### Success Criteria

Tests are considered passing when:

1. **Completion**: All requests finish without critical errors
2. **Status Codes**: Majority (>95%) return 2xx status codes
3. **No Timeouts**: Connection and request timeouts are minimal (<1%)
4. **Consistent Performance**: Latency standard deviation is reasonable

### Warning Signs

Watch for these indicators of issues:

- **High Error Rate**: >5% failed requests indicates backend or routing problems
- **Increasing Latency**: Mean latency increasing significantly under load
- **High CPU Usage**: Sustained >80% CPU may indicate bottleneck
- **Memory Leaks**: Memory usage continuously growing over time
- **Connection Failures**: High connection error rate indicates network issues

### Performance Optimization

If tests show poor performance:

1. **Check Resource Limits**: Increase container CPU/memory limits
2. **Review HAProxy Config**: Optimize maxconn, timeout values
3. **Backend Capacity**: Ensure backends can handle the load
4. **Network Tuning**: Adjust kernel TCP parameters
5. **Connection Pooling**: Enable HTTP keep-alive and connection reuse

## Troubleshooting

### h2load Not Found

If h2load is not installed:
```bash
# macOS
brew install nghttp2

# Ubuntu/Debian
sudo apt-get install nghttp2-client
```

### Gateway Not Running

Ensure services are started:
```bash
cd test
podman-compose up -d
```

Wait for health check:
```bash
curl http://localhost:9090/health
```

### Connection Refused

Check that frontends are listening:
```bash
curl http://localhost:8080/  # Default frontend
curl http://localhost:8081/  # API frontend
curl http://localhost:8082/  # Web frontend
```

### Resource Monitoring Not Working

Verify container runtime access:
```bash
# Podman
podman stats --no-stream

# Docker
docker stats --no-stream
```

## Integration with CI/CD

The h2load tests are designed to run in CI/CD pipelines:

```yaml
# Example GitHub Actions workflow
- name: Run Performance Tests
  run: |
    cd test
    ./run-github-action-tests.sh

- name: Upload Performance Results
  uses: actions/upload-artifact@v3
  with:
    name: h2load-results
    path: |
      test/h2load-*.txt
      test/h2load-stress-monitor.csv
```

## References

- [h2load Documentation](https://nghttp2.org/documentation/h2load.1.html)
- [nghttp2 Project](https://nghttp2.org/)
- [HTTP/2 Specification](https://httpwg.org/specs/rfc7540.html)
- [HAProxy Performance Tuning](https://www.haproxy.com/documentation/hapee/latest/performance/introduction/)
