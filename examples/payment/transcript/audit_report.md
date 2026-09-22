# Discharge Report — Audit Rendering

Generated 2026-09-22T04:52:40Z. Source artifact: `.sb/discharge_report.json` (schema_version=1).

**Implementation commit:** `22d7a3e149ce95f82655b3c53c701383ccf32062`

**Spec files:**

- `specs/core.shen` (sha256 `51f38561a5106b251086bfc5f5f68430a27a4c400c6cae5474c6020f2bde2607`)

**Target languages:** go

## Tool Versions

| Tool | Version |
|---|---|
| sb | 0.3.0 |
| shen-derive | 0.3.0 |
| shengen | — |
| shen runtime | shen-derive-eval |

## Summary

- **Rules:** 7 total — 7 discharged, 0 violated, 0 unproven
- **Premises:** 14 total — 12 static, 1 runtime-evaluator, 1 runtime-sampled, 0 unproven

## Gate Strength

A discharge says a gate passed. This section says how much that is worth, by breaking the software on purpose and counting what the gates noticed.

### Mutation score — 100.0%

`sb mutate` applied a fixed operator set to each implementation package and ran **only** the committed spec test against every mutant. 3 of 3 live mutants were caught, with 0 marked equivalent by the author and 0 excluded as invalid (the mutated package did not compile, which is evidence about Go and not about the test).

Measured 2026-09-22T04:52:41Z; per-mutant timeout 1m0s. A mutant that times out counts as caught: the gate's verdict on it was still "not this implementation".

| Spec | Impl | Test | Score | Caught | Survived | Equivalent | Invalid |
|---|---|---|---:|---:|---:|---:|---:|
| `processable` | `Processable` | `TestSpec_Processable` | 100.0% | 3 | 0 | 0 | 0 |

**Per operator.** A column of survivors under one operator names the shape of the blind spot, not just its size.

| Operator | Caught | Survived | Equivalent | Invalid |
|---|---:|---:|---:|---:|
| `cmp-flip` | 1 | 0 | 0 | 0 |
| `off-by-one` | 1 | 0 | 0 | 0 |
| `zero-return` | 1 | 0 | 0 | 0 |

No survivors: every mutant this operator set produced was either caught by the spec test or marked equivalent.


### Forgery corpus

`sb forgery` staged 3 program(s) from `forgeries` and ran the check each one's header declares. 3 produced their declared outcome. 0 succeeded — that is, obtained or used a guard value the proof chain never justified.


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

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `account-id.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

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

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `account-state.field-id` | `Id : account-id` | static | guard-type-at-boundary | Id is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |
| `account-state.field-balance` | `Balance : amount` | static | guard-type-at-boundary | Balance is typed amount; values of that type can only be constructed via shengen's guarded constructor, which enforces all of amount's premises transitively. |

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

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `amount.field-x` | `X : number` | static | guard-type-at-boundary | X is typed number; the target language's type system rejects non-number values at construction. |
| `amount.verified-x-0` | `(>= X 0) : verified` | runtime-evaluator | runtime-via-evaluator | amount is evaluated at runtime by the shen-derive evaluator against the spec expression (>= X 0); the spec and the runtime check are the same source. |

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

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `balance-invariant.field-bal` | `Bal : number` | static | guard-type-at-boundary | Bal is typed number; the target language's type system rejects non-number values at construction. |
| `balance-invariant.field-tx` | `Tx : transaction` | static | guard-type-at-boundary | Tx is typed transaction; values of that type can only be constructed via shengen's guarded constructor, which enforces all of transaction's premises transitively. |
| `balance-invariant.verified-bal-head-tx` | `(>= Bal (head Tx)) : verified` | static | guard-constructor-validates | shengen's generated constructor for balance-invariant rejects inputs that do not satisfy (>= Bal (head Tx)), so this premise holds for any value of type balance-invariant reachable in the impl. |

- `balance-invariant.field-bal` code references: `internal/shenguard/guards_gen.go:117`
- `balance-invariant.field-tx` code references: `internal/shenguard/guards_gen.go:117`
- `balance-invariant.verified-bal-head-tx` code references: `internal/shenguard/guards_gen.go:117`

### `processable` — define (✅ Discharged)

A pure function processable : amount --> (list transaction) --> boolean. The Shen spec is the oracle; the impl is asserted to match it on every sampled input. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> ...)
```

Continuously discharged since commit `22d7a3e149ce95f82655b3c53c701383ccf32062`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `processable.oracle-spec-equiv` | `spec(processable) ≡ impl(Processable) on sampled inputs` | runtime-sample | prover-z3-path-cover | shen-derive symbolically executed the spec (list unroll depth 4), enumerated 10 path(s), and used z3 to find a concrete witness for each of the 9 feasible one(s) (1 dead, 0 undecided). Those 9 witness(es) plus the boundary pool make up the 44 committed cases; the emitted Go test asserts impl returns the spec's value on each. |


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

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `safe-transfer.field-tx` | `Tx : transaction` | static | guard-type-at-boundary | Tx is typed transaction; values of that type can only be constructed via shengen's guarded constructor, which enforces all of transaction's premises transitively. |
| `safe-transfer.field-check` | `Check : balance-checked` | static | guard-type-at-boundary | Check is typed balance-checked; values of that type can only be constructed via shengen's guarded constructor, which enforces all of balance-checked's premises transitively. |

- `safe-transfer.field-tx` code references: `internal/shenguard/guards_gen.go:165`
- `safe-transfer.field-check` code references: `internal/shenguard/guards_gen.go:165`

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

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `transaction.field-amount` | `Amount : amount` | static | guard-type-at-boundary | Amount is typed amount; values of that type can only be constructed via shengen's guarded constructor, which enforces all of amount's premises transitively. |
| `transaction.field-from` | `From : account-id` | static | guard-type-at-boundary | From is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |
| `transaction.field-to` | `To : account-id` | static | guard-type-at-boundary | To is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |

- `transaction.field-amount` code references: `internal/shenguard/guards_gen.go:89`
- `transaction.field-from` code references: `internal/shenguard/guards_gen.go:89`
- `transaction.field-to` code references: `internal/shenguard/guards_gen.go:89`


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

- **Unproven** — the tool could not confidently classify the premise
  in this release. Treat the premise as outside the verified
  boundary until a future version of the tool can address it.

**What this report does not claim**

- It is not a SOC-2, ISO-27001, or any other compliance certification.
  It is a verification artifact that compliance and audit workflows
  may reference as evidence.
- It is not signed or attested. The `signature` field in the JSON is
  reserved for a future signing integration; in this release it is
  always null.
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
