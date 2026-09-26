---
date: 2026-09-26T00:00:00Z
researcher: claude
git_commit: 6b9dde0
branch: claude/tla-plus-shen-synthesis-wqw29z
repository: pyrex41/Shen-Backpressure
topic: "TLA+-style specs in Shen: one state-machine spec, four consumers"
tags: [research, design, tla-plus, state-machines, model-checking, liveness, trace-validation, shencheck]
status: prototype
last_updated: 2026-09-26
last_updated_by: claude
---

# TLA+-style specs in Shen

This note responds to Reasonable's "The internet discovers TLA+. Now
what?" (Mészáros et al., 25 Sept 2026). It collects the ways the Shen
projects have been reaching for the same thing and proposes one clean
shape for it.

A working prototype is included:

- `shen-derive check` and `shen-derive trace`;
- `examples/leader-election/`, which is the article's running example;
- `examples/tla-mutex/`, which checks a spec against a real Go
  implementation.

## What the article says, and what it asks for

The article teaches TLA+ through leader election among three computers:

- a model has **states** and **actions**;
- a property says which **executions** are acceptable;
- TLC explores every reachable state of a finite instance.

The article's numbers:

| Case | Result |
|---|---|
| 3 computers | 38 states, "never two leaders" holds |
| A bug that lets a computer vote twice | a six-step counterexample ending with two leaders |
| A typo that stops anything from happening | safety still passes; only liveness (`<> someone is leader`) catches it |
| Liveness in general | needs fairness: WF(A) for actions that stay enabled, SF(A) for actions enabled infinitely often |

It then names three places where TLA+ stops short:

1. **Model checking only covers finite instances.** 38 states with three
   computers becomes more than a million with nine. General claims need
   proofs, and TLAPS's automation is limited, especially for liveness.
2. **The model is not the implementation.** Nothing keeps them in step
   as the code changes.
3. **Linear time can't state everything.** "From any state, a new
   election can still be started" is CTL (branching time). "This
   computer has a strategy to become leader" is ATL (strategic).

Reasonable's answer is to move into Verus:

- a TLA+→Verus transpiler;
- a prover–reviewer agent loop with an anti-cheat gatekeeper, which
  checks the agents didn't modify the spec or slip in `assume(false)`;
- a library of WF1/WF2/SF1/SF2 liveness rules as proven lemmas;
- refinement proofs in the style of Anvil;
- more than 3,000 machine-checked proofs from 16,459 TLA+
  spec/property pairs.

Further out they name program synthesis and *protocol search*, where a
cheap verifier becomes the objective function in an evolutionary loop.

## What has been tried here, and why it felt scattered

| Where | What it does | What was missing |
|---|---|---|
| Shen-Backpressure `examples/.archive/{order,pipeline}-state-machine`, `circuit-breaker`, `workflow-saga` | "The state machine is the type system": a datatype per legal transition. | It checks only single steps. There is nothing over sequences of steps (reachability, invariants, deadlock, liveness), and the transition graph is written by hand as types. |
| Shen-Backpressure `2026-05-05-feature-holographic-mock.md`, `...-counterexample-traces.md` | A Shen state machine as a test double; counterexamples as agent feedback. | Both are design prompts only. |
| shencheck `transition : model --> action --> observation --> list model` | Possible-state semantics over real histories, with bounded liveness. | Models are JSON strings and checkers are Rust. Nothing explores the model on its own before a system exists. |
| yggdrasil `trace-check` | Runtime traces checked against a static claim (Datalog containment). | It is a different domain, but it has the same shape: observed behaviour must lie inside specified behaviour. |

Each is one facet of the TLA+ workflow. Nothing had the core of it:
exhaustive exploration of the design, with safety *and* liveness,
before code.

## The shape: one spec, four consumers

Write the system the way TLA+ does, in plain Shen `define`s:

```shen
(define init -> [S0 ...])                       \* Init *\
(define next S -> [(@p "Action" S') ...])       \* Next = A1 \/ A2 \/ ... *\
(define inv  S -> Bool)                         \* safety: []Inv *\
(define step-inv S S' -> Bool)                  \* action property: [][A]_vars *\
(define p S -> Bool)                            \* for <>p, p ~> q, AG EF p *\
(define quorum -> 2)                            \* CONSTANTS; --const overrides *\
```

