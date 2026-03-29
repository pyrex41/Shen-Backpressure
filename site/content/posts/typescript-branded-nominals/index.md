---
title: "TypeScript Guard Types: From private to #private, Brands, and Frozen Proofs"
date: 2026-03-29
draft: false
description: "TypeScript's private keyword disappears at runtime. ES2022 #private fields, branded types, and Object.freeze close the gaps that matter."
tags: ["shen", "typescript", "backpressure", "formal-verification", "branded-types", "hardening"]
---

*TypeScript's `private` keyword disappears at runtime. ES2022 `#private` fields, branded types, and `Object.freeze` close the gaps that matter.*

---

In the [previous post](/posts/one-spec-every-language/), I showed how the same Shen spec generates guard types across Go, TypeScript, Rust, and Python. The conclusion was that TypeScript lands in an awkward middle ground: `tsc` catches bypass at compile time, but after transpilation, JavaScript sees a plain class with plain fields. The `private` keyword is an illusion that evaporates in production.

This post is about closing that gap. Not with a different language -- with TypeScript features that survive the compiler.

## Attack B: The private Illusion

Here is the guard type from the previous post:

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

From TypeScript, `tsc` enforces the boundary:

```typescript
new Amount(50);     // tsc error: Constructor is private
amount._v;          // tsc error: Property '_v' is private
```

But transpile it. The emitted JavaScript is:

```javascript
class Amount {
  constructor(v) { this._v = v; }
  static create(x) {
    if (!(x >= 0)) throw new Error(`x must be >= 0: ${x}`);
    return new Amount(x);
  }
  val() { return this._v; }
}
```

No `private`. No `readonly`. Just a class with a public constructor and a public field. Now watch:

```javascript
const amt = Amount.create(100);
(amt)._v = -5;           // works fine
console.log(amt.val());  // -5
```

The guard type is broken. The invariant -- `x >= 0` -- is violated after construction. This is attack B: **post-construction mutation through runtime field access**. The TypeScript compiler said this was impossible. JavaScript disagrees.

This is not a theoretical concern. Any `JSON.parse()`, any `as any` cast, any JavaScript test file, any dependency that touches your guard types from plain JS -- all of them operate in a world where `private` does not exist. The boundary `tsc` enforces is the compilation boundary, not the runtime boundary.

## ES2022 #private Fields: Runtime Enforcement

ES2022 introduced a feature that changes this equation: `#private` fields. Unlike the `private` keyword, `#private` is a JavaScript language feature, not a TypeScript annotation. It survives transpilation because it IS the transpilation target.

```typescript
export class Amount {
  readonly #v: number;
  private constructor(v: number) { this.#v = v; }
  static create(x: number): Amount {
    if (!(x >= 0)) throw new Error(`x must be >= 0: ${x}`);
    return new Amount(x);
  }
  val(): number { return this.#v; }
}
```

The emitted JavaScript now contains real private fields:

```javascript
class Amount {
  #v;
  constructor(v) { this.#v = v; }
  static create(x) {
    if (!(x >= 0)) throw new Error(`x must be >= 0: ${x}`);
    return new Amount(x);
  }
  val() { return this.#v; }
}
```

Now try the same attack:

```javascript
const amt = Amount.create(100);
amt.#v = -5;  // SyntaxError: Private field '#v' must be
              // declared in an enclosing class
```

This is not a type error. It is not a linter warning. It is a **SyntaxError** -- the JavaScript engine refuses to parse code that accesses `#v` from outside the class. There is no `as any` escape hatch. There is no `Object.keys()` enumeration. `JSON.stringify(amt)` does not include `#v`. The field is invisible and inaccessible at runtime.

This is the single highest-impact change for TypeScript guard types. It moves field privacy from Layer 2 (compile-time only) to Layer 1 (runtime enforcement). The gap between Go's unexported fields and TypeScript's `private` keyword -- the gap the previous post identified as TypeScript's fundamental weakness -- closes.

## Branded Types: Preventing Structural Bypass

Private fields solve mutation. But TypeScript has another gap: **structural typing**.

TypeScript uses structural compatibility. If two types have the same shape, they're assignable to each other. This means:

```typescript
function transfer(amount: Amount) { /* ... */ }

// Without brands, this typechecks:
const fake = { val() { return -999; } } as unknown as Amount;
transfer(fake);
```

The `as unknown as Amount` cast bypasses the constructor entirely. But even without casts, structural typing can leak. If `Amount` exposes a `val(): number` method, any object with that method is structurally compatible in certain generic contexts.

Branded types add a nominal marker that prevents this:

```typescript
declare const AmountBrand: unique symbol;

export class Amount {
  declare readonly [AmountBrand]: never;
  readonly #v: number;

  private constructor(v: number) { this.#v = v; }
  static create(x: number): Amount {
    if (!(x >= 0)) throw new Error(`x must be >= 0: ${x}`);
    return new Amount(x);
  }
  val(): number { return this.#v; }
}
```

