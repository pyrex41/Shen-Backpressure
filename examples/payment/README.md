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

## The Three Gates

1. **`go test ./...`** — Do the specific test cases pass?
2. **`go build ./cmd/ralph`** — Does the code compile?
3. **`shen (tc +)`** — Do the sequent-calculus type proofs hold for all inputs?

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
