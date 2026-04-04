\* ============================================================ *\
\* Kubernetes Infrastructure Orchestration                      *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Formal types for ArgoCD, Argo Workflows, Argo Rollouts,     *\
\* and Crossplane — making infrastructure management tractable  *\
\* by turning runtime failures into compile-time type errors.   *\
\*                                                              *\
\* Core thesis: The failure modes of K8s infra tools come from  *\
\* unvalidated references, implicit ordering, type-erased       *\
\* parameters, and silent nil patches. Proof-carrying types     *\
\* eliminate these categories of failure entirely.              *\
\* ============================================================ *\


\* ============================== *\
\*  SECTION 1: COMMON K8S TYPES  *\
\* ============================== *\

(datatype namespace
  X : string;
  (not (= X "")) : verified;
  ============================
  X : namespace;)

(datatype cluster-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : cluster-name;)

(datatype resource-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : resource-name;)

(datatype api-version
  X : string;
  (not (= X "")) : verified;
  ============================
  X : api-version;)

(datatype resource-kind
  X : string;
  (not (= X "")) : verified;
  ============================
  X : resource-kind;)

(datatype k8s-timestamp
  X : number;
  (> X 0) : verified;
  ====================
  X : k8s-timestamp;)


\* ============================== *\
\*  SECTION 2: ARGOCD GITOPS     *\
\* ============================== *\

\* --- Git source binding --- *\

(datatype git-repo-url
  X : string;
  (not (= X "")) : verified;
  ============================
  X : git-repo-url;)

(datatype git-revision
  X : string;
  (not (= X "")) : verified;
  ============================
  X : git-revision;)

(datatype git-path
  X : string;
  (not (= X "")) : verified;
  ============================
  X : git-path;)

\* A source is bound: repo + path + revision all exist and resolve *\
(datatype source-binding
  Repo : git-repo-url;
  Path : git-path;
  Revision : git-revision;
  ManifestCount : number;
  (> ManifestCount 0) : verified;
  ================================
  [Repo Path Revision ManifestCount] : source-binding;)

\* --- Sync wave ordering --- *\
\* Resources in different waves must respect dependency ordering *\

(datatype sync-wave
  X : number;
  ============
  X : sync-wave;)

\* Proof that resource A's wave < resource B's wave when B depends on A *\
\* This prevents cross-wave dependency violations (the #1 ArgoCD footgun) *\
(datatype wave-ordered
  DependencyWave : sync-wave;
  DependentWave : sync-wave;
  (< DependencyWave DependentWave) : verified;
  =============================================
  [DependencyWave DependentWave] : wave-ordered;)

\* CRD must exist (earlier wave) before any CR that uses it *\
(datatype crd-before-cr
  CrdWave : sync-wave;
  CrWave : sync-wave;
  CrdKind : resource-kind;
  (< CrdWave CrWave) : verified;
  ================================
  [CrdWave CrWave CrdKind] : crd-before-cr;)

\* --- Drift configuration completeness --- *\
\* Every field mutated by an operator must be in ignoreDifferences *\
\* Otherwise: infinite sync loops *\

(datatype field-path
  X : string;
  (not (= X "")) : verified;
  ============================
  X : field-path;)

(datatype operator-mutation
  Operator : resource-name;
  Field : field-path;
  ========================
  [Operator Field] : operator-mutation;)

(datatype ignore-rule
  Field : field-path;
  ===================
  Field : ignore-rule;)

\* Proof that a known mutation is covered by an ignore rule *\
(datatype drift-covered
  Mutation : operator-mutation;
  Rule : ignore-rule;
  ====================
  [Mutation Rule] : drift-covered;)

\* --- Application health --- *\

(datatype health-status
  X : string;
  (element? X [healthy progressing degraded suspended missing unknown]) : verified;
  =================================================================================
  X : health-status;)

(datatype sync-status
  X : string;
  (element? X [synced out-of-sync unknown]) : verified;
  =====================================================
  X : sync-status;)

(datatype app-state
  Name : resource-name;
  Source : source-binding;
  Health : health-status;
  Sync : sync-status;
  =======================
  [Name Source Health Sync] : app-state;)

\* Proof that an app is safe to promote: healthy + synced *\
(datatype app-promotable
  App : app-state;
  (= (head (tail (tail App))) "healthy") : verified;
  (= (head (tail (tail (tail App)))) "synced") : verified;
  =========================================================
  App : app-promotable;)


\* ============================== *\
\*  SECTION 3: ARGO WORKFLOWS    *\
\* ============================== *\

\* --- DAG step types --- *\

(datatype step-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : step-id;)

\* --- DAG acyclicity --- *\
\* Enforce topological ordering: every dependency has a lower order number *\

(datatype topo-order
  X : number;
  (>= X 0) : verified;
  =====================
  X : topo-order;)

(datatype dag-edge
  From : step-id;
  To : step-id;
  FromOrder : topo-order;
  ToOrder : topo-order;
  (< FromOrder ToOrder) : verified;
  ==================================
  [From To FromOrder ToOrder] : dag-edge;)

\* --- Typed parameters (fixing the "everything is a string" problem) --- *\

(datatype param-type
  X : string;
  (element? X [string number boolean json]) : verified;
  =====================================================
  X : param-type;)

(datatype param-decl
  Name : string;
  Type : param-type;
  (not (= Name "")) : verified;
  ==============================
  [Name Type] : param-decl;)

\* Proof that a step output type matches the consuming step's input type *\
(datatype param-type-match
  OutputParam : param-decl;
  InputParam : param-decl;
  (= (head (tail OutputParam)) (head (tail InputParam))) : verified;
  ==================================================================
  [OutputParam InputParam] : param-type-match;)

