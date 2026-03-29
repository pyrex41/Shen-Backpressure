E-commerce order lifecycle state machine in Go where every state transition is proven valid and every reachable state has at least one outward transition (no deadlocks). The Shen spec encodes the transition graph; shengen generates guard types that make illegal transitions a compile error.

Stack: Go stdlib net/http, SQLite, htmx frontend for order management. No frameworks.

Domain entities:
- Orders with current state and history
- States: created, paid, processing, shipped, delivered, cancelled, refund-requested, refunded
- Transitions between states with preconditions

Invariants:
- A state transition is only valid if (allowed-transition? from to) holds — the valid pairs are:
  - created → paid, created → cancelled
  - paid → processing, paid → refund-requested
  - processing → shipped, processing → cancelled
  - shipped → delivered, shipped → refund-requested
  - refund-requested → refunded
  - No transitions out of delivered, refunded, or cancelled (terminal states OK — exempt from live-state)
- A state change requires the order's current state to match the transition's "from" state
- Every non-terminal state must have at least one valid outward transition (no deadlocks)
- An order's state history is append-only — transitions are recorded, never deleted

Operations:
- POST /orders → create order (state: created)
- POST /orders/:id/pay → transition created → paid
- POST /orders/:id/ship → transition processing → shipped
- POST /orders/:id/cancel → transition (created|processing) → cancelled
- POST /orders/:id/refund → transition (paid|shipped) → refund-requested → refunded
- GET /orders/:id → show order with full state history
- Dashboard showing orders by state with available actions

Use /sb:ralph-scaffold to set up the Ralph loop with five-gate backpressure, then run the loop to build it out.