The `declare readonly [AmountBrand]: never` line adds a phantom property that exists only in the type system. It emits no JavaScript. But it makes `Amount` nominally unique -- no other type has `[AmountBrand]`, so no structural mimic can satisfy the type checker.

```typescript
const fake = { val() { return -999; } };
transfer(fake);
// tsc error: Property '[AmountBrand]' is missing in type '{ val(): number; }'
```

Without the brand, any object with `{ val(): number }` passes. With the brand, only objects created through `Amount.create()` are valid. The brand is a compile-time seal that says "this value was constructed through the validated path."

Note that brands are compile-time only -- they disappear in JavaScript just like `private`. But that is fine. Brands solve a different problem than `#private`. Private fields prevent runtime mutation. Brands prevent compile-time structural forgery. Together, they close two independent attack vectors.

## Object.freeze: Sealing the Instance

`#private` fields prevent external access. But what about internal mutation? If the class has methods that modify state, or if someone subclasses and overrides behavior, the invariant can break after construction.

`Object.freeze` closes this:

```typescript
export class Amount {
  declare readonly [AmountBrand]: never;
  readonly #v: number;

  private constructor(v: number) { this.#v = v; }
  static create(x: number): Amount {
    if (!(x >= 0)) throw new Error(`x must be >= 0: ${x}`);
    return Object.freeze(new Amount(x));
  }
  val(): number { return this.#v; }
}
```

`Object.freeze` prevents adding new properties, removing existing properties, or changing the values of existing properties. Combined with `#private`, this means:

- External code cannot read `#v` (SyntaxError).
- External code cannot write any property on the instance (frozen).
- The constructor validates the invariant before freezing.

The object is immutable and opaque from the moment it leaves the factory. This closes attacks A (adding rogue properties), B (mutating existing fields), and part of E (prototype pollution -- frozen objects resist property injection, though `Object.freeze` is shallow and does not freeze the prototype chain itself).

## Discriminated Unions for Sum Types

Guard types often model sum types -- values that can be one of several variants. Shen handles this with multiple inference rules. In TypeScript, discriminated unions are the idiomatic translation.

Consider a principal type that can be a human user or a service account:

```typescript
declare const HumanBrand: unique symbol;
declare const ServiceBrand: unique symbol;

export class HumanPrincipal {
  declare readonly [HumanBrand]: never;
  readonly kind = 'human' as const;
  readonly #userId: string;

  private constructor(userId: string) { this.#userId = userId; }
  static create(userId: string): HumanPrincipal {
    if (!userId || !userId.startsWith('u-'))
      throw new Error(`invalid user ID: ${userId}`);
    return Object.freeze(new HumanPrincipal(userId));
  }
  id(): string { return this.#userId; }
}

export class ServicePrincipal {
  declare readonly [ServiceBrand]: never;
  readonly kind = 'service' as const;
  readonly #serviceId: string;

  private constructor(serviceId: string) { this.#serviceId = serviceId; }
  static create(serviceId: string): ServicePrincipal {
    if (!serviceId || !serviceId.startsWith('svc-'))
      throw new Error(`invalid service ID: ${serviceId}`);
    return Object.freeze(new ServicePrincipal(serviceId));
  }
  id(): string { return this.#serviceId; }
}

export type Principal = HumanPrincipal | ServicePrincipal;
```

The `readonly kind = 'human' as const` field serves double duty. First, it enables exhaustive `switch` statements:

```typescript
function auditLog(p: Principal): string {
  switch (p.kind) {
    case 'human':   return `user ${p.id()} performed action`;
    case 'service': return `service ${p.id()} performed action`;
    // no default needed -- tsc proves exhaustiveness
  }
}
```

Second, combined with the brand, it prevents forgery. You cannot create a `HumanPrincipal` by writing `{ kind: 'human', id() { return 'u-evil' } }` -- the brand blocks structural compatibility, the `#private` field blocks runtime field access, and `Object.freeze` blocks post-construction mutation.

Each variant carries its own brand. A `HumanPrincipal` is not assignable to `ServicePrincipal` even though both have `kind` and `id()`. The discriminant (`kind`) enables pattern matching; the brand enables nominal isolation.

## The Hardened Stack

These features compose into a hardening layer. When `shengen-ts` runs in `--mode hardened`, it generates guard types with all four mechanisms:

| Mechanism | What it prevents | Enforcement |
|-----------|-----------------|-------------|
| `#private` fields | Runtime field access and mutation | JavaScript engine (SyntaxError) |
| Branded types | Structural type forgery | TypeScript compiler (`tsc`) |
| `Object.freeze` | Post-construction mutation | JavaScript runtime (silent no-op in sloppy mode, TypeError in strict) |
| Discriminated unions | Variant confusion in sum types | TypeScript compiler (exhaustiveness) |

