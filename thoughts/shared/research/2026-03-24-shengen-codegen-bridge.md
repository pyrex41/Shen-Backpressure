---
date: 2026-03-24T00:00:00-07:00
researcher: reuben
git_commit: 7a671be0c920de5bfce337243728b316a831e09b
branch: main
repository: Shen-Backpressure
topic: "Shengen codegen bridge: architecture, implementation, and integration with existing repo"
tags: [research, codebase, shengen, codegen, guard-types, backpressure]
status: complete
last_updated: 2026-03-24
last_updated_by: reuben
---

# Research: Shengen Codegen Bridge

**Date**: 2026-03-24
**Researcher**: reuben
**Git Commit**: 7a671be0c920de5bfce337243728b316a831e09b
**Branch**: main
**Repository**: Shen-Backpressure

## Research Question

Understanding the shengen codegen tool and how it bridges Shen sequent-calculus specs to Go's type system, plus how the provided files relate to the existing repo structure.

## Summary

**shengen** is a Go codegen tool that parses `specs/core.shen` and emits a Go package with opaque types whose constructors enforce the same invariants Shen proves deductively. This adds a fourth gate to the Ralph loop: before tests, build, and Shen type check, shengen regenerates Go guard types from the spec. The generated types have unexported fields — the only way to create them is through validated constructors. This makes the Shen spec enforceable at Go compile time, not just as a separate verification step.

Five files were provided (unzipped from `files.zip`), currently sitting at the repo root:

| File | Purpose | Lines |
|------|---------|-------|
| `shengen.go` | The codegen tool (`package main`) | 978 |
| `guards_gen.go` | Generated output for email_crud domain (`package shenguard`) | 219 |
| `guards_gen_test.go` | Tests for email_crud guard types | 135 |
| `payment_guards_gen.go` | Generated output for payment domain (`package payment`) | 103 |
| `payment_guards_gen_test.go` | Tests for payment guard types | 78 |

## Detailed Findings

### 1. shengen.go — The Codegen Tool

A single-file Go program (`package main`, 978 lines) with five major components:

**1a. AST types** (lines 27-51): `Premise`, `VerifiedPremise`, `Conclusion`, `Rule`, `Datatype` — representing parsed Shen datatype blocks.

**1b. Symbol table** (lines 57-151): Maps each Shen type to its Go name, category, field layout, and wrapped primitive. Categories:
- **wrapper**: simple newtype (e.g. `string → AccountId`), no validation
- **constrained**: validated primitive (e.g. `number → AgeDecade` with `>= 10, <= 100, mod 10 == 0`)
- **composite**: multi-field struct (e.g. `Transaction{Amount, From, To}`)
- **guarded**: composite with `verified` premises (e.g. `BalanceChecked` checks `bal >= tx.Amount`)
- **alias**: `type X = Y` when one custom type simply wraps another

Two-pass name resolution handles:
- Block name != conclusion type (e.g. `datatype balance-invariant` → conclusion type `balance-checked`)
- Multiple blocks producing the same conclusion type (e.g. `safe-copy-view` and `safe-copy-view-from-prompt` both → `safe-copy-view`)

**1c. S-expression parser** (lines 156-232): Tokenizes and parses Shen's s-expressions into a recursive `SExpr` tree. Used to interpret `verified` premises.

**1d. Accessor chain resolver** (lines 238-587): The most complex part. Translates Shen's `(head X)`, `(tail X)`, and nested combinations into Go field access:
- `(head Tx)` → first field of Tx's type → `tx.Amount`
- `(tail (tail (head Profile)))` → uses structural match fallback when direct resolution fails
- Unwraps wrapper types: if a field is `Amount` (wrapper around `float64`), accessor generates `.Val()`
- Handles `shen.mod`, `length`, `not`, `element?`, and comparison operators

**Structural match fallback** (lines 460-518): When head/tail chains can't be directly resolved (e.g. complex equality like `(= (tail (tail (head Profile))) (tail Copy))`), finds shared non-primitive field types between two composites. Example: `KnownProfile` has `Demo:demographics`, `CopyContent` has `Demo:demographics` → generates `profile.Demo == copy.Demo`.

**1e. Go code generator** (lines 792-916): Emits one Go type per Shen datatype:
- Wrappers: `type X struct{ v T }` + `NewX(T) X` + `Val() T`
- Constrained: same but `NewX(T) (X, error)` with validation checks
- Composites: `type X struct { Field1 T1; Field2 T2 }` + `NewX(T1, T2) X`
- Guarded: like composite but `NewX(...) (X, error)` with `verified` premise checks
- Aliases: `type X = Y`

**Main** (lines 922-977): `shengen [spec-path] [package-name]`, outputs Go to stdout, debug info to stderr.

### 2. guards_gen.go — Email CRUD Domain Output

