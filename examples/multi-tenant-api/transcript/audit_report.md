# Discharge Report — Audit Rendering

Generated 2026-05-23T04:35:14Z. Source artifact: `.sb/discharge_report.json` (schema_version=1).

**Implementation commit:** `d9194a75ecc6c7d1d19355c371da26278610d21e` (working tree dirty)

**Spec files:**

- `specs/core.shen` (sha256 `5cd6e689dff7fa71d761537dcc8eaafa8243bd7240dd07cff1d5883ac1164141`)

**Target languages:** go

## Tool Versions

| Tool | Version |
|---|---|
| sb | 0.3.0 |
| shen-derive | 0.3.0 |
| shengen | — |
| shen runtime | not detected |

## Summary

- **Rules:** 15 total — 15 discharged, 0 violated, 0 unproven
- **Premises:** 33 total — 32 static, 1 runtime-sampled, 0 unproven

## Rules

### `authenticated-user` — guarded (✅ Discharged)

A authenticated-user is a multi-field structure whose constructor enforces 1 cross-field invariant(s). *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype authenticated-user
  Jwt : verified-jwt;
  User : user-id;
  (= User (head (head Jwt))) : verified;
  ====================
  [Jwt User] : authenticated-user;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `authenticated-user.field-jwt` | `Jwt : verified-jwt` | static | guard-type-at-boundary | Jwt is typed verified-jwt; values of that type can only be constructed via shengen's guarded constructor, which enforces all of verified-jwt's premises transitively. |
| `authenticated-user.field-user` | `User : user-id` | static | guard-type-at-boundary | User is typed user-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of user-id's premises transitively. |
| `authenticated-user.verified-user-head-head-jwt` | `(= User (head (head Jwt))) : verified` | static | guard-constructor-validates | shengen's generated constructor for authenticated-user rejects inputs that do not satisfy (= User (head (head Jwt))), so this premise holds for any value of type authenticated-user reachable in the impl. |

- `authenticated-user.field-jwt` code references: `internal/shenguard/guards_gen.go:135`
- `authenticated-user.field-user` code references: `internal/shenguard/guards_gen.go:135`
- `authenticated-user.verified-user-head-head-jwt` code references: `internal/shenguard/guards_gen.go:135`

### `human-principal` — wrapper (✅ Discharged)

A human-principal value is a authenticated-user with no further runtime constraints; the type exists to keep raw authenticated-users from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype human-principal
  Auth : authenticated-user;
  ====================
  Auth : authenticated-principal;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `human-principal.field-auth` | `Auth : authenticated-user` | static | guard-type-at-boundary | Auth is typed authenticated-user; values of that type can only be constructed via shengen's guarded constructor, which enforces all of authenticated-user's premises transitively. |

- `human-principal.field-auth` code references: `internal/shenguard/guards_gen.go:190`

### `jwt-audience` — constrained (✅ Discharged)

A jwt-audience value is a string that satisfies 1 additional constraint(s) checked at construction. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype jwt-audience
  X : string;
  (not (= X "")) : verified;
  ====================
  X : jwt-audience;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `jwt-audience.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |
| `jwt-audience.verified-not-x` | `(not (= X "")) : verified` | static | guard-constructor-validates | shengen's generated constructor for jwt-audience rejects inputs that do not satisfy (not (= X "")), so this premise holds for any value of type jwt-audience reachable in the impl. |

- `jwt-audience.field-x` code references: `internal/shenguard/guards_gen.go:69`
- `jwt-audience.verified-not-x` code references: `internal/shenguard/guards_gen.go:69`

### `jwt-issuer` — constrained (✅ Discharged)

A jwt-issuer value is a string that satisfies 1 additional constraint(s) checked at construction. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype jwt-issuer
  X : string;
  (not (= X "")) : verified;
  ====================
  X : jwt-issuer;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `jwt-issuer.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |
