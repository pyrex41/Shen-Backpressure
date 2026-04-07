Payment processor in Go with balance invariants enforced by Shen sequent-calculus types. The core proof: you cannot transfer money without first proving the balance covers the transaction amount.

Stack: Go stdlib, no frameworks. SQLite optional for persistence.

Domain entities:
- Accounts with IDs and balances
- Transactions (amount, from, to)
- Balance-checked proofs (bal >= tx.amount)
- Safe transfers (transaction + balance proof)

Invariants (specs/core.shen):
- amount must be >= 0
- balance-checked requires bal >= transaction amount (the head of the transaction composite)
- safe-transfer requires both a transaction and its balance-checked proof
- The proof chain is transitive: SafeTransfer -> BalanceChecked -> Amount >= 0

Operations:
- Create accounts with initial balances
- Transfer between accounts (requires balance proof)
- Query account state

The key demonstration: the `Transfer()` function accepts a `shenguard.SafeTransfer`, not raw values. You cannot call it without first constructing the proof chain through the guard type constructors. An overdraft is not a runtime error caught by an if-statement — it is a construction failure at the proof boundary.

Reference guard outputs for Go, TypeScript, Rust, and Python (standard + hardened) are in `reference/`.

Use /sb:ralph-scaffold to set up the Ralph loop with five-gate backpressure, then run the loop to build it out.
