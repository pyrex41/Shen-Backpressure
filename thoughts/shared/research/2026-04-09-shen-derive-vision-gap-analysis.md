---
date: 2026-04-09T12:00:00-05:00
researcher: reuben
git_commit: ac99c3b4c18e01441604e35ee0240258e3bdadb7
branch: claude/build-shen-derive-dh1Ti
repository: pyrex41/Shen-Backpressure
topic: "shen-derive gap analysis: v1 corpus work vs. broader project vision"
tags: [research, codebase, shen-derive, gap-analysis, vision, backpressure]
status: complete
last_updated: 2026-04-09
last_updated_by: reuben
---

# Research: shen-derive Gap Analysis — V1 Corpus Work vs. Broader Project Vision

**Date**: 2026-04-09T12:00:00-05:00
**Researcher**: reuben
**Git Commit**: ac99c3b4c18e01441604e35ee0240258e3bdadb7
**Branch**: claude/build-shen-derive-dh1Ti
**Repository**: pyrex41/Shen-Backpressure

## Research Question

Codex has been doing good work on shen-derive but keeps narrowing scope / getting tunnel vision. Look at what we have and do a gap analysis against the vision.

## Summary

The v1 corpus work is **internally complete and self-referential** — 20/20 targets green, all engineering-done, honest proof boundaries documented. This is real, solid work. But it has become an isolated island: the six V1_* documents form a closed planning loop that references only itself and the 20 corpus targets. Meanwhile, the broader Shen-Backpressure project has a five-gate pipeline, four language targets for shengen, a working CLI orchestrator (`sb`), seven published blog posts, and a distributable skill bundle — none of which shen-derive currently touches.

The tunnel vision manifests in seven specific gaps between what shen-derive builds and what the project needs it to be.

## The Broader Vision (What shen-derive Should Serve)

The Shen-Backpressure project is a framework for making AI coding loops formally correct. It has three sub-systems:

1. **shen-guard** (shengen codegen) — generates opaque guard types from Shen specs; the compiler enforces invariants structurally
2. **shen-derive** (derivation engine) — generates efficient loops from naive functional specs via named algebraic rewrites; produces an auditable derivation transcript
3. **The Ralph loop / sb CLI** — closed-feedback harness tying formal verification gates to an LLM coding agent

The five-gate pipeline that every iteration must pass:

| Gate | Purpose |
|------|---------|
| 1. shengen | Regenerate guard types from specs. Catches spec drift. |
| 2. test | Runtime tests against regenerated types. |
| 3. build | Compiles against regenerated types. Catches structural bypass. |
| 4. shen tc+ | Verifies spec internal consistency. |
| 5. tcb audit | Diffs generated code against committed. Catches tampering. |

The deductive backpressure hierarchy:

```
Level 0: Syntax        — does it parse?
Level 1: Types         — does it compile?
Level 2: Tests         — do specific cases pass?
Level 3: Proof chain   — are invariants enforced for ALL inputs?
Level 4: Deductive     — is the spec itself internally consistent?
```

shen-derive provides a new mechanism at Level 3: the derivation transcript as an auditable proof artifact. You cannot silently skip the naive spec, the named rewrite, the side conditions, or the final loop.

## What shen-derive v1 Actually Built (Detailed)

### Implementation inventory

| Component | LOC (approx) | What it does |
|-----------|-------------|--------------|
| `core/ast.go` | 250 | 9 AST node types, 30 primitive operations |
| `core/types.go` | 170 | 7 concrete types (Int, Bool, String, Fun, List, Tuple, TVar) |
| `core/parse.go` | 900 | Hand-written lexer + recursive-descent parser |
| `core/eval.go` | 570 | Big-step call-by-value evaluator |
| `core/typecheck.go` | 390 | Hindley-Milner inference with unification |
| `core/print.go` | 170 | Precedence-aware pretty printer |
| `laws/catalog.go` | 270 | **4 laws**: map-fusion, map-foldr-fusion, foldr-fusion, all-scanl-fusion |
| `laws/rule.go` | 630 | First-order pattern matching, substitution, path navigation |
| `shen/bridge.go` | 800 | Side-condition → Shen s-expression translation, 4 discharge strategies |
| `shen/prove.go` | 470 | Symbolic polynomial prover over `big.Int` coefficients |
| `codegen/lower.go` | 720 | Go code generator: 7 body patterns (foldl, foldr, map, filter, scanl, if, let) + projected-foldl |
| `runtime/runtime.go` | 10 | Generic `Pair[A,B]` (unused by generated code) |
| `main.go` | 340 | CLI: repl, eval, parse, check, rewrite, lower, laws |

