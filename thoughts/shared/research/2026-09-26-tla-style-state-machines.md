---
date: 2026-09-26T00:00:00Z
researcher: claude
git_commit: 6b9dde0
branch: claude/tla-plus-shen-synthesis-wqw29z
repository: pyrex41/Shen-Backpressure
topic: "TLA+-style specs in Shen: one state-machine spec, four consumers"
tags: [research, design, tla-plus, state-machines, model-checking, trace-validation, shencheck]
status: prototype
last_updated: 2026-09-26
last_updated_by: claude
---

# TLA+-style specs in Shen

Prompted by Reasonable's "The internet discovers TLA+. Now what?"
(reasonable.io/blog/tla-tutorial). The post's mental model is: a model
describes every possible execution trace of a system, a property
describes which traces are acceptable, and verification asks whether
every possible trace is acceptable. TLC answers that for a finite
instance by enumerating reachable states. From there the post argues for
stronger proof machinery (Verus, Lean) and a connection to real code.

This note collects the ways the Shen projects have been reaching for the
same thing and proposes one clean shape for it. A working prototype is
included: `shen-derive check`, `shen-derive trace` and
`examples/tla-mutex/`.

## What has been tried, and why it felt scattered

| Where | What it does | What was missing |
|---|---|---|
| Shen-Backpressure `examples/.archive/{order,pipeline}-state-machine`, `circuit-breaker`, `workflow-saga` | "The state machine is the type system": a datatype per legal transition, so an illegal transition can't be constructed. | It checks only single steps. Nothing asks what happens across *sequences* of steps (reachability, invariants, deadlock), and the transition graph is written out by hand as types. |
| Shen-Backpressure `2026-05-05-feature-holographic-mock.md` | A Shen state machine as a stateful test double. | It is a design prompt only. |
| Shen-Backpressure `2026-05-05-feature-counterexample-traces.md` | Counterexamples fed back to the agent. | It is a design prompt only, and it is about pure functions. |
| shencheck `transition : model --> action --> observation --> list model` | Possible-state semantics: histories from a real system are checked against a Shen model, with bounded liveness. | Models are JSON strings, the checkers live in Rust, and nothing explores the model *on its own* before the system exists. |
| yggdrasil `trace-check` | Runtime traces checked against a static claim (Datalog containment). | It is a different domain, but it has the same shape: observed behaviour must lie inside the specified behaviour. |

Each of these is one facet of the TLA+ workflow. They sit in different
repos, use different encodings, and "trace" means something different in
each. None of them has the part that makes TLA+ worth using: exhaustive
exploration of the design itself, before any code.

## The shape: one spec, four consumers

Write the system the way TLA+ does, as a state machine, in plain Shen
`define`s:

```shen
(define init -> [S0 ...])                       \* Init *\
(define next S -> [(@p "Action" S') ...])       \* Next: a disjunction of actions *\
(define inv  S -> Bool)                         \* invariant: []Inv *\
(define step-inv S S' -> Bool)                  \* action property: [][A]_vars *\
```

Nondeterminism is a list of successors. Each disjunct of TLA+'s `Next`
is one labelled successor. A state is any value the evaluator has
(lists, strings, numbers, booleans, tuples).

That one artifact then feeds four consumers:

1. **`shen-derive check`: model checking (TLC).** It runs a
   breadth-first search over the reachable states of the finite instance,
   checking state invariants, step invariants and deadlock. Because the
   search is breadth-first, a counterexample is a *shortest* trace. This
   is what finds design bugs before code exists. It is **implemented**.
2. **`shen-derive trace`: trace validation.** The implementation logs its
   state (mapped to spec state) at each linearization point as JSONL.
   Every recorded step must be a stutter or a successor that `next`
   allows, with the action label if one was recorded, and every state
   must satisfy the invariants. This is the "connect to real code" step,
   and it catches implementation bugs that a correct design can't. It is
   **implemented**.
3. **Guards: single-step legality at runtime (shengen, evalhost).** The
   same `next` can back a generated `NewStep(from, to)` constructor whose
   check is "`to` ∈ `next(from)`", evaluated by `runtime/evalhost`
   (profile B, `:runtime-via :eval`). That replaces the hand-written
   transition datatypes in the archived state-machine examples. The
   transition graph is derived from `next` instead of being duplicated as
   types. This is **proposed**.
4. **shencheck: workloads, faults and histories.** shencheck's
   `transition m a o` is `next` restricted to successors consistent with
   the observed action and observation. Its possible-state semantics (a
   set of model states) is exactly trace validation with hidden variables
   (see "Partial observation" below). shencheck brings the things a model
   checker can't: generated workloads against a real deployment, fault
   injection, bounded liveness and shrinking. Its histories then become
   inputs to (2). This is **proposed**, and it is mostly a matter of
   aligning signatures and letting shencheck call a typed spec instead of
   JSON strings.

In the Ralph loop, (1) and (2) are just more `sb` gates. A counterexample
trace is ideal backpressure: it is short, concrete, and says which action
broke which invariant. It also fills the counterexample-traces design
prompt without new machinery.

### Mapping the TLA+ vocabulary

| TLA+ | Shen here |
|---|---|
| `VARIABLES` | the state value; optionally a `datatype` giving it a type |
| `Init` | `(define init -> [...])` |
| `Next == A1 \/ A2 \/ ...` | `(define next S -> [(@p "A1" S1) (@p "A2" S2) ...])` |
| `[]Inv` | `--inv name` over `S --> boolean` |
| `[][A]_vars` | `--step-inv name` over `S --> S --> boolean` |
| stuttering | trace validation accepts `s = s'` |
| `CONSTANTS` / `.cfg` | fixed in the spec for the finite instance, or passed as flags later |
| TLC deadlock check | on by default; `--no-deadlock` turns it off |
| `WF`/`SF`, `<>P`, `~>` | **not yet**; see below |
| refinement (`Spec => HighSpec` under a mapping) | trace validation is its implementation-level form; spec-to-spec refinement is a small extension (below) |

