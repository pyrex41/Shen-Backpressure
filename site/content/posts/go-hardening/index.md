---
title: "Hardening Go Guard Types: Sealed Interfaces, Zero-Value Traps, and JSON Re-Validation"
date: 2026-03-29
draft: false
description: "Go's unexported fields get you 90% of the way. Sealed interfaces, zero-value traps, and UnmarshalJSON get the rest."
tags: ["shen", "go", "backpressure", "formal-verification", "hardening", "security"]
---

*Go's unexported fields get you 90% of the way. Sealed interfaces, zero-value traps, and UnmarshalJSON get the rest.*

---

In the [previous posts](/posts/impossible-by-construction/), I showed that Go's unexported fields make it impossible to construct a guard type from outside the `shenguard` package without going through the validated constructor. The Go compiler rejects `Amount{v: 50}` from external code. No syntax, no workaround, no escape hatch.

That blocks the most obvious attack: direct construction. But it does not block every attack. This post covers three gaps in the baseline defense and three hardening techniques that close them.

## The Baseline: What Unexported Fields Buy You

Quick recap. shengen generates guard types with lowercase fields:

```go
type Amount struct {
    v float64
}

func NewAmount(x float64) (Amount, error) {
    if !(x >= 0) {
        return Amount{}, fmt.Errorf("x must be >= 0: %v", x)
    }
    return Amount{v: x}, nil
}

func (t Amount) Val() float64 { return t.v }
```

From outside `shenguard`:

```go
Amount{v: 50}   // compile error: unknown field v in struct literal
```

The only path to an `Amount` is `NewAmount()`, which validates the invariant. This is the structural guarantee. It blocks what I call **Attack A: direct construction**.

Good. But not sufficient.

## The Gaps: Three Attacks the Baseline Doesn't Block

### Attack C: Zero-Value Exploitation

Every Go type has a zero value. For structs, that's all fields set to their zero. So:

```go
var a Amount  // Amount{v: 0.0}
```

This compiles. It runs. You now have an `Amount` with `v == 0.0`. If zero is a valid amount in your domain, this is indistinguishable from a properly constructed `Amount`. The proof chain was never executed -- no call to `NewAmount` happened -- but the value looks legitimate.

Even if zero happens to satisfy the invariant (`0 >= 0` is true), the *validation path was bypassed*. The proof chain requires that validation *happened*, not merely that the final value would have passed. A zero-value `Amount` proves nothing about whether anyone checked anything.

### Attack D: Reflection

Go's `reflect` package can read and write unexported fields:

```go
var a Amount
v := reflect.ValueOf(&a).Elem().FieldByName("v")
v.SetFloat(999.0)
// a.Val() == 999.0, no constructor called
```

This bypasses the constructor entirely. The resulting `Amount` holds an arbitrary value with no validation. The `reflect` package is designed for serialization libraries and debugging tools, but in the hands of a confused developer or an LLM hallucinating a workaround, it's a hole in the proof chain.

### Attack E: JSON Unmarshaling

The standard `encoding/json` package uses reflection internally. If someone unmarshals JSON into a guard type:

```go
var a Amount
json.Unmarshal([]byte(`-50`), &a)
```

The default behavior depends on field export rules -- unexported fields are skipped by `encoding/json`, so this would actually leave `a` at its zero value. But the subtlety is that developers expecting JSON round-tripping will add exported fields or custom marshaling, and if they do it wrong, they open a path around the constructor.

The deeper issue: any code that deserializes guard types from external input (HTTP bodies, message queues, database rows) must re-validate through the constructor. Otherwise the proof chain has a gap between "data arrived" and "data was checked."

## Sealed Interfaces: Hide the Concrete Type

The first hardening technique eliminates Attack C entirely by hiding the struct behind an interface with an unexported method:

```go
type Amount interface {
    Val() float64
    isAmount()  // unexported -- seals the interface
}

type amount struct {
    v     float64
    valid bool
}

func (a *amount) Val() float64 {
    if !a.valid {
        panic("shenguard: use of unvalidated Amount")
    }
    return a.v
}

func (a *amount) isAmount() {}

func NewAmount(x float64) (Amount, error) {
    if !(x >= 0) {
        return nil, fmt.Errorf("x must be >= 0: %v", x)
    }
    return &amount{v: x, valid: true}, nil
}
```

Three things changed:

1. **The concrete type is unexported.** External code cannot name `amount` (lowercase). It can only see the `Amount` interface.

2. **The interface has an unexported method.** The `isAmount()` method is lowercase, which means no type outside the `shenguard` package can implement the `Amount` interface. The interface is *sealed* -- only types within `shenguard` satisfy it.

