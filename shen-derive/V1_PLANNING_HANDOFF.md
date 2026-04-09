# shen-derive v1 Planning Handoff

This handoff is for the next planning pass on `shen-derive` v1.

Use `V1_DONE_CHECKLIST.md` as the completion standard. This file explains how to turn that checklist into a concrete execution plan.

## Mission

Produce a realistic plan to get `shen-derive` to v1 engineering-done for a fixed corpus of 20-30 target functions.

Do not treat this as an open-ended research project.

The planning goal is to answer:

- What is the exact v1 corpus?
- Which parts already pass?
- Which reusable gaps remain?
- What order should the remaining work happen in?
- What is explicitly out of scope for v1?

## Important Framing

The system does not need to be universally complete.

The bar is:

- fixed target corpus,
- honest end-to-end pipeline,
- stable conventions,
- correct artifact regeneration,
- no open rewrite soundness issues in the supported slice.

Quantified obligations are still validation-only. That is acceptable for v1 unless the chosen corpus depends on proof-complete discharge.

## Current Known State

The following are already true and should be treated as starting assumptions unless new evidence contradicts them:

- rewrite binding override bug is fixed,
- payment demo is a real pipeline execution,
- generated payment artifacts are drift-checked,
- `go test ./...` and `go test -race ./...` pass in `shen-derive`,
- `go test ./...` and `go test -race ./...` pass in `shen-derive/demo/payment-derived`,
- current law catalog is still small,
- lowering remains pattern-based,
- quantified obligations are not soundly discharged in general.

## Planning Deliverables

Produce these files.

### 1. `V1_CORPUS_STATUS.md`

This is the main planning artifact.

It should contain one row per candidate target function with columns like:

- `name`
- `category`
- `naive spec shape`
- `expected Go shape`
- `rewrite chain`
- `obligation status`
- `law support`
- `lowering support`
- `artifact test`
- `status`
- `notes`

Use status values:

- `green`: already works end-to-end
- `yellow`: close; needs small reusable extension
- `red`: needs major new machinery or unclear semantics
- `drop`: not worth including in v1 corpus

### 2. `V1_GAP_ANALYSIS.md`

Group remaining work by reusable gap, not by example.

Suggested sections:

- law gaps
- lowering gaps
- core language gaps
- observability / artifact gaps
- test coverage gaps
- documentation / convention gaps

Each gap should state:

- why it blocks corpus items,
- how many corpus items it unlocks,
- whether it is v1-critical or optional.

### 3. `V1_EXECUTION_PLAN.md`

Turn the gaps into an ordered implementation plan.

Prefer workstreams like:

1. finalize corpus
2. close reusable law gaps
3. close reusable lowering gaps
4. add corpus end-to-end tests
5. lock conventions and docs

For each phase include:

- objective
- concrete outputs
- exit criteria
- risk level

## How To Build The Corpus

The corpus should represent the intended slice of the system, not random examples.

Aim for balanced coverage across these categories:

- map fusion
- fold fusion
- scan-to-fold style invariants
- tuple-state folds
- list-producing folds
- boolean checks over sequences
- simple accumulator transformations
- cases with no obligations
- cases with ground obligations
- cases with validation-only obligations

Try to include:

- 5-8 examples that are already likely green
- 10-15 that are likely yellow
- only a few reds, mainly to decide whether to drop or defer them

If a candidate looks like a one-off that teaches nothing reusable, prefer dropping it from v1.

## Planning Rules

Use these rules while planning.

### Keep v1 narrow

If a feature only helps one speculative example, it is probably not v1 work.

### Prefer reusable unlocks

A new law or lowerer pattern is worth doing when it unlocks multiple corpus items.

### Separate engineering confidence from proof confidence

Mark examples clearly as:

- `none`
- `ground`
- `validation-only`

Do not blur those together.

### Do not reward heroics

If an example needs manual AST surgery, handwritten generated code, or ad hoc post-processing, it is not green.

### Count new bug classes

Part of the planning output should note whether recent failures are:

- regressions inside a stable model, or
- new categories of failure.

This is a maturity signal.

## Questions The Planner Must Answer

The planning pass is not complete until it answers all of these:

1. What exactly are the 20-30 target functions?
2. Which 5 are already strongest proofs of the current architecture?
3. Which examples share the same missing law?
4. Which examples share the same missing lowering pattern?
5. Which reds should be removed from the v1 corpus rather than implemented?
6. What conventions still need to be written down and enforced by tests?
7. What is the minimum test matrix needed so future regressions are obvious?
8. What would cause us to declare v1 done with confidence?

## Suggested Planning Sequence

Use this order.

1. Inventory current capabilities.
2. Propose a raw candidate corpus larger than 20-30.
3. Classify each candidate by supported pattern.
4. Drop low-value or one-off candidates.
5. Build the final corpus table.
6. Extract reusable gaps from the non-green rows.
7. Order gaps by unlock-count and risk.
8. Write the execution plan.

## Good Planning Outcome

A good outcome looks like this:

- the corpus is fixed,
- there are only a handful of reusable missing capabilities,
- each remaining task unlocks multiple examples,
- v1 out-of-scope items are explicitly dropped,
- "done" becomes a checklist rather than a feeling.

## Bad Planning Outcome

A bad outcome looks like this:

- the corpus keeps changing,
- every hard example gets included by default,
- planning mixes v1 and v2 goals,
- there is no distinction between proof-complete and validation-only cases,
- work is organized as many one-off demo fixes instead of reusable capabilities.

## Final Instruction To The Next Agent

Do not start by coding.

Start by producing the three planning files above. The point of this handoff is to force a clear corpus and reusable gap model before more implementation work happens.
