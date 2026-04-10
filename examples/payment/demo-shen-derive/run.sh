#!/usr/bin/env bash
# run.sh — Side-by-side demo: shengen types vs shen-derive spec-equivalence.
#
# For each of three hand-picked bugs, rotate the buggy version into
# place, run `go build` (the shengen / compiler gate) and
# `make shen-derive-verify` (the shen-derive gate), and report which
# gate caught it. Restore the original after each run.
#
# Bugs are chosen so that shengen alone is happy — the Go compiler
# type-checks every variant — but shen-derive exposes the divergence
# against the Shen spec's behavior.

set -u

DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$DIR/.." && pwd)"
IMPL="$ROOT/internal/derived/processable.go"
ORIG_BACKUP="$DIR/.processable.orig"

cleanup() {
    if [[ -f "$ORIG_BACKUP" ]]; then
        cp "$ORIG_BACKUP" "$IMPL"
        rm -f "$ORIG_BACKUP"
        echo "restored original processable.go"
    fi
}
trap cleanup EXIT INT TERM

cp "$IMPL" "$ORIG_BACKUP"

declare -a BUGS=(
    "bug1_sign_flip:Sign flip (balance += instead of -=)"
    "bug2_b0_truncate:Silent truncation of initial balance through int64"
    "bug3_strict_zero:Strict-zero boundary (balance <= 0 instead of < 0)"
)

echo ""
echo "============================================================"
echo "Side-by-side: shengen vs shen-derive on hand-picked bugs"
echo "============================================================"
echo ""
echo "For each bug:"
echo "  1. rotate the buggy impl into place"
echo "  2. run 'go build ./...'   (shengen / compiler gate)"
echo "  3. run 'make shen-derive-verify'  (shen-derive gate)"
echo "  4. record which gate caught it"
echo ""

declare -a RESULTS=()
RESULTS+=("bug,shengen_build,shen_derive_verify")

for entry in "${BUGS[@]}"; do
    name="${entry%%:*}"
    desc="${entry##*:}"
    bak="$DIR/${name}.go.bak"

    if [[ ! -f "$bak" ]]; then
        echo "error: missing $bak"
        exit 1
    fi

    echo "------------------------------------------------------------"
    echo "BUG: $desc"
    echo "     $name.go.bak"
    echo "------------------------------------------------------------"

    cp "$bak" "$IMPL"

    echo ""
    echo "  [1/2] go build ./..."
    if (cd "$ROOT" && go build ./... 2>&1 | sed 's/^/        /'); then
        build_status="PASS"
        echo "        => go build PASS  (shengen can't tell the impl is wrong)"
    else
        build_status="FAIL"
        echo "        => go build FAIL"
    fi

    echo ""
    echo "  [2/2] make shen-derive-verify"
    verify_out="$(cd "$ROOT" && make shen-derive-verify 2>&1 || true)"
    echo "$verify_out" | tail -12 | sed 's/^/        /'
    if echo "$verify_out" | grep -qE "FAIL|DRIFT"; then
        derive_status="FAIL"
        # Count failing subtests
        fail_count=$(echo "$verify_out" | grep -c '^    --- FAIL' || true)
        echo ""
        echo "        => shen-derive FAIL  ($fail_count case(s) disagreed with the spec)"
    else
        derive_status="PASS"
        echo ""
        echo "        => shen-derive PASS  (this bug slipped through — improve the spec or the pool)"
    fi

    RESULTS+=("$name,$build_status,$derive_status")
    echo ""
done

cp "$ORIG_BACKUP" "$IMPL"
rm -f "$ORIG_BACKUP"
trap - EXIT INT TERM

echo "============================================================"
echo "Summary"
echo "============================================================"
printf "%-22s  %-14s  %-22s\n" "bug" "go build" "shen-derive verify"
printf "%-22s  %-14s  %-22s\n" "----------------------" "--------------" "----------------------"
for row in "${RESULTS[@]:1}"; do
    IFS=',' read -r name b s <<< "$row"
    printf "%-22s  %-14s  %-22s\n" "$name" "$b" "$s"
done
echo ""
echo "Expected: every bug passes 'go build' (shengen types are fine)"
echo "          and fails 'shen-derive verify' (spec disagrees)."
echo ""
echo "Why: shengen proves that every *value* (Amount, Transaction)"
echo "     satisfies its premises. It has no way to prove anything"
echo "     about a *function*. shen-derive evaluates the spec on a"
echo "     sample set and asserts pointwise equality with the impl."
echo ""
echo "See DEMO.md in this directory for the detailed walkthrough."
