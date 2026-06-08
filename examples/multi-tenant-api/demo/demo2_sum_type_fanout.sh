#!/usr/bin/env bash
#
# DEMO 2 — "Sum types fan out (DNF lowering)."
#
# Point: a sum type in the spec (a conclusion type proved by several rule
# variants) is lowered to a *disjunction* in every policy tier — one Cedar
# `permit` block per variant, one Rego `<rule> if { ... }` body per variant.
# Add a third variant to the spec, rerun `sb policy --regen`, and EVERY tier
# regrows in lockstep: 2 -> 3 bodies, with the new variant's premise lowered
# into a fresh, distinct condition. The disjunction is structural, not
# hand-maintained per backend.
#
# Safe + idempotent: snapshots every file --regen rewrites and restores them
# on EXIT (even on error / ctrl-c), so the git tree is left exactly as found.
#
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

SPEC="$EXAMPLE_DIR/specs/core.shen"
CEDAR_POLICIES="$EXAMPLE_DIR/policies/cedar/policies.cedar"
CEDAR_JSON="$EXAMPLE_DIR/policies/cedar/policies.json"
CEDAR_SCHEMA_JSON="$EXAMPLE_DIR/policies/cedar/schema.json"
CEDAR_SCHEMA_TXT="$EXAMPLE_DIR/policies/cedar/schema.cedarschema"
REGO="$EXAMPLE_DIR/policies/rego/authz.rego"
# `sb policy --regen` also runs the decidable-fragment tier, which rewrites
# these two sidecars next to the spec; snapshot them too so we restore cleanly.
DECIDABLE_CERT="$EXAMPLE_DIR/specs/decidable-fragment.cert"
DECIDABLE_STUB="$EXAMPLE_DIR/specs/decidable_fragment_eval_stub.go"

# sb_run <args...> — invoke the sb CLI with cwd at the example dir (so it finds
# sb.toml + specs/core.shen by relative path) regardless of sb being its own Go
# module. `go run ../../cmd/sb` fails (different module) and `go run -C` would
# move cwd away from the example dir, so: prefer the prebuilt binary; otherwise
# build sb to a temp path from its module dir, then run it from the example dir.
SB_BIN="$REPO_ROOT/cmd/sb/sb"
_SB_TMP=""
sb_run() {
  if [ ! -x "$SB_BIN" ]; then
    if [ -z "$_SB_TMP" ]; then
      _SB_TMP="$(mktemp "${TMPDIR:-/tmp}/demo2-sb.XXXXXX")"
      ( cd "$REPO_ROOT/cmd/sb" && go build -o "$_SB_TMP" . )
    fi
    ( cd "$EXAMPLE_DIR" && "$_SB_TMP" "$@" )
    return
  fi
  ( cd "$EXAMPLE_DIR" && "$SB_BIN" "$@" )
}

# count_matches <pattern> <file> — grep -c that never trips `set -e` (grep
# exits 1 when there are zero matches); prints 0 in that case.
count_matches() {
  local pat="$1" file="$2"
  if [ ! -f "$file" ]; then printf '0\n'; return 0; fi
  grep -c -- "$pat" "$file" 2>/dev/null || true
}

# report_counts <label> — print the per-tier disjunct counts with a label.
report_counts() {
  local label="$1"
  local cedar_permits cedar_deleg rego_tenant rego_resource rego_deleg
  cedar_permits="$(count_matches '^permit (' "$CEDAR_POLICIES")"
  cedar_deleg="$(count_matches 'context.delegator != ""' "$CEDAR_POLICIES")"
  rego_tenant="$(count_matches '^tenant_access if' "$REGO")"
  rego_resource="$(count_matches '^resource_access if' "$REGO")"
  rego_deleg="$(count_matches 'input.delegator != ""' "$REGO")"
  note "$label"
  note "  Cedar  policies.cedar : permit blocks            = $cedar_permits   (tenant + resource variants)"
  note "  Cedar  policies.cedar : 'context.delegator != \"\"' = $cedar_deleg"
  note "  Rego   authz.rego     : 'tenant_access if'        = $rego_tenant"
  note "  Rego   authz.rego     : 'resource_access if'      = $rego_resource"
  note "  Rego   authz.rego     : 'input.delegator != \"\"'   = $rego_deleg"
}

# ---------------------------------------------------------------------------
section "DEMO 2 — Sum types fan out (DNF lowering)"
note "A sum-typed conclusion (one type, several proof rules) lowers to a"
note "DISJUNCTION in each policy tier: one Cedar permit / one Rego 'if' body"
note "per variant. We will add a third variant and watch all tiers regrow."
pause

# ---------------------------------------------------------------------------
section "Today: authenticated-principal = human OR service"
step "The spec proves 'authenticated-principal' two ways (a sum type):"
show_file_lines "$SPEC" 'authenticated-principal;'
note ""
note "Each variant becomes its own clause in the DNF, so each access target"
note "(tenant-access, resource-access) lowers to TWO permit blocks / TWO bodies:"
note "  human-principal  chain -> sig != \"\", exp > 0"
note "  service-principal chain -> secret != \"\""
echo
step "Cedar: count permit blocks in policies.cedar"
note "$(count_matches '^permit (' "$CEDAR_POLICIES") permit blocks (2 tenant variants + 2 resource variants = 4)"
step "Rego: count rule bodies in authz.rego"
note "$(count_matches '^tenant_access if' "$REGO") 'tenant_access if' bodies, $(count_matches '^resource_access if' "$REGO") 'resource_access if' bodies"
echo
report_counts "BEFORE (two disjuncts):"
pause

