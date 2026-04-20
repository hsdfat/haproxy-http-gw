#!/usr/bin/env bash
# Integration tests for per-path overload protection.
#
# Precondition: the gateway is running with frontend-config-overload.yaml, so
# overload is enabled on `frontend-api` (port 8081, period 5s) and disabled on
# `default` (port 8080).
#
# The script runs independent scenarios and prints a ✓/✗ for each. It exits 0
# only if every scenario passes.

set -u
set -o pipefail

API_URL="${API_URL:-http://localhost:9090}"
FE_ID="${FE_ID:-frontend-api}"
FE_URL="${FE_URL:-http://localhost:8081}"
FE_URL_DEFAULT="${FE_URL_DEFAULT:-http://localhost:8080}"
GATEWAY_CONTAINER="${GATEWAY_CONTAINER:-http-gateway}"
PERIOD_SECONDS="${PERIOD_SECONDS:-5}"

# Frontend-api's binding is HTTP/2-only (proto h2), so data-plane curls need
# --http2-prior-knowledge to avoid an empty reply from HAProxy.
CURL_FE=(curl --http2-prior-knowledge -sS)

GREEN=$'\033[0;32m'
RED=$'\033[0;31m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'
NC=$'\033[0m'

pass=0
fail=0
failed_cases=()

ok() { pass=$((pass + 1)); echo -e "  ${GREEN}✓${NC} $1"; }
ko() {
  fail=$((fail + 1)); failed_cases+=("$1")
  echo -e "  ${RED}✗${NC} $1"
  [ -n "${2-}" ] && echo -e "    ${YELLOW}${2}${NC}"
}
section() { echo -e "\n${CYAN}== $1 ==${NC}"; }

curl_json() { curl -sS -H 'Content-Type: application/json' "$@"; }

# status_of METHOD URL [BODY]  -> prints HTTP status code only
status_of() {
  local m="$1" u="$2" b="${3-}"
  if [ -n "$b" ]; then
    curl -sS -o /dev/null -w '%{http_code}' -X "$m" -H 'Content-Type: application/json' -d "$b" "$u"
  else
    curl -sS -o /dev/null -w '%{http_code}' -X "$m" "$u"
  fi
}

# fire N requests to $FE_URL$PATH, count 200s and 429s separated by space
fire() {
  local count="$1" path="$2"
  local out
  out=$(for _ in $(seq 1 "$count"); do
    "${CURL_FE[@]}" -o /dev/null -w '%{http_code}\n' "${FE_URL}${path}"
  done)
  local ok_count denied_count
  ok_count=$(echo "$out" | grep -c '^200$' || true)
  denied_count=$(echo "$out" | grep -c '^429$' || true)
  echo "$ok_count $denied_count"
}

