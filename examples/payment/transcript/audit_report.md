# Discharge Report — Audit Rendering

Generated 2026-09-22T04:53:12Z. Source artifact: `transcript/discharge_report.json` (schema_version=1).

**Implementation commit:** `db919d64e3aa7d9c1afb8993149390868602eeef` (working tree dirty)

**Spec files:**

- `specs/core.shen` (sha256 `51f38561a5106b251086bfc5f5f68430a27a4c400c6cae5474c6020f2bde2607`)

**Target languages:** go

## Tool Versions

| Tool | Version |
|---|---|
| sb | 0.3.0 |
| shen-derive | 0.3.0 |
| shengen | shengen 0.3.0 |
| shen runtime | shen-derive-eval |

## Toolchain

| Component | Version |
|---|---|
| Go | `go1.24.7` |
| platform | `linux/amd64` |
| shengen | `shengen 0.3.0` |
| shengen sha256 | `4a7d38acf282002e50cc85431a73e838edb0e659514e6a26943b85858b106088` |
| z3 | `Z3 version 5.1.0 - 64 bit` |

The shengen hash matters because the guard types are a pure function of the spec bytes and that binary. Re-run the same emitter on the same spec and you get the same guards file, byte for byte, from any directory.

## Signature

**Unsigned.** The `signature` field is null. Nobody has attested that this document came out of the pipeline it describes. The claims can still be re-derived (see below) — a signature says who produced a report, not whether it is true.

## Summary

- **Rules:** 7 total — 7 discharged, 0 violated, 0 unproven
- **Premises:** 14 total — 12 static, 1 runtime-evaluator, 1 runtime-sampled, 0 unproven
- **Weakest evidence anywhere in this report:** runtime

## Rules

### `account-id` — wrapper (✅ Discharged)

A account-id value is a string with no further runtime constraints; the type exists to keep raw strings from being mistaken for one. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype account-id
  X : string;
  ====================
  X : account-id;)
```

Continuously discharged since commit `505c6c6dba9b6a148e778a41eb924fbdd59e50ee`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `account-id.field-x` | `X : string` | **static** | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `account-id.field-x` code references: `internal/shenguard/guards_gen.go:54`

### `account-state` — composite (✅ Discharged)

A account-state bundles 2 fields (Id, Balance) into a single typed value. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype account-state
  Id : account-id;
  Balance : amount;
  ====================
  [Id Balance] : account-state;)
```

Continuously discharged since commit `505c6c6dba9b6a148e778a41eb924fbdd59e50ee`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `account-state.field-id` | `Id : account-id` | **static** | static | guard-type-at-boundary | Id is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |
| `account-state.field-balance` | `Balance : amount` | **static** | static | guard-type-at-boundary | Balance is typed amount; values of that type can only be constructed via shengen's guarded constructor, which enforces all of amount's premises transitively. |

- `account-state.field-id` code references: `internal/shenguard/guards_gen.go:142`
- `account-state.field-balance` code references: `internal/shenguard/guards_gen.go:142`

### `amount` — constrained (✅ Discharged)

A amount value is a number that satisfies 1 additional constraint(s) checked at construction. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)
```

Continuously discharged since commit `505c6c6dba9b6a148e778a41eb924fbdd59e50ee`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `amount.field-x` | `X : number` | **static** | static | guard-type-at-boundary | X is typed number; the target language's type system rejects non-number values at construction. |
| `amount.verified-x-0` | `(>= X 0) : verified` | **runtime** | runtime-evaluator | runtime-via-evaluator | amount is evaluated at runtime by the shen-derive evaluator against the spec expression (>= X 0); the spec and the runtime check are the same source. |

- `amount.field-x` code references: `internal/shenguard/guards_gen.go:68`
- `amount.verified-x-0` code references: `internal/shenguard/guards_gen.go:68`

### `balance-invariant` — guarded (✅ Discharged)

A balance-invariant is a multi-field structure whose constructor enforces 1 cross-field invariant(s). *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  ====================
  [Bal Tx] : balance-checked;)
```

Continuously discharged since commit `505c6c6dba9b6a148e778a41eb924fbdd59e50ee`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `balance-invariant.field-bal` | `Bal : number` | **static** | static | guard-type-at-boundary | Bal is typed number; the target language's type system rejects non-number values at construction. |
| `balance-invariant.field-tx` | `Tx : transaction` | **static** | static | guard-brand-bound | shengen's brand inference gives BalanceChecked[B] a phantom brand parameter shared with this premise, so `NewBalanceChecked` accepts only evidence minted for the same subject. The premise `Tx : transaction` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule balance-invariant.) |
| `balance-invariant.verified-bal-head-tx` | `(>= Bal (head Tx)) : verified` | **static** | static | guard-constructor-validates | shengen's generated constructor for balance-invariant rejects inputs that do not satisfy (>= Bal (head Tx)), so this premise holds for any value of type balance-invariant reachable in the impl. |

