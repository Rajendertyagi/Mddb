#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

PASS=0
FAIL=0

step() { printf "\n${CYAN}${BOLD}▸ %s${NC}\n" "$1"; }
ok()   { printf "  ${GREEN}✓ %s${NC}\n" "$1"; PASS=$((PASS+1)); }
fail() { printf "  ${RED}✗ %s${NC}\n" "$1"; FAIL=$((FAIL+1)); }

LEADER_HTTP=":11123"
LEADER_GRPC=":11124"
FOLLOWER_HTTP=":11133"
FOLLOWER_GRPC=":11134"

TMPDIR=$(mktemp -d)
LEADER_DB="$TMPDIR/leader.db"
FOLLOWER_DB="$TMPDIR/follower.db"

LEADER_PID=""
FOLLOWER_PID=""

cleanup() {
    printf "\n${CYAN}Cleaning up...${NC}\n"
    [ -n "$FOLLOWER_PID" ] && kill "$FOLLOWER_PID" 2>/dev/null || true
    [ -n "$LEADER_PID" ] && kill "$LEADER_PID" 2>/dev/null || true
    sleep 1
    # Force kill if still running
    [ -n "$FOLLOWER_PID" ] && kill -9 "$FOLLOWER_PID" 2>/dev/null || true
    [ -n "$LEADER_PID" ] && kill -9 "$LEADER_PID" 2>/dev/null || true
    rm -rf "$TMPDIR"
}
trap cleanup EXIT

wait_healthy() {
    local url="$1"
    local name="$2"
    local max_wait=15
    local i=0
    while [ $i -lt $max_wait ]; do
        if curl -sf "$url/health" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        i=$((i+1))
    done
    return 1
}

# ── 1. Build ──────────────────────────────────────────────
step "Build"
if go build -o "$TMPDIR/mddbd" . 2>&1; then
    ok "go build"
else
    fail "go build"
    exit 1
fi

# ── 2. Start Leader ──────────────────────────────────────
step "Start Leader"
MDDB_PATH="$LEADER_DB" \
MDDB_ADDR="$LEADER_HTTP" \
MDDB_GRPC_ADDR="$LEADER_GRPC" \
MDDB_REPLICATION_ROLE=leader \
MDDB_NODE_ID=leader-1 \
MDDB_METRICS=false \
"$TMPDIR/mddbd" > "$TMPDIR/leader.log" 2>&1 &
LEADER_PID=$!

if wait_healthy "http://localhost${LEADER_HTTP}" "leader"; then
    ok "Leader healthy (PID=$LEADER_PID, HTTP=$LEADER_HTTP, gRPC=$LEADER_GRPC)"
else
    fail "Leader did not become healthy in 15s"
    cat "$TMPDIR/leader.log"
    exit 1
fi

# ── 3. Start Follower ───────────────────────────────────
step "Start Follower"
MDDB_PATH="$FOLLOWER_DB" \
MDDB_ADDR="$FOLLOWER_HTTP" \
MDDB_GRPC_ADDR="$FOLLOWER_GRPC" \
MDDB_REPLICATION_ROLE=follower \
MDDB_REPLICATION_LEADER_ADDR="localhost${LEADER_GRPC}" \
MDDB_NODE_ID=follower-1 \
MDDB_METRICS=false \
"$TMPDIR/mddbd" > "$TMPDIR/follower.log" 2>&1 &
FOLLOWER_PID=$!

if wait_healthy "http://localhost${FOLLOWER_HTTP}" "follower"; then
    ok "Follower healthy (PID=$FOLLOWER_PID, HTTP=$FOLLOWER_HTTP, gRPC=$FOLLOWER_GRPC)"
else
    fail "Follower did not become healthy in 15s"
    cat "$TMPDIR/follower.log"
    exit 1
fi

# Wait for initial snapshot sync
sleep 3

# ── 4. Write Documents to Leader ────────────────────────
step "Write documents to leader"

DOC_COUNT=10
WRITE_OK=0
for i in $(seq 1 $DOC_COUNT); do
    STATUS=$(curl -sf -o /dev/null -w "%{http_code}" \
        -X POST "http://localhost${LEADER_HTTP}/v1/add" \
        -H "Content-Type: application/json" \
        -d "{
            \"collection\": \"test\",
            \"key\": \"doc-${i}\",
            \"lang\": \"en\",
            \"meta\": {\"idx\": [\"${i}\"]},
            \"contentMd\": \"# Document ${i}\nReplication test content.\"
        }" 2>/dev/null || echo "000")
    if [ "$STATUS" = "200" ]; then
        WRITE_OK=$((WRITE_OK+1))
    fi
done

