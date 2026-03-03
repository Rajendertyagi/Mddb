#!/usr/bin/env bash
# =============================================================================
# test_cluster_stress.sh — MDDB cluster bulk write + consistency stress test
#
# Tests:
#   1. Write N documents to leader
#   2. Wait for full replication
#   3. Verify all N docs exist on both followers via search
#   4. Delete half the docs on leader, verify deletion propagated
#
# Usage:
#   ./scripts/tests/test_cluster_stress.sh [DOC_COUNT]
#   ./scripts/tests/test_cluster_stress.sh 50
# =============================================================================

set -euo pipefail

LEADER="http://localhost:11023"
FOLLOWER1="http://localhost:11033"
FOLLOWER2="http://localhost:11043"

DOC_COUNT="${1:-20}"
COLLECTION="stress-$$"
PASS=0
FAIL=0

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
blue()  { printf '\033[34m%s\033[0m\n' "$*"; }

ok()   { green "  ✓ $1"; PASS=$((PASS + 1)); }
fail() { red   "  ✗ $1"; FAIL=$((FAIL + 1)); }

check() {
  local desc="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then ok "$desc"; else fail "$desc — want: '$want', got: '$got'"; fi
}

blue "=== STRESS TEST: $DOC_COUNT documents, collection=$COLLECTION ==="

# ── write docs ───────────────────────────────────────────────────────────────
blue "\n--- Writing $DOC_COUNT documents to leader ---"
for i in $(seq 1 "$DOC_COUNT"); do
  curl -sf -X POST "${LEADER}/v1/add" \
    -H "Content-Type: application/json" \
    -d "{\"collection\":\"${COLLECTION}\",\"key\":\"doc-$i\",\"lang\":\"en\",\"contentMd\":\"# Doc $i\",\"meta\":{\"n\":[\"$i\"]}}" \
    > /dev/null
done
ok "Wrote $DOC_COUNT documents to leader"

# ── wait for replication ──────────────────────────────────────────────────────
blue "\n--- Waiting for replication (max 15s) ---"
for attempt in $(seq 1 30); do
  f1_count=$(curl -sf "${FOLLOWER1}/v1/stats" \
    | python3 -c "import sys,json; d=json.load(sys.stdin); cols=[c for c in d.get('collections',[]) if c['name']=='${COLLECTION}']; print(cols[0]['documentCount'] if cols else 0)" 2>/dev/null || echo "0")
  if [ "$f1_count" -eq "$DOC_COUNT" ]; then break; fi
  sleep 0.5
done

f2_count=$(curl -sf "${FOLLOWER2}/v1/stats" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); cols=[c for c in d.get('collections',[]) if c['name']=='${COLLECTION}']; print(cols[0]['documentCount'] if cols else 0)" 2>/dev/null || echo "0")

check "Follower-1 document count == $DOC_COUNT" "$f1_count" "$DOC_COUNT"
check "Follower-2 document count == $DOC_COUNT" "$f2_count" "$DOC_COUNT"

# ── content verification ──────────────────────────────────────────────────────
blue "\n--- Verifying content on followers ---"
f1_search_count=$(curl -sf -X POST "${FOLLOWER1}/v1/search" \
  -H "Content-Type: application/json" \
  -d "{\"collection\":\"${COLLECTION}\",\"lang\":\"en\",\"limit\":$((DOC_COUNT + 10))}" \
  | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

check "Follower-1 search returns $DOC_COUNT results" "$f1_search_count" "$DOC_COUNT"

# ── delete half ───────────────────────────────────────────────────────────────
HALF=$((DOC_COUNT / 2))
blue "\n--- Deleting first $HALF documents on leader ---"
for i in $(seq 1 "$HALF"); do
  curl -sf -X POST "${LEADER}/v1/delete" \
    -H "Content-Type: application/json" \
    -d "{\"collection\":\"${COLLECTION}\",\"key\":\"doc-$i\",\"lang\":\"en\"}" \
    > /dev/null
done
ok "Deleted $HALF documents from leader"

# wait for delete replication
sleep 2
for attempt in $(seq 1 20); do
  f1_after=$(curl -sf "${FOLLOWER1}/v1/stats" \
    | python3 -c "import sys,json; d=json.load(sys.stdin); cols=[c for c in d.get('collections',[]) if c['name']=='${COLLECTION}']; print(cols[0]['documentCount'] if cols else 0)" 2>/dev/null || echo "$DOC_COUNT")
  if [ "$f1_after" -le $((DOC_COUNT - HALF)) ]; then break; fi
  sleep 0.5
done

check "Follower-1 count after deletions == $((DOC_COUNT - HALF))" "$f1_after" "$((DOC_COUNT - HALF))"

# ── LSN consistency ────────────────────────────────────────────────────────────
blue "\n--- Final LSN check ---"
repl_json=$(curl -sf "${LEADER}/v1/replication/status")
leader_lsn=$(echo "$repl_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['current_lsn'])")
f1_lsn=$(echo "$repl_json"     | python3 -c "import sys,json; d=json.load(sys.stdin); fs=[f for f in d['followers'] if 'follower-1' in f['follower_id']]; print(fs[0]['confirmed_lsn'] if fs else 0)")
f2_lsn=$(echo "$repl_json"     | python3 -c "import sys,json; d=json.load(sys.stdin); fs=[f for f in d['followers'] if 'follower-2' in f['follower_id']]; print(fs[0]['confirmed_lsn'] if fs else 0)")

ok "Leader LSN: $leader_lsn"
check "Follower-1 LSN == Leader LSN" "$f1_lsn" "$leader_lsn"
check "Follower-2 LSN == Leader LSN" "$f2_lsn" "$leader_lsn"

# ── summary ───────────────────────────────────────────────────────────────────
echo ""
blue "============================================"
if [ $FAIL -eq 0 ]; then
  green "  ALL $PASS STRESS TESTS PASSED ✓"
else
  red   "  $PASS passed, $FAIL FAILED ✗"
fi
blue "============================================"

[ $FAIL -eq 0 ] && exit 0 || exit 1