| `jwt-issuer.verified-not-x` | `(not (= X "")) : verified` | static | guard-constructor-validates | shengen's generated constructor for jwt-issuer rejects inputs that do not satisfy (not (= X "")), so this premise holds for any value of type jwt-issuer reachable in the impl. |

- `jwt-issuer.field-x` code references: `internal/shenguard/guards_gen.go:55`
- `jwt-issuer.verified-not-x` code references: `internal/shenguard/guards_gen.go:55`

### `parsed-claims` — guarded (✅ Discharged)

A parsed-claims is a multi-field structure whose constructor enforces 1 cross-field invariant(s). *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype parsed-claims
  Sub : user-id;
  Exp : number;
  Iss : jwt-issuer;
  Aud : jwt-audience;
  (> Exp 0) : verified;
  ====================
  [Sub Exp Iss Aud] : parsed-claims;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `parsed-claims.field-sub` | `Sub : user-id` | static | guard-type-at-boundary | Sub is typed user-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of user-id's premises transitively. |
| `parsed-claims.field-exp` | `Exp : number` | static | guard-type-at-boundary | Exp is typed number; the target language's type system rejects non-number values at construction. |
| `parsed-claims.field-iss` | `Iss : jwt-issuer` | static | guard-type-at-boundary | Iss is typed jwt-issuer; values of that type can only be constructed via shengen's guarded constructor, which enforces all of jwt-issuer's premises transitively. |
| `parsed-claims.field-aud` | `Aud : jwt-audience` | static | guard-type-at-boundary | Aud is typed jwt-audience; values of that type can only be constructed via shengen's guarded constructor, which enforces all of jwt-audience's premises transitively. |
| `parsed-claims.verified-exp-0` | `(> Exp 0) : verified` | static | guard-constructor-validates | shengen's generated constructor for parsed-claims rejects inputs that do not satisfy (> Exp 0), so this premise holds for any value of type parsed-claims reachable in the impl. |

- `parsed-claims.field-sub` code references: `internal/shenguard/guards_gen.go:83`
- `parsed-claims.field-exp` code references: `internal/shenguard/guards_gen.go:83`
- `parsed-claims.field-iss` code references: `internal/shenguard/guards_gen.go:83`
- `parsed-claims.field-aud` code references: `internal/shenguard/guards_gen.go:83`
- `parsed-claims.verified-exp-0` code references: `internal/shenguard/guards_gen.go:83`

### `resource-access` — guarded (✅ Discharged)

A resource-access is a multi-field structure whose constructor enforces 1 cross-field invariant(s). *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype resource-access
  Access : tenant-access;
  Resource : resource-id;
  IsOwned : boolean;
  (= IsOwned true) : verified;
  ====================
  [Access Resource IsOwned] : resource-access;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `resource-access.field-access` | `Access : tenant-access` | static | guard-type-at-boundary | Access is typed tenant-access; values of that type can only be constructed via shengen's guarded constructor, which enforces all of tenant-access's premises transitively. |
| `resource-access.field-resource` | `Resource : resource-id` | static | guard-type-at-boundary | Resource is typed resource-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of resource-id's premises transitively. |
| `resource-access.field-isowned` | `IsOwned : boolean` | static | guard-type-at-boundary | IsOwned is typed boolean; the target language's type system rejects non-boolean values at construction. |
| `resource-access.verified-isowned-true` | `(= IsOwned true) : verified` | static | guard-constructor-validates | shengen's generated constructor for resource-access rejects inputs that do not satisfy (= IsOwned true), so this premise holds for any value of type resource-access reachable in the impl. |

- `resource-access.field-access` code references: `internal/shenguard/guards_gen.go:250`
- `resource-access.field-resource` code references: `internal/shenguard/guards_gen.go:250`
- `resource-access.field-isowned` code references: `internal/shenguard/guards_gen.go:250`
- `resource-access.verified-isowned-true` code references: `internal/shenguard/guards_gen.go:250`

### `resource-id` — wrapper (✅ Discharged)

