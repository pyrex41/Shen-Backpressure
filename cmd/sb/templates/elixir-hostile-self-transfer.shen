\* Hostile: a transfer from an account to itself. Witness for (not (= From To)). *\
(define self-transfer
  {account-id --> amount --> transfer}
  A Amt -> [A A Amt])
