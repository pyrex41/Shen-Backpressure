# shen-derive v1 Execution Plan

This is the standalone execution plan for getting `shen-derive` to **v1 engineering-done**.

It intentionally optimizes for:

- a fixed scope,
- a trustworthy end-to-end pipeline,
- reusable coverage,
- and a clear stopping point.

It does **not** try to make `shen-derive` universally complete.

The fixed 20-target v1 stop rule is already satisfied. The first half of this file preserves that core definition of done; the later sections record the explicit post-v1 expansion roadmap toward a broader, still-bounded target universe.

## v1 Goal

Ship a stable `shen-derive` v1 that can handle a fixed corpus of **20 target functions** in the intended slice:

- fold-shaped pure computations,
- sequence transforms and checks,
- tuple-state accumulators,
- map/filter/fold/scan patterns,
- named rewrites that lower to Go with no handwritten replacement code.

The bar is **engineering-done**, not proof-complete.

## Scope Decision

Freeze the **core v1 corpus at 20 targets**.

Do **not** include major new primitives or nested-combinator generality in v1 unless the fixed corpus cannot be completed without them.

## Fixed Core Corpus

These are the 20 execution targets for v1.

### Green Baseline Targets

These should all end up with explicit corpus artifact tests.

1. `sum`
2. `running-sum`
3. `squares`
4. `filter-positive`
5. `all-non-negative`
6. `sum-positives`
7. `singleton-bool`
8. `inc-all-let`
9. `non-neg-flags`
10. `identity-bools`
11. `payment-processable`

### Yellow Targets To Promote

These are in-scope for v1 and should be promoted unless evidence shows they need major new machinery.

12. `product`
13. `running-product`
14. `any-negative`
15. `count-non-negative`
16. `balance-final`
17. `balance-state-pair`
18. `map-fusion-basic`
19. `map-inc-then-square`
20. `map-foldr-identity`

## Explicitly Deferred From Core v1

These were useful, but not required for the fixed v1 core corpus.

### Already-working appendix track

- `negate-sum`
- `double-sum`

These were useful for documenting proof boundaries, but they did not need to block the main v1 execution path.

### Post-v1 expansion track (current language / small reusable extensions)

- `safe-under-limit`
- `prefix-nonzero`
- `equal-zero-flags`
- `string-eq-flags`
- `map-after-filter`
- `filter-after-map`
- `concat-map`

### Post-v1 new-machinery track

- `zip-with-sum`
- `take-while-positive`
- `drop-while-positive`

These are no longer treated as accidental scope creep. They are the next explicit workstreams after the fixed v1 core.

## Non-Negotiable v1 Rules

These rules apply to all implementation work in this plan.

- Every corpus function must run through the real path: spec -> rewrite if needed -> lower -> compile/test.
- No checked-in generated file may be handwritten and labeled as generated.
- Every rewrite in a corpus item must use a named law from `laws/`.
- Supplemental rewrite bindings must not override LHS matches.
- Generated artifacts must be drift-checked.
- Unsupported quantified obligations must be labeled explicitly as diagnostic-only / not formally proved.
- Every new bug class found during execution must get a regression test before the task is considered complete.

## Phase Plan

## Phase 1: Freeze Corpus And Test Harness

### Objective

Turn the chosen 20 targets into a real tracked execution set with a uniform harness shape.

### Outputs

- finalize `V1_CORPUS_STATUS.md` to match the fixed 20-target corpus
- add a dedicated corpus tracking section or file if needed
- define a standard per-target test pattern:
  - naive spec
  - optional rewrite chain
  - evaluator equivalence checks
  - Go lowering
  - compile/run generated Go
  - artifact drift check

### Exit Criteria

- the 20 core targets are frozen
- no target has ambiguous status
- every target has an owner category and expected path

### Risk

Low.

## Phase 2: Convert Green Targets Into Corpus Tests

### Objective

Turn the current green examples into explicit corpus-grade tests instead of relying on scattered unit coverage.

### Outputs

- end-to-end corpus tests for all 11 green targets
- checked-in generated artifacts where appropriate
- drift checks for those artifacts
- shared helper utilities for corpus execution to avoid test duplication

### Exit Criteria

- all 11 green targets are green by the v1 checklist, not just "probably supported"
- no green target depends on handwritten generated code

### Risk

Low to medium.

### Notes

This phase should focus on harness quality and consistency, not new semantics.

## Phase 3: Promote Low-Risk Yellow Targets