A resource-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype resource-id
  X : string;
  ====================
  X : resource-id;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `resource-id.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `resource-id.field-x` code references: `internal/shenguard/guards_gen.go:44`

### `same-user?` — define (✅ Discharged)

A pure function same-user? : user-id --> user-id --> boolean. The Shen spec is the oracle; the impl is asserted to match it on every sampled input. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(define same-user?
  {user-id --> user-id --> boolean}
  A B -> ...)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `same-user.oracle-spec-equiv` | `spec(same-user?) ≡ impl(SameUser) on sampled inputs` | runtime-sample | shen-derive-sampled | shen-derive evaluated the spec on 9 sampled cases (deterministic-default) and emitted a Go test asserting impl returns the same value on each. |


- `same-user.oracle-spec-equiv`: sampled 9 cases (seed: deterministic-default); 9 passed, 0 failed.

### `service-credential` — guarded (✅ Discharged)

A service-credential is a multi-field structure whose constructor enforces 1 cross-field invariant(s). *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype service-credential
  Service : service-id;
  Secret : string;
  (not (= Secret "")) : verified;
  ====================
  [Service Secret] : service-credential;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `service-credential.field-service` | `Service : service-id` | static | guard-type-at-boundary | Service is typed service-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of service-id's premises transitively. |
| `service-credential.field-secret` | `Secret : string` | static | guard-type-at-boundary | Secret is typed string; the target language's type system rejects non-string values at construction. |
| `service-credential.verified-not-secret` | `(not (= Secret "")) : verified` | static | guard-constructor-validates | shengen's generated constructor for service-credential rejects inputs that do not satisfy (not (= Secret "")), so this premise holds for any value of type service-credential reachable in the impl. |

- `service-credential.field-service` code references: `internal/shenguard/guards_gen.go:168`
- `service-credential.field-secret` code references: `internal/shenguard/guards_gen.go:168`
- `service-credential.verified-not-secret` code references: `internal/shenguard/guards_gen.go:168`

### `service-id` — wrapper (✅ Discharged)

A service-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype service-id
  X : string;
  ====================
  X : service-id;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `service-id.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `service-id.field-x` code references: `internal/shenguard/guards_gen.go:157`

### `service-principal` — wrapper (✅ Discharged)

A service-principal value is a service-credential with no further runtime constraints; the type exists to keep raw service-credentials from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype service-principal
  Cred : service-credential;
  ====================
  Cred : authenticated-principal;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `service-principal.field-cred` | `Cred : service-credential` | static | guard-type-at-boundary | Cred is typed service-credential; values of that type can only be constructed via shengen's guarded constructor, which enforces all of service-credential's premises transitively. |

- `service-principal.field-cred` code references: `internal/shenguard/guards_gen.go:207`

### `tenant-access` — guarded (✅ Discharged)

A tenant-access is a multi-field structure whose constructor enforces 1 cross-field invariant(s). *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype tenant-access
  Principal : authenticated-principal;
  Tenant : tenant-id;
  IsMember : boolean;
  (= IsMember true) : verified;
  ====================
  [Principal Tenant IsMember] : tenant-access;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `tenant-access.field-principal` | `Principal : authenticated-principal` | static | guard-type-at-boundary | Principal is typed authenticated-principal; values of that type can only be constructed via shengen's guarded constructor, which enforces all of authenticated-principal's premises transitively. |
| `tenant-access.field-tenant` | `Tenant : tenant-id` | static | guard-type-at-boundary | Tenant is typed tenant-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of tenant-id's premises transitively. |
| `tenant-access.field-ismember` | `IsMember : boolean` | static | guard-type-at-boundary | IsMember is typed boolean; the target language's type system rejects non-boolean values at construction. |
| `tenant-access.verified-ismember-true` | `(= IsMember true) : verified` | static | guard-constructor-validates | shengen's generated constructor for tenant-access rejects inputs that do not satisfy (= IsMember true), so this premise holds for any value of type tenant-access reachable in the impl. |

