#!/bin/bash
set -euo pipefail

# shenguard-audit.sh — Gate 5: Verify shenguard package integrity.
#
# Multi-tenant layers two checks on top of the standard shengen drift audit:
#
#   1. A grep gate ensuring `shenguard.New(TenantAccess|ResourceAccess)` is
#      only called from `internal/verified/access.go` — the TCB wrappers
#      that perform the DB membership/ownership check. This is the W2.1
#      "social half" of the trust model: shengen emits these constructors
#      exported, so we enforce package-discipline via grep.
#
#   2. The standard regen + diff drift audit, delegated to the repo-root
#      `bin/shenguard-audit.sh --lang go` (W2.2 centralised the audit
#      logic so every example benefits from language-aware audits without
#      duplicating per-language allowlists / emitter discovery).
#
# Usage: ./bin/shenguard-audit.sh [--grep-only] [spec-path] [package-name] [output-path]
#
# --grep-only runs step 1 and stops. That mode is what the `flow` gate
# in sb.toml names as its fallback: when no SCIP indexer is on PATH,
# `sb flow` cannot discharge the flow premises in specs/core.shen and
# runs this grep instead, recording the premises as unproven with
# basis "grep-fallback". See ../../docs/FLOW.md.

GREP_ONLY=0
if [ "${1:-}" = "--grep-only" ]; then
    GREP_ONLY=1
    shift
fi

ROOT_SCRIPT="$(cd "$(dirname "$0")/../../.." && pwd)/bin/shenguard-audit.sh"

if [ "$GREP_ONLY" -eq 0 ] && [ ! -x "$ROOT_SCRIPT" ]; then
    echo "FAIL: repo-root audit script not found at $ROOT_SCRIPT"
    exit 1
fi

# --- Step 1: Forbid direct calls to the dangerous raw constructors. ---
#
# `shenguard.NewTenantAccess` and `shenguard.NewResourceAccess` are the
# W2.1 "would be package-private if Go allowed it" constructors. The HN
# critique (singron / max_unbearable) was specifically that anyone who can
# call `NewTenantAccess(..., isMember=true)` bypasses the DB membership
# check. The ONLY file allowed to call them is `internal/verified/access.go`,
# which wraps them inside `CheckTenantAccess` / `CheckResourceAccess` (the
# TCB functions that actually consult the DB).
#
# This is the social half of the trust model. The structural half (the
# cross-field premise `(= User (head (head Jwt)))` on `authenticated-user`)
# is what makes the token↔user binding unforgeable. The grep gate is the
# second-line defence against handlers that try to construct the lower-tier
# guards directly.
# The allowlist is file-granular, because that is all a regex over
# source text can express. The (flow ...) premise in specs/core.shen
# states the same discipline per *function*: only
# internal/verified/CheckTenantAccess and the Cedar differential
# oracle cmd/cedar-verify/computeGuardAllow may reference the raw
# constructors. This grep has to exempt the whole of
# cmd/cedar-verify/main.go to say the weaker half of that, which is a
# concrete example of what the flow gate buys.
ALLOWED_CALLERS="internal/verified/access.go cmd/cedar-verify/main.go"
SCAN_DIRS=""
[ -d internal ] && SCAN_DIRS="$SCAN_DIRS internal"
[ -d cmd ] && SCAN_DIRS="$SCAN_DIRS cmd"
BAD_CALLS=""
if [ -n "$SCAN_DIRS" ]; then
    while IFS= read -r f; do
        case "$f" in
            */shenguard/*) continue ;;
            */bypass_attempts/*) continue ;;
            */bypass_harness/*) continue ;;
        esac
        allowed=0
        for a in $ALLOWED_CALLERS; do
            case "$f" in
                "$a"|"./$a") allowed=1 ;;
            esac
        done
        [ "$allowed" -eq 1 ] && continue
        BAD_CALLS="$BAD_CALLS $f"
    done < <(grep -rln -E 'shenguard\.New(TenantAccess|ResourceAccess)\b' $SCAN_DIRS 2>/dev/null || true)
fi
if [ -n "$BAD_CALLS" ]; then
    echo "FAIL: direct call(s) to the raw TenantAccess / ResourceAccess constructors outside $ALLOWED_CALLERS:"
    for f in $BAD_CALLS; do
        echo "  $f"
    done
    echo ""
    echo "These constructors must only be called from $ALLOWED_CALLERS (the"
    echo "Check* wrappers that consult the DB before constructing the proof)."
    echo "Direct calls bypass the DB membership / ownership check."
    exit 1
fi

if [ "$GREP_ONLY" -eq 1 ]; then
    echo "PASS: no direct calls to the raw constructors outside $ALLOWED_CALLERS"
    echo "NOTE: this is a regex over source text. It cannot see an aliased"
    echo "      import (bypass_attempts/08_aliased_import.go.bak). Install a"
    echo "      SCIP indexer and let the flow gate discharge the premise."
    exit 0
fi

# --- Step 2: Standard regen + drift audit (delegated to root script). ---
# --brands: this example opts into GDP brands, so the audit regenerates
# with --brands and additionally requires the witness field on every
# wrapper type in the committed guards file.
exec "$ROOT_SCRIPT" --lang go --brands "$@"
