# DEMO 1 — One spec, four enforcers, provably agreeing

## The point

A single authorization fact is written **once**, as a Shen sequent premise in
`specs/core.shen`:

```
(= IsMember true) : verified;
```

From that one source of truth, the toolchain lowers the same fact into **four
independent enforcement tiers**, each with its own language, runtime, and
expressive power (the lattice is `Cedar (SMT) ⊂ Rego (terminating) ⊂
Decidable-Shen-fragment ⊂ full-TC pure-Shen`):

1. **Guard constructor (Go)** — `internal/shenguard/guards_gen.go`,
   `NewTenantAccess`. The oracle: the smart constructor literally refuses to
   build a `TenantAccess` value unless `isMember == true`. This arm is
   independent of `policyspec`; it is the hard gate everything else is checked
   against.
2. **Cedar permit policy (SMT tier)** — `policies/cedar/policies.cedar`. The
   SMT-strongest decidable layer; `context.isMember == true` appears in every
   tenant-level `permit`.
3. **Rego rule (terminating tier)** — `policies/rego/authz.rego`. OPA /
   Datalog-derived middle tier; `input.isMember == true` guards each
   `tenant_access if { ... }` body.
4. **Decidable-Shen fragment (native, terminating)** —
   `specs/decidable-fragment.cert` + `specs/decidable_fragment_eval_stub.go`.
   Shen-native but decidable (no recursion, Horn bodies only); the certified
   `tenant-access` DNF clauses both pin `(= IsMember true)`.

The demo focuses on the **`tenant-access`** target, shows all four
materializations of that one fact in sequence, then runs the n-way differential
to prove they agree — no tier is looser or stricter than the guard oracle.

## How to run

From the example root (`examples/multi-tenant-api`) or anywhere:

```bash
bash demo/demo1_four_enforcers.sh
```

Useful environment knobs (provided by `demo/lib/common.sh`):

- `NO_COLOR=1` — disable ANSI color (also auto-disabled when stdout is not a TTY).
- `DEMO_PAUSE=1` — step through interactively, pressing Enter between sections
  (only when stdin is a TTY).

## What to look for

- **Sections 1/4–4/4**: the *same fact* (`isMember == true` / `(= IsMember
  true)`) reappears in four different syntaxes and runtimes. Each section ends
  with a green check confirming the membership condition is present in that
  tier.
- **Proof: the n-way differential** runs `make cedar-verify`, which generates
  access samples — *including guard-DENY samples* where `isMember=false` — and
  evaluates every tier on each sample. The summary block to watch:
  - `Agreements (guard==cedar): 24 (100.0%)`
  - `Mismatches (guard vs cedar): 0`
  - `pure-shen vs guard mismatches: G-P+=0 G+P-=0 (should be 0 for the fragment)`
  A pass means **no tier over-permits** relative to the guard oracle (no
  privilege-escalation rows) and **none over-denies**.

### Note on the Rego tier

There is no `opa` binary on this machine, so the **Rego tier is skipped** in the
n-way matrix. The report's `Rego allows (opa eval): 0` and the two `Rego`
disagreement rows reflect a *skipped/zeroed* tier, **not** genuine agreement.
The Cedar and pure-shen-fragment tiers are exercised for real and agree 100%
with the guard. (Install `opa` to light up the Rego column too.)

## Safety / side effects

This demo is **read-only**: it edits no source. The only files written are
`make cedar-verify`'s own artifacts under `policies/cedar/` — `verify-samples.jsonl`,
`verify-report.json`, `verify-report.txt`, and the regenerated emitted policy
files. These are generated/untracked artifacts (expected) and are intentionally
**not** snapshotted or restored. No tracked file is modified.
