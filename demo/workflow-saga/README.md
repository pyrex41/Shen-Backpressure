# Workflow Saga — Distributed Transaction Proofs

The Saga pattern where **compensation without execution is a type error**.

## Proof Chain

```
step-completed (forward execution proof)
       │
       ├──► step-ordered (A runs before B)
       │
       ├──► saga-completed (ALL steps have proofs)
       │
       └──► step-compensated (requires completed proof)
                   │
              compensation-ordered (reverse order enforced)
                   │
              saga-rolled-back (all completed steps compensated)
```

## Key Invariants

- Cannot compensate a step that never executed
- Compensation must happen in reverse order
- Saga completes only when ALL steps produce proofs
- Idempotency keys prevent double-execution
- Deadline proofs enforce timeout constraints

## Use Cases

- E-commerce: reserve inventory → charge payment → ship order
- Banking: debit account A → credit account B → record transfer
- Microservices: any multi-service transaction with rollback needs

See `specs/core.shen` for the full specification.
