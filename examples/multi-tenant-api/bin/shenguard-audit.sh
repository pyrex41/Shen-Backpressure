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
# Usage: ./bin/shenguard-audit.sh [spec-path] [package-name] [output-path]

ROOT_SCRIPT="$(cd "$(dirname "$0")/../../.." && pwd)/bin/shenguard-audit.sh"

if [ ! -x "$ROOT_SCRIPT" ]; then
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
ALLOWED_CALLER="internal/verified/access.go"
SCAN_DIRS=""
[ -d internal ] && SCAN_DIRS="$SCAN_DIRS internal"
[ -d cmd ] && SCAN_DIRS="$SCAN_DIRS cmd"
BAD_CALLS=""
if [ -n "$SCAN_DIRS" ]; then
    while IFS= read -r f; do
        case "$f" in
            "$ALLOWED_CALLER") continue ;;
            "./$ALLOWED_CALLER") continue ;;
            */shenguard/*) continue ;;
            */bypass_attempts/*) continue ;;
            */bypass_harness/*) continue ;;
        esac
        BAD_CALLS="$BAD_CALLS $f"
    done < <(grep -rln -E 'shenguard\.New(TenantAccess|ResourceAccess)\b' $SCAN_DIRS 2>/dev/null || true)
fi
if [ -n "$BAD_CALLS" ]; then
    echo "FAIL: direct call(s) to shenguard.NewTenantAccess / NewResourceAccess outside $ALLOWED_CALLER:"
    for f in $BAD_CALLS; do
        echo "  $f"
    done
    echo ""
    echo "These constructors must only be called from $ALLOWED_CALLER (the"
    echo "Check* wrappers that consult the DB before constructing the proof)."
    echo "Direct calls bypass the DB membership / ownership check."
    exit 1
fi

# --- Step 2: Standard regen + drift audit (delegated to root script). ---
exec "$ROOT_SCRIPT" --lang go "$@"
