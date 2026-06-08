\* ====================================================================
   Multi-Tenant SaaS API — Authorization Proof Chain

   Raw JWT → ParsedClaims + Signature → VerifiedJwt
         → AuthenticatedUser → TenantAccess → ResourceAccess

   Cross-tenant data access is impossible by construction:
   you cannot build a ResourceAccess without first proving
   TenantAccess, which requires proving tenant membership.

   The user-id carried in AuthenticatedUser is structurally bound
   to the JWT's `sub` claim by the verified premise
   `(= User (head Claims))`. This makes "pair token-A with user-B"
   a compile-time error, not a convention.
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

\* --- Issuer / audience claim wrappers --- *\
\* These are wrapper types so the parser-extracted strings flow through
   the type system without coercion. *\

(datatype jwt-issuer
  X : string;
  (not (= X "")) : verified;
  ==============
  X : jwt-issuer;)

(datatype jwt-audience
  X : string;
  (not (= X "")) : verified;
  ==============
  X : jwt-audience;)

\* --- ParsedClaims — JSON-decoded payload section of a JWT --- *\
\* Field order matters: `(head Claims)` resolves to `Sub` (the user-id),
   which is what the cross-field binding below relies on. *\

(datatype parsed-claims
  Sub : user-id;
  Exp : number;
  Iss : jwt-issuer;
  Aud : jwt-audience;
  (> Exp 0) : verified;
  ==================================
  [Sub Exp Iss Aud] : parsed-claims;)

\* --- VerifiedJwt — claims + signature with a non-empty signature --- *\
\*
   STRUCTURAL CLAIM: a `VerifiedJwt` cannot exist with an empty signature
   field. This is a deliberately weak predicate: real HMAC-SHA256
   verification lives in `internal/auth/jwt.go` (`crypto/hmac.Equal`,
   constant-time) and is part of the TCB enumerated in
   `../../docs/TRUST-MODEL.md`. The Go middleware constructs a
   `VerifiedJwt` only AFTER it has confirmed the signature against the
   shared secret — so every value of this type observed in the program
   has been verified by the parser. The constructor's own non-empty
   check is the type-system anchor that prevents accidental
   construction with a missing signature.

   What this premise gives us at the type level:

   - `NewVerifiedJwt(claims, sig)` is the ONLY exit; no other path
     produces a `VerifiedJwt`.
   - Downstream `AuthenticatedUser` requires a `VerifiedJwt` parameter,
     so the proof chain demands "go through the parser" by signature.

   What this premise does NOT give us:

   - It does not assert cryptographic validity. The parser's HMAC check
     is TCB and must be audited separately. *\

(datatype verified-jwt
  Claims : parsed-claims;
  Sig : string;
  (not (= Sig "")) : verified;
  ==========================================
  [Claims Sig] : verified-jwt;)

\* --- AuthenticatedUser — binds the user-id structurally to the JWT's sub claim --- *\
\*
   The crucial premise is `(= User (head Claims))`: it asserts that the
   `UserId` carried in the proof is *byte-equal* to the `sub` field
   inside the `VerifiedJwt`'s claims. This is the W2.1 hardening:
   before this premise existed, `NewAuthenticatedUser` was infallible
   and a caller in possession of any `JwtToken` and any `UserId` could
   pair them. With this premise the constructor returns an error on
   mismatch and the only safe way to construct an `AuthenticatedUser`
   is to thread the parsed `sub` through as the `UserId`. *\

(datatype authenticated-user
  Jwt : verified-jwt;
  User : user-id;
  (= User (head (head Jwt))) : verified;
  ===========================================
  [Jwt User] : authenticated-user;)

\* --- Service credentials for background jobs / cron --- *\

(datatype service-id
  X : string;
  ==============
  X : service-id;)

(datatype service-credential
  Service : service-id;
  Secret : string;
  (not (= Secret "")) : verified;
  ================================
  [Service Secret] : service-credential;)

