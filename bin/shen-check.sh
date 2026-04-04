#!/bin/bash
set -euo pipefail

# shen-check.sh — Gate 4: Verify spec internal consistency via Shen tc+.
# Usage: ./bin/shen-check.sh [spec-path]
#
# Backend selection (in priority order):
#   1. $SHEN env var          — explicit path to any Shen binary
#   2. shen-sbcl on PATH      — shen-cl (SBCL), fastest startup
#   3. shen-scheme on PATH    — shen-scheme (Chez), fastest compute
#   4. shen on PATH            — any Shen port
#
# All Shen ports share the same eval CLI: shen eval -e '...' -l file

SPEC="${1:-specs/core.shen}"

if [ ! -f "$SPEC" ]; then
    echo "ERROR: spec file not found at $SPEC"
    exit 1
fi

# Find a Shen backend
if [ -n "${SHEN:-}" ]; then
    : # explicit override
elif command -v shen-sbcl >/dev/null 2>&1; then
    SHEN=shen-sbcl
elif command -v shen-scheme >/dev/null 2>&1; then
    SHEN=shen-scheme
elif command -v shen >/dev/null 2>&1; then
    SHEN=shen
else
    echo "ERROR: no Shen runtime found. Install shen-sbcl or shen-scheme, or set \$SHEN."
    echo "  brew tap Shen-Language/homebrew-shen && brew install shen-sbcl"
    exit 1
fi

echo "Gate 4: Shen tc+ — checking $SPEC (backend: $SHEN)"
timeout 30 "$SHEN" eval -e "(tc +)" -l "$SPEC" 2>&1 || { echo "RESULT: FAIL"; exit 1; }
echo "RESULT: PASS"
