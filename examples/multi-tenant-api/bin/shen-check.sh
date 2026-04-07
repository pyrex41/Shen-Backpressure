#!/bin/bash
set -euo pipefail

# shen-check.sh — Verify Shen specs pass type checking.
# Usage: ./bin/shen-check.sh [spec-path]
#
# Uses shen-sbcl (Shen on SBCL) with --eval and --load flags.
# Runs (tc +) then loads the spec file. Exits 0 on pass, 1 on fail.

SPEC="${1:-specs/core.shen}"

if [ ! -f "$SPEC" ]; then
    echo "ERROR: spec file not found at $SPEC"
    exit 1
fi

# Find shen-sbcl — check PATH first, then common locations
SHEN=""
if command -v shen-sbcl &>/dev/null; then
    SHEN=shen-sbcl
elif [ -f /opt/homebrew/bin/shen-sbcl ]; then
    SHEN=/opt/homebrew/bin/shen-sbcl
fi

if [ -z "$SHEN" ]; then
    echo "ERROR: shen-sbcl not found. Install with: brew tap Shen-Language/homebrew-shen && brew install shen-sbcl"
    exit 1
fi

# Run shen with type checking enabled, loading the spec file.
# --eval and --load prevent the REPL from starting (clean exit).
OUTPUT=$("$SHEN" -q -e "(tc +)" -l "$SPEC" 2>&1) || {
    echo "RESULT: FAIL"
    echo "$OUTPUT"
    exit 1
}

# Check for type errors in output
if echo "$OUTPUT" | grep -qi "type error\|error.*type\|typecheck.*fail"; then
    echo "RESULT: FAIL"
    echo "$OUTPUT"
    exit 1
else
    echo "RESULT: PASS"
    echo "Shen type check passed for $SPEC"
    exit 0
fi
