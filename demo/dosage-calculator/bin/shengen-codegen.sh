#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SHENGEN="$PROJECT_ROOT/bin/shengen"
SPEC="$PROJECT_ROOT/specs/core.shen"
OUT="$PROJECT_ROOT/internal/shenguard/guards_gen.go"

if [ ! -f "$SHENGEN" ]; then
  echo "ERROR: shengen binary not found at $SHENGEN"
  echo "Build it: cd /path/to/Shen-Backpressure/cmd/shengen && go build -o $SHENGEN ."
  exit 1
fi

if [ ! -f "$SPEC" ]; then
  echo "ERROR: spec file not found at $SPEC"
  echo "Create specs/core.shen first (try /sb:init)"
  exit 1
fi

mkdir -p "$(dirname "$OUT")"
"$SHENGEN" -spec "$SPEC" -out "$OUT" -pkg shenguard
echo "shengen: generated $OUT from $SPEC"
