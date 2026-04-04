# Detailed Prompt for Shen + Go (or Fiber) API

Explore using Shen specs with a Go web server.

**Options to explore**:
- Standard library net/http
- Fiber (fast Express-like)
- Gin

**Focus on**:
- Generating guards with existing shengen
- Using guards in HTTP handlers (JSON unmarshal -> NewXXX() constructor)
- State machine for orders or multi-tenant access control
- Backpressure loop using Ralph (already prototyped in other demos)

**Flesh out**:
- How Shen can specify not just data but API contracts (e.g. response must contain certain fields based on input state)
- Benchmarks against Hono and FastAPI
- Usefulness in enterprise: auditability of invariants, easier compliance.

**Alternative hypothesis**: The performance difference between these frameworks is often <10% in real workloads. The real differentiator is how easily the formal spec integrates and how much it reduces production bugs. Measure that.

Draft specs/core.shen for an e-commerce API with order and payment invariants.
