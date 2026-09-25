\* Hostile: write access for any role, including viewers.
   Witness for (can-write? Role) in write-access. *\
(define write-anyway
  {resource-access --> role --> write-access}
  Access Role -> [Access Role])
