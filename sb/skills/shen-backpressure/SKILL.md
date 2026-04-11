---
name: shen-backpressure
description: Formal backpressure for AI coding through Shen sequent-calculus types, shengen guard generation, and optional shen-derive spec-equivalence checks. Activates when the user mentions formal verification, Shen types, guard types, backpressure, invariant enforcement, or spec-vs-implementation verification. Works with any workflow — Ralph loops, CI, manual dev, or custom orchestrators.
user-invocable: false
---

# Shen-Backpressure

Formal type specs (Shen sequent calculus) + codegen bridge (shengen) that generates guard types with opaque constructors in any language with module-level visibility (Go, TypeScript, Rust, etc.). The generated types enforce domain invariants at compile time — you can't construct a value without proving its preconditions.

## Why This Works — Compiler Enforcement, Not LLM Policing

Guard types use the target language's **module-private fields** to make the compiler itself enforce invariants:

- **Go**: struct fields are lowercase (unexported) — code outside the package literally cannot construct the struct
- **TypeScript**: class fields are `private` with static factory — no way to instantiate without validation
- **Rust/Swift/Kotlin**: private fields with public factory methods — same pattern

When a function requires a guard type as input, the caller must have produced it through the constructor chain. If code tries to skip a step, **the build fails** — not because an LLM checked it, but because the compiler rejected it. The LLM writes code; the compiler enforces the proof chain. Gate 3 (build) catches violations automatically.

## How Enforcement Actually Works

Guard types enforce invariants through the **target language's own compiler** — not through LLM checking, linting, or runtime assertions bolted on after the fact. The mechanism is module-private fields:

- **Go**: struct fields are lowercase (unexported). Code outside the `shenguard` package cannot construct the struct directly — there is no syntax for it. The only path is through the generated constructor, which validates the spec's preconditions.
- **TypeScript**: class fields are `private` with a static factory method. Same effect — no way to create an instance without passing through validation.
- **Rust**: struct fields are `pub(crate)` or private. Same pattern.

This means if a function signature requires `TenantAccess`, the caller **must** have gone through `NewTenantAccess()`, which requires a valid `AuthenticatedUser` (which itself requires a valid `JwtToken` + unexpired `TokenExpiry`). There is no way to fake, skip, or shortcut the proof chain — the compiler rejects it.

**The LLM does not enforce any of this.** The LLM's job is to write code that compiles. If it tries to skip a step in the proof chain, `go build` (or `tsc`) fails in Gate 3, the error gets injected back as backpressure, and the LLM must fix it. The invariants are checked by the compiler, not by the LLM reading the code and deciding it looks right.

## Commands (Skills)

- `/sb:help` — Show all commands and when to use each one.
- `/sb:init` — Add Shen backpressure to any project. Specs, guard types, gates. No assumptions about workflow.
- `/sb:loop` — Configure and launch a Ralph loop (headless LLM + the core five gates, plus optional derive verification). Requires init first.
- `/sb:ralph-scaffold` — All-in-one: init + Ralph loop in a single flow.
- `/sb:derive` — Configure or run the optional shen-derive spec-equivalence gate for `(define ...)` functions.
- `/sb:create-shengen` — Build shengen for a new target language.

## Tooling Conventions

When carrying out these commands, prefer the current toolset explicitly:

- Use `ReadFile` for file reads, `rg` for content search, `Glob` for path discovery, and `Shell` for command execution.
- Use `ApplyPatch` for focused file edits and use scripts only for clearly mechanical or generated updates.
- Use `multi_tool_use.parallel` when independent reads or searches can run together.
- Prefer `sb gates`, `sb derive`, and the repo's `bin/` scripts over ad hoc verification commands once the project is configured.

## CLI Tool

The `sb` CLI is a thin launcher — the intelligence lives in these skills:
```
sb init      # Scaffold project (specs, scripts, skills)
sb gen       # Run shengen to generate guard types
sb gates     # Run the core five gates, plus shen-derive when configured
sb derive    # Run spec-equivalence drift checks for configured (define ...) specs
sb loop      # Launch Ralph loop (headless LLM + gates)
```

## Pipeline

```
specs/core.shen          Shen sequent-calculus type rules
       |
       v  (shengen — text parser, NOT a Shen interpreter)
Generated guard types    Private fields — compiler enforces constructors
       |
       v  (import)
Application code         Must use constructors — compiler enforces this
       |
       v  (gates — compiler catches violations)
Verification             shengen -> test -> build -> shen tc+ -> tcb audit
```

When the project configures any `[[derive.specs]]` entries in `sb.toml`, `sb gates` automatically appends an optional `shen-derive` verification gate after the core five. That gate regenerates a committed spec-derived test, fails on drift, and then runs `go test` on the referenced implementation packages.

