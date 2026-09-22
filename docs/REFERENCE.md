# Reference

Material that used to live in the lead README — pattern catalog,
Shen→Go side-by-side, design-decision Q&A, ASCII pipeline. Kept here
so the README can lead with framing and so readers who want depth
have one place to go.

## The Backpressure Pipeline

```
specs/core.shen          Shen sequent-calculus type rules
       |
       v  (shengen)
internal/shenguard/      Generated guard types (Go or TypeScript)
       |
       v  (import)
Application code         Uses guard types at domain boundaries
       |
       v  (gates)
Verification             shengen → test → build → shen tc+ → tcb audit (→ shen-derive)
       |
       v  (fail?)
Backpressure             Gate errors fed back (to LLM, CI, or developer)
```

Gates 1–5 cover the structural pipeline (shengen-driven). Gate 6
(`sb derive`) runs whenever the manifest declares `[[derive.specs]]`
and adds sampled spec-equivalence on top.

## sb.toml gate kinds

The five-gate shape is fixed; additional gates are declared in the
`[[gates]]` array of tables and each carries a `kind`.

| `kind` | Served by | `run` means |
|---|---|---|
| `command` (default) | the shell | the command to run; a non-zero exit fails the gate |
| `derive` | `sb derive` | ignored — auto-appended when `[[derive.specs]]` is present |
| `flow` | `sb flow` | the **fallback** grep, used only when no SCIP indexer is on PATH |
| `forgery` | `sb forgery` | the legacy regex a `grep-miss-flow-catch` forgery must slip past; empty reuses the `flow` gate's |

```toml
[[gates]]
name = "flow"
kind = "flow"
# Not the gate: the legacy regex to fall back to when `sb index` finds
# no SCIP indexer. Omit it when there is no legacy gate to degrade to.
run  = "./bin/shenguard-audit.sh --grep-only"
```

A `flow` gate reads the `(flow <name> …)` forms from the spec named by
`[paths] spec` (and from every `[[derive.specs]]` path), runs
`sb index` — `scip-go` for `lang = "go"`, `scip-typescript` for
`lang = "ts"`, cached by a content hash over the source tree — and
records each premise in the discharge report: discharged with basis
`flow-analysis`, or violated with the offending reference's
`file:line:col` and the shortest violating path. With no indexer the
premises are recorded `unproven` with basis `grep-fallback`. The two
premise forms, the two engines, and what the indexer costs in TCB
terms are in [FLOW.md](FLOW.md).

A `forgery` gate runs the project's corpus of programs that try to
obtain a guard value without its constructor. Every `*.go.bak` under
the corpus directory declares the outcome it expects in a header line
the gate reads, and the gate stages it into a scratch package inside
the project module and runs the check that expectation names:

```go
// sb-forgery: expect grep-miss-flow-catch
// sb-forgery-entry: ReadForgedTenant          // runtime outcomes only
// sb-forgery-replaces: internal/derived/x.go  // derive-catch only
```

| expectation | the check the gate runs |
|---|---|
| `compile-error` | `go build` on the staged package must fail |
| `runtime-panic` | a generated driver calls the entry point; the program must panic |
| `runtime-error` | the driver's entry must return a non-nil error |
| `flow-violation` | it must compile, and `sb flow` over the staged tree must fail |
| `grep-miss-flow-catch` | the legacy regex must **pass** it *and* `sb flow` must fail it |
| `derive-catch` | it replaces the named impl file; the committed spec test must fail |
| `succeeds (documented TCB limit)` | it compiles and runs cleanly, and the corpus says so on purpose |

Any outcome that differs from its declaration fails the gate. A
forgery that succeeds without the last declaration is a failure: the
corpus exists so a *new* success shows up as a red build rather than as
prose nobody re-reads. `grep-miss-flow-catch` is the only expectation
that compares two gates, and the gate refuses to record it unless the
regex really does miss it — a forgery both gates catch is not evidence
that one sees more than the other.

```toml
[[gates]]
name = "forgery"
kind = "forgery"
run  = "./bin/shenguard-audit.sh --grep-only"

[forgery]
dir   = "forgeries"                  # default
stage = "internal/forgery_harness"   # default; removed after every run
```

`sb forgery -markdown` renders the corpus as a table whose outcome
column is measured rather than recorded, and `sb forgery -only <sub>`
runs one entry.

Gates sharing a non-empty `parallel_group` run concurrently;
everything else runs in declared order.

## `sb mutate` — gate strength

