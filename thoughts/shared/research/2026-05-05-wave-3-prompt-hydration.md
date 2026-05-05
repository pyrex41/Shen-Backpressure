---
date: 2026-05-05T17:55:00Z
researcher: claude
git_commit: 6f367c8959a0400b9013f30844927ee087db9495
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Wave 3 — prompt hydration in the Ralph loop"
tags: [research, sb-engine, wave-3, loop, prompt-hydration, post-hoc]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Wave 3 — Prompt hydration in the Ralph loop

Post-hoc record of the Wave 3 work landed in commit `5b35626`
(`sb: Wave 3 — prompt hydration wires sb context into Ralph loop`,
2026-04-12).

## What problem this wave solved

Wave 2 made `sb context` the canonical structured view of a project,
but the loop didn't consume it. `runLoop` still concatenated a static
prompt file, a static plan file, and the gate-failure block — nothing
from `sb context`. That meant the LLM saw the same fixed text every
iteration even as the spec evolved, the manifest changed, and the
proof chain restructured.

Symptoms in real Ralph runs:

- The agent invented constructor names or argument orders that no
  longer matched the regenerated guard types.
- After the user added a new datatype to the spec, the prompt's
  worked-examples (Amount, Transaction, BalanceChecked) did not
  mention it; the agent kept editing only the original three.
- `AGENT_PROMPT.md` was 252 lines, most of which were Amount/
  Transaction examples that became out-of-date the moment a project
  picked any other domain.

Wave 3 closes the loop: every iteration calls `BuildContext(cfg)` and
splices the rendered markdown into the prompt before the harness sees
it. The harness now reads what the engine actually believes about the
project as of this iteration, not what the static prompt said weeks
ago.

## What was built

In-process integration in `cmd/sb/loop.go`:

- `loop.go:65-97` — `runLoop`'s per-iteration body, in order: run
  gates → if pass, exit → write `plans/backpressure.log` → call
  `BuildContext(cfg)` → assemble prompt → call harness. `BuildContext`
  errors are non-fatal: a warning is logged and the loop continues
  with `ctx == nil`.
- `loop.go:120-150` — `buildHarnessPrompt` composes
  `prompt + "## Live Project Context\n\n" + ctx.RenderMarkdown() +
  "## Current Plan\n\n" + plan + "## Backpressure Errors (fix these
  FIRST)\n\n" + gateErrors`. The order is deliberate: static prompt
  first (project policy), live context next (current ground truth),
  plan after that (what the agent is supposed to do), backpressure
  last so it is the most recent thing the agent reads.
- `loop.go:174-216` — `buildLoopScript` (the `--dry-run` shell-out
  variant) shells out to `sb context --format markdown` so the
  generated bash loop has the same shape without depending on the
  in-process Go path.

In `sb/AGENT_PROMPT.md` (canonical, 105 lines, down from 252): the
static "examples" section was deleted and replaced with a short "Live
Project Context" instruction telling the agent that the injected block
is the source of truth for guard types, constructors, the proof
chain, configured gates, and recent backpressure. Per-language
constructor snippets and the spec-pattern taxonomy were removed
because the live context already carries them.

`sb/commands/loop.md` documents the per-iteration prompt assembly
order so prompt authors can predict what the agent will see.

## What was rejected

- **Shelling out to `sb context` from inside `runLoop`.** Considered
  but rejected — `BuildContext` is already a Go function on
  `*Config`, and shelling out would re-parse the spec on every
  iteration. The shell-out form survives only for `--dry-run`, where
  the output is a portable bash loop that has to work without a Go
  runtime.
- **Caching the context across iterations.** The whole point is that
  the spec or manifest can change between iterations (because the
  agent edited it). A stale cached context would produce backpressure
  errors that did not match the current ground truth.
- **Failing the loop when `BuildContext` errors.** Rejected as too
  brittle — a half-edited spec mid-iteration would prevent any further
  forward progress. The fallback (log + continue with `ctx == nil`)
  means the agent gets the static prompt and gate errors but loses
  only the live-context block.

## What's open

- Live-context injection writes the same block on every iteration.
  Diff-based injection (only the changes since the previous iteration)
  could keep the prompt smaller in long-running loops; not implemented.
- The plan file and backpressure log are still concatenated raw.
  Bringing them into `ProjectContext` would unify the input shape but
  would break harnesses that assume the existing section names.
- `--dry-run` and the in-process path can drift, since they share
  intent but not implementation. A test that runs both paths against
  a fixed example and diffs the resulting prompts would lock the
  contract.
