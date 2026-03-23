// Package payment implements a simple payment processor with balance invariants.
// The invariants are formally proven in specs/core.shen via Shen's sequent calculus.
package payment

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrNegativeAmount      = errors.New("amount must be non-negative")
	ErrAccountNotFound     = errors.New("account not found")
	ErrSelfTransfer        = errors.New("cannot transfer to self")
)

// Account represents a bank account with a non-negative balance.
// Shen invariant: (datatype amount) enforces X >= 0.
type Account struct {
	ID      string
	Balance float64
}

// Transaction represents a transfer between accounts.
// Shen invariant: (datatype transaction) requires Amount : amount, From/To : account-id.
type Transaction struct {
	Amount float64
	From   string
	To     string
}

// Processor manages accounts and executes transfers with balance invariants.
type Processor struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	history  []Transaction
}

// NewProcessor creates a new payment processor.
func NewProcessor() *Processor {
	return &Processor{
		accounts: make(map[string]*Account),
	}
}

// CreateAccount adds a new account with the given initial balance.
// Enforces: balance >= 0 (corresponds to Shen's amount type).
func (p *Processor) CreateAccount(id string, initialBalance float64) error {
	if initialBalance < 0 {
		return ErrNegativeAmount
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.accounts[id] = &Account{ID: id, Balance: initialBalance}
	return nil
}

// GetBalance returns the current balance for an account.
func (p *Processor) GetBalance(id string) (float64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	acc, ok := p.accounts[id]
	if !ok {
		return 0, ErrAccountNotFound
	}
	return acc.Balance, nil
}

// Transfer executes a transaction between two accounts.
// Enforces the balance-invariant: sender balance >= transfer amount.
// This is the Go-side enforcement of specs/core.shen's (datatype balance-invariant).
func (p *Processor) Transfer(tx Transaction) error {
	if tx.Amount < 0 {
		return ErrNegativeAmount
	}
	if tx.From == tx.To {
		return ErrSelfTransfer
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	from, ok := p.accounts[tx.From]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAccountNotFound, tx.From)
	}

	to, ok := p.accounts[tx.To]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAccountNotFound, tx.To)
	}

	// Balance invariant check — corresponds to Shen's:
	//   (>= Bal (head Tx)) : verified;
	if from.Balance < tx.Amount {
		return fmt.Errorf("%w: account %s has %.2f, needs %.2f",
			ErrInsufficientBalance, tx.From, from.Balance, tx.Amount)
	}

	from.Balance -= tx.Amount
	to.Balance += tx.Amount
	p.history = append(p.history, tx)

	return nil
}

// History returns a copy of all completed transactions.
func (p *Processor) History() []Transaction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	h := make([]Transaction, len(p.history))
	copy(h, p.history)
	return h
}