`sb mutate` is a measurement, not a gate. For each `[[derive.specs]]`
entry it applies a fixed operator set to the implementation package
over `go/ast` and, for every mutant, runs *only* the committed spec
test.

| operator | rewrite |
|---|---|
| `cmp-flip` | one comparison to its counterpart (`==`↔`!=`, `<`↔`<=`, `>`↔`>=`) |
| `off-by-one` | one integer or float literal to itself plus one |
| `drop-conjunct` | `A && B` → `A && true`, `A || B` → `A || false` |
| `list-swap` | `xs[0]` → `xs[len(xs)-1]`, `xs[1:]` → `xs[:len(xs)-1]` |
| `zero-return` | `return <zero values>` as the function's first statement |

The set is small on purpose: large operator sets manufacture equivalent
mutants, and each one costs an author real time to dismiss.

Outcomes are `caught`, `survived`, `equivalent` and `invalid`. The
score is `caught / (caught + survived)`. Two classification rules are
worth stating because each is a way a kill rate can be quietly
inflated:

- A mutant the compiler rejects is `invalid`, not caught. A build error
  is evidence about Go, not about the test.
- A mutant that times out **is** caught. The test's verdict on it was
  still "not this implementation".

An equivalent mutant is one no test could kill. Only the author may
declare one, keyed by a stable id that is computed against the
committed file so it survives unrelated edits elsewhere:

```toml
[derive.mutation]
timeout    = "60s"
equivalent = [
  "off-by-one:internal/derived/x.go:12:20",  # bound is exclusive either way
]
```

The score lands in the discharge report under a new top-level
`evidence` object — additive and `omitempty`, so `schema_version` does
not move and a project that never runs `sb mutate` emits the same bytes
it did before — and `sb audit-report` renders it as a **Gate Strength**
section that leads with the kill rate and lists every survivor with its
location. The section is omitted entirely when nothing has been
measured: a table of zeros reads as a bad score when the truth is that
nobody looked.

A spec whose `lang` is not `go` is recorded as a `gaps` entry rather
than skipped silently; the operator set is implemented over `go/ast`
only.

## `sb loop --falsify`

After an iteration in which every gate passes, `--falsify` runs a
second phase. Its prompt (`sb/FALSIFIER_PROMPT.md`, overridable per
project at `prompts/falsifier_prompt.md`) is hydrated with the spec,
the generated guards, the current forgery corpus and the mutation
survivors, and asks for exactly one of:

- **a new forgery** written to the corpus with a declared expectation,
  which `sb forgery` runs immediately; or
- **one input where the spec and the implementation disagree**,
  appended to `.sb/falsifier-samples.json`.

That file carries *inputs*, never expected outputs. shen-derive reads
it as a fourth sample source (behind `--falsifier-samples`, which `sb
derive` passes whenever the file exists) and derives each expectation
from the spec's own evaluator. So a right claim becomes a failing test
with a counterexample, and a wrong one becomes an ordinary passing
sample — the falsifier cannot make the suite wrong by being wrong, only
by being uninteresting.

```json
{
  "schema_version": 1,
  "samples": [
    {
      "spec": "processable",
      "note": "boundary: the running balance lands exactly on zero",
      "args": [7, [{"amount": 7, "from": "a", "to": "b"}]]
    }
  ]
}
```

Arguments are decoded against each parameter's Shen type: a scalar for
a `number`/`string`/`boolean` or for a wrapper datatype, a JSON array
for `(list T)`, and either a field-ordered array or a name-keyed object
for a composite. A malformed entry warns and names itself rather than
failing the gate; a newer `schema_version` is an error, because reading
it partially would make the gate quietly weaker.

`sb loop --falsify-prompt` prints the hydrated prompt and calls no
harness; `--falsify-only` runs the phase once against the current tree.

Gate kinds and the loop share one harness command, so a project that
configured `[loop] harness` has configured both phases.

## The Codegen Bridge (shengen)

`shengen` parses `specs/core.shen` and emits target-language types
with **unexported fields** and **validated constructors**. You can't
create a guard type without going through its constructor, and the
constructor enforces the spec's invariants at compile time.

```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

Becomes (Go output):

```go
type BalanceChecked struct {
    bal float64
    tx  Transaction
}

func NewBalanceChecked(bal float64, tx Transaction) (BalanceChecked, error) {
    if !(bal >= tx.amount.Val()) {
        return BalanceChecked{}, fmt.Errorf("bal must be >= tx.amount")
    }
    return BalanceChecked{bal: bal, tx: tx}, nil
}