if [ "$WRITE_OK" -eq "$DOC_COUNT" ]; then
    ok "Wrote $DOC_COUNT documents to leader"
else
    fail "Only $WRITE_OK/$DOC_COUNT writes succeeded"
fi

# ── 5. Verify follower rejects writes ──────────────────
step "Verify follower read-only"
WRITE_STATUS=$(curl -sf -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost${FOLLOWER_HTTP}/v1/add" \
    -H "Content-Type: application/json" \
    -d '{"collection":"test","key":"should-fail","lang":"en","contentMd":"# Nope"}' \
    2>/dev/null || echo "000")

if [ "$WRITE_STATUS" = "403" ]; then
    ok "Follower correctly rejected write (HTTP 403)"
elif [ "$WRITE_STATUS" = "000" ]; then
    fail "Follower write request failed (connection error)"
else
    fail "Follower returned unexpected status $WRITE_STATUS (expected 403)"
fi

# ── 6. Wait for replication & read from follower ───────
step "Read documents from follower"

# Wait for replication to catch up
MAX_WAIT=10
REPLICATED=false
for attempt in $(seq 1 $MAX_WAIT); do
    READ_OK=0
    for i in $(seq 1 $DOC_COUNT); do
        BODY=$(curl -sf -X POST "http://localhost${FOLLOWER_HTTP}/v1/get" \
            -H "Content-Type: application/json" \
            -d "{\"collection\":\"test\",\"key\":\"doc-${i}\",\"lang\":\"en\"}" 2>/dev/null || echo "")
        if echo "$BODY" | grep -q "doc-${i}" 2>/dev/null; then
            READ_OK=$((READ_OK+1))
        fi
    done
    if [ "$READ_OK" -eq "$DOC_COUNT" ]; then
        REPLICATED=true
        break
    fi
    sleep 1
done

if [ "$REPLICATED" = "true" ]; then
    ok "All $DOC_COUNT documents replicated to follower (attempt $attempt)"
else
    fail "Only $READ_OK/$DOC_COUNT documents found on follower after ${MAX_WAIT}s"
fi

# ── 7. Verify document content ─────────────────────────
step "Verify document content"
DOC5=$(curl -sf -X POST "http://localhost${FOLLOWER_HTTP}/v1/get" \
    -H "Content-Type: application/json" \
    -d '{"collection":"test","key":"doc-5","lang":"en"}' 2>/dev/null || echo "")

if echo "$DOC5" | grep -q "Document 5"; then
    ok "Document content matches on follower"
else
    fail "Document content mismatch on follower"
    echo "  Got: $DOC5"
fi

# ── 8. Search on follower ──────────────────────────────
step "Search on follower"
SEARCH_RESULT=$(curl -sf -X POST "http://localhost${FOLLOWER_HTTP}/v1/search" \
    -H "Content-Type: application/json" \
    -d '{"collection":"test","filterMeta":{"idx":["3"]}}' 2>/dev/null || echo "[]")

if echo "$SEARCH_RESULT" | grep -q "doc-3"; then
    ok "Metadata search works on follower"
else
    fail "Metadata search failed on follower"
    echo "  Got: $SEARCH_RESULT"
fi

# ── 9. Check replication status ────────────────────────
step "Replication status"

LEADER_STATUS=$(curl -sf "http://localhost${LEADER_HTTP}/v1/replication/status" 2>/dev/null || echo "{}")
FOLLOWER_STATUS=$(curl -sf "http://localhost${FOLLOWER_HTTP}/v1/replication/status" 2>/dev/null || echo "{}")

LEADER_ROLE=$(echo "$LEADER_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('role',''))" 2>/dev/null || echo "")
FOLLOWER_ROLE=$(echo "$FOLLOWER_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('role',''))" 2>/dev/null || echo "")
LEADER_LSN=$(echo "$LEADER_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('current_lsn',0))" 2>/dev/null || echo "0")
FOLLOWER_LSN=$(echo "$FOLLOWER_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('current_lsn',0))" 2>/dev/null || echo "0")

if [ "$LEADER_ROLE" = "leader" ]; then
    ok "Leader reports role=leader"
else
    fail "Leader reports role=$LEADER_ROLE (expected leader)"
fi

if [ "$FOLLOWER_ROLE" = "follower" ]; then
    ok "Follower reports role=follower"
else
    fail "Follower reports role=$FOLLOWER_ROLE (expected follower)"
fi

if [ "$LEADER_LSN" -gt 0 ]; then
    ok "Leader LSN=$LEADER_LSN"
else
    fail "Leader LSN is 0"
fi

printf "  ${BOLD}Leader LSN:   ${CYAN}%s${NC}\n" "$LEADER_LSN"
printf "  ${BOLD}Follower LSN: ${CYAN}%s${NC}\n" "$FOLLOWER_LSN"

