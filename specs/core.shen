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