### Objective

Promote the likely-supported non-rewrite and simple tuple-state examples first.

### Target Set

- `product`
- `running-product`
- `any-negative`
- `count-non-negative`
- `balance-final`
- `balance-state-pair`

### Outputs

- focused lower/evaluator tests
- corpus end-to-end tests
- artifact checks where useful

### Exit Criteria

- these six targets are either green or explicitly reclassified with a concrete blocker

### Risk

Medium.

### Notes

If any of these require major new semantics, stop and document the blocker before continuing.

## Phase 4: Promote Rewrite-Derived Yellow Targets

### Objective

Promote the rewrite-dependent examples that should fit the current law catalog.

### Target Set

- `map-fusion-basic`
- `map-inc-then-square`
- `map-foldr-identity`

### Outputs

- per-target rewrite chain tests
- semantic equivalence checks between original and rewritten terms
- lowered Go compilation tests for rewritten output
- artifact drift checks if checked-in generated files are added

### Exit Criteria

- all three are green, or a single reusable blocker is documented clearly

### Risk

Medium.

### Notes

This phase should not introduce broad new laws unless multiple corpus items are blocked.

## Phase 5: Close Documentation And Convention Gaps

### Objective

Make the system's scope, confidence level, and operational conventions explicit.

### Outputs

- tighten docs around engineering-done vs proof-done
- document the proved arithmetic fragment and the remaining diagnostic-only quantified obligations clearly
- document corpus conventions
- document artifact regeneration expectations
- ensure demos are honest about proof strength

### Exit Criteria

- proof-strength boundaries are unambiguous
- no demo or doc suggests heuristic discharge is a proof

### Risk

Low.

## Phase 6: Final Verification And Stop Check

### Objective

Run the formal stop rule for v1.

### Outputs

- final corpus status review
- final gap review
- final test/race run logs
- explicit decision: `done`, `done with exclusions`, or `not done`

### Exit Criteria

All of the following are true:

1. the 20-target core corpus is fixed and documented
2. every corpus target passes the full end-to-end path
3. no open rewrite soundness bugs remain in the supported slice
4. no handwritten generated artifacts remain
5. `go test ./...` and `go test -race ./...` pass for `shen-derive`
6. nested demo modules still pass `go test ./...` and `go test -race ./...`
7. every bug class found during corpus work has a regression test
8. documentation clearly distinguishes engineering confidence from proof confidence

### Risk

Low if prior phases were done honestly.

## Optional Phase 7: Proved Appendix (Arithmetic Witnesses)

### Objective

Add a small, explicitly labeled appendix for arithmetic `foldr-fusion` witness cases that are now soundly proved in the supported symbolic fragment, without expanding the fixed core corpus.

### Candidate Targets

- `negate-sum`
- `double-sum`

### Outputs

- tests and docs that mark `negate-sum` and `double-sum` as proved appendix examples
- no claim that unsupported quantified obligations are proof-complete

### Exit Criteria

- these examples are either added as labeled proved appendix material or left out entirely

### Risk

Low if documentation is honest; high if they are allowed to blur the proof boundary.

## Phase 8: Promote The Proved Appendix Into The Tracked Expansion Universe

### Objective

Treat the two already-working appendix examples as first-class tracked expansion targets without pretending they were part of the fixed v1 core corpus.

### Target Set

- `negate-sum`
- `double-sum`

### Outputs

- explicit expansion-universe rows for both examples
- focused end-to-end appendix tests kept honest about proof strength
- optional artifact checks if promoting them into the shared corpus harness becomes worthwhile

### Exit Criteria

- both targets are clearly tracked as working examples
- proof labeling remains `proved-quantified`
- no doc implies that broader quantified obligations are solved in general

### Risk

Low.

## Phase 9: Add Direct Current-Semantics Expansion Targets

### Objective

Broaden coverage using examples that should fit the current core language and existing lowerer patterns with little or no semantic work.

### Target Set

- `safe-under-limit`
- `prefix-nonzero`
- `equal-zero-flags`
- `string-eq-flags`

### Outputs

- naive specs for each target
- focused evaluator / lowerer tests
- corpus-grade end-to-end tests
- checked-in generated artifacts and drift checks where appropriate

### Exit Criteria

- these four targets are either green or reclassified with a concrete blocker
- no one-off hacks are required to make them pass

### Risk

Low to medium.

## Phase 10: Add One Reusable Nested-Combinator Lowering Extension

