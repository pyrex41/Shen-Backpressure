---
name: scaffold
description: All-in-one setup for a Shen-backpressure project with codegen bridge. Gathers domain description, generates specs, builds shengen, produces guard types, scaffolds four-gate orchestrator, and verifies everything works.
---

# Scaffold — Full Shen-Backpressure Setup

One command to go from "I have a Go project" to "four-gate formal verification is running." This combines `/sb:setup` + `/sb:init` into a single flow.

You scaffold everything and verify it works. You do NOT run the loop or implement domain code.

## Step 1: Gather Everything

Ask the user:

1. **Domain description**: What are you building? What are the key entities, invariants, and operations?
2. **LLM harness**: Which tool will Ralph call each iteration?
   - `claude -p` (default), `cursor-agent -p`, `codex -p`, `rho-cli run --prompt`, or custom
3. **Plan items**: What tasks should the loop work through? (becomes `- [ ]` items)
4. **Module name**: Go module path for `go.mod` (e.g., `github.com/user/project`)

## Step 2: Create Directory Structure

```bash
mkdir -p cmd/ralph cmd/shengen bin specs prompts plans internal/shenguard
```

## Step 3: Generate Shen Specs

Translate the user's domain description into `specs/core.shen` with sequent-calculus datatypes. Follow the pattern hierarchy: wrappers → constrained → composites → guarded → proof chains.

**Present the complete spec to the user for confirmation before writing.** Explain each type and what invariant it encodes. Revise if requested.

## Step 4: Build shengen

Place shengen source at `cmd/shengen/main.go` (copy from Shen-Backpressure repo) and create `cmd/shengen/go.mod`:

```bash
cd cmd/shengen && go build -o ../../bin/shengen .
```

Create `bin/shengen-codegen.sh` and `chmod +x`.

## Step 5: Generate Guard Types

```bash
./bin/shengen-codegen.sh specs/core.shen shenguard internal/shenguard/guards_gen.go
```

Show the user what was generated — the Go types, constructors, and which ones return errors (constrained and guarded types).

## Step 6: Install Shen-Go

```bash
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp /tmp/shen-go/shen bin/shen
rm -rf /tmp/shen-go
```

Create `bin/shen-check.sh` and `chmod +x`.

## Step 7: Generate Orchestrator

Create `cmd/ralph/main.go` with four gates:
1. `shengen` — `./bin/shengen-codegen.sh`
2. `go-test` — `go test ./...`
3. `go-build` — `go build ./cmd/server` (or appropriate entry point)
4. `shen-typecheck` — `./bin/shen-check.sh`

Set `defaultHarness` to the user's choice.

Create `go.mod`:
```bash
go mod init <module-name>
go get golang.org/x/sync
go mod tidy
```

## Step 8: Generate Prompt

Create `prompts/main_prompt.md` with:
- Domain context
- Four-gate explanation
- Guard type discipline (from AGENT_PROMPT.md): wrap at boundary, trust internally, follow proof chain, extract with .Val()
- Rules: one plan item per iteration, fix backpressure first, never bypass constructors, never edit guards_gen.go
- Gate failure diagnosis table
- Backpressure errors section

## Step 9: Generate Plan and Makefile

Create `plans/fix_plan.md` with the user's task list.

Create `Makefile` with targets: all, shengen, build, test, shen-check, run, run-relaxed, demo, clean.

## Step 10: Update .gitignore

```
/ralph
/server
bin/shen
bin/shengen
plans/backpressure.log
internal/shenguard/guards_gen.go
```

## Step 11: Verify All Four Gates

```bash
make all
```

Expected:
```
Generated internal/shenguard/guards_gen.go from specs/core.shen (package shenguard)
ok   module/...
RESULT: PASS
```

If any gate fails, fix the issue before declaring setup complete.

## Step 12: Report

Tell the user:
- Everything that was created
- The four-gate architecture
- How to run: `make run` or `/sb:loop`
- How to modify: edit `specs/core.shen` for types, `prompts/main_prompt.md` for instructions, `plans/fix_plan.md` for tasks
- The generated guard types and how the harness should use them
- Environment variables: `RALPH_HARNESS`, `RALPH_MAX_ITER`, `RALPH_DEMO`
