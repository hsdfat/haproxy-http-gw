# HTTP/2 Test Client

A comprehensive HTTP/2 client for testing the HAProxy HTTP Gateway.

## Features

- **Protocol Support**: HTTP/1.1, HTTP/2 over TLS, and H2C (HTTP/2 Cleartext)
- **Concurrency Testing**: Configurable concurrent requests
- **Performance Metrics**: Response times, throughput, transfer rates
- **Verbose Output**: Detailed request/response logging
- **Protocol Detection**: Shows which protocol was actually used

## Installation

### Build from Source

```bash
cd test/client
go build -o http2-client ./cmd/http2-client
```

### Build with Docker

```bash
docker build -f test/Dockerfile.test-client -t http2-client .
```

## Usage

### Basic Examples

**HTTP/1.1 Request** (default for most tools):
```bash
./http2-client -url http://localhost:8080 -n 10
```

**HTTP/2 Cleartext (H2C)**:
```bash
./http2-client -url http://localhost:8080 -n 10 -http2 -h2c
```

**HTTP/2 over TLS**:
```bash
./http2-client -url https://localhost:8443 -n 10 -http2
```

**Disable HTTP/2 (force HTTP/1.1)**:
```bash
./http2-client -url http://localhost:8080 -n 10 -http2=false
```

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `http://localhost:8080` | Target URL |
| `-n` | `10` | Number of requests |
| `-c` | `1` | Concurrency level |
| `-http2` | `true` | Use HTTP/2 |
| `-h2c` | `true` | Use H2C (HTTP/2 Cleartext) |
| `-insecure` | `true` | Skip TLS verification |
| `-v` | `false` | Verbose output |
| `-timeout` | `30s` | Request timeout |

### Advanced Examples

**High Concurrency Test**:
```bash
./http2-client -url http://localhost:8080 -n 100 -c 10 -v
```

**Performance Benchmark**:
```bash
./http2-client -url http://localhost:8080 -n 1000 -c 50
```

**Protocol Comparison**:
```bash
# HTTP/1.1
./http2-client -url http://localhost:8080 -n 100 -c 10 -http2=false

# HTTP/2
./http2-client -url http://localhost:8080 -n 100 -c 10 -http2 -h2c
```

**HTTPS with Self-Signed Cert**:
```bash
./http2-client -url https://localhost:8443 -n 10 -insecure
```

## Output Format

### Summary Output

```
HTTP/2 Client Test
==================
URL: http://localhost:8080
Requests: 100
Concurrency: 10
HTTP/2: true (H2C: true)

Results
=======
Total requests:    100
Successful:        100 (100.00%)
Failed:            0 (0.00%)

Timing
------
Min response time: 2.123ms
Max response time: 45.678ms
Avg response time: 12.345ms
Total time:        1.234567s
Requests/sec:      81.01

Transfer
--------
Total bytes:       12345
Avg bytes/request: 123
Transfer rate:     9.76 KB/s

Protocols
---------
HTTP/2    : 100 (100.00%)
```

### Verbose Output

With `-v` flag, shows individual request details:

```
[0] HTTP/2 200 12.345ms http://localhost:8080 (body: 123 bytes)
[1] HTTP/2 200 8.765ms http://localhost:8080 (body: 123 bytes)
[2] HTTP/2 200 15.234ms http://localhost:8080 (body: 123 bytes)
...
```

## Testing Scenarios

### 1. Protocol Negotiation

Test that the gateway accepts both HTTP/1.1 and HTTP/2:

```bash
# Test HTTP/1.1
./http2-client -url http://localhost:8080 -n 10 -http2=false

# Test HTTP/2
./http2-client -url http://localhost:8080 -n 10 -http2 -h2c

# Check protocols in output
```

### 2. Load Testing

Simulate realistic load:

```bash
# Low load: 10 req/s for 10 seconds
./http2-client -url http://localhost:8080 -n 100 -c 1 -timeout 10s

# Medium load: 100 req/s
./http2-client -url http://localhost:8080 -n 1000 -c 10

# High load: 500 req/s
./http2-client -url http://localhost:8080 -n 5000 -c 50
```

### 3. Connection Multiplexing

Verify HTTP/2 multiplexing vs HTTP/1.1:

```bash
# HTTP/1.1: Each request uses separate connection
./http2-client -url http://localhost:8080 -n 50 -c 50 -http2=false -v

# HTTP/2: Multiple requests over single connection
./http2-client -url http://localhost:8080 -n 50 -c 50 -http2 -h2c -v
```

### 4. Backend Health Check

Test backend availability:

```bash
# Single request to check health
./http2-client -url http://localhost:8080 -n 1 -v

# Should return 200 if backend is healthy
# Should return 503 if no backends available
```

### 5. Stress Testing

Find breaking point:

