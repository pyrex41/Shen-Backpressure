---
title: "How shen-derive Works, and Why It Isn't shengen"
date: 2026-04-10
draft: false
description: "A mechanism-level walkthrough of shen-derive — what it reads, how it samples, how it evaluates clauses, what it emits — contrasted against shengen so you know which tool to reach for."
tags: ["shen", "go", "verification", "property-testing", "codegen", "backpressure"]
---

*Two tools, one spec file, two very different kinds of guarantee.*

---

Shen-Backpressure ships two codegen-ish tools that read the same `.shen` spec files. They look similar on the outside — take a spec, produce Go, plug it into a gate — but they answer different questions and the mechanisms don't really resemble each other once you open them up. This post is a mechanism-level walkthrough of **shen-derive** with **shengen** as the foil, so the next time you're staring at a function and wondering which tool fits, you have a sharp answer.

The short form:

- **shengen** reads `(datatype ...)` blocks and emits Go structs with opaque fields and validating constructors. The guarantee it buys you is *"every value of this type satisfies the spec's premises, because you literally cannot construct one any other way."* The gate is the Go compiler.
- **shen-derive** reads `(define ...)` blocks and emits a Go test file that calls a hand-written implementation and asserts its output matches the spec's output on sampled inputs. The guarantee is *"on the sample set, this hand-written loop behaves exactly like the obvious-correct spec, and the moment it stops behaving that way the repo notices."* The gate is `go test` plus a drift diff.

They're additive. A single function can use both: a `Processable(b0 Amount, txs []Transaction) bool` uses shengen-generated guard types for its parameters (so `Amount` is provably non-negative at the type level) and uses a shen-derive test to pin the loop's behavior against a spec.

## The shared input

Both tools read Shen's s-expression surface syntax. Here's the payment example's `specs/core.shen`, trimmed to the relevant pieces:

```shen
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)

(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx))))) (val B0) Txs)))
```

Two kinds of block. shengen eats the `(datatype ...)` blocks. shen-derive eats the `(define ...)` blocks. They're happy to coexist in one file because they're solving different problems.

## What shengen does (briefly)

shengen is ~1900 lines of Go in `cmd/shengen/main.go`. Given the spec above, it walks the `(datatype ...)` blocks and emits `internal/shenguard/guards_gen.go`:

```go
// --- Amount ---
// Shen: (datatype amount)
type Amount struct{ v float64 }

func NewAmount(x float64) (Amount, error) {
    if !(x >= 0) {
        return Amount{}, fmt.Errorf("x must be >= 0: %v", x)
    }
    return Amount{v: x}, nil
}

func (t Amount) Val() float64 { return t.v }
```

Three pieces work together:

1. **`Amount` has an unexported field `v`**, so code outside `package shenguard` cannot construct an `Amount` with a struct literal.
2. **`NewAmount` is the only constructor**, and it returns an error if the sequent-calculus premise `(>= X 0)` fails.
3. **`Val()` is the only read path**, so the outside world gets a plain `float64` for arithmetic.

Any function that takes `Amount` as a parameter is, by construction, receiving a non-negative number. You don't need a runtime check inside the function. You don't need a test that proves the check exists. The *types* carry the proof, and the Go compiler enforces it. If you delete the `NewAmount` check, Shen's `tc+` gate (the `shen-check.sh` step) notices the sequent rule no longer parses, and the build fails upstream.

This is what "impossible by construction" means in this project. shengen is a proof-carrying-code generator: the spec's premises become compiler-checked preconditions.

## Where shengen runs out of room

shengen handles value-shape invariants beautifully. What it doesn't do is express *behaviors* over those values. You can't write a `datatype` rule that says "given a starting balance and a list of transactions, the function that checks whether every running balance stays non-negative should behave like `all (>= 0) (scanl apply b0 txs)`." That's a claim about a function, not a claim about a value.

You could try. You could invent a `balance-check-result` datatype with a `(processable-correct? B0 Txs Result) : verified` predicate and rely on the programmer to maintain consistency. But now the spec is no longer obviously correct — it's entangled with a particular implementation strategy — and you still can't force the Go function to actually compute the thing the type claims.

This is the gap shen-derive fills. `(define ...)` blocks let you write the obvious-correct behavior as a Shen function, and shen-derive gives you a way to pin a hand-written Go implementation against it.

## What shen-derive actually does

`shen-derive verify` is ~2000 lines of Go across three small packages:

- `core/` — s-expression AST, parser, evaluator, pattern matcher.
- `specfile/` — `.shen` file parser that shares the block-extraction algorithm with shengen and layers in a type-table classifier.
- `verify/` — sample generator, spec evaluator, Go test emitter.

Here's what happens when you run `shen-derive verify specs/core.shen --func processable --impl-pkg your-mod/internal/derived --impl-func Processable --import your-mod/internal/shenguard --out processable_spec_test.go`.