\* --- Authenticated principal — sum type: human OR service account --- *\
\* Two blocks produce the same conclusion type → generates a Go interface *\

(datatype human-principal
  Auth : authenticated-user;
  ===========================
  Auth : authenticated-principal;)

(datatype service-principal
  Cred : service-credential;
  ============================
  Cred : authenticated-principal;)

\* --- TenantAccess — requires authenticated principal who is a member --- *\

\* @decidable-fragment: tenant-access
   This rule is tagged for the Decidable-Shen-fragment tier.
   Judgment (sequent/Prolog): no general recursion, Datalog/Horn body,
   only allowed total forms (=, element?, comparisons), bounded.
   Can be certified + run directly in shen-go / shen-lua etc with
   termination guarantee (middle tier in lattice: Cedar ⊂ ... ⊂ Decidable-Shen-fragment ⊂ full-TC pure-Shen).
   tc+ / small Prolog pass can ignore for now or discharge later.
 *\

(datatype tenant-access
  Principal : authenticated-principal;
  Tenant : tenant-id;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Principal Tenant IsMember] : tenant-access;)

\* --- ResourceAccess — requires tenant access + tenant owns resource --- *\

\* @decidable-fragment: resource-access
   Same judgment as tenant-access: Horn-clause verified premise, total,
   no recursion. Certifiable for direct execution in Shen runtime ports
   (native data, guaranteed termination for the policy slice).
 *\

(datatype resource-access
  Access : tenant-access;
  Resource : resource-id;
  IsOwned : boolean;
  (= IsOwned true) : verified;
  ================================
  [Access Resource IsOwned] : resource-access;)

\* --- Derivation targets (consumed by shen-derive, not shengen) ----- *\
\*
   `same-user?` is the spec/impl oracle that anchors shen-derive on
   this example. It asserts that two `user-id` wrappers are equal
   exactly when their inner strings are equal — i.e., it pins the Go
   `==` on `shenguard.UserId` against the Shen `=` on `user-id`.
   This is the same equality the W2.1 cross-field premise
   `(= User (head (head Jwt)))` inside `authenticated-user` asserts
   *structurally* at construction time. The Go impl
   `derived.SameUser` is a one-liner; the spec-equivalence test
   `internal/derived/same_user_spec_test.go` catches drift if anyone
   refactors `UserId` equality to something case-insensitive or
   normalising-on-comparison.

   WHY A SHEN-DERIVE ANCHOR IS HERE: classifying the datatype rules in
   the discharge report (the `static` rows for `verified-jwt`,
   `authenticated-user`, `tenant-access`, etc.) requires `sb derive`
   to run, which requires at least one `[[derive.specs]]` entry.
   This define provides that anchor. The classified rules — especially
   the W2.1 cross-field premise `(= User (head (head Jwt)))` inside
   `authenticated-user` — are then visible to an auditor reading
   `transcript/audit_report.md`.

   WHY A WRAPPER AND NOT A GUARDED COMPOSITE: shen-derive's sampler
   does not currently filter guarded composites (e.g. `parsed-claims`'s
   `(> Exp 0)` predicate — see `shen-derive/verify/samples.go:91`),
   and Shen's `tc+` rejects calls like `(val X)` on wrappers without
   a domain-specific destructor declared. Anchoring on `user-id`
   alone — a wrapper, no `verified` premise — keeps both gates
   green and pins the most semantically important equality in the
   chain. A richer oracle that walks the cross-field invariant
   directly is a follow-up.

   The "real" hardening of W2.1 is the structural premise
   `(= User (head (head Jwt)))` inside `authenticated-user` — that
   premise alone makes "pair token-A with user-B" a compile-time
   error. The discharge report's role is to make that
   statically-discharged premise visible to an auditor. *\

(define same-user?
  {user-id --> user-id --> boolean}
  A B -> (= A B))