### Gate 5: TCB Audit (`bin/shenguard-audit.sh`)
Re-runs shengen, diffs output against committed file, and rejects unexpected files in the shenguard package. Ensures the forgery boundary contains only generated code.

### Optional Gate 6: `shen-derive` (`sb derive`)
When `sb.toml` includes `[[derive.specs]]`, `sb derive` runs `shen-derive verify` for each entry, diffs the regenerated table-driven test against the committed file, and fails on drift. `sb derive --regen` rewrites the committed output when the drift is intentional.

The gates can run via `sb gates`, in a Ralph loop (`sb loop`), a CI pipeline, or manually — the verification is the same regardless of what triggers it. All gate commands are configurable via `sb.toml` or shell scripts in `bin/`.

## Spec Patterns

Beyond the basic wrapper/constrained/composite/guarded hierarchy, shengen supports:

### Sum Types (alternative constructors)
Multiple `(datatype ...)` blocks concluding to the same type produce a sum type:
- **Go**: interface with private marker method + concrete structs implementing it
- **TypeScript**: union type (`type Principal = HumanPrincipal | ServicePrincipal`)

```shen
(datatype human-principal
  User : authenticated-user;
  =============================
  User : authenticated-principal;)

(datatype service-principal
  Cred : service-credential;
  =============================
  Cred : authenticated-principal;)
```

### Set Membership (`element?`)
```shen
(element? Role [admin owner member]) : verified;
```
Generates idiomatic set membership checks per language.

### Helper Functions (`(define ...)`) — Go shengen only
Pattern-matching `define` blocks with `where` guards generate Go helper functions.

### Scoped DB Wrappers (`--db-wrappers`)
The `--db-wrappers <file>` flag generates proof-carrying DB wrappers that capture a verified ID at construction time, so all queries auto-scope.

## Shen Runtime for Gate 4

Gate 4 (shen tc+) needs a Shen implementation. Two recommended backends:

| Backend | Startup | Compute | Best for |
|---------|---------|---------|----------|
| **shen-sbcl** (shen-cl/SBCL) | **0.06s** | 1x | Gate loops, CI (startup-dominated) |
| **shen-scheme** (Chez Scheme) | 0.44s | **1.6x faster** | Large specs, heavy typechecking |

Install shen-sbcl: `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl`

`bin/shen-check.sh` auto-detects whichever is on PATH (prefers shen-sbcl for startup speed). Override with `SHEN=/path/to/binary` to use any backend. Do NOT use shen-go — it has known memory allocation crash bugs.

**Important:** shengen (the codegen tool) is a separate Go/TS program that reads `.shen` files as text and emits guard types. It does NOT run Shen code. Only Gate 4 needs an actual Shen runtime.

## Bypass Prevention & Hardened Mode

Standard guard types prevent **direct construction** (attack A) through module-private fields. Five total bypass vectors exist:

| Attack | What | Standard blocks? |
|--------|------|-----------------|
| A. Direct construction | `Amount{v: -5}` | Yes (all languages) |
| B. Field mutation | `(obj as any)._v = -5` | Go/Rust yes, TS/Py no |
| C. Zero-value | `var a Amount` (Go) | No |
| D. Reflection/unsafe | `reflect.NewAt(...)` | No |
| E. Deserialization | `json.Unmarshal(data, &amt)` | No |

**Hardened mode** (`shengen --mode hardened`) closes these additional vectors per language:

- **Go**: Sealed interfaces (unexported method), zero-value trap (`valid` flag), `UnmarshalJSON` re-validation
- **Rust**: No Clone/Copy on guarded types (linear proofs), `#[non_exhaustive]`, sealed traits, no Deserialize
- **TypeScript**: ES2022 `#private` fields (runtime enforcement), branded types (`unique symbol`), `Object.freeze`
- **Python**: Closure vaults (values in closure scope), HMAC provenance tokens, `__init_subclass__` prevention

The defense-in-depth stack (each layer catches what the previous misses):
1. Language-level opacity (module privacy)
2. Brands/sealing (nominal types, sealed interfaces)
3. Runtime validation (constructors check invariants)
4. Provenance tokens (HMAC, registries — Python)
5. Static analysis (lint gates in Gate 5)
6. TCB audit (hash verification of generated code)

See `/sb:create-shengen` section 13 for full implementation details.

## Target Language Shengen Implementations

Shengen codegen tools exist for multiple languages:

| Language | Tool | Location |
|----------|------|----------|
| Go | shengen (Go binary) | `cmd/shengen/main.go` |
| TypeScript | shengen-ts | `cmd/shengen-ts/shengen.ts` |
| Rust | shengen-rs (Python script) | `cmd/shengen-rs/shengen.py` |
| Python | shengen-py (Python script) | `cmd/shengen-py/shengen.py` |

All share the same architecture: Parse → Symbol Table → Resolve → Emit. All support `--mode standard|hardened`. Use `/sb:create-shengen` to build shengen for additional languages.
