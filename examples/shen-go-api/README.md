# Shen + Go API Example (Fiber or net/http)

**Direction**: Mature, high-performance backend with excellent concurrency (goroutines/channels).

Go offers great performance, static binaries, and simple concurrency primitives. Pairing with Shen allows proving properties like "no invalid state transitions" or "all API responses are grounded".

**Usefulness**:
- Excellent for microservices and CLI tools too.
- Channels and context can be modeled with Shen linear types for resource protocols.
- shengen (Go version) already exists and works well.
- Battle-tested in production.

**Contrarian take**: Go's simplicity might make the Shen layer feel heavier than in dynamic languages, but that's the point — it adds the missing formal layer without runtime cost. Everyone focuses on "fast", but Go's real strength is predictable performance and easy ops. What if we use Shen to specify concurrency invariants that prevent common Go pitfalls like races (though Go has race detector too).

Compare to Hono: deployment is different (Docker/K8s vs edge).

See prompts/ for details on implementing order state machine API or payment API.