```bash
# Gradually increase load
for c in 10 20 50 100 200; do
  echo "Testing with concurrency: $c"
  ./http2-client -url http://localhost:8080 -n 1000 -c $c
  sleep 5
done
```

## Integration with Test Suite

### Docker Compose

Add to `docker-compose.yml`:

```yaml
test-http2-client:
  build:
    context: ..
    dockerfile: test/Dockerfile.test-client
  container_name: test-http2-client
  command: /http2-client -url http://gateway:8080 -n 100 -c 10 -v
  networks:
    - gateway-net
  depends_on:
    - gateway
  profiles:
    - testing
```

Run with:
```bash
docker-compose --profile testing run test-http2-client
```

### Local Test Script

Add to `run-local-test.sh`:

```bash
# Build HTTP/2 client
print_info "Building HTTP/2 client..."
cd test/client
go build -o http2-client ./cmd/http2-client
cd ../..

# Test HTTP/1.1
print_info "Testing HTTP/1.1..."
./test/client/http2-client -url http://localhost:8080 -n 10 -http2=false

# Test HTTP/2
print_info "Testing HTTP/2 (H2C)..."
./test/client/http2-client -url http://localhost:8080 -n 10 -http2 -h2c
```

## Troubleshooting

### Connection Refused

**Problem**: `connection refused` errors

**Solution**:
```bash
# Check gateway is running
curl http://localhost:8080

# Check network connectivity
docker network inspect gateway-net
```

### H2C Not Working

**Problem**: Falls back to HTTP/1.1 even with `-h2c` flag

**Solution**:
```bash
# Verify gateway has option http-use-htx
docker exec http-gateway cat /tmp/haproxy-gateway/haproxy.cfg | grep http-use-htx

# Check HAProxy logs
docker logs http-gateway
```

### TLS Errors

**Problem**: `x509: certificate signed by unknown authority`

**Solution**:
```bash
# Use -insecure flag
./http2-client -url https://localhost:8443 -insecure

# Or provide CA cert
./http2-client -url https://localhost:8443 -cacert /path/to/ca.crt
```

### Timeout Errors

**Problem**: Requests timing out

**Solution**:
```bash
# Increase timeout
./http2-client -url http://localhost:8080 -timeout 60s

# Reduce concurrency
./http2-client -url http://localhost:8080 -n 100 -c 5
```

### Wrong Protocol Used

**Problem**: Expected HTTP/2 but got HTTP/1.1

**Check**:
```bash
# Verbose output shows actual protocol
./http2-client -url http://localhost:8080 -n 1 -v

# Look for "HTTP/2" or "HTTP/1.1" in response
```

**Solution**:
- For H2C, ensure `-h2c` flag is set
- Gateway must support HTTP/2 (check haproxy.cfg)
- URL must use http:// (not https://) for H2C

## Performance Tuning

### Client-Side

```bash
# Increase file descriptors
ulimit -n 10000

# Disable DNS lookups (use IP)
./http2-client -url http://127.0.0.1:8080 -n 10000 -c 100
```

### Server-Side

Check HAProxy configuration:

```haproxy
# Increase connection limits
global
    maxconn 10000

defaults
    timeout client 30s
    timeout server 30s
    timeout connect 5s

frontend http-gateway
    # Enable HTTP/2
    option http-use-htx
```

## Comparison with Other Tools

### vs curl

**curl**:
```bash
# HTTP/1.1
curl http://localhost:8080

# HTTP/2 (H2C)
curl --http2-prior-knowledge http://localhost:8080
```

**http2-client**:
- Provides performance metrics
- Supports concurrency testing
- Shows protocol distribution
- Better for load testing

### vs ab (Apache Bench)

**ab** (HTTP/1.1 only):
```bash
ab -n 100 -c 10 http://localhost:8080/
```

**http2-client**:
- Supports HTTP/2
- More detailed metrics
- Better protocol detection

### vs hey

**hey** (good HTTP/2 support):
```bash
hey -n 100 -c 10 http://localhost:8080/
```

**http2-client**:
- Built specifically for this project
- More control over H2C
- Customizable for specific tests

## Building for Different Platforms

### Linux
```bash
GOOS=linux GOARCH=amd64 go build -o http2-client-linux ./cmd/http2-client
```

### macOS
```bash
GOOS=darwin GOARCH=amd64 go build -o http2-client-macos ./cmd/http2-client
```

### Windows
```bash
GOOS=windows GOARCH=amd64 go build -o http2-client.exe ./cmd/http2-client
```

### Docker
```bash
docker build -f test/Dockerfile.test-client -t http2-client .
docker run --network gateway-net http2-client -url http://gateway:8080 -n 100
```

## Related Documentation

- [Protocol Support](../PROTOCOL_SUPPORT.md) - HTTP/1.1 and HTTP/2 configuration
- [Testing Guide](../TESTING.md) - Complete testing procedures
- [Backend Registration](../../BACKEND_REGISTRATION.md) - Architecture overview
