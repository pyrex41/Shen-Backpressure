---
name: ralph-scaffold
description: All-in-one setup for a Shen-backpressure project with Ralph loop. Combines /sb:init (specs, shengen, guard types) + /sb:loop (orchestrator, prompt, plan) into a single flow. Goes from zero to running five-gate verification.
---

# Scaffold — Full Setup in One Command

Combines `/sb:init` + `/sb:loop` into one flow. Goes from "I have a project" to "five-gate formal verification is running with a Ralph loop." If you don't want Ralph, use `/sb:init` instead.

You scaffold and verify everything. You do NOT run the loop or implement domain code.

## Step 1: Gather Everything

Ask the user:

1. **Domain description**: Entities, invariants, operations — plain English
2. **Target language**: Go (default) or TypeScript for guard types
3. **LLM harness**: `claude -p` (default), `cursor-agent -p`, `codex -p`, or custom
4. **Build/test commands**: What builds and tests the project
5. **Plan items**: What tasks should the loop work through
6. **Module name**: e.g., `github.com/user/project`

## Step 2: Create Directories

```bash
mkdir -p cmd/ralph bin specs prompts plans internal/shenguard
```

## Step 3: Generate Shen Specs

Draft `specs/core.shen` from the domain description. Use the standard pattern hierarchy: wrappers → constrained → composites → guarded → proof chains.

**Present to the user for confirmation before writing.** Explain each type. Revise if requested.

## Step 4: Install Tooling

Install shen-sbcl (Shen on SBCL) for Gate 4, shen-check.sh with 30-second timeout, shengen, and shengen-codegen.sh. Follow `/sb:init` Step 4 for details. Do NOT use shen-go (known crash bugs).

## Step 5: Generate Guard Types

```bash
./bin/shengen-codegen.sh specs/core.shen shenguard internal/shenguard/guards_gen.go
```

Show the user what was generated.

## Step 6: Generate Ralph Infrastructure

**`cmd/ralph/main.go`** — Orchestrator with five gates: shengen → test → build → shen-check → tcb-audit. Harness set from Step 1.

**`prompts/main_prompt.md`** — Inner harness prompt with guard type discipline, domain context, and backpressure errors section.

**`plans/fix_plan.md`** — Task list from Step 1.

**`bin/shenguard-audit.sh`** — Gate 5: TCB audit. Re-runs shengen, diffs output, rejects unexpected files in shenguard package.

**`Makefile`** — Targets: all, shengen, build, test, shen-check, audit, run, clean.

**`go.mod`** (if needed):
```bash
go mod init <module> && go get golang.org/x/sync && go mod tidy
```

## Step 7: Update .gitignore

```
bin/shen
bin/shengen
plans/backpressure.log
```

## Step 8: Verify All Four Gates

```bash
make all
```

All gates must pass. Fix any failures before declaring setup complete.

## Step 9: Report

Tell the user:
- What was created and where
- The five gates and what each catches
- The proof chain and how to use guard types
- How to run: `make run` or `make run-relaxed`
- How to modify: specs for types, prompt for instructions, plan for tasks
- Environment variables: `RALPH_HARNESS`, `RALPH_MAX_ITER`
