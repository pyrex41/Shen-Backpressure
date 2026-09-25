\* Hostile: bind a document owned by any tenant to a tenant-access proof.
   Witness for (= Owner (head (tail Access))) in resource-access. *\
(define bind-foreign-document
  {tenant-access --> resource-id --> tenant-id --> resource-access}
  Access Resource Owner -> [Access Resource Owner])
