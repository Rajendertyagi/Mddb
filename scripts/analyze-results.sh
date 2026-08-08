#!/bin/bash
# Windows Audit Result Analyzer
# Processes test outputs and generates summary metrics

set -euo pipefail

ARTIFACTS_DIR="${1:-artifacts}"
OUTPUT_FILE="${2:-audit-summary.json}"

echo "=== Windows Audit Analyzer ==="
echo "Processing artifacts from: $ARTIFACTS_DIR"

# Initialize counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
WARNINGS=0

# Create output directory
mkdir -p "$(dirname "$OUTPUT_FILE")"

# Analyze HTTP test results
if [ -f "$ARTIFACTS_DIR/audit-test-results/http-test-results.log" ]; then
    echo "Analyzing HTTP test results..."
    HTTP_PASSED=$(grep -c "^PASS:" "$ARTIFACTS_DIR/audit-test-results/http-test-results.log" 2>/dev/null || echo 0)
    HTTP_FAILED=$(grep -c "^FAIL:" "$ARTIFACTS_DIR/audit-test-results/http-test-results.log" 2>/dev/null || echo 0)
    PASSED_TESTS=$((PASSED_TESTS + HTTP_PASSED))
    FAILED_TESTS=$((FAILED_TESTS + HTTP_FAILED))
    TOTAL_TESTS=$((TOTAL_TESTS + HTTP_PASSED + HTTP_FAILED))
    echo "  HTTP tests: $HTTP_PASSED passed, $HTTP_FAILED failed"
fi

# Analyze unit test results
for log in "$ARTIFACTS_DIR"/unit-test-results/*.log; do
    if [ -f "$log" ]; then
        echo "Analyzing $(basename "$log")..."
        UNIT_PASSED=$(grep -c "PASS:" "$log" 2>/dev/null || echo 0)
        UNIT_FAILED=$(grep -c "FAIL:" "$log" 2>/dev/null || echo 0)
        PASSED_TESTS=$((PASSED_TESTS + UNIT_PASSED))
        FAILED_TESTS=$((FAILED_TESTS + UNIT_FAILED))
        TOTAL_TESTS=$((TOTAL_TESTS + UNIT_PASSED + UNIT_FAILED))
        echo "  $(basename "$log"): $UNIT_PASSED passed, $UNIT_FAILED failed"
    fi
done

# Check security findings
if [ -f "$ARTIFACTS_DIR/security-audit/security-findings.md" ]; then
    echo "Analyzing security findings..."
    CRITICAL=$(grep -c "^\*\*CRITICAL\*\*" "$ARTIFACTS_DIR/security-audit/security-findings.md" 2>/dev/null || echo 0)
    HIGH=$(grep -c "^\*\*HIGH\*\*" "$ARTIFACTS_DIR/security-audit/security-findings.md" 2>/dev/null || echo 0)
    MEDIUM=$(grep -c "^\*\*MEDIUM\*\*" "$ARTIFACTS_DIR/security-audit/security-findings.md" 2>/dev/null || echo 0)
    LOW=$(grep -c "^\*\*LOW\*\*" "$ARTIFACTS_DIR/security-audit/security-findings.md" 2>/dev/null || echo 0)
    echo "  Security: $CRITICAL critical, $HIGH high, $MEDIUM medium, $LOW low"
fi

# Calculate scores
if [ $TOTAL_TESTS -gt 0 ]; then
    PASS_RATE=$((PASSED_TESTS * 100 / TOTAL_TESTS))
else
    PASS_RATE=0
fi

# Generate JSON summary
cat > "$OUTPUT_FILE" << EOF
{
  "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "summary": {
    "total_tests": $TOTAL_TESTS,
    "passed": $PASSED_TESTS,
    "failed": $FAILED_TESTS,
    "pass_rate": $PASS_RATE
  },
  "security": {
    "critical": ${CRITICAL:-0},
    "high": ${HIGH:-0},
    "medium": ${MEDIUM:-0},
    "low": ${LOW:-0}
  },
  "verdict": "$(if [ $FAILED_TESTS -eq 0 ] && [ ${CRITICAL:-0} -eq 0 ]; then echo "Ready for beta"; elif [ ${CRITICAL:-0} -eq 0 ]; then echo "Needs review"; else echo "Blocked"; fi)"
}
EOF

echo ""
echo "=== Analysis Complete ==="
echo "Total tests: $TOTAL_TESTS"
echo "Passed: $PASSED_TESTS ($PASS_RATE%)"
echo "Failed: $FAILED_TESTS"
echo "Output: $OUTPUT_FILE"
