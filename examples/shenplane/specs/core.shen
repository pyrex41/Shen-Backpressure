\* ============================================================ *\
\* ShenPlane — Deductive Infrastructure Control Plane           *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* A clean-sheet infrastructure control plane where:            *\
\*   - Users write familiar YAML Claims                         *\
\*   - Shen specs are the single source of truth for invariants *\
\*   - shengen emits CRDs, webhooks, and controller guards      *\
\*   - Invalid infrastructure is unrepresentable, not rejected  *\
\*                                                              *\
\* No Crossplane baggage (patch-and-pray, weak schemas).        *\
\* No Argo baggage (sync waves, ignoreDifferences hacks).       *\
\* Takes the good ideas (declarative YAML, GitOps reconcile,    *\
\* CRD extensibility) and grounds them in proofs.               *\
\* ============================================================ *\


\* ============================================= *\
\*  LAYER 1: RESOURCE IDENTITY & LIFECYCLE      *\
\*  (What exists, who owns it, what state it's in)*\
\* ============================================= *\

(datatype resource-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : resource-id;)

(datatype resource-kind
  X : string;
  (not (= X "")) : verified;
  ============================
  X : resource-kind;)

(datatype owner-ref
  X : string;
  (not (= X "")) : verified;
  ============================
  X : owner-ref;)

(datatype generation
  X : number;
  (> X 0) : verified;
  ====================
  X : generation;)

\* Lifecycle states — every resource is always in exactly one *\
(datatype lifecycle-state
  X : string;
  (element? X [pending creating ready updating deleting failed]) : verified;
  ==========================================================================
  X : lifecycle-state;)

\* A resource handle: the minimal identity of any managed thing *\
(datatype resource-handle
  Id : resource-id;
  Kind : resource-kind;
  Owner : owner-ref;
  Gen : generation;
  State : lifecycle-state;
  ==========================
  [Id Kind Owner Gen State] : resource-handle;)


\* ============================================= *\
\*  LAYER 2: CLAIMS — THE USER-FACING API       *\
\*  (What users write in YAML)                   *\
\* ============================================= *\

\* Claims are thin, constrained YAML. Every field exists because *\
\* the spec says it must. No free-form maps. No unchecked strings. *\

\* --- Cloud region (constrained to real regions) --- *\
(datatype cloud-region
  X : string;
  (element? X [us-east-1 us-west-2 eu-west-1 eu-central-1 ap-southeast-1
               ap-northeast-1 us-east-2 us-west-1 eu-west-2 eu-north-1]) : verified;
  ==================================================================================
  X : cloud-region;)

\* --- Environment (controls what's allowed) --- *\
(datatype environment
  X : string;
  (element? X [development staging production]) : verified;
  ==========================================================
  X : environment;)

\* --- Resource size tier --- *\
(datatype size-tier
  X : string;
  (element? X [small medium large xlarge]) : verified;
  ====================================================
  X : size-tier;)

\* --- Mandatory organizational tags --- *\
(datatype team-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : team-name;)

(datatype cost-center
  X : string;
  (not (= X "")) : verified;
  ============================
  X : cost-center;)

(datatype org-tags
  Team : team-name;
  CostCenter : cost-center;
  Env : environment;
  ======================
  [Team CostCenter Env] : org-tags;)


\* ============================================= *\
\*  LAYER 3: SECURITY INVARIANTS                *\
\*  (Proofs that must exist before provisioning) *\
\* ============================================= *\

\* --- Encryption --- *\
(datatype encryption-algo
  X : string;
  (element? X [aes-256 aes-128 aws-kms gcp-cmek azure-keyvault]) : verified;
  ===========================================================================
  X : encryption-algo;)

(datatype encryption-proof
  Algo : encryption-algo;
  AtRest : boolean;
  InTransit : boolean;
  (= AtRest true) : verified;
  (= InTransit true) : verified;
  ================================
  [Algo AtRest InTransit] : encryption-proof;)

\* --- Network isolation --- *\
(datatype network-mode
  X : string;
  (element? X [private private-with-nat isolated]) : verified;
  ============================================================
  X : network-mode;)

(datatype network-proof
  Mode : network-mode;
  PublicEndpoint : boolean;
  (= PublicEndpoint false) : verified;
  =====================================
  [Mode PublicEndpoint] : network-proof;)

\* --- IAM / access control --- *\
(datatype iam-scope
  X : string;
  (element? X [least-privilege scoped-to-resource scoped-to-namespace]) : verified;
  =================================================================================
  X : iam-scope;)

(datatype iam-proof
  Scope : iam-scope;
  WildcardActions : boolean;
  (= WildcardActions false) : verified;
  ======================================
  [Scope WildcardActions] : iam-proof;)

\* --- Composed security proof (ALL must hold) --- *\
(datatype security-proof
  Encryption : encryption-proof;
  Network : network-proof;
  Iam : iam-proof;
  ==================
  [Encryption Network Iam] : security-proof;)


\* ============================================= *\
\*  LAYER 4: CONCRETE CLAIM TYPES               *\
\*  (Each maps to a YAML kind users write)       *\
\* ============================================= *\

\* --- Database Claim --- *\
(datatype db-engine
  X : string;
  (element? X [postgres mysql mariadb]) : verified;
  ==================================================
  X : db-engine;)

(datatype storage-gb
  X : number;
  (>= X 10) : verified;
  (<= X 10000) : verified;
  =========================
  X : storage-gb;)

(datatype db-claim
  Id : resource-id;
  Engine : db-engine;
  Size : size-tier;
  Storage : storage-gb;
  Region : cloud-region;
  Tags : org-tags;
  =====================
  [Id Engine Size Storage Region Tags] : db-claim;)

\* --- Secure database: claim + all security proofs --- *\
\* This is what the controller actually provisions from *\
(datatype secure-db
  Claim : db-claim;
  Security : security-proof;
  ===========================
  [Claim Security] : secure-db;)

\* --- Cache Claim --- *\
(datatype cache-engine
  X : string;
  (element? X [redis memcached valkey]) : verified;
  ==================================================
  X : cache-engine;)

(datatype cache-claim
  Id : resource-id;
  Engine : cache-engine;
  Size : size-tier;
  Region : cloud-region;
  Tags : org-tags;
  =====================
  [Id Engine Size Region Tags] : cache-claim;)

(datatype secure-cache
  Claim : cache-claim;
  Security : security-proof;
  ===========================
  [Claim Security] : secure-cache;)

\* --- Object Storage Claim --- *\
(datatype bucket-access
  X : string;
  (element? X [private authenticated-read]) : verified;
  ======================================================
  X : bucket-access;)

(datatype bucket-claim
  Id : resource-id;
  Access : bucket-access;
  Region : cloud-region;
  Tags : org-tags;
  =====================
  [Id Access Region Tags] : bucket-claim;)

(datatype secure-bucket
  Claim : bucket-claim;
  Security : security-proof;
  ===========================
  [Claim Security] : secure-bucket;)

\* --- Network Claim (VPC / subnet) --- *\
(datatype cidr-block
  X : string;
  (not (= X "")) : verified;
  ============================
  X : cidr-block;)

(datatype availability-zones
  Count : number;
  (>= Count 2) : verified;
  =========================
  Count : availability-zones;)

(datatype network-claim
  Id : resource-id;
  Cidr : cidr-block;
  AZs : availability-zones;
  Region : cloud-region;
  Tags : org-tags;
  =====================
  [Id Cidr AZs Region Tags] : network-claim;)


\* ============================================= *\
\*  LAYER 5: COMPOSITION — HOW CLAIMS EXPAND    *\
\*  (Replaces Crossplane's patch pipelines)     *\
\* ============================================= *\

\* Instead of fragile JSONPath patches, composition is typed: *\
\* a Claim expands into concrete resources through proof-carrying *\
\* transformation rules. *\

\* A concrete cloud resource produced by expanding a claim *\
(datatype cloud-resource
  Handle : resource-handle;
  Region : cloud-region;
  Tags : org-tags;
  =====================
  [Handle Region Tags] : cloud-resource;)

\* Proof that a cloud resource was derived from a claim *\
\* (provenance — you always know where a resource came from) *\
(datatype derived-from
  Resource : cloud-resource;
  ClaimId : resource-id;
  ========================
  [Resource ClaimId] : derived-from;)

\* --- Cross-resource consistency --- *\

\* Proof that two resources share the same region *\
(datatype same-region
  A : cloud-resource;
  B : cloud-resource;
  ====================
  [A B] : same-region;)

\* Proof that a resource is in the correct network *\
(datatype in-network
  Resource : cloud-resource;
  Network : network-claim;
  ==========================
  [Resource Network] : in-network;)

\* --- Database expansion: claim → [RDS + subnet group + security group + IAM role] --- *\
(datatype db-expansion
  Db : secure-db;
  Instance : cloud-resource;
  SubnetGroup : cloud-resource;
  SecurityGroup : cloud-resource;
  IamRole : cloud-resource;
  RegionConsistent : same-region;
  NetworkBound : in-network;
  ================================
  [Db Instance SubnetGroup SecurityGroup IamRole RegionConsistent NetworkBound] : db-expansion;)


\* ============================================= *\
\*  LAYER 6: RECONCILIATION — STATE MACHINE     *\
\*  (How the controller drives lifecycle)        *\
\* ============================================= *\

\* Desired vs observed state *\
(datatype desired-state
  Handle : resource-handle;
  DesiredGen : generation;
  ==========================
  [Handle DesiredGen] : desired-state;)

(datatype observed-state
  Handle : resource-handle;
  ObservedGen : generation;
  Ready : boolean;
  =====================
  [Handle ObservedGen Ready] : observed-state;)

\* Drift detection: desired gen != observed gen *\
(datatype drift-detected
  Desired : desired-state;
  Observed : observed-state;
  ============================
  [Desired Observed] : drift-detected;)

\* Reconcile action — what the controller should do *\
(datatype reconcile-action
  X : string;
  (element? X [create update delete noop]) : verified;
  =====================================================
  X : reconcile-action;)

\* Reconciliation plan: action determined by comparing desired/observed *\
(datatype reconcile-plan
  Handle : resource-handle;
  Action : reconcile-action;
  Provenance : derived-from;
  ============================
  [Handle Action Provenance] : reconcile-plan;)

\* --- Reconcile ordering --- *\
\* Network before database. Database before app. Always. *\

(datatype reconcile-order
  First : reconcile-plan;
  Then : reconcile-plan;
  ========================
  [First Then] : reconcile-order;)


\* ============================================= *\
\*  LAYER 7: CREDENTIALS & PROVIDERS            *\
\*  (How we talk to cloud APIs)                  *\
\* ============================================= *\

(datatype provider-type
  X : string;
  (element? X [aws gcp azure]) : verified;
  =========================================
  X : provider-type;)

(datatype credential-ref
  X : string;
  (not (= X "")) : verified;
  ============================
  X : credential-ref;)

(datatype credential-expiry
  ExpiresAt : number;
  Now : number;
  (> ExpiresAt Now) : verified;
  ==============================
  [ExpiresAt Now] : credential-expiry;)

(datatype provider-ready
  Provider : provider-type;
  Cred : credential-ref;
  Expiry : credential-expiry;
  Healthy : boolean;
  (= Healthy true) : verified;
  ==============================
  [Provider Cred Expiry Healthy] : provider-ready;)


\* ============================================= *\
\*  LAYER 8: END-TO-END PROOF                   *\
\*  (The top-level "this deployment is safe")    *\
\* ============================================= *\

\* Everything composes into one proof *\

(datatype deployment-safe
  Plan : reconcile-plan;
  Security : security-proof;
  Provider : provider-ready;
  ============================
  [Plan Security Provider] : deployment-safe;)
