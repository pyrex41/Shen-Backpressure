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

## Elixir Target (`cmd/shengen-ex`)

`lang = "elixir"` in `sb.toml` makes `sb gen` run `cmd/shengen-ex`. It is
written in Go and reuses shengen's datatype parser and classifier. A
parity test diffs the symbol table against `shengen --dry-run` for every
spec in the repo. The runnable example is
[`examples/phoenix-ash-tenant`](../examples/phoenix-ash-tenant/).

### Guard modules

Each datatype becomes one module with an `@enforce_keys` struct, an
`@opaque t`, accessors, and `new/N`:

```shen
(datatype resource-access
  Access : tenant-access;
  Resource : resource-id;
  Owner : tenant-id;
  (= Owner (head (tail Access))) : verified;
  ==============================================
  [Access Resource Owner] : resource-access;)
```

```elixir
def new(access, resource, owner) do
  cond do
    not (is_struct(access, Tenant.Shen.TenantAccess)) ->
      {:error, {:type, "Access : tenant-access", [access: access]}}
    # ... type checks for resource and owner ...
    not (owner == Tenant.Shen.TenantAccess.tenant(access)) ->
      {:error, {:premise, "(= Owner (head (tail Access)))", [owner: owner, access: access]}}
    true ->
      {:ok, %__MODULE__{access: access, resource: resource, owner: owner}}
  end
end
```

- Error values carry the Shen premise text verbatim, plus the values it
  referenced. `new!/N` raises `<NS>.GuardError` instead.
- `head`/`tail` chains over guard types resolve statically to accessor
  calls, as in the Go emitter. Anything else is evaluated over the Shen
  representation: `<NS>.Term.to_shen/1` unwraps wrappers and turns
  composites into lists in conclusion order, keeping literal tags.
- Sum types (several blocks → one conclusion) get a module with
  `@type t :: A.t() | B.t()` and `member?/1`.
- `(define ...)` blocks that premises call are lowered into
  `<NS>.Defines` with Shen data semantics. Clause order,
  non-linear patterns (`X X -> true`) and `where` guards are kept.
  `--all-defines` emits every define.
- `:runtime-via name` premises call `<NS>.Runtime.name/k`; override the
  module with `--runtime-module`.
- A premise outside the supported fragment does not become a silent
  `true`. It becomes a fail-closed branch (`{:unsupported_premise, ...}`)
  with a warning, or an error under `--strict`.
- Output is deterministic and starts with `# Code generated by
  shengen-ex from <spec>. DO NOT EDIT.`. `--check` diffs instead of
  writing, and `sb audit` / `sb gen --check` use it.

### Compile tracer

Elixir has no private fields, so `--tracer-out` generates
`<NS>.GuardTracer`. `mix.exs` loads it with `Code.require_file` and
enables it with `elixirc_options: [tracers: [...]]`. `mix compile` then
refuses:

| Forgery | Caught by |
|---|---|
| `%TenantAccess{...}` / `%TenantAccess{g \| ...}` outside `TenantAccess` | tracer (`:struct_expansion`); pattern matching stays allowed |
| `struct(TenantAccess, ...)` / `struct!/2` | tracer: refused in any module that also references a guard type (decided at `:on_module`) |
| `%{g \| tenant: t}`, `Map.put/3`, `%{__struct__: TenantAccess, ...}` | not visible to tracers. Dialyzer flags them once the value reaches a function with an opaque contract (accessor or `@spec`), and `sb audit` refuses raw `__struct__` maps in hand-written `lib/` code |
| module atoms passed at runtime to a module that never names a guard | not caught (documented gap) |

Dialyzer also reports `call_without_opaque` when code outside the guard
module pattern-matches `%Guard{}` and then calls an accessor. Use
`is_struct/2` there instead.

### Ash policy checks

`--ash-out` (`[elixir] ash_out`) selects access rules the same way
shen-cedar and shen-rego do (`--ash-targets`, else `*-access` / `*-permit`
/ `*-allow` conclusions). For each target it emits:

- `<NS>.Policy.<Target>Check`, an `Ash.Policy.SimpleCheck`. The actor
  must *be* the proof struct. When the proof chain carries a
  `tenant-id` field (found breadth-first; set with `--tenant-type`), its
  value must equal the request tenant.
- `<NS>.Policy.<Target>Filter`, an `Ash.Policy.FilterCheck`, emitted when
  the chain carries a `resource-id` (`--resource-type`). It scopes rows
  to `attribute == proof.resource`.

Unlike the Cedar/Rego lowerings, nothing is re-derived from attributes.
The Shen premises were discharged when the proof was constructed.

### shen-derive for Elixir

`[[derive.specs]]` with `lang = "elixir"` (`impl_pkg` = Elixir module,
`guard_pkg` = namespace) makes `sb derive` run `shengen-ex derive`. It
drift-checks the committed ExUnit file and then runs `mix test` on it.
The oracle is the Shen `define` itself: shen-erl boots in the test VM
(about 130 ms), loads the spec, and each call costs well under a
millisecond. Inputs come from a deterministic boundary pool per Shen
type, drawn from spec literals and built through the guard
constructors so every sample satisfies its premises. When `mix.exs`
depends on `:stream_data`, StreamData properties are added. The oracle
is located through `$SHEN_ERL_EBIN`, `$SHEN_ERL_ROOT`, or a sibling
`shen-erl` checkout.

### Premise mutation (`sb mutate-spec`, any language)

`[mutate]` configures a hostile corpus. Each hostile file is a Shen
program that must *not* typecheck. For every `: verified` premise the
gate blanks that premise, loads `prelude` + `(tc +)` + the mutant spec in
a fresh Shen process, and loads every hostile file under `trap-error`.
The premise is **killed** if some hostile file now typechecks and
**survives** otherwise. Survivors fail the gate. The baseline requires
all hostile files rejected and all `good` files accepted under the real
spec. Results go to `.sb/mutation_report.json`, and to
`premises[].mutation` in `.sb/discharge_report.json` when that report
exists (an additive, omitempty field).

- **Runtime:** `shen` / `args` / `isolate`. Defaults: shen-erl or
  ShenScript-style `script {file}`, one process per mutant; shen-sbcl
  `-l {file}`, one process per mutant × file, because shen-cl drops the
  rest of a script after a trapped type error.
- **Inference budget:** the counter is reset before each file (a hostile
  file that exhausts the budget would otherwise poison later verdicts).
  `max_inferences` raises the budget for older kernels.
- **Prelude:** Shen's `if` only needs `P : boolean`, so guarded
  constructions typecheck only with the `verified-if` rule. It is built
  in, or you can ship it as `specs/verified.shen`:

```shen
(datatype verified-if
  P : boolean;
  P : verified >> X : A;
  Y : A;
  _______________________
  (if P X Y) : A;)
```

Shen typechecking notes from the example: a pattern match on a rule with
a `verified` premise is a type error (the premise cannot be proven for
pattern variables). Double-line (`===`) wrapper and sum-variant rules
add left rules, which can push the search past the inference budget.
Use single-line (`___`) rules where values only flow *into* the type.

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
