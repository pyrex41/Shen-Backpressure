---
date: 2026-04-04T00:00:00Z
researcher: reuben
git_commit: pre-wave-heavy-analysis-2
branch: main
repository: pyrex41/Shen-Backpressure
topic: "Framework-examples synthesis — serving example priorities for shen-rust-axum / shen-hono / shen-go"
tags: [research, synthesis, framework-examples, verification-vs-performance]
status: archived-reference
last_updated: 2026-04-16
last_updated_by: reuben
last_updated_note: "Moved from root heavy_analysis_2.md during demo-readiness cleanup. Framework-starter examples referenced here have since been archived; doc retained as record of the scoping conversation."
---

**Synthesized Recommendation: Opening "Hendoor's Box" for Shen-Verified Serving Examples**

I have reviewed `heavy_analysis.md` (the synthesis on Shen as verifiable interlingua, five-gate/Ralph backpressure loops, sequent-calculus datatypes for invariants/grounding/state machines, and the 10+ exploration directions) plus the existing codebase. This includes the partial scaffolding already present in `examples/` (`shen-hono-api/`, `shen-rust-axum/`, `shen-go-api/`, `shen-fastapi/`, plus the older `payment_*` and `email_crud` patterns that demonstrate `shengen` guard generation).

**Unified view resolving agent perspectives:**
- **Lovelace and Benjamin** correctly diagnose the FastAPI limitations (throughput, tail latency, memory, observability integration, edge fit) and push for systematic performance/ops trade-off exploration. Their proposed stacks (Rust-Axum, Go-Fiber/Echo/Chi, TypeScript-Hono, optimized Python) give excellent contrast.
- **Sappho and Lucas** correctly anchor everything to the project's *core thesis*: Shen as the single source of truth for invariants, grounding rules (`grounded-source`), pipeline states, and business logic. The host runtime (Hono, Axum, Go, FastAPI) is primarily an I/O and performance chassis. Lucas's contrarian point is valuable—**correctness and total cost of ownership (bug/incident reduction) often matter more than raw RPS** for real workloads. Pure performance comparisons risk missing the point.
- **Harper** is right that we must stay grounded in the actual files (the `shen-*` folders already exist with `specs/core.shen`, `prompts/`, and `README.md` files that partially implement the vision).

**Core workload** (answering the repeated question): Verifiable JSON APIs and stateful data pipelines with *enforceable invariants* (validation, authorization, grounding of LLM/output data, order/payment state machines, multi-tenant rules). These are exactly the areas where LLMs hallucinate and traditional type systems fall short. Success is measured in *both* systems qualities (latency, RPS, resource usage, observability) *and* correctness properties (Shen `tc+` passes, zero invalid states at runtime, generated guards catch violations).

**Prioritized first three directions** (maximum contrast, aligned with your desire to move beyond FastAPI while staying true to the Shen vision):
1. **shen-rust-axum** — Maximum performance, safety, low memory, excellent observability (tower/tracing/OpenTelemetry). (Lovelace's strong recommendation.)
2. **shen-hono-api** (Bun/Node/Deno/Cloudflare Workers) — Edge-first, minimal overhead, great DX, multi-runtime flexibility. Already partially scaffolded and directly addresses your Hono mention.
3. **shen-go-api** (net/http + chi or Fiber) — Excellent balance of performance, simplicity, static binaries, and concurrency model. Good middle ground.

**shen-fastapi** remains as the *baseline* for developer experience and data-ecosystem richness (explicitly framed as such in its README, per Lucas/Sappho).

**Prompt and folder standard** (LLM-coder ready):
Each example will use **detailed, step-by-step, "Ralph-loop ready" prompts** (not high-level architectural briefings). These explicitly reference:
- The five-gate backpressure pattern.
- Starting from `specs/core.shen` (copy/adapt invariants from payment/order examples).
- Generating guards via `shengen-*`.
- Implementing idiomatic hot paths in the host framework.
- Adding OpenTelemetry + structured logging + Prometheus from day one.
- Including Docker, docker-compose with k6 load tests, a benchmark script, and a `benchmark.md` section capturing p50/p99/RPS/memory + correctness metrics ("no invalid states leaked", "Shen tc+ passes on all generated code", "before/after bug rate").

**Recommended structure per folder** (already partially present; we will flesh out):
- `README.md` — Why this stack, trade-offs vs FastAPI, usefulness narrative (correctness + perf story), success criteria.
- `prompts/` — Layered prompts (`architecture-prompt.md`, `implementation-prompt.md`, `benchmark-prompt.md`, `llm-refinement-prompt.md`).
- `specs/core.shen` — Invariants, datatypes, state machine rules.
- `src/` (or equivalent) — Entrypoint + generated guards.
- `Dockerfile`, `docker-compose.yml`, basic benchmark harness, OpenTelemetry config.

This creates a true "Hendoor's box"—a systematic, comparable suite rather than being anchored to any single suggestion from the past.

**On broader exploration**: The 10+ directions in `heavy_analysis.md` (Shen as control plane for Hono/FastAPI, provable backpressure primitives via linear/graded modalities, "Shen-light" practitioner layer, hybrid embedding, etc.) are the real prize. Performance comparisons are useful scaffolding, but the killer demonstration is the *same* Shen spec producing correct, auditable implementations across stacks with dramatically lower logical error rates.

**Next steps I recommend**:
1. Fully flesh out the three prioritized folders with richer prompts and complete scaffolding (building on what Sappho/Lucas already started).
2. Add a top-level `examples/README.md` or `framework-examples-overview.md` that ties them together rhetorically (per Sappho) while including quantitative comparison tables (per Lovelace/Lucas).
3. Run benchmarks + correctness validation across them.

This approach balances your performance concerns with the project's deeper mission of high-assurance, LLM-tamed development. It is useful for both practitioners (pick the right chassis) and researchers (prove the interlingua value).

Ready to proceed—confirm the three priorities (or adjust), and I will use the available tools/commands (`/plan`, `/implement`, `write`, `edit`, etc.) to create/enhance the specific files and prompts.

**Confidence**: 85% on overall direction and structure (strongly grounded in `heavy_analysis.md` + existing code); 70% on exact performance numbers (will require actual benchmarking).
