# DeFi Protocol Invariants — Smart Contract Safety

Makes rug pulls, reentrancy, drain exploits, and broken AMM invariants into **type errors**.

The Solidity world discovers these bugs at $600M cost. Shen makes them structurally unconstructable.

## The 10 Layers

| Layer | What It Proves | What It Prevents |
|-------|---------------|-----------------|
| **1. Token Primitives** | Addresses, amounts, balances | Negative amounts, empty addresses |
| **2. Conservation Law** | `supply = minted - burned` | Token creation from nothing, supply inflation |
| **3. Transfer** | Sufficient balance + value preservation | Overdrafts, value creation during transfer |
| **4. AMM / Constant Product** | `x * y = k` preserved after swap, `k' >= k` | Pool drain, broken invariant, sandwich attacks |
| **5. Liquidity** | Proportional deposit, solvent withdrawal | Withdrawing more than pool holds, disproportionate LP |
| **6. Reentrancy Guard** | Mutex available + state committed before external call | Reentrancy exploits (The DAO hack pattern) |
| **7. Flash Loan** | Repaid >= borrowed + fee, same block | Flash loan theft, incomplete repayment |
| **8. Governance / Timelock** | Delay elapsed + quorum met before execution | Instant rug pulls, unauthorized parameter changes |
| **9. Oracle Freshness** | Price updated within staleness window, multi-source agreement | Stale price manipulation, single-oracle attacks |
| **10. Composed Safety** | Transfer + conservation + reentrancy guard | Any operation missing a safety proof |

## Key Proof Chains

### Token Transfer
```
token-balance ──► sufficient-balance (owner has enough)
                        │
                  value-preserved (sender loss = receiver gain)
                        │
transfer-authorized ──► valid-transfer
     OR                     │
within-allowance ─────► valid-transfer
```

### AMM Swap
```
pool-reserve ──► constant-product (k = reserveA × reserveB)
                       │
                 swap-valid (k_after >= k_before)
                       │
slippage-bound ──► safe-swap (output >= minimum)
```

### Flash Loan
```
flash-loan-amount ──► flash-loan-repaid (repaid >= borrowed + fee)
                            │
                      flash-loan-complete (same block)
```

### Anti-Rug Pull
```
governance-proposal ──► timelock-elapsed (delay passed)
                              │
                        governance-executable (quorum + timelock)
```

## Why This Matters

| Exploit | Cost | Shen Proof That Prevents It |
|---------|------|---------------------------|
| The DAO (2016) | $60M | `safe-interaction` (reentrancy guard) |
| Wormhole (2022) | $320M | `supply-conserved` (minting without backing) |
| Mango Markets (2022) | $114M | `oracle-agreement` (price manipulation) |
| Euler Finance (2023) | $197M | `flash-loan-complete` + `swap-valid` |
| Nomad Bridge (2022) | $190M | `valid-transfer` (authorization proof) |

These aren't hypothetical. Every exploit above would fail to construct the required proof object.

## Usage

```bash
# Generate guard types for Solidity companion / off-chain validation
shengen -spec specs/core.shen -out shenguard/guards_gen.go -pkg shenguard

# Use generated types in:
# - Off-chain transaction validators
# - Keeper bot safety checks  
# - Formal verification companions for Solidity
# - Simulation harnesses
```

See `specs/core.shen` for the full 10-layer specification.
