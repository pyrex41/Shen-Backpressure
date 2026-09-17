#!/usr/bin/env bash
set -euo pipefail
: "${SHENGEN_BIN:?set SHENGEN_BIN to the shengen binary}"
tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT
"$SHENGEN_BIN" -spec specs/core.shen -pkg marketguard -out "$tmp_file"
diff -u internal/marketguard/guards_gen.go "$tmp_file"
