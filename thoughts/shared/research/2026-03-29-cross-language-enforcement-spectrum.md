---
date: 2026-03-29T14:30:00-07:00
researcher: reuben
git_commit: 24ad8684ba5043753eb637a096e21be0102fb031
branch: main
repository: Shen-Backpressure
topic: "Cross-language enforcement spectrum: how spec-to-guards codegen works across compiled, interpreted, and mixed-paradigm languages"
tags: [research, codebase, cross-language, codegen, enforcement, compiler-checks, runtime-checks, shengen]
status: complete
last_updated: 2026-03-29
last_updated_by: reuben
---

# Research: Cross-Language Enforcement Spectrum

**Date**: 2026-03-29T14:30:00-07:00
**Researcher**: reuben
**Git Commit**: 24ad8684ba5043753eb637a096e21be0102fb031
**Branch**: main
**Repository**: Shen-Backpressure

## Research Question

How well does the Shen-Backpressure approach work for different kinds of languages? Compiled vs interpreted, statically typed vs dynamically typed. In some languages the guards might be compiler checks, in others runtime checks. The key idea is that a spec that compiles itself can be turned into code checks in the implementation language — but that looks quite different per target. How does this influence what has been built and designed here?

## Summary

The Shen-Backpressure approach decomposes into three independent enforcement layers, and languages differ in which layers they can support:

1. **Shen layer** (universal) — the spec itself type-checks via `shen tc+`. This is language-independent and always provides deductive verification across all cases.
2. **Compiler/type-checker layer** (language-dependent) — opaque types with private fields make bypass a compile-time error. Available in Go, Rust, TypeScript, Java, Kotlin, Swift, C#. Completely absent in Python, Ruby, Lua, plain JavaScript.
3. **Runtime validation layer** (universal) — constructor functions that check `verified` premises before returning the guard type. This runs in every language.

The key insight: **every target language gets layers 1 and 3. Layer 2 is a bonus.** The project is already designed around this — the create-shengen command (`sb/commands/create-shengen.md`) specifies enforcement mechanisms for eight languages, and the AGENT_PROMPT carries tri-language examples (Go, TypeScript, Rust) throughout. What varies is the *strength* of the compile-time guarantee and the *idiom* for expressing it.

## Detailed Findings

### 1. The Three-Layer Model

The codebase implements enforcement as a stack:

```
Layer 3: Shen tc+           — deductive proof over ALL inputs (universal)
Layer 2: Compiler/type-check — structural enforcement via opacity (language-dependent)
Layer 1: Runtime validation  — constructor checks at execution time (universal)
```

Every language gets layers 1 and 3 because:
- Layer 3 runs Shen as a separate subprocess regardless of target language
- Layer 1 is just `if !(condition) { return error }` in the constructor

Layer 2 is where languages diverge. The existing codebase maps this out explicitly.

### 2. Language Spectrum: Compile-Time Enforcement Strength

From the enforcement mechanism table in `sb/commands/create-shengen.md:22-31` and the analysis in `thoughts/shared/research/2026-03-29-closures-opaque-types-shen-idea-space.md:60-69`:

#### Tier 1: Strong compile-time opacity (struct fields are invisible outside the module)

| Language | Mechanism | Bypass possible? | Notes |
|----------|-----------|-------------------|-------|
| **Go** | Unexported fields (`v float64`) — only the `shenguard` package can access | Only via `reflect`/`unsafe` | Currently implemented. `internal/shenguard/` adds a second barrier via Go's `internal` package rule. |
| **Rust** | `pub(crate)` fields or private fields in a module | Only via `unsafe` | Documented in create-shengen. Module-level privacy is more granular than Go's package-level. Worth revisiting for sub-module splitting (per `thoughts/shared/research/2026-03-28-shengen-improvement-research.md:89-91`). |
| **Java** | `final class`, private fields, public static factory | Only via reflection (which security managers can block) | Documented in create-shengen. |
| **Swift** | `private(set)` stored properties, `public init` that throws | Only via `@_silgen_name` or Objective-C runtime | Documented in create-shengen §1a. |
| **Kotlin** | `private constructor`, companion object factory | Only via reflection | Documented in create-shengen §1a. |
| **C#** | `internal`/`private` fields, static `Create` returning `Result<T>` | Only via reflection | Documented in create-shengen §1a. |

