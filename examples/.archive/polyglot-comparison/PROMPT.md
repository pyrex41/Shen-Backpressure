Same Shen spec, five language outputs side by side. Demonstrates that one formal specification produces different but complementary guarantees in each target language.

Uses the payment spec (simplest complete spec) and generates guard types in all supported languages:

1. **Go** (shengen) — unexported fields, `(T, error)` return
2. **TypeScript** (shengen-ts) — private constructor, `static create()`, throws on violation
3. **Rust** (shengen-rs) — private fields, `Result<Self, GuardError>`
4. **Python standard** (shengen-py) — frozen dataclass, `__post_init__` raises ValueError
5. **Python hardened** (shengen-py --mode hardened) — HMAC provenance chain, tamper detection

For each language, write a small program that:
- Creates a valid Amount (succeeds)
- Attempts to create an invalid Amount with negative value (fails, show the error)
- Attempts to bypass the constructor by directly constructing the struct (show the compile/runtime error)
- Creates a full safe-transfer proof chain (Amount -> Transaction -> BalanceChecked -> SafeTransfer)

Spec: Use specs/core.shen (copy from examples/payment/specs/core.shen).

Create:
- specs/core.shen
- go/main.go + go/shenguard/guards_gen.go
- ts/main.ts + ts/shenguard/guards_gen.ts
- rs/src/main.rs + rs/src/shenguard/guards_gen.rs
- py/main.py + py/shenguard/guards_gen.py
- py-hardened/main.py + py-hardened/shenguard/guards_gen.py

Put all five side by side. This directly complements the "enforcement spectrum" blog post.
