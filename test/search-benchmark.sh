#!/bin/bash

# MDDB FTS Algorithm Benchmark
# Compares all FTS search algorithms: tfidf, bm25, bm25f, pmisparse
# Each with and without fuzzy matching.
#
# Self-contained: builds mddbd, starts server, seeds 10K docs, benchmarks, generates report.
#
# Usage:
#   ./test/search-benchmark.sh
#   ITERATIONS=100 DOCS=5000 PORT=11099 ./test/search-benchmark.sh
#   RUNS=10 ITERATIONS=50 ./test/search-benchmark.sh

set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
DOCS=${DOCS:-10000}
ITERATIONS=${ITERATIONS:-50}
RUNS=${RUNS:-1}
PORT=${PORT:-11099}
COLLECTION="bench"
WARMUP=5

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MDDBD_SRC="$PROJECT_ROOT/services/mddbd"
MDDBD_BIN="/tmp/mddbd-bench-$$"
DB_FILE="/tmp/mddb-bench-$$.db"
BENCH_TMP="/tmp/mddb-bench-data-$$"
SERVER_URL="http://127.0.0.1:$PORT"
RESULTS_DIR="$SCRIPT_DIR"
DOCS_DIR="$PROJECT_ROOT/docs"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RAW_FILE="$RESULTS_DIR/search-benchmark-results-$TIMESTAMP.txt"
REPORT_FILE="$DOCS_DIR/BENCHMARK.md"

# Colors
R='\033[0;31m'; G='\033[0;32m'; Y='\033[1;33m'; B='\033[0;34m'
P='\033[0;35m'; C='\033[0;36m'; NC='\033[0m'

