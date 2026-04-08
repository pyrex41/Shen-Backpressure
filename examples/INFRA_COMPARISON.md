# Two Approaches to Deductive Infrastructure

Two demos, two value propositions, same underlying engine (Shen sequent-calculus proofs + generated guard types).

| | **ShenGuard (Bolt-On)** | **ShenPlane (Replacement)** |
|---|---|---|
| **Directory** | [`demo/shenguard-bolt-on/`](./shenguard-bolt-on/) | [`demo/shenplane/`](./shenplane/) |
| **Premise** | Keep Argo + Crossplane, add safety | Replace the composition & validation layers |
| **Effort** | 5 min (CI gate) to 4 hours (Composition Function) | Greenfield build or gradual migration |
| **Types** | 79 (validates existing YAML) | 48 (defines the entire control plane) |
| **User experience** | Unchanged — same YAML, same kubectl, same Argo | Unchanged — same YAML, same kubectl |
| **What users see** | Errors in CI / PreSync / admission | Errors at `kubectl apply` time |
| **Where proofs live** | Scanner reads YAML → calls guard constructors | Controller is built from guard types |
| **Composition model** | Crossplane's JSONPath patches (validated) | Typed expansion with proof objects |
| **Migration** | None | Build new controller or migrate Compositions |

## When To Use Which

### Use ShenGuard (Bolt-On) When:

- You already run Argo + Crossplane in production
- You want to add safety incrementally, starting today
- Your team isn't ready to build custom controllers
- You need to protect existing Compositions from patch typos, credential expiry, wave ordering bugs
- You want to add a CI gate in 5 minutes and expand from there

### Use ShenPlane (Replacement) When:

- You're starting fresh (greenfield infrastructure)
- You've outgrown Crossplane's patch-and-pray model
- You want composition to be typed expansion, not JSONPath patches
- You're building a platform for autonomous/AI-driven infra workflows
- You need structural guarantees, not just validation

### Use Both When:

- Start with ShenGuard to protect what's running today
- Build ShenPlane for new resource types
- Gradually migrate critical paths from Crossplane → ShenPlane
- Keep ShenGuard scanning the Crossplane resources that haven't migrated yet

## Architecture Comparison

### ShenGuard: Verification at the Boundaries

```
┌──────────────────────────────────────────┐
│  Argo CD + Crossplane (untouched)        │
│                                          │
│  ┌──────────┐  ┌──────────────────────┐  │
│  │ ArgoCD   │  │ Crossplane           │  │
│  │ sync     │  │ reconcile            │  │
│  └────▲─────┘  └────▲─────────────────┘  │
│       │              │                    │
│  ═════╪══════════════╪══════════════      │
│       │  SHENGUARD   │                    │
│       │  (bolt-on)   │                    │
│  ┌────┴──┐  ┌───────┴────┐  ┌────────┐  │
│  │Scanner│  │Comp.Function│  │Webhook │  │
│  │(reads │  │(imports     │  │(rejects│  │
│  │ YAML) │  │ shenguard)  │  │ bad    │  │
│  └───────┘  └────────────┘  │ Claims)│  │
│                              └────────┘  │
│       ▲              ▲            ▲      │
│       └──────────────┴────────────┘      │
│                   │                      │
│          specs/core.shen (79 types)      │
└──────────────────────────────────────────┘
```

### ShenPlane: Proofs ARE the Control Plane

```
┌──────────────────────────────────────────┐
│  ShenPlane                               │
│                                          │
│  ┌──────────────────────────────────┐    │
│  │  User writes YAML Claim          │    │
│  │  (same ergonomics as Crossplane) │    │
│  └────────────┬─────────────────────┘    │
│               │                          │
│  ┌────────────▼─────────────────────┐    │
│  │  Admission Webhook               │    │
│  │  (guard constructors validate)   │    │
│  └────────────┬─────────────────────┘    │
│               │                          │
│  ┌────────────▼─────────────────────┐    │
│  │  Controller                      │    │
│  │  (typed expansion, not patches)  │    │
│  │                                  │    │
│  │  SecureDb = DbClaim              │    │
│  │           + SecurityProof        │    │
│  │           (enc ∧ net ∧ iam)      │    │
│  │                                  │    │
│  │  DbExpansion = SecureDb          │    │
│  │              + Instance          │    │
│  │              + SubnetGroup       │    │
│  │              + SecurityGroup     │    │
│  │              + IAMRole           │    │
│  │              + SameRegion proof  │    │
│  │              + InNetwork proof   │    │
│  └────────────┬─────────────────────┘    │
│               │                          │
│  ┌────────────▼─────────────────────┐    │
│  │  Cloud resources provisioned     │    │
│  │  (all proofs satisfied)          │    │
│  └──────────────────────────────────┘    │
│                                          │
│          specs/core.shen (48 types)      │
└──────────────────────────────────────────┘
```

## The Same Error, Two Ways

A database missing encryption:

### ShenGuard (Bolt-On)

The scanner reads your Crossplane Composition, checks if the RDS instance has `storageEncrypted: true`, and reports:

```
$ shen-k8s-scan . --strict

Crossplane (3 Compositions)
  ✗ encryption-enabled  FAIL: compositions/database.yaml
                        RDS Instance missing encryption-at-rest
                        Fix: add spec.forProvider.storageEncrypted: true

Results: 11 passed, 1 failed
```

You fix the Composition YAML. Crossplane is unchanged.

### ShenPlane (Replacement)

The controller tries to construct a `SecureDb` proof:

```
NewSecureDb(dbClaim, securityProof)
                     ↑
              NewSecurityProof(encryptionProof, networkProof, iamProof)
                                ↑
                         encryptionProof is nil → cannot construct

Error: DatabaseClaim "prod-db" rejected:
  encryption-proof required but not satisfied
  (every database must have encryption-at-rest)
```

The Claim is rejected at admission. No Composition to fix — the controller won't provision without the proof.

## Shared Foundation

Both approaches use the same Shen backpressure engine:

1. **Shen specs** define invariants as sequent-calculus datatypes
2. **shengen** generates opaque Go types with private fields
3. **Guard constructors** are the only way to create these types
4. **Proofs compose** — `security-proof` requires `encryption ∧ network ∧ iam`
5. **Invalid states are structurally impossible**, not just rejected

The difference is where the guards live:
- **ShenGuard**: Guards sit at boundaries, scanning and validating
- **ShenPlane**: Guards ARE the controller — the system is built from proofs
