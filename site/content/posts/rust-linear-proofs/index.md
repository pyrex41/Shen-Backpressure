---
title: "Rust: Where Proofs Are Linear and Types Have State"
date: 2026-03-29
draft: false
description: "Rust's ownership system turns guard types into linear proofs that can only be used once. Combined with typestate, you get compile-time enforcement of multi-step verification flows."
tags: ["shen", "rust", "backpressure", "formal-verification", "typestate", "linear-types"]
---

*Rust's ownership system turns guard types into linear proofs that can only be used once. Combined with typestate, you get compile-time enforcement of multi-step verification flows.*

---

In the [previous post](/posts/one-spec-every-language/), I showed how the same Shen spec generates guard types across Go, TypeScript, Rust, and Python -- and how enforcement degrades across that spectrum. Go gives you unexported fields. TypeScript gives you `private` that vanishes after transpilation. Python gives you conventions.

Rust gives you something none of the others can: **linear proofs**.

This post is about what happens when you take the guard type pattern seriously in a language with ownership, move semantics, and no implicit copying. The short version: proofs become values that can only be used once, state machines become types, and the compiler rejects entire categories of bugs that other languages can only catch with tests.

## Why Rust Is the Strongest Target

Every language in the [three-layer model](/posts/one-spec-every-language/#the-three-layer-model) gets layers 1 and 3 -- runtime validation and deductive spec verification via Shen tc+. Layer 2 (compile-time structural opacity) is where languages diverge. Rust sits at the top of that spectrum for three reasons:

**Private fields are truly private.** In Rust, struct fields are private by default. There is no reflection API that reads private fields. There is no equivalent of Go's `reflect` package or Java's `setAccessible(true)`. The only way to touch a private field is `unsafe` code -- and `unsafe` blocks are syntactically marked, greppable, and auditable. You can ban them in CI with a one-line clippy lint.

**There are no zero values.** Go has a footgun: any struct can be constructed as its zero value. `TenantAccess{}` compiles in Go if you are inside the same package (or if fields were accidentally exported). Rust has no zero-value construction. If a struct has private fields, you cannot construct it at all from outside the module -- not with defaults, not with `mem::zeroed()` (that requires `unsafe`), not with anything. The constructor is the only door, and it is the only door unconditionally.

**`unsafe` is explicit and auditable.** Every escape hatch in Rust is marked with the `unsafe` keyword. This makes the trusted computing base trivially identifiable: grep for `unsafe`, audit those blocks, done. Compare this to Go, where `reflect.NewAt` or `unsafe.Pointer` can quietly break encapsulation in code that looks otherwise normal.

These properties mean that shengen-rs output is not just "compiler-enforced" in the same sense as Go. It is enforced with a strictly smaller trusted computing base.

## Linear Proofs via Ownership

Here is the insight that makes Rust qualitatively different from Go and TypeScript for guard types: **a proof value that doesn't implement Clone or Copy can only be used once.**

In the [payment example](/posts/one-spec-every-language/#the-proof-chain-across-languages), a `BalanceChecked` proves that an account has sufficient funds for a transaction. In Go, nothing stops you from passing the same `BalanceChecked` to two different downstream constructors -- the proof is a struct, structs are copyable, and the compiler has no opinion about it.

In Rust, if you don't derive `Clone` or `Copy` on `BalanceChecked`, the ownership system enforces single use:

```rust
let tx1 = Transaction::new(amount.clone(), from.clone(), to.clone());
let tx2 = Transaction::new(amount.clone(), from.clone(), to2.clone());

let check = BalanceChecked::new(100.0, tx1)?;
let safe1 = check.into_safe_transfer(tx2);  // moves `check`
let safe2 = check.into_safe_transfer(tx2);  // ERROR: value used after move
```

The compiler rejects the second line. `check` was moved into `safe1` -- it no longer exists. You cannot reuse a stale balance check because the value is gone. Not invalidated, not marked as consumed in some runtime flag -- *gone from the type system*. The variable `check` is no longer in scope after the move.

This is linear typing in practice. Not the full linear logic of Girard -- Rust technically has affine types (you can drop a value without using it) -- but for guard types, the effect is the same: **a proof is consumed when used, and the compiler enforces it**.

### Why This Matters for AI Loops

In an AI coding loop, the LLM generates code, the compiler checks it, and errors feed back as [backpressure](https://banay.me/dont-waste-your-backpressure). The ownership system adds a new category of backpressure that Go and TypeScript cannot provide.

Consider an LLM writing a payment flow. It generates code that checks the balance once and then uses the proof for two transfers. In Go, this compiles. The bug is a logic error that only tests (or a careful reviewer) would catch. In Rust, `cargo build` fails with `use of moved value: check`. The LLM gets immediate feedback, fixes the code to perform two separate balance checks, and the proof chain is restored.

The compiler is teaching the LLM about linearity without the LLM needing to understand the concept. It just sees "this doesn't compile" and adjusts. That is the backpressure thesis in its purest form.

## The Typestate Pattern

Linear proofs handle single-step consumption. But what about multi-step verification flows where a value must pass through several states in order? Rust's type system can encode this too, using the **typestate pattern**.

The idea: a generic struct parameterized by a state marker. Methods are only available when the type is in the right state. Transitions consume the old state and produce a new one.

```rust
use std::marker::PhantomData;

// State markers — zero-sized types, never instantiated
pub struct Unchecked;
pub struct BalanceVerified;
pub struct Authorized;

pub struct Transfer<S> {
    amount: f64,
    from: String,
    to: String,
    _state: PhantomData<S>,
}

impl Transfer<Unchecked> {
    pub fn new(amount: f64, from: String, to: String) -> Self {
        Transfer { amount, from, to, _state: PhantomData }
    }

    pub fn verify_balance(self, account_balance: f64)
        -> Result<Transfer<BalanceVerified>, GuardError>
    {
        if account_balance < self.amount {
            return Err(GuardError {
                message: "insufficient balance".to_string(),
            });
        }
        Ok(Transfer {
            amount: self.amount,
            from: self.from,
            to: self.to,
            _state: PhantomData,
        })
    }
}

impl Transfer<BalanceVerified> {
    pub fn authorize(self, auth_token: &str)
        -> Result<Transfer<Authorized>, GuardError>
    {
        if auth_token.is_empty() {
            return Err(GuardError {
                message: "empty auth token".to_string(),
            });
        }
        Ok(Transfer {
            amount: self.amount,
            from: self.from,
            to: self.to,
            _state: PhantomData,
        })
    }
}

impl Transfer<Authorized> {
    pub fn execute(self) -> Receipt {
        // Only reachable after both verify_balance and authorize
        Receipt { amount: self.amount, from: self.from, to: self.to }
    }
}
```

Now try to skip a step:

```rust
let t = Transfer::new(500.0, "alice".into(), "bob".into());
t.authorize("token123");  // ERROR: no method named `authorize` found
                           // for `Transfer<Unchecked>` in the current scope
```

The method `.authorize()` does not exist on `Transfer<Unchecked>`. It is not hidden, not gated behind a runtime check -- it literally does not exist for that type. The compiler error is not "you called it wrong," it is "there is no such method." And because each transition consumes `self` by move, you cannot hold onto a `Transfer<Unchecked>` after calling `.verify_balance()` on it. The old state is gone.

This is the proof chain from the [first post](/posts/impossible-by-construction/) -- JWT, then tenant access, then resource access -- expressed as a state machine in the type system. Each step proves a property and transitions to the next state. The compiler ensures the steps happen in order, exactly once.

## Sealed Traits for Sum Types

When your Shen spec defines a sum type -- say, a `TransferResult` that is either `Success` or `Failure` -- shengen-rs emits it as a trait with the sealed pattern:

```rust
mod private {
    pub trait Sealed {}
}

pub trait TransferResult: private::Sealed {
    fn description(&self) -> &str;
}

pub struct Success { /* ... */ }
pub struct Failure { /* ... */ }

impl private::Sealed for Success {}
impl private::Sealed for Failure {}
impl TransferResult for Success {
    fn description(&self) -> &str { "transfer completed" }
}
impl TransferResult for Failure {
    fn description(&self) -> &str { "transfer failed" }
}
```

The `Sealed` trait lives in a private module. External crates can see `TransferResult` and call its methods, but they cannot implement it -- because implementing `TransferResult` requires implementing `Sealed`, and `Sealed` is not accessible outside the crate. The set of variants is closed. An LLM cannot accidentally add a `Pending` variant that sidesteps the proof chain.

## #[non_exhaustive]

Rust has one more trick that matters for guard types: the `#[non_exhaustive]` attribute. When applied to a struct, it prevents code outside the defining crate from using struct literal syntax or exhaustive destructuring:

```rust
#[non_exhaustive]
pub struct BalanceChecked {
    bal: f64,
    tx: Transaction,
}
```

Even if the fields were somehow public, external crates could not write `BalanceChecked { bal: 100.0, tx: t }` -- the `#[non_exhaustive]` attribute makes the struct permanently open to new fields from the compiler's perspective. This is defense in depth: private fields already prevent direct construction, and `#[non_exhaustive]` adds a second barrier.

## Serde Gating

Rust's ecosystem defaults to `#[derive(Serialize, Deserialize)]` on everything. For guard types, this is a vulnerability. If `BalanceChecked` derives `Deserialize`, an attacker (or a confused LLM) can construct one from JSON without going through the constructor:

```rust
// BAD: bypasses the balance check entirely
let check: BalanceChecked = serde_json::from_str(
    r#"{"bal": 1000000, "tx": ...}"#
)?;
```

The fix: **never derive `Deserialize` on guard types.** Instead, provide a `from_json` method that deserializes raw fields into a temporary struct and then feeds them through the constructor chain:

```rust
impl BalanceChecked {
    pub fn from_json(json: &str) -> Result<Self, GuardError> {
        #[derive(serde::Deserialize)]
        struct Raw { bal: f64, tx_json: String }

        let raw: Raw = serde_json::from_str(json)
            .map_err(|e| GuardError {
                message: format!("invalid JSON: {}", e),
            })?;

        let tx = Transaction::from_json(&raw.tx_json)?;
        BalanceChecked::new(raw.bal, tx)
    }
}
```

The `Raw` struct derives `Deserialize` -- it is a plain data carrier with no invariants. The guard type does not. Every deserialization path goes through the constructor, and the constructor validates. This is the same "parse, don't validate" principle, applied to the serialization boundary.

## The Hardened Flag

The standard shengen-rs output is already strong -- private fields, constructors, no zero values. But there is a gap between "strong" and "hardened." The standard output does not suppress `Clone`, does not add `#[non_exhaustive]`, and does not gate serde.

`shengen-rs --mode hardened` closes these gaps. It adds:

1. **No Clone/Copy on guarded types.** `BalanceChecked`, `SafeTransfer`, and any type whose constructor enforces a `verified` premise become move-only. Wrapper types like `AccountId` still derive `Clone` -- they carry no invariant that staleness could violate.

2. **`#[non_exhaustive]` on all types.** Defense in depth against struct literal construction from external crates.

3. **Sealed traits on sum types.** The `mod private { pub trait Sealed {} }` pattern is emitted automatically for any Shen union type.

4. **No `Deserialize` on guard types.** Only `from_json` methods that re-validate through the constructor chain.

Compare the [standard output](https://github.com/pyrex41/Shen-Backpressure/blob/main/examples/payment_rs/guards_gen.rs) with the [hardened output](https://github.com/pyrex41/Shen-Backpressure/blob/main/examples/payment_rs/guards_gen_hardened.rs). The structural difference is small -- a few missing derive macros, a few added attributes. The semantic difference is large: guarded types become linear proofs with closed variant sets and no serialization backdoor.

## When to Relax

Not every type needs linear semantics. The hardened mode distinguishes between two categories:

**Wrapper types** carry a value with at most a simple constraint (non-empty string, non-negative number). `AccountId` is a wrapper. `Amount` is a wrapper. These SHOULD derive `Clone` -- you will need to pass an `AccountId` to multiple functions, clone it into log messages, store it in multiple data structures. There is no staleness risk because the invariant (non-empty, non-negative) does not change over time.

**Guarded types** carry a proof that depends on external state. `BalanceChecked` proves that an account had sufficient funds *at the time of the check*. `TenantAccess` proves that a user was a member of a tenant *at the time of the lookup*. These should NOT be cloneable, because a clone is a second proof from the same check -- and the state may have changed.

The heuristic is simple: if the type's constructor has `verified` premises that reference external state, suppress `Clone`. If it is a pure value constraint, allow it.

## The Backpressure Story

This post is about Rust specifically, but the broader point connects back to the [backpressure hierarchy](/posts/impossible-by-construction/#the-backpressure-hierarchy):

```
Level 0: Syntax        -- does it parse?
Level 1: Types         -- does it compile?
Level 2: Tests         -- do specific cases pass?
Level 3: Proof chain   -- are invariants enforced for ALL inputs?
Level 4: Deductive     -- is the spec itself consistent?
```

Rust's ownership system operates at Level 1 but catches errors that most languages only catch at Level 2 or 3. "Value used after move" is a type error, not a test failure. "No method named `authorize` found for `Transfer<Unchecked>`" is a type error, not a runtime panic. The compiler is doing proof-chain-level work through the type system.

For AI coding loops, this means the feedback cycle is tighter. The LLM does not need to write a test that catches proof reuse -- the compiler catches it. The LLM does not need to reason about state machine transitions -- the compiler enforces them. Every error message is a specific, actionable correction: "you moved this value on line 42, you can't use it on line 43." That is exactly the kind of backpressure that makes autonomous coding loops converge.

---

*[Shen-Backpressure](https://github.com/pyrex41/Shen-Backpressure) is open source. The Rust guard type examples are at `examples/payment_rs/`. The standard and hardened outputs are `guards_gen.rs` and `guards_gen_hardened.rs` respectively.*

*Previous: [One Spec, Every Language](/posts/one-spec-every-language/) | [Making Cross-Tenant Access Impossible to Accidentally Bypass](/posts/impossible-by-construction/)*
