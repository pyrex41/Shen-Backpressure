#!/bin/bash
# Regenerate rs/guards_gen.rs from specs/core.shen using the Rs shengen.
set -euo pipefail

cd "$(dirname "$0")/.."

SPEC=specs/core.shen
OUT=rs/guards_gen.rs

if [ ! -f "../../cmd/shengen-rs/shengen.py" ]; then
    echo "ERROR: shengen-rs source not found at ../../cmd/shengen-rs/shengen.py"
    exit 1
fi

mkdir -p "$(dirname "$OUT")"
python3 ../../cmd/shengen-rs/shengen.py "$SPEC" --out "$OUT"
echo "Generated $OUT (Rs) from $SPEC"