For these languages, the five-gate loop provides: **shengen regeneration → test → compile → shen tc+ → tcb audit**. Gate 3 (compile/build) catches structural bypass at compile time. An LLM writing `Amount{v: 50}` in Go or `Amount { v: 50.0 }` in Rust gets a compile error from Gate 3.

#### Tier 2: Compile-time types, but runtime bypass is trivial

| Language | Mechanism | Bypass possible? | Notes |
|----------|-----------|-------------------|-------|
| **TypeScript** | `private constructor` + `private readonly` fields + static factory | Yes — `private` is compile-time only. After transpilation to JS, all fields are public. `(obj as any)._v` bypasses at the TS level; plain field access bypasses at JS level. | Currently implemented (`examples/payment_ts/guards_gen.ts`). The enforcement is a tsc guarantee, not a runtime guarantee. |

For TypeScript, the five-gate loop works the same way — Gate 3 (`tsc`) catches bypass — **but only if all consuming code is TypeScript**. If anything touches the generated types from plain JavaScript (e.g., a JS test file, a JSON.parse cast), the structural guarantee vanishes. The runtime validation in constructors (the `throw new Error(...)`) survives transpilation and provides layer 1.

This is a meaningful distinction from Go/Rust: in Go, `Amount{v: 50}` is a compile error period. In TypeScript, `new Amount(50)` is a tsc error but valid JavaScript.

#### Tier 3: No compile-time opacity — runtime checks only

| Language | Best available mechanism | Bypass | Notes |
|----------|------------------------|--------|-------|
| **Python** | `__slots__` + `__post_init__` validation, or `@dataclass(frozen=True)` | Yes — `object.__dict__` hacking, `object.__new__()` bypasses `__init__` | Documented in create-shengen §1a. The closures research (`thoughts/shared/research/2026-03-29-closures-opaque-types-shen-idea-space.md:66`) notes closures would be stronger. |
| **Ruby** | Private attrs | Yes — `instance_variable_get` bypasses | Noted in closures research. |
| **Lua** | Metatables | Yes — raw table access bypasses | Closures are the standard idiom. |
| **Plain JavaScript** | WeakMap or `#` private fields (ES2022) | WeakMap is leakable; `#` fields are genuinely private but require class syntax | Noted in closures research. |
| **Clojure** | `deftype` with protocols | Closures are natural | Noted in closures research. |

