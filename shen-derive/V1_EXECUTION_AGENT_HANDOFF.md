# shen-derive v1 Execution Agent Handoff

This handoff is for the next agent that will execute the v1 plan.

Read these files first:

1. `V1_DONE_CHECKLIST.md`
2. `V1_CORPUS_STATUS.md`
3. `V1_EXECUTION_PLAN.md`

Treat `V1_EXECUTION_PLAN.md` as the source of truth for what to do next.

## Mission

Execute the fixed v1 plan for `shen-derive` until the core 20-target corpus is engineering-done, or until a concrete blocker forces reclassification of one or more targets.

Your job is not to broaden the system.
Your job is to make the fixed plan true.

## Core Scope

The v1 core corpus is fixed at 20 targets:

- `sum`
- `running-sum`
- `squares`
- `filter-positive`
- `all-non-negative`
- `sum-positives`
- `singleton-bool`
- `inc-all-let`
- `non-neg-flags`
- `identity-bools`
- `payment-processable`
- `product`
- `running-product`
- `any-negative`
- `count-non-negative`
- `balance-final`
- `balance-state-pair`
- `map-fusion-basic`
- `map-inc-then-square`
- `map-foldr-identity`

Do not expand this set unless explicitly approved.

## Out Of Scope Unless Forced By The Corpus

Do not proactively work on:

- general nested-combinator lowering
- new primitives like `zipWith`, `takeWhile`, `dropWhile`
- proof-complete quantified obligation discharge
- v2-style generality work
- beautifying generated Go just because it could look nicer

If one of these becomes necessary to finish the fixed corpus, stop and document the exact blocker first.

## Current Known Truths

Assume these are true unless you find evidence otherwise:

- rewrite soundness bug from supplemental binding override is fixed
- payment demo is a real end-to-end slice
- payment artifacts are drift-checked
- `go test ./...` and `go test -race ./...` pass in `shen-derive`
- `go test ./...` and `go test -race ./...` pass in `shen-derive/demo/payment-derived`
- quantified obligations are now soundly proved only for the supported arithmetic `foldr-fusion` fragment; all other quantified obligations remain diagnostic-only

## Execution Priorities

Follow the phases in `V1_EXECUTION_PLAN.md`.

In practical terms, this means:

1. freeze the corpus and harness shape
2. convert all green targets into explicit corpus-grade tests
3. promote the low-risk yellow targets
4. promote the rewrite-derived yellow targets
5. tighten docs and conventions
6. run the stop check

Do not skip straight to speculative red items.

## Rules While Executing

- Every corpus function must use the real path: spec -> rewrite if needed -> lower -> compile/test.
- No handwritten "generated" files are allowed.
- Every rewrite used in the corpus must be a named law in `laws/`.
- Every new bug class must get a regression test before it is considered fixed.
- Examples outside the supported proof fragment must be labeled explicitly as diagnostic-only / not formally proved.
- If a target only works because of a one-off hack, it is not green.

## How To Treat Failures

When a target fails, classify the failure before writing code.

Use one of these buckets:

- missing corpus harness only
- missing reusable law coverage
- missing reusable lowering support
- core language limitation
- doc / artifact convention gap
- out-of-scope / should be dropped

If the failure is a new bug class, add a regression test.
If the failure is a one-off need, prefer dropping or reclassifying the target instead of distorting v1.

## Required Outputs During Execution

Keep these files updated as you work:

- `V1_CORPUS_STATUS.md`
- `V1_EXECUTION_PLAN.md` if sequencing changes materially

If you discover reusable blockers, add:

- `V1_GAP_ANALYSIS.md` if it does not exist yet

If you convert a target to green, update its status explicitly.

## First Concrete Tasks

Start here:

1. Confirm the corpus-grade harness pattern to use for all targets.
2. Promote the 11 green targets into explicit corpus tests with artifact checks where appropriate.
3. Only after that, move to:
   - `product`
   - `running-product`
   - `any-negative`
   - `count-non-negative`
   - `balance-final`
   - `balance-state-pair`
4. Then move to rewrite-derived targets:
   - `map-fusion-basic`
   - `map-inc-then-square`
   - `map-foldr-identity`

## Decision Rules

Use these decision rules while executing:

### Promote

Promote a target to `green` only if it:

- has a real naive spec
- uses a real rewrite chain when needed
- lowers through the real codegen
- compiles and passes focused tests
- has honest artifact handling

### Reclassify

Reclassify a target to `red` if it needs major new machinery.

### Drop

Drop a target from v1 if:

- it teaches nothing reusable
- it needs speculative new semantics
- it encourages scope creep

## Stop Condition

You are done when the `V1_EXECUTION_PLAN.md` stop check is satisfied for the 20-target core corpus.

If you cannot reach that state, your final output should be:

- which targets are still blocked
- the exact reusable blocker
- whether the blocker is truly v1-critical or the target should be removed from the core corpus

## Final Instruction

Do not optimize for activity.
Optimize for getting the fixed corpus to a truthful green state with the smallest set of reusable changes.