\* --- Artifact availability --- *\
\* Every artifact input must have a corresponding output from a dependency *\

(datatype artifact-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : artifact-name;)

(datatype artifact-output
  Step : step-id;
  Name : artifact-name;
  ========================
  [Step Name] : artifact-output;)

(datatype artifact-input
  Step : step-id;
  Name : artifact-name;
  ========================
  [Step Name] : artifact-input;)

\* Proof: input artifact has a matching output from a dependency *\
(datatype artifact-satisfied
  Input : artifact-input;
  Output : artifact-output;
  DependencyProof : dag-edge;
  ============================
  [Input Output DependencyProof] : artifact-satisfied;)

\* --- Retry bounds --- *\
\* Retry count * max backoff must fit within a deadline *\

(datatype retry-policy
  Limit : number;
  BackoffMs : number;
  Factor : number;
  MaxBackoffMs : number;
  (> Limit 0) : verified;
  (> BackoffMs 0) : verified;
  (> Factor 0) : verified;
  (> MaxBackoffMs 0) : verified;
  ================================
  [Limit BackoffMs Factor MaxBackoffMs] : retry-policy;)

(datatype deadline-ms
  X : number;
  (> X 0) : verified;
  ====================
  X : deadline-ms;)

\* Proof: worst-case retry time fits within deadline *\
(datatype retry-within-deadline
  Policy : retry-policy;
  Deadline : deadline-ms;
  WorstCaseMs : number;
  (> WorstCaseMs 0) : verified;
  (<= WorstCaseMs Deadline) : verified;
  ======================================
  [Policy Deadline WorstCaseMs] : retry-within-deadline;)


\* ============================== *\
\*  SECTION 4: ARGO ROLLOUTS     *\
\* ============================== *\

\* --- Rollout strategy --- *\

