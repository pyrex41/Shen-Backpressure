#!/bin/bash
set -euo pipefail

# shenguard-audit.sh — Gate 5: Verify shenguard package integrity.
#
# Re-runs shengen and diffs output against the committed guards_gen.go.
# Catches manual edits to the forgery boundary and stale generated code.
#
# Usage: ./bin/shenguard-audit.sh [spec-path] [package-name] [output-path]

SPEC="${1:-specs/core.shen}"
PKG="${2:-shenguard}"
OUT="${3:-internal/shenguard/guards_gen.go}"
SHENGUARD_DIR="$(dirname "$OUT")"

echo "Gate 5: TCB Audit — verifying shenguard package integrity"

# --- Step 1: Find or build shengen ---
SHENGEN=""
for candidate in bin/shengen "$(dirname "$0")/shengen" ../../bin/shengen; do
    if [ -f "$candidate" ]; then
        SHENGEN="$candidate"
        break
    fi
done

if [ -z "$SHENGEN" ]; then
    for src in "${SHENGEN_SRC:-cmd/shengen}" ../../cmd/shengen; do
        if [ -f "$src/main.go" ]; then
            mkdir -p bin
            (cd "$src" && go build -o "$OLDPWD/bin/shengen" .) 2>/dev/null
            SHENGEN=bin/shengen
            break
        fi
    done
fi

if [ -z "$SHENGEN" ]; then
    echo "FAIL: shengen binary not found and source not at cmd/shengen/main.go (or ../../cmd/shengen)"
    exit 1
fi

if [ ! -f "$SPEC" ]; then
    echo "FAIL: spec file not found at $SPEC"
    exit 1
fi

if [ ! -f "$OUT" ]; then
    echo "FAIL: generated file not found at $OUT"
    exit 1
fi

# --- Step 2: Check for unexpected files in shenguard package ---
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
    echo "Move hand-written code to a separate package."
    exit 1
fi

# --- Step 3: Regenerate and diff ---
TEMP_OUT=$(mktemp)
trap 'rm -f "$TEMP_OUT"' EXIT

"$SHENGEN" "$SPEC" "$PKG" > "$TEMP_OUT" 2>/dev/null

if ! diff -q "$OUT" "$TEMP_OUT" > /dev/null 2>&1; then
    echo "FAIL: $OUT does not match shengen output"
    echo ""
    echo "Either the spec changed without regenerating, or the file was manually edited."
    echo "Diff:"
    diff -u "$OUT" "$TEMP_OUT" | head -40 || true
    echo ""
    echo "Fix: run ./bin/shengen-codegen.sh $SPEC $PKG $OUT"
    exit 1
fi

echo "PASS: shenguard package contains only generated code, output matches shengen"
