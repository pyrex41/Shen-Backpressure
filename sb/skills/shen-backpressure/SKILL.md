---
name: shen-backpressure
description: Formal backpressure for AI coding through Shen sequent-calculus types and a codegen bridge. Activates when the user mentions formal verification, Shen types, guard types, backpressure, or invariant enforcement. Works with any workflow — Ralph loops, CI, manual dev, or custom orchestrators.
user-invocable: false
---

# Shen-Backpressure

Formal type specs (Shen sequent calculus) + codegen bridge (shengen) that generates guard types with opaque constructors in Go or TypeScript. The generated types enforce domain invariants at compile time — you can't construct a value without proving its preconditions.

## How Enforcement Actually Works

Guard types enforce invariants through the **target language's own compiler** — not through LLM checking, linting, or runtime assertions bolted on after the fact. The mechanism is module-private fields:

- **Go**: struct fields are lowercase (unexported). Code outside the `shenguard` package cannot construct the struct directly — there is no syntax for it. The only path is through the generated constructor, which validates the spec's preconditions.
- **TypeScript**: class fields are `private` with a static factory method. Same effect — no way to create an instance without passing through validation.
- **Rust**: struct fields are `pub(crate)` or private. Same pattern.

This means if a function signature requires `TenantAccess`, the caller **must** have gone through `NewTenantAccess()`, which requires a valid `AuthenticatedUser` (which itself requires a valid `JwtToken` + unexpired `TokenExpiry`). There is no way to fake, skip, or shortcut the proof chain — the compiler rejects it.

**The LLM does not enforce any of this.** The LLM's job is to write code that compiles. If it tries to skip a step in the proof chain, `go build` (or `tsc`) fails in Gate 3, the error gets injected back as backpressure, and the LLM must fix it. The invariants are checked by the compiler, not by the LLM reading the code and deciding it looks right.

## Commands

- `/sb:help` — Show all commands and when to use each one.
- `/sb:init` — Add Shen backpressure to any project. Specs, guard types, gates. No assumptions about workflow.
- `/sb:loop` — Configure and launch a Ralph loop (autonomous LLM harness). Requires init first.
- `/sb:ralph-scaffold` — All-in-one: init + Ralph loop in a single flow.
- `/sb:create-shengen` — Build shengen for a new target language.

## Pipeline

```
specs/core.shen          Shen sequent-calculus type rules
       |
       v  (shengen — text parser, NOT a Shen interpreter)
internal/shenguard/      Generated guard types with private fields
       |
       v  (import)
Application code         Must use constructors — compiler enforces this
       |
       v  (gates — compiler catches violations)
Verification             shengen -> test -> build -> shen tc+ -> tcb audit
```

### Gate 5: TCB Audit (`bin/shenguard-audit.sh`)
Re-runs shengen, diffs output against committed file, and rejects unexpected files in the shenguard package. Ensures the forgery boundary contains only generated code.

The gates can run in a Ralph loop, CI pipeline, or manually — the verification is the same regardless of what triggers it.

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
