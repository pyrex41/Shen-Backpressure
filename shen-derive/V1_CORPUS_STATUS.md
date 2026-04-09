# shen-derive v1 Corpus Status

**Current (post-execution, aligned with `V1_EXECUTION_PLAN.md`)**. See also `V1_GAP_ANALYSIS.md` for reusable gaps and `V1_DONE_CHECKLIST.md` for completion criteria.

The fixed v1 core corpus remains **20 targets**, all covered by the shared end-to-end harness in `codegen/TestV1Corpus` with checked-in generated Go artifacts in `codegen/testdata/corpus/`.

After the v1 stop rule was satisfied, the planning scope broadened again. This file now tracks a **32-target universe**:

- **20** fixed core v1 targets
- **2** already-working proved appendix targets
- **6** near-term expansion targets reachable with the current language plus small reusable lowering work
- **1** medium-risk expansion target still inside the current core language (`concat-map`)
- **3** explicit new-machinery targets (`zip-with-sum`, `take-while-positive`, `drop-while-positive`)

Status meanings:
- `green`: passes the full corpus path `spec -> rewrite if needed -> eval equivalence -> lower -> compile/test -> artifact drift check`
- `yellow`: in-scope candidate not yet corpus-complete
- `red`: blocked by missing reusable machinery
- `drop`: explicitly out of scope for v1

Obligation meanings:
- `none`: unconditional rewrite or direct lowering
- `ground`: can be discharged exactly on closed terms
- `proved-quantified`: soundly discharged via symbolic proof for quantified integer polynomial obligations in foldr-fusion witnesses (e.g. negate-sum, double-sum)
- `validation-only`: other quantified obligations (heuristic/diagnostic only); useful for documentation, but not part of the core v1 stop rule

## Fixed v1 Target Set
The core v1 corpus remains frozen at **20 targets** for purposes of the v1 done checklist. Expansion candidates are tracked separately below.

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

## Expansion Universe Beyond Core v1

These targets are **not** retroactively part of the fixed 20-target v1 stop rule. They are the next explicit expansion set for broadening coverage after the core corpus reached a truthful green state.

### Already-Working Appendix Targets (2)
These already work today and document the strongest current proof slice.

| Name | Category | Rewrite chain | Obligation | Artifact test | Status | Notes |
|---|---|---|---|---|---|---|
| `negate-sum` | proved appendix | `foldr-fusion` with `?h` | `proved-quantified` | no | `green` | Soundly proved in the current symbolic fragment; remains outside the fixed core because it depends on quantified witness discharge. |
| `double-sum` | proved appendix | `foldr-fusion` with `?h` | `proved-quantified` | no | `green` | Same proof boundary as `negate-sum`; useful as a real end-to-end proved appendix example. |

### Near-Term Expansion Targets (6)
These are the next targets to add if the goal is to cover the broader 20-30-ish intended slice without introducing major new semantics.

| Name | Category | Expected gap | Obligation | Artifact test | Status | Notes |
|---|---|---|---|---|---|---|
| `safe-under-limit` | boolean invariant check | closed in Phase 9 | `none` | yes | `green` | Added as a direct fold-based list/property check using existing integer-comparison and boolean lowering. |
| `prefix-nonzero` | prefix / running-property check | closed in Phase 9 | `none` | yes | `green` | Added as a boolean `scanl` target; confirms running-prefix logic with non-additive accumulators. |
| `equal-zero-flags` | boolean map/comparison | closed in Phase 9 | `none` | yes | `green` | Added as a direct `map` target over integer equality. |
| `string-eq-flags` | string equality map | closed in Phase 9 | `none` | yes | `green` | Added as a string-typed `map` target; confirms end-to-end lowering for captured string equality. |
| `map-after-filter` | nested pipeline | one reusable nested-combinator lowering path | `none` | no | `yellow` | Still waiting on a shared shape-bounded lowering path for nested pipelines. |
| `filter-after-map` | nested pipeline | same reusable nested-combinator lowering path | `none` | no | `yellow` | Still waiting on the same reusable nested-pipeline lowering extension. |

### Existing-Language Stretch Target (1)
This target still fits the current core language, but it likely needs a slightly larger reusable lowering extension than the near-term yellow items.

| Name | Category | Blocking gap | Obligation | Artifact test | Status | Notes |
|---|---|---|---|---|---|---|
| `concat-map` | flattening pipeline | `concat (map ...)` lowering / fused append path | `none` | no | `red` | `concat` exists in the core language and evaluator, but the Go lowerer does not yet treat flattening as a first-class supported path. |

### New-Machinery Track (3)
These are explicitly **language-growth** targets. They are no longer treated as accidental scope creep; they are their own planned machinery workstream.

| Name | Category | Blocking gap | Obligation | Artifact test | Status | Notes |
|---|---|---|---|---|---|---|
| `zip-with-sum` | zipped sequence transform | add `zipWith` primitive, typing, evaluation, lowering, tests | `none` | no | `red` | Best implemented via a new `zipWith` primitive rather than by contorting existing list machinery. |
| `take-while-positive` | prefix extraction | add `takeWhile` primitive, typing, evaluation, lowering, tests | `none` | no | `red` | Requires prefix-sensitive traversal and early termination in lowering. |
| `drop-while-positive` | prefix dropping | add `dropWhile` primitive, typing, evaluation, lowering, tests | `none` | no | `red` | Pairs naturally with `takeWhile`; should be planned together but implemented independently and tested separately. |

## Stop-Rule Snapshot
Current evidence for the fixed corpus:

- **20/20 core targets** pass the real end-to-end path.
- All core rewrites use named laws from `laws/`.
- All core targets have checked-in generated Go artifacts and drift checks.
- The corpus produced one new reusable bug class during execution: lowering generic unary function applications inside `map`/`filter`. It is now covered by the corpus rewrite targets.
- The main remaining red gap inside the broader current-language universe is still **nested / flattening pipeline lowering** (`map-after-filter`, `filter-after-map`, `concat-map`).

## Expansion Snapshot
For the broader 32-target universe:

- **20** fixed core targets are `green`
- **2** appendix targets are already working and honestly proved in the current quantified arithmetic fragment
- **4** direct expansion targets are now `green`
- **2** expansion targets remain `yellow`
- **1** expansion target is `red` but still within the current core language
- **3** targets are `red` because they require explicit new primitives

## Current Read
For the fixed v1 corpus: **green (engineering-done)**. For the broader planned universe: **26 targets already working**, **2 remaining near-term nested-pipeline additions**, **1 current-language stretch target**, and **3 explicit machinery-growth targets**. **Proof-done** remains limited to the supported arithmetic foldr-fusion fragment plus the newly-added boolean/comparison symbolic slice; unsupported quantified obligations still remain **diagnostic-only**.
