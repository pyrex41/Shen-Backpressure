# ShenPlane — Deductive Infrastructure Control Plane

YAML on the outside. Proofs on the inside.

Users write familiar Kubernetes-style YAML Claims. The control plane enforces invariants that are **provably correct by construction** — not checked after the fact by schemas, policies, or runtime reconciliation.

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│  WHAT USERS SEE                                             │
│                                                             │
│  apiVersion: shenplane.io/v1                                │
│  kind: DatabaseClaim             ◄── just YAML              │
│  spec:                               no Shen syntax         │
│    engine: postgres                  no Lisp                 │
│    size: large                       looks like Crossplane   │
│    storageGB: 500                    feels like kubectl      │
│    region: us-east-1                                        │
│    tags:                                                    │
│      team: payments                                         │
│      costCenter: eng-platform                               │
│      environment: production                                │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      │ kubectl apply / git push / Argo sync
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  WHAT HAPPENS UNDERNEATH                                    │
│                                                             │
│  ┌──────────────────┐    ┌──────────────────┐              │
│  │ Admission Webhook │    │ specs/core.shen  │              │
│  │ (generated Go)    │◄───│ (8 layers of     │              │
│  │                   │    │  proof types)    │              │
│  │ Parses YAML ──────┤    └────────┬─────────┘              │
│  │ Calls guard       │             │                        │
│  │   constructors ───┤    ┌────────┴─────────┐              │
│  │ ALL pass? ────────┤    │ shenguard/       │              │
│  │   ✓ admit         │    │ guards_gen.go    │              │
│  │   ✗ reject with   │    │ (generated types │              │
│  │     proof-derived │    │  with private    │              │
│  │     error message │    │  fields)         │              │
│  └──────────┬────────┘    └──────────────────┘              │
│             │                                               │
│             ▼                                               │
│  ┌──────────────────┐                                       │
│  │ Controller        │                                      │
│  │ (reconcile loop)  │                                      │
│  │                   │                                      │
│  │ Expand claim ─────┤─► RDS instance                       │
│  │   (typed, not     │─► subnet group                       │
│  │    patched)       │─► security group                     │
│  │                   │─► IAM role                           │
│  │ All resources ────┤                                      │
│  │   carry proofs:   │   security-proof                     │
│  │   encryption ✓    │   network-proof                      │
│  │   private net ✓   │   iam-proof                          │
│  │   tagged ✓        │   derived-from (provenance)          │
│  │   same-region ✓   │   same-region (consistency)          │
│  └───────────────────┘                                      │
└─────────────────────────────────────────────────────────────┘
```

## The 8 Layers

The Shen spec is organized as 8 composable layers:

| Layer | What It Defines | What It Prevents |
|-------|----------------|-----------------|
| **1. Identity & Lifecycle** | Resource handles, lifecycle states | Nameless resources, invalid states |
| **2. Claims** | User-facing YAML fields (regions, sizes, tags) | Unconstrained strings, missing tags, invalid regions |
| **3. Security Invariants** | Encryption, network isolation, IAM scope | Unencrypted resources, public endpoints, wildcard IAM |
| **4. Concrete Claim Types** | DatabaseClaim, CacheClaim, BucketClaim, etc. | Creating resources without meeting all constraints |
| **5. Composition** | How claims expand into cloud resources | Silent nil patches, cross-region inconsistency, orphaned resources |
| **6. Reconciliation** | Desired vs observed, drift detection, action planning | Unordered provisioning, missing provenance |
| **7. Credentials** | Provider config, expiry proofs | Expired creds, unhealthy providers |
| **8. End-to-End** | `deployment-safe` = plan + security + provider | Any deployment without full proof chain |

## What's Different From Crossplane

| Crossplane | ShenPlane |
|-----------|-----------|
| Compositions use JSONPath patches | Composition is typed expansion with proof objects |
| Schema validation via OpenAPI (weak) | Invariant enforcement via opaque guard types (strong) |
| Patch typo → silent nil → runtime failure | Patch typo → constructor rejects → admission denied |
| Policy enforcement via OPA/Gatekeeper (post-hoc) | Policy enforcement via proof composition (structural) |
| "Every DB should be encrypted" = a policy you hope is applied | "Every DB is encrypted" = you can't construct `secure-db` without `encryption-proof` |
| Provider creds expire → "Cannot Observe" cascade | `credential-expiry` proof fails → caught before reconciliation |
| XRD schema evolution breaks Claims silently | Spec evolution forces downstream adaptation (compiler errors) |

## What's Different From Argo CD

| Argo CD | ShenPlane |
|---------|-----------|
| Sync waves via annotations (fragile, implicit) | Reconcile ordering via typed proofs |
| `ignoreDifferences` for operator drift (manual allowlist) | Drift detection is structural (desired vs observed generation) |
| Health checks via Lua scripts | Health is a typed lifecycle state |
| Partial syncs from cross-wave dependencies | Ordering proven before reconciliation starts |

## What Users Actually Write

### DatabaseClaim
```yaml
apiVersion: shenplane.io/v1
kind: DatabaseClaim
metadata:
  name: prod-db
  namespace: payments
spec:
  engine: postgres       # {postgres, mysql, mariadb}
  size: large            # {small, medium, large, xlarge}
  storageGB: 500         # [10, 10000]
  region: us-east-1      # constrained to real regions
  tags:
    team: payments       # required
    costCenter: eng      # required
    environment: prod    # {development, staging, production}
```

### CacheClaim
```yaml
apiVersion: shenplane.io/v1
kind: CacheClaim
metadata:
  name: session-cache
  namespace: auth