# ---------------------------------------------------------------------------
section "Add a third disjunct: delegated-principal"
note "We snapshot every file 'sb policy --regen' will rewrite, then arm an"
note "EXIT trap so the tree is restored no matter how this script ends."
snapshot "$SPEC" "$CEDAR_POLICIES" "$CEDAR_JSON" "$CEDAR_SCHEMA_JSON" \
         "$CEDAR_SCHEMA_TXT" "$REGO" "$DECIDABLE_CERT" "$DECIDABLE_STUB"
arm_restore

step "Append a third variant of authenticated-principal to specs/core.shen"
note "delegated-credential carries a flat string premise (not (= Delegator \"\"))"
note "— the same lowerable shape service-credential uses for 'secret != \"\"'."
cat >> "$SPEC" <<'SHEN'

\* --- DEMO 2: third disjunct of authenticated-principal --- *\
(datatype delegated-credential
  Delegator : string;
  Token : string;
  (not (= Delegator "")) : verified;
  ====================================
  [Delegator Token] : delegated-credential;)

(datatype delegated-principal
  Cred : delegated-credential;
  ============================
  Cred : authenticated-principal;)
SHEN
good "appended delegated-principal variant to the spec"
pause

# ---------------------------------------------------------------------------
step "Regenerate the policy tiers"
note "'sb policy --regen' re-parses specs/core.shen DIRECTLY via the"
note "shen-cedar / shen-rego emitters — no Go-guard regen needed. The sum-type"
note "fan-out (BuildRuleVariants -> CollectClauses DNF) runs in policyspec."
sb_run policy --regen
good "emitters re-ran; Cedar + Rego rewritten from the updated spec"
pause

# ---------------------------------------------------------------------------
section "Now: three bodies in every tier"
report_counts "AFTER (three disjuncts):"
echo

CEDAR_PERMITS_AFTER="$(count_matches '^permit (' "$CEDAR_POLICIES")"
CEDAR_DELEG_AFTER="$(count_matches 'context.delegator != ""' "$CEDAR_POLICIES")"
REGO_TENANT_AFTER="$(count_matches '^tenant_access if' "$REGO")"
REGO_RESOURCE_AFTER="$(count_matches '^resource_access if' "$REGO")"
REGO_DELEG_AFTER="$(count_matches 'input.delegator != ""' "$REGO")"

ok=1
if [ "$CEDAR_PERMITS_AFTER" = "6" ]; then
  good "Cedar permit blocks: 4 -> $CEDAR_PERMITS_AFTER (3 tenant + 3 resource)"
else
  bad "Cedar permit blocks expected 6, got $CEDAR_PERMITS_AFTER"; ok=0
fi
if [ "$REGO_TENANT_AFTER" = "3" ] && [ "$REGO_RESOURCE_AFTER" = "3" ]; then
  good "Rego bodies: tenant_access 2 -> $REGO_TENANT_AFTER, resource_access 2 -> $REGO_RESOURCE_AFTER"
else
  bad "Rego bodies expected 3/3, got $REGO_TENANT_AFTER/$REGO_RESOURCE_AFTER"; ok=0
fi
echo
step "Prove the NEW disjunct lowered into both tiers (distinct condition)"
if [ "$CEDAR_DELEG_AFTER" -ge 1 ] 2>/dev/null && [ "$REGO_DELEG_AFTER" -ge 1 ] 2>/dev/null; then
  good "Cedar gained 'context.delegator != \"\"' x$CEDAR_DELEG_AFTER; Rego gained 'input.delegator != \"\"' x$REGO_DELEG_AFTER"
  note "The (not (= Delegator \"\")) premise lowered to delegator != \"\" in BOTH backends."
else
  bad "new delegator condition missing (cedar=$CEDAR_DELEG_AFTER rego=$REGO_DELEG_AFTER)"; ok=0
fi
echo
step "The new Cedar permit / Rego body:"
show_file_lines "$REGO" 'input.delegator != ""'
show_file_lines "$CEDAR_POLICIES" 'context.delegator != ""'
pause

# ---------------------------------------------------------------------------
section "Restore — leave the tree exactly as found"
restore_snapshots
step "git status of the files we touched (expect no demo-induced changes):"
git -C "$EXAMPLE_DIR" status --short -- \
  specs/core.shen \
  policies/cedar/policies.cedar policies/cedar/policies.json \
  policies/cedar/schema.json policies/cedar/schema.cedarschema \
  policies/rego/authz.rego || true
echo
if [ "$ok" = "1" ]; then
  good "DEMO 2 complete: one spec variant -> every tier regrew in lockstep (2 -> 3)."
else
  bad "DEMO 2 finished with unexpected counts — see above."
  exit 1
fi
note "Takeaway: the disjunction is STRUCTURAL. Add a sum-type variant to the"
note "spec and Cedar + Rego (and the decidable fragment) all fan out together;"
note "you never hand-edit each backend's disjunction."
