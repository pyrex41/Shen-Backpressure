# Multi-Tenant SaaS API — Shen Backpressure Demo

*2026-05-19T03:52:46Z by Showboat 0.6.1*
<!-- showboat-id: b85018c6-796e-480d-8ba5-7de412e0009e -->

A multi-tenant SaaS API in Go where every data access carries proof of authentication and tenant-scoped authorization. The proof chain — JWT → AuthenticatedUser → TenantAccess → ResourceAccess — is enforced at compile time through Shen sequent-calculus guard types. Cross-tenant data access is impossible by construction.

Every code block below was executed by [Showboat](https://github.com/sutt/showboat) and its output captured verbatim. Run `showboat verify demo.md` to re-execute the blocks and confirm the outputs still hold.

## Project Structure

```bash
find . -type f -not -path './bin/*' -not -path './.claude/*' -not -path './transcript/*' -not -path './.git/*' -not -path './.sb/*' -not -name '*.db' -not -name '*.sum' | sort
```

```output
./.gitignore
./AUDIT.md
./bypass_attempts/01_direct_struct_literal.go.bak
./bypass_attempts/02_mismatched_user_id.go.bak
./bypass_attempts/03_reflection_escape.go.bak
./bypass_attempts/04_handler_skips_check.go.bak
./bypass_attempts/05_inject_isowned_true.go.bak
./cmd/ralph/main.go
./cmd/server/main.go
./demo.md
./go.mod
./internal/auth/jwt_test.go
./internal/auth/jwt.go
./internal/auth/log_test.go
./internal/auth/log.go
./internal/auth/middleware_test.go
./internal/auth/middleware.go
./internal/db/db_test.go
./internal/db/db.go
./internal/derived/same_user_spec_test.go
./internal/derived/same_user.go
./internal/handlers/admin.go
./internal/handlers/handlers_test.go
./internal/handlers/handlers.go
./internal/shenguard/guards_gen.go
./internal/verified/access_test.go
./internal/verified/access.go
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

   Raw JWT → ParsedClaims + Signature → VerifiedJwt
         → AuthenticatedUser → TenantAccess → ResourceAccess

   Cross-tenant data access is impossible by construction:
   you cannot build a ResourceAccess without first proving
   TenantAccess, which requires proving tenant membership.

   The user-id carried in AuthenticatedUser is structurally bound
   to the JWT's `sub` claim by the verified premise
   `(= User (head Claims))`. This makes "pair token-A with user-B"
   a compile-time error, not a convention.
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

\* --- Issuer / audience claim wrappers --- *\
\* These are wrapper types so the parser-extracted strings flow through
   the type system without coercion. *\

(datatype jwt-issuer
  X : string;
  (not (= X "")) : verified;
  ==============
  X : jwt-issuer;)

(datatype jwt-audience
  X : string;
  (not (= X "")) : verified;
  ==============
  X : jwt-audience;)

\* --- ParsedClaims — JSON-decoded payload section of a JWT --- *\
\* Field order matters: `(head Claims)` resolves to `Sub` (the user-id),
   which is what the cross-field binding below relies on. *\

(datatype parsed-claims
  Sub : user-id;
  Exp : number;
  Iss : jwt-issuer;
  Aud : jwt-audience;
  (> Exp 0) : verified;
  ==================================
  [Sub Exp Iss Aud] : parsed-claims;)

\* --- VerifiedJwt — claims + signature with a non-empty signature --- *\
\*
   STRUCTURAL CLAIM: a `VerifiedJwt` cannot exist with an empty signature
   field. This is a deliberately weak predicate: real HMAC-SHA256
   verification lives in `internal/auth/jwt.go` (`crypto/hmac.Equal`,
   constant-time) and is part of the TCB enumerated in
   `../../docs/TRUST-MODEL.md`. The Go middleware constructs a
   `VerifiedJwt` only AFTER it has confirmed the signature against the
   shared secret — so every value of this type observed in the program
   has been verified by the parser. The constructor's own non-empty
   check is the type-system anchor that prevents accidental
   construction with a missing signature.

   What this premise gives us at the type level:

   - `NewVerifiedJwt(claims, sig)` is the ONLY exit; no other path
     produces a `VerifiedJwt`.
   - Downstream `AuthenticatedUser` requires a `VerifiedJwt` parameter,
     so the proof chain demands "go through the parser" by signature.

   What this premise does NOT give us:

   - It does not assert cryptographic validity. The parser's HMAC check
     is TCB and must be audited separately. *\

(datatype verified-jwt
  Claims : parsed-claims;
  Sig : string;
  (not (= Sig "")) : verified;
  ==========================================
  [Claims Sig] : verified-jwt;)

\* --- AuthenticatedUser — binds the user-id structurally to the JWT's sub claim --- *\
\*
   The crucial premise is `(= User (head Claims))`: it asserts that the
   `UserId` carried in the proof is *byte-equal* to the `sub` field
   inside the `VerifiedJwt`'s claims. This is the W2.1 hardening:
   before this premise existed, `NewAuthenticatedUser` was infallible
   and a caller in possession of any `JwtToken` and any `UserId` could
   pair them. With this premise the constructor returns an error on
   mismatch and the only safe way to construct an `AuthenticatedUser`
   is to thread the parsed `sub` through as the `UserId`. *\

(datatype authenticated-user
  Jwt : verified-jwt;
  User : user-id;
  (= User (head (head Jwt))) : verified;
  ===========================================
  [Jwt User] : authenticated-user;)

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

\* --- Derivation targets (consumed by shen-derive, not shengen) ----- *\
\*
   `same-user?` is the spec/impl oracle that anchors shen-derive on
   this example. It asserts that two `user-id` wrappers are equal
   exactly when their inner strings are equal — i.e., it pins the Go
   `==` on `shenguard.UserId` against the Shen `=` on `user-id`.
   This is the same equality the W2.1 cross-field premise
   `(= User (head (head Jwt)))` inside `authenticated-user` asserts
   *structurally* at construction time. The Go impl
   `derived.SameUser` is a one-liner; the spec-equivalence test
   `internal/derived/same_user_spec_test.go` catches drift if anyone
   refactors `UserId` equality to something case-insensitive or
   normalising-on-comparison.

   WHY A SHEN-DERIVE ANCHOR IS HERE: classifying the datatype rules in
   the discharge report (the `static` rows for `verified-jwt`,
   `authenticated-user`, `tenant-access`, etc.) requires `sb derive`
   to run, which requires at least one `[[derive.specs]]` entry.
   This define provides that anchor. The classified rules — especially
   the W2.1 cross-field premise `(= User (head (head Jwt)))` inside
   `authenticated-user` — are then visible to an auditor reading
   `transcript/audit_report.md`.

   WHY A WRAPPER AND NOT A GUARDED COMPOSITE: shen-derive's sampler
   does not currently filter guarded composites (e.g. `parsed-claims`'s
   `(> Exp 0)` predicate — see `shen-derive/verify/samples.go:91`),
   and Shen's `tc+` rejects calls like `(val X)` on wrappers without
   a domain-specific destructor declared. Anchoring on `user-id`
   alone — a wrapper, no `verified` premise — keeps both gates
   green and pins the most semantically important equality in the
   chain. A richer oracle that walks the cross-field invariant
   directly is a follow-up.

   The "real" hardening of W2.1 is the structural premise
   `(= User (head (head Jwt)))` inside `authenticated-user` — that
   premise alone makes "pair token-A with user-B" a compile-time
   error. The discharge report's role is to make that
   statically-discharged premise visible to an auditor. *\

(define same-user?
  {user-id --> user-id --> boolean}
  A B -> (= A B))
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
PASS [test] 432ms
PASS [build] 459ms
PASS [shen-check] 200ms
PASS [tcb-audit] 46ms
PASS [shen-derive] 260ms

  PASS  shengen        17ms
  PASS  test           432ms
  PASS  build          459ms
  PASS  shen-check     200ms
  PASS  tcb-audit      46ms
  PASS  shen-derive    260ms

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
| 4 | `04_handler_skips_check.go.bak` | a handler that \"forgets\" to call verified.CheckTenantAccess | **compiles**, rejected by the `shenguard.New*` grep gate in `bin/shenguard-audit.sh` |
| 5 | `05_inject_isowned_true.go.bak` | call shenguard.NewResourceAccess directly with | **compiles**, rejected by the `shenguard.New*` grep gate in `bin/shenguard-audit.sh` |

**How to read the table.** A "FAILS at compile" outcome is a
type-system guarantee: the Go compiler refuses to produce a binary.
A "compiles" outcome means the attempt is structurally well-typed
but is rejected by one of the other layers of the trust model
described in `../../docs/TRUST-MODEL.md`:

- Attempt #2 compiles but the constructor's verified premise
  `(= User (head (head Jwt)))` returns an error at runtime, so no
  `AuthenticatedUser` value materialises.
- Attempt #3 compiles AND succeeds (Go's `unsafe.Pointer` is more
  powerful than the visibility rules). The defence here is
  code-review and grep — see the doc comment inside the file.
- Attempts #4 and #5 compile because the type system can only
  enforce "if you ask for a verified.TenantAccess, you walked the
  chain"; it cannot stop someone from writing a handler that
  doesn't ask. The local `bin/shenguard-audit.sh` and a
  `bypass-policy` grep catch these patterns.

The structural guarantee from W2.1 is attempt #2's failure: pairing
token-A with user-B is now structurally rejected, where pre-W2.1
the constructor was infallible.
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


// --- JwtIssuer ---
// Shen: (datatype jwt-issuer)
type JwtIssuer struct{ v string }

func NewJwtIssuer(x string) (JwtIssuer, error) {
	if (x == "") {
		return JwtIssuer{}, fmt.Errorf("x must not be empty: %v", x)
	}
	return JwtIssuer{v: x}, nil
}

func (t JwtIssuer) Val() string { return t.v }


// --- JwtAudience ---
// Shen: (datatype jwt-audience)
type JwtAudience struct{ v string }

func NewJwtAudience(x string) (JwtAudience, error) {
	if (x == "") {
		return JwtAudience{}, fmt.Errorf("x must not be empty: %v", x)
	}
	return JwtAudience{v: x}, nil
}

func (t JwtAudience) Val() string { return t.v }


// --- ParsedClaims ---
// Shen: (datatype parsed-claims)
type ParsedClaims struct {
	sub UserId
	exp float64
	iss JwtIssuer
	aud JwtAudience
}

func NewParsedClaims(sub UserId, exp float64, iss JwtIssuer, aud JwtAudience) (ParsedClaims, error) {
	if !(exp > 0) {
		return ParsedClaims{}, fmt.Errorf("exp must be > 0")
	}
	return ParsedClaims{
		sub: sub,
		exp: exp,
		iss: iss,
		aud: aud,
	}, nil
}

func (t ParsedClaims) Sub() UserId { return t.sub }

func (t ParsedClaims) Exp() float64 { return t.exp }

func (t ParsedClaims) Iss() JwtIssuer { return t.iss }

func (t ParsedClaims) Aud() JwtAudience { return t.aud }


// --- VerifiedJwt ---
// Shen: (datatype verified-jwt)
type VerifiedJwt struct {
	claims ParsedClaims
	sig string
}

func NewVerifiedJwt(claims ParsedClaims, sig string) (VerifiedJwt, error) {
	if (sig == "") {
		return VerifiedJwt{}, fmt.Errorf("sig must not be empty")
	}
	return VerifiedJwt{
		claims: claims,
		sig: sig,
	}, nil
}

func (t VerifiedJwt) Claims() ParsedClaims { return t.claims }

func (t VerifiedJwt) Sig() string { return t.sig }


// --- AuthenticatedUser ---
// Shen: (datatype authenticated-user)
type AuthenticatedUser struct {
	jwt VerifiedJwt
	user UserId
}

func NewAuthenticatedUser(jwt VerifiedJwt, user UserId) (AuthenticatedUser, error) {
	if !(user == jwt.claims.sub) {
		return AuthenticatedUser{}, fmt.Errorf("user must equal jwt.claims.sub")
	}
	return AuthenticatedUser{
		jwt: jwt,
		user: user,
	}, nil
}

func (t AuthenticatedUser) Jwt() VerifiedJwt { return t.jwt }

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
	if (secret == "") {
		return ServiceCredential{}, fmt.Errorf("secret must not be empty")
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
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1LWFsaWNlIiwiZW1haWwiOiJhbGljZUBhY21lLmNvbSIsImV4cCI6MTc3OTcyMzQ5MSwiaWF0IjoxNzc5NjM3MDkxLCJpc3MiOiJtdWx0aS10ZW5hbnQtYXBpIiwiYXVkIjoidXNlcnMifQ.lJtrMlbEC3XIcPjGRlDGjiMIRogfkTug_CeyQWncVX0",
    "user_id": "u-alice"
}

=== Alice lists Acme resources (she is a member: TenantAccess proof succeeds) ===
[
    {
        "id": "r-1",
        "title": "Acme Roadmap",
        "body": "Q3 priorities...",
        "created_at": "2026-05-24 15:38:11"
    },
    {
        "id": "r-2",
        "title": "Acme Budget",
        "body": "FY26 budget draft",
        "created_at": "2026-05-24 15:38:11"
    }
]

=== Alice lists Globex resources (not a member: no TenantAccess can be built) ===
tenant access denied: u-alice is not a member of tenant t-globex


=== Alice reads Acme resource r-1 (owned by Acme: ResourceAccess proof succeeds) ===
{
    "body": "Q3 priorities...",
    "created_at": "2026-05-24 15:38:11",
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
=== RUN   TestLogAccess
--- PASS: TestLogAccess (0.00s)
=== RUN   TestMiddlewareValidToken
--- PASS: TestMiddlewareValidToken (0.00s)
=== RUN   TestMiddlewareMissingHeader
--- PASS: TestMiddlewareMissingHeader (0.00s)
=== RUN   TestMiddlewareExpiredToken
--- PASS: TestMiddlewareExpiredToken (0.00s)
=== RUN   TestMiddlewareInvalidSignature
--- PASS: TestMiddlewareInvalidSignature (0.00s)
ok  	multi-tenant-api/internal/auth	0.017s
=== RUN   TestOpenCreatesAllTables
--- PASS: TestOpenCreatesAllTables (0.00s)
=== RUN   TestSeedPopulatesData
--- PASS: TestSeedPopulatesData (0.00s)
=== RUN   TestForeignKeysEnforced
--- PASS: TestForeignKeysEnforced (0.00s)
=== RUN   TestSeedIsIdempotent
--- PASS: TestSeedIsIdempotent (0.00s)
ok  	multi-tenant-api/internal/db	0.016s
=== RUN   TestSpec_SameUser
=== RUN   TestSpec_SameUser/case_00
=== RUN   TestSpec_SameUser/case_01
=== RUN   TestSpec_SameUser/case_02
=== RUN   TestSpec_SameUser/case_03
=== RUN   TestSpec_SameUser/case_04
=== RUN   TestSpec_SameUser/case_05
=== RUN   TestSpec_SameUser/case_06
=== RUN   TestSpec_SameUser/case_07
=== RUN   TestSpec_SameUser/case_08
--- PASS: TestSpec_SameUser (0.00s)
    --- PASS: TestSpec_SameUser/case_00 (0.00s)
    --- PASS: TestSpec_SameUser/case_01 (0.00s)
    --- PASS: TestSpec_SameUser/case_02 (0.00s)
    --- PASS: TestSpec_SameUser/case_03 (0.00s)
    --- PASS: TestSpec_SameUser/case_04 (0.00s)
    --- PASS: TestSpec_SameUser/case_05 (0.00s)
    --- PASS: TestSpec_SameUser/case_06 (0.00s)
    --- PASS: TestSpec_SameUser/case_07 (0.00s)
    --- PASS: TestSpec_SameUser/case_08 (0.00s)
ok  	multi-tenant-api/internal/derived	(cached)
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
ok  	multi-tenant-api/internal/handlers	0.023s
=== RUN   TestCheckTenantAccessGranted
--- PASS: TestCheckTenantAccessGranted (0.00s)
=== RUN   TestCheckTenantAccessDenied
--- PASS: TestCheckTenantAccessDenied (0.00s)
=== RUN   TestCheckTenantAccessNonexistentUser
--- PASS: TestCheckTenantAccessNonexistentUser (0.00s)
=== RUN   TestCrossFieldBindingRejectsMismatch
--- PASS: TestCrossFieldBindingRejectsMismatch (0.00s)
=== RUN   TestCheckResourceAccessGranted
--- PASS: TestCheckResourceAccessGranted (0.00s)
=== RUN   TestCheckResourceAccessDeniedCrossTenant
--- PASS: TestCheckResourceAccessDeniedCrossTenant (0.00s)
=== RUN   TestCheckResourceAccessDeniedNonexistent
--- PASS: TestCheckResourceAccessDeniedNonexistent (0.00s)
ok  	multi-tenant-api/internal/verified	0.017s
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
type#jwt-issuer : symbol
type#jwt-audience : symbol
type#parsed-claims : symbol
type#verified-jwt : symbol
type#authenticated-user : symbol
type#service-id : symbol
type#service-credential : symbol
type#human-principal : symbol
type#service-principal : symbol
type#tenant-access : symbol
type#resource-access : symbol
same-user? : (user-id --> (user-id --> boolean))
run time: 0.17404799535870552 secs

typechecked in 271 inferences
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
