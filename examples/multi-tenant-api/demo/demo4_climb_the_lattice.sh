#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

# ===========================================================================
# DEMO 4 — "Climbing the lattice until you fall off."
#
# Walks REAL premises from specs/core.shen up the expressiveness lattice:
#
#     Cedar (SMT) ⊂ Rego (terminating) ⊂ Decidable-Shen-fragment ⊂ full-TC pure-Shen
#
# showing where each tier's reach ends — and proving empirically that the
# flat policy tiers deliberately stop at AUTHORIZATION, leaving the one
# cross-field AUTHENTICATION premise — (= User (head (head Jwt))) — to the
# guard constructor instead of lowering it into Cedar / Rego.
#
# This demo is read-only / illustrative. It mutates nothing. We still arm a
# restore trap defensively so the tree is left exactly as found.
# ===========================================================================

# Defensive: the (read-only) decidable check should not touch these, but if
# anything ever did, snapshot+restore leaves the tree exactly as found.
arm_restore

SPEC="specs/core.shen"
CEDAR_POLICIES="policies/cedar/policies.cedar"
REGO_MODULE="policies/rego/authz.rego"

# ---------------------------------------------------------------------------
section "The lattice"
# ---------------------------------------------------------------------------
note "Four runtime policy tiers, ordered by expressiveness (⊂ = strictly weaker than):"
echo
printf '   %sCedar (SMT)%s  ⊂  %sRego (terminating)%s  ⊂  %sDecidable-Shen-fragment%s  ⊂  %sfull-TC pure-Shen%s\n' \
  "$_C_BOLD" "$_C_RESET" "$_C_BOLD" "$_C_RESET" "$_C_BOLD" "$_C_RESET" "$_C_BOLD" "$_C_RESET"
echo
step "What each tier guarantees:"
note "Cedar (SMT)                 — SMT-decidable; analyzable by a solver. Strongest guarantees, narrowest reach."
note "Rego (terminating)          — guaranteed-terminating foreign runtime; adds aggregation/joins/walk/graph Cedar can't express."
note "Decidable-Shen-fragment     — Shen-native but restricted (sequent calculus + Prolog gatekeeper); no general recursion, Horn bodies, total forms only — termination guaranteed."
note "full-TC pure-Shen           — full Turing-complete Shen; maximal expressiveness, NO termination guarantee. Top of the lattice."
echo
note "Rule of thumb: you only climb a tier when you NEED its extra expressiveness — and you pay for it in lost guarantees."
pause

# ---------------------------------------------------------------------------
section "Premise expressiveness table"
# ---------------------------------------------------------------------------
note "Real premises drawn from $SPEC, mapped to the lowest tier that can express them."
echo
printf '   %s%-34s %s%s\n' "$_C_BOLD" "premise (from core.shen)" "tier reach" "$_C_RESET"
printf '   %s%s%s\n' "$_C_DIM" "-------------------------------------------------------------------------------" "$_C_RESET"
printf '   %-34s %s\n' '(= IsMember true)'           'all tiers — boolean equality'
printf '   %-34s %s\n' '(> Exp 0)'                   'all tiers — numeric comparison'
printf '   %-34s %s\n' '(not (= Sig ""))'            'all tiers — inequality'
printf '   %-34s %s\n' '(element? Elem Coll)'        'Cedar contains / Rego `in` / decidable ✓'
printf '   %-34s %s\n' ''                            '   NOTE: inline set literals [A B C] not yet parsed; two-operand form only'
printf '   %-34s %s\n' '(= User (head (head Jwt)))'  '✗ flat fragment (uses head) — scope-to-authorization'
printf '   %-34s %s\n' ''                            '   discharged UPSTREAM by the guard constructor, not the policy layer'
printf '   %-34s %s\n' 'general recursion / unbounded' 'only full-TC pure-Shen (top of lattice)'
echo
step "Confirming these premises are REAL (grepped live from $SPEC):"
grep -n '(= IsMember true)' "$SPEC" || true
grep -n '(> Exp 0)' "$SPEC" || true
grep -n '(not (= Sig "' "$SPEC" || true
grep -n '(= User (head (head Jwt)))' "$SPEC" || true
note "(element? appears only in the spec's commentary on allowed total forms; no live element? premise in this spec.)"
pause

# ---------------------------------------------------------------------------
section "Empirical proof of the boundary"
# ---------------------------------------------------------------------------
note "Claim: the cross-field AUTHENTICATION premise (= User (head (head Jwt)))"
note "exists in the spec but is DELIBERATELY NOT lowered into Cedar/Rego."
note "PremiseLowerable() rejects it because (head (head Jwt)) is a nested call,"
note "not a flat atom — so the flat policy tiers skip it as an authentication concern."
echo

