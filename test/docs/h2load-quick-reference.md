# h2load Quick Reference Card

## Installation

```bash
# macOS
brew install nghttp2

# Ubuntu/Debian
sudo apt-get install nghttp2-client

# Fedora/RHEL
sudo dnf install nghttp2
```

## Running Tests

```bash
# Full test suite (includes h2load)
cd test
./run-github-action-tests.sh

# h2load tests only
cd test
./scripts/run-h2load-tests.sh
```

## Test Scenarios

| Test | Frontend | Requests | Clients | Purpose |
|------|----------|----------|---------|---------|
| Low Load | Default (8080) | 1,000 | 10 | Baseline |
| Medium Load | API (8081) | 10,000 | 50 | Moderate traffic |
| High Load | Web (8082) | 50,000 | 100 | Heavy traffic |
| Stress Test | All | 100,000 each | 200 | Max capacity |

## Output Files

| File | Test Type |
|------|-----------|
| `h2load-low-default.txt` | Low load baseline |
| `h2load-medium-api.txt` | Medium load |
| `h2load-high-web.txt` | High load |
| `h2load-stress-default.txt` | Stress - default |
| `h2load-stress-api.txt` | Stress - API |
| `h2load-stress-web.txt` | Stress - web |
| `h2load-stress-monitor.csv` | Resource monitoring |

## Key Metrics

```bash
# View summary
grep -E "requests:|finished in|req/s" h2load-*.txt

# Check errors
grep -E "failed|errored|timeout" h2load-*.txt

# Resource usage
tail -20 h2load-stress-monitor.csv
```

## Expected Performance (Local)

| Test | Req/sec | Latency | Success |
|------|---------|---------|---------|
| Low | 200-400 | 20-50ms | 100% |
| Medium | 500-1500 | 30-80ms | >99% |
| High | 1000-3000 | 50-150ms | >98% |
| Stress | 2000-5000 | 100-300ms | >95% |

## Troubleshooting

```bash
# Check if h2load is installed
which h2load

# Check if gateway is running
curl http://localhost:9090/health

# Check frontends
curl http://localhost:8080/  # Default
curl http://localhost:8081/  # API
curl http://localhost:8082/  # Web

# View container stats
podman stats --no-stream

# Check test logs
tail -f h2load-*.txt
```

## Manual h2load Commands

```bash
# Basic test
h2load -n 1000 -c 10 http://localhost:8080/

# With custom streams
h2load -n 10000 -c 50 -m 10 http://localhost:8081/api

# Stress test
h2load -n 100000 -c 200 -m 10 http://localhost:8082/
```

## Resource Monitoring

```bash
# Monitor in real-time
podman stats gateway

# Continuous logging
while true; do
  podman stats --no-stream gateway
  sleep 1
done > stats.log
```

## Common Issues

| Issue | Solution |
|-------|----------|
| h2load not found | Install nghttp2 package |
| Connection refused | Start services with `podman-compose up -d` |
| Gateway not healthy | Wait 30s, check logs: `podman-compose logs gateway` |
| High error rate | Check backend capacity, increase resources |
| Timeouts | Increase HAProxy timeouts in config |

## Quick Checks

```bash
# Is gateway running?
curl -f http://localhost:9090/health && echo "✓ Healthy" || echo "✗ Not running"

# Are all frontends responding?
for port in 8080 8081 8082; do
  curl -sf http://localhost:$port/ >/dev/null && echo "✓ Port $port" || echo "✗ Port $port"
done

# Check h2load version
h2load --version
```

## Full Documentation

For detailed information, see [h2load-testing.md](h2load-testing.md)
