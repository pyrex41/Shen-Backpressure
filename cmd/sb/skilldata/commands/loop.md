---
name: loop
description: Configure and launch a Ralph loop — a headless LLM harness in a bash loop with Shen backpressure, using the core five gates plus optional shen-derive verification when configured. Requires /sb:init first.
---

# Ralph Loop — Configure and Launch

You configure and launch a Ralph loop — a headless LLM harness that runs in a loop, validates through the core five gates, optionally runs `shen-derive`, and injects failures back as backpressure. This is ONE way to use Shen backpressure. For CI or manual workflows, see `/sb:init`.

**Prerequisite**: Run `/sb:init` first to set up specs, shengen, and guard types.
**CLI shortcut**: Once configured, `sb loop` runs the loop directly. `sb loop --dry-run` prints the bash script.

```
Ralph (outer loop)
  └─> Gate 1: shengen (regenerate guard types from spec)
  └─> call harness (claude -p, cursor-agent, codex, etc.)
       └─> harness makes code changes
  └─> Gate 2: test (go test, npm test, cargo test, etc.)
  └─> Gate 3: build (compile against regenerated types)
  └─> Gate 4: shen tc+ (verify spec consistency)
  └─> Gate 5: tcb audit (diff generated code, reject unexpected files)
  └─> Gate 6: shen-derive (optional; spec-equivalence drift gate if configured)
       ├─> ALL PASS → next iteration (or done)
       └─> FAIL → inject errors into prompt → call harness again
```

## Step 1: Check Prerequisites

Verify `/sb:init` was already run:
- `specs/core.shen` exists
- Guard types exist (generated file from shengen)
- `bin/shen-check.sh` exists and is executable
- shengen tooling exists

If any are missing, tell the user to run `/sb:init` first.

Also verify Gate 4 works by running `bin/shen-check.sh` once. If it crashes or times out, the Shen runtime needs fixing before the loop can run — check which runtime shen-check.sh uses and switch to shen-sbcl if needed.

## Step 2: Gather Loop Configuration

Ask the user:

1. **Which LLM harness will Ralph call each iteration?**
   - `claude -p` (default), `cursor-agent -p`, `codex -p`, `rho-cli run --prompt`, or custom command

2. **What's the build command?** (e.g., `go build ./cmd/server`, `npm run build`, `cargo build`)

3. **What's the test command?** (e.g., `go test ./...`, `npm test`, `cargo test`)

4. **What should the plan contain?** Task items the loop should work through (`- [ ]` checklist)

## Step 3: Generate Loop Infrastructure

Create these files:

**Ralph orchestrator** (e.g., `cmd/ralph/main.go` for Go, `ralph.ts` for TS, or a shell script) — runs the core five gates in order, then `sb derive` when configured:
1. shengen (regenerate guard types)
2. test
3. build
4. shen-check
5. tcb-audit (diff generated code, reject unexpected files)
6. `sb derive` if `sb.toml` contains any `[[derive.specs]]` entries

Set the harness command from Step 2.
- `RALPH_MAX_ITER` env var (default 10)
- `RALPH_HARNESS` env var for harness override
- `RALPH_HARNESS_TIMEOUT` env var for per-call timeout (default 10 minutes)
- Backpressure error injection: on gate failure, append the error output to the harness prompt

**`prompts/main_prompt.md`** — What the harness receives each iteration. Include:
- Domain context and file locations
- Guard type discipline (wrap at boundary, trust internally, follow proof chain)
- Rules: one plan item per iteration, fix backpressure errors first
- Gate failure diagnosis
- Backpressure errors section (orchestrator injects here)

**`plans/fix_plan.md`** — Task plan with `- [ ]` items from Step 2.

**`Makefile`** — Targets: all, shengen, build, test, shen-check, audit, derive, run, clean.

## Step 4: Verify Clean Starting State

```bash
sb gates
```

All configured gates must pass. Fix any failures before launching.

## Step 5: Launch

```bash
sb loop
```

Or via Makefile:
```bash
make run
```

Options:
- `sb loop --dry-run` — print the bash script without running
- `sb gates --relaxed` — test and build in parallel
- `sb derive --regen` — if the optional derive gate is configured and the committed generated tests need to be refreshed intentionally
- `RALPH_HARNESS="<cmd>" sb loop` — override harness
- `RALPH_MAX_ITER=20 sb loop` — max iterations (default 10)
- `RALPH_HARNESS_TIMEOUT=15m sb loop` — increase harness timeout

Configure via `sb.toml [loop]` section for persistent settings.

The loop runs autonomously. Ctrl+C to stop.

## Live Context Injection

Each iteration, `sb loop` calls `BuildContext` and injects the rendered markdown into the harness prompt under a `## Live Project Context` heading, between the static prompt and the current plan. This replaces the old model where the agent had to discover guard types from files or be told about them in a static prompt.

The prompt assembled per iteration looks like:

1. Static prompt (`prompts/main_prompt.md` — discipline rules, anti-patterns)
2. `## Live Project Context` — fresh output of `sb context --format markdown`
3. `## Current Plan` — `plans/fix_plan.md`
4. `## Backpressure Errors (fix these FIRST)` — last gate failure, if any

The agent consuming the prompt should trust the `## Live Project Context` block as the source of truth for project-specific facts (guard type inventory, constructor signatures, proof chain, configured gates, derive coverage). Context generation failure is non-fatal — the loop logs a warning and continues without the live block. No user action is needed; injection happens automatically per iteration.

`sb loop --dry-run` generates a bash script that shells out to `sb context --format markdown` inside the loop body so the generated script mirrors the in-process behavior.