### Objective

Unlock the most common next step beyond the fixed core corpus without attempting a fully general nested-combinator compiler.

### Target Set

- `map-after-filter`
- `filter-after-map`

### Outputs

- one shared lowering extension in `codegen/` for the targeted nested-pipeline shapes
- focused regression tests for both supported shapes and unsupported nearby shapes
- corpus-grade end-to-end tests and artifacts for both targets

### Exit Criteria

- both targets are green
- the implementation is clearly reusable and shape-bounded
- unsupported broader nesting still fails honestly rather than falling through unsafely

### Risk

Medium.

### Notes

Prefer shape recognition such as `map f (filter p xs)` and `filter p (map f xs)` over any attempt at broad optimizer-style normalization.

## Phase 11: Add Flattening Support For `concat-map`

### Objective

Support the smallest useful flattening case inside the current language without committing to general list-monad machinery.

### Target Set

- `concat-map`

### Outputs

- either a dedicated lowering path for `concat (map f xs)` or a clearly bounded reusable flattening helper
- focused tests for empty, singleton, and multi-element expansion cases
- end-to-end target coverage with artifact drift check

### Exit Criteria

- `concat-map` is green, or the blocker is documented precisely enough to justify continued deferral

### Risk

Medium.

## Phase 12: New-Machinery Track For Three Additional Sequence Primitives

### Objective

Expand the core language deliberately rather than incidentally, adding exactly the machinery needed for three common sequence operators.

### Target Set

- `zip-with-sum`
- `take-while-positive`
- `drop-while-positive`

### Outputs

- new primitives in the core AST / parser / pretty-printer
- typechecker support
- evaluator support
- focused Go lowering support
- one corpus-grade target per primitive
- regression tests for boundary behavior

### Exit Criteria

- each primitive has a complete vertical slice: parse -> typecheck -> eval -> lower -> compile/test
- the implementation remains minimal and does not drag in speculative extra semantics

### Risk

Medium.

### Primitive-by-primitive plan

1. `zipWith`
   - add `PrimZipWith`
   - type: `(a -> b -> c) -> [a] -> [b] -> [c]`
   - eval: iterate to the shorter input
   - lower: index loop over both slices
   - first target: `zip-with-sum`

2. `takeWhile`
   - add `PrimTakeWhile`
   - type: `(a -> Bool) -> [a] -> [a]`
   - eval: consume prefix while predicate holds
   - lower: single loop with early break
   - first target: `take-while-positive`

3. `dropWhile`
   - add `PrimDropWhile`
   - type: `(a -> Bool) -> [a] -> [a]`
   - eval: skip prefix while predicate holds
   - lower: locate first failing index, then return suffix
   - first target: `drop-while-positive`

## What Not To Do During v1 Execution

Do not spend v1 time on:

- general nested-combinator support beyond what the fixed corpus forces
- new non-list primitives like `zipWith`, `takeWhile`, or `dropWhile`
- proof-complete quantified obligation discharge
- generalized optimization or optimizer-style benchmark chasing
- cosmetic "idiomatic Go" cleanup unless it affects correctness, trust, or maintainability

## Main Risk Register

### Risk 1: Corpus creep

Symptom:

- new interesting examples keep getting added

Response:

- refuse additions unless they replace an existing corpus target or unblock a real gap

### Risk 2: One-off fixes masquerading as progress

Symptom:

- a target only passes after ad hoc handling specific to that example

Response:

- either generalize the fix or reclassify the target out of v1

### Risk 3: Validation-only ambiguity

Symptom:

- docs or demos imply quantified obligations are proved

Response:

- label them explicitly or remove them from the main corpus

### Risk 4: Harness drift

Symptom:

- examples pass in isolated tests but not in corpus form

Response:

- consolidate around one corpus-grade harness pattern

## Success Definition

This plan succeeds when:

- the 20-target corpus is real and stable,
- almost all work happened by promoting targets through reusable support,
- failures stopped being new categories and became ordinary regressions,
- and the system's proof limits are documented clearly enough that users cannot confuse engineering confidence with formal proof.

For the post-v1 expansion roadmap, success means:

- the repo distinguishes clearly between the finished 20-target core and the broader 32-target universe
- the next 6 near-term targets land through small reusable changes rather than heroics
- `concat-map` is either supported honestly or documented as a still-real blocker
- the 3 new-machinery targets are implemented as deliberate primitive additions, not accidental backend hacks
