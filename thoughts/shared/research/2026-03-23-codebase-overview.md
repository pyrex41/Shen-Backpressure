---
date: 2026-03-23T00:00:00-07:00
researcher: reuben
git_commit: b85dde5a4eee389b29699a40f7db1b3f2cab3bca
branch: main
repository: Shen-Backpressure
topic: "What is this project and how does it work?"
tags: [research, codebase, overview, shen, backpressure, ralph, orchestrator]
status: complete
last_updated: 2026-03-23
last_updated_by: reuben
---

# Research: What is this project and how does it work?

**Date**: 2026-03-23
**Researcher**: reuben
**Git Commit**: b85dde5a4eee389b29699a40f7db1b3f2cab3bca
**Branch**: main
**Repository**: Shen-Backpressure

## Research Question
What is this, how does it work?

## Summary

Shen-Backpressure is a framework for **AI coding loops with formal type-system backpressure**. It pairs any LLM coding harness (Claude Code, Cursor, Codex, Rho, or custom) with **Shen**, a language whose type system is based on sequent calculus. The core idea: before an LLM's code changes can advance, they must pass three gates — Go tests, Go build, and Shen type checking. If any gate fails, the errors are fed back into the LLM prompt for the next iteration. This creates "backpressure" that forces the LLM to fix logical errors, not just pass specific test cases.

The included demo domain is a **payment processor** where the invariant is "balance can never go negative."

## Detailed Findings

### 1. Orchestrator (`cmd/ralph/main.go`)

The orchestrator ("Ralph") is a Go binary that runs an iterative loop:

1. **Validate tooling** — checks that `go`, `specs/core.shen`, and `bin/shen-check.sh` all exist (`cmd/ralph/main.go:48-75`)
2. **Build prompt** — reads `prompts/main_prompt.md` and injects any errors from the previous iteration into a `## Backpressure Errors` section (`cmd/ralph/main.go:77-103`)
3. **Call LLM** — shells out to the configured harness (default: `claude -p`) passing the prompt via stdin (`cmd/ralph/main.go:105-111`)
4. **Run gates** — executes three validation gates in sequence (strict mode) or with partial parallelism (relaxed mode) (`cmd/ralph/main.go:113-173`)
5. **Loop or exit** — if all gates pass AND no plan items remain, exit successfully. Otherwise, collect failed gate outputs, log them, and feed them back into the next iteration (`cmd/ralph/main.go:254-295`)

Key configuration:
- `RALPH_DEMO=1` — skips the LLM call, just runs gates (for testing the framework itself)
- `RALPH_HARNESS` — which LLM CLI to invoke (default: `claude -p`)
- `RALPH_MAX_ITER` — max loop iterations before giving up (default: 10)
- `--relaxed` flag — run `go test` and `go build` in parallel via `errgroup`, but always serialize the Shen type check last

### 2. Three Gates

The gates are defined at `cmd/ralph/main.go:37-41`:

| Gate | Command | Purpose |
|------|---------|---------|
| `go-test` | `go test ./...` | Empirical correctness — specific test cases pass |
| `go-build` | `go build ./cmd/ralph` | Compilation — code is syntactically valid |
| `shen-typecheck` | `./bin/shen-check.sh` | Formal proof — sequent calculus types hold for all inputs |

In **strict mode**, gates run sequentially and fail-fast. In **relaxed mode**, `go-test` and `go-build` run concurrently via `errgroup`, then `shen-typecheck` runs last.

### 3. Shen Type Specifications (`specs/core.shen`)

The Shen spec file defines the formal type system for the payment domain using sequent calculus datatypes:

- **`account-id`** — a string that represents an account identifier
- **`amount`** — a number that is >= 0 (non-negative constraint baked into the type)
- **`transaction`** — a triple `[Amount From To]` where Amount is an `amount` and From/To are `account-id`s
- **`balance-invariant`** — the key proof: a pair `[Bal Tx]` is `balance-checked` only if `Bal >= head(Tx)` (balance covers the transaction amount)
- **`account-state`** — a pair `[Id Balance]`
- **`safe-transfer`** — a transaction paired with its balance-check proof

The sequent calculus rules use the form: premises above the line, conclusion below. Shen's `(tc +)` command verifies internal consistency of all rules.

### 4. Shen Check Wrapper (`bin/shen-check.sh`)

A bash script that works around `shen-go`'s behavior of looping on EOF instead of exiting:

1. Creates a temp file for output
2. Pipes `(load "specs/core.shen")` and `(tc +)` into the shen binary as background process
3. Polls the output file every 0.5s for up to 10s, looking for "type error" (fail) or "true" (pass)
4. Kills the shen process (which would otherwise loop forever)
5. Returns exit code 0 for pass, 1 for fail

### 5. Demo Domain: Payment Processor (`src/payment/`)

