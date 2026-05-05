# Feature Design Prompt: Holographic Mock

You are designing a deep-tech demo and possible product feature for Shen-Backpressure: a Shen specification that acts as a stateful test double for an external system. This is a design task, not an implementation task. Produce an implementation-ready design document, but do not edit code.

## Context

Shen-Backpressure currently uses Shen specs as:

- a deductive source for guard types via shengen
- an oracle for pure function behavior via shen-derive
- an internal consistency gate via Shen typechecking

The unexplored direction: use the same Shen spec as an executable **stateful environment** during target-language tests.

The target-language application still deploys normally. Shen only runs in development/test/CI as an active oracle or mock.

## Goal

Design a **Holographic Mock** feature:

> A target-language app talks to an external system during tests, but that external system is actually a Shen spec running as a stateful oracle.

This should demonstrate that "the spec is not just documentation or generated types; the spec can be the test environment."

## Canonical Demo Candidate

Use `examples/payment/` as the primary design target.

Possible mock domains:

- ledger service
- Stripe-like payment authorization service
- bounded job queue / lease service
- external balance store

The Go app talks to the mock over HTTP, stdio, or IPC. The mock maintains state according to Shen rules and rejects impossible transitions.

Example scenario:

```text
1. Go app asks ledger to reserve $50 from account A.
2. Shen mock checks the balance invariant.
3. If valid, Shen updates symbolic ledger state and returns approved.
4. If invalid, Shen returns insufficient-funds with a trace.
5. Test asserts the Go app handles both cases correctly.
```

## Design Requirements

### 1. Scope

Keep this narrow. Do not design a general simulation operating system.

v0 should support:

- one target-language client
- one Shen-backed mock process
- one state model
- deterministic requests and responses
- clear counterexample/error traces

Avoid:

- distributed simulation
- K8s mocks
- arbitrary database replacement
- production runtime dependency on Shen

### 2. Architecture Options

Evaluate at least three communication patterns:

1. **HTTP local server**
   - Shen/SBCL process exposes a small JSON API.
   - Target-language app points test config at localhost.
   - Most realistic for service mocks.

2. **stdio subprocess**
   - Target-language tests spawn `sb mock`.
   - Requests/responses are newline-delimited JSON.
   - Simple and CI-friendly.

3. **in-process target-language adapter**
   - Target tests call generated adapter functions that shell out to Shen.
   - Lower setup, less realistic boundary.

Recommend one for v0 and explain why.

### 3. Shen State Model

Design how state is represented in Shen.

Address:

- initial state
- transition functions
- query functions
- rejected transitions
- trace output
- deterministic reset between tests
- seed/config input for edge cases

Example shape:

```shen
(define reserve
  Account Amount Ledger -> ...)

(define commit
  Reservation Ledger -> ...)

(define fail
  Reservation Reason Ledger -> ...)
```

The exact syntax can be illustrative. Do not require new Shen language features.

### 4. Mock Protocol

Design the request/response protocol.

Include:

- operation name
- input payload
- current state id or session id
- expected response shape
- error shape
- trace shape

Example:

```json
{
  "op": "reserve",
  "session": "test-001",
  "input": { "account": "acct_1", "amount": 50 }
}
```

Rejected response:

```json
{
  "ok": false,
  "error": "insufficient_funds",
  "trace": [
    "balance(acct_1)=25",
    "amount=50",
    "requires balance >= amount"
  ]
}
```

### 5. Integration With Existing Gates

Design how this participates in `sb gates`.

Consider:

- a new gate kind or ordinary manifest command
- test setup command that starts the mock
- teardown behavior
- artifact capture for traces
- evidence report integration
- counterexample trace integration

The v0 should work as ordinary commands in `sb.toml`; a first-class command can come later.

### 6. Relationship To shen-derive

Explain the difference:

- `shen-derive` checks pure functions pointwise against a Shen oracle.
- Holographic mock checks stateful external interactions against a Shen state machine.

Design how they can share concepts:

- sample generation
- expected/actual traces
- counterexample artifacts
- spec parsing

### 7. Failure Modes

Address:

- mock diverges from real external service
- tests overfit to mock behavior
- Shen mock becomes too slow
- state leaks across tests
- target-language adapter hides real network behavior
- LLM writes code that passes mock but fails against real service

Mitigate by requiring the mock to model only invariants and protocol semantics, not every implementation detail of the external service.

## Guardrails

- Do not pitch this as replacing integration tests against real services.
- Do not require Shen in production.
- Do not make the mock an unbounded platform before one demo works.
- Do not hide rejected transitions; every rejection should produce a trace.
- Do not let the LLM invent mock behavior outside the Shen spec.

## Deliverable

Write a detailed design document with:

1. Problem statement.
2. One chosen v0 demo domain.
3. Architecture recommendation.
4. Shen state model sketch.
5. Mock protocol schema.
6. `sb.toml` integration sketch.
7. Example test flow in Go using `examples/payment/`.
8. Evidence/counterexample artifact integration.
9. Risks and mitigations.
10. Staged roadmap from demo to reusable feature.