### Step 1 — Parse the spec file

The parser is the same two-phase shape shengen uses: find balanced-paren blocks starting with `(datatype ` or `(define `, then parse each block independently. For the datatype blocks, shen-derive builds a `TypeTable` that classifies each Shen type as *wrapper* (unvalidated, like `account-id`), *constrained* (wrapper + `verified` predicates, like `amount`), *composite* (fields, like `transaction`), *guarded* (composite + predicates), *alias*, or *sum type*. The classifier is a deliberate parallel to shengen's — we don't share code because shengen is a monolithic single-file tool and loose coupling is worth more than deduplication.

For the define blocks, the parser extracts the optional `{...}` type signature and then splits the body into clauses. A single-clause define like `processable` produces one clause with all-variable patterns `B0 Txs`. A multi-clause define with `where` guards (like `pair-in-list?` in the dosage-calculator example) produces a slice of `Clause{Patterns, Guard, Body}` records. Everything — patterns, guards, bodies — is stored as s-expressions. There's no secondary typed AST; Shen's own representation is the representation.

### Step 2 — Generate samples for each parameter type

This is where the verification model earns its keep. Given `B0 : amount` and `Txs : (list transaction)`, the harness walks the type signature and builds a sample pool per parameter.

For `amount`: start with the primitive pool for `number` — `{0, 1, -1, 5, 2.5, 100}`, a deliberate mix of zero, positive, negative, and fractional. Look up `amount` in the type table, see it's a `CatConstrained` wrapping `number` with the predicate `(>= X 0)`. Evaluate the predicate against each candidate using the core evaluator itself — the same evaluator that will run the spec body — and drop failures. Negative values get filtered out. You end up with `{0, 1, 5, 2.5, 100}`.

This is important: the sample filter shares the evaluator with the spec body, so a predicate that references another Shen function works for free.

For `transaction`: sample each field (`amount`, `account-id`, `account-id`) and produce one composite per field-sample index. `account-id` is a wrapper over `string`, so its samples are `{"", "alice", "bob"}` (length-3); `amount` has 5 post-filter samples; the composite count is `max(field_counts)` = 5.

For `(list transaction)`: empty list, plus one singleton per transaction sample (capped at 6), plus a 3-element multi-list. That's roughly 7 list shapes.

The cartesian product of 5 balances × 7 list shapes is 35, already under the `--max-cases 50` cap. Each cell in the product is a `[]Sample` — one sample per parameter.

**Why one singleton per transaction sample matters.** An earlier version of the sampler made the composite once per index and then built only three list shapes: empty, singleton of the *first* transaction, and a 3-element mix. A "tricky" composite at index 3 (e.g. the one with a fractional amount) never ended up inside any list, so an implementation that silently `int`-truncated amounts passed all 35 cases. The current rule surfaces the bug.

**Why seeded random is opt-in.** `--seed 42` layers eight random draws on top of each primitive pool (half ints in `[-1000, 1000]`, half two-decimal floats in the same range). With a non-zero seed, the cartesian product gets larger and the chance of finding a bug unknown to the boundary set goes up. But the committed test file is the drift gate, so if random were on by default every run would churn the file and the gate would become noise. Seed = 0 keeps it deterministic; anyone who wants deeper exploration passes the flag explicitly and the seed is stamped into the generated file's header for reproducibility.

### Step 3 — Build the evaluation environment

Before evaluating the spec on any sample, the harness builds a *base environment* once — a linked-list `Env` that maps names to `Value`s:

- `val` is bound to the identity function (wrapped in a `BuiltinFn`). Spec bodies use `(val B0)` to get the underlying primitive from a wrapper type; at evaluation time the "wrapper" is just the primitive itself, so identity is correct.
- Each composite field accessor becomes a `BuiltinFn` that projects the corresponding index out of a `ListVal`. So `(amount Tx)` for a transaction sample returns the first element of `Tx`'s list representation. Both the original case (`Amount`) and the lowered case (`amount`) are registered.
- Every `(define ...)` in the spec file becomes a *curried* `BuiltinFn` that collects N arguments and then dispatches to the clause-match evaluator. This is what lets `pair-in-list?` recurse on itself: clause 4's body calls `(pair-in-list? A B Rest)`, which resolves through the same base env, which eventually reaches the per-clause dispatch.

`BuiltinFn` is the escape hatch that keeps the core evaluator free of domain-specific coupling. The evaluator doesn't know what `amount` is, or what field it projects; it just knows how to apply a function to an argument. Everything domain-specific lives in the base env.

### Step 4 — Evaluate the spec on each sample

For each cell in the cartesian product, the harness calls `evalDefine(def, vals, base)`. `evalDefine` walks the define's clauses in order:

