---
title: "Five Ways to Bypass a Guard Type (And How to Close Each One)"
date: 2026-03-29
draft: false
description: "A systematic taxonomy of guard type bypass attacks across Go, Rust, TypeScript, and Python — and the defense mechanisms that close each one."
tags: ["shen", "backpressure", "formal-verification", "security", "hardening", "bypass-prevention"]
---

*A systematic taxonomy of guard type bypass attacks across Go, Rust, TypeScript, and Python --- and the defense mechanisms that close each one.*

---

The [previous posts](/posts/impossible-by-construction/) in this series made a claim: guard types generated from a Shen spec make it impossible to *accidentally* bypass authorization. The compiler enforces the proof chain. The AI coding agent cannot skip a step because `go build` fails.

That claim has a footnote, and the footnote matters: **impossible to accidentally bypass is not the same as impossible to deliberately bypass.** A motivated developer --- or a sufficiently creative LLM --- can circumvent guard types in every language we support. The question is not whether bypass is possible. The question is how hard it is, what it looks like, and whether the defense stack catches it before it ships.

This post is the taxonomy. Five distinct bypass attacks, four languages, three enforcement tiers. A matrix of what works, what doesn't, and what to do about it.

## The Five Attacks

Every guard type bypass we have found falls into one of five categories:

**A. Direct Construction** --- Building the struct/class without going through the validated constructor. In Go, this means writing `TenantAccess{auth: user, tenant: id, isMember: true}` from outside the package. In Rust, constructing the struct with private fields from outside the module.

**B. Field Mutation** --- Obtaining a legitimately constructed guard value and then modifying its internal fields to change the validated state. Swapping the tenant ID after the membership check already passed.

**C. Zero-Value Exploitation** --- Using the language's default zero value to obtain a guard type instance without calling any constructor. In Go, `var ta TenantAccess` gives you a valid zero-value struct. In Python, `object.__new__(Amount)` skips `__init__` entirely.

**D. Reflection/Unsafe Bypass** --- Using runtime introspection, `unsafe` blocks, or metaprogramming to forge a guard value. Go's `reflect` package can set unexported fields. Rust's `unsafe` can transmute arbitrary bytes into a struct. Python's `inspect` module can reach into closure scopes.

**E. Serialization Roundtrip** --- Serializing a guard type to JSON (or any wire format) and deserializing it back, bypassing the constructor. `json.Unmarshal` in Go, `JSON.parse` in TypeScript, `pickle.loads` in Python. The deserialized value has the right shape but was never validated.

## The Matrix

Here is what each language blocks in standard mode (default shengen output) versus hardened mode (`--hardened` flag):

| Attack | Go Std | Go Hard | Rust Std | Rust Hard | TS Std | TS Hard | Py Std | Py Hard |
|--------|--------|---------|----------|-----------|--------|---------|--------|---------|
| A. Direct construction | Blocked | Blocked | Blocked | Blocked | Blocked (tsc) | Blocked (tsc + brand) | Bypassable | Blocked (closure) |
| B. Field mutation | Blocked | Blocked | Blocked | Blocked | Blocked (tsc) | Blocked (tsc + brand) | Partial (`frozen`) | Blocked (closure) |
| C. Zero-value exploit | Bypassable | Blocked (registry) | Blocked | Blocked | N/A | N/A | Bypassable | Blocked (closure) |
| D. Reflection/unsafe | Bypassable | Blocked (HMAC) | Bypassable (`unsafe`) | Blocked (HMAC) | N/A | N/A | Bypassable | Partial (closure) |
| E. Serialization roundtrip | Bypassable | Blocked (custom unmarshal) | Bypassable | Blocked (custom deser) | Bypassable | Blocked (branded deser) | Bypassable | Blocked (custom deser) |

Read the table like this: "Blocked" means the language or tooling prevents this attack with no additional effort. "Bypassable" means a developer who knows the trick can do it. "Partial" means the defense covers some but not all variants. "N/A" means the attack does not apply to that language (TypeScript has no zero values; JavaScript variables are `undefined`, not zero-initialized typed structs).

The pattern is clear: **standard mode relies on the language's native privacy model. Hardened mode adds active defenses for each gap the language leaves open.**

## Attack A: Direct Construction

The most obvious bypass. If you can build the struct yourself, you skip the constructor's validation entirely.

**Go** blocks this at compile time. Unexported fields cannot be set from outside the package:

```go
// From outside shenguard package:
ta := shenguard.TenantAccess{auth: user, tenant: id, isMember: true}
// compile error: unknown field auth in struct literal of type shenguard.TenantAccess
```

**Rust** blocks this identically. Private fields are private to the module:

