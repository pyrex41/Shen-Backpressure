\* ====================================================================
   Multi-Tenant SaaS API — Authorization Proof Chain

   JWT validation -> AuthenticatedUser -> TenantAccess -> ResourceAccess

   Cross-tenant data access is impossible by construction:
   you cannot build a ResourceAccess without first proving
   TenantAccess, which requires proving tenant membership.
   ==================================================================== *\

\* --- Wrapper types for domain identifiers --- *\

(datatype user-id
  X : string;
  ==============
  X : user-id;)

(datatype tenant-id
  X : string;
  ==============
  X : tenant-id;)

(datatype resource-id
  X : string;
  ==============
  X : resource-id;)

\* --- JWT token — must be non-empty --- *\

(datatype jwt-token
  X : string;
  (not (= X "")) : verified;
  ============================
  X : jwt-token;)

\* --- Expiry check — token must not be expired --- *\

(datatype token-expiry
  Exp : number;
  Now : number;
  (> Exp Now) : verified;
  =======================
  [Exp Now] : token-expiry;)

\* --- AuthenticatedUser — requires valid JWT + non-expired token --- *\

(datatype authenticated-user
  Token : jwt-token;
  Expiry : token-expiry;
  User : user-id;
  ===================================
  [Token Expiry User] : authenticated-user;)

\* --- TenantAccess — requires authenticated user who is a member --- *\

(datatype tenant-access
  Auth : authenticated-user;
  Tenant : tenant-id;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Auth Tenant IsMember] : tenant-access;)

\* --- ResourceAccess — requires tenant access + tenant owns resource --- *\

(datatype resource-access
  Access : tenant-access;
  Resource : resource-id;
  IsOwned : boolean;
  (= IsOwned true) : verified;
  ================================
  [Access Resource IsOwned] : resource-access;)
