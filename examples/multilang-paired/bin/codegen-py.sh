#!/bin/bash
# Regenerate py/guards_gen.py from specs/core.shen using the Py shengen.
set -euo pipefail

cd "$(dirname "$0")/.."

SPEC=specs/core.shen
OUT=py/guards_gen.py

if [ ! -f "../../cmd/shengen-py/shengen.py" ]; then
    echo "ERROR: shengen-py source not found at ../../cmd/shengen-py/shengen.py"
    exit 1
fi

mkdir -p "$(dirname "$OUT")"
python3 ../../cmd/shengen-py/shengen.py "$SPEC" --out "$OUT"
echo "Generated $OUT (Py) from $SPEC"
