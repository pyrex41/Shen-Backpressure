# shen-derive: Design Notes

## Core Product Vision (v1 Solidified)
**shen-derive v1** delivers a **trustworthy, narrowly-scoped derivation engine** for fold-shaped pure functional computations over lists and tuples. It transforms naive specifications into efficient, idiomatic Go using:

- **Equational rewriting** via a catalog of named algebraic laws (Bird-Meertens style, stored in `laws/`)
- **Pattern-based lowering** from a typed lambda calculus AST to Go (in `codegen/`)
- **Honest validation** : engineering confidence via end-to-end tests, evaluator equivalence, compile/run checks, and artifact drift detection. Proof-complete discharge is explicitly *not* required for v1 (validation-only obligations must be labeled as such).

The **fixed 20-target corpus** (see `V1_CORPUS_STATUS.md`, `V1_EXECUTION_PLAN.md`, `V1_GAP_ANALYSIS.md`) acts as the regression suite and scope contract. "Done" is falsifiable per `V1_DONE_CHECKLIST.md`: fixed corpus, real pipeline with no handwritten generated code, named laws only, no open soundness issues in the supported slice, clear docs on boundaries, and the last several targets requiring only small reusable extensions (no new core semantics).

**This is not general program synthesis, not a full theorem prover, and not an open-ended optimizer.** It is a disciplined tool for deriving reliable implementations of common sequence transformations and invariant checks — directly supporting the larger Shen-Backpressure project's goal of *deductive backpressure* via generated guards and processors (e.g. `payment-processable`).

Key principles (repeated across all V1 docs to prevent drift):
- Prefer reusable gaps (laws, lowering patterns, harness) that unlock multiple targets.
- No one-off hacks or manual AST surgery for corpus items.
- Separate "engineering-done" (pipeline works, tests pass, artifacts honest) from "proof-done".
- Runtime helpers (`runtime/`) kept minimal (currently just `Pair` for tuple state).
- Future multi-language: portable core (parser, rewrite engine, laws, evaluator in `core/`, `laws/`, `shen/`) + thin per-language codegen backends.

See `V1_*` files for execution details. The planning and gap analysis enforce these boundaries rigorously.

## Architecture
(Original content follows...)

shen-derive has two layers with different portability characteristics:

### Derivation engine (language-agnostic)

The spec parser, type checker, rewrite engine, law catalog, and
side-condition discharge logic live in `core/`, `laws/`, and `shen/`.
None of these packages know about Go as a target language. They operate
on a typed lambda calculus AST with list/tuple combinators and apply
named algebraic laws from the Bird-Meertens catalog.

### Code generator (language-specific)

`codegen/` lowers the AST to Go source code. This is where
language-specific judgment lives: `foldr` becomes a reverse-iterating
`for` loop, `foldl` becomes `for _, x := range`, `map` becomes a
`make` + indexed loop, etc. Producing idiomatic target code requires
understanding the target language's idioms — these aren't mechanical
translations.

## Near-term direction: make shen-derive excellent as a tool

The v1 corpus proves the engine works. The next step is making it
usable outside of Go test code.

### 1. Spec file format and real CLI

Today every derivation target is constructed as Go AST nodes inside
test files. The parser already handles the surface syntax — the gap is
in the pipeline. A user or LLM should be able to write:

```
-- sum.spec
sum : [Int] -> Int
sum = foldl (+) 0
```

and run `shen-derive derive sum.spec --out sum.go --transcript sum.derivation`.
The `lower` command needs configurable function name and package.
The `rewrite` command needs path specification (the payment-processable
rewrite uses path `{0,0}`, but the CLI only supports root).

### 2. First-class derivation transcripts

The derivation transcript is what differentiates shen-derive from
"just an optimizer." It should be a structured, machine-readable
artifact — not a pretty-printed string from a test helper. It should
record: the original spec, each rewrite step (law name, path, bindings),
obligations and how they were discharged, and the final derived term.

### 3. Law catalog growth

The catalog (currently 4 laws) should grow as real derivation targets
demand it. Obvious candidates: `foldl-fusion`, `foldr-map` (fold after
map), `filter-fusion`, scan laws. Keep the existing discipline: no
laws without corpus targets that exercise them.

### 4. Its own gate pattern

shen-derive and shen-guard are parallel tools for different situations,
not layers in one pipeline. A project might use one, both, or neither.
If a project uses shen-derive, it should have its own gate: regenerate
derived code from specs, drift-check against committed artifacts,
fail the build if they diverge. This is independent of the shen-guard
five-gate pipeline.

## Future direction: multi-language support

The derivation engine (core/, laws/, shen/) is free of Go-specific
assumptions. The code generator (codegen/) is where language knowledge
lives. Multi-language support means adding codegen backends.

The value of shen-derive is producing **idiomatic target code**. A
`foldr` becomes:

- Go: `for i := len(xs)-1; i >= 0; i--`
- Rust: `iter().rev().fold()`
- Python: `functools.reduce` or a comprehension
- TypeScript: `.reduceRight()`

This means codegen must be written by someone who knows the target
language's idioms. Two viable approaches:

1. **AST-as-JSON**: the engine serializes the rewritten AST as JSON,
   thin per-language lowerers consume it. The engine stays in Go (or
   eventually one canonical language). Lowerers are small, independent
   programs in each target language.
2. **Reimplement per language**: each language gets its own engine +
   codegen. The core is ~3600 lines and the abstractions are clear.
   This is more work but avoids serialization boundaries.

The **wrong** approach is porting the engine to Rust and exposing FFI
bindings. The rewrite engine is language-agnostic, but the consumer of
its output needs to understand the AST well enough to lower it — which
means reimplementing the type system on the receiving side. You'd port
3600 lines to Rust to avoid duplicating ~600 lines of codegen per
language. That's backwards. FFI also makes the codegen hostile to
contributors who work in the target language.

**Current guidance:** keep iterating in Go. The engine API is still
stabilizing (4 laws, pattern-based lowering, heuristic-only discharge
for most quantified obligations). When a second language target becomes
a real need, evaluate AST-as-JSON vs. reimplementation based on what
the API looks like at that point.

## Package layout

```
shen-derive/
  core/       Typed lambda calculus AST, parser, evaluator, type checker
  laws/       Rewrite rule catalog, pattern matching, substitution
  shen/       Shen runtime bridge for side-condition validation
  codegen/    Go code generator (the only Go-specific package)
  runtime/    Helper types for generated Go code
  demo/       End-to-end derivation examples
  main.go     CLI entry point
```
