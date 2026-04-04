---
date: 2026-03-31T00:00:00-07:00
researcher: reuben
git_commit: 1086adf
branch: main
repository: Shen-Backpressure
topic: "Full codebase exploration: what we have and how it works"
tags: [research, codebase, overview, shengen, sb-cli, demos, skills, blog, architecture]
status: complete
last_updated: 2026-03-31
last_updated_by: reuben
---

# Research: Full Codebase Exploration

**Date**: 2026-03-31
**Researcher**: reuben
**Git Commit**: 1086adf
**Branch**: main
**Repository**: [pyrex41/Shen-Backpressure](https://github.com/pyrex41/Shen-Backpressure)

## Research Question

Explore what we have here — how does it work?

## Summary

Shen-Backpressure is a framework for adding **formal verification backpressure** to AI coding loops. The core pipeline: Shen sequent-calculus specs define domain invariants → a codegen bridge (**shengen**) generates opaque guard types in Go/TS/Rust/Python → the target language's compiler enforces proof chains → a five-gate verification pipeline catches violations → failures feed back as errors into the LLM's next prompt.

The repository contains:
- **4 codegen tools** (Go, TypeScript, Python, Rust shengen implementations)
- **1 CLI orchestrator** (`sb` — thin launcher for gates, loop, init, gen)
- **5 demo projects** spanning payments, email campaigns, multi-tenant auth, clinical dosage, and generative UI
- **1 SKM skill bundle** with 5 slash commands and an auto-activated skill
- **1 Hugo blog site** with 7 published posts
- **8 research documents** in `thoughts/shared/research/`
- **Reference guard type outputs** for Go, TypeScript, Python (standard + hardened), Rust (standard + hardened)

## Detailed Findings

### 1. The Core Idea: Spec → Codegen → Compiler Enforcement → Backpressure

```
specs/core.shen              Shen sequent-calculus type rules
       |
       v  (shengen)
internal/shenguard/          Generated guard types (Go, TS, Rust, Python)
       |
       v  (import)
Application code             Uses guard types at domain boundaries
       |
       v  (five gates)
Verification                 shengen → test → build → shen tc+ → tcb audit
       |
       v  (fail?)
Backpressure                 Gate errors fed back to LLM/CI/developer
```

Guard types have **module-private fields** (Go: unexported, TS: `private`, Rust: `pub(crate)`, Python: closure vaults). The ONLY way to create a value is through the constructor, which validates the spec's preconditions. The compiler enforces this — not the LLM.

### 2. Codegen Tools (`cmd/`)

#### `cmd/shengen/` — Go Shengen (1,886 lines + 589 lines tests)

The primary codegen tool. Pure Go, no external dependencies. Parses `.shen` files, extracts `(datatype ...)` and `(define ...)` blocks, classifies each into five categories (wrapper, constrained, composite, guarded, alias), resolves accessor chains through nested `head`/`tail` expressions, and emits Go structs with unexported fields and validated constructors.

Key architecture:
- **AST**: `Premise`, `VerifiedPremise`, `Conclusion`, `Rule`, `Datatype`, `Define`
- **Symbol table**: Three-pass build — count conclusion producers → classify types → register sum types
- **S-expression parser**: Tokenizes and recursively parses Shen expressions for verified premises
- **Accessor chain resolver**: Translates `(head Tx)` → `tx.Amount()`, `(tail (tail (head Profile)))` → structural match fallback
- **Define handler**: Emits iterative Go functions from Shen pattern-matching `define` blocks

Supports `--spec`, `--pkg`, `--out`, `--db-wrappers` flags. Also generates scoped DB wrapper types that auto-scope queries by capturing verified IDs.

#### `cmd/sb/` — CLI Orchestrator (v0.2.0)

Thin launcher with four subcommands:
- `sb init` — scaffolds a new project (specs, scripts, skills)
- `sb gen` — runs shengen to generate guard types
- `sb gates` — runs 5 configurable gate commands from `sb.toml`
- `sb loop` — launches a Ralph loop (headless LLM + five-gate verification)

Config loads from `sb.toml` with convention defaults and env var overrides (`RALPH_HARNESS`, `RALPH_MAX_ITER`, `RALPH_HARNESS_TIMEOUT`). `sb loop --dry-run` emits a standalone bash script. Timeout uses Go's `context.WithTimeout` (cross-platform).

`cmd/sb/skilldata/` contains embedded copies of the skill bundle, synced via `cmd/sb/Makefile` `sync-skills` target.

#### `cmd/shengen-py/` — Python Shengen (745 lines)

Python script that generates Python guard types. Supports `--mode standard|hardened`. Hardened mode emits closure vaults (values in closure scope, not object attributes), HMAC provenance tokens, and `__init_subclass__` prevention.

#### `cmd/shengen-rs/` — Rust Shengen (647 lines)

Python script (despite living in `cmd/shengen-rs/`) that generates Rust guard types. Supports `--mode standard|hardened`. Hardened mode emits `#[non_exhaustive]`, no Clone/Copy on guarded types, sealed traits for sum types.

Note: `cmd/shengen-ts/` is referenced in docs but not present at repo root — TypeScript shengen may live within demo projects or be generated per-project.

### 3. The Five Gates

| Gate | Command | What it catches |
|------|---------|----------------|
| 1. shengen | `./bin/shengen-codegen.sh` | Stale guard types (spec drift) |
| 2. test | `go test ./...` (configurable) | Runtime invariant violations |
| 3. build | `go build ./...` (configurable) | Type signature mismatches from spec changes |
| 4. shen tc+ | `./bin/shen-check.sh` | Contradictory/inconsistent specs |
| 5. tcb audit | `./bin/shenguard-audit.sh` | Hand-edited generated code, unexpected files |

Gates 1 and 4 verify the formal foundation (synchronized and consistent). Gates 2 and 3 verify the implementation (tested and compiled). Gate 5 ensures the trusted computing base contains only generated code.

### 4. Demo Projects (`demo/`)

#### `demo/payment/` — Payment Processor (simplest)
- **Domain**: Balance invariant — transfer only if balance covers amount
- **Shen types**: `account-id`, `amount` (>= 0), `transaction`, `balance-checked`, `safe-transfer`
- **Stack**: Go, 3-gate Ralph loop
- **Status**: Partially complete (some plan items unchecked)
- **Role**: Reference example for the basic pattern

#### `demo/email_crud/` — Email Campaign Personalization
- **Domain**: Personalized copy only for users with known demographics
- **Shen types**: `age-decade` (10-100 by tens), `us-state` (2 chars), `demographics`, `known-profile`, `copy-delivery` (enforces demographics match), `safe-copy-view` (sum type: direct or prompt-flow)
- **Stack**: Go, bash Ralph loop (`run-ralph.sh`), browser automation (`.rodney/`)
- **Status**: Complete (all plan items checked)
- **Special**: Does not use shengen — invariants reflected manually in Go

#### `demo/multi-tenant-api/` — Multi-Tenant SaaS Authorization (most developed)
- **Domain**: JWT → AuthenticatedUser → TenantAccess → ResourceAccess proof chain
- **Shen types**: `jwt-token`, `token-expiry`, `authenticated-user`, `authenticated-principal` (sum type: human | service), `tenant-access` (isMember=true), `resource-access` (isOwned=true)
- **Stack**: Go, 5-gate Ralph loop, SQLite, admin dashboard
- **Status**: Complete (all 8 plan items checked), includes `demo.md` with live curl output
- **Special**: Sum type generation (Go interfaces), full Showboat-verified demo, build transcript in `transcript/`

#### `demo/dosage-calculator/` — Clinical Dosage Calculator
- **Domain**: Safe drug administration requiring both dose-in-range and allergy-clear proofs
- **Shen types**: Includes `(define ...)` helper functions for list-based allergy checking (`has-allergy`, `all-allergies-clear`)
- **Stack**: Go, 5-gate via `sb` CLI commands
- **Status**: Has `.claude/commands/sb/` installed
- **Special**: Most complex Shen spec — uses `define` blocks with pattern matching, not just datatypes

#### `demo/shen-prolog-ui/` — Generative UI Dashboard
- **Domain**: Configurable dashboard builder where Shen Prolog resolves layout constraints
- **Stack**: Go backend + Arrow.js frontend + shengen-ts + Pretext measurement layer
- **Status**: Prompt only (`DEMO_START_PROMPT.md`), not yet built
- **Special**: Three-layer pipeline — Shen proves layout validity, Pretext pre-calculates text dimensions (zero DOM reflow), Arrow renders reactively

### 5. SKM Skill Bundle (`sb/`)

Registered via `skm.toml` as bundle name `sb`. Contains:

#### AGENT_PROMPT.md — Inner LLM Reference
The prompt injected into the inner harness LLM each iteration. Teaches four guard type discipline rules (wrap at boundary, trust internally, follow proof chain, extract with accessors), documents all five spec patterns with per-language constructor signatures (Go, TS, Rust), lists anti-patterns, and describes hardened mode.

#### 5 Slash Commands
| Command | File | Purpose |
|---------|------|---------|
| `/sb:help` | `commands/help.md` | Complete command reference |
| `/sb:init` | `commands/init.md` | Add backpressure to any project (8 steps) |
| `/sb:loop` | `commands/loop.md` | Configure and launch Ralph loop |
| `/sb:ralph-scaffold` | `commands/ralph-scaffold.md` | All-in-one: init + loop (9 steps) |
| `/sb:create-shengen` | `commands/create-shengen.md` | Build shengen for any target language — full grammar, classification algorithm, per-language enforcement, hardened mode (section 13) |

#### Auto-Activated Skill
`skills/shen-backpressure/SKILL.md` — activates on mentions of formal verification, Shen types, guard types, or backpressure. Contains the proof chain explanation, bypass taxonomy (attacks A-E), defense-in-depth stack (6 layers), and target language shengen table.

### 6. Reference Examples (`examples/`)

| Directory | Language | Contents |
|-----------|----------|----------|
| `examples/payment/` | Go | `guards_gen.go`, `guards_gen_test.go` |
| `examples/email_crud/` | Go | `guards_gen.go`, `guards_gen_test.go` |
| `examples/payment_ts/` | TypeScript | `guards_gen.ts` |
| `examples/email_crud_ts/` | TypeScript | `guards_gen.ts` |
| `examples/payment_py/` | Python | `guards_gen.py` (standard), `guards_gen_hardened.py` |
| `examples/payment_rs/` | Rust | `guards_gen.rs` (standard), `guards_gen_hardened.rs` |

### 7. Shell Scripts (`bin/`)

| Script | Purpose |
|--------|---------|
| `bin/shengen` | Compiled Go shengen binary (4.9 MB) |
| `bin/sb` | Compiled sb CLI binary |
| `bin/shengen-codegen.sh` | Gate 1 wrapper — finds/builds shengen, runs it |
| `bin/shenguard-audit.sh` | Gate 5 — checks for unexpected files, diffs regenerated output |

### 8. Hugo Blog Site (`site/`)

Hugo with Ananke theme. 7 published posts:

| Post | Topic |
|------|-------|
| `impossible-by-construction` | Multi-tenant auth proof chain (the foundational post) |
| `one-spec-every-language` | Cross-language enforcement spectrum |
| `bypass-taxonomy` | Five bypass attack vectors (A-E) |
| `go-hardening` | Sealed interfaces, zero-value traps, UnmarshalJSON |
| `python-closure-vaults` | HMAC provenance tokens, closure-based guards |
| `rust-linear-proofs` | No Clone/Copy, #[non_exhaustive], sealed traits |
| `typescript-branded-nominals` | #private fields, branded types, Object.freeze |

### 9. Research Documents (`thoughts/shared/research/`)

8 documents spanning 2026-03-23 through 2026-03-29:
- Codebase overview, shengen codegen bridge analysis
- Blog series positioning, shengen improvement research
- Implementation log (what was actually built after scud review)
- CLI vs skill framework analysis
- Closures/opaque types idea space exploration
- Cross-language enforcement spectrum

## Architecture Documentation

### The Guard Discipline Pattern

At system boundaries (HTTP handlers, CLI commands, message consumers):
1. Parse raw input (strings, floats)
2. Immediately construct guard types (`NewAmount(raw)`)
3. Constructor validates → error if invariant violated
4. Internal code only accepts guard types, never raw primitives
5. To get raw values back: `.Val()` on wrappers, accessor methods on composites

### The Proof Chain

Types form a dependency graph. `SafeTransfer` requires `BalanceChecked`, which requires `Transaction` and a balance value. `Transaction` requires `Amount` (non-negative) and two `AccountId`s. You cannot skip a step — the compiler enforces the chain.

### The Backpressure Loop

```
LLM Harness → Code Changes → Gate 1-5 → Pass? → Done
                                          ↓ Fail
                                 Error injection into prompt → LLM Harness
```

The loop terminates when all five gates pass AND no unchecked plan items remain.

### Hardened Mode (--mode hardened)

Extends standard guard types with additional bypass prevention:
- **Go**: Sealed interfaces, zero-value traps, UnmarshalJSON re-validation
- **Rust**: No Clone/Copy on guarded types, #[non_exhaustive], sealed traits
- **TypeScript**: ES2022 #private fields, branded types, Object.freeze
- **Python**: Closure vaults, HMAC provenance tokens, __init_subclass__ prevention

## Code References

- [`README.md`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/README.md) — Project overview and quick start
- [`cmd/shengen/main.go`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/cmd/shengen/main.go) — Go shengen (1,886 lines)
- [`cmd/sb/main.go`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/cmd/sb/main.go) — sb CLI entry point
- [`cmd/sb/loop.go`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/cmd/sb/loop.go) — Ralph loop implementation
- [`cmd/sb/gates.go`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/cmd/sb/gates.go) — Five-gate runner
- [`cmd/sb/config.go`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/cmd/sb/config.go) — sb.toml loader with convention defaults
- [`cmd/shengen-py/shengen.py`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/cmd/shengen-py/shengen.py) — Python shengen (745 lines)
- [`cmd/shengen-rs/shengen.py`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/cmd/shengen-rs/shengen.py) — Rust shengen (647 lines)
- [`sb/AGENT_PROMPT.md`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/sb/AGENT_PROMPT.md) — Inner LLM reference
- [`sb/commands/create-shengen.md`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/sb/commands/create-shengen.md) — Language-agnostic shengen builder guide
- [`sb/skills/shen-backpressure/SKILL.md`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/sb/skills/shen-backpressure/SKILL.md) — Auto-activated skill
- [`demo/multi-tenant-api/specs/core.shen`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/demo/multi-tenant-api/specs/core.shen) — Most complex spec (sum types, proof chains)
- [`demo/dosage-calculator/specs/core.shen`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/demo/dosage-calculator/specs/core.shen) — Spec with define blocks
- [`bin/shenguard-audit.sh`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/bin/shenguard-audit.sh) — Gate 5 TCB audit
- [`blog/post-3-impossible-by-construction/post.md`](https://github.com/pyrex41/Shen-Backpressure/blob/1086adf/blog/post-3-impossible-by-construction/post.md) — Foundational blog post

## Historical Context (from thoughts/)

- `thoughts/shared/research/2026-03-23-codebase-overview.md` — First orientation research; documented the original 3-gate loop before it became 5 gates
- `thoughts/shared/research/2026-03-24-shengen-codegen-bridge.md` — Deep dive into shengen's parsing and symbol table
- `thoughts/shared/research/2026-03-28-shengen-improvement-research.md` — Evaluated phantom types (rejected), TCB reduction (Gate 5 audit + TenantDB), and ergonomics (sum type codegen)
- `thoughts/shared/research/2026-03-28-cli-vs-skill-framework.md` — Answered "CLI or skills?" with "both" — CLI for deterministic ops, skills for LLM conceptual guidance
- `thoughts/shared/research/2026-03-28-implementation-log.md` — Records what was built after scud review of blog post 3
- `thoughts/shared/research/2026-03-28-blog-series-deterministic-backpressure.md` — Blog positioning strategy
- `thoughts/shared/research/2026-03-29-closures-opaque-types-shen-idea-space.md` — Explores closures as enforcement mechanism for weak-opacity languages
- `thoughts/shared/research/2026-03-29-cross-language-enforcement-spectrum.md` — Three-tier language ranking by compile-time opacity strength

## Open Questions

1. `cmd/shengen-ts/` is referenced in `SKILL.md` but does not exist at repo root — TypeScript shengen may be per-project or missing
2. `demo/email_crud/` does not use shengen (invariants reflected manually) — is this intentional or an older demo predating shengen?
3. `demo/shen-prolog-ui/` is prompt-only — not yet built
4. `demo/payment/` still has unchecked plan items
5. Blog site `baseURL` is `http://localhost:1313/` — not configured for production hosting
