# Shen-Backpressure

Ralph autonomous coding loop with Shen sequent-calculus type system for formal backpressure.

## What This Is

A Go orchestrator that runs a Ralph-style agent loop with three gates that must all pass before advancing:

1. **Go tests** (`go test ./...`) — empirical correctness
2. **Go build** (`go build ./cmd/ralph`) — compilation
3. **Shen type check** (`(load "specs/core.shen") (tc +)`) — formal proof via sequent calculus

If any gate fails, the error is recorded as backpressure and fed to the next LLM iteration. The agent cannot advance on invalid states.

## Quick Start

```bash
# 1. Build the Shen binary
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && make shen && cp shen /path/to/this/repo/bin/
cd /path/to/this/repo

# 2. Build the orchestrator
go build ./cmd/ralph

# 3. Run (strict sequential mode)
./ralph

# 4. Run (relaxed parallel mode — Go tests + build run concurrently)
./ralph --relaxed

# 5. Demo mode (single iteration, exit after showing gates)
RALPH_DEMO=1 ./ralph
```

## Project Structure

```
├── cmd/ralph/main.go          # Go orchestrator loop
├── specs/core.shen            # Shen formal type specifications
├── src/payment/               # Demo: payment processor with balance invariants
│   ├── processor.go
│   └── processor_test.go
├── prompts/main_prompt.md     # LLM instruction template
├── plans/fix_plan.md          # Dynamic plan for the agent
├── bin/shen                   # Shen-Go binary (build from source)
└── go.mod
```

## How Backpressure Works

The Shen spec (`specs/core.shen`) defines datatypes with sequent rules:

```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

This proves at type-check time that a transaction can only be `balance-checked` if the balance covers the amount. The Go code enforces the same invariant at runtime. Together: empirical + formal.

## Modes

- **Strict** (default): Gates run sequentially. Fail-fast on first error.
- **Relaxed** (`--relaxed`): Go tests and build run in parallel via `errgroup`. Shen type check is always the final serialized gate.

## Claude Code Skill

The skill file is at `~/.claude/skills/ralph-shen-typed-loop/SKILL.md`. It activates when you mention "Ralph loop", "Shen Lisp", "formal verification", or "type-driven backpressure".
