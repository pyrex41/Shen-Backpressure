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

## Usage

```bash
# Generate guard types
shengen -spec specs/core.shen -out shenguard/guards_gen.go -pkg shenguard

# Use in a GitOps controller or CI pipeline
# Every deployment must construct a platform-ready proof
# If any sub-proof fails (expired creds, missing patch path, etc.),
# the deployment cannot proceed — it won't compile.
```

## How This Fits Real Workflows

1. **CI/CD Gate**: Before ArgoCD syncs, validate the manifest set against the spec. Sync wave ordering, drift coverage, and CRD ordering are checked at PR time, not at 3 AM.

2. **Crossplane Composition Linter**: When authoring Compositions, validate patch paths against the XRD schema and target MR schema. Catch JSONPath typos before `kubectl apply`.

3. **Rollout Pre-flight**: Before starting a canary rollout, prove the ingress supports traffic splitting and the analysis template returns data. No more stuck rollouts.

4. **Credential Rotation Alert**: When credentials approach expiry, the `credential-valid` proof fails — proactively, not when managed resources start failing.

See `specs/core.shen` for the full specification.