```rust
// From outside the shenguard module:
let ta = TenantAccess { auth: user, tenant: id, is_member: true };
// error[E0451]: field `auth` of struct `TenantAccess` is private
```

**TypeScript** blocks this through `private constructor`:

```typescript
const ta = new TenantAccess(user, id, true);
// tsc error: Constructor of class 'TenantAccess' is private
```

But after transpilation to JavaScript, `new TenantAccess(user, id, true)` works fine. The defense is compile-time only. Hardened mode adds a **brand** --- a unique symbol that the constructor sets and that downstream consumers check:

```typescript
const __brand = Symbol("TenantAccess");
// In hardened mode, consumers verify: if (!((__brand) in ta)) throw ...
```

**Python** has no mechanism to prevent `Amount(_v=50)`. The underscore prefix is convention, not enforcement. Hardened mode switches to **closure-based construction**, where the validated state is captured in a closure scope and genuinely inaccessible:

```python
def make_tenant_access(auth, tenant, is_member):
    if not is_member:
        raise ValueError("is_member must be true")
    def _get_auth(): return auth
    def _get_tenant(): return tenant
    return TenantAccessProof(_get_auth, _get_tenant)
```

No `_v` attribute to set. No constructor to call with forged arguments. The closure IS the opacity.

**Trade-off:** Closure-based guards lose serializability, collection storage ergonomics, and debugger inspectability. They are stronger but less convenient. For most Python codebases, the standard `@dataclass(frozen=True)` plus the five-gate loop is sufficient; closures are for when the threat model demands it.

## Attack B: Field Mutation

You obtained a legitimate `TenantAccess` for Tenant A. Now you want to change the tenant field to point at Tenant B.

**Go and Rust** block this by default. Unexported/private fields are not writable from outside the defining package/module. Go's `frozen` is structural (no setter exists); Rust's ownership model means you would need `&mut` access to a private field, which the module does not expose.

**TypeScript** blocks mutation through `private readonly`:

```typescript
private readonly _tenant: TenantId;
```

`tsc` rejects writes. But at JavaScript runtime, `ta._tenant = forgedId` works. Hardened mode's brand check catches this if consumers verify the brand after receiving the value.

**Python's** `frozen=True` on `@dataclass` prevents `ta._tenant = forgedId` by raising `FrozenInstanceError`. But `object.__setattr__(ta, '_tenant', forgedId)` bypasses it. Closure-based guards close this completely --- there is no attribute to set.

## Attack C: Zero-Value Exploitation

Go's zero-value semantics mean every type has a default: `var ta shenguard.TenantAccess` gives you a struct with all fields at their zero values. The constructor never ran. The invariants were never checked.

```go
var ta shenguard.TenantAccess  // zero value --- no error, no validation
ProcessRequest(ta)              // accepts it, because the type matches
```

This is Go's most significant gap. The type system says "this is a `TenantAccess`" but the constructor says "I never validated this."

**Hardened mode** closes this with a **validity registry** or an **HMAC tag**. The constructor registers each instance (or stamps it with an HMAC of its contents), and consuming functions check the registry (or verify the tag) before proceeding:

```go
func NewTenantAccess(auth AuthenticatedUser, tenant TenantId, isMember bool) (TenantAccess, error) {
    // ... validation ...
    ta := TenantAccess{auth: auth, tenant: tenant, isMember: isMember}
    ta.hmac = computeHMAC(ta, secretKey)
    return ta, nil
}

func (t TenantAccess) Valid() bool {
    return verifyHMAC(t, t.hmac, secretKey)
}
```

A zero-value `TenantAccess` has a zero HMAC, which fails verification.

**Rust** does not have this problem. There is no way to construct a struct with private fields from outside the module, not even as a zero value. `Default` must be explicitly implemented, and shengen does not derive it.

**TypeScript and Python** do not have Go-style zero values. TypeScript variables are `undefined` until assigned. Python variables do not exist until assigned. The attack vector does not apply.

**Trade-off:** HMAC validation adds a per-access cost. For hot paths, this matters. The registry approach (a `sync.Map` of valid instances) avoids per-access crypto but introduces memory pressure. Choose based on your performance profile.

## Attack D: Reflection and Unsafe

Every language has an escape hatch for when the rules are too restrictive. In Go it is `reflect`. In Rust it is `unsafe`. In Python it is `inspect` and `ctypes`. In TypeScript/JavaScript it is just... the lack of runtime enforcement.

**Go reflection** can set unexported fields:

```go
ta := shenguard.TenantAccess{}
v := reflect.ValueOf(&ta).Elem()
f := v.FieldByName("isMember")
f.SetBool(true)  // bypasses the constructor entirely
```

