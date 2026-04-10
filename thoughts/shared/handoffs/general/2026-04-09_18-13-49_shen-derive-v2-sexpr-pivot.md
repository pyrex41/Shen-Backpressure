---
date: 2026-04-09T18:13:49-0500
researcher: reuben
git_commit: a357f531fd35ef8e5a5cdb14d801cd12fd85344f
branch: claude/build-shen-derive-dh1Ti
repository: pyrex41/Shen-Backpressure
topic: "shen-derive v2: S-expression representation pivot"
tags: [implementation, shen-derive, s-expressions, architecture-pivot]
status: in_progress
last_updated: 2026-04-09
last_updated_by: reuben
type: implementation_strategy
---

# Handoff: shen-derive v2 — Replace custom AST with Shen s-expressions

## Task(s)

**Architectural pivot of shen-derive from a custom lambda calculus to Shen s-expressions as the core representation.** This is a multi-phase rewrite. The motivation: shen-derive v1 had a toy type system (7 types: Int/Bool/String/List/Tuple/Fun/TVar) that made it impossible to derive over shengen guard types or anything Shen's sequent calculus can express. The fix: use Shen's own s-expression representation, delegate type checking to Shen `tc+`, and share `.shen` spec files with shengen.

### Status by phase:

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1a: S-expr types | **Completed** | `core/sexpr.go` — `Atom` + `List`, replaces 16 old types |
| Phase 1b: S-expr parser | **Completed** | `core/sexpr_parse.go` — ~200 lines, replaces ~950 |
| Phase 1c: S-expr printer | **Completed** | `core/sexpr_print.go` — with cons-list sugar |
| Phase 1d: S-expr evaluator | **Not started** | Minimal evaluator for fold/map/filter/arithmetic fragment |
| Phase 1e: Delete typecheck | **Completed** | Type checking delegates to Shen `tc+` |
| Phase 1f: Rewrite DESIGN.md | **Not started** | Needs full rewrite for v2 architecture |
| Phase 2: Rewrite engine | **Completed** | `laws/rule.go` + `laws/catalog.go` — 7 laws, all tests pass |
| Phase 3: Codegen | **Not started** | Rewrite `codegen/lower.go` for s-expressions |
| Phase 4: CLI + spec format | **Not started** | `.shen` files as input, `derive` command |
| Phase 5: Gate + demo | **Not started** | Payment demo migration, `sb` gate integration |
| Phase 6: Bridge cleanup | **Not started** | Simplify `shen/bridge.go` for native s-expressions |

## Critical References

1. **Implementation plan**: `/Users/reuben/.claude/plans/glowing-jumping-journal.md` — the full 6-phase plan approved by the user
2. **Vision gap analysis**: `thoughts/shared/research/2026-04-09-shen-derive-vision-gap-analysis.md` — documents the 7 gaps between v1 corpus work and the broader project vision
3. **Updated DESIGN.md**: `shen-derive/DESIGN.md` — already partially updated (Rust port rejected, near-term direction added), needs full rewrite for v2

## Recent changes

All changes are in commit `a357f53`:

- `shen-derive/core/sexpr.go` — New `Sexpr` interface with `Atom` and `List` types, helper constructors (`Sym`, `Num`, `Str`, `Bool`, `SList`, `Lambda`, `SApply`), inspection helpers (`IsSym`, `IsMetaVar`, `HeadSym`, `ListElems`, `SexprIntVal`, etc.), `DeepCopy`
- `shen-derive/core/sexpr_parse.go` — S-expression parser: atoms, `(lists)`, `[cons|sugar]`, `"strings"`, `\\` and `--` comments
- `shen-derive/core/sexpr_print.go` — Pretty printer with automatic cons-list to `[a b c]` sugar
- `shen-derive/core/sexpr_test.go` — 12 tests covering parse, print, round-trip, deep copy, metavar detection
- `shen-derive/laws/rule.go` — Rewrite engine on s-expressions: `Match`, `Substitute`, `AtPath`, `ReplacePath`, `Rewrite`, `RewriteWithSupplementalBindings`
- `shen-derive/laws/catalog.go` — 7 laws defined as parsed s-expression strings via `mustParse()`. New laws: `filter-fusion`, `foldr-map`, `foldl-fusion`
- `shen-derive/laws/laws_test.go` — 14 tests covering all 7 laws, path-based rewriting, supplemental bindings, consistency checking
- Deleted: 6 stale `V1_*` planning docs, all old `core/` files (ast, types, parse, print, eval, typecheck + tests)