**Total core**: ~5,700 lines of Go.
**Corpus**: 20 green targets with checked-in golden artifacts and drift detection.
**Proved fragment**: Symbolic polynomial arithmetic for foldr-fusion witnesses (negate-sum, double-sum).

### What the law catalog contains

1. **map-fusion**: `map f . map g = map (f . g)` — no side conditions
2. **map-foldr-fusion**: `map f . foldr cons nil = foldr (\x xs -> cons (f x) xs) nil` — no side conditions
3. **foldr-fusion**: `f . foldr g e = foldr h (f e)` — 1 side condition: `f (g x y) = h x (f y) for all x, y`
4. **all-scanl-fusion**: fuses `foldr (\x acc -> p x && acc) True (scanl f e xs)` into a single `foldl` pass — no side conditions

### What the code generator handles

7 body patterns in priority order:
1. projected-foldl: `fst/snd (foldl ...)`
2. foldr: reverse-indexed for-loop
3. foldl: forward range loop
4. map: pre-allocated slice + indexed loop
5. filter: nil slice + conditional append
6. scanl: slice with initial element + accumulation
7. if/let: statement-level emission

Generated code is correct, compiles without imports, uses `// Code generated by shen-derive. DO NOT EDIT.` headers. Not idiomatic (anonymous structs, IIFEs for step closures, `_arg1` naming).

## The Seven Gaps

### Gap 1: No integration with the five-gate pipeline

**What exists**: shen-derive is a standalone Go module with its own CLI. The `sb` CLI (`sb init`, `sb gen`, `sb gates`, `sb loop`) operates entirely on the shen-guard world. There is no `sb derive` subcommand. shen-derive is not a gate.

**What the vision needs**: shen-derive output should participate in the pipeline. A derived function like `Processable` should be regenerable via a gate (like Gate 1 regenerates guard types), with drift detection (like Gate 5 audits generated code). Currently, the payment demo re-derives in a test, but this is test-side validation, not pipeline-side enforcement.

**Impact**: An LLM in a Ralph loop cannot currently benefit from shen-derive backpressure unless someone manually wires it in. The "derivation as audit artifact" story from the blog post is aspirational — the tool produces the artifact, but no gate enforces it.

### Gap 2: No connection between shen-derive output and shen-guard types

**What exists**: shen-derive generates standalone Go functions. shengen generates opaque guard types. They share no imports, no cross-references, no module dependencies. The `payment-processable` corpus target takes `int` and `[]int`, not guard types.

**What the vision needs**: Derived functions that consume and produce guard types. A real `Processable` function should take `Amount` and `[]Transaction` (shengen types), not raw ints. This is the "per-function choice" framing from the README — shen-guard for I/O boundaries, shen-derive for pure computation — but the two tools must interoperate at the type level.

**Impact**: Without this, shen-derive-generated code lives in a separate world from the guard-type-enforced code. Users must manually wrap/unwrap. The compile-time enforcement chain has a gap.

### Gap 3: Go-only code generation

**What exists**: One codegen backend (`codegen/lower.go`) targeting Go only. The architecture cleanly separates the derivation engine (core/, laws/, shen/) from the code generator (codegen/), and the DESIGN.md explicitly describes this separation as enabling multi-language backends.

**What the vision needs**: At minimum, the same languages shengen already targets: Go, TypeScript, Rust, Python. The blog series describes how each language's idioms differ for fold-shaped code:
- Rust: `iter().fold()`
- Python: comprehensions or `functools.reduce`
- TypeScript: `.reduce()`

**Impact**: shengen already has four language targets. shen-derive has one. For the "one spec, every language" story to extend to derived computation (not just guard types), additional codegen backends are needed. The DESIGN.md correctly defers this but the gap is growing as shengen expands.

### Gap 4: CLI is incomplete for integration use

**What exists**: `main.go` exposes `repl`, `eval`, `parse`, `check`, `rewrite`, `lower`, `laws`. The `rewrite` command always operates at root path. The `lower` command hard-codes `funcName="Derived"` and `pkg="derived"`.

