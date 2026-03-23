# Shen-Backpressure

Autonomous coding loops with Shen sequent-calculus type system for formal backpressure. Works with any LLM harness: Claude Code, Cursor, Codex, Rho, or your own.

## The Idea

Most AI coding loops use tests as the only gate. Tests are empirical — they check specific cases. Shen adds a **deductive** gate: sequent-calculus type proofs that must hold for *all* cases. If the LLM breaks an invariant, the type check fails and the loop cannot advance. This is **backpressure** — the system rejects invalid states before they accumulate.

```
┌──────────────────────────────────────────────────────┐
│                   Ralph Loop                          │
│                                                       │
│  ┌─────────┐    ┌─────────┐    ┌──────────────────┐  │
│  │ LLM     │───▶│ Apply   │───▶│ Gates            │  │
│  │ harness │    │ changes │    │                  │  │
│  └─────────┘    └─────────┘    │ 1. go test       │  │
│       ▲                        │ 2. go build      │  │
│       │                        │ 3. shen (tc +)   │  │
│       │                        └────────┬─────────┘  │
│       │                                 │             │
│       │    ┌──────────────┐    ALL PASS? │             │
│       │    │ backpressure │◀── NO ───────┘             │
│       │    │ log errors   │                           │
│       │    └──────┬───────┘                           │
│       └───────────┘           YES ──▶ commit + next   │
└──────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Install Shen-Go (one-time)

```bash
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp shen /path/to/your/project/bin/
```

### 2. Run the included demo

```bash
cd Shen-Backpressure

# Build everything and run all gates
make all

# Run demo mode (single iteration, shows all gates passing)
make demo

# Run orchestrator in strict sequential mode
make run

# Run in relaxed parallel mode (tests + build concurrent, Shen serialized)
make run-relaxed
```

### 3. Expected output

```
15:43:08 [ralph] Starting Ralph-Shen loop (mode=strict)
15:43:08 [ralph] Tooling validated: go=OK, specs=OK, shen=OK
15:43:08 [ralph] === Iteration 1 ===
15:43:09 [ralph] PASS [go-test]
15:43:09 [ralph] PASS [go-build]
15:43:09 [ralph] PASS [shen-typecheck]
15:43:09 [ralph] All gates passed on iteration 1
```

## Using This for Your Own Project

### Option A: Claude Code skills (recommended)

If you use Claude Code, the included skills let you set up a loop interactively:

```
> Set up a Ralph-Shen loop for my inventory management system
```

Claude Code will:
1. Ask which LLM harness to use (Claude Code, Cursor, Codex, Rho, or custom)
2. Ask you to describe your domain invariants in plain English
3. Generate `specs/core.shen` with the formal type rules
4. Generate the orchestrator, prompts, and plan files
5. Run the gates to verify everything works

See [Skills](#claude-code-skills) below.

### Option B: Manual setup

#### Step 1: Copy the scaffolding

```bash
mkdir -p my-project/{cmd/ralph,specs,prompts,plans,bin,src}
cp Shen-Backpressure/cmd/ralph/main.go my-project/cmd/ralph/
cp Shen-Backpressure/bin/shen-check.sh my-project/bin/
cp Shen-Backpressure/Makefile my-project/
cp /path/to/shen my-project/bin/
```

#### Step 2: Write your Shen spec

Create `specs/core.shen` with datatypes for your domain. Example for an inventory system:

```shen
(datatype sku
  X : string;
  ==============
  X : sku;)

(datatype quantity
  X : number;
  (>= X 0) : verified;
  ====================
  X : quantity;)

(datatype stock-entry
  Item : sku;
  Qty : quantity;
  ========================
  [Item Qty] : stock-entry;)

(datatype withdrawal
  Entry : stock-entry;
  Amount : quantity;
  (>= (head (tail Entry)) Amount) : verified;
  ============================================
  [Entry Amount] : safe-withdrawal;)
```

#### Step 3: Write your domain code + tests

Write Go (or any language) code that enforces the same invariants at runtime. The Shen spec is the formal proof; the code is the implementation.

#### Step 4: Configure the harness

Edit `prompts/main_prompt.md` to describe your domain. Edit `plans/fix_plan.md` with your task list. The orchestrator reads these every iteration.

#### Step 5: Choose your LLM harness

The orchestrator's inner loop calls an LLM to propose changes. Configure this in `cmd/ralph/main.go` or via environment variable:

| Harness | Command | Notes |
|---------|---------|-------|
| Claude Code | `claude -p "$(cat prompts/main_prompt.md)"` | Default, works out of the box |
| Cursor | `cursor-agent -p "$(cat prompts/main_prompt.md)"` | Cursor's agent mode |
| Codex | `codex -p "$(cat prompts/main_prompt.md)"` | OpenAI Codex CLI |
| Rho | `rho-cli run --prompt prompts/main_prompt.md` | [github.com/pyrex41/rho](https://github.com/pyrex41/rho) |
| Custom | Any CLI that reads a prompt and outputs code changes | Set `RALPH_HARNESS` env var |

## Project Structure

```
├── cmd/ralph/main.go          # Go orchestrator — runs the loop
├── bin/shen-check.sh           # Shen subprocess wrapper (handles shen-go EOF quirks)
├── bin/shen                    # Shen-Go binary (build from source, gitignored)
├── specs/core.shen             # Shen formal type specifications
├── src/payment/                # Demo domain: payment processor
│   ├── processor.go            #   Balance invariant enforcement
│   └── processor_test.go       #   8 tests including invariant test
├── prompts/main_prompt.md      # LLM instruction template
├── plans/fix_plan.md           # Dynamic task plan
├── Makefile                    # build / test / shen-check / demo
└── go.mod
```

## How Shen Backpressure Works

### The three gates

Every iteration of the loop must pass all three gates before advancing:

1. **`go test ./...`** — Empirical correctness. Do the specific test cases pass?
2. **`go build ./cmd/ralph`** — Compilation. Does the code even compile?
3. **`shen (tc +)`** — Formal proof. Do the sequent-calculus types hold for *all* possible inputs?

### Sequent calculus in 30 seconds

Shen's type system uses sequent calculus — a proof system where you write rules of the form:

```
premise1;
premise2;
premise3;
============
conclusion;
```

If all premises are true, the conclusion follows. Example:

```shen
(datatype balance-invariant
  Bal : number;                        \* premise: Bal is a number *\
  Tx : transaction;                    \* premise: Tx is a transaction *\
  (>= Bal (head Tx)) : verified;      \* premise: balance covers amount *\
  =======================================
  [Bal Tx] : balance-checked;)         \* conclusion: this pair is safe *\
