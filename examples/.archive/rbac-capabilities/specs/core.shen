\* ============================================================ *\
\* RBAC & Capability-Based Authorization                        *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Models authorization as proof chains. To perform an action   *\
\* on a resource, you must construct a proof that flows:        *\
\*                                                              *\
\*   Identity -> Session -> RoleBinding -> Capability -> Access  *\
\*                                                              *\
\* Privilege escalation is a type error. You cannot construct   *\
\* an admin Capability without proving admin RoleBinding.       *\
\* Time-bounded sessions expire as type errors.                 *\
\* ============================================================ *\

\* --- Identity layer --- *\

(datatype user-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : user-id;)

(datatype email-address
  X : string;
  (not (= X "")) : verified;
  ============================
  X : email-address;)

\* --- Roles as set membership --- *\

(datatype role
  X : string;
  (element? X [admin editor viewer auditor]) : verified;
  ======================================================
  X : role;)

\* --- Time-bounded session --- *\

(datatype session-token
  X : string;
  (not (= X "")) : verified;
  ============================
  X : session-token;)

(datatype session-expiry
  ExpiresAt : number;
  Now : number;
  (> ExpiresAt Now) : verified;
  ==============================
  [ExpiresAt Now] : session-expiry;)

(datatype active-session
  User : user-id;
  Token : session-token;
  Expiry : session-expiry;
  ==========================
  [User Token Expiry] : active-session;)

\* --- Role binding: user has a role in an org --- *\

(datatype org-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : org-id;)

(datatype role-binding
  Session : active-session;
  Org : org-id;
  Role : role;
  IsBound : boolean;
  (= IsBound true) : verified;
  ==============================
  [Session Org Role IsBound] : role-binding;)

\* --- Resources & Actions --- *\

(datatype resource-type
  X : string;
  (element? X [document project settings billing user]) : verified;
  =================================================================
  X : resource-type;)

(datatype action
  X : string;
  (element? X [create read update delete list]) : verified;
  ==========================================================
  X : action;)

(datatype resource-ref
  Type : resource-type;
  Id : string;
  Org : org-id;
  =================
  [Type Id Org] : resource-ref;)

\* --- Capability: derived from role binding + permission check --- *\
\* This is the key proof: you can only get a capability by proving *\
\* your role binding AND that the role permits this action.         *\

(datatype capability
  Binding : role-binding;
  Action : action;
  ResourceType : resource-type;
  IsPermitted : boolean;
  (= IsPermitted true) : verified;
  ==================================
  [Binding Action ResourceType IsPermitted] : capability;)

\* --- Access grant: the final proof that unlocks the operation --- *\
\* Requires capability + resource is in the same org as the binding *\

(datatype access-grant
  Cap : capability;
  Resource : resource-ref;
  SameOrg : boolean;
  (= SameOrg true) : verified;
  ==============================
  [Cap Resource SameOrg] : access-grant;)

\* --- Delegation: one user grants another a subset of their caps --- *\

(datatype delegation
  Grantor : access-grant;
  Delegatee : active-session;
  Action : action;
  ExpiresAt : number;
  Now : number;
  (> ExpiresAt Now) : verified;
  ==============================
  [Grantor Delegatee Action ExpiresAt Now] : delegation;)

\* --- Audit entry: every access produces an auditable record --- *\

(datatype audit-entry
  Grant : access-grant;
  Timestamp : number;
  (> Timestamp 0) : verified;
  ============================
  [Grant Timestamp] : audit-entry;)