**What the vision needs**: 
- `lower` needs configurable function name and package
- `rewrite` needs path specification for non-root rewrites (the `payment-processable` rewrite uses path `{0,0}`)
- A `derive` command that chains rewrite → lower → write-to-file in one invocation
- Spec input from files (not just command-line strings) for non-trivial terms
- Integration points for `sb` to call

**Impact**: The CLI exists but is not production-usable. Everything meaningful happens through Go test code. An LLM in a Ralph loop cannot invoke shen-derive from the command line to perform a derivation — it must write Go test code.

### Gap 5: The derivation transcript is not a first-class artifact

**What exists**: The payment demo generates a `derivation.txt` that records the steps:
- Original spec (pretty-printed)
- Rule applied, at what path
- Obligations (none in this case)
- Resulting term
- Generated Go

This is produced by `renderTranscript` in `derive_test.go` — a test helper, not part of the shen-derive library or CLI.

**What the vision needs**: The blog post frames the derivation transcript as the core value proposition of shen-derive for backpressure. It should be:
- A first-class output of the CLI (`shen-derive derive --transcript out.txt`)
- A checked-in artifact with drift detection (like the generated `.go` files)
- Machine-readable (structured format, not just pretty-printed text)
- Part of the gate pipeline (Gate 5 could audit derivation transcripts alongside generated code)

**Impact**: The audit trail story is the thing that differentiates shen-derive from "just an optimizer." Without first-class transcript support, users get the optimized code but not the justification chain.

### Gap 6: The law catalog is minimal and static

**What exists**: 4 laws, all hard-coded in `catalog.go`. Adding a new law requires writing Go code (a `Rule` struct with `Term` AST nodes for LHS/RHS/conditions). There is no file-based law format, no law discovery, no user-extensible catalog.

**What the vision needs**: The Bird-Meertens catalog has dozens of laws. The DESIGN.md says the catalog is "intentionally small" for v1, but the v1 planning docs have created a self-reinforcing loop: the corpus is limited to what the catalog supports, and the catalog is limited to what the corpus needs. Common laws not present:
- `foldl-fusion`: `f . foldl g e = foldl h (f e)` (the dual of foldr-fusion)
- `foldr-map`: `foldr g e . map f = foldr (g . f) e` (fold after map)
- `scan-fusion`: various scan optimization laws
- `filter-fusion`: `filter p . filter q = filter (\x -> p x && q x)`
- `unfold-fold`: connections between unfoldr and foldr (the "banana-split" theorem)

**Impact**: Without catalog growth, shen-derive cannot handle the broader class of sequence transformations that real applications need. The current catalog handles map-map fusion, map-foldr fusion, foldr fusion, and one specific all-scanl pattern. Real codebases have filter-after-map, nested folds, accumulator transformations, and more.

### Gap 7: No spec file format / declarative derivation definitions

**What exists**: Every derivation target is defined as Go code constructing AST terms (e.g., `core.MkLam("xs", core.MkTList(core.TInt{}), core.MkApps(core.MkPrim(core.PrimFoldl), ...))`). The specs live inside test files.

**What the vision needs**: A declarative format for defining derivation targets. Something like:
```
-- sum.spec
sum : [Int] -> Int
sum = foldl (+) 0
```
Or even integration with `.shen` files where the Shen spec defines both the guard type and the derivation target. The parser already handles this surface syntax — the gap is in the pipeline, not the parser.

**Impact**: Without a file-based spec format, shen-derive is only usable by people who write Go code. An LLM in a Ralph loop would need to construct Go test files to use shen-derive, rather than writing spec files that the tool consumes. This is the biggest barrier to integration with the `sb` pipeline.

## What the V1 Documents Don't See

The V1_* documents (CORPUS_STATUS, GAP_ANALYSIS, EXECUTION_PLAN, DONE_CHECKLIST, PLANNING_HANDOFF, EXECUTION_AGENT_HANDOFF) form a closed system. They reference:
- Each other
- The 20 corpus targets
- The 4 laws
- The codegen patterns

They do **not** reference:
- The `sb` CLI or the five-gate pipeline
- shengen or guard types
- The blog series or the discourse positioning
- The multi-language direction beyond a DESIGN.md mention
- The Ralph loop integration
- The skill bundle or distribution story
- Any demo beyond `payment-processable`

