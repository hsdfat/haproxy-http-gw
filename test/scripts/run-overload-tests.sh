#!/usr/bin/env bash
# Orchestrator for the per-path overload integration suite.
#
# Brings up the stack with the overload compose override (which swaps
# frontend-config-overload.yaml onto the gateway), waits for the gateway and
# backends to be ready, then runs test-overload.sh.
#
# Flags:
#   --keep        leave the stack running after tests (default: tear down)
#   --no-build    skip `compose build` (use images already present)
#   --no-up       assume the stack is already running; just run the tests
#   --logs-on-fail   dump gateway logs if any test fails (default on)
#   --no-logs-on-fail
#
# Env overrides:
#   COMPOSE_CMD   explicit compose binary (docker-compose / podman-compose)
#   API_URL, FE_URL, FE_URL_DEFAULT, PERIOD_SECONDS — forwarded to test-overload.sh

set -u
set -o pipefail

# Ensure podman-compose is in PATH (for macOS)
export PATH="/Library/Frameworks/Python.framework/Versions/3.14/bin:$PATH"

GREEN=$'\033[0;32m'
RED=$'\033[0;31m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'
NC=$'\033[0m'

info()    { echo -e "${YELLOW}→${NC} $*"; }
success() { echo -e "${GREEN}✓${NC} $*"; }
fail()    { echo -e "${RED}✗${NC} $*"; }
section() { echo; echo -e "${CYAN}=== $* ===${NC}"; }

KEEP=0
DO_BUILD=1
DO_UP=1
LOGS_ON_FAIL=1

while [ $# -gt 0 ]; do
  case "$1" in
    --keep)              KEEP=1 ;;
    --no-build)          DO_BUILD=0 ;;
    --no-up)             DO_UP=0 ;;
    --logs-on-fail)      LOGS_ON_FAIL=1 ;;
    --no-logs-on-fail)   LOGS_ON_FAIL=0 ;;
    -h|--help)
      sed -n '2,20p' "$0"; exit 0 ;;
    *)
      fail "unknown flag: $1"; exit 2 ;;
  esac
  shift
done

# Resolve paths. This script lives in test/scripts/; compose files are in test/.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$TEST_DIR"

# Pick a compose command.
if [ -z "${COMPOSE_CMD:-}" ]; then
  if command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD="docker-compose"
  elif command -v podman-compose >/dev/null 2>&1; then
    COMPOSE_CMD="podman-compose"
  elif docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
  else
    fail "need docker-compose, podman-compose, or 'docker compose' on PATH"
    exit 1
  fi
fi
success "compose: $COMPOSE_CMD"

COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.overload.yml)
compose() { $COMPOSE_CMD "${COMPOSE_FILES[@]}" "$@"; }

teardown() {
  if [ $KEEP -eq 1 ]; then
    info "leaving stack running (--keep). Tear down with:"
    echo "    cd $TEST_DIR && $COMPOSE_CMD ${COMPOSE_FILES[*]} down -v"
    return
  fi
  section "Tearing stack down"
  compose down -v >/dev/null 2>&1 || true
}

dump_logs() {
  [ $LOGS_ON_FAIL -eq 1 ] || return 0
  section "gateway logs (last 100 lines)"
  compose logs --tail=100 gateway 2>&1 || true
}

# --- Preflight ------------------------------------------------------------

section "Preflight"

if [ ! -f "certs/server.pem" ]; then
  info "generating SSL certificates"
  chmod +x scripts/generate-certs.sh
  ./scripts/generate-certs.sh
  success "certs generated"
else
  success "certs already present"
fi

# --- Build ----------------------------------------------------------------

if [ $DO_UP -eq 1 ] && [ $DO_BUILD -eq 1 ]; then
  section "Building images"
  if ! compose build; then
    fail "build failed"
    exit 1
  fi
  success "images built"
fi

# --- Up -------------------------------------------------------------------

if [ $DO_UP -eq 1 ]; then
  section "Starting stack (overload override)"
  # Ensure no stale containers from a previous plain run leak in.
  $COMPOSE_CMD -f docker-compose.yml down >/dev/null 2>&1 || true
  compose up -d
  success "services started"
  trap teardown EXIT
fi

# --- Wait for gateway health ---------------------------------------------

section "Waiting for gateway /health"
API_URL="${API_URL:-http://localhost:9090}"
max=30
i=0
while [ $i -lt $max ]; do
  if curl -sf "$API_URL/health" >/dev/null 2>&1; then
    success "gateway healthy (api $API_URL)"
    break
  fi
  i=$((i+1))
  [ $i -eq $max ] && { fail "gateway not healthy after $max attempts"; dump_logs; exit 1; }
  echo "  attempt $i/$max..."
  sleep 2
done

# --- Wait for backend registration ---------------------------------------
# test-overload.sh fires real HTTP traffic at ports 8080/8081, so the backends
# must have registered and passed health-checks first.

section "Waiting for backend registration"
need_backend_in() {
  local fe="$1" name="$2"
  curl -sf "$API_URL/api/frontends/$fe/backends" 2>/dev/null | grep -q "$name"
}

wait_for_backend() {
  local fe="$1" name="$2" tries=30
  for _ in $(seq 1 $tries); do
    if need_backend_in "$fe" "$name"; then
      success "$name registered on $fe"
      return 0
    fi
    sleep 2
  done
  fail "$name did not register on $fe within $((tries*2))s"
  return 1
}

fe_ok=1
wait_for_backend "default"      "api-backend"    || fe_ok=0
wait_for_backend "frontend-api" "api-v2-backend" || fe_ok=0
if [ $fe_ok -ne 1 ]; then
  dump_logs
  exit 1
fi

# Sanity: hit each frontend once so we know HAProxy + backend path work even
# before overload rules are introduced. The overload test config declares both
# frontends with http2:true, but the default frontend also inherits a plain
# HTTP/1.1 bind from haproxy-init.cfg. Some connections may land on either
# binding depending on TCP-level scheduling, so try both protocols.
section "Sanity: each frontend responds"
ok_on() {
  local url="$1"
  curl -fsS --max-time 5 --http2-prior-knowledge "$url" >/dev/null 2>&1 && return 0
  curl -fsS --max-time 5 "$url" >/dev/null 2>&1
}
# Retry briefly to absorb transient bring-up conditions (backend sync races).
retry_ok_on() {
  local url="$1" tries=10
  for _ in $(seq 1 $tries); do
    ok_on "$url" && return 0
    sleep 1
  done
  return 1
}
if ! retry_ok_on "http://localhost:8080/"; then
  fail "default frontend (8080) not serving"; dump_logs; exit 1
fi
success "default frontend 8080 OK"
if ! retry_ok_on "http://localhost:8081/"; then
  fail "frontend-api (8081) not serving"; dump_logs; exit 1
fi
success "frontend-api 8081 OK"

# --- Run the test suite --------------------------------------------------

section "Running test-overload.sh"
chmod +x "$SCRIPT_DIR/test-overload.sh"

set +e
"$SCRIPT_DIR/test-overload.sh"
rc=$?
set -e

if [ $rc -ne 0 ]; then
  fail "test-overload.sh exited with status $rc"
  dump_logs
  exit $rc
fi

success "all overload tests passed"