**`processor.go`** — A thread-safe payment processor with:
- `Account` struct with ID and Balance
- `Transaction` struct with Amount, From, To
- `Processor` with mutex-protected account map and transaction history
- `CreateAccount()` — rejects negative initial balance (mirrors Shen's `amount` type)
- `Transfer()` — checks balance >= amount before executing (mirrors Shen's `balance-invariant`)
- `GetBalance()`, `History()` — read operations

**`processor_test.go`** — 8 tests covering:
- Account creation (valid and negative balance)
- Transfer (valid, insufficient balance, negative amount, self-transfer, missing account)
- Transaction history
- **`TestBalanceNeverNegative`** — the key invariant test that mirrors the Shen proof: sequences of transfers cannot make any balance negative

### 6. Prompt and Plan Files

**`prompts/main_prompt.md`** — The template sent to the LLM every iteration. Instructs it to:
- Read `specs/core.shen` and `plans/fix_plan.md`
- Implement ONE next item from the plan
- Strengthen Shen types alongside Go code
- Prioritize fixing any backpressure errors shown below the marker
- The orchestrator injects gate failure output into a `## Backpressure Errors` section

**`plans/fix_plan.md`** — A markdown task list tracking progress. The orchestrator checks for `- [ ]` items to determine if work remains.

### 7. Build System (`Makefile`)

| Target | Action |
|--------|--------|
| `all` | `build` + `test` + `shen-check` |
| `build` | `go build -o ralph ./cmd/ralph` |
| `test` | `go test ./...` |
| `shen-check` | runs `bin/shen-check.sh` |
| `run` | builds then runs `./ralph` in strict mode |
| `run-relaxed` | builds then runs `./ralph --relaxed` |
| `demo` | builds then runs with `RALPH_DEMO=1` (no LLM call) |
| `clean` | removes built binary and backpressure log |

## Code References

- [`cmd/ralph/main.go`](https://github.com/pyrex41/Shen-Backpressure/blob/b85dde5a4eee389b29699a40f7db1b3f2cab3bca/cmd/ralph/main.go) — Go orchestrator (296 lines)
- [`specs/core.shen`](https://github.com/pyrex41/Shen-Backpressure/blob/b85dde5a4eee389b29699a40f7db1b3f2cab3bca/specs/core.shen) — Shen formal types (48 lines)
- [`bin/shen-check.sh`](https://github.com/pyrex41/Shen-Backpressure/blob/b85dde5a4eee389b29699a40f7db1b3f2cab3bca/bin/shen-check.sh) — Shen subprocess wrapper (68 lines)
- [`src/payment/processor.go`](https://github.com/pyrex41/Shen-Backpressure/blob/b85dde5a4eee389b29699a40f7db1b3f2cab3bca/src/payment/processor.go) — Payment processor (119 lines)
- [`src/payment/processor_test.go`](https://github.com/pyrex41/Shen-Backpressure/blob/b85dde5a4eee389b29699a40f7db1b3f2cab3bca/src/payment/processor_test.go) — Tests (152 lines)
- [`prompts/main_prompt.md`](https://github.com/pyrex41/Shen-Backpressure/blob/b85dde5a4eee389b29699a40f7db1b3f2cab3bca/prompts/main_prompt.md) — LLM prompt template
- [`plans/fix_plan.md`](https://github.com/pyrex41/Shen-Backpressure/blob/b85dde5a4eee389b29699a40f7db1b3f2cab3bca/plans/fix_plan.md) — Task tracking

## Architecture Documentation

The system follows a **closed-loop feedback architecture**:

```
LLM Harness → Code Changes → Gate 1 (tests) → Gate 2 (build) → Gate 3 (Shen types) → Pass? → Commit
                                                                                       ↓ Fail
                                                                              Error injection into prompt → LLM Harness
```

Key architectural patterns:
- **Gate pattern**: Each validation step is a `gate` struct with name, command, and args. Gates are run via `runOneGate()` which captures combined stdout/stderr output.
- **Backpressure via prompt injection**: Failed gate outputs are formatted as markdown and inserted into the LLM prompt's backpressure section, creating a feedback loop.
- **Plan-driven termination**: The loop exits when all gates pass AND no unchecked `- [ ]` items remain in `plans/fix_plan.md`.
- **Dual enforcement**: Invariants are stated both formally (Shen sequent calculus) and operationally (Go runtime checks + tests). The Shen layer covers cases tests might miss.
- **Harness-agnostic**: The LLM is invoked via shell command, making it pluggable across different AI coding tools.

## Historical Context (from thoughts/)
No prior research documents exist — this is the first.

## Open Questions
- The `bin/shen` binary must be built separately from `shen-go` and is gitignored. The project cannot fully function without it.
- The plan has unchecked items (transfer history tracking, multi-currency, concurrent safety, stress testing) suggesting active/planned development.
- The LLM integration path (`callLLM`) passes the prompt via stdin to a shell command but doesn't capture the LLM's output programmatically — it streams to stdout/stderr directly.