**Rust unsafe** can transmute arbitrary memory into a struct:

```rust
let ta: TenantAccess = unsafe { std::mem::zeroed() };
// or
let ta: TenantAccess = unsafe { std::mem::transmute(raw_bytes) };
```

**Python** can reach into closure scopes:

```python
import inspect
cells = inspect.getclosurevars(proof._get_auth)
# or via __code__ and cell contents
```

**Hardened mode** defends against reflection/unsafe with **provenance tokens**. The HMAC approach from Attack C also closes Attack D: even if you forge the struct via reflection, the HMAC will not verify because you do not have the secret key (which is generated at init time and held in an unexported variable inside the guard package).

For Rust, the defense is simpler: any use of `unsafe` in application code can be banned by a lint gate (`#![forbid(unsafe_code)]`), and the guard crate itself never exposes `unsafe` APIs. The TCB audit (Gate 5) verifies that the generated guard code has not been modified.

**Trade-off:** You cannot prevent all reflection/unsafe access. What you can do is make the forged value fail validation when consumed. The defense is not "you cannot forge it" but "a forged value is distinguishable from a legitimate one." This is the HMAC guarantee.

## Attack E: Serialization Roundtrip

The subtlest attack. Every real application serializes data --- to JSON for APIs, to bytes for caches, to rows for databases. If your guard type round-trips through serialization, the deserialized value was never validated.

```go
data, _ := json.Marshal(legitimateTenantAccess)
var forged shenguard.TenantAccess
json.Unmarshal(data, &forged)  // populates exported fields, skips constructor
```

Wait --- Go's `json.Unmarshal` only populates exported fields, and shenguard fields are unexported. So this specific attack fails in standard Go. But a custom `MarshalJSON`/`UnmarshalJSON` that exports the fields for wire compatibility would reopen it.

In TypeScript, `JSON.parse` returns a plain object. Casting it to the guard type (`as TenantAccess`) satisfies `tsc` but the brand is missing:

```typescript
const raw = JSON.parse(jsonString) as TenantAccess;
// TypeScript says OK, but no constructor ran
```

**Hardened mode** generates custom deserialization that re-validates:

```go
func (t *TenantAccess) UnmarshalJSON(data []byte) error {
    var raw struct{ Auth string; Tenant string; IsMember bool }
    json.Unmarshal(data, &raw)
    validated, err := NewTenantAccess(/* reconstruct from raw */)
    if err != nil { return err }
    *t = validated
    return nil
}
```

The deserialized value passes through the constructor. The proof chain is rebuilt, not assumed. In TypeScript, hardened mode generates a `fromJSON` factory that checks the brand. In Python, it generates a custom `__reduce__` that routes through the constructor, blocking `pickle` bypass.

**Trade-off:** Re-validating on deserialization adds latency to every cache read, API response parse, and database row scan. For most applications this is negligible. For high-throughput data pipelines, it may require careful placement of validation boundaries.

## The Defense Stack

No single defense closes all five attacks. The full defense is a stack of six layers, each catching what the layers below miss:

**Layer 1: Language-Level Opacity.** Go unexported fields, Rust private fields, TypeScript `private` keyword. This is the foundation --- it blocks Attack A and Attack B at compile time in Go and Rust, at type-check time in TypeScript. It costs nothing at runtime.

**Layer 2: Brands and Sealing.** TypeScript unique symbols, Go sealed interfaces. These add a runtime-verifiable marker that distinguishes values created by the constructor from values created by other means. This strengthens the TypeScript story against JavaScript interop and provides a secondary check in Go.

**Layer 3: Runtime Validation.** The constructor checks invariants. This is universal --- every language, every mode. It catches invalid *values* (negative amounts, non-member tenant access). It does not catch forged *construction* unless combined with layers above.

**Layer 4: Provenance Tokens.** HMAC tags or registry entries that prove a value was created by the legitimate constructor. This closes Attacks C and D --- zero values and reflection-forged values fail provenance checks. The cost is per-construction and per-verification crypto or map lookup.

**Layer 5: Static Analysis.** Lint gates that ban dangerous patterns: `reflect.ValueOf` on guard types, `unsafe` blocks in application code, `object.__new__` on guard classes, `as` casts of `JSON.parse` results to guard types. These are not foolproof but raise the bar from "easy bypass" to "must disable the linter first."

**Layer 6: TCB Audit.** Gate 5 of the five-gate loop. Regenerate the guard types from the Shen spec and diff against the committed code. If someone hand-edited the generated file --- to add a public constructor, remove validation, export private fields --- the diff catches it. This is the backstop that protects the entire stack.

