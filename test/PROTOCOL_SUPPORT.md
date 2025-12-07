# HTTP Protocol Support

## Overview

The HAProxy HTTP Gateway supports both **HTTP/1.1** and **HTTP/2** (H2C - HTTP/2 Cleartext) protocols.

## Configuration

### Frontend Binding

The frontend is configured to accept both protocols:

```haproxy
frontend http-gateway
    bind :8080
    bind [::]:8080 v4v6
    mode http
    option http-use-htx
    default_backend api-backend
```

### Key Points

1. **No `proto h2` on bind**: This allows both HTTP/1.1 and HTTP/2
2. **`option http-use-htx`**: Enables HAProxy's internal HTX (HTTP Transaction) representation
3. **Client-driven protocol**: The client chooses which protocol to use

## Protocol Selection

### HTTP/1.1 (Default)

Standard curl requests use HTTP/1.1 by default:

```bash
curl http://localhost:8080/
```

### HTTP/2 (H2C)

For HTTP/2 cleartext, use the `--http2-prior-knowledge` flag:

```bash
curl --http2-prior-knowledge http://localhost:8080/
```

Or with HTTP/2 upgrade:

```bash
curl --http2 http://localhost:8080/
```

## Why Both Protocols?

### HTTP/1.1 Support

**Required for**:
- Standard curl commands and testing
- Legacy clients
- Simple health checks
- Load balancer health probes
- Most existing tools and scripts

### HTTP/2 Support

**Benefits**:
- Multiplexing (multiple requests over single connection)
- Header compression
- Server push capability
- Better performance for modern applications

## Backend Configuration

Backends must support the protocol they receive. Our test backends support both:

```go
// Backend supports both HTTP/1.1 and HTTP/2
h2s := &http2.Server{}
server := &http.Server{
    Addr:    ":9000",
    Handler: h2c.NewHandler(mux, h2s), // H2C support
}
```

## Testing Both Protocols

### HTTP/1.1 Test

```bash
# Should work without special flags
curl -v http://localhost:8080/

# Response headers show HTTP/1.1
< HTTP/1.1 200 OK
```

### HTTP/2 Test

```bash
# Requires HTTP/2 support in curl
curl --http2-prior-knowledge -v http://localhost:8080/

# Response headers show HTTP/2
< HTTP/2 200
```

## Common Issues

### `<BADREQ>` Errors

**Problem**: HAProxy shows `<BADREQ>` in logs

**Cause**: Frontend was configured with `proto h2` which forces HTTP/2-only

**Fix**: Remove `proto h2` from bind lines

**Before** (HTTP/2 only):
```haproxy
bind :8080 proto h2
```

**After** (Both protocols):
```haproxy
bind :8080
option http-use-htx
```

### Protocol Mismatch

**Problem**: Client expects HTTP/2 but gets HTTP/1.1

**Solution**: Use `--http2-prior-knowledge` or `--http2` with curl

### Backend Connection Refused

**Problem**: HAProxy can't connect to backend

**Check**:
1. Backend is listening on correct port
2. Backend supports the protocol
3. Network connectivity between containers

## Performance Comparison

### HTTP/1.1

- One request per connection (or keep-alive)
- Separate connections for parallel requests
- Text-based headers

### HTTP/2

- Multiple requests per connection (multiplexing)
- Binary protocol
- Header compression (HPACK)
- Typically 20-30% faster for multiple requests

## Upgrade Path

### From HTTP/1.1 to HTTP/2

```bash
# Client makes HTTP/1.1 request with Upgrade header
curl --http2 http://localhost:8080/

# HAProxy responds with:
HTTP/1.1 101 Switching Protocols
Upgrade: h2c
Connection: Upgrade

# Then switches to HTTP/2
```

### Direct HTTP/2

```bash
# Client uses HTTP/2 from the start
curl --http2-prior-knowledge http://localhost:8080/
```

## Monitoring

### Check Protocol in Use

From HAProxy stats:

```bash
echo "show stat" | socat stdio /var/run/haproxy-runtime-api.sock
```

From logs:

```
# HTTP/1.1 request
192.168.1.10:54321 [07/Dec/2025:08:00:00.000] http-gateway api-backend/server1 0/0/1/2/3 200 1234 - - ---- 1/1/0/0/0 0/0 "GET / HTTP/1.1"

# HTTP/2 request
192.168.1.10:54322 [07/Dec/2025:08:00:01.000] http-gateway api-backend/server1 0/0/1/2/3 200 1234 - - ---- 1/1/0/0/0 0/0 "GET / HTTP/2.0"
```

## Best Practices

1. **Support both protocols**: Don't force HTTP/2-only unless required
2. **Let clients choose**: Allow protocol negotiation
3. **Test both**: Ensure your application works with both protocols
4. **Monitor usage**: Track which protocols clients use
5. **Upgrade gradually**: Don't break HTTP/1.1 clients

## HAProxy Configuration Options

### HTTP/1.1 Only

```haproxy
frontend http-gateway
    bind :8080
    mode http
    # No HTTP/2 support
```

### HTTP/1.1 and HTTP/2 (Current)

```haproxy
frontend http-gateway
    bind :8080
    mode http
    option http-use-htx
    # Supports both
```

### HTTP/2 Only

```haproxy
frontend http-gateway
    bind :8080 proto h2
    mode http
    # HTTP/2 only (will reject HTTP/1.1)
```

### HTTPS with ALPN

```haproxy
frontend https-gateway
    bind :8443 ssl crt /etc/haproxy/certs alpn h2,http/1.1
    mode http
    # ALPN negotiates protocol over TLS
```

## Related Documentation

- [HAProxy HTTP/2 Documentation](https://www.haproxy.com/documentation/haproxy-configuration-tutorials/http2/)
- [RFC 7540 - HTTP/2](https://tools.ietf.org/html/rfc7540)
- [RFC 7230 - HTTP/1.1](https://tools.ietf.org/html/rfc7230)
