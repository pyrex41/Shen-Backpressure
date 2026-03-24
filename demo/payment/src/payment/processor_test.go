package payment

import (
	"errors"
	"testing"

	"ralph-shen-agent/internal/shenguard"
)

// helper builds a SafeTransfer from raw values, proving the balance invariant.
func mustSafeTransfer(t *testing.T, amount float64, from, to string, balance float64) shenguard.SafeTransfer {
	t.Helper()
	amt, err := shenguard.NewAmount(amount)
	if err != nil {
		t.Fatalf("NewAmount(%v) failed: %v", amount, err)
	}
	tx := shenguard.NewTransaction(amt, shenguard.NewAccountId(from), shenguard.NewAccountId(to))
	check, err := shenguard.NewBalanceChecked(balance, tx)
	if err != nil {
		t.Fatalf("NewBalanceChecked(%v, tx{%v}) failed: %v", balance, amount, err)
	}
	return shenguard.NewSafeTransfer(tx, check)
}

func TestCreateAccount(t *testing.T) {
	p := NewProcessor()

	if err := p.CreateAccount("alice", 100); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	bal, err := p.GetBalance("alice")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if bal != 100 {
		t.Fatalf("expected balance 100, got %.2f", bal)
	}
}

func TestCreateAccountNegativeBalance(t *testing.T) {
	p := NewProcessor()

	// NewAmount rejects negative values — this is the guard type in action
	err := p.CreateAccount("bob", -50)
	if err == nil {
		t.Fatal("expected error for negative balance, got nil")
	}
}

func TestTransfer(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)
	p.CreateAccount("bob", 50)

	safe := mustSafeTransfer(t, 30, "alice", "bob", 100)
	err := p.Transfer(safe)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	aliceBal, _ := p.GetBalance("alice")
	bobBal, _ := p.GetBalance("bob")

	if aliceBal != 70 {
		t.Errorf("expected alice balance 70, got %.2f", aliceBal)
	}
	if bobBal != 80 {
		t.Errorf("expected bob balance 80, got %.2f", bobBal)
	}
}

func TestBalanceCheckRejectsInsufficientFunds(t *testing.T) {
	// The balance check happens at SafeTransfer construction time,
	// not at Transfer time. NewBalanceChecked will reject this.
	amt, err := shenguard.NewAmount(50)
	if err != nil {
		t.Fatal(err)
	}
	tx := shenguard.NewTransaction(amt, shenguard.NewAccountId("alice"), shenguard.NewAccountId("bob"))

	// Alice only has 20 — BalanceChecked should reject
	_, err = shenguard.NewBalanceChecked(20, tx)
	if err == nil {
		t.Fatal("NewBalanceChecked(20, tx{50}) must fail — insufficient funds")
	}
	t.Logf("Correctly rejected at proof construction: %v", err)
}

func TestTransferSelfTransfer(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)

	safe := mustSafeTransfer(t, 10, "alice", "alice", 100)
	err := p.Transfer(safe)
	if !errors.Is(err, ErrSelfTransfer) {
		t.Fatalf("expected ErrSelfTransfer, got %v", err)
	}
}

func TestTransferAccountNotFound(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)

	safe := mustSafeTransfer(t, 10, "alice", "ghost", 100)
	err := p.Transfer(safe)
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestHistory(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)
	p.CreateAccount("bob", 0)

	safe1 := mustSafeTransfer(t, 25, "alice", "bob", 100)
	p.Transfer(safe1)

	safe2 := mustSafeTransfer(t, 10, "alice", "bob", 75)
	p.Transfer(safe2)

	h := p.History()
	if len(h) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(h))
	}
	if h[0].Tx().Amount().Val() != 25 {
		t.Errorf("expected first tx amount 25, got %.2f", h[0].Tx().Amount().Val())
	}
}

// TestBalanceNeverNegative is the key invariant test that mirrors
// the Shen type proof: no sequence of valid transfers can make a balance negative.
func TestBalanceNeverNegative(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("a", 100)
	p.CreateAccount("b", 0)

	// First transfer: 50 from a to b (balance 100 covers it)
	safe1 := mustSafeTransfer(t, 50, "a", "b", 100)
	if err := p.Transfer(safe1); err != nil {
		t.Fatalf("transfer 1: unexpected error: %v", err)
	}

	// Second transfer: 50 from a to b (balance 50 covers it)
	safe2 := mustSafeTransfer(t, 50, "a", "b", 50)
	if err := p.Transfer(safe2); err != nil {
		t.Fatalf("transfer 2: unexpected error: %v", err)
	}

	// Third transfer: 10 from a to b — should fail at BalanceChecked construction
	// because a only has 0 left
	amt, _ := shenguard.NewAmount(10)
	tx := shenguard.NewTransaction(amt, shenguard.NewAccountId("a"), shenguard.NewAccountId("b"))
	_, err := shenguard.NewBalanceChecked(0, tx)
	if err == nil {
		t.Fatal("BalanceChecked should reject: balance 0 < amount 10")
	}

	aBal, _ := p.GetBalance("a")
	bBal, _ := p.GetBalance("b")

	if aBal < 0 {
		t.Fatalf("INVARIANT VIOLATION: account a balance is negative: %.2f", aBal)
	}
	if bBal < 0 {
		t.Fatalf("INVARIANT VIOLATION: account b balance is negative: %.2f", bBal)
	}
}