This is the tunnel vision. The documents are well-crafted for their internal scope, but they define "done" as "the corpus is green" rather than "shen-derive serves its role in the broader system."

## Architecture Documentation

### Current package dependencies (shen-derive)

```
main.go → core/, laws/, shen/, codegen/
codegen/ → core/
shen/ → core/, laws/
laws/ → core/
core/ → (stdlib only)
runtime/ → (stdlib only, no dependents)
```

### Current package dependencies (broader project)

```
cmd/sb/ → (standalone binary, calls shengen via shell)
cmd/shengen/ → (standalone binary, no Go library dependencies)
shen-derive/ → (standalone module, no dependency on cmd/*)
sb/ → (Markdown skill bundle, no Go code)
```

The three sub-systems (shen-guard, shen-derive, sb CLI) are fully decoupled at the module level. Integration happens through shell scripts and convention, not Go imports.

## Historical Context (from thoughts/)

- `thoughts/shared/research/2026-03-23-codebase-overview.md` — earliest snapshot; shen-derive was already described as "complementary to shengen"
- `thoughts/shared/research/2026-03-24-shengen-codegen-bridge.md` — detailed analysis of how shengen's codegen bridge works
- `thoughts/shared/research/2026-03-28-blog-series-deterministic-backpressure.md` — the five-level hierarchy that positions shen-derive at Level 3
- `thoughts/shared/research/2026-03-28-shengen-improvement-research.md` — shengen feature development (sum types, db-wrappers, hardened mode)
- `thoughts/shared/research/2026-03-29-cross-language-enforcement-spectrum.md` — the three-layer, three-tier framework for multi-language enforcement
- `thoughts/shared/research/2026-03-31-full-codebase-exploration.md` — comprehensive snapshot showing four shengen implementations

## Code References

- `shen-derive/DESIGN.md:4-12` — Core vision statement
- `shen-derive/DESIGN.md:45-62` — Multi-language future direction
- `shen-derive/V1_CORPUS_STATUS.md:19-49` — All 20 targets, all green
- `shen-derive/V1_GAP_ANALYSIS.md:44-53` — Gap 5 (docs) mentions broader vision link as a TODO
- `shen-derive/laws/catalog.go:10` — `Catalog()` returns exactly 4 laws
- `shen-derive/main.go:211` — `lower` hard-codes funcName="Derived", pkg="derived"
- `shen-derive/main.go:192` — `rewrite` always uses `laws.RootPath`
- `shen-derive/codegen/lower.go:189-230` — The 7 body patterns in priority order
- `shen-derive/shen/bridge.go:174-178` — Hard-coded all-number type annotations for Shen
- `shen-derive/demo/payment-derived/derive_test.go:176-231` — The only end-to-end demo
- `cmd/sb/main.go` — sb CLI with no shen-derive integration
- `cmd/shengen/main.go` — shengen standalone, no shen-derive dependency

## Related Research

- `thoughts/shared/research/2026-03-28-blog-series-deterministic-backpressure.md` — Positions shen-derive in the backpressure hierarchy
- `thoughts/shared/research/2026-03-29-cross-language-enforcement-spectrum.md` — Multi-language enforcement that shen-derive should eventually participate in
- `thoughts/shared/research/2026-03-31-full-codebase-exploration.md` — Comprehensive codebase snapshot (pre-v1 corpus completion)

## Open Questions

1. **Should shen-derive become a gate in the five-gate pipeline?** If so, what triggers it? Which functions get derived vs. hand-written?
2. **How should derived functions consume guard types?** Does the codegen need to import shenguard packages? Or does the user wrap/unwrap at the boundary?
3. **What's the priority order for the seven gaps?** Gap 4 (CLI) and Gap 7 (spec files) are prerequisites for Gap 1 (pipeline integration). Gap 6 (law catalog) is a prerequisite for broader applicability.
4. **Is the Rust port still the right call for the derivation engine core?** The Go implementation is clean and the architecture already separates concerns. Is multi-language codegen (adding TypeScript/Rust/Python backends to the existing Go engine) a better near-term investment?
5. **Should the derivation transcript format be standardized?** If it becomes an audit artifact, it needs a stable schema — not just pretty-printed text.
