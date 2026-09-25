\* ====================================================================
   Tenant-scoped documents — authorization proof chain (Elixir/Ash port
   of examples/multi-tenant-api).

   verified-token → authenticated-user ─┐
   operator-credential ─────────────────┴→ authenticated-principal
       → tenant-access → resource-access → write-access

   A resource-access cannot be built for a document owned by a
   different tenant than the one the tenant-access proof was issued
   for: `(= Owner (head (tail Access)))` is a premise, so the
   cross-tenant case is a constructor error in Elixir and a type error
   in Shen.
   ==================================================================== *\

\* --- identifiers ---------------------------------------------------- *\

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

(datatype operator-id
  X : string;
  ==============
  X : operator-id;)

\* --- authentication ------------------------------------------------- *\

\* A token whose signature was checked by the (TCB) verifier. The
   premise only anchors "no empty signature"; HMAC verification is the
   Check wrapper's job (Tenant.Authn.verify_token/2). *\
(datatype verified-token
  Sub : user-id;
  Sig : string;
  (not (= Sig "")) : verified;
  ==============================
  [Sub Sig] : verified-token;)

\* The user id is structurally bound to the token subject. *\
(datatype authenticated-user
  Token : verified-token;
  User : user-id;
  (= User (head Token)) : verified;
  ==================================
  [Token User] : authenticated-user;)

\* Operators (background jobs, support staff) authenticate with a
   shared secret instead of a user token. *\
(datatype operator-credential
  Operator : operator-id;
  Secret : string;
  (not (= Secret "")) : verified;
  ========================================
  [Operator Secret] : operator-credential;)

\* --- principal: human user OR operator (sum type) ------------------- *\
\* Single-line (right-only) rules: a user or operator IS a principal. The
   double-line form would also add left rules, which send the typechecker
   into an unbounded search when a principal is pattern-bound. *\

(datatype human-principal
  Auth : authenticated-user;
  ___________________________
  Auth : authenticated-principal;)

(datatype operator-principal
  Op : operator-credential;
  ____________________________
  Op : authenticated-principal;)

\* --- authorization ---------------------------------------------------- *\

(datatype tenant-access
  Principal : authenticated-principal;
  Tenant : tenant-id;
  IsMember : boolean;
  (= IsMember true) : verified;
  ============================================
  [Principal Tenant IsMember] : tenant-access;)

\* `(head (tail Access))` is the Tenant field of the tenant-access proof
   (field order: Principal, Tenant, IsMember). *\
(datatype resource-access
  Access : tenant-access;
  Resource : resource-id;
  Owner : tenant-id;
  (= Owner (head (tail Access))) : verified;
  ==============================================
  [Access Resource Owner] : resource-access;)

(datatype role
  X : string;
  (element? X ["viewer" "editor" "admin"]) : verified;
  =====================================================
  X : role;)

(define can-write?
  {string --> boolean}
  "editor" -> true
  "admin" -> true
  _ -> false)

(datatype write-access
  Access : resource-access;
  Role : role;
  (can-write? Role) : verified;
  ================================
  [Access Role] : write-access;)

\* --- pure functions pinned by shen-derive (sb derive, lang = elixir) --- *\

(define member-of?
  {string --> (list string) --> boolean}
  _ [] -> false
  X [X | _] -> true
  X [_ | Ys] -> (member-of? X Ys))

(define visible-titles
  {string --> (list (list string)) --> (list string)}
  _ [] -> []
  Tenant [[Tenant Title] | Docs] -> [Title | (visible-titles Tenant Docs)]
  Tenant [_ | Docs] -> (visible-titles Tenant Docs))
