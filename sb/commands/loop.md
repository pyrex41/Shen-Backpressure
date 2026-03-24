---
name: loop
description: Verify prerequisites and launch a Ralph loop with four-gate Shen backpressure (shengen, test, build, shen-check). Ralph calls the LLM harness — you configure and launch, not code.
---

# Ralph Loop — Launch

You are launching a Ralph loop. Ralph is the outer loop that calls an LLM harness repeatedly. Each iteration must pass four gates. You verify prerequisites, confirm config, then launch. You do NOT do the harness's work.

```
Ralph (outer loop)
  └─▶ Gate 1: shengen (regenerate guard types from spec)
  └─▶ call harness (claude -p, cursor-agent, etc.)
       └─▶ harness makes code changes
  └─▶ Gate 2: go test
  └─▶ Gate 3: go build
  └─▶ Gate 4: shen tc+
       ├─▶ ALL PASS → next iteration (or done)
       └─▶ FAIL → inject errors into prompt → call harness again
```

## Step 1: Check Prerequisites

Verify these exist:
- `cmd/ralph/main.go` or `cmd/server/main.go` — application entry point
- `bin/shengen` or `cmd/shengen/main.go` — codegen tool (binary or source)
- `bin/shengen-codegen.sh` — codegen wrapper (executable)
- `bin/shen-check.sh` — Shen subprocess wrapper (executable)
- `bin/shen` — Shen-Go binary
- `specs/core.shen` — Shen type specifications
- `internal/shenguard/guards_gen.go` — generated guard types
- `prompts/main_prompt.md` — prompt for the harness
- `plans/fix_plan.md` — task plan with `- [ ]` items

If any are missing, tell the user and suggest `/sb:setup` or `/sb:scaffold`. Do not proceed.

## Step 2: Confirm Configuration

Present:
1. **Harness command** (from orchestrator or `RALPH_HARNESS` env)
2. **Four gates**: shengen → go test → go build → shen tc+
3. **Prompt summary** from `prompts/main_prompt.md`
4. **Remaining plan items** from `plans/fix_plan.md`
5. **Spec summary** from `specs/core.shen`

Ask user to confirm or adjust.

## Step 3: Verify Clean Starting State

```bash
make all
```

All four gates must pass before starting the loop. Fix any failures first.

## Step 4: Launch

```bash
make run
```

Options:
- `make run-relaxed` — tests and build in parallel, shengen and shen-check serialized
- `RALPH_HARNESS="<cmd>" make run` — override harness
- `RALPH_MAX_ITER=20 make run` — change max iterations (default 10)

The loop runs autonomously. Ctrl+C to stop.
