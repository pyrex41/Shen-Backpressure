# How It Actually Connects To Your Infrastructure

## The Missing Piece: Discovery

The Shen spec defines *what* invariants must hold. The generated guard types enforce them at compile time. But something needs to **read your actual YAML/cluster state and feed it into the guard constructors**. That something is the **scanner**.

## Architecture

```
YOUR REPO / CLUSTER
─────────────────────
├── argocd/
│   ├── apps/                  ← Argo Application YAMLs
│   └── appsets/               ← ApplicationSets
├── crossplane/
│   ├── xrds/                  ← XRD definitions
│   ├── compositions/          ← Composition pipelines
│   ├── claims/                ← User-facing Claims
│   └── provider-configs/      ← Cloud credentials
├── manifests/
│   └── *.yaml                 ← Raw K8s manifests (any structure)
└── shen/
    ├── specs/core.shen        ← Your invariants
    └── shenguard/guards_gen.go ← Generated (by shengen)

           │
           ▼

┌─────────────────────────────────────────┐
│          shen-k8s-scanner               │
│                                         │
│  1. DISCOVER  — find YAML files         │
│     (configurable paths / auto-detect)  │
│                                         │
│  2. PARSE     — extract structured data │
│     (sync waves, patch paths, XRD       │
│      schemas, provider configs, etc.)   │
│                                         │
│  3. CONSTRUCT — attempt to build proof  │
│     objects using guard constructors    │
│                                         │
│  4. REPORT    — which proofs pass/fail  │
│     and exactly why                     │
└─────────────────────────────────────────┘
```

## Configuration: `shen-k8s.yaml`

The scanner needs to know where your files are. A single config file tells it:

```yaml
# shen-k8s.yaml — drop this in your repo root
spec: shen/specs/core.shen

# Where to find things. All paths relative to repo root.
# Each section is optional — only scan what you use.
sources:
  argo:
    # Where Argo Application / ApplicationSet YAMLs live
    applications:
      - argocd/apps/**/*.yaml
      - argocd/appsets/**/*.yaml
    # Manifests that Argo will sync (rendered or raw)
    manifests:
      - manifests/**/*.yaml
      - helm/rendered/**/*.yaml

  crossplane:
    # XRD definitions
    xrds:
      - crossplane/xrds/**/*.yaml
    # Compositions (the patch pipelines)
    compositions:
      - crossplane/compositions/**/*.yaml
    # Claims (user-facing)
    claims:
      - crossplane/claims/**/*.yaml
    # Provider configs (credentials)
    provider_configs:
      - crossplane/provider-configs/**/*.yaml

  rollouts:
    # Argo Rollout definitions
    - rollouts/**/*.yaml

  workflows:
    # Argo Workflow templates
    - workflows/**/*.yaml

# Known operator mutations (for drift coverage checks).
# These are fields that operators/webhooks modify after apply.
# The scanner checks that every one has a matching ignoreDifferences rule.
known_mutations:
  - operator: cert-manager
    fields:
      - spec.tls.certificate
      - metadata.annotations.cert-manager.io/issuer
  - operator: istio-sidecar-injector
    fields:
      - metadata.labels.istio.io/rev
      - metadata.annotations.sidecar.istio.io/status

# Ingress capabilities (which controllers support traffic splitting)
ingress_capabilities:
  istio: { weighted_routing: true }
  nginx: { weighted_routing: true }  # requires nginx-ingress >= 0.49
  traefik: { weighted_routing: true }
  alb: { weighted_routing: true }
  contour: { weighted_routing: true }
  ambassador: { weighted_routing: true }
  # bare metal nginx without canary annotations:
  # nginx-basic: { weighted_routing: false }

# Optional: pull live state from cluster (requires kubeconfig)
live_cluster:
  enabled: false        # set true to also check running state
  kubeconfig: ~/.kube/config
  context: production   # which context to use
```

### Auto-Detection

If no `shen-k8s.yaml` exists, the scanner auto-detects by looking for:
- Files with `kind: Application` → Argo apps
- Files with `kind: CompositeResourceDefinition` → Crossplane XRDs
- Files with `kind: Composition` → Crossplane Compositions
- Files with `kind: Rollout` → Argo Rollouts
- Files with `kind: Workflow` / `kind: WorkflowTemplate` → Argo Workflows
- Files with `kind: ProviderConfig` → Crossplane credentials

This means **zero config for standard layouts** — just run `shen-k8s-scan .` and it finds everything.

## What The Scanner Extracts (Per Tool)

### From Argo Application YAMLs

```yaml
# The scanner reads this:
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
spec:
  source:
    repoURL: https://github.com/org/repo
    path: manifests/
    targetRevision: main
  destination:
    server: https://kubernetes.default.svc
    namespace: production
  syncPolicy:
    automated:
      prune: true
  ignoreDifferences:
    - group: ""
      kind: Service
      jsonPointers:
        - /spec/clusterIP
```

