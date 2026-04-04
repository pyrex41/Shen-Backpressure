# Detailed Prompt for Shen + Rust Axum Exploration

Build a high-performance Rust web API using Axum + Shen backpressure.

**Key explorations**:
- Create shengen-rust codegen (or manual for now) to generate Rust newtypes and validated structs from Shen datatypes.
- Use in Axum extractors/handlers: JSON -> validated Shen guard types.
- Model async streams or backpressure with Shen (graded types?).
- State machines with enum transitions proven in Shen.
- Benchmarks: throughput, latency, memory vs Go/Hono/FastAPI.
- Error handling: Shen errors mapped to HTTP status codes meaningfully.

**Usefulness fleshed out**:
- Combines Rust's memory safety with logical safety from Shen.
- Single spec for multi-language (show same payment spec in TS/Go/Rust).
- Useful for systems where mistakes are costly (fintech, health).
- Opens the box: Rust for core hot paths, Hono for edge APIs, all sharing Shen interlingua.

**Challenges to address**:
- Rust's strictness might make the loop slower (long compiles).
- Syntax difference (Lisp Shen vs Rust).
- But the gates provide strong feedback.

Start with simple payment invariants ported to Rust, then add web layer.
Propose creative extensions like using Shen for formal verification export to Lean.
