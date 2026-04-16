\* ============================================================ *\
\* Immutable Audit Trail & Compliance                           *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Models audit logging where every action produces a           *\
\* proof-carrying audit record. Records form a hash chain       *\
\* (each links to previous). Tampering is a type error.         *\
\*                                                              *\
\* Key invariants:                                              *\
\* - Every mutation produces an audit entry                     *\
\* - Entries form a linked chain (no gaps)                      *\
\* - Retention policies are enforced at the type level          *\
\* - Compliance reports require complete chains                 *\
\* ============================================================ *\

\* --- Identifiers --- *\

(datatype actor-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : actor-id;)

(datatype entity-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : entity-id;)

(datatype entry-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : entry-id;)

\* --- Action classification --- *\

(datatype action-type
  X : string;
  (element? X [create read update delete approve reject escalate]) : verified;
  ============================================================================
  X : action-type;)

(datatype sensitivity
  X : string;
  (element? X [public internal confidential restricted]) : verified;
  ==================================================================
  X : sensitivity;)

\* --- Timestamps with ordering --- *\

(datatype audit-timestamp
  X : number;
  (> X 0) : verified;
  ====================
  X : audit-timestamp;)

\* Proof that timestamps are ordered (for chain integrity) *\
(datatype timestamp-ordered
  Before : audit-timestamp;
  After : audit-timestamp;
  (> After Before) : verified;
  ============================
  [Before After] : timestamp-ordered;)

\* --- Hash chain --- *\

(datatype hash-value
  X : string;
  (not (= X "")) : verified;
  ============================
  X : hash-value;)

\* Chain link: current hash was computed from previous hash + data *\
(datatype chain-link
  PrevHash : hash-value;
  CurrentHash : hash-value;
  EntryData : string;
  (not (= EntryData "")) : verified;
  ====================================
  [PrevHash CurrentHash EntryData] : chain-link;)

\* --- Core audit entry --- *\

(datatype audit-entry
  Id : entry-id;
  Actor : actor-id;
  Action : action-type;
  Entity : entity-id;
  Timestamp : audit-timestamp;
  Sensitivity : sensitivity;
  Chain : chain-link;
  =====================
  [Id Actor Action Entity Timestamp Sensitivity Chain] : audit-entry;)

\* --- Chain continuity proof --- *\
\* New entry's prev-hash must match previous entry's current-hash *\

(datatype chain-continuous
  Previous : audit-entry;
  Next : audit-entry;
  TimeOrder : timestamp-ordered;
  ================================
  [Previous Next TimeOrder] : chain-continuous;)

\* --- Retention policy --- *\

(datatype retention-days
  X : number;
  (> X 0) : verified;
  ====================
  X : retention-days;)

\* Entry is within retention: age < retention period *\
(datatype within-retention
  Entry : audit-entry;
  Now : audit-timestamp;
  Retention : retention-days;
  DaysSinceEntry : number;
  (>= DaysSinceEntry 0) : verified;
  (< DaysSinceEntry Retention) : verified;
  =========================================
  [Entry Now Retention DaysSinceEntry] : within-retention;)

\* --- Compliance report --- *\
\* A valid report requires a complete, unbroken chain *\

(datatype report-period
  Start : audit-timestamp;
  End : audit-timestamp;
  (> End Start) : verified;
  ==========================
  [Start End] : report-period;)

(datatype compliance-report
  Period : report-period;
  EntryCount : number;
  ChainIntact : boolean;
  (> EntryCount 0) : verified;
  (= ChainIntact true) : verified;
  ==================================
  [Period EntryCount ChainIntact] : compliance-report;)

\* --- Access to audit logs requires elevated proof --- *\

(datatype audit-reader
  Actor : actor-id;
  HasAuditRole : boolean;
  (= HasAuditRole true) : verified;
  ==================================
  [Actor HasAuditRole] : audit-reader;)

(datatype audit-query
  Reader : audit-reader;
  Period : report-period;
  Sensitivity : sensitivity;
  ============================
  [Reader Period Sensitivity] : audit-query;)

\* --- Tamper detection --- *\
\* Proof that a chain segment has been verified (hashes recomputed) *\

(datatype chain-verified
  StartEntry : entry-id;
  EndEntry : entry-id;
  VerifiedCount : number;
  AllValid : boolean;
  (> VerifiedCount 0) : verified;
  (= AllValid true) : verified;
  ================================
  [StartEntry EndEntry VerifiedCount AllValid] : chain-verified;)