```
→ Constructs: NewSourceBinding(repo, path, revision, manifestCount)
→ Constructs: NewIgnoreRule(fieldPath) for each ignoreDifferences entry
→ Checks: drift-covered for all known_mutations
→ Checks: app-state with current health/sync (if live_cluster enabled)
```

### From Manifests (sync wave extraction)

```yaml
# The scanner reads sync-wave annotations from every manifest:
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
---
apiVersion: my.org/v1
kind: MyCustomResource
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "1"
```

```
→ Builds wave map: {CRD: -1, CR: 1}
→ Constructs: NewWaveOrdered(-1, 1) — passes (CRD before CR)
→ Constructs: NewCrdBeforeCr(-1, 1, "MyCustomResource") — passes
→ If a CR has wave <= its CRD's wave → proof fails → error reported
```

### From Crossplane Compositions

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
spec:
  compositeTypeRef:
    apiVersion: database.org/v1alpha1
    kind: XDatabase
  resources:
    - name: rds-instance
      base:
        apiVersion: rds.aws.upbound.io/v1beta1
        kind: Instance
      patches:
        - type: FromCompositeFieldPath
          fromFieldPath: spec.parameters.region
          toFieldPath: spec.forProvider.region
        - type: FromCompositeFieldPath
          fromFieldPath: spec.parameters.engine
          toFieldPath: spec.forProvider.engine
```

```
→ Loads XRD schema for XDatabase → knows spec.parameters.region exists
→ Loads MR schema for rds.Instance → knows spec.forProvider.region exists
→ Constructs: NewPatchSourceValid("spec.parameters.region", "XDatabase", true)
→ Constructs: NewPatchTargetValid("spec.forProvider.region", "Instance", true)
→ Constructs: NewPatchValid(source, target) — passes

→ If fromFieldPath has a typo ("spec.paramters.region"):
   NewPatchSourceValid("spec.paramters.region", "XDatabase", false)
   → constructor rejects (PathExists must be true) → ERROR reported
```

### From Crossplane XRDs (schema extraction)

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: CompositeResourceDefinition
metadata:
  name: xdatabases.database.org
spec:
  versions:
    - name: v1alpha1
      schema:
        openAPIV3Schema:
          properties:
            spec:
              properties:
                parameters:
                  properties:
                    region: { type: string }
                    engine: { type: string, enum: [postgres, mysql] }
                    storageGB: { type: integer, minimum: 10 }
                  required: [region, engine]
```

```
→ Extracts field paths: [spec.parameters.region, spec.parameters.engine, ...]
→ Extracts required fields: [region, engine]
→ Used by patch validation (does this path exist in the XRD?)
→ Used by xrd-compatible checks (did a new version remove a field?)
→ Used by resource-complete checks (are all required fields populated?)
```

### From Crossplane ProviderConfigs

```yaml
apiVersion: aws.upbound.io/v1beta1
kind: ProviderConfig
metadata:
  name: aws-prod
spec:
  credentials:
    source: Secret
    secretRef:
      name: aws-creds
      namespace: crossplane-system
```

```
→ Constructs: NewProviderConfig("aws-prod", "aws")
→ If live_cluster enabled: reads the Secret, checks expiry
   NewCredentialValid(config, expiresAt, now) — fails if expired
→ If live_cluster disabled: checks that the Secret reference is valid
   (Secret exists in manifests or known to exist)
```

### From Argo Rollouts

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
spec:
  strategy:
    canary:
      steps:
        - setWeight: 20
        - pause: { duration: 5m }
        - analysis:
            templates:
              - templateName: success-rate
        - setWeight: 50
        - analysis:
            templates:
              - templateName: success-rate
        - setWeight: 100
```

```
→ Extracts weight sequence: [20, 50, 100]
→ Constructs: NewWeightMonotonic(20, 50) — passes (non-decreasing)
→ Constructs: NewWeightMonotonic(50, 100) — passes
→ Checks each analysis template exists and has valid metric queries
→ Checks ingress controller supports traffic splitting
→ Constructs: NewSafeRollout(trafficCapable, 6, allHaveFallback)
```

### From Argo Workflows

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Workflow
spec:
  templates:
    - name: etl-pipeline
      dag:
        tasks:
          - name: extract
            template: extract-tmpl
          - name: transform
            template: transform-tmpl
            dependencies: [extract]
          - name: load
            template: load-tmpl
            dependencies: [transform]
```

```
→ Assigns topological order: extract=0, transform=1, load=2
→ Constructs: NewDagEdge("extract", "transform", 0, 1) — passes
→ Constructs: NewDagEdge("transform", "load", 1, 2) — passes
→ If "load" also depended on "extract": NewDagEdge("extract", "load", 0, 2) — passes
→ If there were a cycle (load→extract): no valid topo ordering exists → ERROR
```

