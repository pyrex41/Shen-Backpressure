# shen-derive v1 Gap Analysis

This document groups work by **reusable gap** rather than by individual target. It began as the v1 gap analysis; it now also records the post-v1 expansion gaps for the broader tracked target universe.

## Current Corpus Summary
- **Fixed v1 corpus**: 20 targets (11 green baseline + 9 yellow to promote)
- **Strongest existing slice**: `payment-processable` (uses `all-scanl-fusion`, tuple-state foldl, projection)
- **Green baseline coverage**: Good for direct map/filter/fold/scan, boolean folds, let-bindings, simple list producers.
- **Yellow coverage goals**: Additional accumulators (`product`, `count-non-negative`), scans, tuple-state (projected and full), and rewrite chains (`map-fusion`, `map-foldr-fusion`).
- **Post-v1 tracked universe**: 32 total targets = 20 fixed core + 2 already-working appendix targets + 6 near-term expansion targets + 1 existing-language stretch target + 3 new-machinery targets.

## Reusable Gaps

### 1. Corpus Test Harness & Conventions (v1-critical, high unlock count)
**Why it blocks**: Most "green" targets rely on scattered unit tests (`TestLowerFoldlSum` etc.) rather than uniform end-to-end corpus tests with naive-eval vs derived-eval equivalence, lowering, compile/run, and artifact drift checks.
**Unlocks**: All 20 targets. Turns "probably works" into "verified by regression corpus".
**Status**: Closed for the fixed core corpus. `codegen/TestV1Corpus` now covers all 20 targets, and `codegen/testdata/corpus/` holds the checked-in generated Go artifacts.
**Recommended action**: Keep new examples out of the fixed corpus unless they replace an existing target or close a reusable gap the corpus truly needs.
**Exit**: Achieved for v1 core corpus.

### 2. Law Catalog Coverage for Fusion (v1-critical for rewrite yellows)
**Why it blocks**: `map-fusion-basic`, `map-inc-then-square`, `map-foldr-identity` require `map-fusion`, `map-foldr-fusion` (and supporting laws like foldr as reduce).
**Unlocks**: The 3 rewrite-derived targets + future map/fold compositions. Also supports payment-style scanl-all fusion.
**Status**: Closed for the fixed core corpus. The existing law catalog was enough for all rewrite-derived core targets, and the corpus harness now verifies end-to-end equivalence and lowering after rewrite.
**Recommended action**: Keep the catalog corpus-driven. Do not add laws just because they are interesting.
**Note**: This aligns with core vision — named laws from Bird-Meertens-style catalog only. No ad-hoc rewrites.

### 3. Lowering Pattern Coverage (medium risk)
**Why it blocks**: 
- Tuple-state folds and projections (`balance-final`, `balance-state-pair`) — `bodyProjectedFoldl` and Pair runtime helper seem to cover.
- Non-additive primitives (`product`, `running-product`).
- Boolean accumulators with short-circuit (`any-negative`).
- Rewrite output lowering (post-fusion single loop).
**Unlocks**: The 6 low-risk yellow + rewrite targets.
**Status**: Closed for the fixed core corpus. Corpus execution promoted all nine yellow targets. One new reusable bug class was found and fixed during this phase: generic unary function applications inside `map`/`filter` must lower by lowering the actual application term, not by blindly rendering `fn(arg)` as strings.
**Recommended action**: Keep general nested-combinator lowering out of v1. The corpus no longer needs it.
**Alternative hypothesis (contrarian)**: Nested combinators like `map (filter ...)` are extremely common in practice. If the pattern-based approach proves too limiting even for the fixed corpus, reconsider a small extension to lowering for common nesting (e.g. recognize `map f (filter p xs)` shape). However, this risks scope creep; only do if corpus forces it. Current plan correctly defers it.

### 4. Obligation & Proof Boundary Handling (acceptable for engineering-done)
**Why it blocks**: Quantified obligations outside the arithmetic polynomial fragment (e.g., non-polynomial ?h witnesses remain diagnostic-only).
**Unlocks**: Clear documentation of engineering vs proof confidence.
**Status**: Closed for arithmetic polynomial witnesses via symbolic proof path (ec6ba79). General quantified obligations outside this fragment remain diagnostic-only.
**Recommended action**: `negate-sum`/`double-sum` promoted to proved appendix examples in `V1_CORPUS_STATUS.md`. Keep explicit boundaries: arithmetic fragment proved, others diagnostic-only (no formal proof claim).
**Core vision alignment**: Separating these two definitions of done is one of the strongest aspects of the planning. Do not blur them.

### 5. Documentation, Conventions & Observability (v1-critical)
**Why it blocks**: Conventions not yet enforced by tests/docs (artifact regeneration, no one-off hacks, named laws only, honest demos).
**Unlocks**: All future work, prevents regression to heroics.
**Recommended action**: Phase 5 updates. Add to DESIGN.md or top-level docs:
- Clear statement of what shen-derive *is* (equational derivation + idiom-aware lowering for fold-like programs) and *is not* (general program synthesis, full theorem prover).
- Link to larger Shen-Backpressure vision: this enables reliable, performant backpressure guards/processors.
- Corpus conventions: how to add a new target (must fit existing patterns or justify new reusable gap).
- Runtime contract: generated code may depend on `runtime/` helpers (currently minimal `Pair`); keep this contract narrow.
**Creative reframing**: The docs are not just commentary — they are the "social proof" layer that makes the technical corpus trustworthy. They turn the 20 targets from isolated examples into a coherent *contract* between spec, laws, lowering, and runtime.

