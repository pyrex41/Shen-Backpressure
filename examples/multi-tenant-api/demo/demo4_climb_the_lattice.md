# DEMO 4 — Climbing the lattice until you fall off

## The point

The policy pipeline lowers a single Shen spec (`specs/core.shen`) into several
runtime tiers, ordered by expressiveness:

```
Cedar (SMT)  ⊂  Rego (terminating)  ⊂  Decidable-Shen-fragment  ⊂  full-TC pure-Shen
```

Each `⊂` is a strict gain in expressiveness paid for with a loss of guarantees.
You **climb a tier only when you need its extra reach**. And — crucially — the
flat policy tiers (Cedar/Rego) **deliberately stop at authorization**: the one
cross-field *authentication* premise in the spec,
`(= User (head (head Jwt)))`, is **not** lowered into any flat policy. It is
discharged upstream by the typed guard constructor. That premise is where you
"fall off" the flat lattice into the typed-ctor world.

This demo is **read-only / illustrative**. It mutates nothing permanently; the
one live `sb` invocation re-derives the Cedar text companions as a side effect,
so the demo snapshots them and restores the tree exactly as found.

## What each tier guarantees

| Tier | Guarantee | Reach |
|---|---|---|
| **Cedar (SMT)** | SMT-decidable; solver-analyzable | strongest guarantees, narrowest reach |
| **Rego (terminating)** | guaranteed-terminating foreign runtime | adds aggregation / joins / walk / graph Cedar can't express |
| **Decidable-Shen-fragment** | termination guaranteed (sequent calculus + Prolog gatekeeper) | Shen-native but restricted: no general recursion, Horn bodies, total forms only |
| **full-TC pure-Shen** | **none** (Turing-complete) | maximal expressiveness; no termination guarantee — top of the lattice |

## The full premise expressiveness lattice table

Real premises drawn from `specs/core.shen`, mapped to the lowest tier that can
express them:

| Premise (from `core.shen`) | Lowest tier that expresses it | Notes |
|---|---|---|
| `(= IsMember true)` | Cedar (all tiers) | boolean equality |
| `(> Exp 0)` | Cedar (all tiers) | numeric comparison |
| `(not (= Sig ""))` | Cedar (all tiers) | inequality |
| `(element? Elem Coll)` | Cedar (all tiers) | Cedar `contains` / Rego `in` / decidable ✓. **Limitation:** inline set literals `[A B C]` not yet parsed; two-operand form only |
| `(= User (head (head Jwt)))` | **✗ none of the flat tiers** | uses `head` (a nested call), so `PremiseLowerable()` rejects it. Scope-to-**authentication**: discharged upstream by the guard constructor `NewAuthenticatedUser`, not by any policy |
| general recursion / unbounded | **full-TC pure-Shen only** | top of the lattice; no termination guarantee |

## How to run

From the example root (`examples/multi-tenant-api`) or anywhere:

```bash
bash demo/demo4_climb_the_lattice.sh
```

Useful environment knobs:

- `DEMO_PAUSE=1` — step through interactively (press enter between sections).
- `NO_COLOR=1` — disable ANSI color (also auto-disabled when not a TTY).
- `DEMO_SKIP_DECIDABLE=1` — skip the live `sb policy --decidable` invocation
  (the rest of the demo is pure grep/illustration and needs no toolchain).

## What to look for

1. **The lattice** — the containment chain and a one-line guarantee per tier.
2. **Premise expressiveness table** — each premise is grepped **live** from
   `specs/core.shen` to prove it is real, not invented.
3. **Empirical proof of the boundary** (the heart of the demo):
   - **1/3 — the spec HAS it:** `grep` finds `(= User (head (head Jwt)))` in the
     `authenticated-user` datatype (`specs/core.shen:109`).
   - **2/3 — the policies DON'T:** grepping `policies/cedar/policies.cedar` and
     `policies/rego/authz.rego` for any `user`/`sub`/`head`/`jwt` condition
     returns **nothing** — the premise was skipped. The conditions that *were*
     lowered (`isMember == true`, `exp > 0`, `sig != ""`) are all
     authorization-scope, flat, atom-only premises.
   - **3/3 — why:** `(head (head Jwt))` is `IsCall()`, so `PremiseLowerable()`
     returns `false` and both emitters (and the evaluator) silently drop it.
     The guard constructor enforces it instead. The tiers agree on the
     *omission*, so no tier is secretly more permissive than the guard.
4. **Decidable-fragment certification** — `sb policy --decidable` (check-only,
   no `--regen`) prints `fragment check OK for targets [tenant-access
   resource-access]`, certifying the targets sit inside the
   termination-guaranteed fragment.

## Takeaway

You climb a tier only when you need the expressiveness — flat boolean / numeric /
inequality premises live happily in Cedar, set membership pushes you to Rego,
Horn-shaped total logic to the Decidable-Shen-fragment, and general recursion all
the way up to full-TC pure-Shen (losing termination guarantees). And the flat
policy tiers deliberately stop at authorization: the cross-field JWT-binding
premise `(= User (head (head Jwt)))` is left to the guard constructor — the point
where you fall off the flat lattice.
