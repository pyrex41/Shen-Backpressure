---
title: "One Spec, Every Language: How Guard Types Change Shape Across Go, TypeScript, Rust, and Python"
date: 2026-03-29
draft: false
description: "The same Shen spec generates guard types in any language. But 'guard type' means something different when you have a compiler versus when you don't."
tags: ["shen", "codegen", "go", "typescript", "rust", "python", "backpressure", "formal-verification"]
---

*The same Shen spec generates guard types in any language. But "guard type" means something different when you have a compiler versus when you don't.*

---

In the [previous post](/posts/impossible-by-construction/), I showed how a Shen sequent-calculus spec generates Go types that make cross-tenant access impossible to accidentally bypass. The compiler enforces the proof chain — unexported fields mean the only way to get a `TenantAccess` is through its constructor, which requires an `AuthenticatedUser`, which requires a valid JWT.

A natural question: **does this only work in Go?**

The honest answer is that the approach works in every language, but it works *differently*. In some languages the compiler catches bypass at build time. In others, the type checker catches it before you ship. In others, nothing catches it until runtime. Understanding where your language falls on this spectrum is the key to knowing what you're actually getting.

## The Same Spec, Four Languages

Here's a simple spec — a non-negative amount type:

```shen
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)
```

One Shen datatype. One invariant: the number must be non-negative. Now watch what happens when shengen targets four different languages.

### Go: The Compiler Says No

```go
type Amount struct{ v float64 }

func NewAmount(x float64) (Amount, error) {
    if !(x >= 0) {
        return Amount{}, fmt.Errorf("x must be >= 0: %v", x)
    }
    return Amount{v: x}, nil
}

func (t Amount) Val() float64 { return t.v }
```

The field `v` is lowercase — unexported. From outside the `shenguard` package:

```go
Amount{v: 50}  // compile error: unknown field v in struct literal
```

There is no syntax to construct an `Amount` without calling `NewAmount`. The Go compiler enforces this. Not a linter warning, not a convention — a hard compile error that blocks `go build`.

### Rust: The Compiler Says No (Even Harder)

```rust
pub struct Amount {
    v: f64,  // private by default
}

impl Amount {
    pub fn new(x: f64) -> Result<Amount, String> {
        if !(x >= 0.0) {
            return Err(format!("x must be >= 0: {}", x));
        }
        Ok(Amount { v: x })
    }

    pub fn val(&self) -> f64 { self.v }
}
```

Same idea — `v` is private to the module. But Rust goes further. Unless you `#[derive(Clone)]`, the value can't even be copied. And Rust's ownership system means the guard type is *consumed* when you move it into a downstream constructor — you can't accidentally reuse a stale proof. This is enforcement Go can't express.

### TypeScript: The Type Checker Says No (But JavaScript Says Sure)

```typescript
export class Amount {
  private readonly _v: number;
  private constructor(v: number) { this._v = v; }
  static create(x: number): Amount {
    if (!(x >= 0)) throw new Error(`x must be >= 0: ${x}`);
    return new Amount(x);
  }
  val(): number { return this._v; }
}
```

From TypeScript:

```typescript
new Amount(50);     // tsc error: Constructor of class 'Amount' is private
amount._v;          // tsc error: Property '_v' is private
```

The TypeScript compiler catches it. But after transpilation to JavaScript, `private` disappears. The emitted JS is a plain class with a normal constructor and normal properties. `new Amount(50)` is valid JavaScript. `amount._v` is readable.

This means the guarantee holds **within the TypeScript compilation boundary**. If all your code is TypeScript and you trust `tsc`, you get compile-time enforcement. If anything touches these types from plain JavaScript — a test file, a `JSON.parse()` cast, a `require()` from a JS module — the structural guarantee vanishes. The runtime `throw` in `create()` survives transpilation, so the validation still runs if you call the right function. But nobody forces you to call the right function.

### Python: Nobody Says No (Unless You Ask Nicely)

```python
from dataclasses import dataclass

@dataclass(frozen=True)
class Amount:
    _v: float

    def __post_init__(self):
        if not (self._v >= 0):
            raise ValueError(f"v must be >= 0: {self._v}")

    @staticmethod
    def create(x: float) -> "Amount":
        return Amount(_v=x)

    def val(self) -> float:
        return self._v
```

