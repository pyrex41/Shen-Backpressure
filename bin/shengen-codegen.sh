#!/bin/bash
set -euo pipefail

# shengen-codegen.sh — Generate Go guard types from Shen specs.
# Usage: ./bin/shengen-codegen.sh [spec-path] [package-name] [output-path]
#
# Set SHENGEN_BRANDS=1 to emit GDP brand parameters and witness fields
# (proof binding, W1). Opt-in: unset, the output is the pre-brand shape.
# A project that opts in must pass the same flag to its TCB audit gate
# (`bin/shenguard-audit.sh --brands`), or the drift check will fail.

SPEC="${1:-specs/core.shen}"
PKG="${2:-shenguard}"
OUT="${3:-internal/shenguard/guards_gen.go}"

# Find shengen binary — check local bin/ first, then repo root
SHENGEN=""
if [ -f bin/shengen ]; then
    SHENGEN=bin/shengen
elif [ -f "$(dirname "$0")/shengen" ]; then
    SHENGEN="$(dirname "$0")/shengen"
fi

# Build from source if not found
if [ -z "$SHENGEN" ]; then
    SHENGEN_SRC="${SHENGEN_SRC:-cmd/shengen}"
    if [ -f "$SHENGEN_SRC/main.go" ]; then
        echo "Building shengen from $SHENGEN_SRC..."
        (cd "$SHENGEN_SRC" && go build -o "$(pwd)/../../bin/shengen" .)
        SHENGEN=bin/shengen
    else
        echo "ERROR: shengen binary not found and source not at $SHENGEN_SRC/main.go"
        exit 1
    fi
fi

if [ ! -f "$SPEC" ]; then
    echo "ERROR: spec file not found at $SPEC"
    exit 1
fi

mkdir -p "$(dirname "$OUT")"
BRAND_FLAGS=()
if [ -n "${SHENGEN_BRANDS:-}" ]; then
    BRAND_FLAGS=(--brands)
fi

"$SHENGEN" "${BRAND_FLAGS[@]}" "$SPEC" "$PKG" > "$OUT" 2>/dev/null
echo "Generated $OUT from $SPEC (package $PKG)"
