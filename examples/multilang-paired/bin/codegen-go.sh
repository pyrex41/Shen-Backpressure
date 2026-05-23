#!/bin/bash
# Regenerate go/guards_gen.go from specs/core.shen using the Go shengen.
set -euo pipefail

cd "$(dirname "$0")/.."

SPEC=specs/core.shen
PKG=multilang_paired
OUT=go/multilang_paired/guards_gen.go

# Locate or build the shengen binary.
SHENGEN=""
for candidate in ../../bin/shengen ../payment/bin/shengen; do
    if [ -f "$candidate" ]; then
        SHENGEN="$candidate"
        break
    fi
done
if [ -z "$SHENGEN" ]; then
    if [ -f "../../cmd/shengen/main.go" ]; then
        mkdir -p ../../bin
        (cd ../../cmd/shengen && go build -o ../../bin/shengen .)
        SHENGEN="../../bin/shengen"
    fi
fi
if [ -z "$SHENGEN" ] || [ ! -f "$SHENGEN" ]; then
    echo "ERROR: shengen binary not found"
    exit 1
fi

mkdir -p "$(dirname "$OUT")"
"$SHENGEN" "$SPEC" "$PKG" > "$OUT" 2>/dev/null
echo "Generated $OUT (Go) from $SPEC"
