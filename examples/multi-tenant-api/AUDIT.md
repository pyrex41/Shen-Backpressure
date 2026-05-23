# Multi-Tenant API — Auditor Workflow

> One-page reviewer workflow for the JWT/authorization proof chain
> demo. Read [`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md) first
> for the project-level trust model.
>
> **W2.1 update (this PR).** The two convention-only gaps that earlier
> versions of this document called out — (1) the token↔user binding
> and (2) `CheckTenantAccess`'s redundant `userID string` parameter
> — have been closed. See the "What's new in W2.1" section below.

## What this demo proves

A live Go HTTP service where every data access carries a compile-time
proof:

```
raw JWT → ParsedClaims → VerifiedJwt → AuthenticatedUser
  → TenantAccess → ResourceAccess
```

A handler that demands `verified.ResourceAccess` cannot be called
without the caller having walked the whole chain. Cross-tenant data
access is **impossible by construction** in the structural sense —
there is no Go expression that produces a `ResourceAccess` for the
wrong tenant.

The user-id is structurally bound to the JWT's `sub` claim. The
spec premise `(= User (head (head Jwt))) : verified` inside
`authenticated-user` is discharged at construction time, so
`NewAuthenticatedUser(jwt, NewUserId("u-bob"))` for a JWT whose
`sub` is `u-alice` returns an error rather than an
`AuthenticatedUser`.

Three integration tests cover the chain:

- `TestCrossTenantAccessRejected` and
  `TestCrossTenantResourceAccessRejected` in
  `internal/handlers/handlers_test.go` exercise the chain through
  the live HTTP layer.
- `TestCrossFieldBindingRejectsMismatch` in
  `internal/verified/access_test.go` is the unit test for the W2.1
  cross-field binding: it pairs Alice's JWT with Bob's UserId and
  asserts the constructor returns an error.

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
PASS  shengen        17ms
PASS  test           164ms
PASS  build          469ms
PASS  shen-check     206ms      ← needs shen-sbcl; otherwise FAIL
PASS  tcb-audit      35ms
PASS  shen-derive    195ms

6/6 gates passed
```

`tcb-audit` (`bin/shenguard-audit.sh`) is the gate that catches
two specific classes of attack:

1. Someone hand-editing `internal/shenguard/guards_gen.go` to
   weaken a constructor. The script re-runs shengen on the spec
   and byte-diffs against the committed generated file.

2. **W2.1 addition.** Someone calling `shenguard.NewTenantAccess`
   or `shenguard.NewResourceAccess` from outside
   `internal/verified/access.go`. These constructors are
   "would-be-package-private" — Go does not let us hide them, but
   the grep gate enforces that the only file allowed to call them
   is the one that consults the DB first. The gate fails on any
   direct call from a handler or test file.

`shen-derive` (gate 6) generates a table-driven spec-equivalence
test for the `(define same-user?)` block and runs `go test` against
the committed copy. It also emits the discharge report enumerated
below.

### 4. Read the TCB for this demo

The structural chain rests on a small named TCB. **Read every line
of these files** — that is the audit.

| Piece | Where | What you're checking |
|---|---|---|
| **JWT parser** | `internal/auth/jwt.go` (`Parse` function) | HMAC-SHA256 with `crypto/hmac.Equal` (constant-time). Verify the order: signature → JSON-decode → expiry. Look for short-circuits that decode before verifying. The test `TestParseTamperedPayload` is the obvious threat. Post-W2.1 the parser also returns the raw signature segment alongside the claims so the middleware can thread it into `shenguard.NewVerifiedJwt`. |
| **Middleware** | `internal/auth/middleware.go` (`buildPrincipal`) | The proof-chain construction. Verify that every step uses values derived from the parsed JWT (no hard-coded strings other than the demo defaults for issuer/audience). The structural binding `(= User (head (head Jwt)))` is enforced inside `shenguard.NewAuthenticatedUser` — the middleware just threads the same `UserId` through both `NewParsedClaims` and `NewAuthenticatedUser`. |
| **`verified.CheckTenantAccess`** | `internal/verified/access.go` | The SQL query is `SELECT COUNT(*) FROM tenant_memberships WHERE user_id = ? AND tenant_id = ?`. Read it twice. Schema changes that introduce `OR user_id IS NULL` (or similar) silently break the chain. Post-W2.1 the function **derives the `userID` from `principal.Auth().User().Val()`** — there is no separately-passed `userID string` parameter. |
| **`verified.CheckResourceAccess`** | `internal/verified/access.go` | SQL: `SELECT COUNT(*) FROM resources WHERE id = ? AND tenant_id = ?`. The `tenant_id` comes from `access.Tenant().Val()` — i.e., the tenant dimension is structurally bound here. This is the link in the chain whose binding *is* type-enforced. |
| **Generated guards** | `internal/shenguard/guards_gen.go` | Generated from the spec; `tcb-audit` catches drift. Spot-check that the lowering of `(= User (head (head Jwt))) : verified` inside `NewAuthenticatedUser` is `if !(user == jwt.claims.sub) { return ..., err }` and that of `(= IsMember true) : verified` inside `NewTenantAccess` is `if !(isMember == true) { return ..., err }`. |
| **Grep gate** | `bin/shenguard-audit.sh` step 2b | The script greps the source tree for `shenguard.NewTenantAccess` / `shenguard.NewResourceAccess` outside `internal/verified/access.go` and fails on any. This is the social half of the package-private discipline. |

### 5. Walk a request through the chain

A real request, from the curl transcript at `demo.md`:

1. `POST /login` → `Sign(claims, secret)` in `internal/auth/jwt.go`.
2. `GET /tenants/T1/resources/R1` with `Authorization: Bearer <jwt>`
   → middleware runs `Parse` → `buildPrincipal` walks the
   shenguard constructor chain (NewJwtIssuer → NewParsedClaims →
   NewVerifiedJwt → NewAuthenticatedUser).
3. Handler calls `verified.CheckTenantAccess(db, principal,
   tenantID)` — note: NO `userID string` parameter. The function
   derives the user-id from the principal. → SQL lookup →
   `shenguard.NewTenantAccess`.
4. Handler calls `verified.CheckResourceAccess(db, access,
   resourceID)` → SQL lookup → `shenguard.NewResourceAccess`.
5. Handler reads the resource. The handler's signature demands a
   `verified.ResourceAccess` parameter — it cannot be called from
   elsewhere without first walking steps 3–4.

A cross-tenant request takes the same path, but the SQL at step 3
returns `0`, `isMember := false`, `NewTenantAccess` returns an
error, the handler returns 403. The 403 comes from the constructor
layer, not from a hand-written `if principal.tenant() ==
resource.tenant()` check.

### 6. What's new in W2.1 (this PR)

The previous version of this document listed two
convention-only gaps. Both are now closed:

1. **Token ↔ user binding is now structural.** The spec premise
   `(= User (head (head Jwt))) : verified` inside `authenticated-user`
   makes `NewAuthenticatedUser(jwt, NewUserId("u-bob"))` for a JWT
   whose `sub` is `u-alice` return an error rather than an
   `AuthenticatedUser`. The discharge report classifies this
   premise as `static` with basis `guard-constructor-validates`.
   See `transcript/audit_report.md` for the rendering.

2. **`verified.CheckTenantAccess` derives userID from the
   principal.** The function moved from `internal/auth/tenant.go`
   to `internal/verified/access.go` and dropped the `userID
   string` parameter. The SQL query is now structurally keyed by
   the authenticated user.

The package-private discipline for `NewTenantAccess` /
`NewResourceAccess` is not perfect (Go forces them to be
exported by shengen), but the local `bin/shenguard-audit.sh` greps
for direct calls outside `internal/verified/access.go` and fails
the gate on any. The `bypass_attempts/` directory ships five
forging attempts that exercise the remaining defences; run
`bin/show-bypass-attempts.sh` to reproduce the table in
`demo.md`'s "Bypass Attempts" section.

The bigger residual TCB items are unchanged: the JWT parser, the
SQL queries, the shengen emitter itself, and the (small) Shen
runtime. They are listed in
[`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md) under "What's
assumed (the TCB)."

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
