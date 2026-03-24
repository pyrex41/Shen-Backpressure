package payment_test

import (
	"testing"

	"shengen/payment"
)

func TestAmountMustBeNonNegative(t *testing.T) {
	if _, err := payment.NewAmount(-10); err == nil {
		t.Fatal("NewAmount(-10) must fail — amounts cannot be negative")
	}
	amt, err := payment.NewAmount(100)
	if err != nil {
		t.Fatalf("NewAmount(100) should succeed: %v", err)
	}
	if amt.Val() != 100 {
		t.Errorf("expected 100, got %v", amt.Val())
	}
}

func TestBalanceCheckedRequiresSufficientFunds(t *testing.T) {
	// This is the KEY invariant: (>= Bal (head Tx))
	// Resolved via symbol table to: bal >= tx.Amount.Val()

	amt, _ := payment.NewAmount(50)
	tx := payment.NewTransaction(
		amt,
		payment.NewAccountId("alice"),
		payment.NewAccountId("bob"),
	)

	// Balance covers the transaction — should pass
	check, err := payment.NewBalanceChecked(100, tx)
	if err != nil {
		t.Fatalf("NewBalanceChecked(100, tx{50}) should succeed: %v", err)
	}
	_ = check

	// Exact amount — should pass
	check2, err := payment.NewBalanceChecked(50, tx)
	if err != nil {
		t.Fatalf("NewBalanceChecked(50, tx{50}) should succeed: %v", err)
	}
	_ = check2

	// Insufficient funds — MUST fail
	_, err = payment.NewBalanceChecked(30, tx)
	if err == nil {
		t.Fatal("NewBalanceChecked(30, tx{50}) MUST fail — insufficient funds")
	}
	t.Logf("Correctly rejected: %v", err)
}

func TestSafeTransferRequiresBalanceCheck(t *testing.T) {
	// SafeTransfer can only be constructed with a BalanceChecked proof.
	// This means you can't create a transfer without first proving
	// the balance covers it — the Go type system enforces this.

	amt, _ := payment.NewAmount(25)
	tx := payment.NewTransaction(
		amt,
		payment.NewAccountId("alice"),
		payment.NewAccountId("bob"),
	)

	// Must first get a BalanceChecked proof
	proof, err := payment.NewBalanceChecked(100, tx)
	if err != nil {
		t.Fatalf("balance check failed: %v", err)
	}

	// Now we can create the SafeTransfer
	transfer := payment.NewSafeTransfer(tx, proof)
	_ = transfer
	t.Log("SafeTransfer created successfully with proof of sufficient funds")
}