For these languages, **Gate 3 changes meaning entirely**. There is no "compile" step that catches structural bypass. The gate becomes:
- Python: `pytest` or `mypy` (if using type hints) — tests catch violations empirically, mypy catches type mismatches statically (but mypy is optional and doesn't enforce field privacy)
- Ruby: `rspec` — tests only
- Lua: tests only
- Plain JS: tests only, unless ESLint rules enforce the pattern

The five-gate architecture still works — the gates just mean different things:

| Gate | Go/Rust (Tier 1) | TypeScript (Tier 2) | Python (Tier 3) |
|------|------------------|--------------------|--------------------|
| 1. shengen | Regenerate `.go` | Regenerate `.ts` | Regenerate `.py` |
| 2. test | `go test` | `jest`/`vitest` | `pytest` |
| 3. build/typecheck | `go build` — catches bypass | `tsc` — catches bypass (compile-time only) | `mypy` — catches type mismatches, NOT privacy bypass |
| 4. shen tc+ | Spec consistency | Spec consistency | Spec consistency |
| 5. tcb audit | Diff generated files | Diff generated files | Diff generated files |

### 3. How the Codebase Already Addresses This

The project has been designed with multi-language targeting as a first-class concern from the beginning:

**The create-shengen command** (`sb/commands/create-shengen.md`) is a ~875-line language-agnostic compiler specification. It explicitly parameterizes every code generation decision by target language:
- Construction exclusivity mechanism (§1a — 8 languages)
- Error handling idiom (§8d — 5 languages)
- Naming conventions (§8c — 5 languages)
- Value accessor syntax (§6c — 5 languages)
- Sum type implementation (§8b — 3 languages: Go interface, TS union, Rust enum)
- Set membership syntax (§7e — 4 languages)
- Fallback TODO markers (§7f — 4 languages)

**The AGENT_PROMPT** (`sb/AGENT_PROMPT.md`) carries tri-language examples (Go, TypeScript, Rust) for every pattern: wrapping at boundaries, trust internally, proof chains, accessor extraction, constructor bypass prevention, sum types.

**Two shengen implementations exist**: Go (`cmd/shengen/main.go`, ~1600 lines) and TypeScript examples (`examples/payment_ts/guards_gen.ts`, `examples/email_crud_ts/guards_gen.ts`). The Go shengen is a working CLI tool; the TypeScript examples demonstrate the output shape.

**The closures research** (`thoughts/shared/research/2026-03-29-closures-opaque-types-shen-idea-space.md`) explicitly maps out the closure-based alternative for Tier 3 languages, proposing that shengen could emit closure-based guards where struct opacity is weak.

### 4. The Key Insight: Spec Compilation as the Invariant

The user's question identifies the core idea precisely: **a spec that compiles itself can be turned into code checks in the implementation language**. This is the `create-shengen` contract (§1):

> 1a. **Construction exclusivity** — values of guard types can ONLY be created through generated constructor functions
> 1b. **Validation faithfulness** — every `verified` premise becomes a runtime check in the constructor
> 1c. **Type propagation** — composite types accept guard types, not raw primitives

These three properties are **language-independent**. The spec compiles to three things:

1. **Types** — opaque containers (struct, class, closure, enum variant)
2. **Constructors** — functions that validate and return the type (or error)
3. **Proof chains** — constructors that require other guard types as inputs

What differs per language is HOW each property is enforced:

| Property | Go | TypeScript | Rust | Python |
|----------|------|------|------|------|
| Construction exclusivity | Unexported fields (compile-time) | Private constructor (compile-time, not runtime) | Private fields in module (compile-time) | Closure captures / `__slots__` (convention, bypassable) |
| Validation faithfulness | `if !(cond) { return err }` | `if (!(cond)) throw` | `if !(cond) { return Err(...) }` | `if not cond: raise ValueError(...)` |
| Type propagation | Compiler rejects wrong types | tsc rejects wrong types | Compiler rejects wrong types | mypy rejects (optional), runtime doesn't |

### 5. The Spectrum of Gate 3

The most important language-dependent gate is Gate 3 (build/typecheck). Here's what it actually catches per language family:

**Compiled, statically typed (Go, Rust, Java, Swift, Kotlin, C#)**:
- Structural bypass: `Amount{v: 50}` → compile error
- Type mismatch: passing `float64` where `Amount` expected → compile error
- Missing proof chain step: passing `Transaction` where `BalanceChecked` expected → compile error
- Stale signatures after spec change: old code using removed field → compile error

**Transpiled, statically typed (TypeScript)**:
- Same as above, but ONLY when consumed from TypeScript
- JS consumers bypass all of the above
- The `throw` in constructors survives transpilation (layer 1 holds)

**Interpreted, optionally typed (Python with mypy)**:
- Type mismatch: `mypy` catches `float` where `Amount` expected
- Does NOT catch: structural bypass (`object.__new__(Amount)`, direct attribute access)
- Does NOT catch: missing proof chain step at runtime (only if mypy is configured strictly)

**Interpreted, dynamically typed (Python without mypy, Ruby, Lua, JS)**:
- Gate 3 is effectively Gate 2 (tests). No static analysis catches anything.
- All enforcement is in layer 1 (constructor validation) and layer 3 (Shen tc+)

### 6. Design Implications: What This Means for the Project

The codebase is already structured to handle this spectrum, but there are several implications:

**The five-gate architecture is language-parametric.** The gates themselves don't change — what changes is the tool that implements each gate. `go build` becomes `tsc` becomes `mypy` becomes `pytest`. The gate NAMES in the README table (`sb/AGENT_PROMPT.md:8-13`) are Go-specific, but the CONCEPT is universal.

**Gate 3 degrades gracefully.** In Tier 1 languages, Gate 3 is a hard barrier (compile error). In Tier 2, it's a soft barrier (tsc error, bypassable at runtime). In Tier 3, it may not exist at all. But layers 1 and 3 always hold. The system never has ZERO enforcement — it just has fewer layers.

**The TCB audit (Gate 5) becomes MORE important for weaker languages.** In Go, the compiler prevents bypass, so Gate 5 is defense-in-depth. In Python, the compiler doesn't prevent bypass, so Gate 5 (ensuring the generated file hasn't been hand-edited) is a primary enforcement mechanism. If someone edits the Python guard to skip validation, Gate 5 catches it.

**The create-shengen command is the actual multi-language strategy.** It's not a CLI tool — it's a 875-line prompt that teaches an LLM how to build shengen for any target language. This is a deliberate architectural choice: rather than building N shengen implementations, the project provides one spec that any LLM can follow to build shengen for language N. The spec itself is the compiler for compilers.

**Closure-based guards expand the Tier 3 story.** The closures research proposes that for Python/Ruby/Lua, shengen could emit closure-based guards instead of class-based ones. A closure capturing validated state is harder to bypass than a class with `__slots__` because there's no `__dict__` to hack — the variable is captured in the closure scope. This is documented but not implemented.

### 7. How "Spec Compiles Itself" Translates Per Language

The user's phrasing — "a spec that compiles itself" — maps to a concrete pipeline per language:

**Go (Tier 1, compiled)**:
```
specs/core.shen → shengen → guards_gen.go → go build (COMPILE-TIME enforcement)
                                           → go test (RUNTIME checks in tests)
                → shen tc+ (DEDUCTIVE verification of spec)
```
Three independent verification passes. Gate 3 is a hard wall.

**TypeScript (Tier 2, transpiled)**:
```
specs/core.shen → shengen-ts → guards_gen.ts → tsc (COMPILE-TIME enforcement within TS)
                                              → jest/vitest (RUNTIME checks)
                → shen tc+ (DEDUCTIVE verification)
```
Three passes, but Gate 3 is soft — holds within the TS compilation boundary only.

**Rust (Tier 1, compiled)**:
```
specs/core.shen → shengen-rs → guards_gen.rs → cargo build (COMPILE-TIME, stronger than Go)
                                              → cargo test (RUNTIME)
                → shen tc+ (DEDUCTIVE)
```
Three passes. Gate 3 is the hardest wall of any language — Rust's ownership system adds additional enforcement that Go doesn't have (e.g., you can't clone a guard type without the implementation deciding to allow it).

**Python (Tier 3, interpreted)**:
```
specs/core.shen → shengen-py → guards_gen.py → mypy (OPTIONAL static type check — catches type mismatches, not privacy)
                                              → pytest (RUNTIME — only checks test cases)
                → shen tc+ (DEDUCTIVE)
```
Two-and-a-half passes. Gate 3 (mypy) is optional and doesn't enforce privacy. Layer 1 (constructor validation) carries the primary enforcement burden. Layer 3 (Shen) provides the deductive guarantee that the spec itself is sound.

**C (no target yet, hypothetical)**:
```
specs/core.shen → shengen-c → guards_gen.h/c → gcc/clang (COMPILE-TIME via opaque struct + translation unit boundaries)
                                              → tests (RUNTIME)
                → shen tc+ (DEDUCTIVE)
```
C can achieve Tier 1 enforcement through opaque structs (forward-declared in .h, defined only in .c). The header exposes only the constructor and accessor function signatures. This is the classic C information-hiding pattern. It's messy (requires managing .h/.c pairs, void* casting if you want polymorphism), but the compile-time guarantee is as strong as Go's.

### 8. What the Project Has Built vs. What Each Language Needs

| Component | Status | Go | TS | Rust | Python | C |
|-----------|--------|------|------|------|--------|---|
| Shen spec language | Built | Same | Same | Same | Same | Same |
| shengen codegen tool | Built (Go) | `cmd/shengen/main.go` | Example output exists | Documented | Documented | Not addressed |
| Guard type patterns | Built | 6 patterns | 6 patterns (examples) | Documented | Documented | Not addressed |
| Constructor validation | Built | `if !(cond) { return err }` | `if (!(cond)) throw` | `if !(cond) { Err(...) }` | `if not cond: raise` | `if (!(cond)) return NULL` |
| Compile-time opacity | Built | Unexported fields | Private constructor | `pub(crate)` | N/A (closures proposed) | Opaque struct (.h/.c) |
| Sum types | Detected, not generated | Interface + marker | Union type | Enum | ABC / Union | Tagged union |
| Five-gate script | Built (Go) | `go test/build` | `tsc`/`jest` | `cargo test/build` | `mypy`/`pytest` | `gcc`/`make test` |
| TCB audit | Built | Diff guards_gen.go | Diff guards_gen.ts | Diff guards_gen.rs | Diff guards_gen.py | Diff guards_gen.c |
| create-shengen spec | Built | Full coverage | Full coverage | Full coverage | Partial | Not covered |

## Code References

- `sb/commands/create-shengen.md:22-31` — Per-language enforcement mechanism table (8 languages)
- `sb/commands/create-shengen.md:724-730` — Per-language error handling table
- `sb/commands/create-shengen.md:709-718` — Per-language naming conventions
- `sb/commands/create-shengen.md:443-450` — Per-language value accessor table
- `sb/commands/create-shengen.md:683-704` — Sum type templates (Go, TS, Rust)
- `sb/AGENT_PROMPT.md:32-99` — Tri-language guard discipline examples
- `sb/AGENT_PROMPT.md:160-168` — Why the compiler catches bypass (tri-language)
- `cmd/shengen/main.go:1614-1700` — Go code generation (unexported field emission)
- `examples/payment_ts/guards_gen.ts` — TypeScript output shape (private constructors)
- `examples/payment/guards_gen.go` — Go output shape (unexported fields)

## Architecture Documentation

The project's architecture is built on a clean separation: **the Shen spec is universal; the codegen bridge is parameterized by target language; the five-gate loop is language-parametric**. This separation means the core value proposition (deductive spec verification + codegen-enforced guards) holds across all target languages. What degrades across the language spectrum is the STRENGTH of Gate 3, from "compile-time hard wall" (Go, Rust) through "compile-time soft wall" (TypeScript) to "optional static check" (Python with mypy) to "no static enforcement" (dynamic languages without type checkers).

The `create-shengen` command is the central artifact for multi-language support — it's a spec-for-building-specs that any LLM can follow to produce shengen for a new target language.

## Historical Context (from thoughts/)

- `thoughts/shared/research/2026-03-29-closures-opaque-types-shen-idea-space.md` — Maps the closure-based alternative for Tier 3 languages (Python, Ruby, Lua, Clojure)
- `thoughts/shared/research/2026-03-28-shengen-improvement-research.md` — Notes Go's package-level privacy limits sub-package splitting; flags Rust as worth revisiting for its more granular module privacy
- `thoughts/shared/research/2026-03-28-blog-series-deterministic-backpressure.md` — Frames the multi-language story as a potential Post 6 on building shengen for new targets
- `thoughts/shared/research/2026-03-24-shengen-codegen-bridge.md` — Deep dive on the shengen pipeline in language-agnostic terms

## Related Research

- `thoughts/shared/research/2026-03-29-closures-opaque-types-shen-idea-space.md` — Directly adjacent; covers the alternative enforcement mechanism for weak-opacity languages
- `thoughts/shared/research/2026-03-28-shengen-improvement-research.md` — Covers sum types, TCB audit, and Go vs Rust privacy granularity

## Open Questions

1. **Python shengen priority**: Should the Python target use `@dataclass(frozen=True)` with `__post_init__` (pragmatic, familiar, bypassable) or closure-based guards (stronger, unfamiliar)? The closures research proposes closures but notes LLMs may handle struct patterns better.

2. **Gate 3 for Tier 3 languages**: Should the project define a standard "lint gate" for languages without compile-time opacity? E.g., a custom ESLint/pylint rule that detects direct construction of guard types without going through the factory?

3. **Rust's extra enforcement**: Rust's ownership system prevents cloning a guard type unless `Clone` is derived. Should shengen-rs deliberately NOT derive Clone on guarded types, making the proof non-copyable (linear typing)? This would be enforcement Go and TypeScript cannot express.

4. **C target**: The opaque struct pattern in C is well-understood but requires managing .h/.c pairs and function pointer tables for sum types. Is there demand for a C shengen?

5. **The LLM-targeting question**: The create-shengen command is designed to be read by an LLM, not executed by a compiler. Does this mean the spec format should be optimized for LLM comprehension rather than formal precision? The current balance (pseudocode algorithms + language tables) seems to work, but hasn't been tested beyond Go and TypeScript.

6. **Hybrid enforcement for TypeScript**: Should the TS shengen emit branded types (compile-time nominal) alongside runtime validation, to get closer to Tier 1 enforcement? The create-shengen spec mentions this as an option but the current TS output uses only classes.