1. Match the clause's patterns against the argument values. The pattern matcher (`core/match.go`) handles wildcards (`_`), uppercase-variable binding (`A`, `Xs`), literal atoms, the `nil` symbol (for empty lists), and cons patterns (`(cons H T)`, which is what `[X | Xs]` and `[A B]` desugar to via the s-expression parser). On structural mismatch the matcher returns `(nil, false, nil)`; on a malformed pattern it returns an error.
2. If a `where` guard is present, evaluate it in the clause's extended env. Non-true guards skip the clause.
3. Evaluate the body in the extended env. Return the result.

If no clause matches, the harness errors out with `non-exhaustive clauses` — that's a spec bug, not an implementation bug, and it's worth failing loudly.

For `processable`, there's only one clause and all patterns are simple variables, so the matcher just binds `B0` and `Txs` and evaluates the body. The body uses `foldr`, `scanl`, `lambda`, `@p` tuples, `and`, `>=`, subtraction — all handled by the core evaluator's ~500 lines of arithmetic, comparison, list, and control-flow primitives. The return value is a `BoolVal`.

### Step 5 — Convert the value to a Go literal

For each case, the harness converts the evaluated `core.Value` to a Go source expression matching the spec's return type. For a `boolean` return it's just `"true"` or `"false"`. For `amount` (a wrapper return), it emits the underlying `float64` literal and the test comparison uses `got.Val() != tc.want`. For a list-of-wrapper return, it walks each element.

The Go expressions for *inputs* are built during sample generation: a number sample carries `{Value: IntVal(5), GoExpr: "5"}`; an amount sample wraps it as `{Value: IntVal(5), GoExpr: "mustAmount(5)"}`; a transaction sample wraps further. The `mustXxx` helpers are emitted once at the top of the test file — the collector scans the generated expressions for `mustFoo` identifiers and emits definitions that unwrap the shengen constructors with `panic` on error.

### Step 6 — Emit the test file

The output is a normal Go test:

```go
// Code generated by shen-derive. DO NOT EDIT.
// Regenerate with: shen-derive verify <spec.shen> --func processable

package derived_test

import (
    "testing"
    shenguard "ralph-shen-agent/internal/shenguard"
    derived "ralph-shen-agent/internal/derived"
)

func mustAmount(x float64) shenguard.Amount { v, err := shenguard.NewAmount(x); if err != nil { panic(err) }; return v }
func mustAccountId(x string) shenguard.AccountId { return shenguard.NewAccountId(x) }
func mustTransaction(a0 shenguard.Amount, a1 shenguard.AccountId, a2 shenguard.AccountId) shenguard.Transaction {
    return shenguard.NewTransaction(a0, a1, a2)
}

func TestSpec_Processable(t *testing.T) {
    cases := []struct {
        name string
        b0   shenguard.Amount
        txs  []shenguard.Transaction
        want bool
    }{
        {
            name: "case_00",
            b0:   mustAmount(0),
            txs:  []shenguard.Transaction{},
            want: true,
        },
        // ... 34 more cases ...
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got := derived.Processable(tc.b0, tc.txs)
            if got != tc.want {
                t.Fatalf("%s: spec says %v, impl returned %v", tc.name, tc.want, got)
            }
        })
    }
}
```

Notice the composition: the test uses shengen's `Amount`/`Transaction` types for its inputs (so the compiler is already enforcing non-negative amounts when the helpers run), and it calls the hand-written `derived.Processable` that the user chose to write. shen-derive doesn't generate the impl. It generates the *check*. The impl is free to use whatever Go idioms the author wants — early returns, direct arithmetic, pointer tricks — as long as its behavior on the sample set matches the spec's.

### Step 7 — Gate on drift + `go test`

The generated file is committed to the repo. The `sb derive` gate (or `make shen-derive-verify` in the payment example) regenerates it to a tempfile, diffs the tempfile against the committed copy, fails on any difference, and then runs `go test` on the impl package. Two failure modes:

- **Drift**: the committed test file doesn't match what shen-derive would generate today. Cause is usually a spec edit or a sampling-strategy change. Fix is `sb derive --regen` (or `make shen-derive-regen`) after reviewing the diff.
- **Mismatch**: `go test` reports `case_13: spec says false, impl returned true`. The hand-written Go is wrong relative to the spec. Fix is to correct the implementation.

Both failures are loud and localized. You see exactly which case diverged, and which field's value caused the divergence.

## Side-by-side mechanism comparison

