# Discharge Report — Audit Rendering

Generated 2026-09-22T04:53:15Z. Source artifact: `transcript/discharge_report.json` (schema_version=1).

**Implementation commit:** `db919d64e3aa7d9c1afb8993149390868602eeef` (working tree dirty)

**Spec files:**

- `specs/core.shen` (sha256 `e58f2ba7e92e94b8df7f234f6fd346faa8d05b0f175b09ea44c757690e676f56`)

**Target languages:** go

## Tool Versions

| Tool | Version |
|---|---|
| sb | 0.3.0 |
| shen-derive | 0.3.0 |
| shengen | shengen 0.3.0 |
| shen runtime | not detected |

## Toolchain

| Component | Version |
|---|---|
| Go | `go1.24.7` |
| platform | `linux/amd64` |
| shengen | `shengen 0.3.0` |
| shengen sha256 | `4a7d38acf282002e50cc85431a73e838edb0e659514e6a26943b85858b106088` |
| scip-go | `0.2.7` |
| scip-typescript | `0.4.0` |

The shengen hash matters because the guard types are a pure function of the spec bytes and that binary. Re-run the same emitter on the same spec and you get the same guards file, byte for byte, from any directory.

## Signature

**Unsigned.** The `signature` field is null. Nobody has attested that this document came out of the pipeline it describes. The claims can still be re-derived (see below) — a signature says who produced a report, not whether it is true.

## Summary

- **Rules:** 17 total — 17 discharged, 0 violated, 0 unproven
- **Premises:** 36 total — 35 static, 1 runtime-sampled, 0 unproven
- **Weakest evidence anywhere in this report:** sampled

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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `authenticated-user.field-jwt` | `Jwt : verified-jwt` | **static** | static | guard-brand-bound | shengen's brand inference gives AuthenticatedUser[B] a phantom brand parameter shared with this premise, so `NewAuthenticatedUser` accepts only evidence minted for the same subject. The premise `Jwt : verified-jwt` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule authenticated-user.) |
| `authenticated-user.field-user` | `User : user-id` | **static** | static | guard-type-at-boundary | User is typed user-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of user-id's premises transitively. |
| `authenticated-user.verified-user-head-head-jwt` | `(= User (head (head Jwt))) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for authenticated-user rejects inputs that do not satisfy (= User (head (head Jwt))), so this premise holds for any value of type authenticated-user reachable in the impl. |

- `authenticated-user.field-jwt`: proof binding — the constructor's signature is `AuthenticatedUser[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `authenticated-user.field-jwt` code references: `internal/shenguard/guards_gen.go:190`, `internal/shenguard/guards_gen.go:NewAuthenticatedUser`
- `authenticated-user.field-user` code references: `internal/shenguard/guards_gen.go:190`
- `authenticated-user.verified-user-head-head-jwt` code references: `internal/shenguard/guards_gen.go:190`

### `human-principal` — wrapper (✅ Discharged)

A human-principal value is a authenticated-user with no further runtime constraints; the type exists to keep raw authenticated-users from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype human-principal
  Auth : authenticated-user;
  ====================
  Auth : authenticated-principal;)
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `human-principal.field-auth` | `Auth : authenticated-user` | **static** | static | guard-brand-bound | shengen's brand inference gives HumanPrincipal[B] a phantom brand parameter shared with this premise, so `NewHumanPrincipal` accepts only evidence minted for the same subject. The premise `Auth : authenticated-user` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule human-principal.) |

- `human-principal.field-auth`: proof binding — the constructor's signature is `HumanPrincipal[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `human-principal.field-auth` code references: `internal/shenguard/guards_gen.go:255`, `internal/shenguard/guards_gen.go:NewHumanPrincipal`

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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `jwt-audience.field-x` | `X : string` | **static** | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |
| `jwt-audience.verified-not-x` | `(not (= X "")) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for jwt-audience rejects inputs that do not satisfy (not (= X "")), so this premise holds for any value of type jwt-audience reachable in the impl. |

- `jwt-audience.field-x` code references: `internal/shenguard/guards_gen.go:113`
- `jwt-audience.verified-not-x` code references: `internal/shenguard/guards_gen.go:113`

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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `jwt-issuer.field-x` | `X : string` | **static** | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |
| `jwt-issuer.verified-not-x` | `(not (= X "")) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for jwt-issuer rejects inputs that do not satisfy (not (= X "")), so this premise holds for any value of type jwt-issuer reachable in the impl. |