step "1/3  The SPEC HAS it — inside the authenticated-user datatype:"
grep -n '(= User (head (head Jwt)))' "$SPEC" | while IFS= read -r line; do
  good "spec premise present: $line"
done
echo

step "2/3  The POLICIES DON'T — no head/sub/user-equality condition was emitted."
note "Searching $CEDAR_POLICIES and $REGO_MODULE for any user/sub/head/jwt condition..."
if grep -in 'user\|sub\|head\|jwt' "$CEDAR_POLICIES" "$REGO_MODULE" >/dev/null 2>&1; then
  bad "UNEXPECTED: found a derived condition referencing the auth premise:"
  grep -in 'user\|sub\|head\|jwt' "$CEDAR_POLICIES" "$REGO_MODULE" || true
else
  good "no user/sub/head/jwt condition in either emitted policy file — the premise was skipped."
fi
echo
note "What the flat tiers DID lower (authorization-scope premises only):"
printf '   %sCedar (%s):%s\n' "$_C_DIM" "$CEDAR_POLICIES" "$_C_RESET"
grep -nE 'isMember|exp >|sig !=' "$CEDAR_POLICIES" | head -6 || true
printf '   %sRego (%s):%s\n' "$_C_DIM" "$REGO_MODULE" "$_C_RESET"
grep -nE 'isMember|exp >|sig !=' "$REGO_MODULE" | head -6 || true
echo

step "3/3  WHY — the scope-to-authorization boundary."
note "(head (head Jwt)) is IsCall() => PremiseLowerable() returns false => the"
note "evaluator and both emitters silently drop it. The guard constructor"
note "NewAuthenticatedUser enforces it instead, returning an error on mismatch."
note "So the policy layer stays at AUTHORIZATION; AUTHENTICATION is discharged"
note "upstream by the typed ctor. The tiers agree on the OMISSION, so no tier is"
note "secretly made more permissive than the guard."
good "spec has it / policies don't / and here's exactly why."
pause

# ---------------------------------------------------------------------------
section "Optional: surface the decidable-fragment certification"
# ---------------------------------------------------------------------------
note "sb policy --decidable runs the native decidable-Shen-fragment recognizer"
note "(check-only, read-only) on $SPEC. It proves the targets sit inside the"
note "termination-guaranteed fragment: no recursion, Horn bodies, total forms."
echo
# run_sb_policy_decidable — prefer the sb() helper (go run cmd/sb); if that
# fails to resolve (it lives in a separate module and `go run` cannot always
# build it here), fall back to the prebuilt binary at cmd/sb/sb. Either way
# this is a CHECK-ONLY, read-only run (no --regen): it does not modify the tree.
run_sb_policy_decidable() {
  if sb policy --decidable 2>/dev/null; then
    return 0
  fi
  local prebuilt="$REPO_ROOT/cmd/sb/sb"
  if [ -x "$prebuilt" ]; then
    note "(go-run sb unavailable here; using prebuilt binary $prebuilt)"
    "$prebuilt" policy --decidable 2>&1
    return $?
  fi
  return 1
}

if [ "${DEMO_SKIP_DECIDABLE:-}" = "1" ]; then
  note "DEMO_SKIP_DECIDABLE=1 set — skipping the live sb run."
else
  # Although run with no --regen (the fragment check is read-only), `sb policy`
  # re-derives the Cedar text companions in place as a side effect. Snapshot
  # everything it can touch so arm_restore returns the tree exactly as found.
  for f in policies/cedar/policies.cedar policies/cedar/schema.cedarschema \
           policies/cedar/policies.json policies/cedar/schema.json \
           policies/rego/authz.rego; do
    [ -f "$f" ] && snapshot "$f"
  done
  step "Running: sb policy --decidable  (check-only, no --regen)"
  if run_sb_policy_decidable | grep -E 'decidable|fragment check OK|lattice|targets'; then
    good "decidable-fragment check OK — targets certified inside the terminating tier."
  else
    note "(sb run produced no fragment lines to highlight; see full output above.)"
  fi
fi
echo

# ---------------------------------------------------------------------------
section "Takeaway"
# ---------------------------------------------------------------------------
note "You climb a tier only when you need the expressiveness:"
note "  • flat boolean/numeric/inequality premises live happily in Cedar (the strongest tier);"
note "  • set membership / joins push you up to Rego;"
note "  • Horn-shaped total logic lives in the Decidable-Shen-fragment;"
note "  • general recursion forces you all the way up to full-TC pure-Shen — losing termination guarantees."
note "And the flat policy tiers DELIBERATELY stop at authorization: the cross-field"
note "JWT-binding premise (= User (head (head Jwt))) is left to the guard constructor,"
note "which is where you 'fall off' the flat lattice into the typed-ctor world."
good "Demo 4 complete. Tree left exactly as found."
