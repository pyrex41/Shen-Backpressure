\\ specs/medicare.shen — Medicare insurance domain types + generative UI
\\
\\ Types for: plan lookup, caching, AND the UI panel tree.
\\ The LLM generates a layout spec; Shen validates it against these types
\\ before the frontend renders anything. This is the backpressure gate:
\\ you cannot render data you haven't fetched and grounded.

\\ =========================================================================
\\ Domain types
\\ =========================================================================

(datatype medicare-plan-type
  ___________________________
  "original" : medicare-plan-type;

  ___________________________
  "advantage" : medicare-plan-type;

  ___________________________
  "part-d" : medicare-plan-type;

  ___________________________
  "supplement" : medicare-plan-type;

  ___________________________
  "part-a" : medicare-plan-type;

  ___________________________
  "part-b" : medicare-plan-type;)

(datatype zip-code
  X : string;
  (string? X) : verified;
  ___________________________
  X : zip-code;)

(datatype medicare-query
  PlanType : medicare-plan-type;
  Zip : zip-code;
  ___________________________
  [medicare-query PlanType Zip] : medicare-query;

  \\ Query with optional drug/service filter
  PlanType : medicare-plan-type;
  Zip : zip-code;
  Filter : string;
  ___________________________
  [medicare-query PlanType Zip Filter] : medicare-query;)

(datatype plan-premium
  Name : string;
  Monthly : string;
  Deductible : string;
  ___________________________
  [plan-premium Name Monthly Deductible] : plan-premium;)

(datatype plan-detail
  Name : string;
  Carrier : string;
  PlanType : medicare-plan-type;
  Premium : plan-premium;
  Rating : string;
  Url : string;
  ___________________________
  [plan-detail Name Carrier PlanType Premium Rating Url] : plan-detail;)

(datatype medicare-result
  Query : medicare-query;
  Plans : (list plan-detail);
  Summary : string;
  Sources : (list string);
  Timestamp : number;
  ___________________________
  [medicare-result Query Plans Summary Sources Timestamp] : medicare-result;

  \\ Cached result wraps a medicare-result with TTL
  Result : medicare-result;
  CachedAt : number;
  TTL : number;
  ___________________________
  [cached-medicare Result CachedAt TTL] : medicare-result;)

(datatype price-comparison
  Plans : (list plan-detail);
  CheapestMonthly : plan-detail;
  CheapestDeductible : plan-detail;
  AveragePremium : string;
  ___________________________
  [price-comparison Plans CheapestMonthly CheapestDeductible AveragePremium]
    : price-comparison;)

\\ =========================================================================
\\ Generative UI panel types
\\ =========================================================================
\\ The LLM generates a layout-spec (JSON). Shen parses it into these types.
\\ If the LLM hallucinates a panel that references data we don't have,
\\ the type checker rejects it. This is the safety gate.

\\ Panel kinds — what the frontend knows how to render
(datatype panel-kind
  ___________________________
  "header" : panel-kind;          \\ title + subtitle

  ___________________________
  "search-form" : panel-kind;     \\ zip + plan type + filter inputs

  ___________________________
  "chat-input" : panel-kind;      \\ conversational follow-up input

  ___________________________
  "progress" : panel-kind;        \\ pipeline stage indicator

  ___________________________
  "summary" : panel-kind;         \\ AI-generated markdown summary

  ___________________________
  "cost-table" : panel-kind;      \\ tabular cost breakdown

  ___________________________
  "plan-cards" : panel-kind;      \\ grid of plan cards

  ___________________________
  "comparison" : panel-kind;      \\ side-by-side plan comparison

  ___________________________
  "source-list" : panel-kind;     \\ attributed sources

  ___________________________
  "disclaimer" : panel-kind;      \\ legal/accuracy disclaimer

  ___________________________
  "detail" : panel-kind;          \\ deep-dive on one plan/topic

  ___________________________
  "chart" : panel-kind;           \\ cost visualization

  ___________________________
  "followup" : panel-kind;        \\ suggested follow-up questions

  ___________________________
  "filter-pills" : panel-kind;    \\ active filter tags

  ___________________________
  "error" : panel-kind;)          \\ error message display

\\ A single UI panel: kind + props (key-value pairs as nested lists)
(datatype med-panel
  Kind : panel-kind;
  Props : (list (list string));
  ___________________________
  [Kind Props] : med-panel;)

\\ A layout is a list of panels — the order is the render order
(datatype med-layout
  Panels : (list med-panel);
  ___________________________
  Panels : med-layout;)

\\ A grounded layout: can only be constructed from a medicare-result
\\ This prevents rendering hallucinated data
(datatype grounded-layout
  Result : medicare-result;
  Layout : med-layout;
  ___________________________
  [Result Layout] : grounded-layout;)

\\ Conversation turn — tracks user follow-ups
(datatype conv-turn
  Role : string;
  Content : string;
  ___________________________
  [Role Content] : conv-turn;)

(datatype conv-history
  Turns : (list conv-turn);
  ___________________________
  Turns : conv-history;)

\\ LLM layout intent — what the LLM wants to show
\\ Must be validated by Shen before rendering
(datatype layout-intent
  Panels : (list string);       \\ panel kind names
  Emphasis : string;            \\ what to highlight
  Reasoning : string;           \\ why this layout
  ___________________________
  [layout-intent Panels Emphasis Reasoning] : layout-intent;)
