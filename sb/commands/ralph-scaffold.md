---
name: ralph-scaffold
description: All-in-one setup for a Shen-backpressure project with Ralph loop. Combines /sb:init (specs, shengen, guard types, optional shen-derive config) + /sb:loop (orchestrator, prompt, plan) into a single flow. Goes from zero to running the core five gates, with optional spec-equivalence verification when configured.
---

# Scaffold — Full Setup in One Command

Combines `/sb:init` + `/sb:loop` into one flow. Goes from "I have a project" to "formal verification is running with a Ralph loop." If you don't want Ralph, use `/sb:init` instead.

You scaffold and verify everything. You do NOT run the loop or implement domain code.

## Step 0: Detect Prior Work

Before asking questions, check if `/sb:init` was already run:
- Does `specs/core.shen` exist?
- Does `internal/shenguard/` exist with generated guard types?
- Does `bin/shen-check.sh` exist?

If init was already done, tell the user: "It looks like `/sb:init` was already run — I can see specs and guard types. I'll skip to the Ralph loop setup." Then jump to Step 6 (generate Ralph infrastructure). Don't re-ask domain questions or regenerate specs.

## Step 1: Gather Everything

Ask the user:

1. **Domain description**: Entities, invariants, operations — plain English
2. **Target language**: What language for the guard types and application code?
3. **LLM harness**: `claude -p` (default), `cursor-agent -p`, `codex -p`, or custom
4. **Build/test commands**: What builds and tests the project
5. **Plan items**: What tasks should the loop work through
6. **Optional derive targets**: Any pure `(define ...)` functions that should become `shen-derive` drift gates

## Step 2: Create Directories

Create the directory structure appropriate for the target language:
```bash
mkdir -p bin specs prompts plans
# Plus language-specific directories (e.g., cmd/ralph for Go, src/ for TS)
```

## Step 3: Generate Shen Specs

Draft `specs/core.shen` from the domain description. Use the standard pattern hierarchy: wrappers → constrained → composites → guarded → proof chains.

**Present to the user for confirmation before writing.** Explain each type. Revise if requested.

## Step 4: Install Tooling

Install shen-sbcl (Shen on SBCL) for Gate 4, shen-check.sh with 30-second timeout, shengen, and shengen-codegen.sh. See `/sb:init` Step 4 for details. Do NOT use shen-go (known crash bugs).

## Step 5: Generate Guard Types

```bash
./bin/shengen-codegen.sh specs/core.shen <package-name> <output-path>
```

Show the user what was generated.

## Step 6: Generate Ralph Infrastructure

The Ralph loop is handled by `sb loop` — a headless LLM harness that runs gates, injects errors, and calls the LLM repeatedly. Set it up:

**`sb.toml`** — Configure the loop (or generate with `sb init --config`):
```toml
[loop]
harness = "claude -p"    # from Step 1
max_iter = 10
timeout = "10m"
prompt = "prompts/main_prompt.md"
plan = "plans/fix_plan.md"
```

**`prompts/main_prompt.md`** — Inner harness prompt with guard type discipline, domain context, and backpressure errors section.

**`plans/fix_plan.md`** — Task list from Step 1.

**`bin/shenguard-audit.sh`** — Gate 5: TCB audit. Re-runs shengen, diffs output, rejects unexpected files in shenguard package.

**`sb.toml [derive]`** (optional) — If the project has pure `(define ...)` functions to pin against handwritten Go implementations, add `[[derive.specs]]` entries so `sb derive` and `sb gates` can run the spec-equivalence gate.

**`Makefile`** (optional) — Targets: all, shengen, build, test, shen-check, audit, derive, run, clean.

**Project init** (if needed) — `go mod init`, `npm init`, `cargo init`, etc.

## Step 7: Update .gitignore

```
bin/shen
bin/shengen
plans/backpressure.log
```

## Step 8: Verify All Configured Gates

```bash
sb gates
```

If derive coverage was configured, initialize or refresh the committed generated tests with:

```bash
sb derive --regen
sb derive
```

All configured gates must pass. Fix any failures before declaring setup complete.

If Gate 4 (shen-check) fails with a timeout or crash, verify shen-sbcl is installed: `shen-sbcl -q -e "(+ 1 1)"`

## Step 9: Report

Tell the user:
- What was created and where
- The core five gates and what each catches
- Whether the optional `shen-derive` gate was configured
- The proof chain and how to use guard types
- How to run: `sb loop` or `sb loop --dry-run` to get the bash script
- How to modify: specs for types, prompt for instructions, plan for tasks, and `[[derive.specs]]` for spec-equivalence coverage
- Configuration: `sb.toml [loop]` or env vars `RALPH_HARNESS`, `RALPH_MAX_ITER`, `RALPH_HARNESS_TIMEOUT`
