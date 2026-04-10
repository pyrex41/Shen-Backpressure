# Payment Processor Demo

Demonstrates Shen-Backpressure with a payment processor domain.

**Invariant**: Balance can never go negative through any sequence of transfers.

## Quick Start

### 1. Install Shen-Go (one-time)

```bash
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp shen ../../examples/payment/bin/
```

### 2. Run

```bash
cd examples/payment

# Build everything and run all gates
make all

# Run demo mode (single iteration, shows all gates passing)
make demo

# Run orchestrator in strict sequential mode
make run

# Run in relaxed parallel mode
make run-relaxed
```

### 3. Expected output

```
15:43:08 [ralph] Starting Ralph-Shen loop (mode=strict)
15:43:08 [ralph] Tooling validated: go=OK, specs=OK, shen=OK
15:43:08 [ralph] === Iteration 1 ===
15:43:09 [ralph] PASS [go-test]
15:43:09 [ralph] PASS [go-build]
15:43:09 [ralph] PASS [shen-typecheck]
15:43:09 [ralph] All gates passed on iteration 1
```

## What's Here

```
├── cmd/ralph/main.go          # Go orchestrator — runs the loop
├── bin/shen-check.sh          # Shen subprocess wrapper
├── specs/core.shen            # Shen formal type specifications
├── src/payment/
│   ├── processor.go           # Balance invariant enforcement
│   └── processor_test.go      # 8 tests including invariant test
├── prompts/main_prompt.md     # LLM instruction template
├── plans/fix_plan.md          # Dynamic task plan
├── Makefile                   # build / test / shen-check / demo
└── go.mod
```

## What gets checked

`make all` runs four checks:

- **`go build`** — does the code compile?
- **`go test ./...`** — do the specific test cases pass?
- **`./bin/shen-check.sh`** — do the Shen sequent-calculus type proofs hold for all inputs?
- **`make shen-derive-verify`** — does `internal/derived/Processable` still match the `(define processable ...)` spec in `specs/core.shen`?

Each check is a separate kind of evidence. Tests cover the specific
cases the author thought of. `go build` catches type mismatches from
spec changes. `shen tc+` proves the domain invariants hold for all
possible inputs (the sequent-calculus rules in `specs/core.shen`).
`shen-derive verify` pins the hand-written `Processable` loop against
a Shen `(define ...)` oracle on sampled inputs, and fails the build
if the committed generated test drifts from what the current
spec+sampler would produce.

A full `sb gates` run (outside this example's `make all`) also
regenerates guard types via `shengen` and audits the TCB. See the
root README for the canonical pipeline — this example focuses on
the subset relevant to the balance invariant.

### How `shen-derive verify` works here

`specs/core.shen` contains a `(define processable ...)` block that
expresses the obvious-correct version of the balance-check as a fold
over the running balances. `shen-derive verify` evaluates that spec
on a set of sampled inputs (boundary values by default, optionally
plus seeded random draws) and emits
`internal/derived/processable_spec_test.go` — a table-driven test
that calls the real implementation and asserts pointwise equality
against the spec's outputs.

The committed copy of that test file is the drift gate. Changing
`processable.go`, the spec, or the sampling strategy without
regenerating the test file fails the check. `make shen-derive-regen`
(or `sb derive --regen`) rewrites it.

See `sb.toml` for the `[[derive.specs]]` entry and
`../../shen-derive/DESIGN.md` for how the harness builds its samples
and evaluation environment.

## Shen Type Specs

The key rule in `specs/core.shen`:

```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

This proves that a `[Balance Transaction]` pair is only `balance-checked` if the balance covers the transaction amount — for *all* possible values, not just test cases.