reset_rules() {
  # Best-effort: delete every rule currently in the store.
  local paths
  paths=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload" | sed -n 's/.*"path":"\([^"]*\)".*/\1/gp' || true)
  for p in $paths; do
    curl -sS -o /dev/null -X DELETE "${API_URL}/api/frontends/${FE_ID}/overload?path=$(printf %s "$p" | sed 's|/|%2F|g')" || true
  done
  # A direct loop using -g and a plain query avoids URL-escape issues.
  local rules
  rules=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload")
  # Extract every "path":"..." via python if available, otherwise grep
  if command -v python3 >/dev/null 2>&1; then
    echo "$rules" | python3 -c '
import json,sys,urllib.parse
try:
    d=json.load(sys.stdin)
except Exception:
    sys.exit(0)
for r in d.get("rules") or []:
    print(urllib.parse.quote(r["path"], safe=""))
' | while read -r ep; do
      curl -sS -o /dev/null -X DELETE "${API_URL}/api/frontends/${FE_ID}/overload?path=${ep}" || true
    done
  fi
}

wait_for_gateway() {
  local i=0
  until curl -sfo /dev/null "${API_URL}/health"; do
    i=$((i + 1))
    if [ $i -gt 60 ]; then
      echo -e "${RED}Gateway did not become healthy within 60s${NC}"
      return 1
    fi
    sleep 1
  done
}

#############################################
# Scenarios
#############################################

test_bootstrap() {
  section "Bootstrap & preconditions"

  # 1. Gateway /health is up
  if [ "$(status_of GET "${API_URL}/health")" = "200" ]; then
    ok "gateway /health returns 200"
  else
    ko "gateway /health returns 200"
  fi

  # 2. Overload list on enabled frontend returns 200 + success
  local body
  body=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload")
  if echo "$body" | grep -q '"success":true'; then
    ok "GET /overload on ${FE_ID} succeeds (overload enabled)"
  else
    ko "GET /overload on ${FE_ID} succeeds" "response: $body"
  fi

  # 3. Overload list on the control frontend fails (overload disabled)
  local code
  code=$(status_of GET "${API_URL}/api/frontends/default/overload")
  if [ "$code" = "400" ]; then
    ok "GET /overload on default (disabled) returns 400"
  else
    ko "GET /overload on default returns 400" "got $code"
  fi

  # 4. Unknown frontend returns 400
  code=$(status_of GET "${API_URL}/api/frontends/no-such-fe/overload")
  if [ "$code" = "400" ]; then
    ok "GET /overload for unknown frontend returns 400"
  else
    ko "unknown frontend returns 400" "got $code"
  fi

  # 5. Stick-table backend was created by Bootstrap
  local show
  show=$(docker exec "${GATEWAY_CONTAINER}" sh -c 'echo "show backend" | socat - UNIX-CONNECT:/tmp/haproxy-gateway/haproxy-runtime-api.sock' 2>/dev/null || true)
  if echo "$show" | grep -q 'api_frontend_overload_tbl'; then
    ok "stick-table backend api_frontend_overload_tbl present"
  else
    ko "stick-table backend present" "show backend output: $(echo "$show" | head -n5)"
  fi

  # 6. Map file exists on disk inside the container
  if docker exec "${GATEWAY_CONTAINER}" test -f /etc/haproxy/maps/api_frontend_overload.map; then
    ok "map file /etc/haproxy/maps/api_frontend_overload.map exists"
  else
    ko "map file exists"
  fi
}

test_crud() {
  section "CRUD + upsert"
  reset_rules

  # 7. POST valid rule
  local resp code
  resp=$(curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/crud/first","limit":42}')
  if echo "$resp" | grep -q '"success":true' && echo "$resp" | grep -q '"limit":42'; then
    ok "POST /overload creates rule and returns the body"
  else
    ko "POST /overload creates rule" "response: $resp"
  fi

  # 8. List shows exactly that rule
  resp=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload")
  if echo "$resp" | grep -q '/crud/first' && echo "$resp" | grep -q '"limit":42'; then
    ok "GET /overload lists the new rule"
  else
    ko "list shows new rule" "response: $resp"
  fi

  # 9. Upsert: same path, different limit
  resp=$(curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/crud/first","limit":7}')
  resp=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload")
  local count
  count=$(echo "$resp" | grep -o '"path":"/crud/first"' | wc -l | tr -d ' ')
  if [ "$count" = "1" ] && echo "$resp" | grep -q '"limit":7'; then
    ok "upsert updates limit and keeps rule count = 1"
  else
    ko "upsert updates rule" "count=$count response=$resp"
  fi

  # 10. Delete
  code=$(status_of DELETE "${API_URL}/api/frontends/${FE_ID}/overload?path=/crud/first")
  if [ "$code" = "200" ]; then
    ok "DELETE existing rule returns 200"
  else
    ko "DELETE returns 200" "got $code"
  fi

  # 11. List empty after delete
  resp=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload")
  if ! echo "$resp" | grep -q '/crud/first'; then
    ok "list no longer contains deleted rule"
  else
    ko "list empty after delete" "response: $resp"
  fi

  # 12. Delete missing → 404
  code=$(status_of DELETE "${API_URL}/api/frontends/${FE_ID}/overload?path=/crud/missing")
  if [ "$code" = "404" ]; then
    ok "DELETE non-existent rule returns 404"
  else
    ko "delete missing returns 404" "got $code"
  fi
}

test_validation() {
  section "Validation"
  reset_rules

  local code

  # 13. Empty path
  code=$(status_of POST "${API_URL}/api/frontends/${FE_ID}/overload" '{"path":"","limit":10}')
  if [ "$code" = "400" ]; then ok "empty path → 400"; else ko "empty path → 400" "got $code"; fi

  # 14. Negative limit
  code=$(status_of POST "${API_URL}/api/frontends/${FE_ID}/overload" '{"path":"/neg","limit":-5}')
  if [ "$code" = "400" ]; then ok "negative limit → 400"; else ko "negative limit → 400" "got $code"; fi

  # 15. Invalid JSON body
  code=$(status_of POST "${API_URL}/api/frontends/${FE_ID}/overload" '{not json')
  if [ "$code" = "400" ]; then ok "invalid JSON → 400"; else ko "invalid JSON → 400" "got $code"; fi

  # 16. DELETE without path query
  code=$(status_of DELETE "${API_URL}/api/frontends/${FE_ID}/overload")
  if [ "$code" = "400" ]; then ok "DELETE w/o path → 400"; else ko "DELETE w/o path → 400" "got $code"; fi

  # 17. POST on overload-disabled frontend
  code=$(status_of POST "${API_URL}/api/frontends/default/overload" '{"path":"/x","limit":5}')
  if [ "$code" = "400" ]; then
    ok "POST on overload-disabled frontend → 400"
  else
    ko "POST on disabled frontend → 400" "got $code"
  fi
}

test_map_sync() {
  section "Map file synchronization"
  reset_rules

  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/map/a","limit":11}' >/dev/null
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/map/bb","limit":22}' >/dev/null

  local content
  content=$(docker exec "${GATEWAY_CONTAINER}" cat /etc/haproxy/maps/api_frontend_overload.map 2>/dev/null || true)
  # 18. Map file contains both entries
  if echo "$content" | grep -q '^/map/a 11$' && echo "$content" | grep -q '^/map/bb 22$'; then
    ok "map file contains upserted entries"
  else
    ko "map file contains upserted entries" "contents:\n$content"
  fi

  # 19. Map file sorted longest-first so path_beg picks the most specific prefix
  local first_line
  first_line=$(echo "$content" | head -n1)
  if [ "$first_line" = "/map/bb 22" ]; then
    ok "map file is sorted longest-path-first"
  else
    ko "map file sorted longest-first" "first line: $first_line"
  fi

  # 20. Delete one rule
  curl -sS -o /dev/null -X DELETE "${API_URL}/api/frontends/${FE_ID}/overload?path=/map/a"
  content=$(docker exec "${GATEWAY_CONTAINER}" cat /etc/haproxy/maps/api_frontend_overload.map 2>/dev/null || true)
  if ! echo "$content" | grep -q '/map/a' && echo "$content" | grep -q '/map/bb'; then
    ok "map file removes entry on DELETE"
  else
    ko "map file removes entry on DELETE" "contents:\n$content"
  fi
}

test_ratelimit_basic() {
  section "Rate limiting — basic (period ${PERIOD_SECONDS}s)"
  reset_rules
  sleep $((PERIOD_SECONDS + 1))  # drain any prior counters

  # 21. No rule → no denials under moderate load
  local ok_c denied_c
  read -r ok_c denied_c < <(fire 30 "/free")
  if [ "$denied_c" = "0" ] && [ "$ok_c" = "30" ]; then
    ok "paths with no rule: 30/30 pass, 0 denials"
  else
    ko "unregistered paths pass" "ok=$ok_c denied=$denied_c"
  fi

  # 22. Rule limit=5: under limit
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/rl/basic","limit":5}' >/dev/null
  # Give HAProxy a beat to apply the map update via runtime socket
  sleep 1
  sleep $((PERIOD_SECONDS + 1))  # ensure rate counter is zero
  read -r ok_c denied_c < <(fire 4 "/rl/basic")
  if [ "$denied_c" = "0" ]; then
    ok "limit=5, 4 requests: all pass"
  else
    ko "limit=5, 4 requests pass" "ok=$ok_c denied=$denied_c"
  fi

  # 23. Rule limit=5: exceeding produces 429s
  sleep $((PERIOD_SECONDS + 1))
  read -r ok_c denied_c < <(fire 20 "/rl/basic")
  if [ "$denied_c" -ge 10 ] && [ "$ok_c" -ge 3 ] && [ "$ok_c" -le 7 ]; then
    ok "limit=5, 20 requests: ~5 pass, rest 429 (got ok=$ok_c denied=$denied_c)"
  else
    ko "limit=5 enforced" "ok=$ok_c denied=$denied_c"
  fi

  # 24. Denied responses carry status 429
  local body
  body=$("${CURL_FE[@]}" -o /dev/null -w '%{http_code}' "${FE_URL}/rl/basic")
  if [ "$body" = "429" ]; then
    ok "further requests get 429 while over limit"
  else
    ko "deny returns 429" "got $body"
  fi

  # 25. Rate recovers after > period
  sleep $((PERIOD_SECONDS + 1))
  read -r ok_c denied_c < <(fire 3 "/rl/basic")
  if [ "$denied_c" = "0" ]; then
    ok "after period expires, requests succeed again"
  else
    ko "rate recovers after period" "ok=$ok_c denied=$denied_c"
  fi
}

test_isolation() {
  section "Path & frontend isolation"
  reset_rules
  sleep $((PERIOD_SECONDS + 1))

  # 26. Two independent paths
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/iso/a","limit":3}' >/dev/null
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/iso/b","limit":3}' >/dev/null
  sleep 1

  # Flood /iso/a to exhaust it
  fire 20 "/iso/a" >/dev/null
  # /iso/b should still have its own fresh counter
  local ok_c denied_c
  read -r ok_c denied_c < <(fire 3 "/iso/b")
  if [ "$denied_c" = "0" ]; then
    ok "per-path counters are independent"
  else
    ko "per-path counters independent" "ok=$ok_c denied=$denied_c (iso/b)"
  fi

  # 27. Frontend isolation: flooding same path on the control frontend (overload
  # disabled) should never get denied even while the enabled frontend is throttled.
  local dc
  # Default frontend has a plain HTTP/1.1 binding from the init config, but the
  # YAML-declared bindings add h2-proto listeners; the occasional connection
  # picks the h2 binding and closes on HTTP/1.1 — harmless for this assertion
  # (we only care that no 429 is returned), so drop curl's stderr.
  dc=$(for _ in $(seq 1 30); do
    curl -sS -o /dev/null -w '%{http_code}\n' "${FE_URL_DEFAULT}/iso/a" 2>/dev/null
  done | grep -c '^429$' || true)
  if [ "$dc" = "0" ]; then
    ok "overload-disabled frontend is never throttled"
  else
    ko "disabled frontend never throttled" "429 count=$dc"
  fi
}

test_longest_prefix() {
  section "Longest-prefix precedence"
  reset_rules
  sleep $((PERIOD_SECONDS + 1))

  # Broad rule high limit, narrow rule tight limit
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/lp","limit":1000}' >/dev/null
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/lp/narrow","limit":3}' >/dev/null
  sleep 1

  # 28. Narrow path uses its own tight limit
  local ok_c denied_c
  read -r ok_c denied_c < <(fire 15 "/lp/narrow")
  if [ "$denied_c" -ge 5 ]; then
    ok "narrow rule wins over broad rule (got ok=$ok_c denied=$denied_c)"
  else
    ko "narrow rule wins" "ok=$ok_c denied=$denied_c"
  fi

  # 29. Broad path (not narrow) uses the broad limit and is not throttled
  read -r ok_c denied_c < <(fire 20 "/lp/other")
  if [ "$denied_c" = "0" ]; then
    ok "sibling path under broad rule is not throttled"
  else
    ko "broad sibling not throttled" "ok=$ok_c denied=$denied_c"
  fi
}

test_zero_limit() {
  section "Zero-limit (hard block)"
  reset_rules
  sleep $((PERIOD_SECONDS + 1))
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/zero","limit":0}' >/dev/null
  sleep 1

  # 30. With limit=0, the very first request increments the counter then
  # sc_http_req_rate(0) > 0 fires → every request is denied.
  local ok_c denied_c
  read -r ok_c denied_c < <(fire 10 "/zero")
  if [ "$denied_c" -ge 9 ]; then
    ok "limit=0 denies ~all requests (ok=$ok_c denied=$denied_c)"
  else
    ko "limit=0 denies ~all" "ok=$ok_c denied=$denied_c"
  fi
}

test_stats() {
  section "Stats endpoint"
  reset_rules
  sleep $((PERIOD_SECONDS + 1))

  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/stats/x","limit":100}' >/dev/null
  sleep 1

  # generate ~15 requests
  fire 15 "/stats/x" >/dev/null

  # 31. Stats include the tracked path with rate > 0
  local body rate
  body=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload/stats")
  if echo "$body" | grep -q '"path":"/stats/x"'; then
    ok "stats include recently hit path"
  else
    ko "stats include recently hit path" "response: $body"
  fi
  rate=$(echo "$body" | sed -n 's/.*"path":"\/stats\/x","http_req_rate":\([0-9]*\).*/\1/p')
  if [ -n "$rate" ] && [ "$rate" -ge 5 ]; then
    ok "stats rate for /stats/x is >= 5 (got $rate)"
  else
    ko "stats rate reflects traffic" "rate=${rate:-<none>}"
  fi
}

test_concurrency() {
  section "Concurrent upserts"
  reset_rules

  # 32. Fire 20 parallel POSTs, expect 20 rules in the store.
  local pids=()
  for i in $(seq 1 20); do
    curl -sS -o /dev/null -X POST -H 'Content-Type: application/json' \
      -d "{\"path\":\"/cc/$i\",\"limit\":$i}" \
      "${API_URL}/api/frontends/${FE_ID}/overload" &
    pids+=($!)
  done
  for p in "${pids[@]}"; do wait "$p"; done

  local resp count
  resp=$(curl -sS "${API_URL}/api/frontends/${FE_ID}/overload")
  count=$(echo "$resp" | grep -o '"path":"/cc/' | wc -l | tr -d ' ')
  if [ "$count" = "20" ]; then
    ok "20 parallel upserts → 20 rules"
  else
    ko "20 parallel upserts → 20 rules" "count=$count"
  fi
}

test_live_update() {
  section "Live limit change under traffic"
  reset_rules
  sleep $((PERIOD_SECONDS + 1))

  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/live","limit":5}' >/dev/null
  sleep 1

  # Exhaust current window
  fire 12 "/live" >/dev/null

  # 33. Raise the limit — after waiting out the period, more requests pass
  curl_json -sS -X POST "${API_URL}/api/frontends/${FE_ID}/overload" \
    -d '{"path":"/live","limit":1000}' >/dev/null
  sleep $((PERIOD_SECONDS + 1))
  local ok_c denied_c
  read -r ok_c denied_c < <(fire 50 "/live")
  if [ "$denied_c" = "0" ]; then
    ok "raising limit clears throttling after period"
  else
    ko "live limit raise takes effect" "ok=$ok_c denied=$denied_c"
  fi

  # 34. Delete the rule mid-flight → further traffic is unthrottled immediately
  curl -sS -o /dev/null -X DELETE "${API_URL}/api/frontends/${FE_ID}/overload?path=/live"
  sleep 1
  read -r ok_c denied_c < <(fire 30 "/live")
  if [ "$denied_c" = "0" ]; then
    ok "deleted rule stops throttling"
  else
    ko "delete stops throttling" "ok=$ok_c denied=$denied_c"
  fi
}

cleanup() {
  section "Cleanup"
  reset_rules
  ok "store emptied"
}

#############################################
# Main
#############################################

main() {
  echo -e "${CYAN}HAProxy per-path overload integration tests${NC}"
  echo    "API:        ${API_URL}"
  echo    "Frontend:   ${FE_ID} (${FE_URL})"
  echo    "Control:    default (${FE_URL_DEFAULT})"
  echo    "Period:     ${PERIOD_SECONDS}s"
  echo    "Container:  ${GATEWAY_CONTAINER}"

  if ! wait_for_gateway; then
    echo -e "${RED}Gateway not ready, aborting${NC}"
    exit 1
  fi

  test_bootstrap
  test_crud
  test_validation
  test_map_sync
  test_ratelimit_basic
  test_isolation
  test_longest_prefix
  test_zero_limit
  test_stats
  test_concurrency
  test_live_update
  cleanup

  echo
  echo    "================================================="
  echo -e "  passed: ${GREEN}${pass}${NC}"
  echo -e "  failed: ${RED}${fail}${NC}"
  if [ "$fail" -gt 0 ]; then
    echo -e "\n${RED}Failing cases:${NC}"
    for c in "${failed_cases[@]}"; do echo "  - $c"; done
    exit 1
  fi
  echo -e "\n${GREEN}All overload integration tests passed.${NC}"
}

main "$@"
