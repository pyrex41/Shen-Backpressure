#!/bin/bash
set -euo pipefail

# shenguard-audit.sh — Gate 5: Verify shenguard package integrity.
#
# Thin wrapper that delegates to the repo-root `bin/shenguard-audit.sh`
# script. Centralising the audit logic at the repo root lets every example
# benefit from language-aware audits without duplicating the per-language
# allowlists and emitter-discovery code (added in W2.2).
#
# Usage: ./bin/shenguard-audit.sh [spec-path] [package-name] [output-path]
# This script defaults to `--lang go`.

ROOT_SCRIPT="$(cd "$(dirname "$0")/../../.." && pwd)/bin/shenguard-audit.sh"

if [ ! -x "$ROOT_SCRIPT" ]; then
    echo "FAIL: repo-root audit script not found at $ROOT_SCRIPT"
    exit 1
fi

exec "$ROOT_SCRIPT" --lang go "$@"
