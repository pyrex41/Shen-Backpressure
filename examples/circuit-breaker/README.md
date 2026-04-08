# Circuit Breaker & Resilience Patterns

State machine for circuit breakers where **invalid transitions are type errors**.

## Proof Chain

```
failure-count ─► threshold-breached ─► open-circuit
                                            │
                        cooldown-elapsed ◄──┘
                              │
                     half-open-circuit
                        │          │
               probe-success   probe-failure
                    │                │
            closed-circuit      open-circuit (re-trip)
```

## Composable Resilience

Three independent guards compose into one:

```
closed-circuit ─┐
rate-allowed ───┼─► resilience-cleared
bulkhead-permit─┘
```

A request can only proceed if ALL three proofs exist.

## Key Invariants

- Cannot open circuit without enough failures
- Cannot probe without cooldown elapsed  
- Cannot send requests through open circuit
- Rate limiting and bulkhead are statically proven

## Usage

```bash
# Generate guard types
shengen -spec specs/core.shen -out shenguard/
```

See `specs/core.shen` for the full specification.
