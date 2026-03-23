You are operating inside a Ralph loop with Shen sequent-calculus backpressure.

## What you are building

An email campaign app with personalized CTA landing pages.

**Flow**: Admin creates campaigns with email copy variants keyed by (age_decade, state). Emails go out with a CTA link. When a user clicks the link:
- If their profile is **known** (has age_decade + state) → show tailored copy immediately
- If their profile is **unknown** → prompt them for age decade and US state first, then show tailored copy
- Users can always update their demographics

**Stack**: Go backend (stdlib net/http, SQLite via go-sqlite3), htmx + alpine.js + vanilla JS frontend. No frameworks.

## Context files — read these EVERY iteration

- `specs/core.shen` — Shen sequent-calculus type specifications. These are the formal invariants. Your Go code must enforce what these types prove.
- `plans/fix_plan.md` — The plan. Pick the FIRST unchecked `- [ ]` item and implement it. Check it off `- [x]` when done. Do NOT skip ahead.
- `internal/models/models.go` — Domain types
- `internal/db/` — Database layer
- `internal/handlers/handlers.go` — HTTP handlers
- `cmd/server/main.go` — HTTP server entry point
- `templates/` — HTML templates (htmx/alpine)

## Rules

1. **ONE task per iteration.** Pick the first unchecked item from fix_plan.md. Implement it fully. Check it off. Stop.
2. **No placeholders.** Every file you touch must be complete, compilable, runnable code.
3. **Before making changes, search the codebase.** Don't assume something isn't implemented — look first. Use subagents for search.
4. **Every behavior must have a corresponding Shen rule.** If you add a new domain concept, add a datatype to specs/core.shen.
5. **All three gates must pass:**
   - `go test ./...`
   - `go build ./cmd/server`
   - `./bin/shen-check.sh`
6. **If a gate fails, fix it before ending your response.** Run the gate yourself to confirm.
7. **Update fix_plan.md** — check off the completed item. If you discover new work, append it to the bottom.
8. **Keep templates simple.** htmx for server round-trips, alpine for small client-side state (like the demographics prompt form), vanilla JS only where needed.
9. **Shen invariant enforcement in Go:** The key rule is `copy-delivery` — tailored copy requires a known-profile whose demographics match the copy's target. The Go handler must enforce this: check `user.IsKnown()` before serving personalized content.

## Backpressure Errors (from previous iteration)
