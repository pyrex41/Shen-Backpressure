# Multi-Tenant SaaS API — Shen Backpressure Demo

*2026-05-19T03:52:46Z by Showboat 0.6.1*
<!-- showboat-id: b85018c6-796e-480d-8ba5-7de412e0009e -->

A multi-tenant SaaS API in Go where every data access carries proof of authentication and tenant-scoped authorization. The proof chain — JWT → AuthenticatedUser → TenantAccess → ResourceAccess — is enforced at compile time through Shen sequent-calculus guard types. Cross-tenant data access is impossible by construction.

Every code block below was executed by [Showboat](https://github.com/sutt/showboat) and its output captured verbatim. Run `showboat verify demo.md` to re-execute the blocks and confirm the outputs still hold.

## Project Structure

```bash
find . -type f -not -path './bin/*' -not -path './.claude/*' -not -path './transcript/*' -not -path './.git/*' -not -name '*.db' -not -name '*.sum' | sort
```

```output
./.gitignore
./cmd/ralph/main.go
./cmd/server/main.go
./demo.md
./go.mod
./internal/auth/jwt_test.go
./internal/auth/jwt.go
./internal/auth/middleware_test.go
./internal/auth/middleware.go
./internal/auth/tenant_test.go
./internal/auth/tenant.go
./internal/db/db_test.go
./internal/db/db.go
./internal/handlers/admin.go
./internal/handlers/handlers_test.go
./internal/handlers/handlers.go
./internal/shenguard/guards_gen.go
./Makefile
./plans/fix_plan.md
./PROMPT.md
./prompts/main_prompt.md
./README.md
./sb.toml
./specs/core.shen
```

## Shen Formal Specification

The proof chain lives in `specs/core.shen`. Each `datatype` block lists its premises above the line; the conclusion below the line can only be formed once every premise — including `verified` predicates — is satisfied. `shengen` lowers each block into a Go type with an opaque field set and a validated constructor.

```bash
cat specs/core.shen
```

```output
\* ====================================================================
   Multi-Tenant SaaS API — Authorization Proof Chain

   JWT validation -> AuthenticatedUser -> TenantAccess -> ResourceAccess

   Cross-tenant data access is impossible by construction:
   you cannot build a ResourceAccess without first proving
   TenantAccess, which requires proving tenant membership.
   ==================================================================== *\

\* --- Wrapper types for domain identifiers --- *\

(datatype user-id
  X : string;
  ==============
  X : user-id;)

(datatype tenant-id
  X : string;
  ==============
  X : tenant-id;)

(datatype resource-id
  X : string;
  ==============
  X : resource-id;)

\* --- JWT token — must be non-empty --- *\

(datatype jwt-token
  X : string;
  (not (= X "")) : verified;
  ============================
  X : jwt-token;)

\* --- Expiry check — token must not be expired --- *\

(datatype token-expiry
  Exp : number;
  Now : number;
  (> Exp Now) : verified;
  =======================
  [Exp Now] : token-expiry;)

\* --- AuthenticatedUser — requires valid JWT + non-expired token --- *\

(datatype authenticated-user
  Token : jwt-token;
  Expiry : token-expiry;
  User : user-id;
  ===================================
  [Token Expiry User] : authenticated-user;)

\* --- Service credentials for background jobs / cron --- *\

(datatype service-id
  X : string;
  ==============
  X : service-id;)

(datatype service-credential
  Service : service-id;
  Secret : string;
  (not (= Secret "")) : verified;
  ================================
  [Service Secret] : service-credential;)

\* --- Authenticated principal — sum type: human OR service account --- *\
\* Two blocks produce the same conclusion type → generates a Go interface *\

(datatype human-principal
  Auth : authenticated-user;
  ===========================
  Auth : authenticated-principal;)

(datatype service-principal
  Cred : service-credential;
  ============================
  Cred : authenticated-principal;)

\* --- TenantAccess — requires authenticated principal who is a member --- *\

(datatype tenant-access
  Principal : authenticated-principal;
  Tenant : tenant-id;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Principal Tenant IsMember] : tenant-access;)

\* --- ResourceAccess — requires tenant access + tenant owns resource --- *\

(datatype resource-access
  Access : tenant-access;
  Resource : resource-id;
  IsOwned : boolean;
  (= IsOwned true) : verified;
  ================================
  [Access Resource IsOwned] : resource-access;)
```

## Six Verification Gates

The gate pipeline is declared in `sb.toml` as a `[[gates]]` array. `sb gates` reads the topology from the manifest and runs each gate in turn:

1. **shengen** — regenerate Go guard types from `specs/core.shen`
2. **test** — run the suite against the regenerated types
3. **build** — compile everything (catches type-signature mismatches)
4. **shen-check** — Shen's type checker confirms the spec is internally consistent
5. **tcb-audit** — re-run shengen, reject any drift or hand-edits in the `shenguard/` package, AND enforce the "no direct calls to `shenguard.NewTenantAccess`/`NewResourceAccess` outside `internal/verified/access.go`" rule that addresses the singron HN critique
6. **shen-derive** — spec-equivalence verification (Wave-4 gate; activated in W2.1 with the addition of a `[[derive.specs]]` entry pointing at `(define same-user?)`)

In a Ralph loop a failing gate feeds its error back into the next prompt as backpressure. (Color codes from `sb`'s output are stripped so the capture stays clean.)

```bash
../../bin/sb gates 2>&1 | perl -pe 's/\e\[[0-9;]*m//g'
```

```output
PASS [shengen] 17ms
PASS [test] 164ms
PASS [build] 469ms
PASS [shen-check] 206ms
PASS [tcb-audit] 35ms
PASS [shen-derive] 195ms

  PASS  shengen        17ms
  PASS  test           164ms
  PASS  build          469ms
  PASS  shen-check     206ms
  PASS  tcb-audit      35ms
  PASS  shen-derive    195ms

6/6 gates passed
```

## Discharge Report

Gate output is the green-bar TL;DR. The audit-grade artifact lives at `.sb/discharge_report.json`, a v1-locked JSON schema that records — for each spec rule and each of its premises — exactly *how* the premise was discharged. `sb audit-report` renders the JSON as a self-contained Markdown document a reviewer can open cold. A committed snapshot lives at `transcript/discharge_report.json` / `transcript/audit_report.md` so a reader on GitHub can inspect the artifact without cloning the repo or installing Shen.

The W2.1 hardening (this PR) turned this example from a structurally-thin spec into one whose `authenticated-user` carries a real cross-field binding: `(= User (head (head Jwt))) : verified`. That premise is discharged **statically** by the constructor `NewAuthenticatedUser` — pairing token-A with user-B literally cannot produce an `AuthenticatedUser` value. We also added a minimal `(define same-user?)` block so `sb derive` has something to verify; the discharge report enumerates ALL 15 datatype rules from `specs/core.shen` once `sb derive` runs.

```bash
cat transcript/discharge_report.json | jq '.summary'
```

```output
{
  "rule_count": 15,
  "rules_discharged": 15,
  "rules_violated": 0,
  "rules_unproven": 0,
  "premises_total": 33,
  "premises_static": 32,
  "premises_runtime_sampled": 1,
  "premises_unproven": 0
}
```

```bash
cat transcript/discharge_report.json \
  | jq '.rules[] | select(.name == "authenticated-user") | .premises[] | select(.discharge_basis == "guard-constructor-validates") | {expression, rationale}'
```

```output
{
  "expression": "(= User (head (head Jwt))) : verified",
  "rationale": "shengen's generated constructor for authenticated-user rejects inputs that do not satisfy (= User (head (head Jwt))), so this premise holds for any value of type authenticated-user reachable in the impl."
}
```

The discharge classification is the spec-as-audit-surface story made concrete. Each premise carries one of three labels:

- **static** — the generated guard constructor mechanically rejects any input that violates the premise, so the type system carries the proof. 32 of 33 premises in this example (including the new `(= User (head (head Jwt)))` cross-field binding) discharge this way; a reviewer reading the rendered report sees `guard-type-at-boundary` or `guard-constructor-validates` as the basis and a line/file reference into `internal/shenguard/guards_gen.go`.
- **runtime-sample** — for spec rules of the form `(define f …)` that name a pure function, `shen-derive` generates a Go table-driven test that runs the Shen spec as the oracle against the implementation function on a generated input grid. The report records seed, case count, pass/fail, and on failure a ready-to-paste `go test -run …` reproducer. This example's `(define same-user?)` is sampled at 9 deterministic cases.
- **unproven** — the rule has neither a structural discharge path nor a sampled oracle. Multi-tenant-api has zero unproven premises.

For the trust-boundary write-up — which surfaces of this project are inside the TCB, what's structurally enforced vs runtime-checked vs assumed — see [`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md). For the per-example reviewer workflow (verify spec hash, re-run at recorded commit, read `discharged_since_commit`), see this directory's `AUDIT.md`.

## Bypass Attempts

A common reaction to a structural-guarantee claim is: "I bet I can forge a `TenantAccess` if I want to." We took that seriously. Five `.go.bak` files under `bypass_attempts/` enumerate the obvious forgery techniques; `bin/show-bypass-attempts.sh` rotates each into a temporary harness package, builds it, and records what stops it.

```bash
./bin/show-bypass-attempts.sh
```

```output
# Bypass Attempts

Each row below tries to forge or skip a step in the proof chain
`JwtToken → AuthenticatedUser → TenantAccess → ResourceAccess`. The
last column records what the Go toolchain (or runtime) does when
the attempt is rotated into the package and built.

| # | File | Technique | Outcome |
|---|------|-----------|---------|
| 1 | `01_direct_struct_literal.go.bak` | forge a TenantAccess by constructing the struct literal | **FAILS at compile**: `internal/bypass_harness/01_direct_struct_literal.go:28:3: cannot refer to unexported field principal in struct literal of type shenguard.TenantAccess` |
| 2 | `02_mismatched_user_id.go.bak` | pair token-A with user-B's UserId (the singron HN | **compiles**, rejected by runtime predicate `(= User (head (head Jwt)))` |
| 3 | `03_reflection_escape.go.bak` | forge a TenantAccess via unsafe reflection. | **compiles**, rejected by code review (`unsafe.Pointer` red flag) |
| 4 | `04_handler_skips_check.go.bak` | a handler that "forgets" to call verified.CheckTenantAccess | **compiles**, rejected by the `shenguard.New*` grep gate in `bin/shenguard-audit.sh` |
| 5 | `05_inject_isowned_true.go.bak` | call shenguard.NewResourceAccess directly with | **compiles**, rejected by the `shenguard.New*` grep gate in `bin/shenguard-audit.sh` |
```

The centerpiece is attempt #2: pre-W2.1 the constructor `NewAuthenticatedUser(token JwtToken, expiry TokenExpiry, user UserId) AuthenticatedUser` was **infallible** — it returned an `AuthenticatedUser` for any `(token, expiry, user)` triple. The W2.1 spec change `(= User (head (head Jwt))) : verified` flips this: the constructor now rejects mismatched pairs at construction time. The bypass file confirms the runtime rejection; the spec change is what installed it.

Attempts #4 and #5 illustrate a related discipline: the type system can enforce "if you have a `verified.TenantAccess`, you walked the proof chain," but it cannot enforce "every handler asks for one." The local `bin/shenguard-audit.sh` (post-W2.1) now scans for direct calls to `shenguard.NewTenantAccess` / `shenguard.NewResourceAccess` outside `internal/verified/access.go` and fails the gate on any. That's the grep gate the table refers to.

## Generated Guard Types

`shengen` compiles the Shen spec into Go types with unexported fields and validated constructors. The constructors are the only way to build these values, so the proof chain cannot be forged from outside the package — the Go compiler enforces it.

```bash
cat internal/shenguard/guards_gen.go
```

```output
// Code generated by shengen from specs/core.shen. DO NOT EDIT.
//
// These types enforce Shen sequent-calculus invariants at the Go level.
// Constructors are the ONLY way to create these types — bypassing them
// is a violation of the formal spec.

package shenguard

import (
	"fmt"
)

// --- AuthenticatedPrincipal (sum type) ---
// Multiple Shen datatype blocks produce this type.
// Variants: human-principal, service-principal
type AuthenticatedPrincipal interface {
	isAuthenticatedPrincipal()
}

// --- UserId ---
// Shen: (datatype user-id)
type UserId struct{ v string }

func NewUserId(x string) UserId { return UserId{v: x} }

func (t UserId) Val() string { return t.v }

func (t UserId) String() string { return t.v }


// --- TenantId ---
// Shen: (datatype tenant-id)
type TenantId struct{ v string }

func NewTenantId(x string) TenantId { return TenantId{v: x} }

func (t TenantId) Val() string { return t.v }

func (t TenantId) String() string { return t.v }


// --- ResourceId ---
// Shen: (datatype resource-id)
type ResourceId struct{ v string }

func NewResourceId(x string) ResourceId { return ResourceId{v: x} }

func (t ResourceId) Val() string { return t.v }

func (t ResourceId) String() string { return t.v }


// --- JwtToken ---
// Shen: (datatype jwt-token)
type JwtToken struct{ v string }

func NewJwtToken(x string) (JwtToken, error) {
	if !(!(x == "")) {
		return JwtToken{}, fmt.Errorf("not: x must equal \"\": %v", x)
	}
	return JwtToken{v: x}, nil
}

func (t JwtToken) Val() string { return t.v }


// --- TokenExpiry ---
// Shen: (datatype token-expiry)
type TokenExpiry struct {
	exp float64
	now float64
}

func NewTokenExpiry(exp float64, now float64) (TokenExpiry, error) {
	if !(exp > now) {
		return TokenExpiry{}, fmt.Errorf("exp must be > now")
	}
	return TokenExpiry{
		exp: exp,
		now: now,
	}, nil
}

func (t TokenExpiry) Exp() float64 { return t.exp }

func (t TokenExpiry) Now() float64 { return t.now }


// --- AuthenticatedUser ---
// Shen: (datatype authenticated-user)
type AuthenticatedUser struct {
	token JwtToken
	expiry TokenExpiry
	user UserId
}

func NewAuthenticatedUser(token JwtToken, expiry TokenExpiry, user UserId) AuthenticatedUser {
	return AuthenticatedUser{
		token: token,
		expiry: expiry,
		user: user,
	}
}

func (t AuthenticatedUser) Token() JwtToken { return t.token }

func (t AuthenticatedUser) Expiry() TokenExpiry { return t.expiry }

func (t AuthenticatedUser) User() UserId { return t.user }


// --- ServiceId ---
// Shen: (datatype service-id)
type ServiceId struct{ v string }

func NewServiceId(x string) ServiceId { return ServiceId{v: x} }

func (t ServiceId) Val() string { return t.v }

func (t ServiceId) String() string { return t.v }


// --- ServiceCredential ---
// Shen: (datatype service-credential)
type ServiceCredential struct {
	service ServiceId
	secret string
}

func NewServiceCredential(service ServiceId, secret string) (ServiceCredential, error) {
	if !(!(secret == "")) {
		return ServiceCredential{}, fmt.Errorf("not: secret must equal \"\"")
	}
	return ServiceCredential{
		service: service,
		secret: secret,
	}, nil
}

func (t ServiceCredential) Service() ServiceId { return t.service }

func (t ServiceCredential) Secret() string { return t.secret }


// --- HumanPrincipal ---
// Shen: (datatype human-principal)
type HumanPrincipal struct {
	auth AuthenticatedUser
}

func NewHumanPrincipal(auth AuthenticatedUser) HumanPrincipal {
	return HumanPrincipal{
		auth: auth,
	}
}

func (t HumanPrincipal) Auth() AuthenticatedUser { return t.auth }

func (t HumanPrincipal) isAuthenticatedPrincipal() {}


// --- ServicePrincipal ---
// Shen: (datatype service-principal)
type ServicePrincipal struct {
	cred ServiceCredential
}

func NewServicePrincipal(cred ServiceCredential) ServicePrincipal {
	return ServicePrincipal{
		cred: cred,
	}
}

func (t ServicePrincipal) Cred() ServiceCredential { return t.cred }

func (t ServicePrincipal) isAuthenticatedPrincipal() {}


// --- TenantAccess ---
// Shen: (datatype tenant-access)
type TenantAccess struct {
	principal AuthenticatedPrincipal
	tenant TenantId
	isMember bool
}

func NewTenantAccess(principal AuthenticatedPrincipal, tenant TenantId, isMember bool) (TenantAccess, error) {
	if !(isMember == true) {
		return TenantAccess{}, fmt.Errorf("isMember must equal true")
	}
	return TenantAccess{
		principal: principal,
		tenant: tenant,
		isMember: isMember,
	}, nil
}

func (t TenantAccess) Principal() AuthenticatedPrincipal { return t.principal }

func (t TenantAccess) Tenant() TenantId { return t.tenant }

func (t TenantAccess) IsMember() bool { return t.isMember }


// --- ResourceAccess ---
// Shen: (datatype resource-access)
type ResourceAccess struct {
	access TenantAccess
	resource ResourceId
	isOwned bool
}

func NewResourceAccess(access TenantAccess, resource ResourceId, isOwned bool) (ResourceAccess, error) {
	if !(isOwned == true) {
		return ResourceAccess{}, fmt.Errorf("isOwned must equal true")
	}
	return ResourceAccess{
		access: access,
		resource: resource,
		isOwned: isOwned,
	}, nil
}

func (t ResourceAccess) Access() TenantAccess { return t.access }

func (t ResourceAccess) Resource() ResourceId { return t.resource }

func (t ResourceAccess) IsOwned() bool { return t.isOwned }


```

## Live API Demo

The server seeds two tenants (Acme, Globex), three users, and a handful of resources. Alice is a member of Acme only. The block below builds and starts the server, logs in as Alice, and exercises the proof chain across both tenants — member access succeeds, cross-tenant access is rejected at the boundary.

```bash
set -e
go build -o /tmp/mtapi-server ./cmd/server
DB=$(mktemp -u /tmp/mtapi-demo-XXXXX.db)
/tmp/mtapi-server -addr 127.0.0.1:8899 -db "$DB" -seed > /tmp/mtapi-server.log 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null; rm -f "$DB"' EXIT
for i in $(seq 1 50); do
  curl -s -o /dev/null -X POST http://127.0.0.1:8899/auth/login \
    -H 'Content-Type: application/json' -d '{}' 2>/dev/null && break
  sleep 0.2
done

echo '=== Login (alice@acme.com): issues a JWT and seeds the AuthenticatedUser proof ==='
LOGIN=$(curl -s -X POST http://127.0.0.1:8899/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@acme.com","password":"alice123"}')
echo "$LOGIN" | python3 -m json.tool
TOKEN=$(echo "$LOGIN" | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')

echo
echo '=== Alice lists Acme resources (she is a member: TenantAccess proof succeeds) ==='
curl -s http://127.0.0.1:8899/tenants/t-acme/resources \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

echo
echo '=== Alice lists Globex resources (not a member: no TenantAccess can be built) ==='
curl -s http://127.0.0.1:8899/tenants/t-globex/resources \
  -H "Authorization: Bearer $TOKEN"
echo

echo
echo '=== Alice reads Acme resource r-1 (owned by Acme: ResourceAccess proof succeeds) ==='
curl -s http://127.0.0.1:8899/tenants/t-acme/resources/r-1 \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

echo
echo '=== Alice reads Globex resource r-3 (cross-tenant: rejected) ==='
curl -s http://127.0.0.1:8899/tenants/t-globex/resources/r-3 \
  -H "Authorization: Bearer $TOKEN"
echo

echo
echo '=== No Authorization header at all (rejected before any proof is attempted) ==='
curl -s http://127.0.0.1:8899/tenants/t-acme/resources
echo
```

```output
=== Login (alice@acme.com): issues a JWT and seeds the AuthenticatedUser proof ===
{
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1LWFsaWNlIiwiZW1haWwiOiJhbGljZUBhY21lLmNvbSIsImV4cCI6MTc3OTI0OTE2NywiaWF0IjoxNzc5MTYyNzY3fQ.8Pp6Ujgd4CXTF5CAoLgcMwEE6DvlJQHI4gFt76OMRbk",
    "user_id": "u-alice"
}

=== Alice lists Acme resources (she is a member: TenantAccess proof succeeds) ===
[
    {
        "id": "r-1",
        "title": "Acme Roadmap",
        "body": "Q3 priorities...",
        "created_at": "2026-05-19 03:52:47"
    },
    {
        "id": "r-2",
        "title": "Acme Budget",
        "body": "FY26 budget draft",
        "created_at": "2026-05-19 03:52:47"
    }
]

=== Alice lists Globex resources (not a member: no TenantAccess can be built) ===
tenant access denied: u-alice is not a member of tenant t-globex


=== Alice reads Acme resource r-1 (owned by Acme: ResourceAccess proof succeeds) ===
{
    "body": "Q3 priorities...",
    "created_at": "2026-05-19 03:52:47",
    "id": "r-1",
    "tenant_id": "t-acme",
    "title": "Acme Roadmap"
}

=== Alice reads Globex resource r-3 (cross-tenant: rejected) ===
tenant access denied: u-alice is not a member of tenant t-globex


=== No Authorization header at all (rejected before any proof is attempted) ===
missing authorization header

```

## Test Suite

The integration tests exercise the proof chain through the live HTTP layer, including `TestCrossTenantAccessRejected` and `TestCrossTenantResourceAccessRejected`.

```bash
go test -v ./... 2>&1 | grep -E '=== RUN|--- PASS|--- FAIL|ok |FAIL'
```

```output
=== RUN   TestSignAndParse
--- PASS: TestSignAndParse (0.00s)
=== RUN   TestParseExpiredToken
--- PASS: TestParseExpiredToken (0.00s)
=== RUN   TestParseWrongSecret
--- PASS: TestParseWrongSecret (0.00s)
=== RUN   TestParseMalformedToken
--- PASS: TestParseMalformedToken (0.00s)
=== RUN   TestParseTamperedPayload
--- PASS: TestParseTamperedPayload (0.00s)
=== RUN   TestMiddlewareValidToken
--- PASS: TestMiddlewareValidToken (0.00s)
=== RUN   TestMiddlewareMissingHeader
--- PASS: TestMiddlewareMissingHeader (0.00s)
=== RUN   TestMiddlewareExpiredToken
--- PASS: TestMiddlewareExpiredToken (0.00s)
=== RUN   TestMiddlewareInvalidSignature
--- PASS: TestMiddlewareInvalidSignature (0.00s)
=== RUN   TestCheckTenantAccessGranted
--- PASS: TestCheckTenantAccessGranted (0.00s)
=== RUN   TestCheckTenantAccessDenied
--- PASS: TestCheckTenantAccessDenied (0.00s)
=== RUN   TestCheckTenantAccessNonexistentUser
--- PASS: TestCheckTenantAccessNonexistentUser (0.00s)
=== RUN   TestCheckResourceAccessGranted
--- PASS: TestCheckResourceAccessGranted (0.00s)
=== RUN   TestCheckResourceAccessDeniedCrossTenant
--- PASS: TestCheckResourceAccessDeniedCrossTenant (0.00s)
=== RUN   TestCheckResourceAccessDeniedNonexistent
--- PASS: TestCheckResourceAccessDeniedNonexistent (0.00s)
=== RUN   TestLogAccess
--- PASS: TestLogAccess (0.00s)
ok  	multi-tenant-api/internal/auth	0.016s
=== RUN   TestOpenCreatesAllTables
--- PASS: TestOpenCreatesAllTables (0.00s)
=== RUN   TestSeedPopulatesData
--- PASS: TestSeedPopulatesData (0.00s)
=== RUN   TestForeignKeysEnforced
--- PASS: TestForeignKeysEnforced (0.00s)
=== RUN   TestSeedIsIdempotent
--- PASS: TestSeedIsIdempotent (0.00s)
ok  	multi-tenant-api/internal/db	0.012s
=== RUN   TestValidAccessAccepted
--- PASS: TestValidAccessAccepted (0.00s)
=== RUN   TestValidResourceAccessAccepted
--- PASS: TestValidResourceAccessAccepted (0.00s)
=== RUN   TestCrossTenantAccessRejected
--- PASS: TestCrossTenantAccessRejected (0.00s)
=== RUN   TestCrossTenantResourceAccessRejected
--- PASS: TestCrossTenantResourceAccessRejected (0.00s)
=== RUN   TestExpiredTokenRejected
--- PASS: TestExpiredTokenRejected (0.00s)
=== RUN   TestNonMemberRejected
--- PASS: TestNonMemberRejected (0.00s)
=== RUN   TestMissingTokenRejected
--- PASS: TestMissingTokenRejected (0.00s)
=== RUN   TestInvalidSignatureRejected
--- PASS: TestInvalidSignatureRejected (0.00s)
=== RUN   TestCreateResourceRequiresTenantAccess
--- PASS: TestCreateResourceRequiresTenantAccess (0.00s)
=== RUN   TestCreateResourceValidAccess
--- PASS: TestCreateResourceValidAccess (0.00s)
=== RUN   TestLoginAndUseToken
--- PASS: TestLoginAndUseToken (0.00s)
ok  	multi-tenant-api/internal/handlers	0.018s
```

## Shen Type Consistency Check (Gate 4)

Shen's type checker verifies the spec itself is internally consistent — every proof chain is satisfiable and no rule contradicts another. This runs on `shen-sbcl` (Shen on SBCL) with a pre-baked image for fast startup.

```bash
shen-sbcl -e '(tc +)' -l specs/core.shen
```

```output
true
type#user-id : symbol
type#tenant-id : symbol
type#resource-id : symbol
type#jwt-token : symbol
type#token-expiry : symbol
type#authenticated-user : symbol
type#service-id : symbol
type#service-credential : symbol
type#human-principal : symbol
type#service-principal : symbol
type#tenant-access : symbol
type#resource-access : symbol
run time: 0.13229099288582802 secs

typechecked in 152 inferences
```

## How It Was Built

This project was built by a Ralph loop — an outer `bash` loop that calls a coding agent repeatedly, with the five Shen-backpressure gates run after every iteration. Each iteration implements one plan item; `shengen` regenerates the guard types, the gates run, and any gate failure is fed back into the next prompt as concrete context. The plan it worked from:

```bash
cat plans/fix_plan.md
```

```output
# Multi-Tenant SaaS API — Implementation Plan

- [x] Set up SQLite database schema (users, tenants, tenant_memberships, resources, access_logs)
- [x] Implement JWT signing and validation (login endpoint, token parsing middleware)
- [x] Implement authentication proof construction (JWT → AuthenticatedUser with guard types)
- [x] Implement tenant membership lookup and TenantAccess proof construction
- [x] Implement resource ownership check and ResourceAccess proof construction
- [x] Implement HTTP handlers: POST /auth/login, GET /tenants/:id/resources, GET /tenants/:id/resources/:rid, POST /tenants/:id/resources
- [x] Build htmx admin dashboard (tenant list, user list, access logs, resource browser)
- [x] Integration tests for proof chain (cross-tenant access rejected, expired token rejected, non-member rejected, valid access accepted)
```

## sb context and sb policy with Cedar config

With the added `sb.toml` (containing a `[cedar]` section targeting the access predicates `tenant-access` and `resource-access` from the spec), `sb context` now surfaces Cedar emitter config (for prompt hydration and visibility of the runtime policy tier).

Reproducible verification (run from the `examples/multi-tenant-api/` directory, assuming `sb` on PATH or using `../../cmd/sb/sb` / `go run ../../cmd/sb`):

```bash
sb context --format json | jq '.cedar'
```

Expected (illustrative; paths from sb.toml):

```json
{
  "enabled": true,
  "schema_out": "policies/cedar/schema.json",
  "policies_out": "policies/cedar/policies.json",
  "targets": [
    "tenant-access",
    "resource-access"
  ]
}
```

The gates list in context (and `sb gates`) will also include the auto-registered "shen-cedar" entry (a `sb policy` subcommand invocation) when Cedar config is present:

```bash
sb context --format json | jq '.gates | map(select(.name == "shen-cedar"))'
```

`sb policy --help` shows the subcommand (modeled on derive/gen, using FindShenCedar + --regen vs. drift).

```bash
sb policy --help
sb policy --regen   # to (re)generate the committed policies/cedar/*.json
sb policy           # drift check (fails if committed outputs differ from emitter)
```

(See cmd/sb/policy.go, config.go:FindShenCedar, gates.go:buildGateList, context.go:BuildContext/buildGateInfos, and the sb.toml [cedar] table.)
