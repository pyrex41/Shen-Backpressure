#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

# ===========================================================================
# DEMO 3 — "The bug that can't escape (the gate has teeth)."
#
# Point: the n-way differential (`make cedar-verify`) is a real proof
# obligation. The guard constructors are the oracle; the lowered policy tiers
# (Cedar / Rego / pure-Shen-fragment) are checked sample-by-sample against
# that oracle. If a lowering drifts over-permissive — allowing access the
# guard would DENY — the harness reports it as a guard-deny-but-tier-allow
# row and FAILS the build with a non-zero exit. Such a bug literally cannot
# ship past the gate.
#
# We prove this by injecting the minimal over-permitting bug into the shared
# evaluator (policyspec.go EvalVerified: drop the membership/owned check), and
# watching the gate go red. Then we revert and watch it go green again.
#
# This demo is fully reversible: the mutated source file is snapshotted and an
# EXIT/INT/TERM trap restores it even if the script errors or is interrupted.
# ===========================================================================

POLICYSPEC="$REPO_ROOT/policyspec/policyspec.go"

# Tracked artifacts that `make cedar-verify` regenerates each run (the emitters
# rewrite the committed Cedar/Rego policy files, and — when the real cedar
# binary is present — the .cedar/.cedarschema text companions). We snapshot
# these so any regeneration drift is reverted, leaving the git tree untouched.
EMITTED_ARTIFACTS=(
  "$EXAMPLE_DIR/policies/cedar/policies.json"
  "$EXAMPLE_DIR/policies/cedar/schema.json"
  "$EXAMPLE_DIR/policies/cedar/policies.cedar"
  "$EXAMPLE_DIR/policies/cedar/schema.cedarschema"
  "$EXAMPLE_DIR/policies/rego/authz.rego"
)

# Transient (untracked) report artifacts the harness writes on every run. We
# only delete the ones that did NOT already exist when the demo started, so a
# pre-existing developer artifact is never clobbered.
TRANSIENT_ARTIFACTS=(
  "$EXAMPLE_DIR/policies/cedar/verify-report.json"
  "$EXAMPLE_DIR/policies/cedar/verify-report.txt"
  "$EXAMPLE_DIR/policies/cedar/verify-samples.jsonl"
)
PREEXISTING_TRANSIENT=()
for f in "${TRANSIENT_ARTIFACTS[@]}"; do
  [ -e "$f" ] && PREEXISTING_TRANSIENT+=("$f")
done

# cleanup_transient — remove only the transient artifacts the demo created.
cleanup_transient() {
  local f keep pre
  for f in "${TRANSIENT_ARTIFACTS[@]}"; do
    keep=0
    for pre in "${PREEXISTING_TRANSIENT[@]:-}"; do
      [ "$f" = "$pre" ] && keep=1 && break
    done
    [ "$keep" -eq 0 ] && [ -e "$f" ] && rm -f "$f"
  done
  return 0
}

# full_cleanup — restore every snapshot, then clean transient artifacts. Wired
# into the EXIT/INT/TERM trap so it runs even on error or ctrl-c.
full_cleanup() {
  restore_snapshots
  cleanup_transient
  return 0
}

# Run `make cedar-verify` without letting `set -e` abort the script, capturing
# both the combined output and the exit code. Echoes output for the viewer.
#   usage: run_gate <var-to-hold-output> <var-to-hold-rc>
run_gate() {
  local __out_var="$1" __rc_var="$2" _out _rc
  set +e
  _out="$(make cedar-verify 2>&1)"
  _rc=$?
  set -e
  printf '%s\n' "$_out"
  printf -v "$__out_var" '%s' "$_out"
  printf -v "$__rc_var" '%s' "$_rc"
}

# ---------------------------------------------------------------------------
# Arm reversibility up front: even the baseline `make cedar-verify` re-emits the
# tracked policy artifacts, so we snapshot them (and the soon-to-be-mutated
# policyspec.go) and install the restore+cleanup trap before running anything.
# ---------------------------------------------------------------------------
snapshot "$POLICYSPEC" "${EMITTED_ARTIFACTS[@]}"
trap full_cleanup EXIT INT TERM
note "armed auto-restore (EXIT/INT/TERM) for policyspec.go + emitted policy artifacts."

# ---------------------------------------------------------------------------
section "Baseline: the lowered tiers agree with the guard oracle"
# ---------------------------------------------------------------------------
step "make cedar-verify  (n-way differential: guard vs Cedar vs pure-shen-fragment)"
note "The guard constructors (NewTenantAccess / NewResourceAccess) are the oracle."
note "Each generated request is decided by the guard AND by each lowered tier;"
note "the harness counts every disagreement. Default -strict exits non-zero on ANY."

run_gate BASE_OUT BASE_RC

step "Baseline result"
if [ "$BASE_RC" -ne 0 ]; then
  bad "baseline cedar-verify exited $BASE_RC — expected 0. Environment is not clean; aborting."
  exit 1
