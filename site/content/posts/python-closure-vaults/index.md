---
title: "Python Guard Types: Closure Vaults, HMAC Provenance, and Making Bypass Harder Than Compliance"
date: 2026-03-29
draft: false
description: "Python can't prevent bypass. But closure vaults and HMAC provenance tokens make it harder than just calling the constructor."
tags: ["shen", "python", "backpressure", "formal-verification", "closures", "hmac", "hardening"]
---

*Python can't prevent bypass. But closure vaults and HMAC provenance tokens make it harder than just calling the constructor.*

---

The [previous post](/posts/one-spec-every-language/) ended with a frank assessment: Python sits in the "nobody says no" tier. There are no unexported fields, no `private` keyword with compiler backing, no module-level visibility controls that prevent construction bypass. The language philosophy is "we're all consenting adults."

That post showed a closure-based sketch as an alternative. This post takes that sketch seriously and builds it out into a full hardening strategy: closure vaults for genuine information hiding, HMAC provenance tokens for tamper detection, recursive token chains for transitive integrity, and static lint gates to catch the easy mistakes before they ship.

None of this makes bypass impossible. Python doesn't allow that. The goal is different: **make bypass harder than compliance.** If calling `new_amount(50)` is the path of least resistance and bypassing it requires closure introspection, HMAC forgery, and fighting a linter, developers and LLMs will take the easy path.

## Python's Honesty Problem

Let's start with what Python actually gives you. No private fields — a leading underscore is a convention, not a barrier. `object.__new__()` bypasses `__init__` and `__post_init__` entirely. `frozen=True` on a dataclass stops `amt.x = 5` but not `object.__setattr__(amt, 'x', 5)`. Every protection mechanism has a documented escape hatch.

This isn't a flaw — it's a design decision. CPython's data model is transparent by intent. But it means that any guard type strategy in Python operates on a fundamentally different contract than Go or Rust. You're not proving that bypass is impossible. You're raising the cost of bypass until it exceeds the cost of compliance.

## The Standard Approach: Dataclass With Validation

The simplest guard type in Python uses `@dataclass(frozen=True, slots=True)` with `__post_init__` validation:

```python
from dataclasses import dataclass

@dataclass(frozen=True, slots=True)
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

This catches invalid values:

```python
Amount.create(-5)   # raises ValueError
Amount(_v=-10)      # raises ValueError from __post_init__
```

And `frozen=True` prevents casual reassignment:

```python
amt = Amount.create(50)
amt._v = -5  # raises FrozenInstanceError
```

But it doesn't survive intentional bypass:

```python
# Bypass 1: skip __init__ entirely
fake = object.__new__(Amount)
object.__setattr__(fake, '_v', -999)
fake.val()  # -999 — no validation ran

# Bypass 2: mutate a frozen instance
amt = Amount.create(50)
object.__setattr__(amt, '_v', -999)
amt.val()  # -999 — frozen=True defeated
```

Both attacks are trivial. Both are well-documented Python. For the standard dataclass approach, this is the end of the road — there's nothing more the language gives you.

## The Closure Vault

Here's a different approach. Instead of storing the validated value as an object attribute, store it in a closure's lexical scope:

```python
class _AmountImpl:
    __slots__ = ('val',)

    def __init_subclass__(cls, **kwargs):
        raise TypeError("_AmountImpl cannot be subclassed")

def _build_amount_factory():
    def create(x: float) -> '_AmountImpl':
        if not (x >= 0):
            raise ValueError(f"x must be >= 0: {x}")
        impl = object.__new__(_AmountImpl)
        impl.val = lambda: x  # x lives in closure scope
        return impl
    return create

new_amount = _build_amount_factory()
```

Usage is clean:

```python
amt = new_amount(50)
amt.val()  # 50
```

Why this works: the value `x` is captured in the lambda's closure, not stored as a data attribute on the object. There's no `__dict__` entry to mutate (because of `__slots__`). There's no `_v` field to overwrite with `object.__setattr__`. The only way to get the value out is to call `amt.val()`, which returns whatever `x` was at the time the closure was created.

Can you still attack this? Yes. You can replace the `val` attribute itself:

```python
amt = new_amount(50)
amt.val = lambda: -999
amt.val()  # -999
```

The closure containing `50` still exists somewhere in memory, but you've replaced the reference to it. This is where HMAC provenance comes in.

## HMAC Provenance Tokens

The idea: when the factory creates a value, it also signs the value with a per-process secret. The secret lives inside the factory closure and is inaccessible from outside. Verification checks whether the signature matches:

```python
import hmac
import hashlib
import os