# ── 10. Delete on leader, verify on follower ───────────
step "Delete replication"
DEL_STATUS=$(curl -sf -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost${LEADER_HTTP}/v1/delete" \
    -H "Content-Type: application/json" \
    -d '{"collection":"test","key":"doc-1","lang":"en"}' 2>/dev/null || echo "000")

if [ "$DEL_STATUS" = "200" ]; then
    ok "Delete on leader succeeded"
else
    fail "Delete on leader returned $DEL_STATUS"
fi

# Wait for delete to replicate
sleep 2

DEL_CHECK=$(curl -sf -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost${FOLLOWER_HTTP}/v1/get" \
    -H "Content-Type: application/json" \
    -d '{"collection":"test","key":"doc-1","lang":"en"}' 2>/dev/null || echo "000")

if [ "$DEL_CHECK" = "404" ] || [ "$DEL_CHECK" = "400" ]; then
    ok "Delete replicated to follower (doc-1 not found)"
else
    fail "Deleted doc-1 still found on follower (HTTP $DEL_CHECK)"
fi

# ── 11. Follower reconnect test ────────────────────────
step "Follower reconnect"

# Kill follower
kill "$FOLLOWER_PID" 2>/dev/null || true
sleep 1
kill -9 "$FOLLOWER_PID" 2>/dev/null || true
FOLLOWER_PID=""
ok "Follower stopped"

# Write more documents while follower is down
for i in $(seq 11 15); do
    curl -sf -o /dev/null \
        -X POST "http://localhost${LEADER_HTTP}/v1/add" \
        -H "Content-Type: application/json" \
        -d "{
            \"collection\": \"test\",
            \"key\": \"doc-${i}\",
            \"lang\": \"en\",
            \"contentMd\": \"# Document ${i}\nWritten while follower was down.\"
        }" 2>/dev/null || true
done
ok "Wrote 5 more documents while follower was down"

# Restart follower
MDDB_PATH="$FOLLOWER_DB" \
MDDB_ADDR="$FOLLOWER_HTTP" \
MDDB_GRPC_ADDR="$FOLLOWER_GRPC" \
MDDB_REPLICATION_ROLE=follower \
MDDB_REPLICATION_LEADER_ADDR="localhost${LEADER_GRPC}" \
MDDB_NODE_ID=follower-1 \
MDDB_METRICS=false \
"$TMPDIR/mddbd" > "$TMPDIR/follower2.log" 2>&1 &
FOLLOWER_PID=$!

if wait_healthy "http://localhost${FOLLOWER_HTTP}" "follower"; then
    ok "Follower restarted (PID=$FOLLOWER_PID)"
else
    fail "Follower did not restart in 15s"
    cat "$TMPDIR/follower2.log"
fi

# Wait for catch-up
sleep 5

# Check new documents on follower
CATCH_UP_OK=0
for i in $(seq 11 15); do
    BODY=$(curl -sf -X POST "http://localhost${FOLLOWER_HTTP}/v1/get" \
        -H "Content-Type: application/json" \
        -d "{\"collection\":\"test\",\"key\":\"doc-${i}\",\"lang\":\"en\"}" 2>/dev/null || echo "")
    if echo "$BODY" | grep -q "doc-${i}" 2>/dev/null; then
        CATCH_UP_OK=$((CATCH_UP_OK+1))
    fi
done

if [ "$CATCH_UP_OK" -eq 5 ]; then
    ok "Follower caught up: all 5 new documents replicated after reconnect"
else
    fail "Follower caught up only $CATCH_UP_OK/5 documents after reconnect"
fi

# ── Summary ──────────────────────────────────────────────
printf "\n${BOLD}━━━ Replication Test Summary ━━━${NC}\n"
printf "  ${GREEN}Passed: %d${NC}  ${RED}Failed: %d${NC}\n\n" "$PASS" "$FAIL"

if [ "$FAIL" -gt 0 ]; then
    printf "${RED}${BOLD}FAILED${NC}\n"
    printf "\n${YELLOW}Leader log (last 20 lines):${NC}\n"
    tail -20 "$TMPDIR/leader.log" 2>/dev/null || true
    printf "\n${YELLOW}Follower log (last 20 lines):${NC}\n"
    tail -20 "$TMPDIR/follower.log" 2>/dev/null || true
    [ -f "$TMPDIR/follower2.log" ] && { printf "\n${YELLOW}Follower restart log (last 20 lines):${NC}\n"; tail -20 "$TMPDIR/follower2.log" 2>/dev/null || true; }
    exit 1
else
    printf "${GREEN}${BOLD}ALL REPLICATION TESTS PASSED${NC}\n"
    exit 0
fi
