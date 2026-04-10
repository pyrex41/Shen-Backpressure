---
date: 2026-04-10T12:39:23-0500
researcher: reuben
git_commit: 49e179c
branch: claude/build-shen-derive-dh1Ti
repository: pyrex41/Shen-Backpressure
topic: "shen-derive verification gate — next directions"
tags: [handoff, shen-derive, verification, sampling, property-testing, sb-gates]
status: ready
last_updated: 2026-04-10
last_updated_by: reuben
type: implementation_strategy
---

# Handoff: shen-derive verification gate — next directions

## Current state

shen-derive is now a **verification gate**, not a code generator. The commit
at `49e179c` ("Pivot shen-derive from code generator to verification gate")
completed the pivot end-to-end. See the commit message for the full diff
summary.

TL;DR of the model: you write a Shen spec (the obvious-correct definition).
An LLM or human writes the Go implementation freely. `shen-derive verify`
evaluates the spec on sampled inputs and emits a table-driven Go test that
asserts the implementation's outputs match the spec's. The spec is the
oracle. Drift detection + the generated test is the gate.

### What works today

- `shen-derive/core/` — s-expression evaluator with IntVal/FloatVal,
  mixed-type arithmetic, BuiltinFn for host-injected operations
- `shen-derive/specfile/` — `.shen` file parser and Shen→Go type table
- `shen-derive/verify/` — sample generator + spec evaluator + Go test
  emitter with auto-generated `mustXxx` helpers
- `shen-derive verify` CLI with `--func`, `--impl-pkg`, `--import`, etc.
- Payment example end-to-end: `examples/payment/specs/core.shen` has a
  `(define processable ...)` block, `internal/derived/processable.go`
  is a hand-written impl, `processable_spec_test.go` has 35 generated
  cases, and `make shen-derive-verify` gates it with drift detection.

### What's archived (under `shen-derive/archive/`, all `.go.bak`)

The rewrite engine (`laws/`), bespoke Go lowering (`codegen/`), Shen tc+
bridge and symbolic prover (`shen/`), and the old codegen demo
(`demo/payment-derived/`) are parked. None of this is deleted. The
algebraic laws catalog and the polynomial prover are valid work that may
return later as **optional** tooling (hints to the LLM, drift checks on
refactors, etc.). They are not on the critical path for the verification
gate.

### Passing tests

```
go test ./...  # all four packages green
cd examples/payment && make shen-derive-verify  # 35 cases pass
```

Introducing a bug in `processable.go` (e.g. `balance += ...` instead of
`balance -= ...`, or silently truncating `b0` to int) makes specific
cases fail with messages like `case_25: spec says true, impl returned
false`.

## Critical references

1. **The commit**: `git show 49e179c` — full picture of the pivot
2. **The plan**: `/Users/reuben/.claude/plans/snazzy-conjuring-spring.md`
3. **Prior handoff (superseded)**: `thoughts/shared/handoffs/general/2026-04-09_18-13-49_shen-derive-v2-sexpr-pivot.md`
   — this one describes the codegen-era plan that got rejected. Useful
   for context on *why* we pivoted but its "next steps" no longer apply.
4. **Shengen parser reference**: `cmd/shengen/main.go:1129-1231` for the
   extractBlocks/parseDatatype algorithm that `specfile/parse.go` mirrors

## Key architecture notes

### Why the spec evaluator has a BuiltinFn value type

The spec body uses operations like `(val B0)` and `(amount Tx)` that are
domain-specific — they depend on what datatypes are in the spec file.
Hardcoding them into the core evaluator would couple it to the type
table. Instead, `core.BuiltinFn{Name, Fn}` wraps a host Go function that
`core.Apply` dispatches to alongside closures and primitives.

The verify harness builds a per-spec `Env` in `verify/harness.go:buildSpecEnv`
that binds:
- `val` as identity (wrapper types are primitives at eval time — samples
  store the primitive directly, not a wrapped form)
- Each composite field name (both original case and lowercase) as a
  projection closure that pulls the corresponding index out of a ListVal

This is why sample values for `transaction` are `ListVal([amount, from, to])`
and `(amount Tx)` just returns index 0.

### Why constraint-aware sampling matters

The primitive number pool is `{0, 1, -1, 5, 2.5, 100}` — includes
negative and fractional values. For a constrained type like `amount`
(which has `(>= X 0) : verified`), the harness evaluates each predicate
against each candidate using the core evaluator itself and drops
failures. This is in `verify/samples.go:filterByConstraints`.