def _build_amount_system():
    _secret = os.urandom(32)  # per-process, lives in closure

    def create(x: float) -> '_AmountImpl':
        if not (x >= 0):
            raise ValueError(f"x must be >= 0: {x}")

        tag = hmac.new(
            _secret,
            str(x).encode(),
            hashlib.sha256
        ).hexdigest()

        impl = object.__new__(_AmountImpl)
        impl.val = lambda: x
        impl.token = lambda: tag
        return impl

    def verify(amt) -> bool:
        try:
            x = amt.val()
            tag = amt.token()
            expected = hmac.new(
                _secret,
                str(x).encode(),
                hashlib.sha256
            ).hexdigest()
            return hmac.compare_digest(tag, expected)
        except Exception:
            return False

    return create, verify

new_amount, verify_amount = _build_amount_system()
```

Now watch the attack fail:

```python
amt = new_amount(50)
verify_amount(amt)    # True

amt.val = lambda: -5
verify_amount(amt)    # False — HMAC mismatch

# The attacker changed val() but can't recompute the HMAC
# because _secret is trapped in the factory closure
```

The attacker would need to forge a valid HMAC for the new value. To do that, they'd need `_secret`. To get `_secret`, they'd need to introspect the factory closure:

```python
# This is what bypass actually looks like now
import types
cells = new_amount.__closure__
# Walk through cell contents looking for bytes...
```

This is not impossible. But it's closure introspection — fragile, version-dependent, and visibly wrong in any code review. The cost of bypass now significantly exceeds the cost of calling `new_amount(x)`.

## Recursive HMAC Chains

Guard types don't exist in isolation. The [proof chain pattern](/posts/impossible-by-construction/) means downstream types require upstream types as inputs. In Python, we can extend HMAC provenance to cover the entire chain.

When `BalanceChecked` is constructed, its HMAC incorporates the tokens of its inputs:

```python
def _build_balance_checked_system(verify_amount_fn, verify_tx_fn):
    _secret = os.urandom(32)

    def create(balance: float, tx) -> '_BalanceCheckedImpl':
        if not verify_tx_fn(tx):
            raise ValueError("transaction failed provenance check")

        tx_amount = tx.amount().val()
        if not (balance >= tx_amount):
            raise ValueError(
                f"insufficient balance: {balance} < {tx_amount}"
            )

        # Chain: incorporate upstream tokens into our HMAC
        tx_token = tx.token()
        amt_token = tx.amount().token()
        msg = f"{balance}:{tx_token}:{amt_token}".encode()

        tag = hmac.new(_secret, msg, hashlib.sha256).hexdigest()

        impl = object.__new__(_BalanceCheckedImpl)
        impl.balance = lambda: balance
        impl.tx = lambda: tx
        impl.token = lambda: tag
        return impl

    def verify(checked) -> bool:
        try:
            balance = checked.balance()
            tx = checked.tx()
            if not verify_tx_fn(tx):
                return False
            tx_token = tx.token()
            amt_token = tx.amount().token()
            msg = f"{balance}:{tx_token}:{amt_token}".encode()
            expected = hmac.new(
                _secret, msg, hashlib.sha256
            ).hexdigest()
            return hmac.compare_digest(checked.token(), expected)
        except Exception:
            return False

    return create, verify
```

Now tampering at any level breaks the entire chain:

```python
amt = new_amount(50)
tx = new_transaction(amt, "alice", "bob")
checked = new_balance_checked(1000, tx)

verify_balance_checked(checked)  # True

# Tamper with the amount buried inside the transaction
tx.amount().val = lambda: -9999
verify_balance_checked(checked)  # False — amt_token mismatch
```

The recursive structure means you can't surgically tamper with one layer. The HMAC at each level depends on the tokens below it. Changing a leaf invalidates every node up to the root.

## Blocking Subclass Exploits

Without protection, an attacker could subclass the implementation to override behavior:

```python
class Evil(_AmountImpl):
    def val(self):
        return -9999
```

The `__init_subclass__` hook prevents this:

```python
class _AmountImpl:
    __slots__ = ('val', 'token')

    def __init_subclass__(cls, **kwargs):
        raise TypeError("_AmountImpl cannot be subclassed")
```

Now `class Evil(_AmountImpl)` raises `TypeError` at class definition time — not at instantiation, at definition. The class body never executes.

This isn't bulletproof either. An attacker can use `type()` to create a class that mimics `_AmountImpl` without inheriting from it. But that class won't pass `isinstance` checks, and more importantly, it won't have a valid HMAC token. The provenance system catches it regardless of how the object was constructed.

## Static Lint Gate: Catching the Easy Mistakes

Runtime hardening handles intentional bypass. But most violations aren't intentional — they're a developer (or LLM) taking a shortcut. A static lint gate catches these before they run:

```python
import ast
import sys

GUARD_TYPES = {"Amount", "Transaction", "BalanceChecked", "SafeTransfer"}
FORBIDDEN_PATTERNS = {"object.__setattr__", "object.__new__"}

