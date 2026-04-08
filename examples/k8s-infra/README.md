# Kubernetes Infrastructure Orchestration

Formal types for **ArgoCD + Argo Workflows + Argo Rollouts + Crossplane** that turn runtime infrastructure failures into compile-time type errors.

## The Problem

K8s infra tools fail in predictable ways that share a common root cause: **unvalidated references, implicit ordering, type-erased parameters, and silent nil patches.** These aren't exotic edge cases — they're the #1 source of 3 AM incidents.

| Tool | Common Failure | Root Cause |
|------|---------------|------------|
| ArgoCD | Infinite sync loop | Operator-mutated field not in `ignoreDifferences` |
| ArgoCD | Partial sync crash | CRD in later sync wave than its CRs |
| Argo Workflows | Silent deadlock | Cycle in DAG dependencies |
| Argo Workflows | Runtime type error | All parameters are strings; number expected |
| Argo Rollouts | Stuck rollout | Analysis query returns no data (vacuously true) |
| Argo Rollouts | Traffic black hole | Ingress doesn't support weighted routing |
| Crossplane | Silent nil patch | JSONPath typo in Composition patch |
| Crossplane | Cascade failure | Provider credentials expired |
| Crossplane | Breaking change | XRD schema removes field used by existing Claims |

## What The Spec Enforces

### ArgoCD (GitOps)

```
source-binding ─► app-state (source resolves to manifests)
                      │
wave-ordered ────► crd-before-cr (CRDs before CRs, always)
                      │
drift-covered ───► app-promotable (healthy + synced + drift handled)
```

- **`wave-ordered`**: Dependency wave < dependent wave. Cross-wave violations are type errors.
- **`crd-before-cr`**: CRD sync wave strictly before CR sync wave. Always.
- **`drift-covered`**: Every known operator mutation has a matching `ignoreDifferences` rule.
- **`app-promotable`**: Can only promote apps that are `healthy` AND `synced`.

### Argo Workflows (DAG Engine)

```
dag-edge ─────────► topo-order proof (from < to, no cycles possible)
                          │
param-type-match ───► typed parameters (no string-to-number surprises)
                          │
artifact-satisfied ─► every input has a matching output from a dependency
                          │
retry-within-deadline ► worst-case retry fits within timeout
```

- **`dag-edge`**: Topological ordering enforced — `FromOrder < ToOrder`. Cycles are structurally impossible.
- **`param-type-match`**: Output type must equal input type. No more "all parameters are strings."
- **`artifact-satisfied`**: Every artifact input has a corresponding artifact output from an upstream dependency.
- **`retry-within-deadline`**: `retries * maxBackoff ≤ deadline`. No infinite retry loops.

### Argo Rollouts (Progressive Delivery)

```
traffic-split-capable ─► ingress supports weighted routing
                              │
weight-monotonic ─────────► canary weights non-decreasing
                              │
analysis-reachable ───────► metric query returns real data
                              │
step-with-fallback ───────► every step has an on-failure path
                              │
                         safe-rollout (all proofs composed)
```

- **`traffic-split-capable`**: Proof that the ingress controller supports traffic splitting. No silent weight ignoring.
- **`weight-monotonic`**: Canary weights must be non-decreasing. No traffic oscillation.
- **`analysis-reachable`**: Metric query has `sampleCount > 0`. No vacuously-true analysis.
- **`step-with-fallback`**: Every rollout step specifies what happens on failure. No stuck rollouts.

### Crossplane (Infrastructure as Code)

```
credential-valid ──────► provider config not expired
                              │
patch-source-valid ───┐
patch-target-valid ───┼──► patch-valid (both paths exist in schema)
                      │
mr-dependency ────────► dependency ready before dependent created
                              │
xrd-compatible ───────► schema changes don't break existing claims
                              │
resource-complete ────► all required fields populated after patches
                              │
mr-credentialed ──────► every managed resource has valid credentials
```

- **`patch-valid`**: Both source (XRD) and target (MR) paths verified to exist. No silent nil patches.
- **`credential-valid`**: Provider config expiry checked. No "Cannot Observe" cascades.
- **`xrd-compatible`**: No removed fields, no new required fields in same major version.
- **`resource-complete`**: All required `forProvider` fields populated. No partial cloud API calls.

### Composed Platform Proof

```
app-promotable ──┐
safe-rollout ────┼──► gitops-deploy-ready
                 │
resource-complete ──┐
mr-credentialed ────┼──► infra-provisioned
                    │
                    └──► platform-ready (infra + deploy)
```

The final `platform-ready` type requires proof that infrastructure is provisioned AND the application is deployable. This is the complete proof that a platform change is safe to apply.

## Bolt-On Architecture: Nothing Gets Replaced

