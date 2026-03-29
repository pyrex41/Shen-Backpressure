#!/bin/bash
set -euo pipefail
SPEC="${1:-specs/core.shen}"
[ -f "$SPEC" ] || { echo "ERROR: $SPEC not found"; exit 1; }
timeout 30 shen-sbcl -q -e "(tc +)" -l "$SPEC" 2>&1 || { echo "RESULT: FAIL"; exit 1; }
echo "RESULT: PASS"
