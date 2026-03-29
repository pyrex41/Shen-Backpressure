---
date: 2026-03-29T08:30:00-07:00
researcher: reuben
git_commit: 24ad868
branch: main
repository: Shen-Backpressure
topic: "Closures, opaque types, and Shen: exploring the idea space for backpressure and beyond"
tags: [research, closures, opaque-types, shen, backpressure, codegen, idea-space, type-theory]
status: complete
last_updated: 2026-03-29
last_updated_by: reuben
---

# Research: Closures, Opaque Types, and Shen — Exploring the Idea Space

**Date**: 2026-03-29
**Researcher**: reuben
**Git Commit**: 24ad868
**Branch**: main
**Repository**: Shen-Backpressure

## Research Question

An opaque type and a closure are distinct concepts but share deep connections through information hiding. How do these concepts intersect with Shen-Backpressure's approach? Where might closures be useful — in the Shen specs themselves, in the codegen bridge, in the generated target code, or in entirely new patterns? Could Shen itself be used more directly for code, not just specs?

## Summary

The Shen-Backpressure project currently uses **opaque struct types with validated constructors** as its enforcement mechanism. This is the right default for Go (unexported fields are a first-class language feature) and works well for TypeScript (private constructors + static factory). But closures open up several unexplored dimensions:

1. **Closures as the enforcement mechanism** for languages where struct opacity is weak (Python, Ruby, Lua, Clojure)
2. **Closure-typed guards in Shen specs** — specs that define function types, not just data types
3. **Continuation-passing proof chains** — threading validated state through closures instead of structs
4. **Shen as application logic** — using Shen directly for domain code, not just as a spec language
5. **Closure-based capability tokens** — validated closures as unforgeable capabilities in the object-capability model

Each of these represents a different level of ambition and a different trade-off.

## Detailed Findings

### 1. The Current Approach: Opaque Structs

The existing shengen pipeline works as follows:

```
Shen spec (datatype blocks)
    → shengen parses to AST (Premise, VerifiedPremise, Conclusion, Rule)
    → Symbol table classifies: wrapper, constrained, composite, guarded, alias
    → Go/TS emitter produces opaque types with validated constructors
```

**Go enforcement**: unexported fields (`type Amount struct{ v float64 }`) — code outside the package literally cannot construct the struct. (`cmd/shengen/main.go`)

**TypeScript enforcement**: `private constructor` + `static create()` factory — `new Amount(50)` won't compile outside the class. (`examples/payment_ts/guards_gen.ts:19-27`)

**Key property**: The opaque type IS the proof. Holding a value of type `BalanceChecked` means the balance-covers-amount invariant was verified. The proof chain (`Amount → Transaction → BalanceChecked → SafeTransfer`) is enforced transitively by the type system.

### 2. Where Closures Enter: Languages Without Structural Opacity

Go and TypeScript have good opacity stories. But shengen's `create-shengen` command (`sb/commands/create-shengen.md`) already lists target languages where opacity is weaker:

| Language | Opacity mechanism | Closure alternative |
|----------|-------------------|---------------------|
| Go | Unexported fields | Not needed — native opacity is strong |
| TypeScript | Private constructor | Not needed — private fields work |
| Rust | `pub(crate)` fields | Not needed — module privacy is strong |
| **Python** | `__slots__` + `__post_init__` | **Closures would be stronger** — `__dict__` hacking bypasses slots |
| **Ruby** | Private attrs | **Closures would be stronger** — `instance_variable_get` bypasses |
| **Lua** | Metatables | **Closures are the standard idiom** |
| **Clojure** | deftype with protocols | **Closures are natural** — close over validated state |
| **JavaScript (no TS)** | WeakMap or # private fields | **Closures are the classic pattern** |

For these languages, shengen could emit **closure-based guards**:

```python
# Python: closure-based Amount guard
def create_amount_factory():
    _validated = {}  # WeakMap would be better

    def new_amount(x: float):
        if not (x >= 0):
            raise ValueError(f"x must be >= 0: {x}")
        key = id(x)  # simplified
        return _make_amount(x)

    class Amount:
        __slots__ = ('_token',)
        def __init__(self, token):
            # Can only be called from within this closure scope
            raise TypeError("Use new_amount() to create Amount values")
        def val(self): ...

    def _make_amount(x):
        obj = object.__new__(Amount)
        obj._v = x
        return obj

    return new_amount

new_amount = create_amount_factory()
```