```

When Shen runs `(tc +)`, it verifies that all these rules are internally consistent. If someone adds code that could create a `balance-checked` pair without proving the balance covers the amount, the type check fails.

### Why this matters for AI coding

Without Shen, an LLM might:
1. Generate code that passes the 5 specific test cases you wrote
2. But violates the invariant for case #6 that you didn't think to test

With Shen, the LLM must generate code that satisfies the *proof* — which covers all cases by construction. The backpressure is: "your code is logically wrong, here's the type error, fix it."

## Example Domains

### Payment processor (included demo)

Invariant: *balance can never go negative through any sequence of transfers*

```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

### State machine (no deadlocks)

Invariant: *every reachable state has at least one valid transition*

```shen
(datatype live-state
  S : state;
  Transitions : (list transition);
  (not (= Transitions [])) : verified;
  =====================================
  [S Transitions] : live-state;)
```

### Resource allocator (no double-free)

Invariant: *a resource can only be freed if it is currently allocated*

```shen
(datatype allocated-resource
  R : resource-id;
  Owner : process-id;
  ========================
  [R Owner] : allocated;)

(datatype safe-free
  Alloc : allocated;
  Requester : process-id;
  (= (tail Alloc) Requester) : verified;
  =========================================
  [Alloc Requester] : safe-free;)
```

## Modes

- **Strict** (default): Gates run sequentially. Fail-fast on first error.
- **Relaxed** (`--relaxed`): Go tests and build run in parallel via `errgroup`. Shen type check is always the final serialized gate.

## Claude Code Skills

Three skills are provided for Claude Code users:

| Skill | Trigger | What it does |
|-------|---------|-------------|
| `ralph-shen-typed-loop` | "Ralph loop", "formal verification", "type-driven backpressure" | Full setup and execution of the loop |
| `shen-init` | "generate Shen types", "create type specs", "Shen spec from description" | Generates `specs/core.shen` from natural language domain description |
| `loop-setup` | "set up a loop", "configure Ralph harness", "scaffold backpressure loop" | Interactive setup: harness selection, directory scaffolding, prompt generation |

### Workflow

```
User: "I want to build an inventory system with Shen backpressure"

Claude Code activates loop-setup:
  → Asks: which harness? (claude -p / cursor-agent / codex / rho / custom)
  → Asks: describe your invariants in English
  → Generates specs/core.shen, prompts/, plans/, orchestrator
  → Runs make all to verify gates pass
  → Reports: "Ready. Run `make run` to start the loop."
```

## Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| `shen binary not found` | Haven't built shen-go | See [Install Shen-Go](#1-install-shen-go-one-time) |
| `Shen type check timed out` | Turing-complete type system hit a loop | Simplify your sequent rules; check for circular dependencies |
| `type error` in Shen output | A datatype rule is inconsistent | Read the error — it tells you which rule failed. Fix `specs/core.shen`. |
| `shen-go requires go >= 1.25` | Your Go is too old for latest shen-go | Use `GOTOOLCHAIN=local` and patch go.mod, or upgrade Go |
| Gates pass but LLM isn't called | Orchestrator demo mode only runs gates | Set `RALPH_HARNESS` and remove `RALPH_DEMO=1` for full loop |

## Design Decisions

- **Why Go for the orchestrator?** Fast compilation, trivial cross-compilation, `errgroup` for parallel gates, static binary output.
- **Why Shen over Coq/Lean/Agda?** Shen is Turing-complete (real programs, not just proofs), has a Lisp syntax (LLMs handle it well), and the type checker runs as a subprocess (no compilation step).
- **Why shell wrapper for shen-go?** `shen-go` loops on EOF instead of exiting cleanly. The wrapper script manages the subprocess lifecycle.
- **Why three gates instead of just Shen?** Defense in depth. Tests catch runtime bugs Shen can't see (I/O, timing). Shen catches logical bugs tests don't cover. Build catches syntax.