Without this: a bug that silently truncates fractional amounts (one of
the tests in the commit) passes the old hardcoded `{0, 1, 100}` pool.
With this: `case_25` with `b0=mustAmount(2.5)` catches it.

### Why list sampling is one-singleton-per-elem

An earlier version produced only 3 composite samples and then wrapped
them in lists as `{empty, singleton_of_first, multi_of_first_three}`.
A "tricky" composite at index 3 (e.g. the fractional-amount transaction)
was never inside any list, so the 35-case suite wasn't actually testing
it against list-based specs. The fix in `verify/samples.go:listSamples`
produces one singleton per elem sample (capped at 6) — this is what
lets the truncation bug surface.

### The TypeTable is the shengen bridge

`specfile/typetable.go` classifies Shen types using the same logic as
`cmd/shengen/main.go:161-177`. This is deliberately parallel code, not
a shared library, because shengen is a monolithic single-file tool and
extracting a shared parser would risk destabilizing it. The classifier
covers wrapper, constrained, composite, guarded, alias, and sumtype —
but the sampler only handles the first four. Alias and sumtype sampling
return errors today.

## Next directions

The user asked about three specific follow-ons. Here's what I think each
involves, with file pointers and concrete starting steps.

### Direction A — Property-based / seeded-random sampling

**Problem**: the current sampler is deterministic — six hand-picked
number values, cartesian product. This is great for reproducibility and
fast to regenerate, but it doesn't explore the input space. A bug that
only manifests at, say, `b0=3.7` with `tx=[1.8, 2.1]` slips through.

**Approach**:
- Add a `Seed` field to `verify.HarnessConfig` (default 0 = deterministic).
- Build a `math/rand` source from the seed in `verify.BuildHarness`.
- Extend `GenSamples` to produce a configurable number of random draws
  per primitive type in addition to the handpicked boundary set. The
  boundary set is still valuable (0, ±1, maxint, etc.) — random draws
  layer on top.
- The handpicked-boundary logic lives in `verify/samples.go:numberSamples`
  and the analogous helpers for strings/booleans. Add a
  `randomNumberSamples(rnd, n)` and let callers mix them.
- Stamp the seed into the generated test file's header comment so
  regeneration is reproducible given the same seed.
- For composites and lists, the current "one variation per field-sample-
  index" logic naturally extends: if each field has K random samples,
  the composite also gets K variations.
- **Constraint filter still applies**: `filterByConstraints` runs after
  generation, so random values violating `(>= X 0)` get dropped.

**Caveat on test size**: random + cartesian product explodes fast.
`MaxCases` (currently default 50) becomes the critical knob. Consider
switching to a sampling strategy: instead of enumerating the full
cartesian product and truncating, draw N combinations uniformly at
random from the space. `verify/harness.go:cartesian` is where this
would change — replace the odometer with `rand.Shuffle(indexGrid)` or
similar.

**Files**: `verify/samples.go`, `verify/harness.go`, `main.go`
(add `--seed` flag).

**Test**: add a case where the handpicked pool misses a bug but seeded
random catches it. One concrete example: a spec that checks `x mod 7 = 0`
and an impl that checks `x mod 8 = 0`. Handpicked {0, 1, -1, 5, 2.5, 100}
— all except 0 fail the spec predicate so are filtered out, and the
remaining 0 passes both. Bug uncaught. Random draws at size 50 will
hit a value that distinguishes them.

**Risk**: non-determinism can make CI flaky. The answer is: always use
a fixed seed by default, only randomize when explicitly requested. The
generated test file is deterministic given the same seed.

---

### Direction B — Multi-clause defines with `where` guards

**Problem**: `specfile/parse.go:parseDefine` only accepts a single-
clause shape: `Param1 Param2 -> BODY`. Real Shen defines use multiple
clauses with pattern matching:

```shen
(define drug-clear-of-list?
  _ [] _ -> true
  Drug [Med | Meds] Pairs -> false where (pair-in-list? Drug Med Pairs)
  Drug [_ | Meds] Pairs -> (drug-clear-of-list? Drug Meds Pairs))
```

Three clauses: (1) empty-list base case, (2) guarded case, (3) recursive
case. This is how shengen parses defines (see `parseDefine` and
`parseDefineClause` in `cmd/shengen/main.go`). shen-derive currently
rejects these.

**Approach**:
1. Extend `specfile.Define` with a `Clauses []Clause` field. Each clause
   has patterns (one per parameter), an optional guard expression, and
   a body s-expression.
