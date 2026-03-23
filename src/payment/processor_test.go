package payment

import (
	"errors"
	"testing"
)

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

	err := p.CreateAccount("bob", -50)
	if !errors.Is(err, ErrNegativeAmount) {
		t.Fatalf("expected ErrNegativeAmount, got %v", err)
	}
}

func TestTransfer(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)
	p.CreateAccount("bob", 50)

	err := p.Transfer(Transaction{Amount: 30, From: "alice", To: "bob"})
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

func TestTransferInsufficientBalance(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 20)
	p.CreateAccount("bob", 0)

	err := p.Transfer(Transaction{Amount: 50, From: "alice", To: "bob"})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}

	// Balance must be unchanged — backpressure in action
	aliceBal, _ := p.GetBalance("alice")
	if aliceBal != 20 {
		t.Errorf("expected alice balance unchanged at 20, got %.2f", aliceBal)
	}
}

func TestTransferNegativeAmount(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)
	p.CreateAccount("bob", 100)

	err := p.Transfer(Transaction{Amount: -10, From: "alice", To: "bob"})
	if !errors.Is(err, ErrNegativeAmount) {
		t.Fatalf("expected ErrNegativeAmount, got %v", err)
	}
}

func TestTransferSelfTransfer(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)

	err := p.Transfer(Transaction{Amount: 10, From: "alice", To: "alice"})
	if !errors.Is(err, ErrSelfTransfer) {
		t.Fatalf("expected ErrSelfTransfer, got %v", err)
	}
}

func TestTransferAccountNotFound(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)

	err := p.Transfer(Transaction{Amount: 10, From: "alice", To: "ghost"})
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestHistory(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("alice", 100)
	p.CreateAccount("bob", 0)

	p.Transfer(Transaction{Amount: 25, From: "alice", To: "bob"})
	p.Transfer(Transaction{Amount: 10, From: "alice", To: "bob"})

	h := p.History()
	if len(h) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(h))
	}
	if h[0].Amount != 25 {
		t.Errorf("expected first tx amount 25, got %.2f", h[0].Amount)
	}
}

// TestBalanceNeverNegative is the key invariant test that mirrors
// the Shen type proof: no sequence of valid transfers can make a balance negative.
func TestBalanceNeverNegative(t *testing.T) {
	p := NewProcessor()
	p.CreateAccount("a", 100)
	p.CreateAccount("b", 0)

	transfers := []Transaction{
		{Amount: 50, From: "a", To: "b"},
		{Amount: 50, From: "a", To: "b"},
		{Amount: 10, From: "a", To: "b"}, // should fail: only 0 left
	}

	for i, tx := range transfers {
		err := p.Transfer(tx)
		if i == 2 {
			if !errors.Is(err, ErrInsufficientBalance) {
				t.Fatalf("transfer %d: expected insufficient balance error, got %v", i, err)
			}
		} else if err != nil {
			t.Fatalf("transfer %d: unexpected error: %v", i, err)
		}
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