From Python:

```python
Amount(_v=50)       # works fine — Python has no private fields
Amount(_v=-10)      # raises ValueError from __post_init__
object.__new__(Amount)  # bypasses __post_init__ entirely
```

There is no compile-time enforcement. `frozen=True` prevents reassignment after construction, but `object.__new__()` skips `__init__` and `__post_init__` completely. The leading underscore `_v` is a *convention*, not a barrier — Python's philosophy is "we're all consenting adults here."

The `__post_init__` validation runs when you construct normally. But nothing in the language forces you to construct normally.

## The Three-Layer Model

What's happening here is that enforcement decomposes into three independent layers:

```
Layer 1: Runtime validation     — constructor checks (universal)
Layer 2: Compiler/type-checker  — structural opacity (language-dependent)
Layer 3: Shen tc+               — deductive proof over ALL inputs (universal)
```

Every language gets layers 1 and 3. Layer 2 is the bonus.

- **Layer 1** is just `if !(condition) { return error }` in the constructor. It works in every language. If you call `NewAmount(-5)` in Go or `Amount.create(-5)` in TypeScript or `Amount.create(-5)` in Python, you get an error. This is runtime enforcement — it catches violations when the code actually executes.

- **Layer 2** is the structural guarantee that you *must* go through the constructor. Go gives you this through unexported fields. Rust gives you this through module privacy (plus ownership). TypeScript gives you this through `private` (compile-time only). Python doesn't give you this at all.

- **Layer 3** is the Shen type checker running `tc+` on the spec. This verifies that the spec itself is internally consistent — the rules don't contradict each other, the proof chain is valid. It runs as a separate subprocess regardless of target language. A bad spec fails Gate 4 whether you're targeting Go or Python.

## What Happens to the Five Gates

