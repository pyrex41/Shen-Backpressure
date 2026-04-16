# Shen + Rust (Axum) Example

**Direction**: Maximum performance and safety using Rust's type system + Shen's logical types.

Rust (with Axum or Actix) is known for zero-cost abstractions and fearless concurrency. Shen adds higher-level logical invariants that Rust's borrow checker doesn't cover (business rules, state machines).

**Usefulness**:
- Compile-time enforcement pushed even further (Shen proofs + Rust types).
- Great for high-throughput services (finance, gaming, infra).
- WASM possible for edge too.
- Very low memory usage.

**Lucas' contrarian view**: Rust has a steep learning curve. The combination with Shen (which has Lisp syntax) might be too alien for most teams. However, if we can generate most of the boilerplate and guards, it lowers the bar. Maybe the missing piece is a visual spec tool or "Shen-light" layer. Everyone assumes perf is king, but for most apps developer velocity and correctness matter more. This example can benchmark "time to correct implementation" using the Ralph loop vs traditional.

Potential to extend shengen to Rust target.

**Prompts** explore payment processor or API with strong typing.
