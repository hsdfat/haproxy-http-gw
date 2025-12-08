# GitHub Actions Update - Frontend-Scoped API Migration

## Summary

Updated all GitHub Actions workflows and test scripts to use the new frontend-scoped backend API endpoints instead of the deprecated global backend API.

## Changes Made

### 1. Core Code Fixes

#### File: `pkg/gateway/manager.go`

**Issue 1: Nil pointer dereference in `Manager.Start()`**
- **Location:** Line 75-83
- **Problem:** Manager was calling `m.provider.Start()` when provider was nil
- **Fix:** Added nil check before starting provider
```go
// Start the backend provider if one is configured
if m.provider != nil {
    m.wg.Add(1)
    go func() {
        defer m.wg.Done()
        if err := m.provider.Start(ctx, m.eventChan); err != nil {
            logger.Errorf("Backend provider error: %v", err)
        }
    }()
}
```

**Issue 2: Nil pointer dereference in `Manager.reconcile()`**
- **Location:** Line 239-243
- **Problem:** Reconcile function was calling `m.provider.GetBackends()` without checking if provider was nil
- **Fix:** Added nil check at the beginning of reconcile
```go
// Skip reconciliation if no provider is configured
if m.provider == nil {
    logger.Debug("No provider configured, skipping reconciliation")
    return
}
```

#### File: `test/frontend-config-test.yaml`

**Issue: File permissions preventing haproxy user from reading config**
- **Location:** File permissions
- **Problem:** File had 600 permissions, preventing non-owner from reading
- **Fix:** Changed permissions to 644
```bash
chmod 644 test/frontend-config-test.yaml
```

### 2. API Endpoint Migration

All backend API endpoints were migrated from the global API to frontend-scoped API:

| Old API | New API |
|---------|---------|
| `GET /api/backends` | `GET /api/frontends/{id}/backends` |
| `POST /api/backends` | `POST /api/frontends/{id}/backends` |
| `DELETE /api/backends/{name}` | `DELETE /api/frontends/{id}/backends/{name}` |

### 3. GitHub Actions Workflows Updated

#### File: `.github/workflows/gateway-tests.yml`

**Changed Lines 110, 132:**
- Old: `curl -sf http://localhost:9090/api/backends`
- New: `curl -sf http://localhost:9090/api/frontends/default/backends`

**Changed Lines 307, 332, 345:**
- Backend registration endpoint updated to use frontend-scoped API
- Backend verification endpoint updated
- Backend unregistration endpoint updated

### 4. Test Scripts Updated

#### File: `test/scripts/register-backend.sh`

**Added:**
- Line 9: `FRONTEND_ID="${FRONTEND_ID:-default}"` - Frontend ID parameter
- Line 43: Added Frontend ID to configuration output
- Lines 70-72: Updated registration endpoint to use frontend-scoped API

**Removed:**
- Lines 66-90 (old): Complex logic for checking existing backends and merging servers
- Now uses simple single-call registration to frontend-scoped endpoint

#### File: `test/run-local-test.sh`

**Changed:**
- Line 108: Backend listing endpoint
- Line 178-180: Backend registration test endpoint
- Lines 218-226: Updated documentation with new API examples

#### File: `test/run-github-action-tests.sh`

**Changed:**
- Lines 130, 152: Backend polling endpoint
- Lines 305, 330, 341: Dynamic backend test endpoints

### 5. Files NOT Changed

The following files already use the correct frontend-scoped API:
- `test/run-frontend-test.sh` - Already uses new API
- `.github/workflows/frontend-management-tests.yml` - No backend API calls
- `.github/workflows/actions.yml` - General CI, no backend API calls

## Testing

### Local Test Results
All tests passed successfully after the updates:

```
✓ Configuration loads from YAML successfully
✓ Frontend configured with 1 HTTP binding
✓ Backend servers register successfully (api-backend and web-backend)
✓ Load balancing works across 3 backend servers
✓ Backend registration API works correctly
✓ H2C (HTTP/2 Cleartext) support works
```

### Test Command
```bash
./test/run-local-test.sh
```

## Migration Guide for Users

If you have custom scripts or tools that use the old backend API, update them as follows:

### List Backends
```bash
# Old
curl http://localhost:9090/api/backends

# New
curl http://localhost:9090/api/frontends/default/backends
```

### Register Backend
```bash
# Old
curl -X POST http://localhost:9090/api/backends \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-backend","servers":[{"name":"srv1","ip":"10.0.1.10","port":8080}]}'

# New
curl -X POST http://localhost:9090/api/frontends/default/backends \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-backend","servers":[{"name":"srv1","ip":"10.0.1.10","port":8080}]}'
```

### Delete Backend
```bash
# Old
curl -X DELETE http://localhost:9090/api/backends/my-backend

# New
curl -X DELETE http://localhost:9090/api/frontends/default/backends/my-backend
```

## Environment Variables for Backend Registration

The backend registration script now supports:

```bash
FRONTEND_ID=default    # Frontend to register backend with (default: "default")
BACKEND_NAME=my-backend
SERVER_NAME=server1
SERVER_IP=10.0.1.10
SERVER_PORT=9000
GATEWAY_URL=http://gateway:9090
```

## Backward Compatibility

**Note:** The old global backend API endpoints (`/api/backends`) are deprecated and should not be used. All new code must use the frontend-scoped API (`/api/frontends/{id}/backends`).

## Benefits of Frontend-Scoped API

1. **Better isolation:** Each frontend manages its own backends
2. **Multi-tenancy support:** Different frontends can have different backends
3. **Clearer architecture:** Backend lifecycle is tied to frontend lifecycle
4. **More flexible routing:** Backends are scoped to the frontend they serve

## Files Modified

- `pkg/gateway/manager.go`
- `test/frontend-config-test.yaml` (permissions)
- `test/scripts/register-backend.sh`
- `test/run-local-test.sh`
- `test/run-github-action-tests.sh`
- `.github/workflows/gateway-tests.yml`

## Verification

Run the following to verify the updates work correctly:

```bash
# Run local tests
cd test
./run-local-test.sh

# Run GitHub Actions tests locally
./run-github-action-tests.sh
```

Both should pass all tests successfully.