But a cleaner Python pattern uses closures directly:

```python
# The closure IS the opaque type
def make_amount(x: float):
    if not (x >= 0):
        raise ValueError(f"x must be >= 0: {x}")
    # The closure closes over validated x
    def val():
        return x
    val._is_amount = True  # type tag
    return val

amt = make_amount(50)
amt()  # → 50
```

This is the **"closures as opaque types"** pattern. The closure captures validated state. You can't access `x` except through the closure's interface. The closure IS the proof that validation happened.

### 3. Closures in Shen Itself

Shen has first-class closures via lambda (`/. X body`) and partial application:

```shen
\* Lambda *\
(/. X (* X X))       \* → closure that squares its argument *\

\* Partial application *\
(+ 1)                 \* → closure that increments *\

\* Named functions are closable *\
(map (/. X (* X 2)) [1 2 3])  \* → [2 4 6] *\
```

Shen's type system handles function types with `-->`:

```shen
(define square
  {number --> number}
  X -> (* X X))
```

**Can Shen specs include function-typed premises?** Yes. Shen's sequent calculus operates over any type expression, including function types:

```shen
\* Hypothetical: a guard that carries a validation function *\
(datatype validated-transform
  F : (number --> number);
  X : number;
  (>= (F X) 0) : verified;
  ===========================
  [F X] : validated-transform;)
```

This would define a guard type where a function `F` applied to `X` must produce a non-negative result. The shengen codegen doesn't currently handle `(A --> B)` type expressions — it would need to:
- Map `(number --> number)` to `func(float64) float64` in Go
- Map to `(x: number) => number` in TypeScript
- Map to `Fn(f64) -> f64` in Rust

This is a **genuine extension point**: Shen specs could define function-typed guards, and shengen could emit corresponding closure-typed fields in the generated code.

### 4. Continuation-Passing Proof Chains

The current proof chain is a sequence of struct constructions:

```go
amount, err := NewAmount(rawFloat)
tx := NewTransaction(amount, from, to)
checked, err := NewBalanceChecked(balance, tx)
transfer := NewSafeTransfer(tx, checked)
```

An alternative is **continuation-passing style (CPS)**, where each step returns a closure to the next step:

```go
// CPS proof chain — each step unlocks the next
func ValidateAmount(raw float64) func(AccountId, AccountId) func(float64) func() SafeTransfer {
    if raw < 0 { return nil }
    amount := Amount{v: raw}
    return func(from, to AccountId) func(float64) func() SafeTransfer {
        tx := Transaction{amount: amount, from: from, to: to}
        return func(balance float64) func() SafeTransfer {
            if balance < amount.v { return nil }
            checked := BalanceChecked{bal: balance, tx: tx}
            return func() SafeTransfer {
                return SafeTransfer{tx: tx, check: checked}
            }
        }
    }
}
```

**Trade-offs:**
- **Pro**: The proof chain is a single expression — you can't accidentally skip a step
- **Pro**: Intermediate values are captured, not exposed — stronger information hiding
- **Con**: Deeply nested closures hurt readability (the "callback pyramid")
- **Con**: Error handling becomes awkward (nil checks instead of err returns)
- **Con**: Doesn't play well with Go's error handling idiom

CPS proof chains are more natural in functional languages:

```typescript
// TypeScript CPS proof chain
const validateTransfer = (raw: number) =>
  Amount.create(raw).map(amount =>
    (from: AccountId, to: AccountId) =>
      Transaction.create(amount, from, to)).map(tx =>
        (balance: number) =>
          BalanceChecked.create(balance, tx).map(check =>
            SafeTransfer.create(tx, check)));
```

**Verdict**: CPS proof chains are elegant but don't add safety beyond what the struct-based approach already provides. They're worth considering for functional target languages (Haskell, OCaml, Elixir) where closures are idiomatic, but not for Go/TS where the current approach is clearer.

### 5. Closure-Based Capability Tokens

