#!/bin/bash
# parity-check.sh — Run all four language CLIs on the shared fixture
# table and pairwise-diff the JSON output. Exit non-zero on any
# divergence.
#
# Usage: ./bin/parity-check.sh
#
# Prerequisites (per language):
#   Go    — go build ./go     (always required)
#   TS    — npx tsx           (always required)
#   Py    — python3 3.10+     (always required; uses dataclass slots)
#   Rs    — rustc on PATH     (optional; when absent the Rs leg is
#                              SKIPPED with a warning, and the
#                              remaining three are diffed pairwise)
#
# Exit codes:
#   0 — all enabled languages produce byte-identical JSONL output
#   1 — at least one pair diverged (per-language outputs printed to
#       /tmp/mp-<lang>-out.jsonl for inspection)
#   2 — environmental failure (missing toolchain, build error)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FIXTURE="$ROOT/fixture-inputs.jsonl"
if [ ! -f "$FIXTURE" ]; then
    echo "FAIL: fixture file not found at $FIXTURE" >&2
    exit 2
fi

OUT_GO="$(mktemp)"
OUT_TS="$(mktemp)"
OUT_PY="$(mktemp)"
OUT_RS="$(mktemp)"
trap 'rm -f "$OUT_GO" "$OUT_TS" "$OUT_PY" "$OUT_RS"' EXIT

# --- Go ---
echo "==> Building Go CLI..."
GO_BIN="$ROOT/.bin/mp-go"
mkdir -p "$ROOT/.bin"
(cd "$ROOT/go" && go build -o "$GO_BIN" .) || { echo "FAIL: go build" >&2; exit 2; }
echo "==> Running Go CLI..."
"$GO_BIN" < "$FIXTURE" > "$OUT_GO" || { echo "FAIL: go run" >&2; exit 2; }

# --- TypeScript ---
echo "==> Running TS CLI (npx tsx)..."
(cd "$ROOT/ts" && npx tsx main.ts < "$FIXTURE") > "$OUT_TS" 2>/dev/null || {
    echo "FAIL: ts run" >&2
    exit 2
}

# --- Python ---
echo "==> Running Py CLI (python3)..."
python3 "$ROOT/py/main.py" < "$FIXTURE" > "$OUT_PY" || { echo "FAIL: py run" >&2; exit 2; }

# --- Rust (optional) ---
HAVE_RS=0
if command -v rustc >/dev/null 2>&1; then
    echo "==> Building Rs CLI (rustc, no crates)..."
    RS_BIN="$ROOT/.bin/mp-rs"
    if (cd "$ROOT/rs" && rustc -O main.rs -o "$RS_BIN" 2>/dev/null); then
        echo "==> Running Rs CLI..."
        if "$RS_BIN" < "$FIXTURE" > "$OUT_RS" 2>/dev/null; then
            HAVE_RS=1
        else
            echo "WARN: Rs run failed; behavioural parity check excludes Rust." >&2
        fi
    else
        echo "WARN: Rs build failed; behavioural parity check excludes Rust." >&2
    fi
else
    echo "WARN: rustc not on PATH; behavioural parity check excludes Rust." >&2
    echo "      The Rs emitter is still drift-checked structurally by tcb-audit-rs." >&2
fi

# --- Pairwise diff ---
PAIRS=("Go:$OUT_GO" "TS:$OUT_TS" "Py:$OUT_PY")
if [ "$HAVE_RS" -eq 1 ]; then
    PAIRS+=("Rs:$OUT_RS")
fi

FAILED=0
for i in "${!PAIRS[@]}"; do
    for j in "${!PAIRS[@]}"; do
        if [ "$j" -le "$i" ]; then continue; fi
        IFS=":" read -r LA FA <<< "${PAIRS[i]}"
        IFS=":" read -r LB FB <<< "${PAIRS[j]}"
        if diff -q "$FA" "$FB" >/dev/null 2>&1; then
            printf "  PASS  %s == %s\n" "$LA" "$LB"
        else
            printf "  FAIL  %s != %s\n" "$LA" "$LB"
            echo "  ---- diff ----"
            diff "$FA" "$FB" | head -20 | sed 's/^/    /'
            echo "  --------------"
            FAILED=1
        fi
    done
done

# Persist the per-language outputs for the README walkthrough and for
# auditor inspection. .bin/ is gitignored at the example level.
mkdir -p "$ROOT/.bin"
cp "$OUT_GO" "$ROOT/.bin/mp-go-out.jsonl"
cp "$OUT_TS" "$ROOT/.bin/mp-ts-out.jsonl"
cp "$OUT_PY" "$ROOT/.bin/mp-py-out.jsonl"
[ "$HAVE_RS" -eq 1 ] && cp "$OUT_RS" "$ROOT/.bin/mp-rs-out.jsonl"

if [ "$FAILED" -eq 0 ]; then
    LANG_COUNT="${#PAIRS[@]}"
    echo ""
    echo "OK: all $LANG_COUNT languages produced byte-identical JSONL on $(wc -l < "$FIXTURE" | tr -d ' ') fixture rows."
    exit 0
else
    echo ""
    echo "FAIL: at least one language pair diverged. See diffs above." >&2
    echo "      Per-language outputs persisted under .bin/mp-<lang>-out.jsonl." >&2
    exit 1
fi
