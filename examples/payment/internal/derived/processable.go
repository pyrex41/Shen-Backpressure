// Package derived holds implementations whose correctness is checked by
// shen-derive against the Shen spec at specs/core.shen.
//
// To regenerate the spec-equivalence test:
//
//	cd shen-derive && go run . verify \
//	  ../examples/payment/specs/core.shen \
//	  --func processable \
//	  --impl-pkg ralph-shen-agent/internal/derived \
//	  --impl-func Processable \
//	  --import ralph-shen-agent/internal/guardcompat \
//	  --out ../examples/payment/internal/derived/processable_spec_test.go
package derived

import (
	"ralph-shen-agent/internal/shenguard"
)

// Processable reports whether every running balance stays non-negative
// when the given transactions are applied one-by-one to the initial
// balance b0.
//
// This is a hand-written efficient version. Its correctness is checked
// against the Shen spec (see processable_spec_test.go).
//
// Generic over the transactions' GDP brand B: the predicate holds for a
// list of transactions from any one minting scope, and says nothing
// that would let transactions from two scopes be mixed.
func Processable[B shenguard.Brand](b0 shenguard.Amount, txs []shenguard.Transaction[B]) bool {
	balance := b0.Val()
	for _, tx := range txs {
		balance -= tx.Amount().Val()
		if balance < 0 {
			return false
		}
	}
	return true
}
