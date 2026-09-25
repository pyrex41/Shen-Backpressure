\* Good: guarded constructions must keep typechecking (positive control). *\
(define mk-amount
  {number --> amount}
  X -> (if (>= X 0) X (simple-error "negative amount")))

(define mk-transfer
  {account-id --> account-id --> amount --> transfer}
  From To Amt -> (if (not (= From To)) [From To Amt] (simple-error "self transfer")))
