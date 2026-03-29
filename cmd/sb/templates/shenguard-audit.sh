#!/bin/bash
set -euo pipefail

# shenguard-audit.sh — Gate 5: Verify shenguard package integrity.
# Usage: ./bin/shenguard-audit.sh [spec-path] [package-name] [output-path]

SPEC="${1:-specs/core.shen}"
PKG="${2:-shenguard}"
OUT="${3:-internal/shenguard/guards_gen.go}"
SHENGUARD_DIR="$(dirname "$OUT")"

echo "Gate 5: TCB Audit — verifying shenguard package integrity"

# Find or build shengen
SHENGEN=""
if [ -f bin/shengen ]; then
    SHENGEN=bin/shengen
elif [ -f "$(dirname "$0")/shengen" ]; then
    SHENGEN="$(dirname "$0")/shengen"
fi

if [ -z "$SHENGEN" ]; then
    SHENGEN_SRC="${SHENGEN_SRC:-cmd/shengen}"
    if [ -f "$SHENGEN_SRC/main.go" ]; then
        (cd "$SHENGEN_SRC" && go build -o "$(pwd)/../../bin/shengen" .) 2>/dev/null
        SHENGEN=bin/shengen
    else
        echo "FAIL: shengen binary not found and source not at $SHENGEN_SRC/main.go"
        exit 1
    fi
fi

[ -f "$SPEC" ] || { echo "FAIL: spec file not found at $SPEC"; exit 1; }
[ -f "$OUT" ] || { echo "FAIL: generated file not found at $OUT"; exit 1; }

# Check for unexpected files
UNEXPECTED=""
for f in "$SHENGUARD_DIR"/*.go; do
    [ -f "$f" ] || continue
    base="$(basename "$f")"
    if [ "$base" != "guards_gen.go" ] && [ "$base" != "db_scoped_gen.go" ]; then
        UNEXPECTED="$UNEXPECTED $base"
    fi
done

if [ -n "$UNEXPECTED" ]; then
    echo "FAIL: unexpected files in shenguard package:$UNEXPECTED"
    echo "The shenguard package must contain ONLY generated code."
    exit 1
fi

# Regenerate and diff
TEMP_OUT=$(mktemp)
trap 'rm -f "$TEMP_OUT"' EXIT
"$SHENGEN" "$SPEC" "$PKG" > "$TEMP_OUT" 2>/dev/null

if ! diff -q "$OUT" "$TEMP_OUT" > /dev/null 2>&1; then
    echo "FAIL: $OUT does not match shengen output"
    diff -u "$OUT" "$TEMP_OUT" | head -40 || true
    echo "Fix: run ./bin/shengen-codegen.sh $SPEC $PKG $OUT"
    exit 1
fi

echo "PASS: shenguard package contains only generated code, output matches shengen"