2. Parse the clauses by walking the post-type-sig content line by line
   (or repeated `->` split) and extracting pattern/body/where.
3. The patterns support:
   - wildcards: `_`
   - variable bindings: uppercase names
   - list patterns: `[]`, `[X | Xs]`, `[[X Y] | Rest]`
   - literal atoms: numbers, booleans, strings
4. Add a pattern matcher to the evaluator: `core/match.go` with
   `Match(pattern Sexpr, value Value) (bindings map[string]Value, ok bool)`.
   Patterns are Sexprs (since Shen patterns are s-expression syntax).
5. `verify.evalSpec` becomes: for each clause, try to match the input
   against its patterns; if it matches and the guard (if any) evaluates
   to true, evaluate the body with bindings. Otherwise try the next
   clause. If none match, fall-through error.
6. The sampler doesn't need changes — it still generates inputs by type,
   and the evaluator does the clause dispatch.

**Files to create/modify**:
- `specfile/parse.go`: new `parseDefineClauses` function. The input is
  the content after the type signature; split on line-starting clauses.
- `specfile/define_test.go`: add tests for each clause shape.
- `core/match.go` (new): pattern matcher for Sexpr patterns against
  Values. Supports wildcards, vars, list/cons patterns, literals.
- `core/match_test.go` (new).
- `verify/harness.go:evalSpec`: loop over clauses instead of evaluating
  a single body. Extract `buildSpecEnv` to handle the base environment
  (val + field accessors) separately from per-clause pattern bindings.

**Gotcha**: the Shen cons-list pattern `[X | Xs]` and the tuple pattern
`[X Y]` look similar but mean different things. The parser needs to
disambiguate. shengen's approach: it treats `[X | Xs]` as cons and
`[X Y]` as a fixed-length tuple pattern. Follow that.

**Test target**: port a small real multi-clause define from
`examples/dosage-calculator/specs/core.shen` (which has `pair-in-list?`
and `drug-clear-of-list?`). The commit already found these; see the
exploration in the research agent output referenced in the handoff
at `2026-04-09_18-13-49_shen-derive-v2-sexpr-pivot.md`.

**Caveat**: if a clause calls another define (like `drug-clear-of-list?`
calling `pair-in-list?`), the evaluator needs to resolve that name at
lookup time. That means the env needs **all** defines from the spec
file bound as ClosureVal-like entities at eval start, not just the one
under test. This is a small change to `buildSpecEnv` but worth calling
out: the harness should bind every define in the file so mutual
recursion works.

---

### Direction C — Top-level `sb derive` subcommand hooked into the five-gate pipeline

