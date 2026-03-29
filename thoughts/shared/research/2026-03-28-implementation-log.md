---
date: 2026-03-28T14:00:00-07:00
researcher: reuben
git_commit: bb2ce6668e47cc46fa6335d9545e0480f82f2d4e
branch: claude/add-web-tools-integration-eu9L4
repository: Shen-Backpressure
topic: "Implementation log — Gate 5, sum types, element?, TenantDB, blog post revision"
tags: [implementation, shengen, gate-5, sum-types, tcb, blog]
status: complete
last_updated: 2026-03-28
last_updated_by: reuben
---

# Implementation Log: Shengen Improvements from Scud Review

**Date**: 2026-03-28
**Triggered by**: 16-agent scud heavy review of blog post "Making Cross-Tenant Access Impossible by Construction"

## What Happened

1. Drafted blog post 3 for the deterministic backpressure series
2. Ran `scud heavy --effort high --debate 1` with 6 reviewer perspectives
3. Review identified real limitations: overclaimed title, missing TCB documentation, no sum type support, TS shengen missing `element?`
4. Three parallel research agents investigated phantom types, TCB reduction, and ergonomics
5. Implemented all improvements; revised blog post

## What Was Implemented

### 1. Gate 5: TCB Audit Script (`bin/shenguard-audit.sh`)

**Why**: The scud review identified that the shenguard package is the forgery boundary — any hand-written code inside it can bypass guard types. Gate 5 catches this.

**What it does**:
- Re-runs shengen and diffs output against committed `guards_gen.go`
- Detects unexpected `.go` files in the shenguard package
- Exits 1 on any mismatch

**Wired into**: Ralph orchestrator (`cmd/ralph/main.go`) as gate 5, Makefile as `audit` target.

### 2. Sum Type Code Generation (`cmd/shengen/main.go`)

**Why**: The ergonomics research identified that background jobs/cron can't construct `TenantAccess` because they have no JWT → no `AuthenticatedUser`. Sum types allow alternative proof roots.

**What it does**:
- When multiple `(datatype ...)` blocks produce the same conclusion type, shengen generates a Go interface
- Each concrete variant gets a struct that implements the interface via a private marker method
- Downstream types that reference the sum type use the interface in their constructors

**Example**: `human-principal` and `service-principal` both produce `authenticated-principal`. Generated code:

```go
type AuthenticatedPrincipal interface { isAuthenticatedPrincipal() }
type HumanPrincipal struct { auth AuthenticatedUser }
func (t HumanPrincipal) isAuthenticatedPrincipal() {}
type ServicePrincipal struct { cred ServiceCredential }
func (t ServicePrincipal) isAuthenticatedPrincipal() {}
```

**Key fix**: Sum type variants with wrapped conclusions (e.g., `Auth : authenticated-principal`) were being classified as ALIAS. Fixed by checking `isSumVariant` and forcing COMPOSITE classification for sum type variants.

### 3. `element?` List Literal Support in TS Shengen (`cmd/shengen-ts/shengen.ts`)

**Why**: The Go shengen already supported `(element? X [a b c])` but the TS version fell through to TODO. Needed for role-based access checks.

**What it does**: Generates `new Set(["a", "b", "c"]).has(val)` in TypeScript.

### 4. TenantDB Scoped Query Builder (`cmd/shengen/main.go`, `--db-wrappers` flag)

**Why**: The scud review identified that SQL queries inside `CheckTenantAccess` are in the TCB — a wrong WHERE clause breaks the invariant while types still check. Scoped wrappers capture the verified ID at construction time.

**What it does**: `shengen --db-wrappers output.go` generates a second file with proof-carrying DB wrappers:

```go
type TenantAccessDB struct { DB *sql.DB; tenant string }
func NewTenantAccessDB(db *sql.DB, proof TenantAccess) TenantAccessDB { ... }
```

Developer adds query methods. All queries automatically use the captured, proven tenant ID.

### 5. Blog Post Revision

**Changes based on scud review**:
- Title: "Impossible by Construction" → "Impossible to Accidentally Bypass"
- Added upfront acknowledgment of prior art ("parse don't validate", "make illegal states unrepresentable")
- Added "What the Guard Types Don't Cover" section documenting TCB
- Scoped all claims to "code that uses guard types"
- Honest about AI loop scaffolding

### 6. Multi-Tenant Demo Updated

- Spec now includes `service-id`, `service-credential`, `human-principal`, `service-principal` sum types
- `TenantAccess` takes `AuthenticatedPrincipal` (interface) instead of `AuthenticatedUser` (struct)
- All handlers, middleware, and tests updated
- All 5 gates pass

## What Was NOT Implemented (and Why)

### Phantom Types
Research concluded: don't do it. Tenant IDs are runtime values — the type parameter collapses to `string` everywhere that matters. The current design already prevents cross-tenant confusion.

### Package Splitting
Go's package-level visibility means splitting types/constructors into sub-packages doesn't help (constructors can't set unexported fields from another package). Gate 5 achieves the same goal.

### Static Analysis Pass
Deferred. Gate 5 covers the most critical case (detecting manual edits). A `go/analysis` pass for middleware coverage is useful but higher effort.

## Files Modified

| File | Change |
|------|--------|
| `bin/shenguard-audit.sh` | NEW — Gate 5 script |
| `cmd/shengen/main.go` | Sum types, `--db-wrappers` flag, classification fix |
| `cmd/shengen-ts/shengen.ts` | `element?` support, sum type tracking |
| `demo/multi-tenant-api/specs/core.shen` | Service account sum types |
| `demo/multi-tenant-api/internal/shenguard/guards_gen.go` | Regenerated with interface |
| `demo/multi-tenant-api/internal/auth/middleware.go` | `PrincipalFromContext`, `HumanFromContext` |
| `demo/multi-tenant-api/internal/auth/tenant.go` | `CheckTenantAccess` takes `AuthenticatedPrincipal` |
| `demo/multi-tenant-api/internal/auth/middleware_test.go` | Updated for new API |
| `demo/multi-tenant-api/internal/auth/tenant_test.go` | Updated for new API |
| `demo/multi-tenant-api/internal/handlers/handlers.go` | Uses principal-based auth |
| `demo/multi-tenant-api/cmd/ralph/main.go` | Gate 5 added |
| `demo/multi-tenant-api/Makefile` | `audit` target added |
| `blog/post-3-impossible-by-construction/post.md` | Revised per scud review |

## Verification

All 5 gates pass:
```
Gate 1: shengen      — PASS (regenerated with sum types)
Gate 2: go test      — PASS (all tests updated)
Gate 3: go build     — PASS (compiles with interface types)
Gate 4: shen tc+     — PASS (spec internally consistent)
Gate 5: tcb-audit    — PASS (no unexpected files, output matches)
```
