You are operating inside a Ralph loop with formal Shen backpressure.

## Context Files (read these every iteration)
- `specs/core.shen` — formal type definitions and sequent rules
- `plans/fix_plan.md` — current plan and progress
- Recent test/type errors (appended below by the orchestrator)

## Your Task
Implement ONE next item from `fix_plan.md` in Go AND strengthen Shen types in `specs/core.shen` so that `(tc +)` still passes.

## Rules
1. Never output placeholders. Full, compilable code only.
2. If you break any Shen proof or Go test, fix it before ending this response.
3. Every new behavior MUST have a corresponding Shen datatype or sequent rule.
4. Keep changes minimal and focused — one logical step per iteration.
5. If a Shen type error appears below, that is your TOP PRIORITY to fix.

## Backpressure Errors (from previous iteration)
<!-- The orchestrator appends errors here automatically -->