The layers compound. To bypass a hardened guard type in Go, you would need to: use `reflect` to set unexported fields (bypassing Layer 1), forge an HMAC without the secret key (bypassing Layer 4), avoid patterns the linter catches (bypassing Layer 5), and do it all without modifying the generated code (bypassing Layer 6). Each layer is individually imperfect. The stack is formidable.

## The Three Enforcement Tiers

Shengen supports three tiers via command-line flags:

**Standard** (`shengen` with no flags). Relies on the language's native privacy model plus runtime validation in constructors. Good enough for most applications. The five-gate loop provides defense-in-depth. This is what the previous posts demonstrated.

**Hardened** (`shengen --hardened`). Adds provenance tokens (HMAC or registry), custom deserialization that re-validates, and lint rules for dangerous patterns. Closes Attacks C, D, and E in Go. Closes Attack E in all languages. Adds runtime cost.

**Paranoid** (`shengen --paranoid`). Everything in hardened plus: closure-based construction in Python (closing all five attacks at the cost of ergonomics), `#![forbid(unsafe_code)]` enforcement in Rust, branded nominal types in TypeScript with runtime brand checks on every consumer, and full provenance chain logging. Intended for high-assurance systems where the threat model includes malicious insiders or compromised LLM agents.

The tiers are additive. Each one includes everything from the tier below.

## CPS Proof Chains

There is an alternative to object-based proofs worth mentioning: **continuation-passing style** (CPS) proof chains. Instead of returning a guard type that the caller passes to the next function, the validated function *calls* the next step directly, passing the proof as a closure argument:

```rust
pub fn with_tenant_access<F, R>(
    auth: AuthenticatedUser,
    tenant: TenantId,
    db: &Db,
    f: F,
) -> Result<R, Error>
where
    F: FnOnce(&TenantAccess) -> R,
{
    let is_member = db.check_membership(&auth, &tenant)?;
    if !is_member {
        return Err(Error::AccessDenied);
    }
    let access = TenantAccess { auth, tenant, is_member };
    Ok(f(&access))
}
```

The caller never *owns* a `TenantAccess`. It receives a borrowed reference inside a closure. The proof exists only for the duration of the callback. You cannot store it, serialize it, or pass it to a different context. This eliminates Attacks C, D, and E by construction --- there is no value to forge, zero-initialize, or deserialize.

The Rust version is the strongest because the closure *borrows* the proof (`&TenantAccess`), it does not own it. The borrow checker guarantees the reference cannot escape the closure. In Go, you can approximate this with a callback pattern, but nothing prevents the closure from storing the pointer. In Python, the closure could capture and leak the reference. Rust is the only language where CPS proofs are airtight without additional runtime checks.

The trade-off is composability. CPS chains nest: `with_auth(|auth| with_tenant(auth, |ta| with_resource(ta, |ra| ...)))`. Three levels deep is readable. Ten levels deep is not. Object-based proofs compose linearly; CPS proofs compose by nesting. For shallow proof chains (2-4 steps), CPS is elegant and maximally secure. For deep chains, objects are more practical.

## The Practical Reality

No system is perfectly secure in every language. Python will never have Rust's ownership guarantees. Go will always have `reflect`. TypeScript will always transpile to untyped JavaScript. The goal is not perfection. The goal is **making bypass harder than compliance**.

When the constructor is `Amount.create(x)` and bypass requires `reflect.ValueOf(&ta).Elem().FieldByName("v").SetFloat(x)`, people take the easy path. When the easy path is also the validated path, the system works. Not because it is impossible to circumvent, but because circumvention is conspicuous, effortful, and caught by at least two layers of the defense stack.

This is especially true for AI coding agents. An LLM in a Ralph loop does not *want* to bypass guard types. It wants to write code that passes the gates. If `go build` fails because it tried to construct a guard type directly, backpressure corrects it. The LLM does not reach for `reflect` because that is not in the common training distribution for "how to construct a Go struct." The path of least resistance is the validated constructor, and the gates ensure that path is also the only path that ships.

The taxonomy exists so you know where the gaps are. The defense stack exists so the gaps are covered. And the enforcement tiers exist so you can choose how much coverage your threat model demands, without paying for paranoia you do not need.

---

*[Shen-Backpressure](https://github.com/pyrex41/Shen-Backpressure) is open source. Per-language deep dives: [Go Hardening](/posts/go-hardening/), [TypeScript Branded Nominals](/posts/typescript-branded-nominals/), [Rust Linear Proofs](/posts/rust-linear-proofs/), [Python Closure Vaults](/posts/python-closure-vaults/).*

*Previous: [One Spec, Every Language](/posts/one-spec-every-language/) | [Making Cross-Tenant Access Impossible to Accidentally Bypass](/posts/impossible-by-construction/)*
