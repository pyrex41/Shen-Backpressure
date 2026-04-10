# Heavy Review of PR #10 (shen-derive Implementation)

**Captain Synthesis (from Benjamin + Lovelace + codebase inspection as of 2026-04-09)**

## Summary of Changes vs main
- **Massive new feature**: ~8k LOC across 26 new files under `shen-derive/`.
  - Core: custom functional language with AST (`core/ast.go`), parser (`parse.go`), evaluator (`eval.go`), typechecker with unification (`typecheck.go`), printer.
  - Rewrite engine: catalog of algebraic laws (`laws/catalog.go`, `rule.go`), side-condition discharge via Shen bridge (`shen/bridge.go`).
  - Codegen: sophisticated lowering to idiomatic Go loops (`codegen/lower.go` — foldr/foldl/map/filter/scanl become efficient for-loops with lambda inlining).
  - CLI/REPL, payment demo with derivation transcript, expanded test suite (59→104 tests), hardening for type/thread safety.
- Updated `README.md` to document `shen-derive` vs existing `shen-guard` (for fold-shaped pure computations using Bird-Meertens-style rewrites + Shen validation).
- Commits show iterative, review-responsive development ("Fix review issues...", "Harden shen-derive rewrite safety...", "Expand test suite").

**Git state confirmation** (via diagnostics):
- Branch: `claude/build-shen-derive-dh1Ti` (PR #10, base `main`).
- All tests pass (`go test ./...`).
- No obvious runtime panics in normal paths; strong coverage of parser, typechecker, laws, lowering, Shen interop.

## Strengths (High Confidence)
- **Logical soundness** (Benjamin): Type system, unification, evaluator, and rewrite matching are carefully implemented with good test coverage. The payment demo validates end-to-end derivation → lowering → Go code. Shen bridge cleverly emits obligations for `tc+` validation.
- **Systems quality** (Lovelace): Excellent codegen produces clean, efficient Go (no unnecessary allocations where possible). Empirical discharge + Shen fallback is pragmatic. CLI/REPL improves UX. Fits perfectly into the 5-gate backpressure philosophy.
- **Maintainability**: Step-by-step commits, comprehensive tests, clear separation (core vs laws vs codegen vs shen). README integration is professional.

## Suggested Improvements (Heavy Review Findings)
There are **substantive but contained** opportunities to harden for production use. These are not blockers but align with the project's high-assurance goals. Prioritize:

1. **Eliminate TODOs and Panics (Priority High)**
   - `codegen/lower.go:544`: Default case `/* TODO: %T */` in `exprSubst`. Make switch exhaustive or return `error`. Several term kinds (Let in some contexts, more complex Apps) could fall through.
   - `core/types.go:171`: `MkTFun` panics on bad input. Convert to error.
   - `shen/bridge.go`: `recover()` in `testEquality` for sampling is pragmatic but brittle (type assertion panics on mismatched samples). Document limitations clearly; consider better sampling strategy or static analysis for common cases. The comments already acknowledge that quantified side-conditions are "validation-only" — this is acceptable for v0 but should be called out in README as future work (sound prover).

2. **Lowering Robustness & Type Safety**
   - Monomorphization check in `LowerToGo` is good, but `guessExprType` and closure generation have heuristics that could be tightened using the inferred type map more aggressively.
   - Add more pattern matching for common combinators in `body()` to reduce closure overhead.
   - Ensure all generated code passes `go fmt`, `go vet`, and ideally has comments explaining the derivation.

3. **Error Handling, Observability, Extensibility**
   - Improve error messages with positions from parser (many already do, but consistency helps LLM backpressure).
   - Add benchmarks (`laws/laws_test.go`, codegen perf on larger terms).
   - Modularize: `parse.go` (951 LOC), `bridge.go` (780 LOC) are large — consider splitting complex helpers.
   - Expand laws catalog documentation with proofs/references to Bird-Meertens where possible.

4. **Integration & Documentation**
   - Ensure `shen-derive` artifacts can plug into the existing 5-gate pipeline (shengen regen, shen tc+, TCB audit). The README comparison is excellent — expand with concrete before/after for the payment demo.
   - Add example of using derived function in a larger Go project with guard types.
   - `heavy_analysis.md` and prior reviews should be referenced or folded in.

5. **Testing & Edge Cases (Benjamin Stress Test)**
   - Test rewrite confluence/termination on more complex terms.
   - Race detector (`go test -race`).
   - Fuzz parser/typechecker if not already.
   - Edge cases: deeply nested lets, higher-order functions in side conditions, empty lists in folds, type variable leakage.

## Prompt for Implementation Agent
**Task**: Address the findings above. Create `plans/fix_plan.md` (or update existing) with concrete `- [ ]` items in priority order. Implement **one item per iteration**, checking off as you go. Always fix any new backpressure errors (compile/test/Shen failures) **first**.

Focus first on robustness (TODOs, panics, lowering exhaustiveness, error handling). Then polish docs/integration. Keep changes minimal and targeted — this is already a strong PR.

After fixes:
- Re-run full test suite + `go test -race ./...`
- Update README if new limitations/docs added.
- Verify payment demo still works end-to-end.

Use the existing agent workflow (`/plan`, `/implement`, `/validate`). Reference this file for context.

**Confidence**: 80/100 — Code is already high-quality; suggestions are evolutionary. The derivation approach is a genuine advance for the Shen-Backpressure vision.

---
**Sources**: Direct git diagnostics, `git diff origin/main...HEAD --stat`, file reads on `lower.go`, `bridge.go`, `types.go`, `README.md`, test runs, commit history, existing `heavy_analysis*.md`.