## Running The Scanner

```bash
# Basic: scan repo, auto-detect files
shen-k8s-scan .

# With explicit config
shen-k8s-scan . --config shen-k8s.yaml

# Scan specific sections only
shen-k8s-scan . --only crossplane
shen-k8s-scan . --only argo,rollouts

# With live cluster checks
shen-k8s-scan . --live --context production

# Output formats
shen-k8s-scan . --format json       # machine-readable
shen-k8s-scan . --format table      # human-readable (default)

# In CI (non-zero exit on any failure)
shen-k8s-scan . --strict
```

### Example Output

```
$ shen-k8s-scan .

Shen K8s Infrastructure Scanner
Spec: shen/specs/core.shen (79 types)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ArgoCD (3 Applications, 47 manifests)
  ✓ source-binding       my-app → github.com/org/repo @ main (47 manifests)
  ✓ wave-ordered         14 resources across 4 waves, all ordered
  ✓ crd-before-cr        2 CRDs, all in wave -1 (before CRs in wave 0+)
  ✗ drift-covered        FAIL: cert-manager mutates Service/spec.tls.certificate
                          but no ignoreDifferences rule covers it
                          → Add to argocd/apps/my-app.yaml:
                            ignoreDifferences:
                              - group: ""
                                kind: Service
                                jsonPointers: [/spec/tls/certificate]

Crossplane (2 XRDs, 3 Compositions, 5 Claims)
  ✓ patch-valid          18 patches across 3 Compositions, all paths exist
  ✗ patch-valid          FAIL: compositions/database.yaml line 42
                          fromFieldPath "spec.paramters.region" does not exist in XRD
                          Did you mean: spec.parameters.region?
  ✓ xrd-compatible       v1alpha1 → v1alpha2: no removed fields, no new required
  ✓ resource-complete    all 5 claims fully populate required fields
  ✓ mr-credentialed      all MRs reference valid ProviderConfigs

Argo Rollouts (1 Rollout)
  ✓ weight-monotonic     [20, 50, 100] — non-decreasing
  ✓ traffic-split-capable istio supports weighted routing
  ✓ analysis-reachable   success-rate template returns data (avg 142 samples/window)
  ✓ safe-rollout         3 analysis steps, all have on-failure: abort

Argo Workflows (2 templates)
  ✓ dag-acyclic          etl-pipeline: 3 tasks, valid topological order
  ✓ param-type-match     4 parameter handoffs, all types match
  ✓ artifact-satisfied   2 artifact dependencies, all satisfied
  ✗ retry-within-deadline FAIL: transform step: 5 retries × 60s max backoff = 300s
                          but deadline is 120s. Reduce retries or increase deadline.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Results: 15 passed, 3 failed
```

## Where Schema Information Comes From

The scanner needs to know "does this field path exist?" for patch validation. Sources (checked in order):

1. **XRD schemas** — parsed directly from the `openAPIV3Schema` in your XRD YAML files. These define the Claim/XR field paths.

2. **CRD schemas from providers** — Crossplane providers install CRDs. The scanner can:
   - Read CRD YAMLs if they're in your repo (some teams vendor them)
   - Pull from cluster if `live_cluster.enabled: true`
   - Pull from provider package metadata (upbound marketplace)
   - Use a local cache (`~/.shen-k8s/crds/`) that auto-downloads

3. **Managed Resource docs** — For known providers (aws-upbound, gcp, azure), the scanner ships with field maps for common resource types (RDS Instance, S3 Bucket, etc.) as a fallback.

4. **Explicit schema hints** — In `shen-k8s.yaml`:
   ```yaml
   schema_hints:
     rds.aws.upbound.io/v1beta1/Instance:
       required: [region, engine, instanceClass, allocatedStorage]
       fields: [region, engine, instanceClass, allocatedStorage, ...]
   ```

## Integration Points

### As a PreSync Hook (automatic)

```yaml
# presync-validate.yaml
apiVersion: batch/v1
kind: Job
metadata:
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: HookSucceeded
spec:
  template:
    spec:
      containers:
      - name: scan
        image: your-org/shen-k8s-scanner:latest
        args: ["--strict", "--only", "argo,crossplane"]
```

### As a CI step (recommended starting point)

```yaml
# .github/workflows/shen-k8s.yml
- name: Shen K8s Scan
  run: shen-k8s-scan . --strict --format table
```

### As an admission webhook (strongest)

The scanner can run as a long-lived webhook server that intercepts
CREATE/UPDATE for Application, Composition, Claim, and Rollout resources.

### As a Crossplane Composition Function (deepest)

Import the `shenguard` package directly. No scanner needed — the guard
types are compiled into your function binary.