### 6. Test Matrix & Regression Discipline
**Why it blocks**: New bug classes must get regression tests.
**Unlocks**: Stability signal ("failures are now ordinary regressions, not new categories").
**Recommended action**: Per stop rule in DONE_CHECKLIST. Extend `laws_test.go`, `lower_test.go`, corpus tests with negative cases, boundary cases.

## Post-v1 Expansion Gaps

### 7. Expansion Harness Promotion (high unlock count)
**Why it blocks**: The current shared harness enforces `20` fixed core targets. The broader 32-target universe is documented, but not yet represented as a tracked execution set.
**Unlocks**: All 12 post-v1 targets.
**Recommended action**: Keep the fixed 20-target core intact for the v1 done checklist, but add an explicit expansion-target layer or second harness rather than pretending the broader universe is still "out of scope."

### 8. Direct Current-Semantics Coverage (low risk, 4 unlocks)
**Why it blocked**: `safe-under-limit`, `prefix-nonzero`, `equal-zero-flags`, and `string-eq-flags` were planned but not yet captured as real end-to-end targets.
**Unlocks**: 4 additional targets with little or no semantic growth.
**Status**: Closed. A separate expansion harness now covers all four, with checked-in generated artifacts under `codegen/testdata/expansion/`.
**Recommended action**: Treat this as the template for future post-v1 direct targets: real pipeline, checked artifacts, no one-off lowering hacks.

### 9. Nested-Combinator Lowering (medium risk, 2 unlocks)
**Why it blocks**: `map-after-filter` and `filter-after-map` are common, valuable next-step examples, but the Go lowerer currently recognizes only top-level `map`, `filter`, `foldr`, `foldl`, projected `foldl`, and `scanl` shapes.
**Unlocks**: 2 high-value targets and a more honest statement about practical compositional support.
**Recommended action**: Add one shape-bounded nested lowering path for `map f (filter p xs)` and `filter p (map f xs)`. Do not expand this into a general optimizer.

### 10. Flattening / `concat` Lowering (medium risk, 1 unlock)
**Why it blocks**: `concat` exists in the core language and evaluator, but flattening is not yet a first-class Go lowering path.
**Unlocks**: `concat-map`.
**Recommended action**: Support the smallest useful flattening case, ideally `concat (map f xs)`, with explicit tests. Avoid introducing monadic/generalized flattening semantics unless more than one target requires it.

### 11. Primitive Growth Track (medium risk, 3 unlocks)
**Why it blocks**: `zip-with-sum`, `take-while-positive`, and `drop-while-positive` require new language primitives, parser support, typechecker rules, evaluator behavior, and Go lowering.
**Unlocks**: 3 additional targets and a cleaner path for common sequence operations.
**Recommended action**: Add each primitive as a full vertical slice (`AST -> parse -> print -> typecheck -> eval -> lower -> tests`) and keep the implementations minimal. Do not bundle them into a general sequence-library expansion.

## v1-Critical vs Optional
- **Must close for v1**: Harness (1), law coverage for chosen corpus (2), lowering for chosen corpus (3), docs/conventions (5), regression tests.
- **Explicitly optional/deferred**: General nested lowering, proof-complete discharge, additional yellows, beautification.
- **Drop**: Anything that teaches no reusable pattern or requires one-off hacks.

## Post-v1 Priorities
- **Near-term**: Expansion harness promotion (7), nested-combinator lowering (9).
- **Second wave**: Flattening / `concat` lowering (10).
- **Separate machinery track**: Primitive growth for `zipWith`, `takeWhile`, `dropWhile` (11).

## Alignment with Core Vision (@PLAN.md / DESIGN.md)
This analysis reinforces:
- **Fixed core first, explicit expansion second**: the 20-target core remains the v1 done contract, but the broader universe is now planned explicitly rather than hand-waved.
- **Reusable over heroic**: every task should still be phrased in terms of gaps that unlock *multiple* targets.
- **Honest boundaries**: the proved symbolic fragment is still separated clearly from the remaining diagnostic-only quantified cases.
- **Engineering-done first**: pipeline + artifacts + tests before perfect proofs.
- **Deliberate language growth**: new primitives are acceptable when planned as narrow, explicit vertical slices rather than sneaking in through one-off demos.

If during expansion we find a gap that unlocks multiple targets and stays small/reusable, promote it. If it turns into speculative semantics or one-off handling, document and defer it.

This gap analysis should be kept up-to-date alongside `V1_CORPUS_STATUS.md` and `V1_EXECUTION_PLAN.md`. It makes "done" falsifiable.

Last updated: 2026-04-09