# ── Cleanup trap ──────────────────────────────────────────────────────────────
MDDBD_PID=""
cleanup() {
    if [ -n "$MDDBD_PID" ] && kill -0 "$MDDBD_PID" 2>/dev/null; then
        kill "$MDDBD_PID" 2>/dev/null || true
        wait "$MDDBD_PID" 2>/dev/null || true
    fi
    rm -f "$MDDBD_BIN" "$DB_FILE" "$DB_FILE.lock" 2>/dev/null || true
    rm -rf "$BENCH_TMP" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "$BENCH_TMP"

# ── Topic word pools (20 topics) ─────────────────────────────────────────────
TOPICS_0=("kubernetes" "docker" "containers" "orchestration" "microservices" "deployment" "cluster" "pods" "helm" "ingress" "namespace" "service" "scaling" "load" "balancer")
TOPICS_1=("neural" "network" "training" "model" "inference" "transformer" "attention" "gradient" "backpropagation" "epoch" "layer" "weights" "activation" "loss" "optimizer")
TOPICS_2=("database" "query" "index" "transaction" "schema" "normalization" "replication" "sharding" "partition" "consistency" "isolation" "durability" "join" "aggregate" "cursor")
TOPICS_3=("security" "authentication" "authorization" "encryption" "certificate" "firewall" "vulnerability" "penetration" "token" "oauth" "session" "password" "hash" "salt" "audit")
TOPICS_4=("cloud" "infrastructure" "serverless" "lambda" "storage" "compute" "network" "region" "availability" "elastic" "autoscaling" "terraform" "provisioning" "monitoring" "alerting")
TOPICS_5=("machine" "learning" "supervised" "unsupervised" "classification" "regression" "clustering" "feature" "engineering" "dimensionality" "reduction" "overfitting" "validation" "cross" "ensemble")
TOPICS_6=("frontend" "react" "component" "state" "rendering" "virtual" "dom" "hooks" "context" "redux" "typescript" "webpack" "bundler" "stylesheet" "responsive")
TOPICS_7=("api" "rest" "graphql" "endpoint" "request" "response" "middleware" "routing" "authentication" "rate" "limiting" "versioning" "documentation" "swagger" "gateway")
TOPICS_8=("distributed" "systems" "consensus" "raft" "paxos" "gossip" "protocol" "failure" "detection" "leader" "election" "quorum" "partition" "tolerance" "eventual")
TOPICS_9=("search" "algorithm" "ranking" "relevance" "indexing" "tokenization" "stemming" "inverted" "tfidf" "bm25" "scoring" "precision" "recall" "fuzzy" "matching")
TOPICS_10=("python" "flask" "django" "pandas" "numpy" "scipy" "matplotlib" "jupyter" "virtualenv" "pip" "decorator" "generator" "comprehension" "iterator" "asyncio")
TOPICS_11=("golang" "goroutine" "channel" "interface" "struct" "pointer" "slice" "map" "concurrency" "mutex" "waitgroup" "context" "defer" "panic" "recover")
TOPICS_12=("devops" "pipeline" "continuous" "integration" "delivery" "jenkins" "github" "actions" "artifact" "staging" "production" "rollback" "canary" "blue" "green")
TOPICS_13=("blockchain" "ledger" "smart" "contract" "consensus" "mining" "proof" "stake" "hash" "merkle" "tree" "decentralized" "wallet" "token" "nft")
TOPICS_14=("testing" "unit" "integration" "end" "coverage" "assertion" "mock" "stub" "fixture" "benchmark" "regression" "acceptance" "behavior" "driven" "development")
TOPICS_15=("networking" "tcp" "udp" "http" "dns" "routing" "subnet" "cidr" "vpn" "proxy" "latency" "bandwidth" "throughput" "packet" "socket")
TOPICS_16=("data" "pipeline" "etl" "transformation" "ingestion" "streaming" "batch" "processing" "warehouse" "lake" "catalog" "lineage" "quality" "governance" "schema")
TOPICS_17=("mobile" "ios" "android" "swift" "kotlin" "react" "native" "flutter" "widget" "gesture" "navigation" "notification" "push" "offline" "sync")
TOPICS_18=("observability" "logging" "tracing" "metrics" "prometheus" "grafana" "jaeger" "opentelemetry" "span" "trace" "dashboard" "alert" "slo" "sli" "error")
TOPICS_19=("architecture" "monolith" "microservice" "event" "driven" "cqrs" "saga" "domain" "bounded" "context" "hexagonal" "clean" "layered" "modular" "coupling")

NUM_TOPICS=20

TEMPLATES=(
    "The %s system uses %s for %s with %s integration."
    "Modern %s approaches leverage %s to improve %s and optimize %s."
    "When implementing %s you should consider %s alongside %s for better %s."
    "The relationship between %s and %s is critical for %s in production %s."
    "Advanced %s techniques combine %s with %s to achieve reliable %s."
    "Understanding %s requires knowledge of %s especially when dealing with %s and %s."
    "Best practices for %s include using %s implementing %s and monitoring %s."
    "The %s framework provides %s capabilities for %s with built-in %s support."
)

CATEGORIES=("tech" "ai" "database" "security" "cloud" "ml" "frontend" "api" "distributed" "search" "python" "golang" "devops" "blockchain" "testing" "networking" "data" "mobile" "observability" "architecture")

# ── Helper functions ──────────────────────────────────────────────────────────
now_ns() { date +%s%N; }
now_ms() { echo $(( $(now_ns) / 1000000 )); }

get_topic_words() {
    local topic_idx=$1
    local count=$2
    local varname="TOPICS_${topic_idx}[@]"
    local words=("${!varname}")
    local len=${#words[@]}
    local result=()
    for _ in $(seq 1 "$count"); do
        result+=("${words[$((RANDOM % len))]}")
    done
    echo "${result[@]}"
}

generate_doc() {
    local doc_idx=$1
    local t1=$((doc_idx % NUM_TOPICS))
    local t2=$(( (doc_idx * 7 + 3) % NUM_TOPICS ))
    local t3=$(( (doc_idx * 13 + 11) % NUM_TOPICS ))
    local sentences=$(( (RANDOM % 4) + 3 ))
    local content=""
    for s in $(seq 1 "$sentences"); do
        local tmpl_idx=$(( (doc_idx + s) % ${#TEMPLATES[@]} ))
        local tmpl="${TEMPLATES[$tmpl_idx]}"
        local src=$t1; [ $((s % 3)) -eq 1 ] && src=$t2; [ $((s % 3)) -eq 2 ] && src=$t3
        local w1 w2 w3 w4
        read -r w1 w2 w3 w4 <<< "$(get_topic_words $src 4)"
        # shellcheck disable=SC2059
        local sentence
        sentence=$(printf "$tmpl" "$w1" "$w2" "$w3" "$w4")
        content="$content $sentence"
    done
    echo "$content"
}

# Compute stats from a file of newline-separated nanosecond values
# Outputs: avg_ms p50_ms p95_ms p99_ms min_ms max_ms qps count
compute_stats() {
    local file=$1
    local sorted_file="${file}.sorted"
    sort -n "$file" > "$sorted_file"
    local count
    count=$(wc -l < "$sorted_file" | tr -d ' ')
    if [ "$count" -eq 0 ]; then
        echo "0 0 0 0 0 0 0 0"
        return
    fi
    local sum=0
    while read -r v; do
        sum=$((sum + v))
    done < "$sorted_file"
    local avg_ns=$((sum / count))
    local avg_ms; avg_ms=$(echo "scale=2; $avg_ns / 1000000" | bc)
    local p50_line=$(( (count + 1) / 2 ))
    local p50_ns; p50_ns=$(sed -n "${p50_line}p" "$sorted_file")
    local p50_ms; p50_ms=$(echo "scale=2; $p50_ns / 1000000" | bc)
    local p95_line=$(( (count * 95 + 99) / 100 ))
    [ "$p95_line" -gt "$count" ] && p95_line=$count
    local p95_ns; p95_ns=$(sed -n "${p95_line}p" "$sorted_file")
    local p95_ms; p95_ms=$(echo "scale=2; $p95_ns / 1000000" | bc)
    local p99_line=$(( (count * 99 + 99) / 100 ))
    [ "$p99_line" -gt "$count" ] && p99_line=$count
    local p99_ns; p99_ns=$(sed -n "${p99_line}p" "$sorted_file")
    local p99_ms; p99_ms=$(echo "scale=2; $p99_ns / 1000000" | bc)
    local min_ns; min_ns=$(head -1 "$sorted_file")
    local min_ms; min_ms=$(echo "scale=2; $min_ns / 1000000" | bc)
    local max_ns; max_ns=$(tail -1 "$sorted_file")
    local max_ms; max_ms=$(echo "scale=2; $max_ns / 1000000" | bc)
    local qps; qps=$(echo "scale=0; 1000000000 / $avg_ns" | bc)
    rm -f "$sorted_file"
    echo "$avg_ms $p50_ms $p95_ms $p99_ms $min_ms $max_ms $qps $count"
}

# ── Phase 1: Build & Start ───────────────────────────────────────────────────
echo -e "${B}════════════════════════════════════════════════════════════${NC}"
echo -e "${G}  MDDB FTS Algorithm Benchmark${NC}"
echo -e "${B}════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${C}Building mddbd...${NC}"
(cd "$MDDBD_SRC" && go build -o "$MDDBD_BIN" .)
echo -e "${G}  Built: $MDDBD_BIN${NC}"

echo -e "${C}Starting server on port $PORT...${NC}"
MDDB_ADDR=":$PORT" "$MDDBD_BIN" "$DB_FILE" > /dev/null 2>&1 &
MDDBD_PID=$!

for i in $(seq 1 30); do
    if curl -s "$SERVER_URL/health" > /dev/null 2>&1; then
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e "${R}Server failed to start within 30s${NC}"
        exit 1
    fi
    sleep 0.2
done
echo -e "${G}  Server ready (PID=$MDDBD_PID)${NC}"
echo ""

# ── Phase 2: Seed documents ──────────────────────────────────────────────────
echo -e "${P}Seeding $DOCS documents...${NC}"
seed_start=$(now_ms)

for i in $(seq 1 "$DOCS"); do
    content=$(generate_doc "$i")
    topic_idx=$((i % NUM_TOPICS))
    category="${CATEGORIES[$topic_idx]}"

    curl -s -X POST "$SERVER_URL/v1/add" \
        -H "Content-Type: application/json" \
        -d "{\"collection\":\"$COLLECTION\",\"key\":\"doc-$i\",\"lang\":\"en_US\",\"meta\":{\"category\":[\"$category\"],\"batch\":[\"$((i / 500))\"]},\"contentMd\":$(echo "$content" | jq -Rs .)}" \
        > /dev/null

    if [ $((i % 500)) -eq 0 ] || [ "$i" -eq "$DOCS" ]; then
        pct=$((i * 100 / DOCS))
        elapsed=$(( $(now_ms) - seed_start ))
        rate=$(echo "scale=0; $i * 1000 / ($elapsed + 1)" | bc)
        printf "\r  [%-50s] %d%% (%d/%d, %s docs/sec)" \
            "$(printf '#%.0s' $(seq 1 $((pct / 2))))" \
            "$pct" "$i" "$DOCS" "$rate"
    fi
done

seed_elapsed=$(( $(now_ms) - seed_start ))
echo ""
echo -e "${G}  Seeded $DOCS docs in $((seed_elapsed / 1000)).$((seed_elapsed % 1000 / 100))s ($(echo "scale=0; $DOCS * 1000 / ($seed_elapsed + 1)" | bc) docs/sec)${NC}"
echo ""

# ── Phase 3: Benchmark ───────────────────────────────────────────────────────
QUERIES=(
    "kubernetes deployment cluster"
    "neural network training"
    "database query optimization"
    "machine learning model"
    "security authentication token"
    "cloud infrastructure scaling"
    "data pipeline processing"
    "api gateway middleware"
    "distributed consensus protocol"
    "search algorithm ranking"
)

ALGORITHMS=("tfidf" "bm25" "bm25f" "pmisparse")
FUZZY_MODES=(0 1)

TOTAL_ITERS=$((RUNS * ITERATIONS))
echo -e "${P}Running benchmarks: ${#ALGORITHMS[@]} algorithms x ${#FUZZY_MODES[@]} fuzzy modes x ${#QUERIES[@]} queries x $ITERATIONS iterations x $RUNS runs${NC}"
echo -e "${P}Total samples per config: $((TOTAL_ITERS * ${#QUERIES[@]}))${NC}"
echo ""

# Initialize latency files
for algo in "${ALGORITHMS[@]}"; do
    for fz in "${FUZZY_MODES[@]}"; do
        > "$BENCH_TMP/${algo}_${fz}.lat"
    done
done

for run in $(seq 1 "$RUNS"); do
    echo -e "${B}── Run $run/$RUNS ──${NC}"

    for algo in "${ALGORITHMS[@]}"; do
        for fz in "${FUZZY_MODES[@]}"; do
            label="$algo"
            [ "$fz" -gt 0 ] && label="${algo}+fuzzy"
            latfile="$BENCH_TMP/${algo}_${fz}.lat"

            echo -ne "  ${C}$label${NC}: "

            # Warmup
            for _ in $(seq 1 $WARMUP); do
                q="${QUERIES[$((RANDOM % ${#QUERIES[@]}))]}"
                curl -s -X POST "$SERVER_URL/v1/fts" \
                    -H "Content-Type: application/json" \
                    -d "{\"collection\":\"$COLLECTION\",\"query\":\"$q\",\"algorithm\":\"$algo\",\"limit\":10,\"fuzzy\":$fz}" \
                    > /dev/null
            done

            # Measured runs
            for qi in $(seq 0 $(( ${#QUERIES[@]} - 1 ))); do
                q="${QUERIES[$qi]}"
                for _ in $(seq 1 "$ITERATIONS"); do
                    t_start=$(now_ns)
                    resp=$(curl -s -X POST "$SERVER_URL/v1/fts" \
                        -H "Content-Type: application/json" \
                        -d "{\"collection\":\"$COLLECTION\",\"query\":\"$q\",\"algorithm\":\"$algo\",\"limit\":10,\"fuzzy\":$fz}")
                    t_end=$(now_ns)
                    echo $((t_end - t_start)) >> "$latfile"
                done
                # Save result count from last response for this query
                rc=$(echo "$resp" | jq -r '.total // 0' 2>/dev/null || echo "0")
                echo "$rc" > "$BENCH_TMP/${algo}_${fz}_q${qi}.rc"
            done

            # Per-run stats
            run_count=$(wc -l < "$latfile" | tr -d ' ')
            read -r avg_ms p50_ms p95_ms p99_ms min_ms max_ms qps count <<< "$(compute_stats "$latfile")"
            echo -e "avg=${Y}${avg_ms}ms${NC}  p50=${p50_ms}ms  p95=${p95_ms}ms  p99=${p99_ms}ms  qps=${G}${qps}${NC}  (${count} total samples)"
        done
    done
    echo ""
done

# ── Phase 4: Generate report ─────────────────────────────────────────────────
echo -e "${P}Generating report...${NC}"

GO_VERSION=$(go version | awk '{print $3}')
OS_INFO=$(uname -srm)
RUN_DATE=$(date '+%Y-%m-%d %H:%M:%S')
TOTAL_QUERIES=$(( ${#QUERIES[@]} * ITERATIONS * RUNS ))

TABLE_ROWS=""
CHART_LABELS=""
CHART_LATENCY=""
CHART_QPS=""

for algo in "${ALGORITHMS[@]}"; do
    for fz in "${FUZZY_MODES[@]}"; do
        label="$algo"
        chart_label="$algo"
        [ "$fz" -gt 0 ] && label="${algo}+fuzzy" && chart_label="${algo}+f"
        latfile="$BENCH_TMP/${algo}_${fz}.lat"

        read -r avg_ms p50_ms p95_ms p99_ms min_ms max_ms qps count <<< "$(compute_stats "$latfile")"

        TABLE_ROWS="${TABLE_ROWS}| ${label} | ${avg_ms} | ${p50_ms} | ${p95_ms} | ${p99_ms} | ${min_ms} | ${max_ms} | ${qps} |\n"

        [ -n "$CHART_LABELS" ] && CHART_LABELS="${CHART_LABELS}, "
        CHART_LABELS="${CHART_LABELS}\"${chart_label}\""

        [ -n "$CHART_LATENCY" ] && CHART_LATENCY="${CHART_LATENCY}, "
        CHART_LATENCY="${CHART_LATENCY}${avg_ms}"

        [ -n "$CHART_QPS" ] && CHART_QPS="${CHART_QPS}, "
        CHART_QPS="${CHART_QPS}${qps}"
    done
done

# Build per-query results table
QUERY_TABLE=""
for qi in $(seq 0 $(( ${#QUERIES[@]} - 1 ))); do
    q="${QUERIES[$qi]}"
    row="| ${q} |"
    for algo in "${ALGORITHMS[@]}"; do
        rcfile="$BENCH_TMP/${algo}_0_q${qi}.rc"
        rc="0"
        [ -f "$rcfile" ] && rc=$(cat "$rcfile")
        row="${row} ${rc} |"
    done
    QUERY_TABLE="${QUERY_TABLE}${row}\n"
done

# Write report
cat > "$REPORT_FILE" << ENDOFMD
# FTS Algorithm Benchmark

> Auto-generated by \`test/search-benchmark.sh\` on ${RUN_DATE}

## Environment

| Parameter | Value |
|-----------|-------|
| OS | ${OS_INFO} |
| Go | ${GO_VERSION} |
| Documents | ${DOCS} |
| Queries | ${#QUERIES[@]} diverse queries |
| Iterations | ${ITERATIONS} per query per algorithm |
| Runs | ${RUNS} (benchmark repeated ${RUNS}x, results aggregated) |
| Total searches | ${TOTAL_QUERIES} per algorithm config |
| Warmup | ${WARMUP} queries per run (discarded) |

## Algorithms

| Algorithm | Description |
|-----------|-------------|
| **tfidf** | Classic TF-IDF term frequency scoring |
| **bm25** | Okapi BM25 probabilistic ranking with length normalization |
| **bm25f** | BM25F field-weighted scoring (title, meta, content) |
| **pmisparse** | BM25 + PMI query expansion (invented by Tradik Limited) |
| **+fuzzy** | Levenshtein distance 1 fuzzy matching variant |

## Latency Results

| Algorithm | Avg (ms) | P50 (ms) | P95 (ms) | P99 (ms) | Min (ms) | Max (ms) | QPS |
|-----------|----------|----------|----------|----------|----------|----------|-----|
$(echo -e "$TABLE_ROWS")

## Average Latency Comparison

\`\`\`mermaid
xychart-beta
    title "Average Search Latency (ms) — lower is better"
    x-axis [${CHART_LABELS}]
    y-axis "Latency (ms)"
    bar [${CHART_LATENCY}]
\`\`\`

## Throughput Comparison

\`\`\`mermaid
xychart-beta
    title "Search Throughput (queries/sec) — higher is better"
    x-axis [${CHART_LABELS}]
    y-axis "QPS"
    bar [${CHART_QPS}]
\`\`\`

## Result Counts per Query

Shows how many documents each algorithm returns (limit=10) to verify they all find relevant results.

| Query | tfidf | bm25 | bm25f | pmisparse |
|-------|-------|------|-------|-----------|
$(echo -e "$QUERY_TABLE")

## Notes

- **tfidf**: Fastest for simple keyword matching. No length normalization.
- **bm25**: Slightly more compute than tfidf due to document length normalization. Best general-purpose algorithm.
- **bm25f**: Adds field-level weighting. Slower due to separate field index lookups.
- **pmisparse**: First search triggers lazy PMI matrix training (not included in benchmark). Subsequent searches include PMI expansion overhead.
- **fuzzy**: Adds Levenshtein distance computation. Expected ~2-3x slower than exact matching.
- All benchmarks run on a warm server with FTS indices already built during document insertion.
ENDOFMD

echo -e "${G}  Report written to: docs/BENCHMARK.md${NC}"

# Save raw results
{
    echo "MDDB FTS Search Benchmark Raw Results"
    echo "======================================"
    echo "Date: $RUN_DATE"
    echo "OS: $OS_INFO"
    echo "Go: $GO_VERSION"
    echo "Documents: $DOCS"
    echo "Iterations per query: $ITERATIONS"
    echo "Queries: ${#QUERIES[@]}"
    echo ""
    echo -e "$TABLE_ROWS"
} > "$RAW_FILE"
echo -e "${G}  Raw results: test/search-benchmark-results-$TIMESTAMP.txt${NC}"

echo ""
echo -e "${B}════════════════════════════════════════════════════════════${NC}"
echo -e "${G}  Benchmark complete!${NC}"
echo -e "${B}════════════════════════════════════════════════════════════${NC}"
