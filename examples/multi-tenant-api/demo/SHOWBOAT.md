# Runtime Policy Tiers — Executable Proof

*2026-06-08T21:26:03Z by Showboat 0.6.1*
<!-- showboat-id: e90b436f-c6e8-49c5-981b-2b657387e03d -->

A Shen sequent spec's **access slice** is lowered into multiple runtime-policy
enforcement tiers, and an n-way differential proves the tiers agree with the
generated Go guards. The tiers form an expressiveness lattice:

> **Cedar (SMT) ⊂ Rego (terminating) ⊂ Decidable-Shen-fragment ⊂ full-TC pure-Shen**

This document re-runs the four demos in `examples/multi-tenant-api/demo/` and
captures the proof-bearing output. Every code block is reproducible — run
`showboat verify demo/SHOWBOAT.md` from this directory to re-execute and diff.
The demos are self-restoring (demos 2 & 3 mutate the spec/lowering then revert),
so verification leaves the tree exactly as it found it.

The recommended viewing arc is 1 -> 3 -> 2 -> 4. **Demo 1** shows the same
authorization fact — the premise `(= IsMember true) : verified` on `tenant-access` —
materialized in all four enforcers (Go guard constructor, Cedar permit, Rego rule,
decidable-Shen fragment), then runs the differential to prove they agree.

```bash
NO_COLOR=1 bash /Users/reuben/projects/Shen-Backpressure/examples/multi-tenant-api/demo/demo1_four_enforcers.sh 2>&1 | grep -E "^== |Agreements \(guard|Mismatches \(guard|pure-shen vs guard|cedar-verify PASSED|No privilege-escalation"

```

```output
== DEMO 1 — One spec, four enforcers, provably agreeing
== 1/4  Guard constructor (Go)
== 2/4  Cedar permit policy (SMT tier)
== 3/4  Rego rule (terminating tier)
== 4/4  Decidable-Shen fragment (native, terminating)
== Proof: the n-way differential
Agreements (guard==cedar): 24 (100.0%)
Mismatches (guard vs cedar): 0
pure-shen vs guard mismatches: G-P+=0 G+P-=0 (should be 0 for the fragment)
    Agreements (guard==cedar): 24 (100.0%)
    Mismatches (guard vs cedar): 0
    pure-shen vs guard mismatches: G-P+=0 G+P-=0 (should be 0 for the fragment)
  ✓ cedar-verify PASSED (exit 0) — all exercised tiers agree with the guard oracle.
  ✓ No privilege-escalation rows: no tier is looser than the guard.
== Takeaway
```

**Demo 3** proves the gate has teeth: it injects a lowering bug into the shared
evaluator (`policyspec.EvalVerified` drops a membership/owned check), which only the
*lowered* tiers read — the guard oracle is independent. The differential then reports
`G-P+=14` (guard-deny but tier-allow, a privilege-escalation row) and the gate exits
non-zero. Reverting restores green. The bug cannot ship.

```bash
NO_COLOR=1 bash /Users/reuben/projects/Shen-Backpressure/examples/multi-tenant-api/demo/demo3_bug_cannot_escape.sh 2>&1 | grep -E "^== |no privilege-escalation rows|Mismatches \(guard vs cedar\): 14|G-P\+=14 |GATE FAILED|CANNOT ship|zero mismatches again"

```

```output
== Baseline: the lowered tiers agree with the guard oracle
  ✓ no privilege-escalation rows — every lowered tier matches the guard oracle.
== Inject a lowering bug (drop the membership/owned check)
Mismatches (guard vs cedar): 14
pure-shen vs guard mismatches: G-P+=14 G+P-=0 (should be 0 for the fragment)
cedar-verify: FAIL — evaluator errors=0 mismatches=14 (G-C+=0 G+C-=0 G-R+=0 G+R-=0 G-P+=14 G+P-=0)
  ✗ GATE FAILED (exit 2) — the lowering over-permits. This is the gate working.
  ✗ escalation row: pure-shen vs guard mismatches: G-P+=14 G+P-=0 (should be 0 for the fragment)
cedar-verify: FAIL — evaluator errors=0 mismatches=14 (G-C+=0 G+C-=0 G-R+=0 G+R-=0 G-P+=14 G+P-=0)
  ✓ The over-permitting bug was caught at the differential and CANNOT ship.
== Revert -> green again
  ✓ exit 0, zero mismatches again: pure-shen vs guard mismatches: G-P+=0 G+P-=0 (should be 0 for the fragment)
== Takeaway
```

**Demo 2** shows sum types lower to a disjunction (DNF). Today `authenticated-principal`
is `human OR service`, so each target emits two policy bodies. Adding a third disjunct
(`delegated-principal`) to the spec and regenerating makes every tier regrow to three
bodies in lockstep — the new `(not (= Delegator ""))` premise lowers into both backends.
The spec edit is snapshotted and restored.

```bash
NO_COLOR=1 bash /Users/reuben/projects/Shen-Backpressure/examples/multi-tenant-api/demo/demo2_sum_type_fanout.sh 2>&1 | grep -E "^== |Cedar permit blocks:|Rego bodies:|gained 'context.delegator|regrew|restored [0-9]+ snapshotted"

```

```output
== DEMO 2 — Sum types fan out (DNF lowering)
== Today: authenticated-principal = human OR service
== Add a third disjunct: delegated-principal
== Now: three bodies in every tier
  ✓ Cedar permit blocks: 4 -> 6 (3 tenant + 3 resource)
  ✓ Rego bodies: tenant_access 2 -> 3, resource_access 2 -> 3
  ✓ Cedar gained 'context.delegator != ""' x2; Rego gained 'input.delegator != ""' x2
== Restore — leave the tree exactly as found
    restored 8 snapshotted file(s)
  ✓ DEMO 2 complete: one spec variant -> every tier regrew in lockstep (2 -> 3).
    restored 8 snapshotted file(s)
```

**Demo 4** walks real premises up the lattice and shows where the flat tiers stop.
The cross-field JWT binding `(= User (head (head Jwt)))` uses a nested call, so
`PremiseLowerable` rejects it: it is authentication, discharged upstream by the guard
constructor, not the policy layer. The proof is empirical — the premise is in the spec
but no condition derived from it appears in the emitted Cedar/Rego.

```bash
NO_COLOR=1 bash /Users/reuben/projects/Shen-Backpressure/examples/multi-tenant-api/demo/demo4_climb_the_lattice.sh 2>&1 | grep -E "^== |spec premise present: 109|no user/sub/head/jwt condition|Tree left exactly as found|Demo 4 complete"

```

```output
== The lattice
== Premise expressiveness table
== Empirical proof of the boundary
  ✓ spec premise present: 109:  (= User (head (head Jwt))) : verified;
  ✓ no user/sub/head/jwt condition in either emitted policy file — the premise was skipped.
== Optional: surface the decidable-fragment certification
== Takeaway
  ✓ Demo 4 complete. Tree left exactly as found.
```

All four tiers agree (Demo 1), a lowering bug is caught and blocked (Demo 3), the
disjunction is structural (Demo 2), and the authorization/authentication boundary is
deliberate (Demo 4). To reproduce and diff every block above, run
`showboat verify demo/SHOWBOAT.md` from this directory.

See `demo/README.md` for the per-demo walkthroughs and `../../../POLICY_TIERS_HANDOFF.md`
for the full handoff.
