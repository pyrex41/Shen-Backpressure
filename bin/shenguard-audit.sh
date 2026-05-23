#!/bin/bash
set -euo pipefail

# shenguard-audit.sh — Gate 5: Verify shenguard package integrity.
#
# Re-runs the appropriate language emitter and diffs the output against the
# committed `guards_gen.<ext>` file. Catches manual edits to the forgery
# boundary and stale generated code.
#
# Usage:
#   ./bin/shenguard-audit.sh [--lang go|ts|py|rs] [spec-path] [package-name] [output-path]
#
# Defaults: --lang go, spec specs/core.shen, package shenguard,
#           output internal/shenguard/guards_gen.go (per language convention).
#
# Per language:
#   go : invokes the compiled `shengen` binary (building from source if needed)
#   ts : invokes `npx tsx <repo>/cmd/shengen-ts/shengen.ts`
#   py : invokes `python3 <repo>/cmd/shengen-py/shengen.py` (experimental)
#   rs : invokes `python3 <repo>/cmd/shengen-rs/shengen.py` (experimental)
#
# Allowlist of expected files in the SHENGUARD_DIR is language-specific.

LANG_FLAG="go"
SKIP_ISOLATION=0
POSITIONAL=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --lang)
            LANG_FLAG="${2:-go}"
            shift 2
            ;;
        --lang=*)
            LANG_FLAG="${1#--lang=}"
            shift
            ;;
        --no-isolation-check)
            # Skip the "unexpected files in shenguard package" check.
            # Use when the generated file lives alongside hand-written code
            # in a shared directory (e.g. shen-web-tools' runtime/ dir).
            SKIP_ISOLATION=1
            shift
            ;;
        -h|--help)
            sed -n '4,18p' "$0"
            exit 0
            ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done

# Determine sensible defaults per language for the output path / allowlist.
case "$LANG_FLAG" in
    go)
        DEFAULT_OUT="internal/shenguard/guards_gen.go"
        GEN_EXT="go"
        ALLOWED_FILES=("guards_gen.go" "db_scoped_gen.go")
        ;;
    ts)
        DEFAULT_OUT="runtime/guards_gen.ts"
        GEN_EXT="ts"
        ALLOWED_FILES=("guards_gen.ts")
        ;;
    py)
        DEFAULT_OUT="shenguard/guards_gen.py"
        GEN_EXT="py"
        ALLOWED_FILES=("guards_gen.py" "__init__.py")
        ;;
    rs)
        DEFAULT_OUT="src/shenguard/guards_gen.rs"
        GEN_EXT="rs"
        ALLOWED_FILES=("guards_gen.rs" "mod.rs")
        ;;
    *)
        echo "FAIL: unsupported --lang value '$LANG_FLAG' (expected go|ts|py|rs)"
        exit 1
        ;;
esac

SPEC="${POSITIONAL[0]:-specs/core.shen}"
PKG="${POSITIONAL[1]:-shenguard}"
OUT="${POSITIONAL[2]:-$DEFAULT_OUT}"
SHENGUARD_DIR="$(dirname "$OUT")"

echo "Gate 5: TCB Audit — verifying shenguard package integrity (lang=$LANG_FLAG)"

# --- Step 1: Locate the appropriate emitter for the language ---
# Resolve repo root by walking up from this script to find the directory
# containing cmd/shengen.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR"
while [ "$REPO_ROOT" != "/" ] && [ ! -d "$REPO_ROOT/cmd/shengen" ]; do
    REPO_ROOT="$(dirname "$REPO_ROOT")"
done

if [ ! -d "$REPO_ROOT/cmd/shengen" ]; then
    # Fall back to the conventional layout (example invoking from its own dir)
    if [ -d "cmd/shengen" ]; then
        REPO_ROOT="$(pwd)"
    elif [ -d "../../cmd/shengen" ]; then
        REPO_ROOT="$(cd ../.. && pwd)"
    else
        echo "FAIL: could not locate repo root (no cmd/shengen found)"
        exit 1
    fi
fi

