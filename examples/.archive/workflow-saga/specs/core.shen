\* ============================================================ *\
\* Workflow Saga — Distributed Transaction Proofs               *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Models the Saga pattern for distributed transactions.        *\
\* Each step produces a proof; compensation (rollback) requires *\
\* the corresponding forward-step proof.                        *\
\*                                                              *\
\* Key invariants:                                              *\
\* - Cannot compensate a step that wasn't executed              *\
\* - Compensation must happen in reverse order                  *\
\* - Saga completes only when ALL steps produce proofs          *\
\* - Partial failure triggers compensations for completed steps *\
\* ============================================================ *\

\* --- Step identifiers --- *\

(datatype saga-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : saga-id;)

(datatype step-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : step-name;)

(datatype step-index
  X : number;
  (>= X 0) : verified;
  =====================
  X : step-index;)

\* --- Forward step execution proof --- *\

(datatype step-completed
  Saga : saga-id;
  Step : step-name;
  Index : step-index;
  Timestamp : number;
  (> Timestamp 0) : verified;
  ============================
  [Saga Step Index Timestamp] : step-completed;)

\* --- Compensation proof --- *\
\* Can only compensate a step that was completed *\

(datatype step-compensated
  Completed : step-completed;
  CompensatedAt : number;
  (> CompensatedAt 0) : verified;
  ================================
  [Completed CompensatedAt] : step-compensated;)

\* --- Step ordering proof --- *\
\* Step B can only run after step A if A's index < B's index *\

(datatype step-ordered
  StepA : step-completed;
  IndexB : step-index;
  (< (head (tail (tail StepA))) IndexB) : verified;
  ==================================================
  [StepA IndexB] : step-ordered;)

\* --- Saga states --- *\

\* All steps completed → saga success *\
(datatype saga-completed
  Saga : saga-id;
  StepCount : number;
  CompletedCount : number;
  (> StepCount 0) : verified;
  (= CompletedCount StepCount) : verified;
  =========================================
  [Saga StepCount CompletedCount] : saga-completed;)

\* Partial failure: some steps completed, failure at a specific step *\
(datatype saga-failed
  Saga : saga-id;
  FailedStep : step-name;
  FailedIndex : step-index;
  CompletedBefore : number;
  (>= CompletedBefore 0) : verified;
  (= CompletedBefore FailedIndex) : verified;
  ============================================
  [Saga FailedStep FailedIndex CompletedBefore] : saga-failed;)

\* --- Compensation ordering --- *\
\* Compensations must happen in reverse: index N before index N-1 *\

(datatype compensation-ordered
  Later : step-compensated;
  Earlier : step-compensated;
  (> (head (tail (tail (head Later)))) (head (tail (tail (head Earlier))))) : verified;
  ====================================================================================
  [Later Earlier] : compensation-ordered;)

\* --- Saga fully compensated proof --- *\

(datatype saga-rolled-back
  Failed : saga-failed;
  CompensationCount : number;
  (= CompensationCount (head (tail (tail (tail Failed))))) : verified;
  ====================================================================
  [Failed CompensationCount] : saga-rolled-back;)

\* --- Idempotency key (prevent double-execution) --- *\

(datatype idempotency-key
  X : string;
  (not (= X "")) : verified;
  ============================
  X : idempotency-key;)

(datatype step-idempotent
  Key : idempotency-key;
  Step : step-name;
  Saga : saga-id;
  =================
  [Key Step Saga] : step-idempotent;)

\* --- Timeout proof (step must complete within deadline) --- *\

(datatype deadline
  X : number;
  (> X 0) : verified;
  ====================
  X : deadline;)

(datatype within-deadline
  StartedAt : number;
  CompletedAt : number;
  Deadline : deadline;
  (> StartedAt 0) : verified;
  (> CompletedAt StartedAt) : verified;
  (<= (- CompletedAt StartedAt) Deadline) : verified;
  ====================================================
  [StartedAt CompletedAt Deadline] : within-deadline;)