## The worked example: `examples/tla-mutex/`

- `specs/mutex.shen`: two processes and a lock, where taking the lock is
  one atomic step. It has invariants `mutex` and `lock-matches-crit`, and
  step invariant `release-by-holder`.
  - `check` result: 8 states, 14 transitions, OK.
- `specs/mutex-racy.shen`: the same system with check-then-take split
  into two steps.
  - `check` result: it fails with the shortest 6-step interleaving that
    puts both processes in `crit`.
- `impl/main.go`: two goroutines using a real `atomic.Bool` lock, in
  either CAS mode or racy mode, writing a JSONL trace.
  - `trace` against `mutex.shen` accepts the CAS implementation's
    1,201-step traces.
  - It rejects the racy implementation, typically within a few hundred
    steps, with `action "p2" does not take ["crit","want",true] to
    ["crit","crit",true]`.
  - In 20 runs it caught the race 18 times using step checking alone,
    with no invariants.

So the same spec rejects the racy *design* exhaustively and rejects the
racy *implementation* from a trace. Those are two different kinds of
evidence, and they should be reported as such. `check` is exhaustive for
the instance. `trace` is sampled: one run, one interleaving. Both fit the
existing discharge-report categories.

## The instrumentation rule

The one thing trace validation asks of the implementer: log spec state
at **linearization points**, atomically with the change. In the example,
the CAS that takes the lock and the pc update are logged inside one
recorder critical section. If they were logged separately, the trace
would show a state the spec has no step for (lock held, nobody in
`crit`), and a correct implementation would be rejected. That isn't a
flaw in the approach. It is the refinement mapping being made explicit,
and it is where most of the real design thinking happens. A generated
`record(state)` helper (from the state datatype via shengen) is the
natural way to make this cheap.

## Up the ladder: invariants as types

The Reasonable post climbs from model checking to proofs. The step Shen
is uniquely placed to take is the **inductive invariant as a type**:

- TLA+ proves `[]Inv` by showing `Init => Inv` and
  `Inv /\ Next => Inv'`.
- In Shen that is: `init : (list safe-state)` and
  `next : safe-state --> (list safe-state)`, where `safe-state` is a
  datatype whose `verified` premises are `Inv`.
- If `tc+` accepts those signatures, the invariant holds for every
  reachable state of *every* instance, not just the finite one `check`
  explored.

Two cheap stages come before full proof:

- **`check --inductive` (proposed).** Enumerate every state in a bounded
  domain that satisfies `Inv`, not just the reachable ones, apply `next`,
  and check that `Inv` holds after. This is the standard trick for
  debugging an inductive invariant with a model checker. It finds the
  missing conjunct long before a proof attempt does.
- **Typecheck the `next` signature against the invariant type.** This is
  the open question. Shen's sequent calculus can state it, but
  discharging arithmetic `verified` premises inside `tc+` needs rules
  that don't exist today. It needs a spike before anyone relies on it.

Beyond that is Verus/Lean territory: emit the obligations
`Init => Inv` and `Inv /\ Next => Inv'` for an external prover. Out of
scope here, but note that the spec shape above is exactly what those
obligations are stated over.

## Next steps, in order

1. **Wire `check` and `trace` into `sb`** as gate kinds. `check` output
   becomes a discharge-report entry ("exhaustive, instance N states").
   Counterexamples go into `sb context`.
2. **Partial observation.** Real systems can't always log all spec
   state. Track the *set* of spec states consistent with the observations
   so far, as shencheck's possible-state semantics already does, and fail
   when it empties. This makes the `observation` in shencheck's
   `transition` and the hidden variables in TLA+ trace validation the
   same feature.
3. **Spec-to-spec refinement:** `check low.shen --refines high.shen
   --map f`. Explore the low-level spec, and require every step to map to
   a high-level `next` step or a stutter. Both halves are already built.
4. **Generated step guards** from `next` via evalhost (consumer 3), and
   retire the hand-written state-machine examples in `.archive/` in
   favour of one spec per system.
5. **Bounded liveness** in `check`: `eventually P within k steps` on
   every path, which is decidable by the same BFS. True liveness under
   fairness (SCC analysis) only if a real example needs it. shencheck's
   deadline-based `Eventually` is already the pragmatic form.
6. **`check --inductive`**, then the typing spike above.

## What not to build

- **A TLA+ parser or TLA+ syntax in Shen.** The value is the mental model
  (behaviours, `Init`/`Next`, invariants, refinement), not the notation.
  Shen `define`s are already a good notation for it, and the same text
  feeds the evaluator, shengen and `tc+`.
- **A competitive model checker.** There is no symmetry reduction, no
  disk-backed state queue and no parallel search here, and there
  shouldn't be. If a spec outgrows in-memory BFS, the escape hatch is to
  *emit* TLA+ from the (small, pure) Shen `init`/`next` and hand it to
  TLC or Apalache, not to rebuild them.

## Limits of the prototype

- It runs on the shen-derive evaluator subset:
  - Symbols aren't values, so use strings (`"idle"`).
  - There is no `length`, `element?`, and so on; compose from
    `map`/`filter`/`foldr`.
  - Lists in patterns must be cons/bracket forms.
- States are deduplicated by printed form. Keep states canonical (sorted
  sets, fixed field order).
- The whole state graph is held in memory. The default bound is
  `--max-states 1000000`, and exit code 3 means "stopped, not proven".
- Liveness and fairness are not checked.
- Trace validation needs full spec state per line until partial
  observation (step 2) lands.