run_go_emitter() {
    local out_path="$1"
    local shengen=""
    for candidate in bin/shengen "$SCRIPT_DIR/shengen" "$REPO_ROOT/bin/shengen"; do
        if [ -f "$candidate" ]; then
            shengen="$candidate"
            break
        fi
    done
    if [ -z "$shengen" ]; then
        if [ -f "$REPO_ROOT/cmd/shengen/main.go" ]; then
            mkdir -p bin
            (cd "$REPO_ROOT/cmd/shengen" && go build -o "$REPO_ROOT/bin/shengen" .) 2>/dev/null
            shengen="$REPO_ROOT/bin/shengen"
        fi
    fi
    if [ -z "$shengen" ] || [ ! -f "$shengen" ]; then
        echo "FAIL: shengen binary not found and could not build from $REPO_ROOT/cmd/shengen/main.go"
        return 1
    fi
    "$shengen" "$SPEC" "$PKG" > "$out_path" 2>/dev/null
}

run_ts_emitter() {
    local out_path="$1"
    local src="$REPO_ROOT/cmd/shengen-ts/shengen.ts"
    if [ ! -f "$src" ]; then
        echo "FAIL: shengen-ts source not found at $src"
        return 1
    fi
    npx tsx "$src" "$SPEC" --out "$out_path" >/dev/null 2>&1
}

run_py_emitter() {
    local out_path="$1"
    local src="$REPO_ROOT/cmd/shengen-py/shengen.py"
    if [ ! -f "$src" ]; then
        echo "FAIL: shengen-py source not found at $src"
        return 1
    fi
    python3 "$src" "$SPEC" --out "$out_path" >/dev/null 2>&1
}

run_rs_emitter() {
    local out_path="$1"
    local src="$REPO_ROOT/cmd/shengen-rs/shengen.py"
    if [ ! -f "$src" ]; then
        echo "FAIL: shengen-rs source not found at $src"
        return 1
    fi
    python3 "$src" "$SPEC" --out "$out_path" >/dev/null 2>&1
}

if [ ! -f "$SPEC" ]; then
    echo "FAIL: spec file not found at $SPEC"
    exit 1
fi

if [ ! -f "$OUT" ]; then
    echo "FAIL: generated file not found at $OUT"
    exit 1
fi

# --- Step 2: Check for unexpected files in shenguard package ---
# Walk every file with the language extension in SHENGUARD_DIR and reject any
# whose basename is not in ALLOWED_FILES. Skipped when --no-isolation-check
# is set (used when the generated file lives in a shared directory).
if [ "$SKIP_ISOLATION" -eq 0 ]; then
    UNEXPECTED=""
    for f in "$SHENGUARD_DIR"/*.${GEN_EXT}; do
        [ -f "$f" ] || continue
        base="$(basename "$f")"
        allowed=0
        for allow in "${ALLOWED_FILES[@]}"; do
            if [ "$base" = "$allow" ]; then
                allowed=1
                break
            fi
        done
        if [ "$allowed" -eq 0 ]; then
            UNEXPECTED="$UNEXPECTED $base"
        fi
    done

    if [ -n "$UNEXPECTED" ]; then
        echo "FAIL: unexpected files in shenguard package:$UNEXPECTED"
        echo "The shenguard package must contain ONLY generated code."
        echo "Move hand-written code to a separate package."
        echo "Allowed: ${ALLOWED_FILES[*]}"
        exit 1
    fi
fi

# --- Step 3: Regenerate and diff ---
TEMP_OUT=$(mktemp)
trap 'rm -f "$TEMP_OUT"' EXIT

case "$LANG_FLAG" in
    go) run_go_emitter "$TEMP_OUT" || exit 1 ;;
    ts) run_ts_emitter "$TEMP_OUT" || exit 1 ;;
    py) run_py_emitter "$TEMP_OUT" || exit 1 ;;
    rs) run_rs_emitter "$TEMP_OUT" || exit 1 ;;
esac

if ! diff -q "$OUT" "$TEMP_OUT" > /dev/null 2>&1; then
    echo "FAIL: $OUT does not match $LANG_FLAG emitter output"
    echo ""
    echo "Either the spec changed without regenerating, or the file was manually edited."
    echo "Diff:"
    diff -u "$OUT" "$TEMP_OUT" | head -40 || true
    echo ""
    case "$LANG_FLAG" in
        go) echo "Fix: re-run shengen and commit the output to $OUT" ;;
        ts) echo "Fix: re-run shengen-ts and commit the output to $OUT" ;;
        py) echo "Fix: python3 cmd/shengen-py/shengen.py $SPEC --out $OUT" ;;
        rs) echo "Fix: python3 cmd/shengen-rs/shengen.py $SPEC --out $OUT" ;;
    esac
    exit 1
fi

echo "PASS: shenguard package contains only generated code, output matches $LANG_FLAG emitter"
