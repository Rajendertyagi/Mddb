#!/usr/bin/env bash
#
# test-go-version.sh — tests for scripts/check-go-version.sh (DOC-003 guard).
#
# Covers both branches of the guard:
#   1. consistent pins        -> exit 0
#   2. a single drifted pin   -> exit 1 (the go.work-lagging-behind case)
#   3. no pins at all         -> exit 2
#
# The guard derives the repo root from its own location, so each case builds
# a throw-away tree, drops a copy of the real script into <tree>/scripts/,
# and runs it there in isolation.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
GUARD="${REPO_ROOT}/scripts/check-go-version.sh"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
PASS=0; FAIL=0

# scaffold <dir> <toolchain-version> — create a minimal repo tree with the
# guard installed and one go.work + one go.mod pinned to the given version.
scaffold() {
	local dir="$1" ver="$2"
	mkdir -p "${dir}/scripts" "${dir}/svc"
	cp "${GUARD}" "${dir}/scripts/check-go-version.sh"
	printf 'go 1.26\n\ntoolchain go%s\n\nuse (\n\t./svc\n)\n' "${ver}" > "${dir}/go.work"
	printf 'module svc\n\ngo 1.26\n\ntoolchain go%s\n' "${ver}" > "${dir}/svc/go.mod"
}

expect_exit() {
	local name="$1" want="$2"; shift 2
	local got=0
	"$@" >/dev/null 2>&1 || got=$?
	if [[ "${got}" -eq "${want}" ]]; then
		echo -e "${GREEN}✓${NC} ${name} (exit ${got})"; PASS=$((PASS + 1))
	else
		echo -e "${RED}✗${NC} ${name}: expected exit ${want}, got ${got}"; FAIL=$((FAIL + 1))
	fi
}

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

# Case 0: the REAL repo must be consistent (post-DOC-003 fix).
expect_exit "real repo is consistent" 0 bash "${GUARD}"

# Case 1: consistent synthetic tree -> 0
scaffold "${TMP}/ok" "1.26.3"
expect_exit "synthetic consistent tree" 0 bash "${TMP}/ok/scripts/check-go-version.sh"

# Case 2: drift go.work behind go.mod (the exact DOC-003 regression) -> 1
scaffold "${TMP}/drift" "1.26.3"
printf 'go 1.26\n\ntoolchain go1.26.2\n\nuse (\n\t./svc\n)\n' > "${TMP}/drift/go.work"
expect_exit "drifted go.work is rejected" 1 bash "${TMP}/drift/scripts/check-go-version.sh"

# Case 3: empty tree (no pins) -> 2
mkdir -p "${TMP}/empty/scripts"
cp "${GUARD}" "${TMP}/empty/scripts/check-go-version.sh"
expect_exit "empty tree reports nothing-found" 2 bash "${TMP}/empty/scripts/check-go-version.sh"

echo "---"
echo "Passed: ${PASS}, Failed: ${FAIL}"
[[ "${FAIL}" -eq 0 ]]