- `jwt-issuer.field-x` code references: `internal/shenguard/guards_gen.go:96`
- `jwt-issuer.verified-not-x` code references: `internal/shenguard/guards_gen.go:96`

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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `parsed-claims.field-sub` | `Sub : user-id` | **static** | static | guard-type-at-boundary | Sub is typed user-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of user-id's premises transitively. |
| `parsed-claims.field-exp` | `Exp : number` | **static** | static | guard-type-at-boundary | Exp is typed number; the target language's type system rejects non-number values at construction. |
| `parsed-claims.field-iss` | `Iss : jwt-issuer` | **static** | static | guard-type-at-boundary | Iss is typed jwt-issuer; values of that type can only be constructed via shengen's guarded constructor, which enforces all of jwt-issuer's premises transitively. |
| `parsed-claims.field-aud` | `Aud : jwt-audience` | **static** | static | guard-type-at-boundary | Aud is typed jwt-audience; values of that type can only be constructed via shengen's guarded constructor, which enforces all of jwt-audience's premises transitively. |
| `parsed-claims.verified-exp-0` | `(> Exp 0) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for parsed-claims rejects inputs that do not satisfy (> Exp 0), so this premise holds for any value of type parsed-claims reachable in the impl. |

- `parsed-claims.field-sub` code references: `internal/shenguard/guards_gen.go:130`
- `parsed-claims.field-exp` code references: `internal/shenguard/guards_gen.go:130`
- `parsed-claims.field-iss` code references: `internal/shenguard/guards_gen.go:130`
- `parsed-claims.field-aud` code references: `internal/shenguard/guards_gen.go:130`
- `parsed-claims.verified-exp-0` code references: `internal/shenguard/guards_gen.go:130`

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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `resource-access.field-access` | `Access : tenant-access` | **static** | static | guard-brand-bound | shengen's brand inference gives ResourceAccess[B] a phantom brand parameter shared with this premise, so `NewResourceAccess` accepts only evidence minted for the same subject. The premise `Access : tenant-access` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule resource-access.) |
| `resource-access.field-resource` | `Resource : resource-id` | **static** | static | guard-type-at-boundary | Resource is typed resource-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of resource-id's premises transitively. |
| `resource-access.field-isowned` | `IsOwned : boolean` | **static** | static | guard-type-at-boundary | IsOwned is typed boolean; the target language's type system rejects non-boolean values at construction. |
| `resource-access.verified-isowned-true` | `(= IsOwned true) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for resource-access rejects inputs that do not satisfy (= IsOwned true), so this premise holds for any value of type resource-access reachable in the impl. |

- `resource-access.field-access`: proof binding — the constructor's signature is `ResourceAccess[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `resource-access.field-access` code references: `internal/shenguard/guards_gen.go:324`, `internal/shenguard/guards_gen.go:NewResourceAccess`
- `resource-access.field-resource` code references: `internal/shenguard/guards_gen.go:324`
- `resource-access.field-isowned` code references: `internal/shenguard/guards_gen.go:324`
- `resource-access.verified-isowned-true` code references: `internal/shenguard/guards_gen.go:324`

### `resource-access-discipline` — flow (✅ Discharged)

Flow discipline: only internal/verified/CheckResourceAccess and cmd/cedar-verify/computeGuardAllow may reference the constructor internal/shenguard/NewResourceAccess. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(flow resource-access-discipline
  (constructor-only internal/shenguard/NewResourceAccess
                    internal/verified/CheckResourceAccess
                    cmd/cedar-verify/computeGuardAllow))
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `constructor-only:internal/shenguard/NewResourceAccess` | `(constructor-only internal/shenguard/NewResourceAccess                     internal/verified/CheckResourceAccess                     cmd/cedar-verify/computeGuardAllow)` | **static** | static | flow-analysis | Resolved references to internal/shenguard/NewResourceAccess, 2 references in all, are confined to internal/verified/CheckResourceAccess, cmd/cedar-verify/computeGuardAllow. Engine: go-datalog; index: scip-go. |


### `resource-id` — wrapper (✅ Discharged)

A resource-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype resource-id
  X : string;
  ====================
  X : resource-id;)
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `resource-id.field-x` | `X : string` | **static** | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `resource-id.field-x` code references: `internal/shenguard/guards_gen.go:82`

### `same-user?` — define (✅ Discharged)

