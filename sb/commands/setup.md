---
name: setup
description: Scaffold directory structure, orchestrator, gates, prompt, and plan for a Ralph-Shen backpressure loop. Configuration only — does not run the loop or do coding work.
---

# Loop Setup — Scaffold a Ralph-Shen Loop

You are scaffolding the files and configuration needed to run a Ralph loop with Shen backpressure. You create the orchestrator, gates, prompt template, and plan. You do NOT run the loop or implement domain code — Ralph and the harness do that.

## Workflow

### Step 1: Gather Configuration

Ask the user:

1. **Which LLM harness will Ralph call each iteration?**

   | Harness | Command | Notes |
   |---------|---------|-------|
   | Claude Code | `claude -p` | Default |
   | Cursor | `cursor-agent -p` | Cursor agent mode |
   | Codex | `codex -p` | OpenAI Codex CLI |
   | Rho | `rho-cli run --prompt` | github.com/pyrex41/rho |
   | Custom | User provides | Any CLI that accepts a prompt as last arg or via stdin |

2. **What is the project domain?** (e.g., payment processor, API server, state machine)

3. **What should the plan contain?** What tasks should the loop work through? These become `- [ ]` items in `plans/fix_plan.md`.

4. **Do they already have Shen specs?** If not, suggest running `/sb:init` after setup.

### Step 2: Create Directory Structure

```bash
mkdir -p cmd/ralph bin specs prompts plans
```

### Step 3: Generate the Orchestrator

Create `cmd/ralph/main.go` — the Go program that IS Ralph. It:
- Reads `prompts/main_prompt.md`
- Calls the harness command with the prompt
- Runs three gates: `go test ./...`, `go build ./cmd/ralph`, `./bin/shen-check.sh`
- If all pass and plan is done → exit
- If any fail → inject errors into prompt → call harness again

Set `defaultHarness` to the user's choice from Step 1.

Create `go.mod` if needed:
```bash
go mod init <module-name>
go get golang.org/x/sync
go mod tidy
```

### Step 4: Generate the Shen Check Wrapper

Create `bin/shen-check.sh` and make it executable (`chmod +x`).

This script works around shen-go's EOF behavior:
- Pipes `(load "specs/core.shen")` and `(tc +)` into the shen binary
- Polls output for "type error" (fail) or "true" (pass)
- Kills the shen process (it loops forever otherwise)
- Returns exit 0 for pass, exit 1 for fail

### Step 5: Generate the Makefile

Create `Makefile` with targets:
- `all`: build + test + shen-check
- `build`: `go build -o ralph ./cmd/ralph`
- `test`: `go test ./...`
- `shen-check`: `./bin/shen-check.sh`
- `run`: build then `./ralph`
- `run-relaxed`: build then `./ralph --relaxed`
- `demo`: build then `RALPH_DEMO=1 ./ralph` (runs gates without calling harness)
- `clean`: remove built binary and backpressure log

### Step 6: Generate the Prompt Template

Create `prompts/main_prompt.md` — this is what the harness receives every iteration. Customize for the user's domain. It must include:

- Instructions to read `specs/core.shen` and `plans/fix_plan.md`
- Rule: implement ONE plan item per iteration
- Rule: every new behavior needs a corresponding Shen datatype
- Rule: if backpressure errors appear, fix those FIRST
- A `## Backpressure Errors (from previous iteration)` section (the orchestrator injects gate failures here)

### Step 7: Generate the Plan

Create `plans/fix_plan.md` with the user's task list as `- [ ]` checkbox items. The orchestrator checks for remaining items to decide when to stop.

### Step 8: Install Shen-Go Binary

Check if `bin/shen` exists. If not, install it:

```bash
mkdir -p bin
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp /tmp/shen-go/shen bin/shen
rm -rf /tmp/shen-go
chmod +x bin/shen-check.sh
```

### Step 9: Update .gitignore

Add:
```
/ralph
bin/shen
plans/backpressure.log
```

### Step 10: Present Summary and Confirm

Show the user everything that was created:
- File list with brief descriptions
- The harness command Ralph will use
- The plan items
- How to run: `make run` or `make run-relaxed`
- How to verify gates: `make all` or `make demo`
- Remind them to run `/sb:init` if `specs/core.shen` still needs domain types

**Do not launch the loop.** The user decides when to start. They use `/sb:loop` or `make run` when ready.
