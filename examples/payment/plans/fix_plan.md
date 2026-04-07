# Fix Plan — Payment Processor

## Goal
Build a payment processor with formal balance invariants enforced by Shen types.

## Completed
- [x] Project scaffolding
- [x] Core Shen type definitions (account, transaction, balance-invariant)
- [x] Go orchestrator skeleton

## In Progress
- [ ] Payment processor domain logic (src/payment/)
- [ ] Integration: orchestrator calls LLM + applies changes + validates

## Backlog
- [ ] Add transfer history tracking with Shen proofs
- [ ] Add multi-currency support with type-level currency tags
- [ ] Add concurrent transfer safety invariants
- [ ] Stress test: 100 iterations without human intervention
