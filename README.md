# Shen-Backpressure

Formal backpressure for AI coding through **Shen sequent-calculus types** and a **codegen bridge** that generates guard types in Go or TypeScript. Works with any workflow — Ralph loops, CI pipelines, manual dev, or custom orchestrators. Installable via [SKM](https://github.com/pyrex41/skill-manager) or manually as Claude Code skills.

## The Idea

Most AI coding loops use tests as the only gate. Tests are empirical — they check specific cases. This project adds two deductive gates:

1. **Shen type checking** — sequent-calculus proofs that must hold for *all* cases
2. **shengen codegen** — generated Go types with opaque constructors that enforce those proofs at compile time

If the LLM breaks an invariant, either the type check fails (Shen) or the code won't compile (Go). This is **backpressure** — the system rejects invalid states before they accumulate.

```
specs/core.shen          Shen sequent-calculus type rules
       |
       v  (shengen)
internal/shenguard/      Generated guard types (Go or TypeScript)
       |
       v  (import)
Application code         Uses guard types at domain boundaries
       |
       v  (gates)
Verification             shengen -> test -> build -> shen tc+ -> tcb audit
       |
       v  (fail?)
Backpressure             Gate errors fed back (to LLM, CI, or developer)
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

```bash
mkdir -p .claude/commands/sb .claude/skills/shen-backpressure
cp Shen-Backpressure/sb/commands/*.md .claude/commands/sb/
cp Shen-Backpressure/sb/skills/shen-backpressure/SKILL.md .claude/skills/shen-backpressure/
```

## Commands

| Command | What it does |
|---------|-------------|
| `/sb:init` | Add Shen backpressure to any project — specs, shengen, guard types, gates. Works with any workflow. |
| `/sb:loop` | Configure and launch a Ralph loop (autonomous LLM harness with five-gate backpressure). Requires init. |
| `/sb:ralph-scaffold` | All-in-one: init + loop setup in a single flow. For people who know they want Ralph. |
| `/sb:create-shengen` | Build a shengen codegen tool for any target language (Go, Rust, TS, Python, Java, etc.) |

### Quick Start

```
> /sb:init       # add Shen backpressure (works with any workflow)
```

Want a Ralph loop?
```
> /sb:ralph-scaffold   # all-in-one: init + Ralph loop setup
```

Or step by step:
```
> /sb:init       # specs, guard types, gates
> /sb:loop       # configure Ralph, launch
```

## The Five Gates

Every iteration of the Ralph loop must pass all five gates:

| Gate | Command | What it catches |
|------|---------|----------------|
| 1. shengen | `./bin/shengen-codegen.sh` | Regenerates Go guard types from spec. Catches stale types. |
| 2. go test | `go test ./...` | Tests against regenerated types. Catches runtime invariant violations. |
| 3. go build | `go build ./cmd/server` | Compiles against regenerated types. Catches type signature mismatches. |
| 4. shen tc+ | `./bin/shen-check.sh` | Verifies spec internal consistency. Catches contradictory rules. |
| 5. tcb audit | `./bin/shenguard-audit.sh` | Re-runs shengen, diffs output, rejects unexpected files in shenguard/. |

### The Codegen Bridge (shengen)

shengen parses `specs/core.shen` and emits Go types with **unexported fields** and **validated constructors**. You can't create a guard type without going through its constructor, and the constructor enforces the spec's invariants.

```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

Becomes:

```go
type BalanceChecked struct {
    bal float64
    tx  Transaction
}

func NewBalanceChecked(bal float64, tx Transaction) (BalanceChecked, error) {
    if !(bal >= tx.amount.Val()) {
        return BalanceChecked{}, fmt.Errorf("bal must be >= tx.amount")
    }
    return BalanceChecked{bal: bal, tx: tx}, nil
}

func (t BalanceChecked) Bal() float64       { return t.bal }
func (t BalanceChecked) Tx() Transaction    { return t.tx }
```

The LLM cannot bypass this — `Amount{v: 50}` won't compile (unexported `v`), `BalanceChecked{bal: 0, tx: tx}` won't compile either (unexported fields), and `SafeTransfer` requires a `BalanceChecked` proof that can only come from `NewBalanceChecked`.

### Guard Type Patterns

| Shen pattern | Go output | Constructor |
|-------------|-----------|-------------|
| Wrapper (`X : string; ==> X : account-id`) | `struct{ v string }` | `NewAccountId(string) AccountId` |
| Constrained (`(>= X 0) : verified`) | `struct{ v float64 }` | `NewAmount(float64) (Amount, error)` |
| Composite (`[A B C] : transaction`) | `struct{ a, b, c }` + accessors | `NewTransaction(A, B, C) Transaction` |
| Guarded (`(>= Bal (head Tx)) : verified`) | `struct{ bal, tx }` + accessors | `NewBalanceChecked(...) (BalanceChecked, error)` |
| Proof chain (`Check : balance-checked`) | `struct{ tx, check }` + accessors | `NewSafeTransfer(Transaction, BalanceChecked) SafeTransfer` |
| Sum type (multiple blocks → same conclusion) | Go interface + concrete structs | `AuthenticatedPrincipal` = `HumanPrincipal \| ServicePrincipal` |

## Demos

- **[`demo/payment/`](demo/payment/)** — Payment processor with balance invariants
- **[`demo/email_crud/`](demo/email_crud/)** — Personalized email campaigns with demographic-based copy

Reference guard type output in [`examples/`](examples/).

## Project Structure

```
cmd/shengen/             Codegen tool source (stdlib only)
sb/                      SKM bundle
  commands/              /sb:ralph-scaffold, /sb:setup, /sb:init, /sb:loop
  skills/                Auto-activated skill description
  AGENT_PROMPT.md        Reference manual for inner LLM harness
examples/                Reference shengen output for each domain
demo/                    Working demo projects
```

## Supported Harnesses

| Harness | Command |
|---------|---------|
| Claude Code | `claude -p` (default) |
| Cursor | `cursor-agent -p` |
| Codex | `codex -p` |
| Rho | `rho-cli run --prompt` |
| Custom | Set `RALPH_HARNESS` env var |

## Further Reading

- **[Don't Waste Your Backpressure](https://banay.me/dont-waste-your-backpressure/)** — The core principle behind this project. AI agents that work autonomously need automated feedback on quality and correctness. Without capturing backpressure metrics, you can't delegate longer-horizon tasks with confidence. Shen-Backpressure makes this concrete: every iteration gets formal verification feedback, not just test results.

- **[Ralph](https://ghuntley.com/ralph/)** — The technique this project implements. Ralph is a bash loop that repeatedly calls an LLM harness (`while :; do cat PROMPT.md | claude-code; done`). The LLM does the coding, the loop provides structure and error recovery. This project adds Shen type checking and codegen guards as backpressure within that loop.

- **[The Loop](https://ghuntley.com/loop/)** — Why loop-based development changes the economics of software. Instead of building brick-by-brick, you allocate specs and goals to an autonomous agent and let it cycle. The key insight: watch the loop itself — failures become learning opportunities fed back as backpressure, not dead ends.

## Shen Runtime (Gate 4)

Gate 4 (`shen tc+`) is the only gate that needs a Shen runtime. The `bin/shen-check.sh` script auto-detects and supports multiple backends:

| Backend | Startup | Compute | Install |
|---------|---------|---------|---------|
| **shen-sbcl** (default) | **0.06s** | 1x | `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl` |
| **shen-scheme** | 0.44s | **1.6x faster** | Build from [shen-scheme](https://github.com/Shen-Language/shen-scheme) |

For gate loops and CI, shen-sbcl wins — startup dominates on small specs. For large specs with heavy typechecking, shen-scheme's faster compute may matter. Override with `SHEN=/path/to/binary ./bin/shen-check.sh`.

shengen (the codegen tool) does NOT use a Shen runtime — it's a standalone parser that reads `.shen` files as text.

### Installing Shen Runtimes (for agents and CI)

**shen-cl (SBCL)** — recommended, fastest startup:

```bash
brew tap Shen-Language/homebrew-shen && brew install shen-sbcl
```

**shen-scheme (Chez Scheme)** — faster compute for large specs:

```bash
# Install Chez Scheme first
brew install chezscheme

# Build shen-scheme from source
git clone https://github.com/Shen-Language/shen-scheme.git
cd shen-scheme
make
sudo cp bin/shen-scheme /usr/local/bin/
```

Verify either runtime works with `shen-check.sh`:

```bash
./bin/shen-check.sh specs/core.shen
```

To force a specific backend: `SHEN=/path/to/binary ./bin/shen-check.sh`.

## Design Decisions

- **Why shengen?** Shen proves invariants deductively but doesn't generate Go code. shengen bridges the gap — the formal spec becomes compile-time enforcement via opaque types.
- **Why five gates?** Gate 1 (shengen) ensures generated types stay in sync with specs. Gate 2 (tests) catches runtime violations. Gate 3 (build) catches type mismatches from spec changes. Gate 4 (shen tc+) catches inconsistent specs. Gate 5 (tcb audit) ensures the forgery boundary contains only generated code. No gap.
- **Why opaque constructors?** Unexported `v` fields mean the Go compiler enforces the spec. You literally cannot create an `Amount` without going through `NewAmount`, which validates `>= 0`.
- **Why Go for the orchestrator?** Fast compilation, `errgroup` for parallel gates, static binary.
- **Why Shen over Coq/Lean/Agda?** Turing-complete, Lisp syntax LLMs handle well, runs as subprocess.