3. **`var a Amount` gives `nil`, not a zero struct.** Interface zero values are `nil`. Any attempt to call `a.Val()` on a nil `Amount` panics immediately. There is no silent zero-value bypass -- the failure is loud and immediate.

External code literally cannot:
- Name the concrete type (`amount` is unexported)
- Implement the interface (the `isAmount()` method is unexported)
- Get a non-nil `Amount` without calling `NewAmount()`
- Use a zero-value `Amount` without panicking

Attack C is dead. The zero value of a sealed interface is `nil`, and `nil` panics on method calls. There is no "looks valid but wasn't validated" state.

## Zero-Value Trap: The `valid` Flag

Even without sealed interfaces, you can close the zero-value gap with a `valid` flag on the struct:

```go
type Amount struct {
    v     float64
    valid bool
}

func NewAmount(x float64) (Amount, error) {
    if !(x >= 0) {
        return Amount{}, fmt.Errorf("x must be >= 0: %v", x)
    }
    return Amount{v: x, valid: true}, nil
}

func (t Amount) Val() float64 {
    if !t.valid {
        panic("shenguard: use of unvalidated Amount")
    }
    return t.v
}
```

`var a Amount` gives `Amount{v: 0, valid: false}`. The moment anyone calls `a.Val()`, it panics. The zero value exists -- Go requires it -- but it's unusable. The `valid` flag is the trip wire.

This matters even when `v == 0` would pass the invariant. The question is not "is 0 a valid amount?" The question is "did the validation *run*?" A zero-value `Amount` with `valid: false` answers that question definitively: no, it did not.

The `valid` flag turns a silent correctness bug into a loud runtime panic. In an AI coding loop, a panic is backpressure -- it shows up in test output, gets fed back to the LLM, and forces a correction. A silently wrong zero value might pass tests and ship.

## JSON Re-Validation: Close the Deserialization Gap

Guard types that cross serialization boundaries must re-validate on the way in. The constructor is the chokepoint, and `UnmarshalJSON` must route through it:

```go
func (a *amount) UnmarshalJSON(data []byte) error {
    var raw float64
    if err := json.Unmarshal(data, &raw); err != nil {
        return fmt.Errorf("Amount: invalid JSON: %w", err)
    }
    validated, err := NewAmount(raw)
    if err != nil {
        return fmt.Errorf("Amount: validation failed: %w", err)
    }
    concrete, ok := validated.(*amount)
    if !ok {
        return fmt.Errorf("Amount: unexpected type from constructor")
    }
    *a = *concrete
    return nil
}
```

The key line is `NewAmount(raw)`. The raw JSON value goes through the same constructor that every other code path uses. The invariant is re-checked. If the JSON contains `-50`, `NewAmount` returns an error, and the unmarshal fails.

Without this, `encoding/json` would either skip the unexported fields (leaving you with a zero value) or, if you added exported JSON tags, write directly into the struct without validation. Either way, the proof chain is broken. With `UnmarshalJSON`, the chain is restored: data enters the system, gets validated, and either becomes a proper guard type or gets rejected.

This applies to any deserialization path -- not just JSON. If you read guard types from a database, a message queue, or a gRPC stream, the deserialization must go through the constructor. The serialized bytes are *untrusted input*, even if they were written by your own system moments ago.

## Composite JSON: Re-Validating the Proof Chain

For composite guard types -- a `Transaction` that contains an `Amount`, a `TenantAccess` that contains an `AuthenticatedUser` -- the `UnmarshalJSON` must rebuild the entire proof chain from raw data:

```go
func (t *transaction) UnmarshalJSON(data []byte) error {
    var raw struct {
        Amount float64 `json:"amount"`
        From   string  `json:"from"`
        To     string  `json:"to"`
    }
    if err := json.Unmarshal(data, &raw); err != nil {
        return fmt.Errorf("Transaction: invalid JSON: %w", err)
    }

    amount, err := NewAmount(raw.Amount)
    if err != nil {
        return fmt.Errorf("Transaction: %w", err)
    }

    from := NewAccountId(raw.From)
    to := NewAccountId(raw.To)

    tx, err := NewTransaction(amount, from, to)
    if err != nil {
        return fmt.Errorf("Transaction: %w", err)
    }

    concrete := tx.(*transaction)
    *t = *concrete
    return nil
}
```

Notice: the raw struct has primitive types (`float64`, `string`). The unmarshal extracts primitives, then rebuilds guard types from scratch through their constructors. `NewAmount` validates the amount. `NewTransaction` validates the transaction-level invariants and requires a valid `Amount` as input.

The entire proof chain re-executes during deserialization. There is no shortcut where a nested guard type is trusted because it "was valid when it was serialized." Serialization is a boundary, and boundaries re-validate.

