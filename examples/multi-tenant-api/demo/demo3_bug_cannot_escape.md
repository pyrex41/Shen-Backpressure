# DEMO 3 — The bug that can't escape (the gate has teeth)

## The point

The whole tier story (Cedar ⊂ Rego ⊂ Decidable-Shen-fragment ⊂ full-TC pure-Shen)
only buys you safety if the **lowerings are checked against ground truth**. This
demo proves that the check is real: `make cedar-verify` runs an *n-way
differential* in which the **guard constructors are the oracle**, and every
lowered policy tier (Cedar, Rego, pure-Shen-fragment) must agree with them on
every generated request.

If a lowering drifts **over-permissive** — it would `allow` a request the guard
would `deny` — the harness reports it as a **guard-deny-but-tier-allow** row and
**fails the build with a non-zero exit**. That class of bug (a silent
privilege-escalation in the policy layer) literally cannot ship past the gate.

We demonstrate this by injecting the single minimal over-permitting bug into the
shared evaluator, watching the gate turn red, then reverting and watching it go
green again.

## What it does

1. **Baseline** — runs `make cedar-verify`. Expect exit `0` and
   `pure-shen vs guard mismatches: G-P+=0 G+P-=0`. All tiers agree with the
   guard oracle; no escalation rows.

2. **Inject a lowering bug** — snapshots
   `policyspec/policyspec.go` (and the emitted policy artifacts) and arms an
   auto-restore trap, then applies one minimal edit to `EvalVerified`: it drops
   the line that rejects a clause when a verified premise is `false`. Now
   `(= IsMember true)` / `(= IsOwned true)` still parse but their result is
   discarded — every clause becomes vacuously satisfied, so the pure-Shen tier
   over-permits.

   The exact edit (in `EvalVerified`):
   ```diff
   -        v, vok := evalPremise(e, env)
   +        _, vok := evalPremise(e, env)
            if !vok {
                return false, false
            }
   -        if !v {
   -            return false, true
   -        }
   +        // BUG (demo): membership/owned check dropped — premise result ignored.
   ```

3. **Re-run the gate** — runs `make cedar-verify` again, capturing the exit code
   without aborting the script. The gate now **fails**: non-zero exit and
   `pure-shen vs guard mismatches: G-P+=14 G+P-=0`, with concrete escalating
   requests printed (`tenant G- P+ (pure-shen) ... member=false ... guard=false pure=true`).
   This is the over-permit being caught.

4. **Revert → green again** — restores the snapshot and re-runs the gate to
   confirm it is back to exit `0` with zero mismatches.

## How to run

From the example root:
```bash
bash demo/demo3_bug_cannot_escape.sh
```
or directly:
```bash
examples/multi-tenant-api/demo/demo3_bug_cannot_escape.sh
```

Options:
- `NO_COLOR=1` — disable ANSI color.
- `DEMO_PAUSE=1` — pause between sections (interactive walk-through).

## What to look for

- **Baseline (green):** `G-P+=0 G+P-=0` and the script reports
  `✓ no privilege-escalation rows`.
- **Bug run (red):** the gate exits non-zero and the script prints
  `✗ GATE FAILED` plus the escalation row `G-P+=14` and several concrete
  `G- P+ (pure-shen)` requests where `guard=false` but `pure=true`. These are
  exactly the member=false / owned=false requests the guard denies but the
  broken lowering would allow.
- **After revert (green again):** `G-P+=0 G+P-=0`, exit `0`.

The greppable proof token for the caught bug is `G-P+=[1-9]` in the report line
(and the stderr `cedar-verify: FAIL — ... G-P+=N ...`).

## Safety / reversibility

The mutated file (`policyspec/policyspec.go`) and the tracked emitted policy
artifacts are snapshotted up front, and an `EXIT`/`INT`/`TERM` trap restores
them — so the source tree is left **exactly as found** even if the script errors
out or is interrupted with ctrl-c. Transient `verify-*` report artifacts created
by the run are cleaned up (pre-existing ones are preserved).

## Takeaway

The differential is the **proof obligation**. The guard constructors are the
oracle of truth; every lowered tier is checked against them on every sample. A
lowering that would over-permit shows up as guard-deny-but-tier-allow
(`G-P+` / `G-C+` / `G-R+`) and fails the build with a non-zero exit. The bug
cannot escape — the gate has teeth.
