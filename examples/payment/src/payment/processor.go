// Package payment implements a simple payment processor with balance invariants.
// The invariants are formally proven in specs/core.shen via Shen's sequent calculus,
// and enforced at compile time through generated guard types in internal/shenguard.
package payment

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"ralph-shen-agent/internal/shenguard"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrAccountNotFound     = errors.New("account not found")
	ErrSelfTransfer        = errors.New("cannot transfer to self")
)

// Account represents a bank account with a non-negative balance.
// Balance is stored as shenguard.Amount, which enforces >= 0 through its constructor.
type Account struct {
	ID      shenguard.AccountId
	Balance shenguard.Amount
}

// Processor manages accounts and executes transfers with balance invariants.
//
// The brand parameter B is the GDP brand of the transfers this processor
// accepts (see internal/shenguard's `Brand`). It is a phantom parameter:
// it costs nothing at runtime and carries the guarantee that a
// SafeTransfer handed to Transfer was built from a Transaction minted in
// the same scope, rather than from some other transaction that happened
// to have a balance proof lying around.
//
// One brand per processor, not one per transfer: Go has no existential
// types, so a heterogeneous history would need an interface box that
// erases the brand again. A caller who wants per-transfer binding
// declares a brand per request scope and keeps a processor per scope.
type Processor[B shenguard.Brand] struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	history  []shenguard.SafeTransfer[B]
}

// NewProcessor creates a new payment processor at the caller's brand.
func NewProcessor[B shenguard.Brand]() *Processor[B] {
	return &Processor[B]{
		accounts: make(map[string]*Account),
	}
}

// CreateAccount adds a new account with the given initial balance.
// Uses shenguard.NewAmount to enforce the non-negative invariant. The
// amount premise is discharged at runtime by the embedded Shen
// evaluator (profile B, `:runtime-via :eval`), so the constructor —
// and therefore this method — takes a context.Context.
func (p *Processor[B]) CreateAccount(ctx context.Context, id string, initialBalance float64) error {
	amt, err := shenguard.NewAmount(ctx, initialBalance)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.accounts[id] = &Account{
		ID:      shenguard.NewAccountId(id),
		Balance: amt,
	}
	return nil
}

// GetBalance returns the current balance for an account.
func (p *Processor[B]) GetBalance(id string) (float64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	acc, ok := p.accounts[id]
	if !ok {
		return 0, ErrAccountNotFound
	}
	return acc.Balance.Val(), nil
}

// Transfer executes a safe transfer between two accounts.
// The caller must construct a shenguard.SafeTransfer, which requires:
//  1. A shenguard.Transaction[B] (with validated Amount, From, To)
//  2. A shenguard.BalanceChecked[B] proof (verifying balance >= amount)
//
// This means the balance invariant is enforced by the type system —
// you cannot call Transfer without first proving sufficient funds, and
// the proof must be about *this* transaction: NewSafeTransfer only
// accepts a BalanceChecked at the same brand as its Transaction.
func (p *Processor[B]) Transfer(ctx context.Context, safe shenguard.SafeTransfer[B]) error {
	tx := safe.Tx()
	fromId := tx.From().Val()
	toId := tx.To().Val()

	if fromId == toId {
		return ErrSelfTransfer
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	from, ok := p.accounts[fromId]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAccountNotFound, fromId)
	}

	to, ok := p.accounts[toId]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAccountNotFound, toId)
	}

	// The balance check was already proven by the BalanceChecked constructor,
	// but we update balances here.
	newFromBal, err := shenguard.NewAmount(ctx, from.Balance.Val() - tx.Amount().Val())
	if err != nil {
		return fmt.Errorf("%w: account %s has %.2f, needs %.2f",
			ErrInsufficientBalance, fromId, from.Balance.Val(), tx.Amount().Val())
	}
	newToBal, err := shenguard.NewAmount(ctx, to.Balance.Val() + tx.Amount().Val())
	if err != nil {
		return err
	}

	from.Balance = newFromBal
	to.Balance = newToBal
	p.history = append(p.history, safe)

	return nil
}

// History returns a copy of all completed safe transfers.
func (p *Processor[B]) History() []shenguard.SafeTransfer[B] {
	p.mu.RLock()
	defer p.mu.RUnlock()

	h := make([]shenguard.SafeTransfer[B], len(p.history))
	copy(h, p.history)
	return h
}
