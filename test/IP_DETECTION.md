# IP Address Auto-Detection

## Overview

Backend servers automatically detect their IP address from the container's network interface and register with the gateway using the actual IP address instead of hostname.

## Why IP-Based Registration?

### Benefits

1. **HAProxy Compatibility**: HAProxy backends work better with IP addresses for health checks and connections
2. **DNS Independence**: No reliance on container DNS resolution
3. **Network Transparency**: Uses actual container IP from network interface
4. **Flexibility**: Works across different container orchestrators (Docker, Podman, Kubernetes)

### Comparison

**Before (Hostname-based)**:
```yaml
environment:
  - SERVER_IP=backend-server-1  # Uses hostname
```

HAProxy config:
```
server backend-server-1 backend-server-1:9000
```

**After (IP-based)**:
```yaml
environment:
  # SERVER_IP is auto-detected from eth0
```

HAProxy config:
```
server backend-server-1 10.88.0.5:9000  # Actual IP
```

## How It Works

### Detection Logic

The backend entrypoint script detects IP in the following order:

1. **Check environment variable** (if explicitly set)
   ```bash
   if [ -n "$SERVER_IP" ]; then
       # Use provided IP
   fi
   ```

2. **Try eth0 interface** (most common in containers)
   ```bash
   SERVER_IP=$(ip addr show eth0 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
   ```

3. **Fallback to hostname -i**
   ```bash
   SERVER_IP=$(hostname -i | awk '{print $1}')
   ```

4. **Last resort: use hostname**
   ```bash
   SERVER_IP=$(hostname)
   ```

### Code Implementation

**File**: `test/scripts/backend-entrypoint.sh`

```bash
#!/bin/bash
set -e

# Auto-detect IP address if not provided
if [ -z "$SERVER_IP" ]; then
    # Try to get IP from eth0 interface (common in Docker/Podman)
    SERVER_IP=$(ip addr show eth0 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 || true)

    # Fallback to hostname -i if eth0 not found
    if [ -z "$SERVER_IP" ]; then
        SERVER_IP=$(hostname -i 2>/dev/null | awk '{print $1}' || true)
    fi

    # Last resort: use hostname
    if [ -z "$SERVER_IP" ]; then
        SERVER_IP=$(hostname)
    fi
fi

echo "Detected IP: $SERVER_IP"
```

## Requirements

### Package Dependencies

The backend Dockerfile includes `iproute2` for the `ip` command:

```dockerfile
FROM golang:1.24-alpine

# Install tools for backend registration and IP detection
RUN apk add --no-cache curl jq bash iproute2
```

### Network Configuration

- Container must have network interface (typically `eth0`)
- Network driver must assign IP addresses (default behavior)
- Works with Docker bridge, Podman bridge, and Kubernetes pod networks

## Usage Examples

### Docker Compose

**Automatic detection (recommended)**:
```yaml
backend-server-1:
  environment:
    - SERVER_NAME=backend-server-1
    # SERVER_IP auto-detected from eth0
    - SERVER_PORT=9000
    - BACKEND_NAME=api-backend
    - GATEWAY_URL=http://gateway:9090
```

**Manual override**:
```yaml
backend-server-1:
  environment:
    - SERVER_NAME=backend-server-1
    - SERVER_IP=10.0.1.10  # Explicitly set
    - SERVER_PORT=9000
    - BACKEND_NAME=api-backend
```

### Kubernetes

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: backend
    env:
    - name: SERVER_NAME
      valueFrom:
        fieldRef:
          fieldPath: metadata.name
    # SERVER_IP auto-detected from pod network
    - name: SERVER_PORT
      value: "9000"
    - name: BACKEND_NAME
      value: "api-backend"
    - name: GATEWAY_URL
      value: "http://gateway:9090"
```

### Standalone Container

```bash
docker run -d \
  --name backend-server-1 \
  -e SERVER_NAME=backend-server-1 \
  -e SERVER_PORT=9000 \
  -e BACKEND_NAME=api-backend \
  -e GATEWAY_URL=http://gateway:9090 \
  backend-image