func (t BalanceChecked) Bal() float64    { return t.bal }
func (t BalanceChecked) Tx() Transaction { return t.tx }
```

The LLM cannot bypass this:

- `Amount{v: 50}` won't compile (unexported `v`).
- `BalanceChecked{bal: 0, tx: tx}` won't compile either (unexported
  fields).
- `SafeTransfer` requires a `BalanceChecked` proof that can only come
  from `NewBalanceChecked`.

Two things it still could do, and the flag that closes them:

- `BalanceChecked{}` — the *empty* literal names no field, so Go
  always allowed it, from any package.
- `NewSafeTransfer(tx2, checkForTx1)` — the pre-brand constructor
  never checked that the proof is about that transaction.

### `--brands` (opt-in): proof binding and the witness

`shengen --brands` (and `shengen-ts --brands`) emits GDP brands: each
proof type gets a phantom brand parameter, inferred from the spec's
existing sharing structure, plus an unexported witness field.

```go
type BalanceChecked[B Brand] struct {
    valid witness
    bal   float64
    tx    Transaction[B]
}

func NewBalanceChecked[B Brand](bal float64, tx Transaction[B]) (BalanceChecked[B], error) {
    tx.valid.mustBeMinted("Transaction")
    if !(bal >= tx.amount.Val()) {
        return BalanceChecked[B]{}, fmt.Errorf("bal must be >= tx.amount")
    }
    return BalanceChecked[B]{valid: mint(), bal: bal, tx: tx}, nil
}

func NewSafeTransfer[B Brand](tx Transaction[B], check BalanceChecked[B]) SafeTransfer[B]
```

- The brand makes an unpaired proof a **compile error**: a
  `BalanceChecked` minted for another transaction is a different type.
- The witness makes the empty literal **loud**: its zero value is not
  minted, and every accessor and consuming constructor panics on it.
  A failing constructor returns that unminted zero value, so a dropped
  `err` panics at first read instead of forging a proof.

Flags:

| Flag | Emitter | Effect |
|------|---------|--------|
| `--brands` | `shengen`, `shengen-ts` | Emit brand parameters + witness. Prints the inferred brand table to stderr. |
| `--no-brands` | `shengen`, `shengen-ts` | Explicit spelling of the default; output is byte-identical to the pre-brand emitter. |
| `SHENGEN_BRANDS=1` | `bin/shengen-codegen.sh` | Passes `--brands` through. |
| `--brands` | `bin/shenguard-audit.sh` | Regenerates with `--brands` and fails if any wrapper type in the committed file lacks its witness field. |

A project that opts in must pass the flag to **both** its codegen and
its TCB audit gate, or the drift check compares branded output against
an unbranded regeneration and fails. `examples/payment` and
`examples/multi-tenant-api` are wired that way; see their
`bin/shengen-codegen.sh` and `bin/shenguard-audit.sh`.

Brands are opt-in by design, and what they do and do not guarantee —
including the caller's role in brand freshness, and the witness panic's
membership in the TCB — is in
[TRUST-MODEL.md](TRUST-MODEL.md#proof-binding-gdp-brands).

The TypeScript output via `cmd/shengen-ts/` mirrors this with
`#`-prefixed private fields and equivalent factory functions; the
Python and Rust reference emitters under `cmd/shengen-py/` and
`cmd/shengen-rs/` produce closure-based and PhantomData-based
opaqueness respectively.

## Guard Type Patterns

| Shen pattern | Go output | Constructor |
|-------------|-----------|-------------|
| Wrapper (`X : string; ==> X : account-id`) | `struct{ v string }` | `NewAccountId(string) AccountId` |
| Constrained (`(>= X 0) : verified`) | `struct{ v float64 }` | `NewAmount(float64) (Amount, error)` |
| Composite (`[A B C] : transaction`) | `struct{ a, b, c }` + accessors | `NewTransaction(A, B, C) Transaction` |
| Guarded (`(>= Bal (head Tx)) : verified`) | `struct{ bal, tx }` + accessors | `NewBalanceChecked(...) (BalanceChecked, error)` |
| Proof chain (`Check : balance-checked`) | `struct{ tx, check }` + accessors | `NewSafeTransfer(Transaction, BalanceChecked) SafeTransfer` |
| Sum type (multiple blocks → same conclusion) | Go interface + concrete structs | `AuthenticatedPrincipal` = `HumanPrincipal \| ServicePrincipal` |

