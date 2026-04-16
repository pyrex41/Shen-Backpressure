\* ============================================================ *\
\* Circuit Breaker & Resilience Patterns                        *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Models circuit breaker states as a proof-carrying state      *\
\* machine. Transitions require proof of conditions:            *\
\*   CLOSED -> OPEN requires failure threshold proof            *\
\*   OPEN -> HALF_OPEN requires cooldown elapsed proof          *\
\*   HALF_OPEN -> CLOSED requires success proof                 *\
\*   HALF_OPEN -> OPEN requires probe failure proof             *\
\*                                                              *\
\* Invalid transitions are compile-time errors, not runtime.    *\
\* ============================================================ *\

\* --- Wrapper types --- *\

(datatype service-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : service-name;)

(datatype timestamp
  X : number;
  (> X 0) : verified;
  ====================
  X : timestamp;)

\* --- Failure tracking --- *\

(datatype failure-count
  X : number;
  (>= X 0) : verified;
  =====================
  X : failure-count;)

(datatype failure-threshold
  X : number;
  (> X 0) : verified;
  ====================
  X : failure-threshold;)

\* --- Cooldown period in milliseconds --- *\

(datatype cooldown-ms
  X : number;
  (> X 0) : verified;
  ====================
  X : cooldown-ms;)

\* --- Circuit breaker states --- *\
\* Each state carries its own proof of how it was entered *\

(datatype closed-circuit
  Service : service-name;
  Failures : failure-count;
  ==========================
  [Service Failures] : closed-circuit;)

\* Threshold breach proof: failures >= threshold *\
(datatype threshold-breached
  Failures : failure-count;
  Threshold : failure-threshold;
  (>= Failures Threshold) : verified;
  ====================================
  [Failures Threshold] : threshold-breached;)

\* OPEN state requires proof that threshold was breached *\
(datatype open-circuit
  Service : service-name;
  Breach : threshold-breached;
  OpenedAt : timestamp;
  ========================
  [Service Breach OpenedAt] : open-circuit;)

\* Cooldown elapsed proof: now - openedAt >= cooldown *\
(datatype cooldown-elapsed
  Now : timestamp;
  OpenedAt : timestamp;
  Cooldown : cooldown-ms;
  (>= (- Now OpenedAt) Cooldown) : verified;
  ==========================================
  [Now OpenedAt Cooldown] : cooldown-elapsed;)

\* HALF_OPEN state requires proof that cooldown elapsed *\
(datatype half-open-circuit
  Service : service-name;
  CooldownProof : cooldown-elapsed;
  ==================================
  [Service CooldownProof] : half-open-circuit;)

\* --- Transition proofs (the state machine edges) --- *\

\* Probe success: half-open -> closed *\
(datatype probe-success
  HalfOpen : half-open-circuit;
  ==============================
  HalfOpen : circuit-recovery;)

\* Probe failure: half-open -> open (re-trip) *\
(datatype probe-failure
  HalfOpen : half-open-circuit;
  Now : timestamp;
  ==============================
  [HalfOpen Now] : circuit-retrip;)

\* --- Rate limiting (composable with circuit breaker) --- *\

(datatype rate-limit
  MaxRequests : number;
  WindowMs : number;
  (> MaxRequests 0) : verified;
  (> WindowMs 0) : verified;
  ============================
  [MaxRequests WindowMs] : rate-limit;)

(datatype request-count
  Count : number;
  (>= Count 0) : verified;
  =========================
  Count : request-count;)

\* Proof that we're under the rate limit *\
(datatype rate-allowed
  Count : request-count;
  Limit : rate-limit;
  (< Count (head Limit)) : verified;
  ===================================
  [Count Limit] : rate-allowed;)

\* --- Bulkhead (concurrency limiter) --- *\

(datatype max-concurrent
  X : number;
  (> X 0) : verified;
  ====================
  X : max-concurrent;)

(datatype active-count
  X : number;
  (>= X 0) : verified;
  =====================
  X : active-count;)

(datatype bulkhead-permit
  Active : active-count;
  Max : max-concurrent;
  (< Active Max) : verified;
  ===========================
  [Active Max] : bulkhead-permit;)

\* --- Composite resilience: request must pass ALL guards --- *\

(datatype resilience-cleared
  CircuitOk : closed-circuit;
  RateOk : rate-allowed;
  BulkheadOk : bulkhead-permit;
  ================================
  [CircuitOk RateOk BulkheadOk] : resilience-cleared;)
