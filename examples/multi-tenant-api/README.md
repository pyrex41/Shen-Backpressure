# Multi-Tenant API — Authorization Proof Chain Demo

A live Go HTTP service where every data access carries a compile-time
proof of authentication and tenant-scoped authorization. The proof
chain — raw JWT → ParsedClaims → VerifiedJwt → AuthenticatedUser →
TenantAccess → ResourceAccess — is generated from a Shen
sequent-calculus spec and enforced by the Go type system.

For the project-level framing, see the [top-level
README](../../README.md). For the trust model — what is
structurally enforced vs runtime-checked vs assumed — see
[`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md). This file walks
the example end-to-end.

## What you'll see

**Cross-tenant data access is impossible by construction.** A
handler that wants to read a resource needs a `verified.ResourceAccess`
value. The only way to construct one is to thread a verified
`TenantAccess` through `verified.CheckResourceAccess`, which in turn
requires proof that the principal is a member of the tenant that
owns the resource.

**The user-id is structurally bound to the JWT's `sub` claim.** This
is the W2.1 hardening: the spec premise `(= User (head (head Jwt))) :
verified` inside `authenticated-user` is discharged at construction
time, so the constructor refuses to build an `AuthenticatedUser`
whose `User` disagrees with the JWT's `sub` claim. Pre-W2.1 the
binding was convention-only — a caller in possession of *any*
`JwtToken` and *any* `UserId` could pair them and the constructor
was infallible. The HN critique from singron/max_unbearable was
explicit on this point; the fix is now in the spec.

**`CheckTenantAccess` derives the user-id from the principal.**
Pre-W2.1 the signature was `CheckTenantAccess(db, principal, userID
string, tenantID)` — the SQL used the *string* parameter, decoupled
from the proof object. Post-W2.1 the function lives in
`internal/verified/access.go` (not `internal/auth/`) and reads the
user-id directly from the principal via the W2.1 cross-field binding
above. The type system now enforces that the queried user-id is the
authenticated user-id.

The handler tests in `internal/handlers/handlers_test.go` include
`TestCrossTenantAccessRejected` and `TestCrossTenantResourceAccessRejected`,
which exercise this through the live HTTP layer. The
`internal/verified/access_test.go` test
`TestCrossFieldBindingRejectsMismatch` is the unit test for the new
spec premise: it constructs Alice's JWT and tries to pair it with
Bob's UserId; the constructor returns an error.

`bypass_attempts/` ships five `.go.bak` files demonstrating common
forging techniques — `bin/show-bypass-attempts.sh` rotates each into
a harness and reports which gate stops it. `demo.md` shows the
output of that script alongside the curl transcript.

## Prerequisites

- Go 1.24+
- `bin/sb` from the repo root (`make build-sb`)
- Optional for Gate 4: `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl`

The example ships an `sb.toml` that declares the six-gate pipeline
explicitly via the `[[gates]]` array. `sb gates` reads the gate
topology straight from the manifest. The sixth gate (`shen-derive`)
is auto-appended by the engine whenever the manifest contains at
least one `[[derive.specs]]` entry — W2.1 added one targeting
`(define same-user?)`, which anchors the discharge report so all
15 datatype rules in `specs/core.shen` get enumerated.

## Walkthrough

```bash
# From the repo root
make build-sb
cd examples/multi-tenant-api

# Run the test suite — this is the canonical pass case.
go test ./...
```

Expected: all tests pass. The handler suite includes the
cross-tenant rejection tests; the auth suite covers JWT signing,
expiry, tampering, and middleware.

```bash
# Run the gate set the way an agent loop would. `sb gates` reads the
# pipeline from sb.toml. `make build-sb` writes the binary to bin/sb
# at the repo root, not on $PATH; reference it explicitly.
../../bin/sb gates
```

Expected output — all six gates pass:

```
PASS  shengen        17ms
PASS  test           164ms
PASS  build          469ms
PASS  shen-check     206ms
PASS  tcb-audit      35ms
PASS  shen-derive    195ms

6/6 gates passed
```

The `shengen` and `tcb-audit` gates locate `shengen` via the
repo-root `cmd/shengen`, building it into `bin/` on first run; Gate 4
(`shen-check`) needs `shen-sbcl` on the system (see Prerequisites).
From a checkout that has neither the repo-root sources nor
`shen-sbcl`, `test` and `build` still run as the baseline guardrails.

## The proof chain

`specs/core.shen` declares the chain. Each datatype is a sequent
rule: the things above the line must hold for the thing below the
line to be constructable.

The W2.1 hardening introduced two new composite types — `parsed-claims`
and `verified-jwt` — that sit between the raw JWT bytes and the
`authenticated-user` proof. Their job is to give the spec
something concrete to talk about when it asserts the token↔user
binding.

```shen
;; The JSON-decoded payload of a JWT. Fields ordered so that
;; (head Claims) resolves to Sub.
(datatype parsed-claims
  Sub : user-id;
  Exp : number;
  Iss : jwt-issuer;
  Aud : jwt-audience;
  (> Exp 0) : verified;
  ==================================
  [Sub Exp Iss Aud] : parsed-claims;)

;; A JWT whose signature is non-empty. (The HMAC verification itself
;; happens in internal/auth/jwt.go and is part of the TCB.)
(datatype verified-jwt
  Claims : parsed-claims;
  Sig : string;
  (not (= Sig "")) : verified;
  ==========================================
  [Claims Sig] : verified-jwt;)

;; AuthenticatedUser binds the user-id structurally to the JWT's
;; sub claim. The verified premise below is the W2.1 centerpiece:
;; the constructor refuses to build an AuthenticatedUser whose
;; `User` disagrees with `(head (head Jwt))` (i.e., the Sub field
;; of the VerifiedJwt's ParsedClaims).
(datatype authenticated-user
  Jwt : verified-jwt;
  User : user-id;
  (= User (head (head Jwt))) : verified;
  ===========================================
  [Jwt User] : authenticated-user;)
```

The shengen output gives every Go handler the same guarantee:

```go
func NewAuthenticatedUser(jwt VerifiedJwt, user UserId) (AuthenticatedUser, error) {
    if !(user == jwt.claims.sub) {
        return AuthenticatedUser{}, fmt.Errorf("user must equal jwt.claims.sub")
    }
    return AuthenticatedUser{jwt: jwt, user: user}, nil
}

func NewTenantAccess(p AuthenticatedPrincipal, t TenantId, isMember bool) (TenantAccess, error) {
    if !(isMember == true) {
        return TenantAccess{}, fmt.Errorf("isMember must equal true")
    }
    return TenantAccess{principal: p, tenant: t, isMember: isMember}, nil
}
```

`AuthenticatedPrincipal` is a sum type generated from two
contributing datatype blocks (`human-principal` and
`service-principal`) — this is how the same proof chain handles
both interactive logins and background-job service credentials.

The handler can demand `verified.ResourceAccess` as a parameter,
knowing the caller could not have produced one without going through
the whole chain. The wrapper `verified.CheckTenantAccess` (the only
public path to a `TenantAccess`) reads the user-id from the
principal — it no longer takes a separately-passed `userID string`
parameter, so the SQL query is structurally keyed by the
authenticated user.

## What's here

```
specs/core.shen                   Source of truth — full proof chain
internal/shenguard/guards_gen.go  Generated by shengen (do not hand-edit)
internal/auth/                    JWT signing, parsing, middleware
internal/verified/                Check* wrappers — the only public path to TenantAccess / ResourceAccess
internal/derived/                 Hand-written impls that shen-derive verifies against the spec
internal/handlers/                HTTP handlers; admin endpoints; cross-tenant tests
internal/db/                      SQLite-backed storage layer
bypass_attempts/                  Five .go.bak forging attempts; bin/show-bypass-attempts.sh runs them all
cmd/server/main.go                HTTP server entry point
cmd/ralph/main.go                 Pre-engine Ralph loop (predates sb loop)
demo.md                           Showboat-format curl transcript with real JWTs
transcript/                       Committed audit artifacts (discharge_report.json / audit_report.md)
                                  plus the JSONL session record from the Ralph build-out
Makefile                          all / shengen / test / build / shen-check / audit / run / run-relaxed
```

## Running the live API

```bash
go run ./cmd/server
```

The server starts on localhost and exposes the auth + tenant +
resource endpoints used in `demo.md`. The transcript shows real
curl invocations: login produces a JWT, subsequent calls thread it
through the proof chain, and cross-tenant attempts return 403 from
the constructor layer rather than from a hand-written check.

## Reference: the curl walkthrough

`demo.md` is the canonical transcript. It is structured Showboat
output, so the prose, commands, and expected output all interleave.
Read it after this README — it shows the chain in action against a
running server with real tokens.

## Auditor workflow

If you're reviewing this demo for security or compliance purposes,
start with [`AUDIT.md`](AUDIT.md) — a one-page reviewer workflow
that walks through verifying the spec hash, reading the committed
audit report at `transcript/audit_report.md`, re-running gates at
the recorded commit, and reading the named TCB (JWT parser,
`verified.CheckTenantAccess` / `verified.CheckResourceAccess`, the
SQL queries, the `shengen.NewAuthenticatedUser` cross-field
binding). The project-level trust model lives at
[`../../docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md).
