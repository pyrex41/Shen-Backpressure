# shen-derive v1 Done Checklist

This file defines "done" for `shen-derive` v1.

The goal is not "no more bugs forever." The goal is:

- a clearly scoped family of target functions,
- an honest end-to-end pipeline for that family,
- stable behavior under new examples,
- and explicit boundaries around what is and is not proved.

## v1 Scope

`shen-derive` v1 is done when it can reliably handle the fixed corpus of **20 target functions** defined in `V1_CORPUS_STATUS.md` and `V1_EXECUTION_PLAN.md`, within the intended problem class:

- fold-shaped pure computations,
- sequence transforms and checks,
- list/tuple accumulator patterns,
- map/filter/fold/scan style laws,
- derivations that lower to runnable Go without hand-editing generated code.

v1 is not "general program synthesis."

## Two Definitions Of Done

There are two useful completion bars. Keep them separate.

### 1. Engineering Done

Engineering-done means every target function in the corpus:

- is expressible in the current core language,
- rewrites using named laws only,
- lowers to Go through the real pipeline,
- compiles and passes focused tests,
- has checked-in generated artifacts that match regeneration,
- needs no manual AST surgery or handwritten replacement code.

### 2. Proof Done

Proof-done means, in addition:

- no target function depends on quantified obligations that are only heuristically checked,
- every obligation is either unconditional or soundly discharged.

Today, `shen-derive` is closer to engineering-done than proof-done.

## Corpus Rules

Keep the target corpus fixed at **20 functions** for v1.

Each target function must have:

- a short name,
- a naive source specification,
- its intended derivation pattern,
- its expected output type,
- at least 3 focused examples,
- one end-to-end regeneration check.

The canonical implementation of those checks is the shared corpus harness in `codegen/TestV1Corpus`, with checked-in generated Go artifacts in `codegen/testdata/corpus/`.

Each target should fit one of a small number of reusable patterns, for example:

- `map . map`
- `map . foldr cons nil`
- `f . foldr g e`
- `all p . scanl f e`
- projected tuple-state `foldl`
- accumulator-preserving filter/map combinations

If a target requires a brand new pattern, that is fine.
If too many targets require one-off patterns, v1 is not done yet.

## Required Conventions

These conventions are part of "done":

- Every rewrite used in a demo or corpus example must be a named law in `laws/`.
- No checked-in generated Go may be handwritten and labeled as generated.
- Every demo must execute the real path: spec -> rewrite -> lower -> compile/test.
- Every new bug class gets a regression test before closing the issue.
- Supplemental metavariable bindings must never override matched bindings.
- Generated artifacts must be drift-checked against regeneration.
- If a derivation is only validation-only, that must be stated explicitly in the transcript and docs.

## Required Test Layers

v1 does not need huge test volume. It does need the right layers.

### A. Law Tests

For each law:

- match succeeds on intended shapes,
- match rejects wrong shapes,
- rewrite output is structurally what you expect,
- semantic equivalence is checked on representative inputs,
- bug regressions are preserved.

### B. Codegen Tests

For each supported lowering pattern:

- lower from AST,
- compile generated Go in a temp module,
- run tests against the compiled result,
- cover at least one non-happy-path case.

### C. Corpus Tests

For each corpus function:

- evaluate naive spec,
- rewrite to derived term,
- evaluate derived term,
- lower to Go,
- compile generated Go,
- confirm generated artifact matches checked-in artifact.

### D. Boundary Tests

You need explicit tests for:

- unsupported rewrites,
- stale checked-in artifacts,
- invalid supplemental bindings,
- negative and empty-input edge cases,
- type mismatches,
- failure modes that should remain failure modes.

## Stop Rule

You can call v1 done when all of the following are true:

1. The target corpus is fixed and documented.
2. Every corpus function passes the full end-to-end pipeline.
3. The last 5-10 added corpus functions required no new core semantics.
4. The last 5 added corpus functions required at most small reusable law or codegen extensions.
5. There are no open soundness bugs in rewrite application.
6. There are no handwritten "generated" artifacts left.
7. `go test ./...` and `go test -race ./...` pass for both `shen-derive` and any nested demo modules.
8. Every known bug found during the corpus build-out has a regression test.
9. The docs clearly distinguish proof-strength from engineering confidence.

## Red / Yellow / Green Status

Use this to decide where the project stands.

### Red

- New examples frequently expose new classes of bugs.
- Demos contain manual steps.
- Generated artifacts are not trusted.
- Rewrite soundness is still in doubt.

### Yellow

- Vertical slices work.
- Most bugs are edge cases, not architecture failures.
- New examples still sometimes force new lowering patterns or law shapes.
- Quantified obligations are still validation-only.

### Green

- Corpus is complete.
- New examples mostly fit existing patterns.
- Failures are ordinary regressions, not new categories.
- Artifacts regenerate cleanly.
- Scope and proof boundaries are explicit and stable.

## What Is Not Required For v1

You do not need all of the following for v1:

- a full theorem prover for quantified obligations,
- universal support for arbitrary higher-order derivations,
- perfect prettiness of generated Go,
- every Bird-Meertens law,
- proof-complete automation for all possible programs.

Those are v2+ goals unless they block the fixed corpus.

## Current Read Of The System

As of now:

- Green: the fixed 20-target corpus exists and is explicit.
- Green: every core corpus target passes the full end-to-end path in `TestV1Corpus`.
- Green: checked-in generated Go artifacts are drift-checked for every core target.
- Green: rewrite binding override hole is fixed.
- Green: the payment demo remains an honest end-to-end vertical slice.
- Yellow: lowering is still pattern-based rather than general, but the fixed corpus no longer requires broader machinery.
- Yellow: law catalog is intentionally small and corpus-driven.
- Yellow: generated Go is correct but not always idiomatic.
- Red for proof-complete ambitions: quantified obligations are still not soundly discharged.

## Recommended Next Step

Keep the fixed corpus frozen and use the stop rule as the only bar for additional work.

For any future candidate example, record:

- `name`
- `spec shape`
- `rewrite chain`
- `needs new law?`
- `needs new lowerer pattern?`
- `obligation status: none | ground | validation-only`
- `artifact test present?`
- `status: red | yellow | green`

Only admit it if it replaces an existing corpus target or closes a reusable gap that the fixed corpus actually needs.