class GuardTypeVisitor(ast.NodeVisitor):
    def __init__(self):
        self.violations = []

    def visit_Call(self, node):
        # Catch direct construction: Amount(...)
        if isinstance(node.func, ast.Name):
            if node.func.id in GUARD_TYPES:
                self.violations.append(
                    f"line {node.lineno}: direct construction "
                    f"of {node.func.id}() — use factory function"
                )

        # Catch object.__setattr__ and object.__new__
        if isinstance(node.func, ast.Attribute):
            if isinstance(node.func.value, ast.Name):
                full = f"{node.func.value.id}.{node.func.attr}"
                if full in FORBIDDEN_PATTERNS:
                    self.violations.append(
                        f"line {node.lineno}: {full} on "
                        f"potential guard type"
                    )

        self.generic_visit(node)

def lint_file(path: str) -> list[str]:
    with open(path) as f:
        tree = ast.parse(f.read(), filename=path)
    visitor = GuardTypeVisitor()
    visitor.visit(tree)
    return visitor.violations
```

This slots into Gate 5 of the five-gate loop. It's an AST walk, not a regex — it catches `Amount(x)` but not `new_amount(x)`. It catches `object.__setattr__(amt, ...)` but not `amt.val = ...` (which the HMAC system handles at runtime).

The linter is intentionally conservative. It flags patterns that are *almost always* wrong when applied to guard types. False positives are possible but rare — if you're calling `object.__new__` on a guard type, you should be able to explain why.

In a CI pipeline, this runs as a pre-commit check:

```bash
#!/bin/bash
# Gate 5b: guard type lint
python -m guard_lint src/ --types Amount,Transaction,BalanceChecked
if [ $? -ne 0 ]; then
    echo "FAIL: guard type bypass detected"
    exit 1
fi
```

## The Defense-in-Depth Stack

Here's the full picture, layered from weakest to strongest:

| Layer | Mechanism | What It Catches | What It Misses |
|-------|-----------|----------------|----------------|
| 1. Convention | `_v` prefix, `create()` factory | Nothing (advisory only) | Everything |
| 2. `frozen=True` | `FrozenInstanceError` on assignment | Casual `amt._v = x` | `object.__setattr__` |
| 3. `__slots__` | `AttributeError` on new attrs | Adding arbitrary attributes | Overwriting existing slots |
| 4. Closure vault | Value in lexical scope | Attribute mutation of the value | Replacing the closure reference |
| 5. HMAC provenance | Signature over closure contents | Replacing closure references | Closure introspection to steal secret |
| 6. Recursive chains | Upstream tokens in downstream HMAC | Surgical tampering at any depth | Full chain reconstruction with stolen secrets |
| 7. `__init_subclass__` | `TypeError` on subclass definition | Inheritance-based override | `type()` metaclass tricks |
| 8. Static lint gate | AST analysis in CI | Direct construction, `object.__setattr__` | Dynamic construction patterns |

Each layer is individually bypassable. Stacked together, bypass requires: closure introspection to extract the HMAC secret, forging a valid token chain, avoiding the lint gate, and doing all of this in a way that passes code review. Meanwhile, compliance requires calling `new_amount(50)`.

## The Philosophy

Go has unexported fields. Rust has module privacy and ownership. These languages can make the compiler say no. Python can't — and pretending otherwise leads to false confidence.

But "the compiler can't enforce it" doesn't mean "don't bother." It means the enforcement strategy shifts from prevention to economics. You can't make bypass impossible, so you make it expensive. Closure vaults raise the cost above "change an attribute." HMAC provenance raises it above "replace a method." Recursive chains raise it above "tamper with one layer." The lint gate catches the accidental cases that aren't even trying to bypass.

This is the same principle behind door locks. A determined attacker can pick any lock. Locks work because they make breaking in harder than using the key. The closure vault is the lock. The HMAC is the deadbolt. The lint gate is the security camera. None of them are proof. Together, they change the calculus.

For AI coding loops, this matters concretely. When an LLM generates Python code that interacts with guard types, it will take the path of least resistance. If `new_amount(x)` is documented, importable, and works on the first try, that's what the model will use. If bypass requires importing `os`, extracting closure cells, computing HMACs with a stolen secret, and suppressing lint warnings — the model will not stumble into that by accident. The backpressure works even without a compiler.

---

*[Shen-Backpressure](https://github.com/pyrex41/Shen-Backpressure) is open source. Python guard type examples with closure vaults and HMAC provenance are at `examples/python_closures/`. The static lint gate is at `tools/guard_lint.py`.*

*Previous: [One Spec, Every Language](/posts/one-spec-every-language/). See also: [Making Cross-Tenant Access Impossible to Accidentally Bypass](/posts/impossible-by-construction/).*
