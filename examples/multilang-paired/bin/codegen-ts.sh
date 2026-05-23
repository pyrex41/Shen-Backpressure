#!/bin/bash
# Regenerate ts/guards_gen.ts from specs/core.shen using the TS shengen.
set -euo pipefail

cd "$(dirname "$0")/.."

SPEC=specs/core.shen
OUT=ts/guards_gen.ts

if [ ! -f "../../cmd/shengen-ts/shengen.ts" ]; then
    echo "ERROR: shengen-ts source not found at ../../cmd/shengen-ts/shengen.ts"
    exit 1
fi

mkdir -p "$(dirname "$OUT")"
npx tsx ../../cmd/shengen-ts/shengen.ts "$SPEC" --out "$OUT" >/dev/null 2>&1
echo "Generated $OUT (TS) from $SPEC"