**Problem**: today you run `shen-derive verify` manually (or via the
payment example's Makefile). There's no `sb` command that knows about
spec-equivalence verification, and no entry in the five-gate pipeline
for it.

**Approach**:
1. Read `cmd/sb/` to understand how existing gates register themselves.
   The existing gates live in something like `cmd/sb/gates.go` with
   per-gate subcommands. Use the same pattern.
2. Add a `[derive]` section to `sb.toml`:
   ```toml
   [derive]
   specs = ["examples/payment/specs/core.shen"]
   impl_pkg = "ralph-shen-agent/internal/derived"
   guard_pkg = "ralph-shen-agent/internal/shenguard"
   funcs = ["processable"]  # or auto-discover from (define ...) blocks
   ```
3. Add `sb derive` subcommand in `cmd/sb/` that:
   - loads the config
   - for each spec, calls into shen-derive's verify package directly
     (import the library — shen-derive is a go module, so `sb` would
     need to declare it as a dependency). **Alternative**: shell out
     to `go run ./shen-derive verify ...` to avoid cross-module imports,
     matching how the payment Makefile does it today.
   - diffs the regenerated test file against the committed copy and
     fails on drift
   - runs `go test ./...` on the implementation package
4. Register `sb derive` as a step in the default pipeline (probably
   after `sb build` and before `sb test`, so that drift is detected
   early).

**Files**:
- `cmd/sb/gates.go` (or equivalent): register the new gate
- `cmd/sb/derive.go` (new): the subcommand implementation
- `cmd/sb/config.go`: add the `[derive]` section type
- `sb.toml`: add the `[derive]` example
- `docs/sb-gates.md` (if present): document the new gate

**Design question**: should `sb derive` regenerate in-place or fail on
drift? The payment Makefile does both (`shen-derive-verify` fails on
drift, `shen-derive-regen` regenerates). Mirror that: `sb derive` fails
on drift by default, with a `--regen` flag to update.

**Auto-discovery vs. explicit funcs**: shengen auto-discovers datatypes
from `(datatype ...)` blocks. The equivalent here is auto-discovering
every `(define ...)` block in the spec file. But the verify command
needs to know `--impl-func` (the Go function name) and `--impl-pkg`
(where it lives) per define. Two options:
- (a) require a `[derive.processable]` sub-section per define with
  `impl_func = "Processable"` etc.
- (b) convention: Shen `processable` → Go `Processable` in the configured
  impl_pkg, no per-func config.
  
Option (b) is simpler; option (a) is more flexible. Start with (b).

**Test**: wire up the payment example's `sb.toml` to use the new gate.
Verify that `sb derive` catches drift and runs tests. Then break the
impl and confirm `sb derive` fails.

---

## Suggested order of attack

1. **Direction B first (multi-clause defines)**. This is the widest
   expansion of what specs can express, which directly increases the
   surface area shen-derive can verify. Without this, specs are limited
   to the obvious-recursion-free fragment (maps, folds, scans). With it,
   specs can describe any pure functional computation over finite
   datatypes — which covers most domain logic.

2. **Direction A (seeded random sampling)**. Amplifies the verification
   power of every spec already in the system, especially once B is
   done and specs can express richer computations.

3. **Direction C (sb gate integration)**. Mostly plumbing. Do it last
   so you're integrating a mature tool rather than a moving target.

All three are independent — no dependencies between them. Parallelizing
is fine if you have multiple sessions.

## Things NOT to do without thinking carefully

- **Don't unarchive `laws/` or `codegen/` reflexively.** They're parked
  for a reason (surface area problem). They may become useful later as
  *hints* to the LLM (e.g. "the rewrite engine says this spec fuses
  into a single-pass loop, suggest that shape"), but that's a different
  integration than what got pivoted away from.
- **Don't try to generate Go implementations from specs.** That was
  the old model. The new model is: the LLM writes the Go, shen-derive
  checks it. Keep the boundary clean.
- **Don't extend `shengen` to read defines.** shengen is happy with
  datatypes. Keeping shen-derive as the only consumer of defines keeps
  the tools decoupled — they share the file format, not the parser.
- **Don't add float64 to composites automatically.** The current
  convention is that wrapper/constrained types store the underlying
  primitive as an IntVal or FloatVal based on where it came from (int
  literal → IntVal, float literal → FloatVal, and the arithmetic
  promotion rule keeps them sane). If you want pure-float semantics
  everywhere for some specs, do it per-define with explicit literals.

## Open questions

1. **What does the gate look like for specs that reference shengen
   types shen-derive doesn't know about?** Today the TypeTable is built
   from the same file, so all referenced types are present. If someone
   writes a spec that references a type from a sibling spec file, the
   parser won't find it. For v1 this is a non-issue; flag it if it
   comes up.

2. **How should specs reference each other?** If `(define
   processable ...)` calls another define `(define all-nonneg ...)`,
   is that supported? Today the harness binds a single define's body
   as the evaluation root. Direction B's "bind all defines in the env"
   fix addresses this naturally.

3. **What happens when the implementation's Go signature doesn't match
   the spec's type signature?** Today shen-derive trusts the user to
   get this right — `--impl-func` is just a name and the harness emits
   a call. If the signature doesn't match, the generated test fails
   to compile. That's acceptable (compile error is a clear failure)
   but a pre-flight check could be nicer. Not a priority.

4. **Should the harness produce a `testing.T` fuzz function (Go 1.18+
   native fuzzing) instead of a table-driven test?** Could be
   complementary: emit both the deterministic 35-case table (for CI)
   and a `func FuzzSpec_Processable` that runs the spec evaluator as
   an oracle (for deep exploration). Worth considering after Direction
   A lands.

## Things worth preserving when iterating

- **The end-to-end payment demo is the gold standard test case.** Any
  change that breaks `make shen-derive-verify` in the payment example
  should be treated as a regression unless intentional.
- **`core/eval.go` is small and well-understood.** Resist adding
  complexity. New operations should go through BuiltinFn unless they
  are truly fundamental (like `val` — but `val` is identity and lives
  in `buildSpecEnv`, not the core).
- **The `.go.bak` convention for archived code.** Do not resurrect old
  files by renaming back to `.go` — that would pull in the old type
  system. Copy specific functions you want and adapt them.