Nondeterminism is a list of successors. Each labelled successor is one
disjunct of TLA+'s `Next`, which is also the "split over actions" the
article's inductive proofs need. That one artifact feeds four consumers:

1. **`shen-derive check`: model checking.** It runs a breadth-first
   search over the reachable states and checks:
   - invariants, step invariants and deadlock;
   - `<>P` and `P ~> Q` under WF(Next), plus per-action `--wf` / `--sf`
     (glob-matched labels);
   - the CTL property `AG EF P` (`--possible`), which TLA+ cannot state.

   Safety counterexamples are shortest. Liveness counterexamples are
   lassos ("back to state k, repeats forever"). **Implemented.**
2. **`shen-derive trace`: trace validation.** The implementation logs
   mapped spec state at its linearization points. Every step must be a
   stutter or an allowed successor, with its action label if one was
   recorded. This is the cheap, evidence-level answer to "the model is
   not the implementation". **Implemented.**
3. **Step guards.** A generated `NewStep(from, to)` constructor checks
   `to ∈ next(from)` via `runtime/evalhost` (profile B,
   `:runtime-via :eval`). The transition graph is derived from `next`
   instead of hand-written as types. **Proposed.**
4. **shencheck.** Its `transition m a o` is `next` restricted to what
   was observed. Its possible-state semantics is trace validation with
   hidden variables. It adds what a model checker can't: real
   deployments, fault injection and shrinking. **Proposed.**

### The article's example, reproduced

`examples/leader-election/specs/election.shen` is modelled from the
article's description. It reproduces every number the article gives and
goes on to exercise fairness:

