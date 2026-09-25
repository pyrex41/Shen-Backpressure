\* Good: constructions that discharge every premise with a guarding `if`
   (needs the verified-if rule in specs/verified.shen). These must keep
   typechecking; they are the positive control for sb mutate-spec. *\
(define bind-tenant
  {authenticated-principal --> tenant-id --> boolean --> tenant-access}
  Principal Tenant IsMember ->
    (if (= IsMember true)
        [Principal Tenant IsMember]
        (simple-error "not a member")))

(define sign-in
  {user-id --> string --> verified-token}
  User Sig -> (if (not (= Sig "")) [User Sig] (simple-error "unsigned")))

(define operator-login
  {operator-id --> string --> operator-credential}
  Op Secret -> (if (not (= Secret "")) [Op Secret] (simple-error "no secret")))

(define grant-write
  {resource-access --> role --> write-access}
  Access Role -> (if (can-write? Role) [Access Role] (simple-error "read-only role")))
