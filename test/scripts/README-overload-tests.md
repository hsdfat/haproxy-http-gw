# Per-path overload — integration test suite

End-to-end tests for the per-path overload protection feature (stick-table +
map-file based rate limiter controlled over the gateway's HTTP API).

## What it covers

`test-overload.sh` runs 12 sections, each with multiple assertions:

| Section | What it verifies |
| --- | --- |
| Bootstrap & preconditions | `/health`, overload list works only on enabled frontends, unknown frontend → 400, HAProxy stick-table backend created, map file on disk |
| CRUD + upsert | POST creates a rule, GET lists it, POST on same path updates in-place (no duplicates), DELETE returns 200, DELETE of missing path returns 404 |
| Validation | Empty path, negative limit, malformed JSON, DELETE without `?path=`, POST on a frontend where overload is disabled — all must 400 |
| Map file synchronization | Map file contains the configured entries, sorted longest-prefix-first; DELETE removes them live |
| Rate limiting — basic | With `limit=5` and a short period, first ~5 requests to a tracked path get 200, the rest get 429, and counters reset after the period expires |
| Path & frontend isolation | Rate-limiting one path does not affect siblings; the control frontend (overload disabled) is never throttled even while the enabled one is |
| Longest-prefix precedence | When both `/api` (broad, high limit) and `/api/hot` (narrow, tight limit) are registered, requests under `/api/hot` use the tight limit — HAProxy `map_beg` picks the most-specific prefix |
| Zero-limit | `limit=0` blocks (near-)every request to the tracked path |
| Stats endpoint | `GET /overload/stats` reports rate ≥ 5 for a path we just flooded |
| Concurrent upserts | 20 parallel POSTs → store ends with exactly 20 rules |
| Live limit change under traffic | Raising the limit after a flood lets traffic flow again once the period rolls; deleting the rule stops throttling immediately |
| Cleanup | Store empties cleanly |

Each assertion prints `✓` or `✗`; the script exits non-zero if any failed and
lists the failing case names at the end.

## How to run

From the repo root:

```bash
test/scripts/run-overload-tests.sh
```

The orchestrator will:

1. generate self-signed certs if missing,
2. `compose build` (both base + `docker-compose.overload.yml` override),
3. bring the stack up with the override (so the gateway uses
   `frontend-config-overload.yaml` — `frontend-api` at :8081 has
   `overload_enabled: true, overload_period: "5s"`, `default` at :8080 is the
   control with overload off),
4. wait for gateway `/health` and for `api-backend` / `api-v2-backend` to
   register,
5. smoke-test each frontend responds,
6. run `test-overload.sh`,
7. tear the stack down (`down -v`) on exit.

### Flags

| Flag | Effect |
| --- | --- |
| `--keep` | Leave the stack running after tests (for poking at the gateway / rerunning `test-overload.sh` directly) |
| `--no-build` | Skip the `compose build` step (use images already present) |
| `--no-up` | Assume the stack is already running — just run the tests |
| `--no-logs-on-fail` | Don't dump gateway logs when a test fails |

### Environment overrides

Forwarded to `test-overload.sh`:

| Var | Default | Meaning |
| --- | --- | --- |
| `API_URL` | `http://localhost:9090` | Gateway management API |
| `FE_ID` | `frontend-api` | Frontend under test (overload enabled) |
| `FE_URL` | `http://localhost:8081` | Data-plane URL for `FE_ID` |
| `FE_URL_DEFAULT` | `http://localhost:8080` | Control frontend (overload disabled) |
| `GATEWAY_CONTAINER` | `http-gateway` | Container name — used for `docker exec` inspection of the runtime socket and map file |
| `PERIOD_SECONDS` | `5` | Must match `overload_period` in `frontend-config-overload.yaml` — used for recovery/expiry waits |

And used only by the orchestrator:

| Var | Meaning |
| --- | --- |
| `COMPOSE_CMD` | Explicit compose binary (`docker-compose`, `podman-compose`, or `docker compose`). Auto-detected if unset. |

## Running `test-overload.sh` standalone

If you already have the stack up with the overload override (e.g., after
`run-overload-tests.sh --keep`):

```bash
test/scripts/test-overload.sh
```

This is the fast inner-loop: the suite runs in ~90s (most of it is the
`PERIOD_SECONDS` waits for the stick-table window to roll over).

## Files involved

- `test/frontend-config-overload.yaml` — frontend config with overload enabled on `frontend-api` only
- `test/docker-compose.overload.yml` — compose override that mounts the above onto the gateway (re-declares the volumes list because compose replaces rather than merges lists)
- `test/scripts/test-overload.sh` — the assertions themselves
- `test/scripts/run-overload-tests.sh` — the orchestrator (this is the entry point)