| Run | Result |
|---|---|
| Base model | 38 states, 57 transitions; `one-leader` holds. |
| `--const double-vote?=true` | A 6-step counterexample ending in two leaders (article level 2). |
| `--eventually has-leader` | Fails: a, b and c all start, each holds only its own vote, and nothing is enabled. The article doesn't say whether its model has this split vote. |
| `--const timeouts?=true` | Split votes retry. Now a livelock: "back to state 0, repeats forever". |
| `... --wf '* votes *'` | Still fails. Votes are disabled once everyone has voted, so weak fairness is met without voting. |
| `... --sf '* votes *'` | Holds. Votes are enabled infinitely often, so strong fairness forces one. |
| `--const quorum=4` | Safety passes and liveness fails (article level 3's point). |
| `--possible can-start` | Fails. After someone wins, no new election can ever start. This is the article's own CTL example, answered for this model. |

These all run in `TestElection`.

### Mapping the TLA+ vocabulary

| TLA+ | Here |
|---|---|
| `VARIABLES` | the state value; optionally a `datatype` giving it a type |
| `Init`, `Next` | `(define init -> [...])`, `(define next S -> [...])` |
| `[]Inv`, `[][A]_vars` | `--inv`, `--step-inv` |
| `<>P`, `P ~> Q` | `--eventually`, `--leads-to P:Q` |
| `WF_vars(Next)` | always assumed: no infinite stuttering while something is enabled |
| `WF_vars(A)`, `SF_vars(A)` | `--wf`, `--sf` on action labels |
| `CONSTANTS` / `.cfg` | nullary defines, overridden with `--const name=expr` |
| TLC deadlock check | on by default; `--no-deadlock` |
| CTL `AG EF P` | `--possible` (not expressible in TLA+) |
| refinement | `trace` for implementations; spec-to-spec is next (below) |

## Where this meets the article's roadmap

- **The gatekeeper already exists.** The article's anti-cheat
  gatekeeper checks that proving agents didn't touch the spec or assume
  their way out. That is what Shen-Backpressure's gates, TCB audit and
  drift checks already do for Ralph loops. `check` and `trace` become two
  more gates, and a counterexample trace is ideal backpressure: short,
  concrete, and it names the action and the property.
- **Protocol search.** It needs a verifier cheap enough to be an
  objective function. An in-process check of a 38-state model takes
  milliseconds. The Ralph loop plus a `check` gate is protocol search
  with an agent as the mutation operator.
- **Proof, the Shen way: invariants as types.** The article's safety
  proofs are inductive:
  - `Init => Inv` and `Inv /\ Next => Inv'`, split over actions.
  - In Shen: `init : (list safe-state)` and
    `next : safe-state --> (list safe-state)`, where `safe-state`'s
    `verified` premises are `Inv`.
  - If `tc+` accepts that, the invariant holds for every instance size,
    which is the article's "38 vs a million" problem.
  - Cheap stage first: `check --inductive` explores every state in a
    bounded domain satisfying `Inv`, not just the reachable ones, and
    finds the missing conjunct.
  - Open question: can `tc+` discharge arithmetic `verified` premises?
    It needs a spike.
- **Liveness proofs, the same way.** Their WF1 rule has three
  premises:
  - `P /\ Next => P' \/ Q'`
  - `P /\ A => Q'`
  - `P => ENABLED A`

  All three are state or step predicates. The first two are step
  invariants and the third is a state invariant, so `check` can already
  test them on an instance. They are also the obligations a typed proof
  (or an emitted Verus lemma) would carry. A `--wf1 P:A:Q` convenience
  flag would make this a first-class certificate check.
- **Richer logics.** `--possible` shows that the explicit graph makes
  CTL cheap. General CTL (`EG`, `AU`, …) is a fixpoint over the same
  graph. ATL (strategies) is out of scope.
- **Verus/Lean.** The spec shape above is exactly what TLA+→Verus
  pipelines translate. If Shen specs ever need machine-checked proofs
  beyond `tc+`, *emit* Verus or TLA+ from `init`/`next` rather than
  rebuilding a prover.

## The instrumentation rule for `trace`

Log spec state at **linearization points**, atomically with the change.
In `examples/tla-mutex/impl`, the CAS that takes the lock and the pc
update are logged inside one recorder critical section. Logging them
separately would show a state the spec has no step for, and a correct
implementation would be rejected. This is the refinement mapping made
explicit. A generated `record(state)` helper would make it cheap.

## Next steps, in order

1. **Wire `check` and `trace` into `sb`** as gate kinds, with discharge
   report entries:
   - "exhaustive for instance (N states)" from `check`;
   - "sampled (one trace)" from `trace`.

   Counterexamples go into `sb context`.
2. **Partial observation in `trace`.** Track the *set* of spec states
   consistent with observations so far, as shencheck's possible-state
   semantics does. This unifies shencheck `transition` and TLA+ trace
   validation's hidden variables.
3. **Spec-to-spec refinement:** `check low.shen --refines high.shen
   --map f`. Every low step must map to a high `next` step or a stutter.
4. **`check --inductive`** and **`--wf1 P:A:Q`**: proof-shaped
   certificates, checked on instances.
5. **The `tc+` typing spike** for invariants as types.
6. **Step guards from `next`** via evalhost. Retire the hand-written
   state-machine examples in `.archive/`.

## What not to build

- **A TLA+ parser, or TLA+ syntax in Shen.** The value is the mental
  model, not the notation. Shen `define`s already feed the evaluator,
  shengen and `tc+`.
- **A competitive model checker.** There is no symmetry reduction, no
  disk-backed queue and no parallel search here, and there shouldn't be.
  If a spec outgrows in-memory BFS, emit TLA+ for TLC/Apalache, or Verus
  for proofs.

## Limits of the prototype

- It runs on the shen-derive evaluator subset:
  - Symbols aren't values, so use strings.
  - Helpers like `nth` are written in the spec.
  - `cn` is the only string primitive.
- States are deduplicated by printed form, so keep them canonical.
- The whole graph, including edges, is in memory. `--max-states`
  bounds it, and exit code 3 means "stopped, not proven". Temporal
  properties are checked only on a complete graph.
- Liveness always assumes WF(Next). A `--wf`/`--sf` glob covers the
  disjunction of the matching actions, not each one separately. For
  per-instance fairness, list the labels individually.
- `trace` needs full spec state on every line until step 2 lands.