- `balance-invariant.field-bal` code references: `internal/shenguard/guards_gen.go:117`
- `balance-invariant.field-tx`: proof binding — the constructor's signature is `BalanceChecked[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `balance-invariant.field-tx` code references: `internal/shenguard/guards_gen.go:117`, `internal/shenguard/guards_gen.go:NewBalanceChecked`
- `balance-invariant.verified-bal-head-tx` code references: `internal/shenguard/guards_gen.go:117`

### `processable` — define (✅ Discharged)

A pure function processable : amount --> (list transaction) --> boolean. The Shen spec is the oracle; the impl is asserted to match it on every sampled input. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> ...)
```

Continuously discharged since commit `505c6c6dba9b6a148e778a41eb924fbdd59e50ee`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `processable.oracle-spec-equiv` | `spec(processable) ≡ impl(Processable) on sampled inputs` | **path-cover** | runtime-sample | prover-z3-path-cover | shen-derive symbolically executed the spec (list unroll depth 4), enumerated 10 path(s), and used z3 to find a concrete witness for each of the 9 feasible one(s) (1 dead, 0 undecided). Those 9 witness(es) plus the boundary pool make up the 44 committed cases; the emitted Go test asserts impl returns the spec's value on each. |


- `processable.oracle-spec-equiv`: sampled 44 cases (seed: deterministic-default); 44 passed, 0 failed.
- `processable.oracle-spec-equiv`: path cover — 10 path(s) enumerated, 9 feasible (one committed sample each), 1 dead (unsatisfiable path condition), 0 undecided.
  A dead path is a branch of the spec no input can reach — worth a look from the spec author.

### `safe-transfer` — composite (✅ Discharged)

A safe-transfer bundles 2 fields (Tx, Check) into a single typed value. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype safe-transfer
  Tx : transaction;
  Check : balance-checked;
  ====================
  [Tx Check] : safe-transfer;)
```

Continuously discharged since commit `505c6c6dba9b6a148e778a41eb924fbdd59e50ee`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `safe-transfer.field-tx` | `Tx : transaction` | **static** | static | guard-brand-bound | shengen's brand inference gives SafeTransfer[B] a phantom brand parameter shared with this premise, so `NewSafeTransfer` accepts only evidence minted for the same subject. The premise `Tx : transaction` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule safe-transfer.) |
| `safe-transfer.field-check` | `Check : balance-checked` | **static** | static | guard-brand-bound | shengen's brand inference gives SafeTransfer[B] a phantom brand parameter shared with this premise, so `NewSafeTransfer` accepts only evidence minted for the same subject. The premise `Check : balance-checked` is therefore discharged by proof binding, not merely by type: a proof of the right type about the wrong value is a compile error, because the brand is an unexported type the caller cannot name. (Rule safe-transfer.) |

- `safe-transfer.field-tx`: proof binding — the constructor's signature is `SafeTransfer[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `safe-transfer.field-tx` code references: `internal/shenguard/guards_gen.go:165`, `internal/shenguard/guards_gen.go:NewSafeTransfer`
- `safe-transfer.field-check`: proof binding — the constructor's signature is `SafeTransfer[B]`, so the brand parameter forces this premise to be evidence about the *same subject* as the conclusion. A proof of the right type about the wrong value does not compile.
- `safe-transfer.field-check` code references: `internal/shenguard/guards_gen.go:165`, `internal/shenguard/guards_gen.go:NewSafeTransfer`

### `transaction` — composite (✅ Discharged)

A transaction bundles 3 fields (Amount, From, To) into a single typed value. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ====================
  [Amount From To] : transaction;)
