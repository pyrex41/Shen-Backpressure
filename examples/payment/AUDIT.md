# Payment Processor — Auditor Workflow

> One-page reviewer workflow for the canonical Shen-Backpressure
> demo. Read [`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md) first
> for the project-level trust model.

## What this demo proves

A payment processor with one invariant:
**a balance can never go negative through any sequence of transfers.**

The invariant is encoded twice:

- **Structural** — `specs/core.shen` declares `balance-checked` as
  a sequent rule whose premise is `(>= Bal (head Tx))`. The
  generated `BalanceChecked` Go type's constructor enforces this
  predicate; `SafeTransfer` demands a `BalanceChecked` parameter,
  so the language compiler refuses to call it without the
  precondition holding.
- **Behavioral** — `(define processable ...)` is the spec for a
  fold over the transaction list. `shen-derive` evaluates it as an
  oracle on a deterministic boundary pool and pins the hand-written
  Go `Processable` against it.

## Auditor steps

### 1. Verify the spec hash matches the committed file

```bash
cd examples/payment
sha256sum specs/core.shen
```

Compare the output to the `spec.files[].sha256` field in
`transcript/discharge_report.json` and `transcript/audit_report.md`.
If they disagree, the committed audit artifact is stale relative to
the spec — re-run gates before continuing.

### 2. Read the rendered audit report

Open `transcript/audit_report.md`. The file is committed and
viewable directly on GitHub. It contains:

- **Spec hash and git commit** the report was produced against
- **Per-rule discharge tables** showing each premise's
  classification (`static` / `runtime-sample` / `unproven`) and the
  basis for the classification
- **Counter-examples** for any failed premise (with `case_id`,
  `spec_output`, `impl_output`, and a ready-to-paste
  `go test -run …` reproducer)
- **`discharged_since_commit`** per rule — the earliest commit in
  `.sb/history/` at which this invariant was discharged in the same
  category. The audit answer to "how long has this invariant held?"
- The **"How to read this report" appendix** with the canonical
  discharge-category glossary

### 3. Re-run the gates at the recorded commit

```bash
git checkout <git_commit-from-transcript/discharge_report.json>
cd examples/payment
../../bin/sb gates
```

Expected output (6/6 with `shen-sbcl` installed, 5/6 without):

```
PASS  shengen        ~90ms
PASS  test           ~120ms
PASS  build          ~210ms
PASS  shen-check     ~2ms      ← needs shen-sbcl; otherwise FAIL
PASS  tcb-audit      ~15ms
PASS  shen-derive    ~160ms
```

The just-generated `.sb/discharge_report.json` should agree with
`transcript/discharge_report.json` modulo the `generated_at`
timestamp. Diff to confirm:

```bash
jq 'del(.generated_at)' .sb/discharge_report.json > /tmp/a.json
jq 'del(.generated_at)' transcript/discharge_report.json > /tmp/b.json
diff /tmp/a.json /tmp/b.json   # expected: empty
```

### 4. Read the TCB

For this demo, the TCB is small:

| Piece | Where | Why it's TCB |
|---|---|---|
| The validating constructors in `internal/shenguard/guards_gen.go` | Generated from spec | The lowering of Shen predicates to Go must be correct. `tcb-audit` catches post-generation tampering. |
| The hand-written `Processable` impl in `internal/derived/processable.go` | Hand-written | Pinned by `shen-derive`'s sampled test, but the pool is finite. A bug outside the pool slips past. |
| The `Amount`, `Balance`, `Transaction` boundary parsing wherever they enter the process | App code | Garbage-in still produces garbage-out — the chain protects against in-process tampering, not against malformed inputs at the I/O boundary. |

The discharge report tells you which premises are `static` (the
compiler enforces them) and which are `runtime-sample` (sampled
evidence). For this demo, the `balance-invariant` premise is
`static`; the `Processable` rule's behavioral premise is
`runtime-sample`.

### 5. Watch a failure mode

```bash
./demo-shen-derive/run.sh
```

This script swaps in three deliberately-buggy `processable.go`
implementations in turn and re-runs `shen-derive verify`. Each
bug compiles cleanly (the structural types still hold) and is
caught by the sampled spec-equivalence test. The discharge report
flips from `7/7 rules discharged` to `6/7` with a populated
`counter_examples` array.

This is the demo for "tests pass, types pass, the impl still
diverges from the spec — and the gate catches it."

## What this demo does NOT claim

- That the boundary pool is exhaustive. It is designed to hit edge
  cases (zero, one, negative, empty list, etc.); inputs outside
  the pool are not verified.
- That the `Amount` boundary is hardened against, e.g.,
  `math.NaN()` injection at the network boundary. The Shen spec
  declares `Amount : number` with `(>= X 0) : verified`; the Go
  predicate `x >= 0` evaluates as the language defines it, so
  `NaN >= 0` is `false` and rejected. But the parser that produces
  the `float64` is outside this demo's scope.
- That the `SafeTransfer` type system prevents non-balance bugs.
  It prevents calling `SafeTransfer` without a balance-checked
  proof; it does not prevent, e.g., double-counting in the calling
  loop. The structural slice is named explicitly.

For the full project-level trust model, see
[`docs/TRUST-MODEL.md`](../../docs/TRUST-MODEL.md).

## Further reading

- `README.md` — five-minute walkthrough
- `demo-shen-derive/DEMO.md` — long-form bug walkthrough
- `transcript/discharge_report.json` and `transcript/audit_report.md`
  — the committed audit artifact (committed in parallel via the
  `claude/audit-artifacts` worktree)
- `../../docs/TRUST-MODEL.md` — project-level trust model
