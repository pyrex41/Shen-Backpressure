Go API example using Shen for formal invariants. Mature, high-performance backend with excellent concurrency (goroutines, channels).

Go offers great performance, static binaries, and simple concurrency primitives. Pairing with Shen allows proving properties like "no invalid state transitions" or "all API responses are grounded".

Uses the payment processor domain (specs/core.shen) as the starting point. Extend with additional invariants as needed.

Stack: Go stdlib net/http or Fiber. No heavy frameworks.

Key value proposition: Go's simplicity might make the Shen layer feel heavier than in dynamic languages, but that's the point — it adds the missing formal layer without runtime cost. Go's real strength is predictable performance and easy ops. Shen adds the formal layer Go's type system can't express on its own.

See prompts/ for details on implementing order state machine API or payment API.

Use /sb:ralph-scaffold to set up the Ralph loop with five-gate backpressure.
