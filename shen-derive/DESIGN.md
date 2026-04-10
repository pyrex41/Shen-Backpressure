# shen-derive: Design Notes

## What it is

**shen-derive is a verification gate, not a code generator.** You write
the obvious-correct definition of a function as a Shen `.shen` spec. A
human (or an LLM) writes the Go implementation freely. `shen-derive
verify` evaluates the spec on sampled inputs and emits a table-driven
Go test that asserts the implementation's outputs match the spec's.

The spec is the oracle. Drift detection on the generated test file,
plus `go test` on the impl, is the gate.

```
specs/core.shen         Shen (define processable ...) — the oracle
       |
       v  (shen-derive verify)
processable_spec_test.go  Generated table-driven test (N cases)
       |
       v  (diff against committed copy)
drift?  --------- yes ---> fail the gate
       |
       no
       v
go test ./internal/derived/...   Run the generated test against the impl
       |
       v
impl mismatches spec? ---------- yes ---> fail the gate
       |
       no
       v
Ship it.
```

Everything else in the package is in service of that pipeline.

## Why the pivot (v2)

The v1 version of shen-derive was a rewrite-engine code generator: it
parsed a typed lambda calculus, applied Bird-Meertens-style algebraic
laws (`map-fusion`, `foldr-fusion`, etc.), and lowered the result to
idiomatic Go loops. The catalog grew to seven laws; the codegen produced
a 20-target corpus; the payment demo derived a single-pass fold from an
`all . scanl` composition and emitted working Go.

It worked. It also hit a hard wall. The surface area shen-derive could
verify was bounded by the law catalog and the codegen patterns. Adding a
new shape of computation meant adding a new law *and* a new lowering
pattern *and* proving its equivalence against the naive form. Every
domain-specific wrinkle (guard types, constrained values, field
accessors) fought the type system. The v1 corpus was the ceiling, and
the ceiling was low.

The verification-gate model flips the polarity. Instead of generating
the fast code *from* the slow spec, we trust the LLM (or the human) to
write the fast code, and we use the slow spec as the oracle that catches
mistakes. The set of specs shen-derive can verify is now any pure
functional computation Shen can express — which is a strictly larger
space than the rewrite engine ever reached.

