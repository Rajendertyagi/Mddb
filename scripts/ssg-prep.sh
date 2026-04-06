#!/usr/bin/env bash
# ssg-prep.sh — prepares docs/ structure for SSG (generates metadata.json and pages/)
# Usage: bash scripts/ssg-prep.sh

set -euo pipefail

DOCS_DIR="docs"
PAGES_DIR="${DOCS_DIR}/pages"
META="${DOCS_DIR}/metadata.json"

mkdir -p "${PAGES_DIR}"

# Copy all markdown files from docs/ root into docs/pages/
find "${DOCS_DIR}" -maxdepth 1 -name "*.md" | while read -r file; do
  cp "$file" "${PAGES_DIR}/"
done

# Create minimal metadata.json if missing
if [ ! -f "${META}" ]; then
  echo '{"categories":[],"media":[],"users":[]}' > "${META}"
fi

echo "✅ SSG prep done: $(ls ${PAGES_DIR}/*.md 2>/dev/null | wc -l | tr -d ' ') pages ready"