The **object-capability model** (ocap) treats unforgeable references as capabilities. A closure capturing validated state is exactly this: you can only invoke the capability (closure) if you received it through the validation path.

```go
// A closure-based capability: the function IS the authorization proof
type TransferCapability func(to AccountId) error

func AuthorizeTransfer(balance float64, from AccountId, amount Amount) TransferCapability {
    if balance < amount.Val() {
        return nil  // no capability granted
    }
    // The closure captures the validated context
    return func(to AccountId) error {
        // Execute the transfer — the closure's existence IS the proof
        return executeTransfer(from, to, amount)
    }
}
```

This pattern is interesting for **middleware and handler composition**:

```go
// Current approach: handler receives proof struct
func TransferHandler(proof shenguard.SafeTransfer) http.HandlerFunc { ... }

// Capability approach: handler receives executable closure
func TransferHandler(doTransfer TransferCapability) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        to := parseRecipient(r)
        if err := doTransfer(to); err != nil { ... }
    }
}
```

**Where this shines**: When the proof chain produces a *capability to act*, not just a *validated data structure*. The blog post's multi-tenant example (`blog/post-3-impossible-by-construction/post.md`) is a natural fit — `ResourceAccess` could be a closure `func() Resource` that, when called, fetches the resource with the proven authorization baked in.

**Where this doesn't work**: When you need to inspect the validated data (e.g., reading `tx.Amount()` for logging). Closures hide everything. You'd need accessor closures alongside the capability closure, which starts looking like the struct pattern with extra steps.

### 6. Using Shen Directly for Application Code

The README notes: "Why Shen over Coq/Lean/Agda? Turing-complete, Lisp syntax LLMs handle well, runs as subprocess."

Shen isn't just a spec language — it's a full programming language. Could Shen itself be used for domain logic?

**Current pipeline:**
```
Shen spec → shengen → Go guard types → Go application code
```

**Hypothetical direct-Shen pipeline:**
```
Shen spec + Shen application code → Shen type checker verifies EVERYTHING → FFI bridge to host language
```

**What Shen offers natively:**
- Pattern matching with `define`
- First-class closures and partial application
- The sequent-calculus type system verifying all code, not just specs
- Prolog-in-Shen for relational reasoning
- Macros for DSL construction

**Example: domain logic directly in Shen:**

```shen
\* The spec AND the logic in one place *\
(define safe-transfer
  {number --> transaction --> (number * transaction) | string}
  Bal Tx -> (let Amount (head Tx)
             (if (>= Bal Amount)
                 (@p Bal Tx)         \* returns balance-checked pair *\
                 "insufficient funds")))
```

With `(tc +)`, Shen type-checks this function against the datatype rules. If `safe-transfer` violates the `balance-invariant` datatype, the type checker rejects it.

**Trade-offs of using Shen directly:**
- **Pro**: No codegen bridge — the spec IS the code. No drift possible.
- **Pro**: Full deductive verification of application logic, not just constructors
- **Pro**: Closures, pattern matching, and type checking are all native
- **Con**: Performance — Shen runs as a subprocess, not compiled to native code
- **Con**: Integration — calling Shen from Go/TS requires subprocess calls or an FFI
- **Con**: LLM familiarity — LLMs handle Go/TS better than Shen for application code
- **Con**: Ecosystem — no HTTP server, no database drivers, no JSON parsing in Shen

**Practical middle ground**: Use Shen for the **decision kernel** — the pure logic that determines whether an operation is valid — and call it from the host language:

```go
// Go calls Shen for the decision, trusts the result
func validateTransfer(balance float64, tx Transaction) (bool, error) {
    result, err := callShen(fmt.Sprintf(
        "(safe-transfer %f [%f %s %s])",
        balance, tx.Amount.Val(), tx.From.Val(), tx.To.Val()))
    return result == "true", err
}
```

This gives you Shen's type checking over the decision logic while keeping Go for I/O, HTTP, database, etc. The closure story enters because **Shen's closures are type-checked** — if you pass a closure through a Shen function, the type checker verifies the types flow correctly.

### 7. Closure-Based Backpressure in Streaming/Reactive Context

