#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SHEN="$PROJECT_ROOT/bin/shen"
SPEC="$PROJECT_ROOT/specs/core.shen"

if [ ! -f "$SHEN" ]; then
  echo "WARN: shen-go binary not found at $SHEN — skipping typecheck"
  echo "Install: see README or run /sb:setup"
  exit 0
fi

if [ ! -f "$SPEC" ]; then
  echo "ERROR: spec file not found at $SPEC"
  exit 1
fi

# shen-go's REPL loops on EOF, so we feed the spec + (quit) and timeout
RESULT=$(timeout 30 "$SHEN" --eval "(load \"$SPEC\") (tc +) (quit)" 2>&1) || {
  EXIT_CODE=$?
  if [ $EXIT_CODE -eq 124 ]; then
    echo "WARN: shen-go timed out (30s) — may be looping on EOF"
    exit 1
  fi
  echo "shen-check FAILED:"
  echo "$RESULT"
  exit 1
}

# Check for type errors in output
if echo "$RESULT" | grep -qi "type error\|error:"; then
  echo "shen-check FAILED — type errors found:"
  echo "$RESULT"
  exit 1
fi

echo "shen-check: specs/core.shen passed typechecking"
