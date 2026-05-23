# Discharge Report — Audit Rendering

Generated 2026-05-23T03:47:33Z. Source artifact: `.sb/discharge_report.json` (schema_version=1).

**Implementation commit:** `81ddf671a482f8b325db11acd04c72ec4182af03` (working tree dirty)

**Spec files:**

- `specs/core.shen` (sha256 `cb5d6c98c409307aa6345870d3a4f70e564085ee1222542a522e75a1bed2d9a7`)

**Target languages:** go

## Tool Versions

| Tool | Version |
|---|---|
| sb | 0.3.0 |
| shen-derive | 0.3.0 |
| shengen | — |
| shen runtime | not detected |

## Summary

- **Rules:** 7 total — 7 discharged, 0 violated, 0 unproven
- **Premises:** 14 total — 13 static, 1 runtime-sampled, 0 unproven

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

Continuously discharged since commit `81ddf671a482f8b325db11acd04c72ec4182af03`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `account-id.field-x` | `X : string` | static | guard-type-at-boundary | X is typed string; the target language's type system rejects non-string values at construction. |

- `account-id.field-x` code references: `internal/shenguard/guards_gen.go:15`

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

Continuously discharged since commit `81ddf671a482f8b325db11acd04c72ec4182af03`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `account-state.field-id` | `Id : account-id` | static | guard-type-at-boundary | Id is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |
| `account-state.field-balance` | `Balance : amount` | static | guard-type-at-boundary | Balance is typed amount; values of that type can only be constructed via shengen's guarded constructor, which enforces all of amount's premises transitively. |

- `account-state.field-id` code references: `internal/shenguard/guards_gen.go:85`
- `account-state.field-balance` code references: `internal/shenguard/guards_gen.go:85`

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

Continuously discharged since commit `81ddf671a482f8b325db11acd04c72ec4182af03`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `amount.field-x` | `X : number` | static | guard-type-at-boundary | X is typed number; the target language's type system rejects non-number values at construction. |
| `amount.verified-x-0` | `(>= X 0) : verified` | static | guard-constructor-validates | shengen's generated constructor for amount rejects inputs that do not satisfy (>= X 0), so this premise holds for any value of type amount reachable in the impl. |

- `amount.field-x` code references: `internal/shenguard/guards_gen.go:26`
- `amount.verified-x-0` code references: `internal/shenguard/guards_gen.go:26`

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

Continuously discharged since commit `81ddf671a482f8b325db11acd04c72ec4182af03`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `balance-invariant.field-bal` | `Bal : number` | static | guard-type-at-boundary | Bal is typed number; the target language's type system rejects non-number values at construction. |
| `balance-invariant.field-tx` | `Tx : transaction` | static | guard-type-at-boundary | Tx is typed transaction; values of that type can only be constructed via shengen's guarded constructor, which enforces all of transaction's premises transitively. |
| `balance-invariant.verified-bal-head-tx` | `(>= Bal (head Tx)) : verified` | static | guard-constructor-validates | shengen's generated constructor for balance-invariant rejects inputs that do not satisfy (>= Bal (head Tx)), so this premise holds for any value of type balance-invariant reachable in the impl. |

- `balance-invariant.field-bal` code references: `internal/shenguard/guards_gen.go:63`
- `balance-invariant.field-tx` code references: `internal/shenguard/guards_gen.go:63`
- `balance-invariant.verified-bal-head-tx` code references: `internal/shenguard/guards_gen.go:63`

### `processable` — define (✅ Discharged)

A pure function processable : amount --> (list transaction) --> boolean. The Shen spec is the oracle; the impl is asserted to match it on every sampled input. *(auto-generated from rule structure; not reviewed by spec author)*

Spec:

```shen
(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> ...)
```

Continuously discharged since commit `81ddf671a482f8b325db11acd04c72ec4182af03`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `processable.oracle-spec-equiv` | `spec(processable) ≡ impl(Processable) on sampled inputs` | runtime-sample | shen-derive-sampled | shen-derive evaluated the spec on 35 sampled cases (deterministic-default) and emitted a Go test asserting impl returns the same value on each. |


- `processable.oracle-spec-equiv`: sampled 35 cases (seed: deterministic-default); 35 passed, 0 failed.

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

Continuously discharged since commit `81ddf671a482f8b325db11acd04c72ec4182af03`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `safe-transfer.field-tx` | `Tx : transaction` | static | guard-type-at-boundary | Tx is typed transaction; values of that type can only be constructed via shengen's guarded constructor, which enforces all of transaction's premises transitively. |
| `safe-transfer.field-check` | `Check : balance-checked` | static | guard-type-at-boundary | Check is typed balance-checked; values of that type can only be constructed via shengen's guarded constructor, which enforces all of balance-checked's premises transitively. |

- `safe-transfer.field-tx` code references: `internal/shenguard/guards_gen.go:104`
- `safe-transfer.field-check` code references: `internal/shenguard/guards_gen.go:104`

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

Continuously discharged since commit `81ddf671a482f8b325db11acd04c72ec4182af03`.

**Premises**

| ID | Expression | Discharge | Basis | Rationale |
|---|---|---|---|---|
| `transaction.field-amount` | `Amount : amount` | static | guard-type-at-boundary | Amount is typed amount; values of that type can only be constructed via shengen's guarded constructor, which enforces all of amount's premises transitively. |
| `transaction.field-from` | `From : account-id` | static | guard-type-at-boundary | From is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |
| `transaction.field-to` | `To : account-id` | static | guard-type-at-boundary | To is typed account-id; values of that type can only be constructed via shengen's guarded constructor, which enforces all of account-id's premises transitively. |

- `transaction.field-amount` code references: `internal/shenguard/guards_gen.go:40`
- `transaction.field-from` code references: `internal/shenguard/guards_gen.go:40`
- `transaction.field-to` code references: `internal/shenguard/guards_gen.go:40`


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
