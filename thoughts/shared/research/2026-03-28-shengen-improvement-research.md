---
date: 2026-03-28T12:00:00-07:00
researcher: reuben
git_commit: bb2ce6668e47cc46fa6335d9545e0480f82f2d4e
branch: claude/add-web-tools-integration-eu9L4
repository: Shen-Backpressure
topic: "Shengen improvement research — phantom types, TCB reduction, ergonomics for real-world use"
tags: [research, shengen, guard-types, tcb, ergonomics, phantom-types, backpressure]
status: complete
last_updated: 2026-03-28
last_updated_by: reuben
---

# Research: Shengen Improvements — Phantom Types, TCB Reduction, Ergonomics

**Date**: 2026-03-28T12:00:00-07:00
**Researcher**: reuben
**Triggered by**: Scud heavy 16-agent review of blog post identified real limitations

## Research Question

Based on critical review feedback, what concrete improvements should be made to shengen and the Shen-Backpressure architecture?

## Summary

Three parallel research agents investigated phantom types, TCB reduction, and ergonomics. The findings converge on a clear priority list. Phantom types are NOT worth pursuing. TCB reduction has two high-value, low-effort wins. Ergonomics improvements require one key shengen enhancement (sum types) that unlocks service accounts, batch operations, and capability tokens.

---

## 1. Phantom Types — Verdict: Don't Do It

**Question**: Can we track WHICH specific tenant at the type level, not just "some verified tenant"?

**Answer**: No, and it's not worth the complexity.

The fundamental problem: tenant IDs are runtime values from HTTP headers/JWTs/databases. At the construction boundary, the type parameter would always collapse to `string`. Go's type parameters (1.18+) would create "viral generics" — every function touching `TenantAccess` needs `[T TenantTag]` annotations. TypeScript branded types work better but only when IDs are string literals known at the call site (tests, not production).

**The current design already prevents cross-tenant confusion.** `TenantAccess` stores the `TenantId` inside it. The membership check runs at construction time. The proof chain validates that the check happened.

**Alternative for the rare case of needing to compare two proofs**: A `MatchedTenantPair` guarded type within existing shengen:

```shen
(datatype matched-tenant-pair
  A : tenant-access;
  B : tenant-access;
  (= (head (tail A)) (head (tail B))) : verified;
  ================================================
  [A B] : matched-tenant-pair;)
```

This stays within the existing architecture — zero shengen changes needed.

---

## 2. TCB Reduction — Two High-Value Wins

### 2a. Gate 5: Checksum Audit (Priority 1 — Low Effort, High Value)

Re-run shengen and diff the output against the committed `guards_gen.go`. Catches:
- Manual edits to the shenguard package (forgery)
- Stale generated code (spec changed, types not regenerated)

Implementation: ~30-line bash script. Could also generate a `.shenguard-manifest.json` with spec hash and file checksums.

The strongest variant: regenerate and diff, which is more robust than checksums alone.

### 2b. TenantDB Scoped Query Builder (Priority 2 — Medium Effort, High Value)

shengen generates a `TenantDB` wrapper constructible only from a `TenantAccess` proof:

```go
type TenantDB struct {
    db       *sql.DB
    tenantID string // captured from TenantAccess at construction time
}

func NewTenantDB(db *sql.DB, access TenantAccess) TenantDB {
    return TenantDB{db: db, tenantID: access.Tenant().Val()}
}
```

Developer writes query methods on `TenantDB`. All queries automatically scope to the proven tenant. Eliminates the SQL correctness TCB item — no more hand-threading `WHERE tenant_id = ?`.

### 2c. Static Analysis for Middleware Coverage (Priority 3 — Medium Effort)

`go/analysis` pass that flags handlers registered without auth middleware that reference shenguard types. Would catch the admin handler gap in the current code. Feasible as pattern matching; full dataflow analysis for SQL taint is harder but a heuristic version works.

### 2d. Package Splitting — Not Worth It in Go

