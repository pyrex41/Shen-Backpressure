---
name: loop
description: Configure and launch a Ralph loop — an autonomous outer loop that calls an LLM harness repeatedly with five-gate Shen backpressure. Requires /sb:init first.
---

# Ralph Loop — Configure and Launch

You configure and launch a Ralph loop — an autonomous outer loop that repeatedly calls an LLM harness to do work, then validates through five gates before allowing the next iteration. This is ONE way to use Shen backpressure. For CI or manual workflows, see `/sb:init`.

**Prerequisite**: Run `/sb:init` first to set up specs, shengen, and guard types.

```
Ralph (outer loop)
  └─> Gate 1: shengen (regenerate guard types from spec)
  └─> call harness (claude -p, cursor-agent, codex, etc.)
       └─> harness makes code changes
  └─> Gate 2: test (go test, npm test, cargo test, etc.)
  └─> Gate 3: build (compile against regenerated types)
  └─> Gate 4: shen tc+ (verify spec consistency)
  └─> Gate 5: tcb audit (diff generated code, reject unexpected files)
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

**Ralph orchestrator** (e.g., `cmd/ralph/main.go` for Go, `ralph.ts` for TS, or a shell script) — runs five gates in order:
1. shengen (regenerate guard types)
2. test
3. build
4. shen-check
5. tcb-audit (diff generated code, reject unexpected files)

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

**`Makefile`** — Targets: all, shengen, build, test, shen-check, run, clean.

## Step 4: Verify Clean Starting State

```bash
make all
```

All five gates must pass. Fix any failures before launching.

## Step 5: Launch

```bash
make run
```

Options:
- `make run-relaxed` — test and build in parallel
- `RALPH_HARNESS="<cmd>" make run` — override harness
- `RALPH_MAX_ITER=20 make run` — max iterations (default 10)
- `RALPH_HARNESS_TIMEOUT=15m make run` — increase harness timeout

The loop runs autonomously. Ctrl+C to stop.
