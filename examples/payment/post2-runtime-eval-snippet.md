# Post-2 Runtime Backpressure Artifact (payment example)

**Spec** (`specs/core.shen`):
```shen
(datatype amount
  X : number;
  (>= X 0) : verified; \* :runtime-via :eval *\
  ====================
  X : amount;)
```

This uses **profile B** (`:runtime-via :eval`).

At construction time, `NewAmount(ctx, x)` does **not** inline the predicate.
Instead it calls the embedded shen-derive evaluator host:

```go
ok, err := evalhost.Check(ctx, "(>= X 0)", []string{"X"}, x)
```

**Why this matters for post 2**:
- The *exact same expression* that appears in the Shen spec is what runs at runtime.
- No hand-written Go version that can drift.
- Compile-time guard (you must go through `NewAmount`) + runtime evaluation from the spec itself.
- Complements the multi-tenant-api story (which uses a named bespoke checker, profile A).

See:
- `src/payment/processor.go` (CreateAccount and transfer paths)
- `src/payment/processor_test.go`
- `internal/derived/processable_spec_test.go`
- shen-derive/runtime/evalhost/host.go

This + the multi-tenant `:runtime-via` named checker work together to show different points on the "runtime backpressure from one Shen spec" spectrum.

Status: Solid and already exercised in the existing tests and derive reports.