```

Continuously discharged since commit `505c6c6dba9b6a148e778a41eb924fbdd59e50ee`.

**Premises**

| ID | Expression | Precision | Discharge | Basis | Rationale |
|---|---|---|---|---|---|
| `transaction.field-amount` | `Amount : amount` | **static** | static | guard-type-at-boundary | Amount is typed amount; values of that type can only be constructed via shengen's guarded constructor, which enforces all of amount's premises transitively. |
| `transaction.field-from` | `From : account-id` | **static** | static | guard-type-at-boundary | From is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |
| `transaction.field-to` | `To : account-id` | **static** | static | guard-type-at-boundary | To is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |

- `transaction.field-amount` code references: `internal/shenguard/guards_gen.go:89`
- `transaction.field-from` code references: `internal/shenguard/guards_gen.go:89`
- `transaction.field-to` code references: `internal/shenguard/guards_gen.go:89`


## How to Verify This Report

Run this in the project directory. It needs no model, no network, and no credentials — only the committed artifacts:

```sh
sb verify-report --in transcript/discharge_report.json
```

It re-hashes every spec, re-runs shengen and diffs the result against the committed guards file, resolves every code reference, re-runs the committed sample tests, re-checks the path counters when z3 is present, and re-evaluates the flow premises from a freshly built index. A check it cannot re-derive is reported UNVERIFIED rather than passed — add `--strict` to treat that as a failure. When a check fails, it names the premises that lost their basis.

## How to Read This Report

This report categorises every premise of every Shen rule by **how**
it was discharged in the implementation under verification.

- **Static** — the target language's type system (Go's static
  typing, applied to shengen's generated guard types) prevents the
  premise from being violated. A premise typed at the function
  boundary cannot be reached with a non-conforming value because the
  compiler refuses to build such a call site. `guard-type-at-boundary` and
  `guard-constructor-validates` are the two static bases this
  release emits.

- **Runtime-sampled** — shen-derive evaluates the Shen spec on a
  deterministic boundary pool (and, when seeded, additional random
  draws) and emits a Go test asserting that the implementation
  returns the same value on every sampled input. A "discharged"
  premise here means *every sampled case agreed*. This is sampled
  evidence, not an exhaustive proof.

- **Path cover** — when the premise's basis is
  `prover-z3-path-cover`, the evidence is stronger than a pool.
  shen-derive symbolically executed the Shen spec, enumerated every
  execution path (unrolling list recursion to a fixed depth), and used
  the Z3 solver to produce one concrete input per *feasible* path.
  Those inputs are committed as test cases alongside the boundary
  pool. Paths whose condition is unsatisfiable are reported as dead:
  branches of the spec no input can reach. This is still bounded
  evidence — the list-unrolling depth is finite — but within that
  bound no path of the spec goes unexercised.

- **Vacuous** — the rule's datatype is uninhabited: the conjunction of
  its verified premises has no solution, so no value of the type can
  be constructed and every claim that consumes one is empty. This is
  a defect in the spec rather than in the implementation, and it
  fails the gate, because an uninhabited guard proves nothing while
  looking like it proves everything.

- **Precision** — each premise also carries a `precision` on a
  total order: `static` (the compiler refuses a violating
  program) is strongest, then `path-cover` (a solver found a
  witness for every feasible spec path), then `sampled`
  (agreement on a pool of inputs), then `runtime` (checked in
  production, on the value in hand, and silent about every value the
  program never sees), then `unproven`. A report is only as
  strong as its weakest premise, which is why the Summary states it.

- **Blame** — each counter-example names one responsible party:
  `spec` (the Shen rule is wrong or uninhabited), `impl`
  (the implementation disagrees with a spec both oracles read the same
  way), `wrapper` (a `:runtime-via` checker), or
  `lowering` (the spec's two evaluators disagree about what it
  means). The `blame_basis` says how the assignment was
  reached; `evaluator-only` means no Shen host was available to
  offer a second reading, so a lowering bug would look identical.

- **Unproven** — the tool could not confidently classify the premise
  in this release. Treat the premise as outside the verified
  boundary until a future version of the tool can address it.

**What this report does not claim**

- It is not a SOC-2, ISO-27001, or any other compliance certification.
  It is a verification artifact that compliance and audit workflows
  may reference as evidence.
- It is not third-party attested. The `signature` field, when
  present, says which key vouched that this document came out of this
  pipeline. It is not a claim that the pipeline's conclusions are
  correct — for that, re-derive them with `sb verify-report`,
  which needs neither the key nor the network. See the Signature
  section above, and docs/TRUST-MODEL.md for what signing does and
  does not move inside the trust boundary.
- It is not third-party verified. The classifications and rationales
  come from this tool's own analysis of the spec and the
  implementation.

**Reproducing this report**

The discharge report is produced as a side effect of every successful
`sb gates` (or `sb derive`) run. Run the gate pipeline against the
same spec and the same git commit recorded in this report and you
will get a byte-identical artifact (modulo the `generated_at`
timestamp). Time-stamped copies accumulate under `.sb/history/`.

For per-case input detail, open the generated test file referenced
in the spec's manifest and look for the matching `case_NN` entry.
