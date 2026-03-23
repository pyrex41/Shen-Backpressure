---
name: setup
description: Interactive setup for a Ralph-Shen backpressure loop — harness selection, directory scaffolding, and prompt generation
---

# Loop Setup — Scaffold a Backpressure Loop

You are setting up the directory structure and configuration for a Ralph-Shen backpressure loop in the user's project.

## Activation

Trigger when the user says: "set up a loop", "configure Ralph harness", "scaffold backpressure loop", "set up Shen backpressure", or asks to add formal type checking to their coding loop.

## Workflow

### Step 1: Choose LLM Harness

Ask which AI coding tool will drive the inner loop:

| Harness | Command | Notes |
|---------|---------|-------|
| Claude Code | `claude -p` | Default, works out of the box |
| Cursor | `cursor-agent -p` | Cursor's agent mode |
| Codex | `codex -p` | OpenAI Codex CLI |
| Rho | `rho-cli run --prompt` | github.com/pyrex41/rho |
| Custom | User provides command | Any CLI that reads stdin prompt |

### Step 2: Create Directory Structure

```bash
mkdir -p cmd/ralph bin specs prompts plans src
```

### Step 3: Generate go.mod

If no `go.mod` exists:

```bash
go mod init <module-name>
go get golang.org/x/sync
```

### Step 4: Generate Files

Create these files (use the `ralph-shen-typed-loop` skill's templates for the full content):

1. **`cmd/ralph/main.go`** — The Go orchestrator. Update `defaultHarness` to match the user's choice from Step 1.

2. **`bin/shen-check.sh`** — The Shen subprocess wrapper. Make executable:
   ```bash
   chmod +x bin/shen-check.sh
   ```

3. **`Makefile`** — Build, test, and gate targets.

4. **`prompts/main_prompt.md`** — The LLM instruction template. Customize the task description for the user's domain.

5. **`plans/fix_plan.md`** — Initial task plan. Ask the user what they want to build and create a checklist.

6. **`specs/core.shen`** — Use the `shen-init` skill to generate this from the user's domain description.

### Step 5: Install Shen-Go Binary

Check if `bin/shen` exists. If not, guide the user:

```bash
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp shen <project>/bin/
```

Add `bin/shen` to `.gitignore` (it's a large binary that should be built locally).

### Step 6: Update .gitignore

Ensure these entries exist:

```
# Built binaries
/ralph
bin/shen

# Backpressure logs (generated at runtime)
plans/backpressure.log
```

### Step 7: Verify Setup

Run:
```bash
make all
```

This runs all three gates: `go test`, `go build`, and `shen-check`. All must pass.

If any gate fails, fix the issue before declaring setup complete.

### Step 8: Report

Tell the user:
- What was created and where
- How to run: `make run` (strict) or `make run-relaxed` (parallel)
- How to demo: `make demo` (runs gates without calling LLM)
- How to customize: edit `prompts/main_prompt.md` for LLM instructions, `plans/fix_plan.md` for task list, `specs/core.shen` for type rules
- Environment variables: `RALPH_HARNESS`, `RALPH_MAX_ITER`, `RALPH_DEMO`

## Modes

- **Strict** (default, `make run`): Gates run sequentially. Fail-fast on first error.
- **Relaxed** (`make run-relaxed`): `go test` and `go build` run in parallel. Shen type check always runs last, serialized.