The [five-gate loop](/posts/impossible-by-construction/#the-backpressure-hierarchy) from the previous post — shengen, test, build, shen tc+, tcb audit — still applies to every language. But the gates mean different things:

| Gate | Go | TypeScript | Python |
|------|------|------|------|
| 1. shengen | Regenerate `.go` | Regenerate `.ts` | Regenerate `.py` |
| 2. test | `go test` | `jest` | `pytest` |
| 3. build | `go build` — **catches bypass** | `tsc` — **catches bypass (compile-time only)** | `mypy` — catches type mismatches, **not privacy bypass** |
| 4. shen tc+ | Spec consistency | Spec consistency | Spec consistency |
| 5. tcb audit | Diff generated code | Diff generated code | Diff generated code |

Gate 3 is where the languages diverge. In Go and Rust, it's a hard wall — the LLM literally cannot write `Amount{v: 50}` and get past `go build`. In TypeScript, it's a soft wall — `tsc` rejects it, but the generated JS allows it. In Python, `mypy` catches type mismatches (passing a `float` where an `Amount` is expected) but cannot enforce field privacy.

Here's the critical thing: **Gate 3 degrades gracefully, but Gate 5 picks up the slack.**

Gate 5 — the TCB audit — re-runs shengen and diffs the output against the committed generated file. If someone (or some LLM) hand-edits `guards_gen.py` to skip the validation, Gate 5 catches it. This matters more for Python than for Go, because in Go the compiler already prevents bypass. In Python, Gate 5 is a primary defense, not just defense-in-depth.

## The Proof Chain Across Languages

The enforcement spectrum matters most for **proof chains** — where guard types require other guard types as inputs:

```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

In Go, the proof chain is compiler-enforced end to end:

```go
amount, err := shenguard.NewAmount(rawFloat)        // must validate
tx := shenguard.NewTransaction(amount, from, to)     // requires Amount, not float64
checked, err := shenguard.NewBalanceChecked(bal, tx)  // requires Transaction
transfer := shenguard.NewSafeTransfer(tx, checked)    // requires BalanceChecked
```

You cannot skip a step. `NewSafeTransfer` requires a `BalanceChecked`, which requires a `Transaction`, which requires an `Amount`. The Go compiler rejects any attempt to pass raw primitives where guard types are expected.

In TypeScript, the same chain works at the `tsc` level:

```typescript
const amount = Amount.create(rawNumber);
const tx = Transaction.create(amount, from, to);
const checked = BalanceChecked.create(bal, tx);
const transfer = SafeTransfer.create(tx, checked);
```

`tsc` rejects `Transaction.create(rawNumber, rawString, rawString)` because `number` is not assignable to `Amount`. But after transpilation, JavaScript doesn't enforce this — you could pass anything.

In Python with mypy:

```python
amount = Amount.create(raw_float)
tx = Transaction.create(amount, from_id, to_id)
checked = BalanceChecked.create(bal, tx)
transfer = SafeTransfer.create(tx, checked)
```

`mypy` catches `Transaction.create(raw_float, raw_str, raw_str)` if you're running it in strict mode. But at runtime, Python happily accepts any arguments. The validation in `create()` catches invalid *values*, but nothing catches invalid *types* unless you opt into mypy.

## Where This Gets Interesting: Closures

For languages in the "nobody says no" tier — Python, Ruby, Lua, plain JavaScript — there's an alternative enforcement mechanism: **closures**.

A closure captures validated state in its scope. You can't access the captured variables from outside the closure. This is genuine information hiding, not a convention:

```python
def make_amount(x: float):
    if not (x >= 0):
        raise ValueError(f"x must be >= 0: {x}")
    # x is captured in closure scope — inaccessible from outside
    def val():
        return x
    return val

amt = make_amount(50)
amt()  # 50 — the only way to get the value
```

There's no `_v` attribute to access. There's no `object.__new__()` bypass. The variable `x` exists only in the closure's scope. The closure IS the opaque type.

This is a real trade-off. Closure-based guards are stronger against bypass but weaker for everything else — you can't serialize them, store them in collections easily, or inspect their contents for logging. For most applications, the `@dataclass(frozen=True)` approach with runtime validation is pragmatic enough. But for high-assurance Python code, closures are worth considering.

## The Universal Part

Here's what stays constant across every language:

**The Shen spec doesn't change.** The same `specs/core.shen` generates guard types for Go, TypeScript, Rust, and Python. The formal verification (Gate 4: `shen tc+`) runs identically regardless of target. The spec is the single source of truth.

**The constructor validation doesn't change.** Every language gets `if !(condition) { error }` in the constructor body. Whether it's `fmt.Errorf` or `throw new Error` or `raise ValueError`, the runtime check is semantically identical.

**The proof chain structure doesn't change.** Guard types require other guard types as constructor arguments. This is the same pattern in every language — only the syntax differs.

**The five-gate architecture doesn't change.** Shengen regenerates, tests run, the build/typecheck tool runs, Shen verifies the spec, the audit diffs the output. The tools filling each gate slot change; the architecture doesn't.

What changes is **one thing**: how hard the language makes it to skip the constructor. That's Layer 2. It ranges from "compile error" to "nothing." And the project is designed so that the other layers compensate when Layer 2 is weak.

## The Meta-Insight: A Spec That Compiles Itself Into Any Language

The deepest thing about this approach is that the Shen spec is a **language-independent formal artifact** that compiles into language-specific enforcement. The spec doesn't know about Go's unexported fields or TypeScript's `private` keyword or Python's `__post_init__`. It defines inference rules — premises above a line, conclusion below. The codegen bridge (shengen) translates those rules into whatever enforcement mechanism the target language provides.

This means adding a new target language doesn't require changing the spec. It requires writing a new shengen emitter — a function that reads the same AST and writes different syntax. The [create-shengen command](https://github.com/pyrex41/Shen-Backpressure) is designed for exactly this: an 875-line specification that teaches any LLM how to build shengen for a new target language. It parameterizes every decision — enforcement mechanism, error handling, naming conventions, value accessors — by target language.

The spec compiles itself. The language determines how strong the compiled output is. And the five-gate loop provides the scaffolding to make even weak enforcement useful, because the gates that don't depend on the language (Shen tc+, TCB audit, tests) catch what the compiler can't.

---

*[Shen-Backpressure](https://github.com/pyrex41/Shen-Backpressure) is open source. The Go shengen tool is at `cmd/shengen/`. TypeScript examples are at `examples/payment_ts/` and `examples/email_crud_ts/`. The create-shengen spec for building shengen in any target language is at `sb/commands/create-shengen.md`.*

*Previous: [Making Cross-Tenant Access Impossible to Accidentally Bypass](/posts/impossible-by-construction/)*
