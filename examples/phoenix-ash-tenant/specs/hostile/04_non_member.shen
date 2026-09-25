\* Hostile: grant tenant access to a non-member.
   Witness for (= IsMember true) in tenant-access. *\
(define grant-anyway
  {authenticated-principal --> tenant-id --> tenant-access}
  Principal Tenant -> [Principal Tenant false])
