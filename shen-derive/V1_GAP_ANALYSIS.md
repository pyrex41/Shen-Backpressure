# shen-derive v1 Gap Analysis

This document groups the remaining work for v1 by **reusable gap** rather than by individual corpus target. This follows the planning handoff directive and supports the core vision of preferring reusable unlocks over one-off fixes.

## Current Corpus Summary
- **Fixed v1 corpus**: 20 targets (11 green baseline + 9 yellow to promote)
- **Strongest existing slice**: `payment-processable` (uses `all-scanl-fusion`, tuple-state foldl, projection)
- **Green baseline coverage**: Good for direct map/filter/fold/scan, boolean folds, let-bindings, simple list producers.
- **Yellow coverage goals**: Additional accumulators (`product`, `count-non-negative`), scans, tuple-state (projected and full), and rewrite chains (`map-fusion`, `map-foldr-fusion`).

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
**Why it blocks**: Validation-only cases (`negate-sum`, `double-sum` using witness `?h` in foldr-fusion).
**Unlocks**: Clear documentation of engineering vs proof confidence.
**Status**: Explicitly deferred to optional appendix. Quantified obligations remain validation-only in general (heuristic via shen/ bridge?).
**Recommended action**: Keep out of main corpus. In docs and any appendix, **explicitly label as "validation-only / engineering confidence only — not a formal proof"**. This is non-negotiable per DONE_CHECKLIST and all handoffs to prevent misleading users about soundness.
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

## v1-Critical vs Optional
- **Must close for v1**: Harness (1), law coverage for chosen corpus (2), lowering for chosen corpus (3), docs/conventions (5), regression tests.
- **Explicitly optional/deferred**: General nested lowering, proof-complete discharge, additional yellows, beautification.
- **Drop**: Anything that teaches no reusable pattern or requires one-off hacks.

## Alignment with Core Vision (@PLAN.md / DESIGN.md)
This analysis reinforces:
- **Fixed corpus over open-ended**: 20 targets only.
- **Reusable over heroic**: Every task phrased in terms of gaps that unlock *multiple* targets.
- **Honest boundaries**: Validation-only clearly separated.
- **Engineering-done first**: Pipeline + artifacts + tests before perfect proofs.
- **No scope creep**: Red candidates like `map-after-filter`, `concat-map`, `takeWhile` explicitly out unless forced.

If during execution we find a gap that blocks >3 corpus items and is small/reusable, it may be promoted into v1. Otherwise, reclassify the affected targets.

This gap analysis should be kept up-to-date alongside `V1_CORPUS_STATUS.md` and `V1_EXECUTION_PLAN.md`. It makes "done" falsifiable.

Last updated: 2026-04-09
