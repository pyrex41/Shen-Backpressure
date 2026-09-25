#!/usr/bin/env bash
# Gate 4: Shen tc+ on the spec (and the good corpus) via shen-erl.
#
# Runtime lookup: $SHEN, shen-erl on PATH, $SHEN_ERL_ROOT/bin/shen-erl,
# then a shen-erl checkout next to the repository.
set -euo pipefail
cd "$(dirname "$0")/.."

SPEC="${1:-specs/core.shen}"
PRELUDE="specs/verified.shen"

find_shen() {
  if [ -n "${SHEN:-}" ]; then echo "$SHEN"; return; fi
  if command -v shen-erl >/dev/null 2>&1; then command -v shen-erl; return; fi
  for c in "${SHEN_ERL_ROOT:-}/bin/shen-erl" ../../../shen-erl/bin/shen-erl ../shen-erl/bin/shen-erl; do
    if [ -x "$c" ]; then echo "$(cd "$(dirname "$c")" && pwd)/$(basename "$c")"; return; fi
  done
  echo "shen-check: no shen-erl found (set SHEN or SHEN_ERL_ROOT)" >&2
  exit 1
}
SHEN_BIN="$(find_shen)"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
{
  echo "(load \"$PWD/$PRELUDE\")"
  echo "(tc +)"
  echo "(load \"$PWD/$SPEC\")"
  for f in specs/good/*.shen; do
    [ -f "$f" ] && echo "(load \"$PWD/$f\")"
  done
  echo '(output "~%SB-SHEN-CHECK-PASS~%")'
} > "$work/check.shen"

out="$(cd "$work" && "$SHEN_BIN" script "$work/check.shen" 2>&1 || true)"
if grep -q "SB-SHEN-CHECK-PASS" <<<"$out"; then
  grep -E "typechecked in [0-9]+ inferences" <<<"$out" | tail -1 || true
  echo "RESULT: PASS"
else
  echo "$out" | grep -v '^ok : symbol' | tail -20
  echo "RESULT: FAIL"
  exit 1
fi