The key insight: **this doesn't require rewriting Argo or Crossplane.** Shen guards bolt onto the existing pipeline at four independent integration points. Each can be adopted incrementally — start with whichever hurts most.

```
┌─────────────────────────────────────────────────────────┐
│                  YOUR EXISTING STACK                     │
│  (Argo CD + Crossplane + whatever else you run)         │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │ ArgoCD   │  │Crossplane│  │  Argo    │              │
│  │ Server   │  │Controllers│  │ Rollouts │              │
│  └────▲─────┘  └────▲─────┘  └────▲─────┘              │
│       │              │              │                    │
│  ═════╪══════════════╪══════════════╪════════════════    │
│       │  BOLT-ON GATES (pick any)   │                    │
│       │              │              │                    │
│  ┌────┴─────┐  ┌────┴─────┐  ┌────┴─────┐  ┌────────┐ │
│  │ PreSync  │  │Composition│  │Admission │  │  CI    │ │
│  │  Hook    │  │ Function  │  │ Webhook  │  │Pipeline│ │
│  │ (7b)     │  │  (7a)     │  │  (7c)    │  │ (7d)   │ │
│  └────▲─────┘  └────▲─────┘  └────▲─────┘  └────▲───┘ │
│       │              │              │              │      │
│       └──────────────┴──────────────┴──────────────┘      │
│                          │                                │
│              ┌───────────┴───────────┐                    │
│              │   shengen guard types  │                    │
│              │   (generated from spec)│                    │
│              └───────────▲───────────┘                    │
│                          │                                │
│              ┌───────────┴───────────┐                    │
│              │  specs/core.shen       │ ◄── single source │
│              │  (formal invariants)   │     of truth      │
│              └───────────────────────┘                    │
└─────────────────────────────────────────────────────────┘
```

### Bolt-On Point 7a: Crossplane Composition Functions

Crossplane Composition Functions are Go (or Python) code that runs *inside* the reconciliation loop. This is where the real logic lives — and where JSONPath typos become silent nil patches.

**How it bolts on:** Import the generated `shenguard` package into your Composition Function. Every resource your function produces must be constructed through guard type constructors.

```go
// Inside your Composition Function (Go)
import "your-org/infra/shenguard"

func RunFunction(req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
    // Parse claim inputs through guard constructors
    name, err := shenguard.NewResourceName(req.Input.Name)
    if err != nil { return nil, err }  // invalid name → rejected immediately

    provider, err := shenguard.NewProviderName("aws")
    if err != nil { return nil, err }

    config := shenguard.NewProviderConfig(name, provider)

    // Credential expiry check — type system forces this
    cred, err := shenguard.NewCredentialValid(config, expiresAt, time.Now().Unix())
    if err != nil { return nil, err }  // expired creds → compile-time-equivalent rejection

    // Patch path validation
    srcPath, _ := shenguard.NewPatchSourcePath("spec.parameters.region")
    src, err := shenguard.NewPatchSourceValid(srcPath, xrdName, pathExists)
    if err != nil { return nil, err }  // JSONPath doesn't exist → caught here, not at 3 AM

    // Policy compliance — can't skip any of these
    enc, _ := shenguard.NewEncryptionEnabled(mr, "AES256")
    priv, _ := shenguard.NewPrivateNetwork(mr, "private", false)
    tagged, _ := shenguard.NewCostTagged(mr, "platform-team", "production")
    policy := shenguard.NewPolicyCompliant(enc, priv, tagged)

    // Final proof: resource is complete + policy-compliant + credentialed
    claim, err := shenguard.NewSecureDbClaim(complete, policy, credentialed)
    // If we get here, ALL invariants are proven. Emit the resources.
}
```

**What changes:** Only your Composition Function code. Crossplane itself is untouched. The function just imports generated types and uses their constructors. If your function compiles, every resource it produces satisfies the invariants.

### Bolt-On Point 7b: Argo PreSync Hook

Argo supports PreSync hooks — Jobs that run *before* any manifests are applied. A Shen-powered PreSync hook validates the entire manifest set.

```yaml
# presync-validate.yaml (committed alongside your manifests)
apiVersion: batch/v1
kind: Job
metadata:
  name: shen-presync-gate
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: HookSucceeded
spec:
  template:
    spec:
      containers:
      - name: validate
        image: your-org/shen-k8s-validator:latest
        command: ["./validator"]
        args:
        - "--spec=/specs/core.shen"
        - "--manifests=/manifests/"
        volumeMounts:
        - name: manifests
          mountPath: /manifests
      restartPolicy: Never
```

The `validator` binary (built with `shenguard` types) reads the rendered manifests, constructs proof objects for wave ordering, CRD precedence, and drift coverage. If any proof fails to construct, the Job exits non-zero → Argo aborts the sync.

**What changes:** One Job YAML added to your repo. Argo itself is untouched.

