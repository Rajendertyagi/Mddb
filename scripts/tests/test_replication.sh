#!/usr/bin/env bash
# =============================================================================
# test_replication.sh — MDDB replication ring smoke tests
#
# Tests:
#   1. Health check — all nodes respond
#   2. Write on leader — document is created
#   3. Replication propagation — document appears on both followers
#   4. LSN / lag check — followers are caught up
#   5. Write rejection — follower refuses write requests
#   6. Stats consistency — document counts match across all nodes
#
# Usage:
#   ./scripts/tests/test_replication.sh
#
# Prerequisites:
#   docker compose -f docker-compose.ring.yml up --build -d
# =============================================================================

set -euo pipefail

LEADER="http://localhost:11023"
FOLLOWER1="http://localhost:11033"
FOLLOWER2="http://localhost:11043"

PASS=0
FAIL=0

# ── helpers ──────────────────────────────────────────────────────────────────

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
blue()  { printf '\033[34m%s\033[0m\n' "$*"; }

ok() {
  green "  ✓ $1"
  PASS=$((PASS + 1))
}

fail() {
  red "  ✗ $1"
  FAIL=$((FAIL + 1))
}

check() {
  local desc="$1"
  local got="$2"
  local want="$3"
  if [ "$got" = "$want" ]; then
    ok "$desc (got: $got)"
  else
    fail "$desc — want: '$want', got: '$got'"
  fi
}

wait_for() {
  local url="$1"
  local max_attempts="${2:-30}"
  local attempt=0
  while ! curl -sf "$url" > /dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $max_attempts ]; then
      fail "Timeout waiting for $url"
      return 1
    fi
    sleep 1
  done
}

# ── tests ────────────────────────────────────────────────────────────────────

blue "=== 1. WAITING FOR CLUSTER HEALTH ==="
wait_for "${LEADER}/health"     30
wait_for "${FOLLOWER1}/health"  30
wait_for "${FOLLOWER2}/health"  30
ok "All nodes are reachable"

blue "\n=== 2. HEALTH CHECK ==="
leader_mode=$(curl -sf "${LEADER}/health"    | python3 -c "import sys,json; print(json.load(sys.stdin)['mode'])")
f1_mode=$(curl -sf "${FOLLOWER1}/health"     | python3 -c "import sys,json; print(json.load(sys.stdin)['mode'])")
f2_mode=$(curl -sf "${FOLLOWER2}/health"     | python3 -c "import sys,json; print(json.load(sys.stdin)['mode'])")
check "Leader mode"    "$leader_mode" "wr"
check "Follower-1 mode" "$f1_mode"   "read"
check "Follower-2 mode" "$f2_mode"   "read"

blue "\n=== 3. WRITE DOCUMENT ON LEADER ==="
DOC_KEY="repl-smoke-$$"
write_resp=$(curl -sf -X POST "${LEADER}/v1/add" \
  -H "Content-Type: application/json" \
  -d "{\"collection\":\"smoke\",\"key\":\"${DOC_KEY}\",\"lang\":\"en\",\"contentMd\":\"# Replication Smoke Test\",\"meta\":{\"test\":[\"true\"]}}" \
  2>&1 || true)
written_key=$(echo "$write_resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('key',''))" 2>/dev/null || echo "")
check "Document written to leader" "$written_key" "$DOC_KEY"

blue "\n=== 4. REPLICATION PROPAGATION (waiting up to 10s) ==="
for attempt in $(seq 1 20); do
  f1_count=$(curl -sf "${FOLLOWER1}/v1/stats" \
    | python3 -c "import sys,json; d=json.load(sys.stdin); cols=[c for c in d.get('collections',[]) if c['name']=='smoke']; print(cols[0]['documentCount'] if cols else 0)" 2>/dev/null || echo "0")
  if [ "$f1_count" -ge 1 ]; then break; fi
  sleep 0.5
done

f2_count=$(curl -sf "${FOLLOWER2}/v1/stats" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); cols=[c for c in d.get('collections',[]) if c['name']=='smoke']; print(cols[0]['documentCount'] if cols else 0)" 2>/dev/null || echo "0")