spec:
  engine: redis          # {redis, memcached, valkey}
  size: medium
  region: us-east-1
  tags:
    team: identity
    costCenter: eng
    environment: production
```

### BucketClaim
```yaml
apiVersion: shenplane.io/v1
kind: BucketClaim
metadata:
  name: audit-logs
  namespace: compliance
spec:
  access: private        # {private, authenticated-read}
  region: us-east-1
  tags:
    team: security
    costCenter: compliance
    environment: production
```

### What Happens When A Claim Is Invalid

```
$ kubectl apply -f bad-claim.yaml

Error from server (Forbidden): error when creating "bad-claim.yaml":
admission webhook "validate.shenplane.io" denied the request:
DatabaseClaim "bad-db" rejected:
  - spec.engine: "sqlite" is not a valid engine (postgres, mysql, mariadb)
  - spec.storageGB: 5 is below minimum (10)
  - spec.tags.costCenter: required field missing
  - spec.region: "moon-base-1" is not a valid region
```

These errors are **derived from the Shen proof failures**, not hand-written validation messages. Each line corresponds to a guard constructor that rejected its input.

## How Composition Works (vs Crossplane Patches)

Crossplane expands a Claim into cloud resources via JSONPath patches:
```yaml
# Crossplane Composition — fragile
patches:
  - type: FromCompositeFieldPath
    fromFieldPath: spec.parameters.region    # typo here = silent nil
    toFieldPath: spec.forProvider.region
```

ShenPlane expands a Claim into cloud resources via **typed expansion with proofs**:
```go
// ShenPlane controller — safe
func expandDatabase(db shenguard.SecureDb) (shenguard.DbExpansion, error) {
    claim := db.Claim()
    security := db.Security()

    // Create concrete cloud resources — every field comes from typed claim
    instance := createRDSInstance(claim.Engine(), claim.Size(), claim.Region())
    subnet := createSubnetGroup(claim.Region())
    sg := createSecurityGroup(security.Network())
    role := createIAMRole(security.Iam())

    // Prove cross-resource consistency
    regionProof := shenguard.NewSameRegion(instance, subnet)
    networkProof := shenguard.NewInNetwork(instance, network)

    // Compose the expansion proof — ALL must succeed
    return shenguard.NewDbExpansion(
        db, instance, subnet, sg, role,
        regionProof, networkProof,
    )
    // If any proof fails, expansion is rejected.
    // No silent nil. No partial provisioning.
}
```

The difference: **there are no string paths to get wrong.** Every field access goes through a typed accessor. Cross-resource consistency is proven, not hoped for.

## GitOps Workflow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Developer    │     │  Git Repo    │     │  Cluster     │
│              │     │              │     │              │
│ Write YAML ──┼────►│ Claims/      │     │              │
│ git push     │     │              │     │              │
│              │     │  CI runs:    │     │              │
│              │     │  shen tc+ ───┼────►│              │
│              │     │  go build ───┼────►│              │
│              │     │  go test ────┼────►│              │
│              │     │              │     │              │
│              │     │  Argo syncs ─┼────►│ Webhook      │
│              │     │              │     │ validates ───┤
│              │     │              │     │              │
│              │     │              │     │ Controller   │
│              │     │              │     │ reconciles ──┤
│              │     │              │     │              │
│              │     │              │     │ Cloud        │
│              │     │              │     │ resources ───┤
│              │     │              │     │ provisioned  │
└──────────────┘     └──────────────┘     └──────────────┘
```

Argo CD (or Flux, or any GitOps tool) just syncs YAML. ShenPlane's webhook and controller handle the rest. **No Argo-specific annotations, waves, or hooks needed.** Ordering comes from the proof types.

## Adding A New Resource Type

1. Define the claim type in `specs/core.shen`:
   ```
   (datatype queue-engine
     X : string;
     (element? X [sqs rabbitmq kafka]) : verified;
     ================================================
     X : queue-engine;)

   (datatype queue-claim
     Id : resource-id;
     Engine : queue-engine;
     Region : cloud-region;
     Tags : org-tags;
     =====================
     [Id Engine Region Tags] : queue-claim;)

   (datatype secure-queue
     Claim : queue-claim;
     Security : security-proof;
     ===========================
     [Claim Security] : secure-queue;)
   ```

2. Run `shengen` → new guard types generated automatically.

3. Write the expansion function (typed, not patched).

4. The CRD, webhook validation, and error messages are all derived from the spec.

## File Structure

```
demo/shenplane/
├── specs/core.shen              ← 8-layer proof spec (single source of truth)
├── shenguard/guards_gen.go      ← generated Go guard types
├── examples/
│   ├── database-claim.yaml      ← what users write
│   ├── cache-claim.yaml
│   ├── bucket-claim.yaml
│   ├── network-claim.yaml
│   └── invalid-claim.yaml       ← shows proof-derived errors
├── cmd/shenplane/main.go        ← controller entry point scaffold
└── README.md                    ← this file
```

## The Key Insight

ShenPlane is not "Crossplane but better." It's a different architecture entirely:

- **Crossplane**: YAML → weak schema → JSONPath patches → runtime reconciliation → hope
- **ShenPlane**: YAML → proof construction → typed expansion → ordered reconciliation → certainty

The YAML surface is deliberately identical. Users don't know or care about Shen. They write `kubectl apply -f database-claim.yaml` and either it works (all proofs pass) or they get a clear error message explaining exactly what's wrong and why.

The proofs are invisible to users and visible to the platform team. That's the design.
