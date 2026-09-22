#!/bin/bash
set -euo pipefail

# shen-check.sh — Gate 4: typecheck a spec in a Shen host (tc+).
#
# Prefer `sb shen-check`, which this script delegates to when an sb
# binary is available. sb owns the host resolution order and, crucially,
# loads the typed prelude that declares shen-derive's evaluator
# intrinsics (`val`, the field accessors, `scanl`, …). Without that
# prelude, tc+ rejects any spec whose `(define …)` bodies use them —
# which is why this gate had never passed on examples/payment.
#
# The bash path below is the fallback for a checkout with no built sb.
# It resolves a host the same way and uses the same flag form, but it
# has no prelude, so it checks the spec's datatypes and any define that
# happens to be plain Shen. It says so rather than claiming more.
#
# Host resolution (both paths):
#   1. $SHEN                — explicit path to any Shen port's launcher
#   2. [shen] bin           — from sb.toml (sb path only)
#   3. shen-sbcl on PATH    — shen-cl (SBCL), fastest startup
#   4. shen-scheme on PATH  — shen-scheme (Chez), fastest compute
#   5. shen on PATH         — any other port; `make shen-go` puts the
#                             Go port at bin/shen
#
# The CLI every port shares. Note that -q is a flag of the `eval`
# subcommand, not a top-level flag: `shen -q -e …` is rejected, and
# writing it that way is the other half of why this gate never ran.
#
#   shen eval -q -e '(tc +)' -l FILE ...

SPEC="${1:-specs/core.shen}"

if [ ! -f "$SPEC" ]; then
    echo "ERROR: spec file not found at $SPEC"
    exit 1
fi

install_hint() {
    cat <<'HINT'
ERROR: no Shen host found. Either:
    make shen-go                # builds the Go port into bin/shen (needs Go 1.27; GOTOOLCHAIN=auto fetches it)
    export SHEN=/path/to/shen   # any Shen port's launcher
    brew tap Shen-Language/homebrew-shen && brew install shen-sbcl
  or set [shen] bin = "..." in sb.toml.
HINT
}

# Preferred path: let sb do it, so the prelude is loaded.
for sb in "${SB:-}" ./bin/sb ../../bin/sb; do
    [ -n "$sb" ] && [ -x "$sb" ] || continue
    exec "$sb" shen-check --spec "$SPEC"
done

echo "WARNING: no sb binary found (build one with 'make build-sb')."
echo "         Falling back to a bare tc+ with no intrinsic prelude:"
echo "         a spec whose (define ...) bodies use shen-derive's"
echo "         evaluator intrinsics will fail here for that reason alone."

if [ -n "${SHEN:-}" ]; then
    : # explicit override
elif command -v shen-sbcl >/dev/null 2>&1; then
    SHEN=shen-sbcl
elif command -v shen-scheme >/dev/null 2>&1; then
    SHEN=shen-scheme
elif command -v shen >/dev/null 2>&1; then
    SHEN=shen
elif [ -x ./bin/shen ]; then
    SHEN=./bin/shen
elif [ -x ../../bin/shen ]; then
    SHEN=../../bin/shen
else
    install_hint
    exit 1
fi

echo "Gate 4: Shen tc+ — checking $SPEC (host: $SHEN, no prelude)"
OUTPUT=$(timeout 60 "$SHEN" eval -q -e "(tc +)" -l "$SPEC" 2>&1) || {
    echo "$OUTPUT"
    echo "RESULT: FAIL"
    exit 1
}
if echo "$OUTPUT" | grep -qi "type error\|maximum inferences exceeded\|^ERROR:"; then
    echo "$OUTPUT"
    echo "RESULT: FAIL"
    exit 1
fi
echo "RESULT: PASS"
