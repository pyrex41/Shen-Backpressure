\* Hostile: accept any string as a role.
   Witness for (element? X ["viewer" "editor" "admin"]) in role. *\
(define any-role
  {string --> role}
  X -> X)
