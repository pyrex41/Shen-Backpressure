// Package guardcompat re-exports the branded guard types at one shared
// brand, for generated code that does not thread brands of its own.
//
// Why this exists. With `--brands` (W1) the generated guard types carry
// a phantom brand parameter: `shenguard.Transaction[B]`. Hand-written
// code threads B through its own signatures, which is the point — that
// is what binds a proof to the value it is about. But shen-derive's
// spec-equivalence harness is generated from the spec alone and writes
// `Transaction`, unparameterized, because shen-derive does not run the
// brand inference. Pointing its `guard_pkg` at this package gives it a
// non-generic name to write.
//
// What is given up. Every value produced here carries the same brand,
// `deriveBrand`, so within this package's world the brands are
// interchangeable and the pairing guarantee is gone: a BalanceChecked
// minted here would type-check against any Transaction minted here.
// That is acceptable for the derive harness, whose job is to compare
// the spec's evaluator against a Go implementation over sampled inputs
// — it never constructs a proof about one value and applies it to
// another. It would NOT be acceptable in application code, which is why
// this package is confined to internal/ and why src/payment threads a
// real brand instead.
//
// The witness field still applies: these are constructor calls, so the
// zero-value defence is untouched.
package guardcompat

import (
	"context"

	"ralph-shen-agent/internal/shenguard"
)

// deriveBrand is the single shared brand of this compatibility layer.
type deriveBrand struct{}

// Unbranded guard types are re-exported unchanged.
type (
	AccountId = shenguard.AccountId
	Amount    = shenguard.Amount
)

// Branded guard types are pinned to deriveBrand.
type (
	Transaction    = shenguard.Transaction[deriveBrand]
	BalanceChecked = shenguard.BalanceChecked[deriveBrand]
	SafeTransfer   = shenguard.SafeTransfer[deriveBrand]
	AccountState   = shenguard.AccountState
)

// NewAccountId mints an AccountId.
func NewAccountId(x string) AccountId { return shenguard.NewAccountId(x) }

// NewAmount mints an Amount, discharging the `(>= X 0)` premise.
func NewAmount(ctx context.Context, x float64) (Amount, error) {
	return shenguard.NewAmount(ctx, x)
}

// NewTransaction mints a Transaction at the shared derive brand.
func NewTransaction(amount Amount, from AccountId, to AccountId) Transaction {
	return shenguard.NewTransaction[deriveBrand](amount, from, to)
}

// NewBalanceChecked proves `Bal >= (head Tx)` for a Transaction at the
// shared derive brand.
func NewBalanceChecked(bal float64, tx Transaction) (BalanceChecked, error) {
	return shenguard.NewBalanceChecked(bal, tx)
}

// NewSafeTransfer pairs a Transaction with its BalanceChecked.
func NewSafeTransfer(tx Transaction, check BalanceChecked) SafeTransfer {
	return shenguard.NewSafeTransfer(tx, check)
}

// NewAccountState mints an AccountState.
func NewAccountState(id AccountId, balance Amount) AccountState {
	return shenguard.NewAccountState(id, balance)
}