```

## Verification

### Check Detected IP

View container logs to see detected IP:

```bash
docker logs backend-server-1 | grep "Detected IP"
```

Output:
```
=========================================
Backend Server Starting
=========================================
Server Name: backend-server-1
Detected IP: 10.88.0.5
Server Port: 9000
Backend Name: api-backend
Gateway URL: http://gateway:9090
=========================================
```

### Query Gateway

Check registered IP in HAProxy:

```bash
curl http://localhost:9090/api/backends | jq
```

Output:
```json
{
  "success": true,
  "backends": {
    "api-backend": {
      "Name": "api-backend",
      "Servers": [
        {
          "Name": "backend-server-1",
          "IP": "10.88.0.5",
          "Port": 9000
        }
      ]
    }
  }
}
```

### Verify HAProxy Config

Check HAProxy runtime configuration:

```bash
# Inside gateway container
echo "show servers state" | socat stdio /var/run/haproxy-runtime-api.sock
```

## Troubleshooting

### IP Not Detected

**Problem**: IP shows as hostname

**Check**:
```bash
docker exec backend-server-1 ip addr show eth0
```

**Solution**:
- Ensure container has network interface
- Verify `iproute2` is installed
- Check entrypoint script permissions

### Wrong IP Detected

**Problem**: Detected IP is not routable

**Check**:
```bash
# View all network interfaces
docker exec backend-server-1 ip addr show

# Check which interface is used
docker exec backend-server-1 hostname -i
```

**Solution**:
- Explicitly set `SERVER_IP` environment variable
- Use correct network interface (not eth0)
- Check Docker/Podman network configuration

### Multiple IPs

**Problem**: Container has multiple network interfaces

**Check**:
```bash
docker exec backend-server-1 hostname -i
```

Output might show: `10.88.0.5 172.17.0.3`

**Solution**:
```bash
# Explicitly set the IP to use
docker run -e SERVER_IP=10.88.0.5 ...
```

### Cannot Reach Gateway

**Problem**: Backend registers but HAProxy can't connect

**Check**:
```bash
# Test connectivity from gateway to backend IP
docker exec gateway curl http://10.88.0.5:9000/health
```

**Solution**:
- Ensure containers are on same network
- Check firewall rules
- Verify port is exposed

## Network Topologies

### Docker Bridge Network

```
┌─────────────────────────────┐
│   Docker Bridge Network     │
│   (10.88.0.0/16)           │
│                             │
│  ┌──────────────────┐      │
│  │  Gateway         │      │
│  │  10.88.0.2:9090  │      │
│  └──────────────────┘      │
│           ↑                 │
│           │ Registration    │
│           │                 │
│  ┌──────────────────┐      │
│  │  Backend 1       │      │
│  │  10.88.0.5:9000  │──────┼─→ Auto-detected
│  └──────────────────┘      │
│                             │
│  ┌──────────────────┐      │
│  │  Backend 2       │      │
│  │  10.88.0.6:9000  │──────┼─→ Auto-detected
│  └──────────────────┘      │
└─────────────────────────────┘
```

### Kubernetes Pod Network

```
┌─────────────────────────────┐
│   Kubernetes Cluster        │
│                             │
│  ┌──────────────────┐      │
│  │  Gateway Pod     │      │
│  │  10.244.0.5:9090 │      │
│  └──────────────────┘      │
│           ↑                 │
│           │                 │
│  ┌──────────────────┐      │
│  │  Backend Pod 1   │      │
│  │  10.244.1.8:9000 │──────┼─→ Auto-detected
│  └──────────────────┘      │
│                             │
│  ┌──────────────────┐      │
│  │  Backend Pod 2   │      │
│  │  10.244.2.3:9000 │──────┼─→ Auto-detected
│  └──────────────────┘      │
└─────────────────────────────┘
```

## Best Practices

1. **Let it auto-detect**: Don't set `SERVER_IP` unless necessary
2. **Use consistent naming**: Set `SERVER_NAME` to match container/pod name
3. **Verify in logs**: Always check detected IP in container logs
4. **Test connectivity**: Ensure gateway can reach the detected IP
5. **Monitor registration**: Check gateway API for registered backends

## Advanced Configuration

### Custom Network Interface

If your containers use a different interface (e.g., `eth1`):

```bash
# Modify entrypoint or set environment
SERVER_IP=$(ip addr show eth1 | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
```

### IPv6 Support

For IPv6 networks:

```bash
# Detect IPv6 address
SERVER_IP=$(ip addr show eth0 | grep 'inet6' | grep -v 'fe80' | awk '{print $2}' | cut -d/ -f1)
```

### Multiple Networks

For multi-network scenarios:

```bash
# Prefer specific network range
SERVER_IP=$(ip addr show | grep 'inet ' | grep '10.88.' | awk '{print $2}' | cut -d/ -f1 | head -1)
```

## Related Documentation

- [Backend Registration Architecture](../BACKEND_REGISTRATION.md)
- [Testing Guide](TESTING.md)
- [Docker Networking](https://docs.docker.com/network/)
- [Podman Networking](https://docs.podman.io/en/latest/markdown/podman-network.1.html)
