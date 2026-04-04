\* ============================================================ *\
\* Consensus & Quorum Protocol                                  *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Models voting / approval workflows where actions require     *\
\* quorum. Cannot act without sufficient votes. Cannot          *\
\* double-vote. Vote tallying is proven correct.                *\
\*                                                              *\
\* Applicable to: governance, approvals, distributed consensus, *\
\* code review gates, budget approvals, change management.      *\
\* ============================================================ *\

\* --- Identifiers --- *\

(datatype proposal-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : proposal-id;)

(datatype voter-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : voter-id;)

\* --- Voting configuration --- *\

(datatype quorum-size
  Total : number;
  Required : number;
  (> Total 0) : verified;
  (> Required 0) : verified;
  (<= Required Total) : verified;
  ================================
  [Total Required] : quorum-size;)

\* --- Vote types --- *\

(datatype vote-choice
  X : string;
  (element? X [approve reject abstain]) : verified;
  ==================================================
  X : vote-choice;)

\* A cast vote: voter + choice + timestamp *\
(datatype cast-vote
  Proposal : proposal-id;
  Voter : voter-id;
  Choice : vote-choice;
  Timestamp : number;
  (> Timestamp 0) : verified;
  ============================
  [Proposal Voter Choice Timestamp] : cast-vote;)

\* --- Eligibility proof (voter is authorized to vote) --- *\

(datatype voter-eligible
  Voter : voter-id;
  Proposal : proposal-id;
  IsEligible : boolean;
  (= IsEligible true) : verified;
  ================================
  [Voter Proposal IsEligible] : voter-eligible;)

\* --- Uniqueness proof (voter hasn't already voted) --- *\

(datatype vote-unique
  Voter : voter-id;
  Proposal : proposal-id;
  PriorVoteCount : number;
  (= PriorVoteCount 0) : verified;
  ==================================
  [Voter Proposal PriorVoteCount] : vote-unique;)

\* --- Valid vote: eligible + unique --- *\

(datatype valid-vote
  Vote : cast-vote;
  Eligible : voter-eligible;
  Unique : vote-unique;
  ========================
  [Vote Eligible Unique] : valid-vote;)

\* --- Tally types --- *\

(datatype vote-tally
  Proposal : proposal-id;
  Approvals : number;
  Rejections : number;
  Abstentions : number;
  (>= Approvals 0) : verified;
  (>= Rejections 0) : verified;
  (>= Abstentions 0) : verified;
  ================================
  [Proposal Approvals Rejections Abstentions] : vote-tally;)

\* --- Quorum reached proof --- *\
\* Total votes cast >= required for quorum *\

(datatype quorum-reached
  Tally : vote-tally;
  Quorum : quorum-size;
  TotalVotes : number;
  (= TotalVotes (+ (+ (head (tail Tally)) (head (tail (tail Tally)))) (head (tail (tail (tail Tally)))))) : verified;
  (>= TotalVotes (head (tail Quorum))) : verified;
  ==================================================
  [Tally Quorum TotalVotes] : quorum-reached;)

\* --- Decision types --- *\

\* Proposal approved: quorum reached + majority approvals *\
(datatype proposal-approved
  QuorumProof : quorum-reached;
  (> (head (tail (head QuorumProof))) (head (tail (tail (head QuorumProof))))) : verified;
  ========================================================================================
  QuorumProof : proposal-approved;)

\* Proposal rejected: quorum reached + majority rejections *\
(datatype proposal-rejected
  QuorumProof : quorum-reached;
  (>= (head (tail (tail (head QuorumProof)))) (head (tail (head QuorumProof)))) : verified;
  =========================================================================================
  QuorumProof : proposal-rejected;)

\* --- Execution proof: can only execute approved proposals --- *\

(datatype execution-authorized
  Approval : proposal-approved;
  ExecutedBy : voter-id;
  Timestamp : number;
  (> Timestamp 0) : verified;
  ============================
  [Approval ExecutedBy Timestamp] : execution-authorized;)

\* --- Veto power (special override) --- *\

(datatype veto-holder
  Voter : voter-id;
  IsVetoHolder : boolean;
  (= IsVetoHolder true) : verified;
  ==================================
  [Voter IsVetoHolder] : veto-holder;)

(datatype veto-cast
  Holder : veto-holder;
  Proposal : proposal-id;
  Reason : string;
  (not (= Reason "")) : verified;
  ================================
  [Holder Proposal Reason] : veto-cast;)
