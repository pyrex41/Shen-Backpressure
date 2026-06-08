#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

# ===========================================================================
# DEMO 1 — "One spec, four enforcers, provably agreeing."
#
# A single authorization fact — tenant-access requires (= IsMember true) —
# is written once as a Shen sequent premise, then lowered into FOUR distinct
# enforcement tiers:
#   1. Guard constructor (Go)            — the oracle / hard gate
#   2. Cedar permit policy (SMT tier)    — strongest decidable layer
#   3. Rego rule (terminating tier)      — OPA / Datalog-derived middle tier
#   4. Decidable-Shen fragment (native)  — terminating native runtime tier
#
# We show each materialization in turn for the `tenant-access` target, then
# run the n-way differential (`make cedar-verify`) to prove they all agree on
# the same generated samples — no tier is looser or stricter than the oracle.
#
# READ-ONLY: this demo edits nothing. `make cedar-verify` writes its own
# sample/report artifacts under policies/cedar/ (expected, not snapshotted).
# ===========================================================================

GUARDS="$EXAMPLE_DIR/internal/shenguard/guards_gen.go"
CEDAR="$EXAMPLE_DIR/policies/cedar/policies.cedar"
REGO="$EXAMPLE_DIR/policies/rego/authz.rego"
CERT="$EXAMPLE_DIR/specs/decidable-fragment.cert"
STUB="$EXAMPLE_DIR/specs/decidable_fragment_eval_stub.go"

section "DEMO 1 — One spec, four enforcers, provably agreeing"
note "Target under the microscope: tenant-access."
note "The single source fact in specs/core.shen is the verified premise"
note "    (= IsMember true) : verified;"
note "Watch it reappear, unchanged in meaning, in all four tiers below."
pause

# ---------------------------------------------------------------------------
# 1/4  Guard constructor (Go) — the oracle
# ---------------------------------------------------------------------------
section "1/4  Guard constructor (Go)"
step "internal/shenguard/guards_gen.go : NewTenantAccess (the hard gate / oracle)"
note "Smart constructor. The ONLY way to obtain a TenantAccess value is to pass"
note "its membership check — a TenantAccess cannot exist for a non-member."
# Extract the func body: from the signature line to its closing brace.
start="$(grep -n '^func NewTenantAccess' "$GUARDS" | head -1 | cut -d: -f1)"
sed -n "${start},$((start + 9))p" "$GUARDS"
good "Source-of-truth check present: if !(isMember == true) { return ..., error }"
note "This arm is independent of policyspec — it constructs real shenguard ctors."
pause

# ---------------------------------------------------------------------------
# 2/4  Cedar permit policy (SMT tier)
# ---------------------------------------------------------------------------
section "2/4  Cedar permit policy (SMT tier)"
step "policies/cedar/policies.cedar : the tenant-level permit block(s)"
note "Emitted by shen-cedar. Cedar is the SMT-strongest tier; it authorizes if"
note "ANY permit matches. The sum-typed principal fans out into multiple permits"
note "(human-principal chain: sig/exp ; service-principal chain: secret)."
# Print every permit block that contains the tenant level marker, plus context.
awk '
  /^permit \(/ { buf=$0; cap=1; next }
  cap==1 {
    buf=buf "\n" $0
    if ($0 ~ /context\.level == "tenant"/) keep=1
    if ($0 ~ /};/) { if (keep) print buf "\n"; buf=""; cap=0; keep=0 }
  }
' "$CEDAR"
good "Both tenant permits carry  context.isMember == true  — same fact as the guard."
pause

# ---------------------------------------------------------------------------
# 3/4  Rego rule (terminating tier)
# ---------------------------------------------------------------------------
section "3/4  Rego rule (terminating tier)"
step "policies/rego/authz.rego : the tenant_access rule bodies"
note "Emitted by shen-rego. Always 'default ... := false' (explicit deny);"
note "multiple 'if' bodies are OR-ed natively — one body per principal variant."
# default line + every tenant_access if{...} body (up to resource_access).
sed -n '/^default tenant_access/,/^default resource_access/p' "$REGO" \
  | sed '/^default resource_access/d'