In `--mode standard` (the default), shengen-ts emits the `private` keyword and no brands -- the same output shown in the previous post. This is fine for projects where all code is TypeScript and `tsc` is the only consumer. `--mode hardened` is for projects with mixed JS/TS codebases, untrusted inputs, or security-critical guard types where defense-in-depth matters.

The generated code for the `Amount` type in hardened mode:

```typescript
declare const AmountBrand: unique symbol;

export class Amount {
  declare readonly [AmountBrand]: never;
  readonly #v: number;

  private constructor(v: number) { this.#v = v; }

  static create(x: number): Amount {
    if (!(x >= 0)) throw new Error(`x must be >= 0: ${x}`);
    return Object.freeze(new Amount(x));
  }

  val(): number { return this.#v; }
}
```

Four lines of defense in a single class. The constructor validates. The `#private` field hides. The brand seals the type. The freeze seals the instance.

## What Remains Open

Two gaps survive even the hardened stack.

**`JSON.parse()` produces unbranded objects.** If you deserialize an `Amount` from a network response, you get `{ _v: 50 }` -- a plain object with no brand, no `#private` field, no frozen seal. It passes no type checks. This is by design (you cannot serialize `#private` fields), but it means every deserialization boundary needs re-validation.

The solution is a `fromJSON` static method on each guard type:

```typescript
export class Amount {
  // ... existing members ...

  static fromJSON(raw: unknown): Amount {
    if (typeof raw !== 'object' || raw === null)
      throw new Error('Amount.fromJSON: expected object');
    const v = (raw as Record<string, unknown>)['v'];
    if (typeof v !== 'number')
      throw new Error('Amount.fromJSON: v must be a number');
    return Amount.create(v);  // re-validates and re-brands
  }
}
```

`fromJSON` accepts `unknown`, validates the shape, and calls `create()` -- which re-runs the invariant check and produces a properly branded, frozen, `#private` instance. The rule is simple: never cast `JSON.parse()` output to a guard type. Always go through `fromJSON`.

**Ephemeral proofs and the CPS pattern.** Some proof chains produce intermediate values that should not outlive a single scope -- a validated session token, a checked permission grant, a verified nonce. If the proof escapes its scope, it can be replayed or reused in contexts where it is no longer valid.

The continuation-passing style (CPS) pattern addresses this:

```typescript
export class TenantAccess {
  declare readonly [TenantAccessBrand]: never;
  readonly #auth: AuthenticatedUser;
  readonly #tenant: TenantId;

  private constructor(auth: AuthenticatedUser, tenant: TenantId) {
    this.#auth = auth;
    this.#tenant = tenant;
  }

  static verified(
    auth: AuthenticatedUser,
    tenant: TenantId,
    isMember: boolean,
    use: (access: TenantAccess) => void
  ): void {
    if (!isMember) throw new Error('not a member');
    use(Object.freeze(new TenantAccess(auth, tenant)));
    // access cannot escape -- it exists only inside `use`
  }
}
```

The `TenantAccess` value is created inside `verified` and passed directly to the `use` callback. The caller never receives it as a return value. It cannot be stored in a variable, cached in a map, or returned from a function. The proof lives exactly as long as the callback executes, then it is unreachable.

This is not airtight -- the callback could close over a mutable variable and stash the reference. TypeScript cannot prevent that. But it makes the intended lifetime explicit in the API surface, and it makes accidental escape structurally awkward. For ephemeral proofs in security-critical paths, that is a meaningful improvement over returning the value directly.

---

The hardened stack does not make TypeScript equivalent to Go or Rust for guard types. Go has true module-level privacy. Rust has ownership and linear types. TypeScript has a type system that compiles away. But `#private` fields, brands, `Object.freeze`, and discriminated unions close the gaps that matter in practice. The remaining gaps -- deserialization boundaries and proof lifetimes -- have known patterns (`fromJSON`, CPS) that shengen can generate automatically.

The goal was never to make TypeScript into something it is not. The goal is to make the safe path easy and the unsafe path visible. With the hardened stack, bypassing a guard type requires deliberate effort -- a cast through `unknown`, a raw `JSON.parse` without `fromJSON`, a closure that captures and leaks an ephemeral proof. These are code review signals, not accidental oversights. And that is the same standard the Go guard types meet: not impossible to bypass, but impossible to bypass *by accident*.

---

*[Shen-Backpressure](https://github.com/pyrex41/Shen-Backpressure) is open source. TypeScript examples are at `examples/payment_ts/` and `examples/email_crud_ts/`. The `shengen-ts --mode hardened` emitter is at `cmd/shengen-ts/`.*

*Previous: [One Spec, Every Language](/posts/one-spec-every-language/)*