## Reflection Firewall: Static Analysis in Gate 5

Attacks D (reflection) cannot be fully blocked at compile time -- `reflect` is a standard library package and there is no language mechanism to restrict its use. But it can be caught by static analysis.

Gate 5, the TCB audit, already diffs generated code to detect hand-edits. Adding a reflection firewall is straightforward: scan all application code (outside `shenguard`) for reflection calls targeting guard types:

```bash
#!/bin/bash
# Part of Gate 5: reflection firewall

VIOLATIONS=$(grep -rn \
    -e 'reflect\.ValueOf' \
    -e 'reflect\.NewAt' \
    -e 'reflect\.New(' \
    -e 'unsafe\.Pointer' \
    --include='*.go' \
    --exclude-dir='shenguard' \
    --exclude-dir='vendor' \
    . | grep -i 'amount\|tenant\|jwt\|token\|balance\|transaction')

if [ -n "$VIOLATIONS" ]; then
    echo "FAIL: reflection/unsafe usage on guard types detected:"
    echo "$VIOLATIONS"
    exit 1
fi
```

This is a grep, not a proof. It has false positives (reflection on non-guard types with similar names) and false negatives (obfuscated reflection). But in an AI coding loop, it serves a specific purpose: if the LLM tries to use reflection to work around a guard type (which LLMs do when they hit a compile error they cannot solve), Gate 5 catches it and feeds the error back as backpressure.

The LLM sees "FAIL: reflection/unsafe usage on guard types detected" and learns that reflection is not the escape hatch. It goes back to the constructor.

## The Trade-Off: Sealed Interfaces vs. Struct with Valid Flag

Sealed interfaces are the stronger defense. They eliminate zero-value exploitation entirely, prevent external implementation, and hide the concrete type. But they come at a cost.

**Heap allocation.** In Go, an interface value is a two-word pair: a pointer to the type descriptor and a pointer to the data. The concrete `amount` struct must be heap-allocated (or at least escape to the heap when returned as an interface). A plain struct `Amount` can live on the stack.

For a guard type that is created once during request processing and threaded through a few function calls, this does not matter. For a guard type used in a hot inner loop -- say, validating millions of amounts in a batch processing pipeline -- the allocation pressure adds up.

**The `--hardened` flag.** shengen supports a `--hardened` flag that controls which mode is emitted:

- Without `--hardened`: struct with unexported fields and `valid` flag. Stack-friendly. Good enough for most applications where the five-gate loop provides the outer safety net.
- With `--hardened`: sealed interface with unexported concrete type. Heap-allocated. Closes every gap except reflection. For security-critical types where the cost of a bypass is high.

You can mix modes in the same package. `Amount` might be a plain struct (high-volume, low-risk), while `TenantAccess` is a sealed interface (low-volume, high-risk). The spec doesn't change -- only the codegen target.

## The Hardening Hierarchy

Putting it all together, the defenses stack:

| Attack | Baseline (unexported fields) | + valid flag | + sealed interface | + UnmarshalJSON | + Gate 5 reflection scan |
|--------|-----|-----|-----|-----|-----|
| A: Direct construction | Blocked | Blocked | Blocked | Blocked | Blocked |
| C: Zero-value exploitation | **Open** | Panics on use | nil (panics on use) | N/A | N/A |
| D: Reflection | **Open** | **Open** | **Open** | N/A | Detected |
| E: JSON unmarshaling | **Open** | **Open** | **Open** | Blocked | N/A |

No single technique closes everything. The combination does. Unexported fields handle the common case. The `valid` flag catches zero-value mistakes. Sealed interfaces make them impossible. `UnmarshalJSON` closes the serialization boundary. Gate 5 catches reflection abuse.

This is defense in depth applied to the type system. Each layer compensates for the gaps in the layers below it. And critically, each layer produces *backpressure* that an AI coding loop can act on -- compile errors, panics in tests, gate failures. The LLM does not need to understand formal verification or sealed interfaces. It needs to see "FAIL" and try a different approach.

The guard types are not a proof of security. They are a proof of *process* -- evidence that validation happened, that the construction path was followed, that every link in the chain was checked. The hardening techniques in this post make that proof harder to forge.

---

*[Shen-Backpressure](https://github.com/pyrex41/Shen-Backpressure) is open source. The `--hardened` flag for sealed interfaces is in `cmd/shengen/`. The reflection firewall script is part of Gate 5 in `bin/shenguard-audit.sh`.*

*Previous: [Making Cross-Tenant Access Impossible to Accidentally Bypass](/posts/impossible-by-construction/) | [One Spec, Every Language](/posts/one-spec-every-language/)*