The project name is "Shen-**Backpressure**." In reactive systems (RxJS, Akka Streams, core.async), backpressure is a flow-control mechanism: downstream signals demand to upstream via closures (callbacks, subscription functions).

**Connection to Shen-Backpressure**: The Ralph loop IS a reactive system:

```
LLM produces code → Gates consume code → Gate failures signal demand for fixes
```

Currently, backpressure is textual — gate errors are injected into the next prompt. But closures could formalize this:

```go
// Gate as a closure that captures its validation context
type Gate func(codeChange CodeChange) (GateResult, BackpressureSignal)

// BackpressureSignal is itself a closure — it knows how to describe what went wrong
type BackpressureSignal func() string

// Compose gates with backpressure propagation
func chainGates(gates ...Gate) Gate {
    return func(change CodeChange) (GateResult, BackpressureSignal) {
        for _, gate := range gates {
            result, signal := gate(change)
            if !result.Passed {
                return result, signal  // backpressure propagates
            }
        }
        return GateResult{Passed: true}, nil
    }
}
```

This makes the five-gate pipeline composable via closures, and backpressure signals carry their context (the closure captures the failed state). Not necessarily better than the current approach for the Ralph loop, but interesting for a hypothetical **streaming shengen** that validates a continuous stream of code changes.

### 8. Synthesis: Where Closures Add Real Value

| Idea | Value for Shen-Backpressure | Effort | Recommendation |
|------|----------------------------|--------|----------------|
| Closure-based guards for Python/Lua/Ruby | **High** — enables strong enforcement in languages where struct opacity is weak | Medium — new emitter pattern in shengen | Worth exploring for Python target |
| Function-typed premises in Shen specs | **Medium** — enables richer specs (transforms, predicates as types) | Medium — extend shengen's type resolver | Novel extension, good for blog post |
| CPS proof chains | **Low** — same safety as structs, worse ergonomics in Go/TS | Low | Skip for imperative targets, consider for Haskell/OCaml |
| Closure-based capabilities | **Medium** — natural for auth/middleware patterns | Low | Good pattern to document, complements structs |
| Shen for application logic | **High conceptual value** — eliminates the spec-code gap entirely | High — FFI, perf, ecosystem challenges | Research project, not near-term |
| Shen decision kernel via subprocess | **Medium** — type-checked decision logic with host-language I/O | Medium — subprocess protocol, error handling | Practical experiment |
| Reactive gate composition | **Low** — current approach works well | Medium | Over-engineering for current use case |

### 9. The Deep Connection: Closures as Proofs

The theoretical connection between closures and opaque types runs deeper than engineering convenience. In the **Curry-Howard correspondence**:

- A type is a proposition
- A value of that type is a proof of the proposition
- A function `A → B` is a proof that A implies B

So a **closure of type `A → B`** that captures a value of type `C` is a proof that "given C, A implies B." This is exactly what a validated constructor does — it's a function from (raw inputs) to (guard type), and its existence proves the invariant holds.

The current shengen approach creates **proof objects** (struct values). A closure-based approach would create **proof functions** (closures capturing validated state). Both are valid proof representations — the choice between them is about the target language's idiom and what you want to DO with the proof:

- If you want to **inspect** the proof (read fields, log values): use structs
- If you want to **execute** the proof (perform an authorized action): use closures/capabilities
- If you want both: use structs with method closures (which is close to what the current approach already does — `amount.Val()` is a method on a struct)

## Code References

- `cmd/shengen/main.go:1-120` — AST types, symbol table, type classification
- `cmd/shengen/main.go:96-102` — TypeInfo categories: wrapper, constrained, composite, guarded, alias
- `examples/payment_ts/guards_gen.ts:19-27` — TypeScript Amount with private constructor
- `examples/payment/guards_gen.go` — Go guard types (reference output)
- `sb/commands/create-shengen.md:1-50` — Language-agnostic enforcement strategies
- `sb/AGENT_PROMPT.md:26-98` — Guard type discipline rules
- `blog/post-3-impossible-by-construction/post.md` — Multi-tenant proof chain demo
- `demo/payment/specs/core.shen` — Payment domain Shen spec
- `demo/email_crud/specs/core.shen` — Email CRUD Shen spec with complex equality guards