Generated from `demo/email_crud/specs/core.shen` into `package shenguard`. Contains 13 types:
- Wrappers: `EmailAddr`, `UserId`, `EmailId`
- Constrained: `AgeDecade` (10-100, mod 10), `UsState` (len == 2)
- Composites: `Demographics`, `KnownProfile`, `UnknownProfile`, `CopyContent`, `CampaignEmail`, `ProfileUpgrade`, `SafeCopyViewFromPrompt`
- Guarded: `CopyDelivery` (enforces `profile.Demo == copy.Demo`)
- Aliases: `PromptRequired = UnknownProfile`, `SafeCopyView = CopyDelivery`

### 3. guards_gen_test.go — Email CRUD Tests

Tests for the email_crud guard types (`package shenguard_test`):
- `TestAgeDecadeConstraints`: validates all decades 10-100, rejects 5, 25, 110
- `TestUsStateConstraints`: validates "MN", rejects "Minnesota" and ""
- `TestCopyDeliveryRequiresDemographicMatch`: the key invariant — matching demographics succeed, mismatched demographics fail
- `TestProfileUpgradeFlow`: full flow unknown → prompt → upgrade → safe-copy-view
- `TestCannotBypassConstructor`: documents that unexported `v` field prevents struct literal bypass

### 4. payment_guards_gen.go — Payment Domain Output

Generated from `demo/payment/specs/core.shen` into `package payment`. Contains 6 types:
- Wrapper: `AccountId`
- Constrained: `Amount` (>= 0)
- Composite: `Transaction`, `AccountState`, `SafeTransfer`
- Guarded: `BalanceChecked` (enforces `bal >= tx.Amount.Val()`)

### 5. payment_guards_gen_test.go — Payment Tests

Tests for the payment guard types:
- `TestAmountMustBeNonNegative`: rejects -10, accepts 100
- `TestBalanceCheckedRequiresSufficientFunds`: the key invariant — balance 100 covers amount 50, balance 50 covers amount 50, balance 30 rejects amount 50
- `TestSafeTransferRequiresBalanceCheck`: proof chain — must get `BalanceChecked` before `SafeTransfer`

## Architecture Documentation

### The Four-Gate Closed Loop

With shengen, the Ralph loop gains a fourth gate that runs FIRST:

```
Gate 1: shengen    → regenerate guards_gen.go from specs/core.shen
Gate 2: go test    → test against regenerated types (catches runtime invariant violations)
Gate 3: go build   → compile against regenerated types (catches type mismatches)
Gate 4: shen tc+   → verify spec is internally consistent
```

The critical property: if the LLM changes the spec, Gate 1 regenerates the types, and any Go code using old signatures breaks at Gate 3. If the LLM writes code that skips a guard constructor, it can't construct downstream types (Go compiler enforces). If the spec is inconsistent, Gate 4 catches it.

### Guard Discipline Pattern

At system boundaries (HTTP handlers, CLI, etc.):
1. Parse raw input (strings, floats)
2. Immediately construct guard types (`NewAmount(raw)`)
3. Constructor validates → error if invariant violated
4. Internal code only accepts guard types, never raw primitives
5. To get raw values back: `.Val()` on wrappers, exported fields on composites

### Relationship to Existing Repo

Current structure:
```
sb/commands/       ← SKM bundle (loop.md, init.md, setup.md)
demo/payment/      ← Payment processor demo (existing)
demo/email_crud/   ← Email CRUD demo (existing)
```

The shengen files at repo root need to be integrated:
- `shengen.go` → lives as `cmd/shengen/main.go` within each demo project, OR as a shared tool
- `guards_gen.go` / `guards_gen_test.go` → belong in `demo/email_crud/internal/shenguard/`
- `payment_guards_gen.go` / `payment_guards_gen_test.go` → belong in `demo/payment/` (or `demo/payment/internal/payment/`)

The sb commands (`/sb:setup`, `/sb:init`, `/sb:loop`) need updating to reference the four-gate architecture with shengen as Gate 1.

### Files Described but Not Yet Present

The user's specification describes three additional files that don't exist yet:
- `AGENT_PROMPT.md` — reference manual for the inner LLM harness
- `sb/commands/scaffold.md` — the `/sb:scaffold` command (full setup in one invocation)
- `SKILL.md` — SKM skill wrapper

These would need to be created to complete the system.

## Code References

- `shengen.go:1-978` — Complete codegen tool
- `shengen.go:57-151` — Symbol table with two-pass name resolution
- `shengen.go:246-263` — Main accessor chain resolver dispatch
- `shengen.go:460-518` — Structural match fallback for complex equality
- `shengen.go:816-845` — Go code generation orchestrator
- `guards_gen.go:168-176` — `NewCopyDelivery` with the key invariant check `profile.Demo == copy.Demo`
- `payment_guards_gen.go:62-70` — `NewBalanceChecked` with `bal >= tx.Amount.Val()`

## Open Questions

- Should shengen live as a shared tool at the repo root, or be copied into each demo's `cmd/shengen/`?
- Should the `*_gen*` reference files be committed or gitignored (since they're regenerated)?
- The sb commands need rewriting to include shengen as Gate 1 and teach the inner prompt about guard discipline — is this the next step?
- The user mentioned AGENT_PROMPT.md, sb_scaffold.md, and SKILL.md as additional files to create.
