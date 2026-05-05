---
date: 2026-05-05T19:00:00Z
researcher: claude
git_commit: 399c00a
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Feature design prompt — Differential Verification"
tags: [research, feature-design, design-prompt, differential-verification]
status: design-prompt
last_updated: 2026-05-05
last_updated_by: claude
---

> Design prompt landed on main in commit `12ccd41` (`docs: add
> feature idea design prompts`). Originally lived at
> `feature_ideas/02-differential-verification.md`; moved into
> `thoughts/shared/research/` during the post-contraction cleanup so
> design-stage material lives alongside the rest of the project's
> research record. This file is a prompt for a future implementation,
> not an implementation.

# Feature Design Prompt: Differential Verification

You are designing differential verification for Shen-Backpressure. This is a design task, not an implementation task. Produce an implementation-ready design document, but do not edit code.

## Context

Shen-Backpressure already runs verification gates and can generate a future mixed-evidence report. The next useful layer is not just "what is verified right now?" but "what changed about verification since the last commit, baseline, or CI run?"

This matters because Shen-Backpressure is aimed at AI coding loops. Agents change code quickly. Humans reviewing those changes need to know whether verified behavior got stronger, weaker, or merely different.

Relevant current pieces:

- `cmd/sb/gates.go` runs configured gates.
- `cmd/sb/context.go` emits project context for agents.
- `cmd/sb/derive.go` detects generated test drift and target implementation mismatches.
- `shengen` and `shengen-ts` generate structural guard artifacts.
- `shen-derive` and `shen-derive-ts` generate deterministic oracle tests from specs.
- `examples/payment/` has committed generated artifacts that can drift.

## Goal

Design a **Differential Verification** feature that compares two evidence states and reports meaningful verification changes.

The feature should answer:

- What new evidence appeared?
- What evidence disappeared?
- What failed now that passed before?
- What passed now that failed before?
- Did sampled coverage decrease?
- Did a spec change without regenerated artifacts?
- Did a generated artifact change without a corresponding spec change?
- Did a manual assumption appear?
- Did an evidence item move from static/structural to sampled/runtime or vice versa?

## Desired User Experience

Example command surface:

```bash
sb evidence --out .sb/evidence/current.json
sb evidence diff --base .sb/evidence/main.json --head .sb/evidence/current.json
sb evidence diff --base git:origin/main --head worktree
sb gates --diff-against origin/main
```

Example output:

```text
Verification Diff

Strengthened:
- processable oracle suite: 35 -> 48 generated cases
- amount guard now rejects fractional truncation case

Regressed:
- safe-transfer guard audit drifted
- processable had 0 failing cases before, now 3 failing cases

Changed:
- specs/core.shen hash changed
- internal/derived/processable_spec_test.go not regenerated

Review required:
- new manual assumption: external-ledger-balanced
```

## Design Requirements

### 1. Baseline Strategy

Design where baselines come from.

Evaluate options:

- committed `verification/evidence.json`
- `.sb/evidence/baseline.json`
- CI artifact from the default branch
- `git show <rev>:path/to/evidence.json`
- fresh run on `origin/main` via temporary worktree

Recommend a v0 strategy and explain why.

Important tradeoffs:

- determinism
- developer ergonomics
- CI usefulness
- avoiding stale baselines
- avoiding accidental commits of noisy local paths/timestamps

### 2. Diff Schema

Design a JSON schema for `evidence-diff.json`.

Include:

- `schema_version`
- `base`
- `head`
- `summary`
- `changes`
- `regressions`
- `strengthenings`
- `requires_review`

Each change should include:

- `id`
- `change_kind`
- `evidence_id`
- `before`
- `after`
- `severity`
- `human_summary`
- `agent_instruction`
- `reproduction_command`

Define change kinds such as:

- `added`
- `removed`
- `status_changed`
- `strength_changed`
- `sample_count_changed`
- `spec_hash_changed`
- `generated_hash_changed`
- `drift_introduced`
- `counterexample_added`
- `manual_assumption_added`
- `manual_assumption_removed`

### 3. Semantics of Strengthening and Weakening

Define conservative rules for classifying changes.

Examples:

- `passed -> failed` is regression.
- `failed -> passed` is strengthening or repair.
- `sample_count 35 -> 20` is possible weakening.
- `sample_count 35 -> 48` is possible strengthening.
- `static -> sampled` is weakening.
- `sampled -> static` is strengthening.
- `new manual_assumption` requires review.
- `removed manual_assumption` is strengthening if evidence still passes.
- `spec hash changed` is neutral until interpreted with evidence changes.

Be careful: do not claim every larger sample count is automatically more sound. Use cautious language.

### 4. Agent Loop Integration

Design how differential verification feeds `sb loop`.

The agent should receive:

- the top regressions
- the command to reproduce
- the spec and implementation files involved
- whether this is a new regression since baseline
- whether it likely needs code repair or spec review

Do not flood the prompt. Propose a compact markdown format.

### 5. Human Review Integration

Design a PR-summary format.

It should answer:

- Did this PR strengthen verified behavior?
- Did it weaken anything?
- Did it introduce new unverified boundaries?
- Did it add or remove generated artifacts?
- Did it change specs?
- Are there counterexamples?

The report should be useful as a PR comment, release note, or CI artifact.

### 6. Example Walkthrough

Use `examples/payment/` as the concrete walkthrough.

Show a hypothetical change where:

- `specs/core.shen` changes.
- `internal/derived/processable_spec_test.go` is stale.
- `shen-derive` generates a new failing case.
- the diff report classifies this as drift plus behavioral regression.

Include the terminal output and JSON shape you would want.

## Guardrails

- Do not let the tool silently update baselines.
- Do not let differential verification approve spec weakening.
- Do not treat a changed spec as good or bad without evidence.
- Do not depend on a network service.
- Do not require a specific CI provider.
- Keep the diff stable under repeated runs with the same inputs.

## Deliverable

Write a detailed design document with:

1. Baseline strategy recommendation.
2. Command surface.
3. Diff JSON schema.
4. Change classification rules.
5. Human-readable output mockups.
6. Agent prompt summary format.
7. CI/PR integration proposal.
8. Staged implementation plan.
9. Risks and failure modes.