- `tenant-access.field-principal` code references: `internal/shenguard/guards_gen.go:224`
- `tenant-access.field-tenant` code references: `internal/shenguard/guards_gen.go:224`
- `tenant-access.field-ismember` code references: `internal/shenguard/guards_gen.go:224`
- `tenant-access.verified-ismember-true` code references: `internal/shenguard/guards_gen.go:224`

### `tenant-id` — wrapper (✅ Discharged)

A tenant-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype tenant-id
  X : string;
  ====================
  X : tenant-id;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `tenant-id.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `tenant-id.field-x` code references: `internal/shenguard/guards_gen.go:33`

### `user-id` — wrapper (✅ Discharged)

A user-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype user-id
  X : string;
  ====================
  X : user-id;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `user-id.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `user-id.field-x` code references: `internal/shenguard/guards_gen.go:22`

### `verified-jwt` — guarded (✅ Discharged)

A verified-jwt is a multi-field structure whose constructor enforces 1 cross-field invariant(s). *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype verified-jwt
  Claims : parsed-claims;
  Sig : string;
  (not (= Sig "")) : verified;
  ====================
  [Claims Sig] : verified-jwt;)
```

Continuously discharged since commit `d9194a75ecc6c7d1d19355c371da26278610d21e`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `verified-jwt.field-claims` | `Claims : parsed-claims` | static | guard-type-at-boundary | Claims is typed parsed-claims; values of that type can only be constructed via shengen's guarded constructor, which enforces all of parsed-claims's premises transitively. |
| `verified-jwt.field-sig` | `Sig : string` | static | guard-type-at-boundary | Sig is typed string; the target language's type system rejects non-string values at construction. |
| `verified-jwt.verified-not-sig` | `(not (= Sig "")) : verified` | static | guard-constructor-validates | shengen's generated constructor for verified-jwt rejects inputs that do not satisfy (not (= Sig "")), so this premise holds for any value of type verified-jwt reachable in the impl. |

- `verified-jwt.field-claims` code references: `internal/shenguard/guards_gen.go:113`
- `verified-jwt.field-sig` code references: `internal/shenguard/guards_gen.go:113`
- `verified-jwt.verified-not-sig` code references: `internal/shenguard/guards_gen.go:113`


## How to Read This Report

This report categorises every premise of every Shen rule by **how**
it was discharged in the implementation under verification.

- **Static** — the target language's type system (Go's static
  typing, applied to shengen's generated guard types) prevents the
  premise from being violated. A premise typed at the function
  boundary cannot be reached with a non-conforming value because the
  compiler refuses to build such a call site. `guard-type-at-boundary` and
  `guard-constructor-validates` are the two static bases this
  release emits.

- **Runtime-sampled** — shen-derive evaluates the Shen spec on a
  deterministic boundary pool (and, when seeded, additional random
  draws) and emits a Go test asserting that the implementation
  returns the same value on every sampled input. A "discharged"
  premise here means *every sampled case agreed*. This is sampled
  evidence, not an exhaustive proof.

- **Unproven** — the tool could not confidently classify the premise
  in this release. Treat the premise as outside the verified
  boundary until a future version of the tool can address it.

**What this report does not claim**

- It is not a SOC-2, ISO-27001, or any other compliance certification.
  It is a verification artifact that compliance and audit workflows
  may reference as evidence.
- It is not signed or attested. The `signature` field in the JSON is
  reserved for a future signing integration; in this release it is
  always null.
- It is not third-party verified. The classifications and rationales
  come from this tool's own analysis of the spec and the
  implementation.

**Reproducing this report**

The discharge report is produced as a side effect of every successful
`sb gates` (or `sb derive`) run. Run the gate pipeline against the
same spec and the same git commit recorded in this report and you
will get a byte-identical artifact (modulo the `generated_at`
timestamp). Time-stamped copies accumulate under `.sb/history/`.

For per-case input detail, open the generated test file referenced
in the spec's manifest and look for the matching `case_NN` entry.
