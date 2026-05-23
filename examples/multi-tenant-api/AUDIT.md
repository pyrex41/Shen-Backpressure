# Multi-Tenant API — Auditor Workflow

> One-page reviewer workflow for the JWT/authorization proof chain
> demo. Read [`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md) first
> for the project-level trust model — and pay special attention to
> the "What's assumed (the TCB)" section, which calls out two specific
> gaps in this demo's spec.

## What this demo proves

A live Go HTTP service where every data access carries a compile-time
proof:

```
JwtToken → AuthenticatedUser → TenantAccess → ResourceAccess
```

A handler that demands `ResourceAccess` cannot be called without the
caller having walked the whole chain. Cross-tenant data access is
**impossible by construction** in the structural sense — there is
no Go expression that produces a `ResourceAccess` for the wrong
tenant.

Two integration tests, `TestCrossTenantAccessRejected` and
`TestCrossTenantResourceAccessRejected` in
`internal/handlers/handlers_test.go`, exercise the chain through the
live HTTP layer.

## Auditor steps

### 1. Verify the spec hash matches the committed file

```bash
cd examples/multi-tenant-api
sha256sum specs/core.shen
```

Compare the value to `spec.files[].sha256` in
`transcript/discharge_report.json` and `transcript/audit_report.md`.
If they disagree, the artifact is stale — re-run gates before
continuing.

### 2. Read the rendered audit report

Open `transcript/audit_report.md`. It contains:

- **Spec hash and git commit** the report was produced against
- **Per-rule discharge tables** classifying each premise as
  `static`, `runtime-sample`, or `unproven` with code references
  back into the impl
- **Counter-examples** for any violated premise (empty for the
  canonical pass case)
- **`discharged_since_commit`** per rule — how stable each invariant
  has been across the project's history
- The canonical discharge-category glossary in the appendix

### 3. Re-run the gates at the recorded commit

```bash
git checkout <git_commit-from-transcript/discharge_report.json>
cd examples/multi-tenant-api
../../bin/sb gates
```

Expected output:

```
PASS  shengen        12ms
PASS  test           136ms
PASS  build          512ms
PASS  shen-check     149ms      ← needs shen-sbcl; otherwise FAIL
PASS  tcb-audit      19ms

5/5 gates passed
```

`tcb-audit` (`bin/shenguard-audit.sh`) is the gate that catches a
specific attack: someone hand-editing `internal/shenguard/guards_gen.go`
to weaken a constructor. The script re-runs shengen on the spec
and byte-diffs against the committed generated file. Note: the
script today is Go-only; the same gate for the TypeScript demo
(shen-web-tools) is being generalized in Wave-2 of the
HN-follow-up plan.

### 4. Read the TCB for this demo

The structural chain rests on a small named TCB. **Read every line
of these files** — that is the audit.

| Piece | Where | What you're checking |
|---|---|---|
| **JWT parser** | `internal/auth/jwt.go:52-82` | HMAC-SHA256 with `crypto/hmac.Equal` (constant-time). Verify the order: signature → JSON-decode → expiry. Look for short-circuits that decode before verifying. The test `TestParseTamperedPayload` is the obvious threat. |
| **Middleware** | `internal/auth/middleware.go:35-51` | The convention-only binding `userID := shenguard.NewUserId(result.Claims.Sub)`. **This is not enforced by the type system.** The spec does not declare `(= User (sub Claims)) : verified`. See "Known gap" below. |
| **`CheckTenantAccess`** | `internal/auth/tenant.go:15-31` | The SQL query is `SELECT COUNT(*) FROM tenant_memberships WHERE user_id = ? AND tenant_id = ?`. Read it twice. Schema changes that introduce `OR user_id IS NULL` (or similar) silently break the chain. Note the signature takes `userID string` separately from `principal` — convention threads it through; the type system does not enforce that it matches. |
| **`CheckResourceAccess`** | `internal/auth/tenant.go:36-52` | SQL: `SELECT COUNT(*) FROM resources WHERE id = ? AND tenant_id = ?`. The `tenant_id` comes from `access.Tenant().Val()` — i.e., the tenant dimension is structurally bound here. This is the link in the chain whose binding *is* type-enforced. |
| **Generated guards** | `internal/shenguard/guards_gen.go` | Generated from the spec; `tcb-audit` catches drift. Spot-check that the lowering of `(= IsMember true) : verified` is `if !(isMember == true) { return ..., err }`. |

### 5. Walk a request through the chain

A real request, from the curl transcript at `demo.md`:

1. `POST /login` → `Sign(claims, secret)` in `jwt.go:31-40`.
2. `GET /tenants/T1/resources/R1` with `Authorization: Bearer <jwt>`
   → middleware runs `Parse` → builds proof chain.
3. Handler calls `auth.CheckTenantAccess(db, principal, userID,
   tenantID)` → SQL lookup → constructor.
4. Handler calls `auth.CheckResourceAccess(db, access, resourceID)`
   → SQL lookup → constructor.
5. Handler reads the resource. The handler's signature demands a
   `ResourceAccess` parameter — it cannot be called from elsewhere
   without first walking steps 3-4.

A cross-tenant request takes the same path, but the SQL at step 3
returns `0`, `isMember := false`, `NewTenantAccess` returns an
error, the handler returns 403. The 403 comes from the constructor
layer, not from a hand-written `if principal.tenant() ==
resource.tenant()` check.

### 6. Known gaps in this demo's spec (read this before you trust the chain)

Two pieces of the chain are convention-only, not type-enforced.
They're documented here because the trust model is meaningless
without them:

1. **Token ↔ user binding is convention.**
   `NewAuthenticatedUser(token JwtToken, expiry TokenExpiry, user
   UserId) AuthenticatedUser` at
   `internal/shenguard/guards_gen.go:97` is **infallible**. It does
   not assert `user == sub(token)`. The middleware
   (`middleware.go:47-48`) does this binding correctly by reading
   `result.Claims.Sub`, but a programmer writing new code can
   construct an `AuthenticatedUser` from any `JwtToken` and any
   `UserId`. **Wave-2 of the HN-follow-up plan introduces a
   `parsed-claims` composite and rewrites this premise as a
   cross-field predicate; until then, the binding is in the TCB,
   not in the type system.**

2. **`CheckTenantAccess` takes `userID string` separately.**
   `internal/auth/tenant.go:15` — the SQL uses the `userID string`
   parameter, not anything extracted from `principal`. Handlers
   thread `human.Auth().User().Val()` through (`handlers.go:84`),
   but the type system does not check that the value matches.

Both of these are listed in the project-level trust model at
[`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md) under "What's
runtime-checked" and "What's assumed (the TCB)."

## What this demo does NOT claim

- That the JWT secret rotation, key management, or distribution is
  solved. The demo uses a static HMAC secret loaded from
  environment variables. Production secret management is out of
  scope.
- That the cross-tenant rejection covers timing-side-channel
  attacks. The SQL `COUNT(*)` returns in roughly constant time
  for an indexed lookup, but the rest of the path has not been
  audited for side channels.
- That SQL injection is structurally impossible. The queries use
  parameter binding (`?`), and the inputs are typed as
  `TenantId`/`ResourceId` wrappers, but Go's `database/sql` is
  doing the actual escaping — that's TCB.
- That a stolen valid JWT cannot be replayed within its expiry
  window. The chain validates the token; revocation lists are not
  modeled in the current spec.

For the full project-level trust model, see
[`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md).

## Further reading

- `README.md` — example walkthrough
- `demo.md` — real curl transcript with JWTs and `go test -v`
  output
- `transcript/discharge_report.json` and `transcript/audit_report.md`
  — the committed audit artifact (committed in parallel via the
  `claude/audit-artifacts` worktree)
- `../../docs/TRUST-MODEL.md` — project-level trust model
