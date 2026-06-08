# Runtime Policy-Tier Demos

A single Shen sequent spec describes who may access what. Its access slice is
mechanically *lowered* into several enforcement tiers — a Go guard constructor,
a Cedar policy set, a Rego policy set, and a decidable-Shen fragment — and an
n-way differential then **proves** that every tier agrees with the generated
guards on the same set of requests. The point of these four demos is to watch
that pipeline work: one fact, many enforcers, all provably in agreement, with a
gate that refuses to pass if any tier disagrees.

## The lattice

The enforcement tiers are ordered by expressiveness. Each tier can decide
strictly more premises than the one to its left, and each buys you a stronger
guarantee:

```
Cedar (SMT)  ⊂  Rego (terminating)  ⊂  Decidable-Shen-fragment  ⊂  full-TC pure-Shen
   decidable        guaranteed to          provably decidable          Turing-complete:
   by SMT solver    terminate              over the fragment           the reference, but
                                                                       no termination promise
```

A premise lowers into the leftmost tier that can express it; premises that fall
off the right edge (e.g. nested calls like `(= User (head (head Jwt)))`) are
discharged by the generated guard constructors instead, not by the policy tiers.

## Prerequisites

- **Run everything from the example root:**
  `/Users/reuben/projects/Shen-Backpressure/examples/multi-tenant-api`.
  The scripts source `lib/common.sh` and resolve `sb.toml` / `specs/` relative
  to that directory (they `cd` there themselves, so they also work from an
  unrelated cwd).
- **`make cedar-verify` works without `cedar` or `opa` installed** — the
  differential degrades gracefully. With neither validator present the Rego tier
  is *skipped* (counted as zero rather than genuinely agreeing); the
  guard/Cedar-text/decidable-Shen tiers still run and the differential still
  proves agreement among them.
- **Optional, for the real validators:** `cargo install cedar-policy-cli`
  enables the actual Cedar evaluator, and installing `opa` enables the Rego
  tier to participate for real instead of being zeroed.

## Recommended viewing arc: 1 → 3 → 2 → 4

| Step | Demo | One-line rationale |
|------|------|--------------------|
| 1 | `demo1_four_enforcers.sh` | **What it does** — one spec premise shown materialized into all four tiers, proven to agree. |
| 2 | `demo3_bug_cannot_escape.sh` | **Why you trust it** — inject a lowering bug; the differential gate catches it as privilege escalation and fails. |
| 3 | `demo2_sum_type_fanout.sh` | **Why it's interesting** — add a sum-type variant and watch every tier regrow a matching disjunct. |
| 4 | `demo4_climb_the_lattice.sh` | **The theory** — walk real premises up the lattice and see exactly where each tier stops. |

Run each with:

```sh
bash demo/demo1_four_enforcers.sh
bash demo/demo3_bug_cannot_escape.sh
bash demo/demo2_sum_type_fanout.sh
bash demo/demo4_climb_the_lattice.sh
```

Knobs (set as environment variables):

- `DEMO_PAUSE=1` — pause between steps so you can read each stage (step-through mode).
- `NO_COLOR=1` — disable ANSI color in the narration.

## What each demo does

- **`demo1_four_enforcers.sh`** — Takes the single premise `(= IsMember true)`
  for the `tenant-access` target and shows its four materializations: the Go
  guard constructor `NewTenantAccess`, the Cedar `permit` blocks, the Rego
  `tenant_access` rules, and the decidable-Shen fragment. Then runs
  `make cedar-verify` and surfaces the n-way differential summary, gated on a
  zero-mismatch result.
- **`demo3_bug_cannot_escape.sh`** — Establishes a green baseline, injects a
  lowering bug (a premise result is silently discarded in the emitter), re-runs
  the gate to show it fails with a privilege-escalation row (`G-P+` non-zero),
  then reverts and shows green again. Proves the differential is load-bearing,
  not decorative.
- **`demo2_sum_type_fanout.sh`** — A sum-typed conclusion
  (`authenticated-principal`, proved by several variants) lowers to a
  disjunction in every tier. Counts disjuncts before, appends a third variant to
  `specs/core.shen`, regenerates, and shows Cedar (`4 → 6` permit blocks) and
  Rego (`2 → 3` rule bodies) each regrow with the new variant's premise.
- **`demo4_climb_the_lattice.sh`** — Prints the containment chain, maps real
  premises from `specs/core.shen` to their tiers, then empirically proves the
  boundary: a nested-call premise exists in the spec but emits *no* Cedar/Rego
  condition because it falls off the right edge and is discharged by a guard
  constructor instead.

## Safety / reversibility

- **Read-only:** `demo1` and `demo4` do not mutate tracked source. (They may
  trigger regeneration of untracked/generated artifacts, but the tracked git
  tree is left byte-for-byte identical.)
- **Mutating but safe:** `demo2` and `demo3` deliberately edit tracked files
  (`specs/core.shen` and `policyspec/policyspec.go` respectively, plus
  regenerated artifacts). Both **snapshot before and restore after**
  automatically, with a trap that restores even if the script is interrupted
  (SIGINT/SIGTERM). After either runs, `git diff` is clean.

## See also

Background, design rationale, and the full tier-lowering contract:
[`../../../POLICY_TIERS_HANDOFF.md`](../../../POLICY_TIERS_HANDOFF.md).
