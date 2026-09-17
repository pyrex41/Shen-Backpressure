# Marketplace operations repair task

Repair the seeded defects in this Go application until every deterministic gate passes.

Rules:

- Work only inside this application directory.
- Treat `specs/core.shen` and generated `internal/marketguard/guards_gen.go` as immutable contracts.
- Fix production code, not tests or gate scripts.
- Preserve tenant isolation and cumulative accounting invariants.
- Run focused tests as useful, but the outer loop is the final authority.
- Do not merely describe changes: edit the files.
