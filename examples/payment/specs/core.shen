\* specs/core.shen - Formal type specifications for Ralph-Shen backpressure *\
\* Domain: Payment processor with balance invariants *\

\* --- Basic types --- *\

(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified;
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
   non-negative after applying each transaction in order? *\

(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx)))))
                     (val B0)
                     Txs)))