### Bolt-On Point 7c: Admission Webhook

A `ValidatingAdmissionWebhook` that intercepts `kubectl apply` for Claims and validates them against the spec before they reach Crossplane.

```go
// admission handler (standard K8s admission webhook)
func validateClaim(review *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
    claim, err := shenguard.NewClaimSubmitted(kind, name, namespace)
    if err != nil {
        return deny("invalid claim structure: " + err.Error())
    }

    // Check schema, policy, credentials
    allowed, err := shenguard.NewAdmissionAllowed(claim, schemaOk, policyOk, credOk)
    if err != nil {
        return deny("admission proof failed: " + err.Error())
    }

    return allow()
}
```

**What changes:** One webhook Deployment + one `ValidatingWebhookConfiguration`. Crossplane and Argo are untouched. Bad Claims are rejected at `kubectl apply` time — they never enter the reconciliation loop.

### Bolt-On Point 7d: CI Pipeline Gate

The lightest integration. Add a step to your GitHub Actions / GitLab CI that runs the five-gate check before changes can merge.

```yaml
# .github/workflows/shen-gate.yml
name: Shen Infrastructure Gate
on: [pull_request]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4

    - name: Generate guard types
      run: shengen -spec specs/core.shen -out shenguard/guards_gen.go -pkg shenguard

    - name: Compile (guards enforce invariants)
      run: go build ./...

    - name: Run tests
      run: go test ./...

    - name: Shen type check (spec consistency)
      run: shen-sbcl -e "(load \"specs/core.shen\") (tc +)"

    - name: Audit TCB
      run: ./bin/shenguard-audit.sh
```

**What changes:** One CI workflow file. Everything else is untouched. Invalid infra specs fail the PR check — they never reach the cluster.

## Incremental Adoption Path

Start with what hurts most. Each gate is independent:

| Pain Point | Start With | Effort | Impact |
|-----------|-----------|--------|--------|
| Silent nil patches in Compositions | **7a** (Composition Function) | Medium — refactor function to use guard types | Eliminates the #1 Crossplane failure mode |
| Bad YAML reaching cluster | **7d** (CI Gate) | Low — add one workflow file | Catches problems at PR time |
| Partial syncs / wave ordering | **7b** (PreSync Hook) | Low — add one Job YAML | Prevents sync-time crashes |
| Invalid Claims applied | **7c** (Admission Webhook) | Medium — deploy one webhook | Rejects bad Claims at `kubectl apply` |
| All of the above | **All four** | Combine incrementally | Full deductive coverage |

The point: **you never touch Argo or Crossplane source code.** You bolt on gates at the boundaries where data enters the system. The guards ensure that by the time a manifest, Claim, or Composition Function response reaches the controller, it's already proven correct.

## Crossplane-Specific Invariants (Policy Proofs)

The spec includes organizational policy types that compose into a `policy-compliant` proof:

```
encryption-enabled ──┐
private-network ─────┼──► policy-compliant
cost-tagged ─────────┘         │
                          secure-db-claim (resource + policy + credential)
```

These encode the invariants that every organization eventually needs:
- **Encryption at rest** — every database, every bucket, every volume
- **No public endpoints** — private subnets only, proven not just configured
- **Cost tagging** — team + environment tags on every resource
- **Credential binding** — every managed resource has valid, non-expired creds

A `secure-db-claim` cannot be constructed without proving ALL of these. This replaces OPA/Gatekeeper policies with compile-time enforcement — the invalid Claim can't exist, not just "gets rejected after the fact."

## Composed Proofs

```
                    ┌── ci-gate-passed (shen tc+ ∧ guards compile ∧ tests pass)
merge-ready ◄───────┤
                    └── presync-cleared (waves valid ∧ CRDs ordered)

                    ┌── app-promotable (healthy ∧ synced)
gitops-deploy-ready◄┤
                    └── safe-rollout (traffic-capable ∧ steps ∧ fallbacks)

                    ┌── resource-complete (all fields populated)
infra-provisioned ◄─┤
                    └── mr-credentialed (valid creds bound)

                    ┌── infra-provisioned
platform-ready ◄────┤
                    └── gitops-deploy-ready
```

`platform-ready` is the top-level proof. To construct it, you need every sub-proof. Any missing link — expired credential, incomplete resource, stuck rollout, bad wave ordering — makes the proof unconstructable. The compiler tells you exactly which proof failed and why.

## Usage

```bash
# Generate guard types (Go)
shengen -spec specs/core.shen -out shenguard/guards_gen.go -pkg shenguard

# Also available for other languages
shengen-ts -spec specs/core.shen -out shenguard/guards_gen.ts
shengen-py -spec specs/core.shen -out shenguard/guards_gen.py
```

See `specs/core.shen` for the full 80+ type specification.
