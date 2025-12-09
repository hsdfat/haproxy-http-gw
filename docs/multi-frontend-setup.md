# Multi-Frontend Setup

This document describes the multi-frontend configuration for HAProxy HTTP Gateway testing.

## Overview

The test environment runs **3 independent frontends** on different ports, demonstrating the gateway's ability to manage multiple HTTP entry points with isolated backend pools.

## Frontend Configuration

| Frontend ID | Port | Default Backend | Backend Servers | Protocol |
|------------|------|-----------------|-----------------|----------|
| `default` | 8080 | api-backend | 3 servers (backend-server-1/2/3) | HTTP/1.1, H2C |
| `frontend-api` | 8081 | api-backend | 3 servers (backend-server-1/2/3) | HTTP/2 (H2C) |
| `frontend-web` | 8082 | web-backend | 2 servers (web-server-1/2) | HTTP/2 (H2C) |

**Note:** All frontends use `default_backend` for routing. Additional backend pools (`api-v2-backend`, `web-v2-backend`) register successfully but are not currently used for routing.

## Backend Pools

| Backend Pool | Servers | Registers To | Used By |
|-------------|---------|--------------|---------|
| api-backend | backend-server-1, 2, 3 | default | default, frontend-api |
| web-backend | web-server-1, 2 | default | frontend-web |
| api-v2-backend | api-v2-server-1, 2 | frontend-api | Not used |
| web-v2-backend | web-v2-server-1, 2 | frontend-web | Not used |

**Total:** 4 backend pools, 9 backend servers

## Key Features

- **Round-robin load balancing** across all backend servers
- **HTTP/2 (H2C)** support on all frontends
- **Dynamic backend registration** via API
- **Frontend isolation** - each frontend manages its own backend pools
- **Automatic HAProxy reload** when backends register/unregister (every 1 second)

## Testing

### Running Tests

```bash
cd test
./run-local-test.sh
```

### Test Coverage

1. **Health Checks** - Gateway API availability
2. **Backend Registration** - All 4 backend pools register successfully
3. **Functional Tests** - Connectivity on all 3 frontends (ports 8080, 8081, 8082)
4. **Load Balancing** - Round-robin distribution verification
5. **H2C Support** - HTTP/2 cleartext protocol validation
6. **Dynamic Updates** - Backend registration/unregistration API

### Expected Behavior

- Port 8080 (default) → Returns `backend-server-*` responses
- Port 8081 (frontend-api) → Returns `backend-server-*` responses (shares api-backend)
- Port 8082 (frontend-web) → Returns `web-server-*` responses

## Configuration Files

- **Frontend Config:** [test/frontend-config-test.yaml](../test/frontend-config-test.yaml)
- **Docker Compose:** [test/docker-compose.yml](../test/docker-compose.yml)
- **Test Script:** [test/run-local-test.sh](../test/run-local-test.sh)
- **GitHub Actions:** [.github/workflows/gateway-tests.yml](../.github/workflows/gateway-tests.yml)

## HAProxy Reload Mechanism

When backends register or unregister:

1. Backend registration triggers `instance.Reload()` flag
2. Main loop checks every 1 second if reload is needed
3. Graceful reload via SIGUSR2 signal to HAProxy
4. Flag reset after successful reload
5. Zero downtime during configuration updates

See [cmd/http-gateway/main.go:80-99](../cmd/http-gateway/main.go#L80-L99) for implementation details.

## Architecture

```
┌─────────────────────────────────────────┐
│     HAProxy HTTP Gateway                │
├─────────────────────────────────────────┤
│                                         │
│  Frontend: default (8080)               │
│    └─> api-backend (RR)                 │
│         ├─> backend-server-1            │
│         ├─> backend-server-2            │
│         └─> backend-server-3            │
│                                         │
│  Frontend: frontend-api (8081, H2C)     │
│    └─> api-backend (RR)                 │
│         ├─> backend-server-1            │
│         ├─> backend-server-2            │
│         └─> backend-server-3            │
│                                         │
│  Frontend: frontend-web (8082, H2C)     │
│    └─> web-backend (RR)                 │
│         ├─> web-server-1                │
│         └─> web-server-2                │
│                                         │
└─────────────────────────────────────────┘
```

## Implementation Notes

- Frontends use existing backends as `default_backend` to satisfy HAProxy's validation requirement
- HAProxy requires backends to exist before frontends can reference them
- Backend pools register dynamically after gateway startup
- Simplified approach using only `default_backend` (no ACL/routing rules)
- All backends successfully register and maintain healthy connections