A pure function same-user? : user-id --> user-id --> boolean. The Shen spec is the oracle; the impl is asserted to match it on every sampled input. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(define same-user?
  {user-id --> user-id --> boolean}
  A B -> ...)
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `same-user.oracle-spec-equiv` | `spec(same-user?) ≡ impl(SameUser) on sampled inputs` | **sampled** | runtime-sample | shen-derive-sampled | shen-derive evaluated the spec on 9 sampled cases (deterministic-default) and emitted a Go test asserting impl returns the same value on each. |


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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `service-credential.field-service` | `Service : service-id` | **static** | static | guard-type-at-boundary | Service is typed service-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of service-id's premises transitively. |
| `service-credential.field-secret` | `Secret : string` | **static** | static | guard-type-at-boundary | Secret is typed string; the target language's type system rejects non-string values at construction. |
| `service-credential.verified-not-secret` | `(not (= Secret "")) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for service-credential rejects inputs that do not satisfy (not (= Secret "")), so this premise holds for any value of type service-credential reachable in the impl. |

- `service-credential.field-service` code references: `internal/shenguard/guards_gen.go:230`
- `service-credential.field-secret` code references: `internal/shenguard/guards_gen.go:230`
- `service-credential.verified-not-secret` code references: `internal/shenguard/guards_gen.go:230`

### `service-id` — wrapper (✅ Discharged)

A service-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype service-id
  X : string;
  ====================
  X : service-id;)
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `service-id.field-x` | `X : string` | **static** | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `service-id.field-x` code references: `internal/shenguard/guards_gen.go:216`

### `service-principal` — wrapper (✅ Discharged)

A service-principal value is a service-credential with no further runtime constraints; the type exists to keep raw service-credentials from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype service-principal
  Cred : service-credential;
  ====================
  Cred : authenticated-principal;)
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `service-principal.field-cred` | `Cred : service-credential` | **static** | static | guard-brand-bound | shengen's brand inference gives ServicePrincipal[B] a phantom brand parameter shared with this premise, so `NewServicePrincipal` accepts only evidence minted for the same subject. The premise `Cred : service-credential` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule service-principal.) |

- `service-principal.field-cred`: proof binding — the constructor's signature is `ServicePrincipal[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `service-principal.field-cred` code references: `internal/shenguard/guards_gen.go:275`, `internal/shenguard/guards_gen.go:NewServicePrincipal`

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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `tenant-access.field-principal` | `Principal : authenticated-principal` | **static** | static | guard-brand-bound | shengen's brand inference gives TenantAccess[B] a phantom brand parameter shared with this premise, so `NewTenantAccess` accepts only evidence minted for the same subject. The premise `Principal : authenticated-principal` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule tenant-access.) |
| `tenant-access.field-tenant` | `Tenant : tenant-id` | **static** | static | guard-type-at-boundary | Tenant is typed tenant-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of tenant-id's premises transitively. |
| `tenant-access.field-ismember` | `IsMember : boolean` | **static** | static | guard-type-at-boundary | IsMember is typed boolean; the target language's type system rejects non-boolean values at construction. |
| `tenant-access.verified-ismember-true` | `(= IsMember true) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for tenant-access rejects inputs that do not satisfy (= IsMember true), so this premise holds for any value of type tenant-access reachable in the impl. |

- `tenant-access.field-principal`: proof binding — the constructor's signature is `TenantAccess[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `tenant-access.field-principal` code references: `internal/shenguard/guards_gen.go:295`, `internal/shenguard/guards_gen.go:NewTenantAccess`
- `tenant-access.field-tenant` code references: `internal/shenguard/guards_gen.go:295`
- `tenant-access.field-ismember` code references: `internal/shenguard/guards_gen.go:295`
- `tenant-access.verified-ismember-true` code references: `internal/shenguard/guards_gen.go:295`

### `tenant-access-discipline` — flow (✅ Discharged)

Flow discipline: only internal/verified/CheckTenantAccess and cmd/cedar-verify/computeGuardAllow may reference the constructor internal/shenguard/NewTenantAccess and every call path from *ListResources* to DB#Query* references internal/verified/CheckTenantAccess first. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(flow tenant-access-discipline
  (constructor-only internal/shenguard/NewTenantAccess
                    internal/verified/CheckTenantAccess
                    cmd/cedar-verify/computeGuardAllow)
  (must-pass-through *ListResources*
                     internal/verified/CheckTenantAccess
                     DB#Query*))
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `constructor-only:internal/shenguard/NewTenantAccess` | `(constructor-only internal/shenguard/NewTenantAccess                     internal/verified/CheckTenantAccess                     cmd/cedar-verify/computeGuardAllow)` | **static** | static | flow-analysis | Resolved references to internal/shenguard/NewTenantAccess, 3 references in all, are confined to internal/verified/CheckTenantAccess, cmd/cedar-verify/computeGuardAllow. Engine: go-datalog; index: scip-go. |
| `must-pass-through:*ListResources*→DB#Query*` | `(must-pass-through *ListResources*                      internal/verified/CheckTenantAccess                      DB#Query*)` | **static** | static | flow-analysis | Every call path from 1 definition matching *ListResources* to a call of DB#Query* passes through a reference to internal/verified/CheckTenantAccess. Engine: go-datalog; index: scip-go. |


### `tenant-id` — wrapper (✅ Discharged)

A tenant-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype tenant-id
  X : string;
  ====================
  X : tenant-id;)
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `tenant-id.field-x` | `X : string` | **static** | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `tenant-id.field-x` code references: `internal/shenguard/guards_gen.go:68`

### `user-id` — wrapper (✅ Discharged)

A user-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype user-id
  X : string;
  ====================
  X : user-id;)
```

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `user-id.field-x` | `X : string` | **static** | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `user-id.field-x` code references: `internal/shenguard/guards_gen.go:54`

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

Continuously discharged since commit `65a24daad14dd78f13886c4a6f376aeef1febc25`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `verified-jwt.field-claims` | `Claims : parsed-claims` | **static** | static | guard-brand-bound | shengen's brand inference gives VerifiedJwt[B] a phantom brand parameter shared with this premise, so `NewVerifiedJwt` accepts only evidence minted for the same subject. The premise `Claims : parsed-claims` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule verified-jwt.) |
| `verified-jwt.field-sig` | `Sig : string` | **static** | static | guard-type-at-boundary | Sig is typed string; the target language's type system rejects non-string values at construction. |
| `verified-jwt.verified-not-sig` | `(not (= Sig "")) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for verified-jwt rejects inputs that do not satisfy (not (= Sig "")), so this premise holds for any value of type verified-jwt reachable in the impl. |

- `verified-jwt.field-claims`: proof binding — the constructor's signature is `VerifiedJwt[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `verified-jwt.field-claims` code references: `internal/shenguard/guards_gen.go:165`, `internal/shenguard/guards_gen.go:NewVerifiedJwt`
- `verified-jwt.field-sig` code references: `internal/shenguard/guards_gen.go:165`
- `verified-jwt.verified-not-sig` code references: `internal/shenguard/guards_gen.go:165`


## How to Verify This Report

Run this in the project directory. It needs no model, no network, and no credentials — only the committed artifacts:

```sh
sb verify-report --in transcript/discharge_report.json
```

It re-hashes every spec, re-runs shengen and diffs the result against the committed guards file, resolves every code reference, re-runs the committed sample tests, re-checks the path counters when z3 is present, and re-evaluates the flow premises from a freshly built index. A check it cannot re-derive is reported UNVERIFIED rather than passed — add `--strict` to treat that as a failure. When a check fails, it names the premises that lost their basis.

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

- **Path cover** — when the premise's basis is
  `prover-z3-path-cover`, the evidence is stronger than a pool.
  shen-derive symbolically executed the Shen spec, enumerated every
  execution path (unrolling list recursion to a fixed depth), and used
  the Z3 solver to produce one concrete input per *feasible* path.
  Those inputs are committed as test cases alongside the boundary
  pool. Paths whose condition is unsatisfiable are reported as dead:
  branches of the spec no input can reach. This is still bounded
  evidence — the list-unrolling depth is finite — but within that
  bound no path of the spec goes unexercised.

- **Vacuous** — the rule's datatype is uninhabited: the conjunction of
  its verified premises has no solution, so no value of the type can
  be constructed and every claim that consumes one is empty. This is
  a defect in the spec rather than in the implementation, and it
  fails the gate, because an uninhabited guard proves nothing while
  looking like it proves everything.

- **Precision** — each premise also carries a `precision` on a
  total order: `static` (the compiler refuses a violating
  program) is strongest, then `path-cover` (a solver found a
  witness for every feasible spec path), then `sampled`
  (agreement on a pool of inputs), then `runtime` (checked in
  production, on the value in hand, and silent about every value the
  program never sees), then `unproven`. A report is only as
  strong as its weakest premise, which is why the Summary states it.

- **Blame** — each counter-example names one responsible party:
  `spec` (the Shen rule is wrong or uninhabited), `impl`
  (the implementation disagrees with a spec both oracles read the same
  way), `wrapper` (a `:runtime-via` checker), or
  `lowering` (the spec's two evaluators disagree about what it
  means). The `blame_basis` says how the assignment was
  reached; `evaluator-only` means no Shen host was available to
  offer a second reading, so a lowering bug would look identical.

- **Unproven** — the tool could not confidently classify the premise
  in this release. Treat the premise as outside the verified
  boundary until a future version of the tool can address it.

**What this report does not claim**

- It is not a SOC-2, ISO-27001, or any other compliance certification.
  It is a verification artifact that compliance and audit workflows
  may reference as evidence.
- It is not third-party attested. The `signature` field, when
  present, says which key vouched that this document came out of this
  pipeline. It is not a claim that the pipeline's conclusions are
  correct — for that, re-derive them with `sb verify-report`,
  which needs neither the key nor the network. See the Signature
  section above, and docs/TRUST-MODEL.md for what signing does and
  does not move inside the trust boundary.
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
