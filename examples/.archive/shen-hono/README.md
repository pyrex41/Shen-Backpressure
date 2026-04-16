# Shen + Hono API Example

This scaffolds an exploration of using **Shen** as the formal specification layer for a **Hono** web API.

## Why this direction?

Hono is one of the fastest and most flexible web frameworks in the JS/TS ecosystem. It runs on multiple runtimes (Node, Bun, Deno, Cloudflare) with minimal overhead. Combined with Shen's sequent-calculus invariants, it gives you:

- Blazing fast HTTP handling
- Formally verified business logic and data invariants
- Generated type guards that prevent invalid states at the boundaries
- Single source of truth in `specs/core.shen` that drives both runtime checks and type safety

## Usefulness

- **API validation & invariants**: Define request payloads, state machines (e.g. order status), authorization rules in Shen. Generate opaque TS types and validators.
- **Backpressure for LLM coding**: Use the Ralph loop with gates: shengen-ts -> tsc -> test -> shen tc+.
- **Edge computing**: Perfect for serverless/edge where you want low latency and high correctness (no bad data propagating).
- **Portability**: The same Shen spec can be used with other backends (Go, Rust) to compare.

**Contrarian perspective (Lucas)**: The obsession with "FastAPI is not performant" misses that for many apps, the framework's DX and ecosystem (OpenAPI docs in FastAPI) might outweigh raw throughput. Performance is easy to fix with caching or scaling; logical errors in payment logic are expensive. This example lets you benchmark not just RPS but also "bug rate" or "incident rate" across stacks.

See `prompts/hono-exploration-prompt.md` for detailed LLM instructions to build this out.

## Structure
- `specs/core.shen` - Formal invariants
- `prompts/` - Detailed prompts for different facets
- `src/` - Hono server implementation using generated guards
