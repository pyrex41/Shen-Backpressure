---
name: shen-backpressure
description: Formal backpressure for AI coding through Shen sequent-calculus types and a codegen bridge. Activates when the user mentions formal verification, Shen types, guard types, backpressure, or invariant enforcement. Works with any workflow — Ralph loops, CI, manual dev, or custom orchestrators.
user-invocable: false
---

# Shen-Backpressure

Formal type specs (Shen sequent calculus) + codegen bridge (shengen) that generates guard types with opaque constructors in Go or TypeScript. The generated types enforce domain invariants at compile time — you can't construct a value without proving its preconditions.

## Commands

- `/sb:help` — Show available commands and what they do.
- `/sb:init` — Add Shen backpressure to any project. Specs, guard types, gates. No assumptions about workflow.
- `/sb:loop` — Configure and launch a Ralph loop (autonomous LLM harness). Requires init first.
- `/sb:ralph-scaffold` — All-in-one: init + Ralph loop in a single flow.
- `/sb:create-shengen` — Build shengen for a new target language.

## How It Works

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
Verification             shengen -> test -> build -> shen tc+
```

The gates can run in a Ralph loop, CI pipeline, or manually — the verification is the same regardless of what triggers it.

## Shen Runtime for Gate 4

Gate 4 (shen tc+) needs a Shen implementation. Use **shen-sbcl** (Shen on SBCL/Common Lisp) — most reliable, fastest startup.

Install: `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl`

Do NOT use shen-go — it has known memory allocation crash bugs.

**Important:** shengen (the codegen tool) is a separate Go/TS program that reads `.shen` files as text and emits guard types. It does NOT run Shen code. Only Gate 4 needs an actual Shen runtime.