The archived v1 code lives under `shen-derive/archive/` as `.go.bak`
files. It is not deleted; the rewrite engine may return later as
*hints* to the LLM ("this spec fuses into a single pass, consider
that shape"), but it is no longer on the critical path.

## Architecture

```
shen-derive/
  core/          S-expression AST, parser, evaluator, pattern matcher
  specfile/      .shen file parser + Shen→Go type table
  verify/        Sample generator + spec evaluator + Go test emitter
  archive/       Parked v1 code (laws/, codegen/, shen/, demo/) — .go.bak
  main.go        CLI: `verify`, `parse`, `lint`
```

### core/

The s-expression layer. `Sexpr` is `Atom | List`, nothing else. The
parser handles Shen's surface syntax: parens, brackets for cons sugar
(`[X | Xs]` → `(cons X Xs)`), strings, atoms, and `\\` / `--` line
comments. The evaluator runs over `Sexpr` directly — there is no
separate typed AST.

`Value` is the runtime value type: `IntVal`, `FloatVal`, `BoolVal`,
`StringVal`, `ListVal`, `TupleVal`, `ClosureVal`, `PrimPartial`, and
`BuiltinFn`. `BuiltinFn` is the escape hatch for host-injected
operations — field accessors, `val`, and curried define references all
travel as `BuiltinFn` values so the core evaluator never needs to know
about the type table or the spec file.

Pattern matching lives in `core/match.go`. Shen patterns are themselves
s-expressions, so `Match(pat Sexpr, v Value)` is a small recursive walk
— wildcards, uppercase-variable binding, literal atoms, the `nil`
atom for empty lists, and `cons` for head-tail destructuring.
Fixed-length list patterns like `[A B]` work automatically via their
desugaring to `(cons A (cons B nil))`.

### specfile/

The `.shen` file parser. It consumes the same files as `shengen` —
`(datatype ...)` blocks for guard types, `(define ...)` blocks for the
specs shen-derive verifies. The datatype parser mirrors shengen
(`cmd/shengen/main.go:1129-1231`) closely enough that both tools read
the same files; the define parser extends that algorithm with
pattern-match clauses and `where` guards.

A `Define` can be a single-clause shape (the payment demo's
`processable`) or a multi-clause shape with nested list patterns and
guards (the dosage-calculator's `pair-in-list?`). Every clause stores
its patterns, optional guard, and body as `Sexpr` values.

The `TypeTable` classifies each Shen type as wrapper, constrained,
composite, guarded, alias, or sumtype, and records the field layout
needed for sample generation and Go literal emission. It is a
deliberate parallel to shengen's classifier rather than a shared
library — shengen is a monolithic single-file tool and keeping them
loosely coupled is worth more than dedup.

### verify/

The harness. Given a `Define` and a `TypeTable`, `BuildHarness`:

1. Generates deterministic boundary samples for each parameter type.
   Numbers get `{0, 1, -1, 5, 2.5, 100}`; strings get
   `{"", "alice", "bob"}`; booleans get both; wrapper/constrained types
   filter the underlying primitive pool against their `verified`
   predicates (evaluated by the core evaluator itself); composite types
   get one variation per field-sample index; list types get empty plus
   one singleton per elem sample plus a small multi-element list.
2. Optionally layers seeded random draws on top of the boundary set,
   gated on `HarnessConfig.Seed != 0`. The default (`seed=0`) stays
   deterministic so committed test files don't drift between runs.
3. Takes the cartesian product of per-parameter samples, capped at
   `MaxCases` (default 50).
4. Builds a single base evaluation environment that binds `val` to the
   identity, each field accessor as a projection closure, and every
   define in the spec file as a curried `BuiltinFn`. The curried form
   is what lets clause bodies call each other (and themselves) by name,
   supporting mutual recursion.
5. For each case, dispatches on the `Define`'s clauses in order: try
   each clause's patterns against the arguments, evaluate the `where`
   guard if any, and run the first matching body.
6. Converts the evaluated value to a Go literal (via the type table's
   return-type mapping) and records the case.

`Harness.Emit()` then produces a package-qualified Go test file:
imports, `mustXxx` helpers for each guard type it constructs, a
table-driven test calling the implementation function, and a pointwise
comparison. Wrapper/constrained return types get unwrapped via
`.Val()` before comparison so the want column stays a plain Go
primitive.

### The `sb derive` gate

`cmd/sb/derive.go` is the integration point with the larger pipeline.
For every `[[derive.specs]]` entry in a project's `sb.toml`, it shells
out to `shen-derive verify` (separate Go modules, so no direct import),
diffs the regenerated output against the committed `out_file`, and
fails on drift. After all diffs pass it runs `go test` on each
referenced impl package. `sb gates` automatically registers `sb derive`
as the sixth gate when any derive specs are configured.

## Design choices and trade-offs

### Why the spec evaluator has `BuiltinFn`

Spec bodies use operations like `(val B0)` and `(amount Tx)` that are
domain-specific — they depend on what datatypes are in the spec file.
Hardcoding them into the core evaluator would couple it to the type
table. Instead, `BuiltinFn{Name, Fn}` wraps a host Go function that
`Apply` dispatches to alongside closures and primitives, and the verify
harness builds a per-spec `Env` that populates all the domain builtins
before evaluation starts.

### Why constraint-aware sampling matters

The primitive number pool includes negatives and a fractional value.
For a constrained type like `amount` (which has `(>= X 0) : verified`),
the harness evaluates each predicate against each candidate using the
core evaluator itself and drops failures. Without this, a bug that
silently truncates fractional amounts slides past a handpicked
`{0, 1, 100}` pool; with it, the truncation case becomes one of the
generated test rows.

### Why list sampling is one-singleton-per-elem

An earlier version produced only three composite samples and then
wrapped them in lists as `{empty, singleton_of_first,
multi_of_first_three}`. A "tricky" composite at index 3 (e.g. the
fractional-amount transaction) was never inside any list, so the
35-case suite wasn't actually testing it against list-based specs. The
current sampler produces one singleton per elem sample (capped at 6) —
this is what lets the truncation bug surface.

### Why the spec is the oracle, not the generator

The LLM writes the Go freely. That is on purpose. The Go code can use
whatever idioms, data structures, and optimizations the author wants —
maps, pointers, goroutines, SIMD — as long as the pointwise behavior
on the sampled inputs matches the spec. shen-derive doesn't care how
you got there. It cares that drifting away gets caught.

### Why seeded random sampling is opt-in

Random sampling risks CI flakiness if the default changed silently.
The committed test files are part of the drift gate, so any change in
the sampling strategy would require a one-time regen. Keeping the
default deterministic means existing gated tests don't churn; projects
that want deeper exploration pass `--seed N` explicitly, and the seed
stamps into the generated file's header for reproducibility.

## What's explicitly out of scope

- **Generating Go implementations.** That was the v1 model and is now
  archived. Don't resurrect it reflexively. If the rewrite engine
  returns, it returns as an *LLM hint*, not a code generator.
- **Extending shengen to read `(define ...)` blocks.** shengen stays a
  monolithic datatype compiler. shen-derive is the only consumer of
  defines. They share the file format, not the parser.
- **Auto-promoting composite fields to float64.** Wrapper/constrained
  types store the underlying primitive as `IntVal` or `FloatVal`
  based on where it came from. Mixed-mode arithmetic promotes. If you
  want pure-float semantics per define, use explicit float literals.
- **Full theorem proving.** The spec is the oracle for the sampled
  inputs. That is engineering confidence, not mathematical proof.
  Shen's `tc+` still enforces the datatype-level sequent rules
  separately via the `shen-check` gate.

## Future directions (non-committal)

- **Property-based sampling extensions.** Today the random draws are
  uniform over numeric and alphanumeric ranges. Per-type generators
  (value classes, user-supplied distributions) could catch more bugs.
- **Go native fuzzing as a complement.** Emit both the deterministic
  N-case table for CI and a `func FuzzSpec_Processable` that runs the
  spec evaluator as an oracle under `go test -fuzz`.
- **Cross-spec type references.** Today the `TypeTable` is built from
  a single file. Specs that reference types from sibling files would
  need a multi-file type table.
- **Signature pre-flight check.** `--impl-func` is currently just a
  name; if the Go signature doesn't match the spec's type signature,
  the generated test fails to compile. That's an acceptable failure,
  but a pre-flight check would give a clearer message.

## Historical note

This document replaces the v1 design document (rewrite engine, law
catalog, codegen). The v1 planning artifacts (V1_*.md files,
DONE_CHECKLIST, CORPUS_STATUS, GAP_ANALYSIS) described a different
project and no longer apply. The pivot rationale is in
`thoughts/shared/handoffs/general/2026-04-09_18-13-49_shen-derive-v2-sexpr-pivot.md`
and the commit message at `49e179c`.
