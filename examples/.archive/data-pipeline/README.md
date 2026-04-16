# Data Pipeline with Schema Evolution

Typed ETL/streaming pipelines where **schema mismatches are type errors**.

## Proof Chain

```
schema-version ─► schema-compatible (same major, minor ≤ target)
                        │
typed-record ──────► stage-input-valid (record schema matches stage)
                        │
stage-contract ────► stage-composable (A's output = B's input)
                        │
checkpoint ────────► resume-valid (new offset > last checkpoint)
```

## Key Invariants

- Cannot process data with wrong schema version
- Cannot compose pipeline stages with incompatible schemas
- Cannot replay already-processed records (exactly-once)
- Dead letter queue entries carry proof of which stage rejected them
- Schema evolution forces downstream adaptation

## Schema Compatibility Rules

- **Minor bump** (1.0 → 1.1): backward compatible, readers accept older
- **Major bump** (1.x → 2.0): breaking change, all stages must update

See `specs/core.shen` for the full specification.
