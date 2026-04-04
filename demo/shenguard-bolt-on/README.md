# ShenGuard — Bolt-On Verification for Existing Argo + Crossplane Stacks

**You keep everything you have. ShenGuard adds compile-time safety at the boundaries.**

## What This Is

ShenGuard is a verification layer that **bolts onto** your existing Argo CD + Crossplane infrastructure. It doesn't replace your tools, change your workflow, or require migration. It reads your existing YAML files and validates them against formal invariants before they reach the cluster.

See also: [`demo/shenplane/`](../shenplane/) — the clean-sheet replacement that eliminates Crossplane/Argo baggage entirely.

## Who This Is For

Teams that:
- Already run Argo CD + Crossplane (or are adopting them)
- Have been burned by silent failures (nil patches, expired creds, stuck rollouts)
- Want safety without rewriting their infra stack
- Need to add verification incrementally, starting with what hurts most

## What Changes, What Doesn't

| Changes | Doesn't Change |
|---------|---------------|
| Add `shen-k8s.yaml` config (optional) | ArgoCD server, config, behavior |
| Add CI workflow step | Crossplane controllers, providers, CRDs |
| Add PreSync hook Job (optional) | Your YAML file layout |
| Import `shenguard` in Composition Functions (optional) | Your existing Compositions, XRDs, Claims |
| Add admission webhook (optional) | `kubectl apply` workflow |

## End-to-End Walkthrough

### Step 1: Drop the spec into your repo

```
your-infra-repo/
├── argocd/apps/my-app.yaml           ← existing
├── crossplane/compositions/db.yaml   ← existing
├── crossplane/xrds/database.yaml     ← existing
├── manifests/                         ← existing
└── shen/                              ← NEW (just this)
    └── specs/core.shen
```

Start with the invariants that hurt most. Example — just Crossplane patch validation:

```
\* Both source and target paths must exist for a patch to be valid *\
(datatype patch-valid
  Source : patch-source-valid;
  Target : patch-target-valid;
  ==============================
  [Source Target] : patch-valid;)
```

### Step 2: Generate guard types

```bash
shengen -spec shen/specs/core.shen -out shen/shenguard/guards_gen.go -pkg shenguard
```

### Step 3: Add a CI gate (~5 min)

```yaml
# .github/workflows/shen-guard.yml
name: ShenGuard
on: [pull_request]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - name: Generate guards
      run: shengen -spec shen/specs/core.shen -out shen/shenguard/guards_gen.go -pkg shenguard
    - name: Scan infrastructure
      run: shen-k8s-scan . --strict
```

### Step 4: See results

A PR that introduces a Composition patch typo:

```diff
  patches:
    - type: FromCompositeFieldPath
-     fromFieldPath: spec.parameters.region
+     fromFieldPath: spec.paramters.region     # typo!
      toFieldPath: spec.forProvider.region
```

CI output:
```
Crossplane (2 XRDs, 3 Compositions, 5 Claims)
  ✗ patch-valid  FAIL: compositions/database.yaml line 42
                 fromFieldPath "spec.paramters.region" does not exist in XRD
                 Did you mean: spec.parameters.region?

Results: 14 passed, 1 failed
```

**The typo is caught at PR time, not at 3 AM during reconciliation.**

### Step 5: Expand coverage incrementally

| Pain Point | Add This | Catches |
|-----------|---------|---------|
| Nil patches | `patch-valid` | JSONPath typos in Compositions |
| Expired creds | `credential-valid` | Provider configs approaching expiry |
| Bad sync waves | `wave-ordered`, `crd-before-cr` | CRDs deployed after their CRs |
| Infinite sync loops | `drift-covered` | Operator mutations without ignoreDifferences |
| Stuck rollouts | `analysis-reachable`, `traffic-split-capable` | Vacuous metrics, wrong ingress |
| Security gaps | `policy-compliant` | Resources missing encryption/network/tagging |

Each invariant is independent. Add one at a time.

## Four Bolt-On Points

```
YOUR EXISTING STACK (unchanged)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     │              │              │              │
  ┌──┴───┐     ┌────┴───┐     ┌───┴────┐    ┌───┴──────┐
  │  CI  │     │PreSync │     │Admis-  │    │Compo-    │
  │ Gate │     │ Hook   │     │sion    │    │sition    │
  │      │     │        │     │Webhook │    │Function  │
  │ (7d) │     │ (7b)   │     │ (7c)   │    │ (7a)     │
  └──┬───┘     └────┬───┘     └───┬────┘    └───┬──────┘
     └──────────────┴─────────────┴──────────────┘
                         │
               shenguard constructors
                         │
               specs/core.shen
```

| Gate | What It Is | Effort | When Failures Are Caught |
|------|-----------|--------|------------------------|
| **CI Gate** | GitHub Actions step | 5 min | At PR time |
| **PreSync Hook** | Argo Job YAML | 10 min | Before Argo applies manifests |
| **Admission Webhook** | K8s webhook deployment | 1 hour | At `kubectl apply` time |
| **Composition Function** | Import shenguard in Go | 2-4 hours | At compile time (strongest) |

## The Scanner

`shen-k8s-scan` bridges YAML files → guard constructors:

1. **Discovers** YAML files (auto-detect by `kind:` field, or explicit config)
2. **Extracts** structured data (sync waves, patch paths, XRD schemas, canary weights)
3. **Constructs** proof objects using generated guard constructors
4. **Reports** pass/fail with file path, line number, and suggested fix

See [SCANNER.md](./SCANNER.md) for the full design.

## Spec Coverage (79 types)

| Section | Types | Validates |
|---------|-------|----------|
| Common K8s | 6 | Namespaces, names, timestamps |
| ArgoCD | 15 | Sync waves, drift, CRD ordering, health |
| Argo Workflows | 13 | DAG acyclicity, typed params, artifacts, retry bounds |
| Argo Rollouts | 11 | Weight monotonicity, traffic splitting, analysis |
| Crossplane | 20 | Patch paths, XRD compat, creds, resource completeness |
| Bolt-On Gates | 14 | Security policies, presync, admission, CI |

## Files

```
demo/shenguard-bolt-on/
├── specs/core.shen              ← 79-type Shen spec
├── shenguard/guards_gen.go      ← generated Go guard types
├── scanner/                     ← YAML → proof adapter
│   ├── config.go
│   ├── discover.go
│   ├── parse.go
│   └── validate.go
├── cmd/scanner/main.go          ← CLI entry point
├── shen-k8s.example.yaml        ← annotated config
├── SCANNER.md                   ← scanner design doc
└── README.md                    ← this file
```