Go's package-level visibility means splitting types/constructors into sub-packages doesn't help (constructors can't set unexported fields from another package). Gate 5 achieves the same goal. Worth revisiting for Rust targets where module-level privacy is more granular.

---

## 3. Ergonomics — Sum Types Unlock Everything

### The Key shengen Gap: Sum Types

The single most impactful shengen improvement is supporting **multiple rules that produce the same conclusion type**. This is already detected by the collision pre-pass but the code generation templates have no strategy for it.

Concrete approach: when multiple blocks conclude the same type, generate a Go interface + concrete struct per constructor:

```go
type AuthenticatedPrincipal interface{ isPrincipal() }

type HumanPrincipal struct{ auth AuthenticatedUser }
func (HumanPrincipal) isPrincipal() {}
func NewHumanPrincipal(auth AuthenticatedUser) AuthenticatedPrincipal { ... }

type ServicePrincipal struct{ cred ServiceCredential }
func (ServicePrincipal) isPrincipal() {}
func NewServicePrincipal(cred ServiceCredential) AuthenticatedPrincipal { ... }
```

This unlocks:

### 3a. Service Accounts (Needs Sum Types)

Alternative proof root for background jobs/cron:

```shen
(datatype human-principal
  Auth : authenticated-user;
  ===========================
  Auth : authenticated-principal;)

(datatype service-principal
  Cred : service-credential;
  ============================
  Cred : authenticated-principal;)
```

`TenantAccess` takes `authenticated-principal` instead of `authenticated-user`. Background jobs construct `ServicePrincipal` from a service credential, humans construct `HumanPrincipal` from JWT flow.

### 3b. Batch Operations (Needs Sum Types + List Types)

`BatchTenantGrant` verified by single bulk query, then individual `TenantAccess` extracted without re-verifying:

```shen
(datatype batch-tenant-grant
  Principal : authenticated-principal;
  Tenants : tenant-set;
  AllMember : boolean;
  (= AllMember true) : verified;
  ================================
  [Principal Tenants AllMember] : batch-tenant-grant;)

(datatype tenant-access-from-batch
  Grant : batch-tenant-grant;
  Tenant : tenant-id;
  InSet : boolean;
  (= InSet true) : verified;
  ================================
  [Grant Tenant InSet] : tenant-access;)
```

### 3c. Capability Tokens / Cross-Service Serialization (Works Today)

No new shengen features needed — all standard GUARDED/COMPOSITE types:

```shen
(datatype capability-token
  Payload : string;
  Signature : string;
  (not (= Payload "")) : verified;
  (not (= Signature "")) : verified;
  ==================================
  [Payload Signature] : capability-token;)

(datatype verified-capability
  Token : capability-token;
  Key : signing-key;
  IsValid : boolean;
  (= IsValid true) : verified;
  ================================
  [Token Key IsValid] : verified-capability;)
```

Service A issues a signed capability token from a `TenantAccess`. Service B verifies signature, constructs `TenantAccess` from the verified capability. No JWT or DB needed on the receiving side.

### 3d. Richer Policies (Roles, Hierarchies, Time-Bounds)

All work within existing shengen patterns:

- **Roles**: `TenantRoleAccess` with role field, `WriteAccess` guarded on role check
- **Hierarchies**: Nested proof chain: `OrgAccess → TeamAccess → ProjectAccess`
- **Time-bounds**: `GrantWindow` with `(>= Now NotBefore)` and `(<= Now NotAfter)` verified premises
- **ABAC**: Compose individual attribute proofs into a composite `AbacDecision`

The one gap: `element?` with list literals (for role membership checks like "is this role in [admin, editor]?") currently falls through to a TODO in shengen.

---

## Key Architectural Insight

The proof-threading "boilerplate" is not boilerplate — it IS the proof. The real improvements are providing **multiple entry points** into the proof chain (service accounts, capability tokens, batch grants), not eliminating proof objects. Each improvement adds a new constructor for an existing conclusion type, which is exactly how sequent calculus works.

---

## shengen Gap Summary

| Gap | Severity | What It Unlocks |
|---|---|---|
| **Sum types** (multiple constructors → interface) | HIGH | Service accounts, batch ops, all alternative proof roots |
| **`element?` with list literals** | MEDIUM | Role-based access with enum constraints |
| **List types in constructor signatures** | MEDIUM | Batch operations with `tenant-set` |
| **Gate 5 checksum script** | HIGH (not shengen) | TCB integrity verification |
| **TenantDB scoped wrapper generation** | MEDIUM | Eliminates SQL scoping TCB |

## Recommended Implementation Priority

1. **Gate 5 checksum audit** — bash script, ~30 lines, immediate value
2. **Sum type code generation** — the single biggest shengen enhancement, unlocks 3 improvement categories
3. **TenantDB scoped query builder** — template addition to shengen
4. **`element?` list literal support** — extends the verified premise resolver
5. **Static analysis pass** — optional, Gate 5 covers the most critical case

## Related Research

- `thoughts/shared/research/2026-03-23-codebase-overview.md`
- `thoughts/shared/research/2026-03-24-shengen-codegen-bridge.md`
- `thoughts/shared/research/2026-03-28-blog-series-deterministic-backpressure.md`
