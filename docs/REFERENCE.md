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
