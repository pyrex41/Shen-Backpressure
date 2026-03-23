---
name: loop
description: Configure and launch a Ralph loop with Shen sequent-calculus backpressure. Ralph calls an LLM harness repeatedly — each iteration must pass tests, build, and Shen type checking before advancing.
---

# Ralph-Shen Loop

You are configuring and launching a Ralph loop — an autonomous outer loop that repeatedly calls an LLM harness (claude -p, cursor-agent, codex, etc.) to do work, then validates that work through gates before allowing the next iteration.

**You are NOT the inner loop.** You configure the loop, then launch it. The loop calls the harness. The harness does the coding. The gates provide backpressure.

```
Ralph (outer loop)
  └─▶ call harness (claude -p "$(cat PROMPT.md)")
       └─▶ harness makes code changes
  └─▶ run gates (go test, go build, shen tc+)
       ├─▶ ALL PASS → next iteration (or done)
       └─▶ FAIL → inject errors into prompt → call harness again
```

## What You Do

1. Verify prerequisites are in place (or run `/sb:setup` first)
2. Confirm the harness command, prompt, plan, and specs with the user
3. Launch the loop

## Workflow

### Step 1: Check Prerequisites

Verify these files exist:
- `cmd/ralph/main.go` — the Go orchestrator
- `bin/shen-check.sh` — Shen subprocess wrapper (executable)
- `bin/shen` — Shen-Go binary
- `specs/core.shen` — Shen type specifications
- `prompts/main_prompt.md` — the prompt fed to the harness each iteration
- `plans/fix_plan.md` — the task plan with `- [ ]` items

If any are missing, tell the user and suggest running `/sb:setup` and/or `/sb:init` first. Do not proceed without these.

### Step 2: Confirm Configuration

Present the current configuration to the user:

1. **Harness command**: Read `defaultHarness` from `cmd/ralph/main.go`, or check `RALPH_HARNESS` env var. Show what command Ralph will execute each iteration.
2. **Prompt**: Show a summary of `prompts/main_prompt.md` — what instructions the harness will receive.
3. **Plan**: Show `plans/fix_plan.md` — the remaining `- [ ]` items the loop will work through.
4. **Specs**: Show `specs/core.shen` — the Shen types that must pass `(tc +)` every iteration.
5. **Gates**: List the three gates: `go test ./...`, `go build ./cmd/ralph`, `./bin/shen-check.sh`

Ask the user to confirm or adjust before launching.

### Step 3: Build and Verify Gates Pass

Before starting the loop, run the gates once to make sure the starting state is clean:

```bash
cd <project-root>
make all
```

If any gate fails, fix the issue first. The loop should start from a passing state.

### Step 4: Launch the Loop

Run the orchestrator:

```bash
make run
```

Or with options:
- `make run-relaxed` — tests and build run in parallel, Shen serialized last
- `RALPH_HARNESS="<custom-cmd>" make run` — override the harness
- `RALPH_MAX_ITER=20 make run` — change max iterations (default 10)

The orchestrator will:
1. Read `prompts/main_prompt.md`
2. Call the harness with the prompt (+ any backpressure errors from previous iteration)
3. Run all three gates
4. If gates pass and no plan items remain → exit success
5. If gates fail → log errors, inject into prompt, loop back to step 2

**You do not need to do anything else.** The loop runs autonomously. The user can watch it or walk away. They can Ctrl+C to stop at any time.

## Key Concepts

- **Ralph is the outer loop.** It calls the harness. You don't do the harness's work.
- **The harness is the inner agent.** It reads the prompt, makes code changes, and exits. Ralph then validates.
- **Backpressure**: Gate failures are injected into the prompt's `## Backpressure Errors` section, forcing the harness to fix them next iteration.
- **The loop terminates** when all gates pass AND no `- [ ]` items remain in `plans/fix_plan.md`, or when `RALPH_MAX_ITER` is reached.
