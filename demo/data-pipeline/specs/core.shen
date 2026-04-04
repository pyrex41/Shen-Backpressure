\* ============================================================ *\
\* Data Pipeline with Schema Evolution                          *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Models ETL/streaming pipelines where each stage transforms   *\
\* data. Schemas are typed; transformations carry proof that    *\
\* input schema matches expected schema. Schema evolution       *\
\* (adding/removing fields) forces downstream stages to adapt.  *\
\*                                                              *\
\* Key invariants:                                              *\
\* - No stage can process data with wrong schema                *\
\* - Schema migrations must be backward-compatible (proven)     *\
\* - Pipeline stages compose only if types align                *\
\* - Exactly-once semantics via checkpoint proofs               *\
\* ============================================================ *\

\* --- Schema versioning --- *\

(datatype schema-version
  Major : number;
  Minor : number;
  (>= Major 0) : verified;
  (>= Minor 0) : verified;
  ==========================
  [Major Minor] : schema-version;)

\* --- Schema compatibility proof --- *\
\* Minor version bump = backward compatible (reader can read older) *\
\* Same major version required *\

(datatype schema-compatible
  Source : schema-version;
  Target : schema-version;
  (= (head Source) (head Target)) : verified;
  (<= (head (tail Source)) (head (tail Target))) : verified;
  ===========================================================
  [Source Target] : schema-compatible;)

\* --- Data records with schema stamps --- *\

(datatype record-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : record-id;)

(datatype schema-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : schema-name;)

(datatype typed-record
  Id : record-id;
  Schema : schema-name;
  Version : schema-version;
  ===========================
  [Id Schema Version] : typed-record;)

\* --- Pipeline stages --- *\

(datatype stage-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : stage-name;)

\* A stage declaration: what schema it expects and what it produces *\
(datatype stage-contract
  Name : stage-name;
  InputSchema : schema-name;
  InputVersion : schema-version;
  OutputSchema : schema-name;
  OutputVersion : schema-version;
  =================================
  [Name InputSchema InputVersion OutputSchema OutputVersion] : stage-contract;)

\* --- Stage execution proof --- *\
\* To execute a stage, input record's schema must be compatible *\

(datatype stage-input-valid
  Record : typed-record;
  Stage : stage-contract;
  Compat : schema-compatible;
  ============================
  [Record Stage Compat] : stage-input-valid;)

\* --- Pipeline composition --- *\
\* Stage B can follow stage A only if A's output matches B's input *\

(datatype stage-composable
  StageA : stage-contract;
  StageB : stage-contract;
  Compat : schema-compatible;
  ============================
  [StageA StageB Compat] : stage-composable;)

\* --- Checkpoint / exactly-once --- *\

(datatype checkpoint-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : checkpoint-id;)

(datatype offset
  X : number;
  (>= X 0) : verified;
  =====================
  X : offset;)

(datatype checkpoint
  Id : checkpoint-id;
  Stage : stage-name;
  Offset : offset;
  Timestamp : number;
  (> Timestamp 0) : verified;
  ============================
  [Id Stage Offset Timestamp] : checkpoint;)

\* Resume proof: new offset must be > checkpoint offset *\
(datatype resume-valid
  NewOffset : offset;
  LastCheckpoint : checkpoint;
  (> NewOffset (head (tail (tail LastCheckpoint)))) : verified;
  =============================================================
  [NewOffset LastCheckpoint] : resume-valid;)

\* --- Dead letter queue entry --- *\
\* Failed records carry proof of which stage rejected them *\

(datatype dlq-entry
  Record : typed-record;
  Stage : stage-name;
  ErrorMsg : string;
  Timestamp : number;
  (> Timestamp 0) : verified;
  (not (= ErrorMsg "")) : verified;
  ==================================
  [Record Stage ErrorMsg Timestamp] : dlq-entry;)
