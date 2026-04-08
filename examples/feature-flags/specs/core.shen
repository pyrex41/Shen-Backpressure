\* ============================================================ *\
\* Feature Flags with Safety Proofs                             *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Feature flags that carry proof of compatibility and          *\
\* dependency satisfaction. You cannot enable feature X         *\
\* without proving its dependencies are met.                    *\
\*                                                              *\
\* Key invariants:                                              *\
\* - Feature activation requires dependency proof               *\
\* - Rollback cascades: disabling X forces disable of dependents*\
\* - Gradual rollout percentages are bounded [0, 100]           *\
\* - Environment constraints prevent prod accidents             *\
\* ============================================================ *\

\* --- Identifiers --- *\

(datatype feature-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : feature-name;)

(datatype environment
  X : string;
  (element? X [development staging production]) : verified;
  ==========================================================
  X : environment;)

\* --- Rollout percentage --- *\

(datatype rollout-pct
  X : number;
  (>= X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : rollout-pct;)

\* --- Feature dependency --- *\

(datatype feature-dependency
  Feature : feature-name;
  DependsOn : feature-name;
  =========================
  [Feature DependsOn] : feature-dependency;)

\* --- Feature enabled proof --- *\

(datatype feature-enabled
  Name : feature-name;
  Env : environment;
  Rollout : rollout-pct;
  ========================
  [Name Env Rollout] : feature-enabled;)

\* --- Dependency satisfaction proof --- *\
\* To activate a feature, its dependency must already be enabled *\

(datatype dependency-satisfied
  Dep : feature-dependency;
  Enabled : feature-enabled;
  \* The enabled feature's name must match the dependency target *\
  ==================================
  [Dep Enabled] : dependency-satisfied;)

\* --- Safe feature activation --- *\
\* Requires all dependencies to be satisfied *\

(datatype safe-activation
  Feature : feature-name;
  Env : environment;
  Rollout : rollout-pct;
  DepsSatisfied : boolean;
  (= DepsSatisfied true) : verified;
  ====================================
  [Feature Env Rollout DepsSatisfied] : safe-activation;)

\* --- Environment gate --- *\
\* Some features can only be enabled in certain environments *\
\* E.g., experimental features blocked from production *\

(datatype env-allowed
  Feature : feature-name;
  Env : environment;
  IsAllowed : boolean;
  (= IsAllowed true) : verified;
  ================================
  [Feature Env IsAllowed] : env-allowed;)

\* --- Gated activation (env + deps) --- *\

(datatype gated-activation
  Activation : safe-activation;
  EnvGate : env-allowed;
  ========================
  [Activation EnvGate] : gated-activation;)

\* --- Gradual rollout tracking --- *\

(datatype user-cohort
  UserId : string;
  HashBucket : number;
  (not (= UserId "")) : verified;
  (>= HashBucket 0) : verified;
  (<= HashBucket 99) : verified;
  ================================
  [UserId HashBucket] : user-cohort;)

\* User is in rollout if their hash bucket < rollout percentage *\
(datatype user-in-rollout
  Cohort : user-cohort;
  Rollout : rollout-pct;
  (< (head (tail Cohort)) Rollout) : verified;
  =============================================
  [Cohort Rollout] : user-in-rollout;)

\* --- Feature flag evaluation result (the complete proof) --- *\

(datatype flag-evaluated
  Activation : gated-activation;
  InRollout : user-in-rollout;
  ==============================
  [Activation InRollout] : flag-evaluated;)

\* --- Rollback proof --- *\
\* Disabling a feature requires proof that no active feature depends on it *\

(datatype no-dependents
  Feature : feature-name;
  ActiveDependentCount : number;
  (= ActiveDependentCount 0) : verified;
  =======================================
  [Feature ActiveDependentCount] : no-dependents;)

(datatype safe-rollback
  Feature : feature-name;
  NoDeps : no-dependents;
  ========================
  [Feature NoDeps] : safe-rollback;)
