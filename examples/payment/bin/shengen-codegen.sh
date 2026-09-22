#!/bin/bash
set -euo pipefail

# shengen-codegen.sh — Generate Go guard types from Shen specs.
# Usage: ./bin/shengen-codegen.sh [spec-path] [package-name] [output-path]

SPEC="${1:-specs/core.shen}"
PKG="${2:-shenguard}"
OUT="${3:-internal/shenguard/guards_gen.go}"

# Find shengen binary — check local bin/, the script's dir, and the
# parent repo's bin/ (when the example is nested under a Shen-Backpressure
# checkout).
SHENGEN=""
for candidate in bin/shengen "$(dirname "$0")/shengen" ../../bin/shengen; do
    if [ -f "$candidate" ]; then
        SHENGEN="$candidate"
        break
    fi
done

# Build from source if not found
if [ -z "$SHENGEN" ]; then
    for src in "${SHENGEN_SRC:-cmd/shengen}" ../../cmd/shengen; do
        if [ -f "$src/main.go" ]; then
            echo "Building shengen from $src..."
            mkdir -p bin
            (cd "$src" && go build -trimpath -o "$OLDPWD/bin/shengen" .)
            SHENGEN=bin/shengen
            break
        fi
    done
fi

if [ -z "$SHENGEN" ]; then
    echo "ERROR: shengen binary not found and source not at cmd/shengen/main.go (or ../../cmd/shengen)"
    exit 1
fi

if [ ! -f "$SPEC" ]; then
    echo "ERROR: spec file not found at $SPEC"
    exit 1
fi

mkdir -p "$(dirname "$OUT")"
# This example opts into GDP brands (W1): generated proof types carry a
# phantom brand parameter binding each proof to the value it is about,
# plus an unexported witness field that makes an empty-literal forgery
# panic on first use. The TCB audit gate passes --brands to match.
"$SHENGEN" --brands "$SPEC" "$PKG" > "$OUT" 2>/dev/null
echo "Generated $OUT from $SPEC (package $PKG)"
