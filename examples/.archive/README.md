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

| Directory | What it explored |
|---|---|
| `ai-grounding/` | Anti-hallucination — closed-enum guard types so an LLM can only return grounded values |
| `audit-trail/` | Append-only audit log with provenance-typed records |
| `category-showcase/` | Teaching aid — all six shengen categories (wrapper, constrained, composite, guarded, proof-chain, sum) in one spec |
| `circuit-breaker/` | State-machine flow control variant (closed/open/half-open) |
| `consensus-quorum/` | Quorum-typed votes, prevents counting without N agreements |
| `crispr-pipeline/` | Bio-domain pipeline with dose-and-genome-typed steps |
| `data-pipeline/` | Pipeline stages typed with stage-of-origin proofs |
| `defi-invariants/` | DeFi balance/conservation invariants |
| `dosage-calculator/` | Healthcare dosage calculator (scaffolded — `internal/shenguard/` never generated) |
| `email-crud/` | Full Go CRUD app, but guards live in `reference/` rather than the build hot path |
| `feature-flags/` | Feature-flag rollout with cohort-typed targeting |
| `k8s-infra/` | Kubernetes admission scanners (near-duplicate of `shenguard-bolt-on`) |
| `llm-hallucination-guard/` | Closed-enum variant of the anti-hallucination thesis |
| `order-state-machine/` | Order state machine — invalid transitions become compile errors |
| `pipeline-state-machine/` | Generic state-machine pipeline |
| `polyglot-comparison/` | Multi-language guard comparison (duplicates `payment/reference/`) |
| `rbac-capabilities/` | RBAC capability proof chain (spec-only stub for the multi-tenant-api thesis) |
| `relational-constraints/` | Cross-record constraints between rows |
| `shen-fastapi/` | Python FastAPI framework scaffold (Wave-4 vintage) |
| `shen-go-advanced/` | Advanced Go service variant |
| `shen-go-api/` | Go HTTP service framework scaffold |
| `shen-hono/` | TypeScript Hono framework scaffold |
| `shen-prolog-ui/` | Prolog/UI exploration |
| `shen-rust-axum/` | Rust Axum framework scaffold |
| `shenguard-bolt-on/` | Bolt `shenguard` onto existing Argo + Crossplane control planes |
| `shenplane/` | Clean-sheet K8s control plane counterpart to `shenguard-bolt-on` |
| `sum-type-showcase/` | Sum-type variant of `category-showcase` |
| `workflow-saga/` | Saga state-machine variant |

`FRAMEWORK_EXAMPLES.md` and `INFRA_COMPARISON.md` document the original
framework-scaffolds and infra-cluster comparisons; both are now reference
docs that live alongside the archived directories they describe.

## Reviving one

To move an archive entry back into rotation:

1. `git mv examples/.archive/<name>/ examples/<name>/`
2. Run `sb gen` if `internal/shenguard/guards_gen.go` is stale.
3. Wire `sb.toml` and the gate commands if it's graduating to Tier A/B.
4. Add a row to the focused set in the top-level `README.md`.
