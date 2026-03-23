# Shen-Backpressure

AI coding skills for autonomous loops with **Shen sequent-calculus type backpressure**. Installable via [SKM](https://github.com/pyrex41/skill-manager) or manually as Claude Code skills.

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

## Install

### Option A: SKM (recommended)

```bash
# Add this repo as a source
skm sources add https://github.com/pyrex41/Shen-Backpressure

# Install the commands into your project
cd your-project
skm sb
```

### Option B: Manual Claude Code install

Copy the command files directly:

```bash
mkdir -p .claude/commands/sb
cp Shen-Backpressure/sb/commands/*.md .claude/commands/sb/
```

## Commands

| Command | Trigger | What it does |
|---------|---------|-------------|
| `/sb:loop` | "Ralph loop", "formal verification", "type-driven backpressure" | Full setup and execution of a backpressure loop |
| `/sb:init` | "generate Shen types", "create type specs", "Shen spec from description" | Generates `specs/core.shen` from natural language domain description |
| `/sb:setup` | "set up a loop", "configure Ralph harness", "scaffold backpressure loop" | Interactive setup: harness selection, directory scaffolding, prompt generation |

### Usage

After installing, invoke directly:

```
> /sb:setup
> /sb:init
> /sb:loop
```

Or describe what you want and Claude Code will activate the right command:

```
> Set up a Ralph-Shen loop for my inventory management system
```

Claude Code will:
1. Ask which LLM harness to use (Claude Code, Cursor, Codex, Rho, or custom)
2. Ask you to describe your domain invariants in plain English
3. Generate `specs/core.shen` with formal type rules
4. Generate the orchestrator, prompts, and plan files
5. Run the gates to verify everything works

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

### Payment processor ([demo included](demo/payment/))

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

## Demos

Working examples live in `demo/`:

- **[`demo/payment/`](demo/payment/)** — Payment processor with balance invariants

Each demo is a self-contained project with its own `go.mod`, `Makefile`, and Shen specs. See each demo's README for instructions.

## Supported Harnesses

| Harness | Command | Notes |
|---------|---------|-------|
| Claude Code | `claude -p` | Default |
| Cursor | `cursor-agent -p` | Cursor's agent mode |
| Codex | `codex -p` | OpenAI Codex CLI |
| Rho | `rho-cli run --prompt` | [github.com/pyrex41/rho](https://github.com/pyrex41/rho) |
| Custom | Any CLI that reads stdin | Set `RALPH_HARNESS` env var |

## Design Decisions

- **Why Go for the orchestrator?** Fast compilation, trivial cross-compilation, `errgroup` for parallel gates, static binary output.
- **Why Shen over Coq/Lean/Agda?** Shen is Turing-complete (real programs, not just proofs), has a Lisp syntax (LLMs handle it well), and the type checker runs as a subprocess (no compilation step).
- **Why shell wrapper for shen-go?** `shen-go` loops on EOF instead of exiting cleanly. The wrapper script manages the subprocess lifecycle.
- **Why three gates instead of just Shen?** Defense in depth. Tests catch runtime bugs Shen can't see (I/O, timing). Shen catches logical bugs tests don't cover. Build catches syntax.
