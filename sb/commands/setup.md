---
name: setup
description: Scaffold directory structure, four-gate orchestrator, shengen codegen, prompt, and plan for a Ralph-Shen backpressure loop. Configuration only — does not run the loop.
---

# Loop Setup — Scaffold a Four-Gate Ralph Loop

You scaffold the files needed to run a Ralph loop with four-gate Shen backpressure (shengen + test + build + shen-check). You do NOT run the loop or implement domain code.

## Step 1: Gather Configuration

Ask the user:

1. **Which LLM harness?**

   | Harness | Command |
   |---------|---------|
   | Claude Code | `claude -p` (default) |
   | Cursor | `cursor-agent -p` |
   | Codex | `codex -p` |
   | Rho | `rho-cli run --prompt` |
   | Custom | User provides command |

2. **Project domain?** (e.g., payment processor, API server, state machine)

3. **Plan items?** What tasks should the loop work through?

4. **Already have Shen specs?** If not, suggest `/sb:init` after setup.

## Step 2: Create Directory Structure

```bash
mkdir -p cmd/ralph bin specs prompts plans internal/shenguard
```

## Step 3: Install shengen

Copy or build the shengen codegen tool:

```bash
# Build from Shen-Backpressure repo source
cd /path/to/Shen-Backpressure/cmd/shengen && go build -o <project>/bin/shengen .
```

Create `bin/shengen-codegen.sh` (wrapper that builds shengen if missing, runs codegen):
```bash
chmod +x bin/shengen-codegen.sh
```

## Step 4: Generate the Orchestrator

Create `cmd/ralph/main.go` with FOUR gates:

```go
var gates = []gate{
    {name: "shengen",        cmd: "./bin/shengen-codegen.sh"},
    {name: "go-test",        cmd: "go", args: []string{"test", "./..."}},
    {name: "go-build",       cmd: "go", args: []string{"build", "./cmd/server"}},
    {name: "shen-typecheck", cmd: "./bin/shen-check.sh"},
}
```

Set `defaultHarness` to the user's choice from Step 1.

Create `go.mod` if needed:
```bash
go mod init <module-name>
go get golang.org/x/sync
go mod tidy
```

## Step 5: Generate Shen Check Wrapper

Create `bin/shen-check.sh` and `chmod +x`. Handles shen-go's EOF looping behavior.

## Step 6: Generate the Makefile

```makefile
.PHONY: all build test shen-check shengen run run-relaxed demo clean

all: shengen test build shen-check

shengen:
	./bin/shengen-codegen.sh

build: shengen
	go build -o server ./cmd/server

test: shengen
	go test ./...

shen-check:
	./bin/shen-check.sh

run: build
	./ralph

run-relaxed: build
	./ralph --relaxed

demo: build
	RALPH_DEMO=1 ./ralph

clean:
	rm -f server ralph bin/shengen plans/backpressure.log
```

## Step 7: Generate the Prompt Template

Create `prompts/main_prompt.md` incorporating guard type discipline from AGENT_PROMPT.md:
- Four-gate explanation
- Guard discipline: wrap at boundary, trust internally, follow proof chain
- Rules: one plan item per iteration, fix backpressure first, never bypass constructors
- Gate failure diagnosis
- Backpressure errors section (orchestrator injects here)

## Step 8: Generate the Plan

Create `plans/fix_plan.md` with user's task list as `- [ ]` items.

## Step 9: Install Shen-Go

Check if `bin/shen` exists. If not:
```bash
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp /tmp/shen-go/shen bin/shen
rm -rf /tmp/shen-go
```

## Step 10: Update .gitignore

```
/ralph
/server
bin/shen
bin/shengen
plans/backpressure.log
internal/shenguard/guards_gen.go
```

## Step 11: Present Summary

Show the user:
- File list with descriptions
- Harness command and four gates
- How to run: `make run` or `make run-relaxed`
- Remind: run `/sb:init` if specs need generating
- Remind: run `/sb:loop` when ready to start

**Do not launch the loop.** User decides when to start.