fi
if echo "$BASE_OUT" | grep -qE 'G-P\+=0 G\+P-=0'; then
  good "exit 0, zero mismatches: $(echo "$BASE_OUT" | grep -E 'pure-shen vs guard mismatches')"
  good "no privilege-escalation rows — every lowered tier matches the guard oracle."
else
  bad "baseline did not show the expected zero-mismatch row; aborting."
  exit 1
fi
pause

# ---------------------------------------------------------------------------
section "Inject a lowering bug (drop the membership/owned check)"
# ---------------------------------------------------------------------------
note "Target: $POLICYSPEC"
note "Function EvalVerified is the shared total evaluator the pure-shen arm calls."
note "We remove the line that rejects a clause when a verified premise is FALSE,"
note "so (= IsMember true) / (= IsOwned true) still parse but their result is"
note "discarded — every clause becomes vacuously satisfied. The tier now over-permits."

note "policyspec.go is already snapshotted and the restore trap is armed (above),"
note "so this file is restored on EXIT / INT / TERM even if the demo errors out."

step "Apply the exact minimal edit"
# Replace the premise-result enforcement with a result-discarding read.
# Fail loudly if the expected original text is not present (recon drift guard).
if ! grep -q 'v, vok := evalPremise(e, env)' "$POLICYSPEC"; then
  bad "expected original text not found in $POLICYSPEC — recon drift; aborting (trap will restore)."
  exit 1
fi
perl -0pi -e 's/\t\tv, vok := evalPremise\(e, env\)\n\t\tif !vok \{\n\t\t\treturn false, false\n\t\t\}\n\t\tif !v \{\n\t\t\treturn false, true\n\t\t\}\n/\t\t_, vok := evalPremise(e, env)\n\t\tif !vok {\n\t\t\treturn false, false\n\t\t}\n\t\t\/\/ BUG (demo): membership\/owned check dropped — premise result ignored.\n/' "$POLICYSPEC"

# Verify the edit actually changed the file.
if grep -q 'BUG (demo): membership/owned check dropped' "$POLICYSPEC" && \
   ! grep -q 'if !v {' "$POLICYSPEC"; then
  good "edit applied: the 'if !v { return false, true }' enforcement is gone."
  note "$(grep -n 'BUG (demo)' "$POLICYSPEC")"
else
  bad "edit did NOT apply as expected; aborting (trap will restore the file)."
  exit 1
fi
pause

# ---------------------------------------------------------------------------
step "Re-run the gate — it must catch the over-permit and fail the build"
# ---------------------------------------------------------------------------
note "The pure-shen arm now allows requests the guard DENIES (member=false / owned=false)."
note "That is the guard-deny-but-tier-allow row — a soundness / privilege-escalation risk."

run_gate BUG_OUT BUG_RC

step "Bug-run result"
ESC_ROW="$(echo "$BUG_OUT" | grep -E 'G-P\+=[1-9]' || true)"
if [ "$BUG_RC" -ne 0 ] && [ -n "$ESC_ROW" ]; then
  bad "GATE FAILED (exit $BUG_RC) — the lowering over-permits. This is the gate working."
  bad "escalation row: $ESC_ROW"
  # Show a couple of the concrete escalating requests, if present.
  echo "$BUG_OUT" | grep -E 'G- P\+ \(pure-shen\)' | head -3 | while IFS= read -r line; do
    bad "$line"
  done
  good "The over-permitting bug was caught at the differential and CANNOT ship."
else
  bad "DEMO FAILED TO REPRODUCE: expected non-zero exit AND a G-P+ escalation row."
  bad "got rc=$BUG_RC ; escalation row='$ESC_ROW'"
  exit 1
fi
pause

# ---------------------------------------------------------------------------
section "Revert -> green again"
# ---------------------------------------------------------------------------
step "Restore the snapshotted source file"
restore_snapshots
if grep -q 'if !v {' "$POLICYSPEC" && ! grep -q 'BUG (demo)' "$POLICYSPEC"; then
  good "source restored: the membership/owned enforcement is back."
else
  bad "restore did not return the file to its original state."
  exit 1
fi

step "Re-run the gate to confirm it is green"
run_gate FIX_OUT FIX_RC
if [ "$FIX_RC" -eq 0 ] && echo "$FIX_OUT" | grep -qE 'G-P\+=0 G\+P-=0'; then
  good "exit 0, zero mismatches again: $(echo "$FIX_OUT" | grep -E 'pure-shen vs guard mismatches')"
  good "the tiers agree with the guard oracle once more."
else
  bad "post-revert run was not green (rc=$FIX_RC); the trap will still restore on exit."
  exit 1
fi
pause

# ---------------------------------------------------------------------------
section "Takeaway"
# ---------------------------------------------------------------------------
note "The differential is the proof obligation. The guard constructors are the"
note "oracle of truth; every lowered tier (Cedar, Rego, pure-Shen-fragment) is"
note "checked against them on every sample. A lowering that would over-permit"
note "shows up as guard-deny-but-tier-allow (G-P+ / G-C+ / G-R+) and fails the"
note "build with a non-zero exit. The bug cannot escape — the gate has teeth."
good "Done. Source tree left exactly as found."
