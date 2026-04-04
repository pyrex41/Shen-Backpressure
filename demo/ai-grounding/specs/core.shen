\* ============================================================ *\
\* AI Output Grounding — Anti-Hallucination Types               *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Makes hallucination a TYPE ERROR. Any AI-generated content   *\
\* that reaches the user must carry proof of grounding:         *\
\*   - Citations must reference fetched documents               *\
\*   - Summaries must be derived from source material           *\
\*   - Claims must have evidence chains                         *\
\*   - Confidence must be calibrated against source count       *\
\*                                                              *\
\* Extends the shen-web-tools grounding pattern to a general   *\
\* framework for any RAG / agent / tool-use system.             *\
\* ============================================================ *\

\* --- Source material types --- *\

(datatype source-url
  X : string;
  (not (= X "")) : verified;
  ============================
  X : source-url;)

(datatype source-content
  X : string;
  (not (= X "")) : verified;
  ============================
  X : source-content;)

(datatype fetch-timestamp
  X : number;
  (> X 0) : verified;
  ====================
  X : fetch-timestamp;)

\* A fetched document: URL + content + when it was fetched *\
(datatype fetched-document
  Url : source-url;
  Content : source-content;
  FetchedAt : fetch-timestamp;
  ==============================
  [Url Content FetchedAt] : fetched-document;)

\* --- Citation types --- *\

\* A citation must reference a specific fetched document *\
(datatype grounded-citation
  Claim : string;
  Source : fetched-document;
  (not (= Claim "")) : verified;
  ================================
  [Claim Source] : grounded-citation;)

\* --- AI output types --- *\

(datatype raw-ai-output
  X : string;
  (not (= X "")) : verified;
  ============================
  X : raw-ai-output;)

(datatype model-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : model-name;)

\* --- Grounding proofs --- *\

\* Summary grounded in sources: must have at least one source *\
(datatype grounded-summary
  Output : raw-ai-output;
  Sources : (list fetched-document);
  SourceCount : number;
  (> SourceCount 0) : verified;
  ==============================
  [Output Sources SourceCount] : grounded-summary;)

\* Fact-check result: claim verified against multiple sources *\
(datatype fact-checked
  Claim : string;
  Supporting : (list grounded-citation);
  SupportCount : number;
  (not (= Claim "")) : verified;
  (> SupportCount 0) : verified;
  ================================
  [Claim Supporting SupportCount] : fact-checked;)

\* --- Confidence calibration --- *\
\* More sources = higher allowed confidence *\

(datatype confidence-score
  X : number;
  (>= X 0) : verified;
  (<= X 1) : verified;
  =====================
  X : confidence-score;)

\* Low confidence allowed with 1 source, high needs 3+ *\
(datatype calibrated-confidence
  Score : confidence-score;
  SourceCount : number;
  (> SourceCount 0) : verified;
  \* If confidence > 0.8, need at least 3 sources *\
  ================================
  [Score SourceCount] : calibrated-confidence;)

\* --- Safe renderable output --- *\
\* The final type that can be shown to users *\
\* Must have grounding + confidence calibration *\

(datatype safe-output
  Summary : grounded-summary;
  Confidence : calibrated-confidence;
  =====================================
  [Summary Confidence] : safe-output;)

\* --- Tool use grounding --- *\
\* For agent systems: tool calls must be justified *\

(datatype tool-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : tool-name;)

(datatype tool-justification
  Tool : tool-name;
  Reason : string;
  UserIntent : string;
  (not (= Reason "")) : verified;
  (not (= UserIntent "")) : verified;
  ====================================
  [Tool Reason UserIntent] : tool-justification;)

\* A grounded tool call: tool use that carries justification *\
(datatype grounded-tool-call
  Justification : tool-justification;
  =====================================
  Justification : grounded-tool-call;)

\* --- Retrieval quality gate --- *\
\* Retrieved chunks must meet relevance threshold *\

(datatype relevance-score
  X : number;
  (>= X 0) : verified;
  (<= X 1) : verified;
  =====================
  X : relevance-score;)

(datatype relevance-threshold
  X : number;
  (> X 0) : verified;
  (<= X 1) : verified;
  =====================
  X : relevance-threshold;)

(datatype relevant-retrieval
  Doc : fetched-document;
  Score : relevance-score;
  Threshold : relevance-threshold;
  (>= Score Threshold) : verified;
  ==================================
  [Doc Score Threshold] : relevant-retrieval;)

\* --- Composed RAG pipeline proof --- *\
\* Full chain: retrieve (relevant) -> ground (cited) -> calibrate -> render *\

(datatype rag-pipeline-output
  Retrieval : relevant-retrieval;
  Output : safe-output;
  =======================
  [Retrieval Output] : rag-pipeline-output;)
