# Archived Examples

These examples were scoped out during the 2026-04-16 demo-readiness pass (see `thoughts/shared/research/2026-04-16-demo-readiness-buttoning-up.md`). They remain git-tracked for reference but are not part of the focused demo surface.

## Why archived

The live `examples/` tree holds six focused examples:

| Kept | Role |
|---|---|
| `payment/` | Flagship Tier-A demo — payment processor with balance invariants, shengen generated, shen-derive gate wired, reference outputs in Go/TS/Rust/Python |
| `multi-tenant-api/` | Tier-A HTTP service — JWT → AuthenticatedUser → TenantAccess → ResourceAccess proof chain with live curl transcript |
| `shen-web-tools/` | Tier-B polyglot — Shen/SBCL backend + Arrow.js frontend, three specs, TS derive gate |
| `order-state-machine/` | State machine example (invalid transition = type error) |
| `shenguard-bolt-on/` | Infrastructure story — bolt `shenguard` onto existing Argo + Crossplane |
| `category-showcase/` | Teaching aid — all six shengen categories in one spec |

Everything else was archived under one of these reasons.

## Overlap clusters resolved

- **K8s / infra** — `k8s-infra/` (near-duplicate of `shenguard-bolt-on`), `shenplane/` (clean-sheet counterpart); `INFRA_COMPARISON.md` is here too.
- **Anti-hallucination** — `ai-grounding/`, `llm-hallucination-guard/` are stubs on the same thesis as `shen-web-tools/`.
- **State machines** — `workflow-saga/`, `circuit-breaker/`, `pipeline-state-machine/` teach the same lesson as the kept `order-state-machine/`.
- **Authorization proof chains** — `rbac-capabilities/` is a spec-only stub redundant with the built-out `multi-tenant-api/`.
- **Polyglot duplicates** — `polyglot-comparison/` and `sum-type-showcase/` overlap with `payment/reference/` and `category-showcase/`.
- **Framework scaffolds** — `shen-hono/`, `shen-fastapi/`, `shen-go-api/`, `shen-go-advanced/`, `shen-rust-axum/` were PROMPT-only scaffolds; the direction is still valid but waits for Wave-4 framework buildout. `FRAMEWORK_EXAMPLES.md` documents the original vision.
- **Domain stubs** — `audit-trail/`, `consensus-quorum/`, `crispr-pipeline/`, `data-pipeline/`, `defi-invariants/`, `feature-flags/`, `relational-constraints/`, `shen-prolog-ui/` are spec-only stubs.
- **Scaffolded-but-never-generated** — `dosage-calculator/` has a Makefile and Go scaffolding but `internal/shenguard/` was never emitted; `email-crud/` has a full Go app but guards live in `reference/` rather than the build hot path.

## Reviving one

To move an archive entry back into rotation:

1. `git mv examples/.archive/<name>/ examples/<name>/`
2. Run `sb gen` if `internal/shenguard/guards_gen.go` is stale.
3. Wire `sb.toml` and the gate commands if it's graduating to Tier A/B.
4. Add a row to the focused set in the top-level `README.md`.