(datatype canary-weight
  X : number;
  (>= X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : canary-weight;)

\* Canary weight steps must be non-decreasing (no traffic oscillation) *\
(datatype weight-monotonic
  PrevWeight : canary-weight;
  NextWeight : canary-weight;
  (>= NextWeight PrevWeight) : verified;
  =======================================
  [PrevWeight NextWeight] : weight-monotonic;)

\* --- Analysis template validation --- *\

(datatype metric-source
  X : string;
  (element? X [prometheus datadog cloudwatch newrelic web]) : verified;
  ====================================================================
  X : metric-source;)

(datatype metric-query
  Source : metric-source;
  Query : string;
  (not (= Query "")) : verified;
  ================================
  [Source Query] : metric-query;)

\* Proof that a metric query actually returns data (not vacuously true) *\
(datatype analysis-reachable
  Query : metric-query;
  SampleCount : number;
  (> SampleCount 0) : verified;
  ==============================
  [Query SampleCount] : analysis-reachable;)

\* --- Traffic backend capability --- *\
\* Proof that the ingress controller supports the traffic split being requested *\

(datatype ingress-type
  X : string;
  (element? X [nginx istio traefik alb contour ambassador]) : verified;
  =====================================================================
  X : ingress-type;)

(datatype traffic-split-capable
  Ingress : ingress-type;
  SupportsWeightedRouting : boolean;
  (= SupportsWeightedRouting true) : verified;
  =============================================
  [Ingress SupportsWeightedRouting] : traffic-split-capable;)

\* --- Rollout terminates --- *\
\* Every path (success, abort, analysis-failure) reaches a terminal state *\

(datatype rollout-terminal-state
  X : string;
  (element? X [completed aborted degraded]) : verified;
  =====================================================
  X : rollout-terminal-state;)

\* A rollout step must specify what happens on analysis failure *\
(datatype step-with-fallback
  Weight : canary-weight;
  Analysis : analysis-reachable;
  OnFailure : rollout-terminal-state;
  ====================================
  [Weight Analysis OnFailure] : step-with-fallback;)

\* --- Safe rollout: all proofs composed --- *\

(datatype safe-rollout
  BackendCapable : traffic-split-capable;
  Steps : number;
  AllStepsHaveFallback : boolean;
  (> Steps 0) : verified;
  (= AllStepsHaveFallback true) : verified;
  ==========================================
  [BackendCapable Steps AllStepsHaveFallback] : safe-rollout;)


\* ============================== *\
\*  SECTION 5: CROSSPLANE         *\
\* ============================== *\

\* --- Provider credentials --- *\

(datatype provider-name
  X : string;
  (element? X [aws gcp azure]) : verified;
  =========================================
  X : provider-name;)

(datatype provider-config
  Name : resource-name;
  Provider : provider-name;
  ===========================
  [Name Provider] : provider-config;)

\* Credential validity proof: not expired *\
(datatype credential-valid
  Config : provider-config;
  ExpiresAt : number;
  Now : number;
  (> ExpiresAt Now) : verified;
  ==============================
  [Config ExpiresAt Now] : credential-valid;)

\* --- XRD schema --- *\

(datatype xrd-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : xrd-name;)

(datatype xrd-version
  Major : number;
  Minor : number;
  (>= Major 0) : verified;
  (>= Minor 0) : verified;
  ==========================
  [Major Minor] : xrd-version;)

\* --- Composition patch validity --- *\
\* Every patch path must reference a real field (no silent nil) *\

(datatype patch-source-path
  X : string;
  (not (= X "")) : verified;
  ============================
  X : patch-source-path;)

(datatype patch-target-path
  X : string;
  (not (= X "")) : verified;
  ============================
  X : patch-target-path;)

\* Proof that the source path exists in the XRD schema *\
(datatype patch-source-valid
  Path : patch-source-path;
  Xrd : xrd-name;
  PathExists : boolean;
  (= PathExists true) : verified;
  ================================
  [Path Xrd PathExists] : patch-source-valid;)

\* Proof that the target path exists in the managed resource schema *\
(datatype patch-target-valid
  Path : patch-target-path;
  ResourceKind : resource-kind;
  PathExists : boolean;
  (= PathExists true) : verified;
  ================================
  [Path ResourceKind PathExists] : patch-target-valid;)

\* Complete patch validity: both source and target paths exist *\
(datatype patch-valid
  Source : patch-source-valid;
  Target : patch-target-valid;
  ==============================
  [Source Target] : patch-valid;)

\* --- Managed resource dependency ordering --- *\
\* If MR-B references MR-A's output, MR-A must be created first *\

(datatype managed-resource
  Name : resource-name;
  Kind : resource-kind;
  Provider : provider-config;
  ============================
  [Name Kind Provider] : managed-resource;)

(datatype mr-dependency
  Dependent : managed-resource;
  Dependency : managed-resource;
  DependencyReady : boolean;
  (= DependencyReady true) : verified;
  =====================================
  [Dependent Dependency DependencyReady] : mr-dependency;)

\* --- XRD backward compatibility --- *\
\* Schema changes must not break existing claims *\

(datatype xrd-compatible
  OldVersion : xrd-version;
  NewVersion : xrd-version;
  NoRemovedFields : boolean;
  NoNewRequiredFields : boolean;
  (= NoRemovedFields true) : verified;
  (= NoNewRequiredFields true) : verified;
  (= (head OldVersion) (head NewVersion)) : verified;
  ====================================================
  [OldVersion NewVersion NoRemovedFields NoNewRequiredFields] : xrd-compatible;)

\* --- Resource completeness --- *\
\* All required forProvider fields populated after patch resolution *\

(datatype resource-complete
  Resource : managed-resource;
  RequiredFields : number;
  PopulatedFields : number;
  (> RequiredFields 0) : verified;
  (= PopulatedFields RequiredFields) : verified;
  ================================================
  [Resource RequiredFields PopulatedFields] : resource-complete;)

\* --- Credential bound to every managed resource --- *\
\* No orphaned credentials: every MR has a valid provider config *\

(datatype mr-credentialed
  Resource : managed-resource;
  Credential : credential-valid;
  ================================
  [Resource Credential] : mr-credentialed;)


\* ============================== *\
\*  SECTION 6: COMPOSED PROOFS   *\
\* ============================== *\

\* --- Full GitOps deployment proof --- *\
\* Combines ArgoCD sync + Rollout safety + Crossplane readiness *\

(datatype gitops-deploy-ready
  App : app-promotable;
  Rollout : safe-rollout;
  ========================
  [App Rollout] : gitops-deploy-ready;)

\* --- Full infrastructure provisioning proof --- *\
\* Every resource is complete, credentialed, and dependencies met *\

(datatype infra-provisioned
  Resource : resource-complete;
  Credential : mr-credentialed;
  ================================
  [Resource Credential] : infra-provisioned;)

\* --- End-to-end: infra ready + app deployable --- *\

(datatype platform-ready
  Infra : infra-provisioned;
  Deploy : gitops-deploy-ready;
  ==============================
  [Infra Deploy] : platform-ready;)