`examples/.archive/category-showcase/` carries one spec exercising
all six patterns; `examples/payment/reference/guards_gen.{go,ts,rs,py}`
shows the same five datatypes in four target languages.

## One Spec, Four Languages — Runnable

`examples/multilang-paired/` is the end-to-end demo of the
multi-emitter story. Same `specs/core.shen`, four independent CLIs,
one shared `fixture-inputs.jsonl`, one parity check.

The spec models a shopping-cart discount decision:

```shen
(datatype discount-eligible
  Cart : cart;
  ItemCount : number;
  MinSubtotal : number;
  MinItems : number;
  (>= (head Cart) MinSubtotal) : verified;
  (>= ItemCount MinItems) : verified;
  ============================================
  [Cart ItemCount MinSubtotal MinItems] : discount-eligible;)
```

The four emitter invocations are wired into `sb.toml` as separate
`[[gates]]` entries (since `[project] lang` is single-valued):

```bash
cd examples/multilang-paired

# Regenerate guards in all four languages.
./bin/codegen-go.sh     # → go/multilang_paired/guards_gen.go
./bin/codegen-ts.sh     # → ts/guards_gen.ts
./bin/codegen-py.sh     # → py/guards_gen.py
./bin/codegen-rs.sh     # → rs/guards_gen.rs

# Drift-check each language's committed output.
../../bin/shenguard-audit.sh --lang go --no-isolation-check \
  specs/core.shen multilang_paired go/multilang_paired/guards_gen.go
# (same shape for --lang ts / py / rs)

# Behavioural parity: each CLI reads the shared fixture, emits JSONL.
./bin/parity-check.sh
#   PASS  Go == TS
#   PASS  Go == Py
#   PASS  Go == Rs
#   ...
#   OK: all 4 languages produced byte-identical JSONL on 20 fixture rows.
```

A Go integration test at `cmd/shengen/parity_test.go` (run via
`go test ./cmd/shengen/... -run TestParity`) runs all four emitters
as subprocesses and asserts each produces a parseable file with the
expected datatype identifiers — the structural parity contract
complementing the behavioural one above.

The full `sb gates` topology for this example is 13 gates
(four `shengen-*`, three `build-*`, four `tcb-audit-*`, one
`shen-check-datatypes`, one `parity`). See
`examples/multilang-paired/README.md` for the per-gate purpose table
and the known emitter-coverage gaps (Go skips free-standing defines;
TS has a `where`-on-last-clause parser bug).

## Design Decisions

- **Why shengen?** Shen proves invariants deductively but doesn't
  generate Go (or TypeScript) code. shengen bridges the gap — the
  formal spec becomes compile-time enforcement via opaque types in a
  language the application is already written in.
- **Why five gates (plus one)?** Gate 1 (`shengen`) ensures generated
  types stay in sync with specs. Gate 2 (`test`) catches runtime
  violations on the cases the author thought of. Gate 3 (`build`)
  catches type signature mismatches when the spec changes. Gate 4
  (`shen tc+`) catches inconsistent specs. Gate 5 (`tcb audit`)
  ensures the forgery boundary contains only generated code. Gate 6
  (`shen-derive`) closes the behavioral gap for pure functions where
  the obvious-correct version is clear but the efficient version is
  not.
- **Why opaque constructors?** Unexported `v` fields mean the
  target-language compiler enforces the spec. You literally cannot
  create an `Amount` without going through `NewAmount`, which
  validates `>= 0`.
- **Why Go for the orchestrator?** Fast compilation, `errgroup` for
  parallel gates, static binary, stdlib-only deployment.
- **Why Shen over Coq/Lean/Agda?** Turing-complete, Lisp syntax that
  LLMs handle well, runs as a subprocess, sequent-calculus type
  rules map cleanly to constructor preconditions.
- **Why a checked-in skilldata mirror?** Embedding the canonical
  `sb/` tree at build time means a fresh clone embeds the right
  bundle without `make` first; CI catches drift via
  `make check-skilldata`. The alternative — fetching at install time
  — was rejected as wrong for an offline-first tool.

## Wave Memos

The three engine waves are documented in `thoughts/shared/research/`:

- `2026-05-05-wave-1-manifest-driven-gates.md` — `[[gates]]` array,
  two-pass parser, auto-appended `shen-derive` gate.
- `2026-05-05-wave-2-sb-context.md` — `ProjectContext`, JSON for CI,
  Markdown for prompts.
- `2026-05-05-wave-3-prompt-hydration.md` — `BuildContext(cfg)` per
  iteration, `buildHarnessPrompt` composition.