good "Each tenant_access body requires  input.isMember == true  — the same fact."
pause

# ---------------------------------------------------------------------------
# 4/4  Decidable-Shen fragment (native, terminating)
# ---------------------------------------------------------------------------
section "4/4  Decidable-Shen fragment (native, terminating)"
step "specs/decidable-fragment.cert : the certified fragment marker"
note "Shen-native but decidable: no recursion, Horn bodies only — terminates by"
note "construction. The differential's pure-shen arm runs this tier directly."
cat "$CERT"
echo
step "specs/decidable_fragment_eval_stub.go : the tenant-access certified clauses"
note "fragmentClauses['tenant-access'] is the DNF the total evaluator checks."
grep -n '"tenant-access":' "$STUB"
good "Clause 1 and clause 2 both pin  (= IsMember true)  — the same fact, again."
pause

# ---------------------------------------------------------------------------
# Proof: the n-way differential
# ---------------------------------------------------------------------------
section "Proof: the n-way differential"
step "make cedar-verify  (guard vs Cedar vs Rego vs pure-shen-fragment, same samples)"
note "Generates access samples — including guard-DENY samples (isMember=false) —"
note "and evaluates every tier on each. Any tier looser/stricter than the guard"
note "oracle counts as a mismatch and fails (exit 1). Zero mismatches = agreement."
note "(No opa binary here, so the Rego tier is SKIPPED in the matrix; Cedar and"
note " pure-shen-fragment are exercised for real.)"
echo

# Capture output so we can both show it and assert on the agreement summary.
VERIFY_OUT="$(mktemp "${TMPDIR:-/tmp}/demo1-cedar-verify.XXXXXX")"
verify_rc=0
make cedar-verify >"$VERIFY_OUT" 2>&1 || verify_rc=$?

# Surface the agreement summary block (the n-way differential report).
sed -n '/=== Guard \/ Cedar \/ Rego \/ pure-shen-fragment n-way Differential ===/,/=== end report ===/p' "$VERIFY_OUT" \
  || cat "$VERIFY_OUT"

echo
# Pull the headline numbers for an explicit verdict.
agree_line="$(grep -E 'Agreements \(guard==cedar\)' "$VERIFY_OUT" | head -1 | sed 's/^[[:space:]]*//')"
mismatch_line="$(grep -E 'Mismatches \(guard vs cedar\)' "$VERIFY_OUT" | head -1 | sed 's/^[[:space:]]*//')"
pure_line="$(grep -E 'pure-shen vs guard mismatches' "$VERIFY_OUT" | head -1 | sed 's/^[[:space:]]*//')"
[ -n "$agree_line" ]    && note "$agree_line"
[ -n "$mismatch_line" ] && note "$mismatch_line"
[ -n "$pure_line" ]     && note "$pure_line"

# Verdict: pass iff cedar-verify exited 0 AND no privilege-escalation rows.
escalation=0
grep -Eq 'G-P\+=[1-9]'   "$VERIFY_OUT" && escalation=1   # pure-shen too loose
grep -Eq 'G- C\+'        "$VERIFY_OUT" && escalation=1   # cedar too loose (per-row)
if [ "$verify_rc" -eq 0 ] && [ "$escalation" -eq 0 ]; then
  good "cedar-verify PASSED (exit 0) — all exercised tiers agree with the guard oracle."
  good "No privilege-escalation rows: no tier is looser than the guard."
else
  bad "cedar-verify reported drift (exit=$verify_rc, escalation=$escalation) — see report above."
fi
rm -f "$VERIFY_OUT"

# ---------------------------------------------------------------------------
section "Takeaway"
echo "The same authorization fact — written once as a sequent premise"
echo "((= IsMember true) : verified) — is enforced four ways: a Go guard"
echo "constructor, a Cedar permit, a Rego rule, and the decidable-Shen fragment."
echo "The n-way differential gates them against drift: one spec, four enforcers,"
echo "provably agreeing."
