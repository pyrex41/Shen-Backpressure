# Archived Examples

These directories were scoped out of the curated demo set during the
2026-04-16 demo-readiness pass and the 2026-05-05 contraction pass. They
are kept under git history for reference — the prompts and specs are
useful as a starting point for future variants, even though none are
part of the live demo surface.

The live `examples/` tree holds three focused demos:

| Kept | Role |
|---|---|
| `payment/` | Flagship Tier-A demo — payment processor with balance invariants, shengen generated, shen-derive gate wired, reference outputs in Go/TS/Rust/Python |
| `multi-tenant-api/` | Tier-A HTTP service — JWT → AuthenticatedUser → TenantAccess → ResourceAccess proof chain with live curl transcript |
| `shen-web-tools/` | Polyglot end-to-end — Shen/SBCL backend + Arrow.js frontend, three specs, TS derive gate |

Everything else lives here.

## Inventory

Each row carries the original direction plus the reason it was scoped
out. Cluster groupings explain why a thesis appears multiple times.

| Directory | What it explored | Scoped out because |
|---|---|---|
| `ai-grounding/` | Anti-hallucination — closed-enum guard types so an LLM can only return grounded values | Anti-hallucination cluster: `shen-web-tools/` is built end-to-end on the same thesis |
| `audit-trail/` | Append-only audit log with provenance-typed records | Domain stub — spec + guards only, no app code |
| `category-showcase/` | Teaching aid — all six shengen categories (wrapper, constrained, composite, guarded, proof-chain, sum) in one spec | Reference material now lives in `docs/REFERENCE.md` and `examples/payment/reference/` |
| `circuit-breaker/` | State-machine flow control variant (closed/open/half-open) | State-machine cluster: same thesis as `order-state-machine/`, `workflow-saga/`, `pipeline-state-machine/` |
| `consensus-quorum/` | Quorum-typed votes, prevents counting without N agreements | Domain stub — spec + guards only |
| `crispr-pipeline/` | Bio-domain pipeline with dose-and-genome-typed steps | Domain stub — spec + guards only |
| `data-pipeline/` | Pipeline stages typed with stage-of-origin proofs | Domain stub — spec + guards only |
| `defi-invariants/` | DeFi balance/conservation invariants | Domain stub — overlaps with `payment/`'s balance-invariant story |
| `dosage-calculator/` | Healthcare dosage calculator | Scaffolded but `internal/shenguard/` was never generated |
| `email-crud/` | Full Go CRUD app, but guards live in `reference/` rather than the build hot path | Shengen never wired into the build path |
| `feature-flags/` | Feature-flag rollout with cohort-typed targeting | Domain stub — spec + guards only |
| `k8s-infra/` | Kubernetes admission scanners | K8s/infra cluster: near-duplicate of `shenguard-bolt-on/` |
| `llm-hallucination-guard/` | Closed-enum variant of the anti-hallucination thesis | Anti-hallucination cluster: simpler dup of `ai-grounding/`, both subsumed by `shen-web-tools/` |
| `order-state-machine/` | Order state machine — invalid transitions become compile errors | State-machine cluster: kept as `category-showcase`-style stub, no end-to-end demo |
| `pipeline-state-machine/` | Generic state-machine pipeline | State-machine cluster — duplicate thesis |
| `polyglot-comparison/` | Multi-language guard comparison | Duplicates `examples/payment/reference/`, which already shows all four target languages |
| `rbac-capabilities/` | RBAC capability proof chain | Authorization-proof-chain cluster: spec-only stub redundant with built-out `multi-tenant-api/` |
| `relational-constraints/` | Cross-record constraints between rows | Domain stub — spec + guards only |
| `shen-fastapi/` | Python FastAPI framework scaffold | Framework-scaffolds cluster (Wave-4 vintage): no end-to-end app, prompt-only |
| `shen-go-advanced/` | Advanced Go service variant | Framework-scaffolds cluster — variant of `shen-go-api/` |
| `shen-go-api/` | Go HTTP service framework scaffold | Framework-scaffolds cluster — no app code |
| `shen-hono/` | TypeScript Hono framework scaffold | Framework-scaffolds cluster — no app code |
| `shen-prolog-ui/` | Prolog/UI exploration | Out-of-band exploration, never integrated |
| `shen-rust-axum/` | Rust Axum framework scaffold | Framework-scaffolds cluster — no app code |
| `shenguard-bolt-on/` | Bolt `shenguard` onto existing Argo + Crossplane control planes | K8s/infra cluster: same thesis as `k8s-infra/` and `shenplane/` |
| `shenplane/` | Clean-sheet K8s control plane counterpart to `shenguard-bolt-on` | K8s/infra cluster: ambitious vision but not built out |
| `sum-type-showcase/` | Sum-type variant of `category-showcase` | Teaching-aid duplicate of `category-showcase/` |
| `workflow-saga/` | Saga state-machine variant | State-machine cluster — duplicate thesis |

`FRAMEWORK_EXAMPLES.md` and `INFRA_COMPARISON.md` document the original
framework-scaffolds and infra-cluster comparisons; both are now reference
docs that live alongside the archived directories they describe.

## Reviving one

To move an archive entry back into rotation:

1. `git mv examples/.archive/<name>/ examples/<name>/`
2. Run `sb gen` if `internal/shenguard/guards_gen.go` is stale.
3. Wire `sb.toml` and the gate commands if it's graduating to Tier A/B.
4. Add a row to the focused set in the top-level `README.md`.
