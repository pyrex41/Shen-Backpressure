\* Hostile: pair any token with any user id (token-A + user-B).
   Witness for (= User (head Token)) in authenticated-user. *\
(define pair-any
  {verified-token --> user-id --> authenticated-user}
  Token User -> [Token User])