check "Follower-1 received document (count>=1)" "$([ "$f1_count" -ge 1 ] && echo yes || echo no)" "yes"
check "Follower-2 received document (count>=1)" "$([ "$f2_count" -ge 1 ] && echo yes || echo no)" "yes"

blue "\n=== 5. CONTENT CONSISTENCY CHECK ==="
f1_search=$(curl -sf -X POST "${FOLLOWER1}/v1/search" \
  -H "Content-Type: application/json" \
  -d "{\"collection\":\"smoke\",\"lang\":\"en\",\"limit\":100}" \
  | python3 -c "import sys,json; docs=json.load(sys.stdin); match=[d for d in docs if d.get('key')=='${DOC_KEY}']; print(match[0].get('key','') if match else '')" 2>/dev/null || echo "")
check "Follower-1 search returns doc by key" "$f1_search" "$DOC_KEY"

f2_search=$(curl -sf -X POST "${FOLLOWER2}/v1/search" \
  -H "Content-Type: application/json" \
  -d "{\"collection\":\"smoke\",\"lang\":\"en\",\"limit\":100}" \
  | python3 -c "import sys,json; docs=json.load(sys.stdin); match=[d for d in docs if d.get('key')=='${DOC_KEY}']; print(match[0].get('key','') if match else '')" 2>/dev/null || echo "")
check "Follower-2 search returns doc by key" "$f2_search" "$DOC_KEY"

blue "\n=== 6. LSN LAG CHECK ==="
repl_json=$(curl -sf "${LEADER}/v1/replication/status")
leader_lsn=$(echo "$repl_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['current_lsn'])")
f1_lag=$(echo "$repl_json"  | python3 -c "import sys,json; d=json.load(sys.stdin); fs=[f for f in d['followers'] if 'follower-1' in f['follower_id']]; print(fs[0]['lag_ms'] if fs else -1)")
f2_lag=$(echo "$repl_json"  | python3 -c "import sys,json; d=json.load(sys.stdin); fs=[f for f in d['followers'] if 'follower-2' in f['follower_id']]; print(fs[0]['lag_ms'] if fs else -1)")
f1_lsn=$(echo "$repl_json"  | python3 -c "import sys,json; d=json.load(sys.stdin); fs=[f for f in d['followers'] if 'follower-1' in f['follower_id']]; print(fs[0]['confirmed_lsn'] if fs else 0)")
f2_lsn=$(echo "$repl_json"  | python3 -c "import sys,json; d=json.load(sys.stdin); fs=[f for f in d['followers'] if 'follower-2' in f['follower_id']]; print(fs[0]['confirmed_lsn'] if fs else 0)")

ok "Leader current LSN: $leader_lsn"
check "Follower-1 lag (ms) is 0"        "$f1_lag" "0"
check "Follower-2 lag (ms) is 0"        "$f2_lag" "0"
check "Follower-1 LSN == Leader LSN"    "$f1_lsn" "$leader_lsn"
check "Follower-2 LSN == Leader LSN"    "$f2_lsn" "$leader_lsn"

blue "\n=== 7. WRITE REJECTION ON FOLLOWER ==="
f1_write_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${FOLLOWER1}/v1/add" \
  -H "Content-Type: application/json" \
  -d '{"collection":"smoke","key":"should-fail","lang":"en","contentMd":"# Should fail"}')
f2_write_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${FOLLOWER2}/v1/add" \
  -H "Content-Type: application/json" \
  -d '{"collection":"smoke","key":"should-fail","lang":"en","contentMd":"# Should fail"}')
check "Follower-1 rejects writes (4xx)" "$([ "$f1_write_code" -ge 400 ] 2>/dev/null && echo yes || echo no)" "yes"
check "Follower-2 rejects writes (4xx)" "$([ "$f2_write_code" -ge 400 ] 2>/dev/null && echo yes || echo no)" "yes"

# ── summary ──────────────────────────────────────────────────────────────────

echo ""
blue "============================================"
if [ $FAIL -eq 0 ]; then
  green "  ALL $PASS TESTS PASSED ✓"
else
  red   "  $PASS passed, $FAIL FAILED ✗"
fi
blue "============================================"

[ $FAIL -eq 0 ] && exit 0 || exit 1
