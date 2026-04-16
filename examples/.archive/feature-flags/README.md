# Feature Flags with Safety Proofs

Feature flags where **dependency violations are type errors**.

## Proof Chain

```
feature-dependency ─► dependency-satisfied (dep is enabled)
                            │
                      safe-activation (all deps satisfied)
                            │
env-allowed ───────────► gated-activation (env + deps)
                            │
user-cohort ──────────► user-in-rollout (hash bucket < rollout %)
                            │
                      flag-evaluated (complete proof)
```

## Key Invariants

- Cannot enable a feature without its dependencies enabled
- Experimental features blocked from production
- Gradual rollout bounded [0, 100] with deterministic cohort hashing
- Cannot roll back a feature that other active features depend on

## Use Cases

- Feature dependencies: "new checkout" requires "payment-v2"
- Environment gates: "debug-mode" blocked in production
- Gradual rollout: roll out to 10%, then 50%, then 100%
- Safe rollback: cascade analysis before disabling

See `specs/core.shen` for the full specification.