| | **shengen** | **shen-derive** |
|---|---|---|
| **Input block** | `(datatype ...)` | `(define ...)` |
| **What it emits** | A Go package (`shenguard/guards_gen.go`) defining struct types with opaque fields and validating constructors | A Go test file calling your implementation and asserting its outputs match the spec's on a sample set |
| **Runtime shape** | No runtime — the generated code *is* the runtime. Types flow through the program; the compiler enforces construction. | The generated test runs during `go test`. It's a normal table-driven test file. |
| **What it checks** | *Values*. Every `Amount` that exists satisfies `(>= X 0)` because the only way to create one checks the predicate. | *Behaviors*. The hand-written function `Processable` returns what the spec says it returns on each sampled input. |
| **Scope of guarantee** | All inputs, because the check is on the constructor (finitely many call sites) not on the consumers (arbitrarily many). | Only the sampled inputs. Boundary pool + filter by constraints + optional seeded random. |
| **Failure mode on drift** | `shen tc+` fails the upstream gate (spec contradicts itself) OR the generated types don't match what's committed (codegen drift) OR client code stops compiling (spec changed shape). | The committed test file diverges from the regenerated output (diff on `out_file`) OR `go test` fails on a specific case. |
| **Complementary role** | Makes the *types* safe to pass around. | Makes the *functions* honest about what they compute. |
| **Authoring cost** | Write a `(datatype ...)` block; shengen emits the struct. Impl code uses the type; compiler does the rest. | Write a `(define ...)` block; hand-write the impl; commit the generated test; gate on drift. |

## When to reach for which

The clean decision rule:

- **Is this a value that flows through the code?** Reach for shengen. Anything where "this string is a valid account id" or "this number is non-negative" matters — account IDs, amounts, percentages, user IDs, tenant IDs, domain enums. The guarantee is `∀ Amount. (>= (Val x) 0)`, enforced by the type system, costing zero runtime checks in consumer code.
- **Is this a function whose behavior is easy to state but hard to implement efficiently?** Reach for shen-derive. Anything where you'd naturally write the obvious loop in `scanl + all` and then hand-roll an early-exit version in Go — balance checks, running aggregates, validation pipelines, state-machine transitions, anything fold-shaped. The guarantee is `Processable(b0, txs) == spec(b0, txs)` on the sample set, enforced by `go test` plus a drift gate.
- **Do you need both?** Write the datatype blocks for the values and the define blocks for the behavior in the same `.shen` file. They interoperate: shen-derive's generated test uses shengen's guard-type constructors for its inputs, and the spec body uses shengen's field accessors to talk about the composites.

And the clean rule for when *not* to use either:

- shengen is the wrong tool if the thing you're trying to pin is a *behavior* — a function you want to check isn't well-modeled as a value.
- shen-derive is the wrong tool if the property you care about is "holds for all possible inputs, including ones I haven't thought of." Use `shen tc+` (which *does* prove quantified claims) or pair shen-derive with Go's native fuzzer using the spec evaluator as the oracle.

## Why these two, not one combined tool

The obvious question is whether this could all be one tool that takes a `.shen` file and emits both the types and the tests. Mechanically, yes. Conceptually, no — because the two halves solve problems with fundamentally different shapes.

shengen is a proof-carrying-code generator. The output has no runtime checks *inside* application code; the runtime checks live at construction boundaries, and the compiler does the rest. The guarantee is structural: "all values of this type satisfy the predicates." Adding code generation for *functions* would mean either choosing a specific implementation strategy (which defeats the point — you wanted to hand-write the efficient one) or generating a reference impl and hoping nobody edits it.

shen-derive is a property-testing harness. The output runs at test time, not at application time. The guarantee is sample-based: "on these inputs, the hand-written code agrees with the spec." Adding construction-time validation would mean rebuilding shengen's entire pipeline inside shen-derive, which is wasteful and would conflate the two guarantees in a confusing way.

Keeping them separate means each tool's guarantee is a sharp, defensible claim. shengen promises "any `Amount` is non-negative, period." shen-derive promises "on the sample set, `Processable` matches the spec, and the repo gates on drift." Combining them would blur both.

## The part I haven't talked about

The archived v1 of shen-derive was an algebraic rewrite engine: Bird-Meertens laws, pattern-based lowering to for-loops, the whole workbench. It could take `all (>= 0) (scanl apply b0 txs)` and *derive* the single-pass early-exit loop mechanically. That's the original shen-derive blog post if you want to read it — it's archived as `shen-derive/archive/` in the repo now.

The pivot to the verification-gate model wasn't because the rewrite engine was broken. It was because the surface area was bounded by the law catalog: every new shape of computation needed a new law *and* a new lowering pattern *and* an equivalence proof. The verification-gate model trades that mechanism for a strictly larger set of verifiable specs at the cost of "only sampled inputs, not all inputs." For the work shen-derive does in Shen-Backpressure — pinning hand-written loops to obvious specs, not proving global claims — that trade is the right one.

If you want proofs, use `shen tc+` and shengen. If you want pinning, use shen-derive. If you want both, use them both in the same `.shen` file. They were designed to coexist.
