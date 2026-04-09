# shen-derive v1 Corpus Status

**Current (post-execution, aligned with `V1_EXECUTION_PLAN.md`)**. See also `V1_GAP_ANALYSIS.md` for reusable gaps and `V1_DONE_CHECKLIST.md` for completion criteria.

The fixed v1 corpus is now **20 targets**, all covered by the shared end-to-end harness in `codegen/TestV1Corpus` with checked-in generated Go artifacts in `codegen/testdata/corpus/`.

Status meanings:
- `green`: passes the full corpus path `spec -> rewrite if needed -> eval equivalence -> lower -> compile/test -> artifact drift check`
- `yellow`: in-scope candidate not yet corpus-complete
- `red`: blocked by missing reusable machinery
- `drop`: explicitly out of scope for v1

Obligation meanings:
- `none`: unconditional rewrite or direct lowering
- `ground`: can be discharged exactly on closed terms
- `validation-only`: quantified obligation; useful for documentation, but not part of the core v1 stop rule

## Fixed v1 Target Set
The core corpus is frozen at **20 targets**. No further additions without explicit replanning.

### Green Baseline (11)
| Name | Category | Rewrite chain | Obligation | Artifact test | Status | Notes |
|---|---|---|---|---|---|---|
| `sum` | direct fold | none | `none` | yes | `green` | Covered by `TestV1Corpus`; checked-in artifact in `codegen/testdata/corpus/sum.go`. |
| `running-sum` | direct scan | none | `none` | yes | `green` | Stable scan baseline. |
| `squares` | direct map | none | `none` | yes | `green` | Stable map baseline. |
| `filter-positive` | direct filter | none | `none` | yes | `green` | Includes empty and all-negative cases. |
| `all-non-negative` | boolean fold | none | `none` | yes | `green` | Stable reverse-fold boolean check. |
| `sum-positives` | conditional fold | none | `none` | yes | `green` | Exercises branchy fold lowering. |
| `singleton-bool` | lambda/list | none | `none` | yes | `green` | Minimal non-fold function in corpus form. |
| `inc-all-let` | map + let | none | `none` | yes | `green` | Covers let-lowering inside map. |
| `non-neg-flags` | list-producing fold | none | `none` | yes | `green` | Covers `foldr` list construction. |
| `identity-bools` | map + if | none | `none` | yes | `green` | Bool-typed map coverage. |
| `payment-processable` | fused invariant check | `all-scanl-fusion` | `none` | yes | `green` | Still the flagship vertical slice; corpus harness and demo both pass. |

### Promoted Targets (9)
| Name | Category | Rewrite chain | Obligation | Artifact test | Status | Notes |
|---|---|---|---|---|---|---|
| `product` | direct fold | none | `none` | yes | `green` | No new semantics required. |
| `running-product` | direct scan | none | `none` | yes | `green` | Confirms multiplicative scan path. |
| `any-negative` | boolean fold | none | `none` | yes | `green` | Pairs with `all-non-negative`. |
| `count-non-negative` | counting fold | none | `none` | yes | `green` | Confirms accumulator-plus-branch lowering. |
| `balance-final` | projected tuple-state fold | none | `none` | yes | `green` | Confirms `fst` projection path through `foldl`. |
| `balance-state-pair` | tuple-state fold | none | `none` | yes | `green` | Confirms tuple-returning `foldl` path. |
| `map-fusion-basic` | rewrite + lower | `map-fusion` | `none` | yes | `green` | Corpus work exposed and fixed generic unary application lowering inside `map`. |
| `map-inc-then-square` | rewrite + lower | `map-fusion` | `none` | yes | `green` | Same reusable lowering fix as `map-fusion-basic`. |
| `map-foldr-identity` | rewrite + lower | `map-foldr-fusion` | `none` | yes | `green` | Bridge from rewrite law to list-building codegen. |

## Explicitly Deferred From Core v1

### Validation-only appendix candidates
These remain outside the fixed core corpus.

| Name | Rewrite chain | Obligation | Status | Notes |
|---|---|---|---|---|
| `negate-sum` | `foldr-fusion` with `?h` | `validation-only` | `deferred` | Useful for documenting proof boundaries, not for the core stop rule. |
| `double-sum` | `foldr-fusion` with `?h` | `validation-only` | `deferred` | Same rationale as `negate-sum`. |

### Out-of-scope / red candidates
| Name | Blocking gap | Status | Notes |
|---|---|---|---|
| `map-after-filter` | nested combinator lowering beyond the fixed corpus | `red` | Deferred by plan. |
| `filter-after-map` | nested combinator lowering beyond the fixed corpus | `red` | Deferred by plan. |
| `concat-map` | missing flattening law + lowering path | `red` | Deferred by plan. |
| `zip-with-sum` | primitive absent from core language | `red` | Deferred by plan. |
| `take-while-positive` | primitive absent; lowering unclear | `red` | Deferred by plan. |
| `drop-while-positive` | primitive absent; lowering unclear | `red` | Deferred by plan. |

## Stop-Rule Snapshot
Current evidence for the fixed corpus:

- **20/20 core targets** pass the real end-to-end path.
- All core rewrites use named laws from `laws/`.
- All core targets have checked-in generated Go artifacts and drift checks.
- The corpus produced one new reusable bug class during execution: lowering generic unary function applications inside `map`/`filter`. It is now covered by the corpus rewrite targets.
- The main remaining red gap is still **general nested combinator lowering**, which stays deferred because the fixed corpus no longer requires it.

## Current Read
For the fixed v1 corpus, the system is now **green on engineering-done** and still **not proof-done** in the quantified-obligation sense.
