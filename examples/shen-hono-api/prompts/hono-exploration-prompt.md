# Detailed Prompt for Shen-Hono Exploration

You are building a demonstration of Shen + Hono for high-assurance web APIs.

**Core Idea**: All business logic, validation rules, and invariants live in Shen datatypes (`specs/core.shen`). `shengen-ts` (extend if needed) generates TypeScript guard classes. The Hono app uses only these guards at handler boundaries. Hot paths stay idiomatic Hono.

**Domain**: Simple payment / order API with invariants like:
- Amounts must be positive
- Transfers require sufficient balance proof
- Order state transitions must follow legal graph (no invalid states)
- Requests must be grounded (e.g. valid user session)

**Tech Stack**:
- Hono for routing/middleware
- Bun or Deno runtime for max perf (benchmark vs FastAPI)
- Zod or custom validators derived from Shen if possible
- TypeScript with strict mode

**Ralph Loop Gates** (adapt the 5-gate pattern):
1. shengen-ts: regenerate guards from specs
2. Type check (tsc or bun typecheck)
3. Tests (vitest or bun test) - test invariants
4. Shen tc+ on specs
5. (Optional) Runtime load test or property-based test

**Tasks to scaffold and explore**:
- Create `specs/core.shen` with API-specific datatypes (RequestPayloads as sum types, ValidatedOrder, etc.)
- Implement Hono routes that consume/produce only Shen guard types
- Demonstrate how changing the spec forces updates to handlers (backpressure)
- Add middleware that enforces Shen-derived rules
- Benchmark latency/throughput vs a FastAPI equivalent
- Document onboarding: how a non-Shen expert can use the generated guards

**Creative reframings**:
- Shen as the "policy engine" or "control plane" for the API surface.
- Instead of one framework, show the same spec used with Hono, FastAPI, and Axum to highlight interlingua value.
- Explore Hono's middleware composition with Shen guards for auth, rate-limiting (linear logic?).

**Devil's advocate**: Is Hono overkill? Would a simple stdlib server in Go or Rust be better for perf? Or is the JS ecosystem's deployment story (Vercel, Cloudflare) the real win? Challenge the assumption that 'fastest' framework wins; measure total cost of ownership including correctness.

Build incrementally. Always update the Shen spec first when adding concepts. Never edit generated guards directly.

Start by drafting the Shen spec for the domain and present it.