## Architecture Documentation

The project's architecture centers on the **shengen codegen bridge** pattern: Shen specs define types deductively, shengen emits opaque types with validated constructors, and the target language's compiler enforces the proof chain. The bridge currently produces struct-based opaque types. Closures represent an alternative enforcement mechanism that could extend shengen to more target languages and enable new patterns (capabilities, CPS chains, function-typed guards).

## Historical Context (from thoughts/)

- `thoughts/shared/research/2026-03-23-codebase-overview.md` — Original project overview documenting the three-gate (now five-gate) architecture
- `thoughts/shared/research/2026-03-24-shengen-codegen-bridge.md` — Deep dive into shengen's parsing, symbol table, and code generation

## Related Research

- `thoughts/shared/research/2026-03-28-shengen-improvement-research.md` — Shengen improvement research (may cover related codegen topics)

## Web Research Findings

### Shen's Native Closure and Type System Capabilities

**Sources**: [Lambda Expressions — shenlanguage.org](https://shenlanguage.org/OSM/Lambda&Let.html), [Shen Kata #1 — mthom.github.io](http://mthom.github.io/blog/2015/06/08/shen-kata-1-type-safe-reference-cells/), [Higher Order Functions — shenlanguage.org](https://shenlanguage.org/OSM/HOF.html), [KLambda spec — Shen-Language wiki](https://github.com/Shen-Language/wiki/wiki/KLambda)

Shen has full first-class closures via `/. X body` (multi-param shorthand) and `(lambda X body)` (single-param). KLambda explicitly states lambdas "capture local scope from the lexical context." Function types use `-->` (right-associative): `A --> B --> C` means `A --> (B --> C)`. All functions are implicitly curried.

**Critical finding — Shen's type system already supports closure-typed opaque types.** The reference-cell kata (`ref A`) demonstrates this:

```shen
(datatype ref-types
  V : (one-cell-vector A);
  _____________________________________________
  (@p (ref-reader V) (ref-writer V)) : (ref A);)
```

The `(ref A)` type is a pair of closures (`(lazy A)` reader + `(A --> unit)` writer) that close over a shared mutable vector. The type checker only accepts the specific pair produced by the constructor — direct vector access is excluded by the type system itself. This is opaque-type construction via sequent rules, not module boundaries.

The `(lazy A)` type names a frozen closure (thunk) created with `(freeze body)`, thawed with `(thaw expr)`.

**Implication for shengen**: Shen specs could define function-typed premises:

```shen
(datatype transfer-action
  F : (account-state --> account-state);
  Check : balance-checked;
  =============================================
  [@p F Check] : transfer-action;)
```

This bundles a callable closure with a proof certificate — the **hybrid approach** (struct containing closure fields).

### The Lambda Calculus / Object-Capability Connection

**Sources**: [Object-capability model — Wikipedia](https://en.wikipedia.org/wiki/Object-capability_model), [Lambda calculus and capability security — killerstorm](https://killerstorm.github.io/software/2026/01/06/caps.html), [awesome-ocap](https://github.com/dckc/awesome-ocap)

The connection between closures and capabilities is exact, not metaphorical. From killerstorm's analysis: "Lambda calculus formalism simply does not provide a way to reference objects which are not passed as parameters and not visible via free variables. If free variables (globals) are forbidden, different parts of a program are naturally isolated from each other."

A closure capturing validated state is an unforgeable capability by construction. The `mkRevocable` pattern from the E language shows attenuation:

```javascript
function mkRevocable(fn) {
  let target = fn;
  return {
    wrapper: (...args) => target ? target(...args) : undefined,
    revoke: () => { target = null; }
  };
}
```

### Struct vs Closure Guard Trade-offs (from literature)

**Sources**: [Names are not type safety — Alexis King](https://lexi-lambda.github.io/blog/2020/11/01/names-are-not-type-safety/), [The Typestate Pattern in Rust — Cliffle](https://cliffle.com/blog/rust-typestate/), [Smart constructors — HaskellWiki](https://wiki.haskell.org/Smart_constructors)

| Property | Struct + constructor | Closure |
|----------|---------------------|---------|
| Serializable / loggable | Yes | No |
| Storable in collections | Yes | Difficult |
| Carries behavior | No | Yes |
| Revocable | Needs wrapper | Natural (`mkRevocable`) |
| Language-agnostic | Moderate | Universal |
| Zero-cost abstraction | Yes (newtypes erased) | Usually no |
| Proof-by-possession | Partial (Alexis King: "newtypes are security blankets") | Full |
| Formal verification friendly | Yes | Depends |

Alexis King's critique is important: "Newtypes like these are security blankets... forcing programmers to jump through a few hoops is not type safety." A `ValidatedEmail` struct doesn't contain the proof — just the raw data with a name tag. A closure-based proof actually captures the validated computation.

The **hybrid approach** (struct containing closures) gets both: the outer struct is storable and typed; the inner closures carry behavior and proof. This matches the Shen `(ref A)` pattern.

### Codegen Systems and Closures

**Sources**: [Zod](https://github.com/colinhacks/zod), [tRPC validators](https://trpc.io/docs/server/validators), [Prisma generators](https://www.prisma.io/docs/orm/prisma-schema/overview/generators)

No mainstream codegen system currently emits closures as the primary output. Zod comes closest — schemas are themselves closure objects with `parse()`, `safeParse()`, `transform()`. The schema IS the capability to validate. tRPC builds on this: procedures are closure chains over Zod validators.

This is an underexplored design space for shengen.

### Key Limitation: `verified` vs Closure-Capture

The `(pred X) : verified` mechanism in existing specs is **stronger** than a closure-typed field in one specific way: it runs the predicate at type-checking time and records it as a sequent assumption. A closure field merely asserts function type — it doesn't run the function. The `verified` pattern is more tightly proof-carrying than a closure field would be.

## External References

- [Shen Kata #1: Type-safe reference cells](http://mthom.github.io/blog/2015/06/08/shen-kata-1-type-safe-reference-cells/) — The definitive example of closure types in Shen datatype rules
- [Poor man's dynamic dispatch with types in Shen](http://mthom.github.io/blog/2016/10/10/poor-man-s-dynamic-dispatch-with-types-in-shen/) — More advanced Shen type patterns
- [Names are not type safety — Alexis King](https://lexi-lambda.github.io/blog/2020/11/01/names-are-not-type-safety/) — Why newtypes alone are insufficient
- [Lambda calculus and capability security](https://killerstorm.github.io/software/2026/01/06/caps.html) — The exact ocap/lambda connection
- [The Typestate Pattern in Rust](https://cliffle.com/blog/rust-typestate/) — Trade-offs with typestate vs smart constructors
- [Parse, don't validate](https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/) — The philosophical foundation
- [Object-capability model — Wikipedia](https://en.wikipedia.org/wiki/Object-capability_model)
- [awesome-ocap](https://github.com/dckc/awesome-ocap) — Curated list of ocap resources
- [Idris 2: Quantitative Type Theory](https://arxiv.org/abs/2104.00480) — Linear types + dependent types for capability proofs

## Open Questions

1. **Python shengen**: Would closure-based guards be the right approach for a Python target? Or would `dataclasses(frozen=True)` + `__post_init__` validation be sufficient despite `__dict__` bypass?
2. **Function-typed premises in shengen**: The Shen `(ref A)` kata proves closure types work in specs. Could shengen emit function-typed fields? The Go emitter would need to handle `func(T) U` fields.
3. **Shen decision kernel**: What's the latency of calling shen-go as a subprocess for individual decisions? Is it practical for hot paths?
4. **Clojure target**: Given the project already has Clojure tooling (mcp-clj REPL), is a Clojure shengen with closure-based protocols a natural next step?
5. **LLM interaction**: Do LLMs handle closure-based guard types as well as struct-based ones? The struct pattern has explicit constructors that are easy to find and call. Closure patterns may be less discoverable.
6. **Hybrid guards**: The `(ref A)` pattern bundles closures inside a struct. Could shengen emit hybrid types — structs with both data fields and capability-closure fields?
7. **Alexis King's critique**: Does Shen-Backpressure's approach fully address "names are not type safety"? The `verified` premises arguably go beyond newtypes — they run actual predicates at construction time.
