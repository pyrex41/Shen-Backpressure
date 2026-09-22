\* testdata/vacuous.shen — a deliberately uninhabited datatype.
   `bounded-fee` demands a value that is at least 10 and strictly less
   than 5. No number satisfies both, so the type has no values: any
   guard built from it proves nothing, because no program can ever
   construct one. shen-derive's vacuity pass must report this and fail
   the gate. *\

(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype bounded-fee
  X : number;
  (>= X 10) : verified;
  (< X 5) : verified;
  ====================
  X : bounded-fee;)

\* A derivation target so `verify` has something to sample; it does not
   mention bounded-fee, which is the point — the spec looks fine until
   the inhabitation question is asked. *\

(define total
  {amount --> (list amount) --> number}
  B0 Xs -> (foldr (lambda X (lambda Acc (+ (val X) Acc))) (val B0) Xs))