## Learnings

1. **S-expression matching is dramatically simpler than typed AST matching.** The old `matchTerm` switched on 9 node types with nested type assertions. The new `matchSexpr` is ~20 lines — atoms match literally or bind as metavars, lists match element-by-element. Same for substitution, path navigation, etc.

2. **Law definitions become one-liners.** Old: 20 lines of nested `core.MkApps(core.MkPrim(...), ...)`. New: `mustParse("(compose (map ?f) (map ?g))")`. This makes law authoring trivial.

3. **Path semantics changed.** Old paths indexed into AST node children with semantic conventions (0=Func for App, 0=Body for Lam, etc.). New paths index into `List.Elems` — purely positional. Path `{1}` means "the second element of this list." This is simpler but means old path values from v1 corpus tests won't transfer directly. For example, `(foldl step init xs)` has `step` at index 1, `init` at index 2, `xs` at index 3.

4. **Name conflicts during transition.** The old and new code can't coexist in the same package because of name collisions (`Apply`, `IntVal`, `BoolVal`, `List`). We resolved this by moving old files to `v1_backup/` directories (untracked) and prefixing conflicting new names (`SApply`, `SexprIntVal`, `SexprBoolVal`). Once the port is complete, these can be renamed back to clean names since the old types will be gone.

5. **The `all-scanl-fusion` law is the most complex.** It involves `let`, `@p` (tuple), `fst`, `snd`, `and`. The s-expression representation handles this cleanly but the parsing needed an unwrap step (the outer parens from multiline formatting create a wrapper list). See `catalog.go:AllScanlFusion()`.

6. **User decisions captured:**
   - shen-guard and shen-derive are **parallel tools**, not layers in one pipeline. They can share specs but don't need tight coupling.
   - Rust FFI port **rejected** — wrong tradeoff (port 3600 lines to avoid duplicating 600 lines of codegen per language). Alternatives: AST-as-JSON or per-language reimplementation.
   - shen-derive should support guard types via a **type mapping file** (Shen type name → Go type + accessors), graduating to auto-extraction from `.shen` specs later.

## Artifacts

- `shen-derive/core/sexpr.go` — S-expression types and helpers
- `shen-derive/core/sexpr_parse.go` — Parser
- `shen-derive/core/sexpr_print.go` — Printer
- `shen-derive/core/sexpr_test.go` — Core tests (12 passing)
- `shen-derive/laws/rule.go` — Rewrite engine
- `shen-derive/laws/catalog.go` — 7-law catalog
- `shen-derive/laws/laws_test.go` — Law tests (14 passing)
- `shen-derive/DESIGN.md` — Partially updated (needs full v2 rewrite)
- `thoughts/shared/research/2026-04-09-shen-derive-vision-gap-analysis.md` — Gap analysis
- `/Users/reuben/.claude/plans/glowing-jumping-journal.md` — Full implementation plan
- `/Users/reuben/.claude/projects/-Users-reuben-projects-Shen-Backpressure/memory/project_rust_port_direction.md` — Updated memory: Rust port rejected

## Action Items & Next Steps

### Immediate: Get the build green (Phases 1d, 3, 6)

The build is currently broken because `codegen/`, `shen/`, and `main.go` still reference `core.Term` and `core.Type` which no longer exist. Priority order:

1. **Write `core/eval.go` — minimal s-expression evaluator** (Phase 1d)
   - Needs to evaluate: arithmetic (`+`, `-`, `*`), comparison (`>=`, `<`, `==`), boolean (`and`, `or`, `not`), `lambda`, `let`, `if`, `foldl`, `foldr`, `map`, `filter`, `scanl`, `cons`, `nil`, `fst`, `snd`, `@p`
   - Used for spec-vs-derived equivalence testing (not full Shen evaluation)
   - Reference: old evaluator at `shen-derive/core/v1_backup/eval.go` (~570 lines) — same logic, different input types
   - Target: ~200-300 lines

2. **Rewrite `codegen/lower.go`** (Phase 3a)
   - Pattern-match on s-expression structure instead of AST node types
   - `isFoldl(s) → (step, init, list, ok)` checks `HeadSym(s) == "foldl"` and extracts `ListElems(s)[1:]`
   - Same Go emission logic (for-loops, range, make+index)
   - The codegen needs type info for Go type strings — start with a simple type mapping approach (`codegen/typemap.go`)
   - Reference: old lower at `shen-derive/core/v1_backup/` — but also read `shen-derive/codegen/lower.go` directly since it hasn't been moved
   - The 20 corpus golden files in `codegen/testdata/corpus/` define the expected output

3. **Update `shen/bridge.go` and `shen/prove.go`** (Phase 6)
   - Change interfaces from `core.Term` to `core.Sexpr`
   - The bridge's `termToShenInner` becomes nearly trivial (s-expr → s-expr string is just `PrettyPrintSexpr`)
   - The prover's polynomial algebra is unchanged — only the interface to extract values from terms changes

4. **Rewrite `main.go`** — update commands to use new types

5. **Port the 20-target corpus** in `codegen/corpus_test.go` — re-express all targets as Shen s-expressions

### After build is green: CLI + integration (Phases 4, 5)

6. **Spec file format** — `.shen` files with `(derive ...)` blocks
7. **`derive` CLI command** — parse spec → rewrite → lower → write files + transcript
8. **Transcript library** — `transcript/` package with JSON + text rendering
9. **Gate integration** — `sb.toml` `[derive]` section, `bin/shen-derive-audit.sh`
10. **Payment demo migration** — single `.shen` spec for both shengen types and shen-derive

### Deferred

- Multi-language codegen backends
- Auto-extraction of type mappings from `.shen` specs (start with explicit mapping file)
- Full DESIGN.md rewrite (Phase 1f — do after architecture stabilizes)

## Other Notes

- **Old code is preserved** in `shen-derive/core/v1_backup/` and `shen-derive/laws/v1_backup/` (gitignored/untracked). These are the reference implementations for porting the evaluator, codegen, and bridge. Don't delete them until the port is complete.

- **`go test ./core/ ./laws/` passes.** `go test ./...` does not (codegen, shen, main are broken). This is expected.

- **The `all-scanl-fusion` law is the integration test.** It's the most complex law and the one used by the payment-processable demo. If it works end-to-end (match → rewrite → evaluate → lower → compile), everything else will work.

- **Naming convention for s-expression helpers**: Functions that conflicted with old names got `S` or `Sexpr` prefixes (`SApply`, `SexprIntVal`, `SexprBoolVal`). Once the old code is fully gone, consider renaming back to clean names.

- **The `@p` Shen tuple syntax** is handled by the parser as `(@p a b)` — a regular list with head symbol `@p`. The printer does NOT desugar this (it prints as-is). The codegen will need to recognize `@p` as tuple construction.

- **Shen uses `true`/`false` as atoms, not capitalized `True`/`False`.** The v1 code used `MkBool(true)` which produced `Lit{BoolVal: true}`. The new code uses `Bool(true)` which produces `Atom{Val: "true", Kind: AtomBool}`. The `all-scanl-fusion` law pattern uses lowercase `true`.
