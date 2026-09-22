\* specs/core.shen - Formal type specifications for Ralph-Shen backpressure *\
\* Domain: Payment processor with balance invariants *\

\* --- Basic types --- *\

(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified; \* :runtime-via :eval *\
  ====================
  X : amount;)

(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)

\* --- Balance invariant: balance must cover transaction amount --- *\

(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)

\* --- Account state --- *\

(datatype account-state
  Id : account-id;
  Balance : amount;
  ========================
  [Id Balance] : account-state;)

\* --- Safe transfer: a transaction that has passed the balance check --- *\

(datatype safe-transfer
  Tx : transaction;
  Check : balance-checked;
  =============================
  [Tx Check] : safe-transfer;)

\* --- Derivation targets (consumed by shen-derive, not shengen) --- *\

\* processable: starting from balance B0, is every running balance
   non-negative after applying each transaction in order?

   `val` is the `amount` destructor: it takes an `amount` to the
   `number` it wraps. It is therefore applied to `B0` and to the
   `amount` field of each transaction, and NOT to the running balances
   `scanl` produces — those are plain numbers, and the whole point of
   the predicate is that one of them may be *negative*, which is
   exactly what an `amount` may not be. Applying `val` to them (as
   this define did until W6) type-checks in shen-derive's evaluator
   only because the evaluator binds `val` to the identity function; it
   is ill-typed under Shen's `tc +`, and gate 4 had never passed on
   this spec as a result. Removing those two applications changes no
   evaluated value — see W6 in
   thoughts/shared/plans/2026-09-22-verifier-throughput-roadmap.md. *\

(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= X 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- B (val (amount Tx)))))
                     (val B0)
                     Txs)))
