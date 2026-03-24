\* specs/core.shen - Formal type specifications for Shen Web Tools demo *\
\* Domain: AI research assistant with web search, fetch, and generation *\
\* All application logic is in Shen; TypeScript is only the I/O bridge *\

\* --- Basic value types --- *\

(datatype query-text
  X : string;
  (> (length X) 0) : verified;
  ==============
  X : query-text;)

(datatype url
  X : string;
  (> (length X) 8) : verified;
  ==============
  X : url;)

(datatype snippet
  X : string;
  ==============
  X : snippet;)

(datatype timestamp
  X : number;
  (> X 0) : verified;
  ==============
  X : timestamp;)

\* --- Search domain --- *\

(datatype search-query
  Text : query-text;
  MaxResults : number;
  (>= MaxResults 1) : verified;
  (<= MaxResults 20) : verified;
  ================================
  [Text MaxResults] : search-query;)

(datatype search-hit
  Title : string;
  Url : url;
  Snippet : snippet;
  ========================
  [Title Url Snippet] : search-hit;)

(datatype search-result
  Query : search-query;
  Hits : (list search-hit);
  Ts : timestamp;
  ===========================
  [Query Hits Ts] : search-result;)

\* --- Web fetch domain --- *\

(datatype fetch-request
  Url : url;
  ==============
  Url : fetch-request;)

(datatype fetched-page
  Url : url;
  Content : string;
  Ts : timestamp;
  =========================
  [Url Content Ts] : fetched-page;)

\* --- AI generation domain --- *\

(datatype ai-prompt
  System : string;
  User : string;
  (> (length System) 0) : verified;
  (> (length User) 0) : verified;
  ==================================
  [System User] : ai-prompt;)

(datatype ai-response
  Prompt : ai-prompt;
  Text : string;
  Ts : timestamp;
  =========================
  [Prompt Text Ts] : ai-response;)

\* --- KEY INVARIANT: summary requires both search results AND fetched content --- *\
\* You cannot generate a summary without first searching and fetching *\
\* This is the core backpressure rule: AI generation requires grounded sources *\

(datatype grounded-source
  Page : fetched-page;
  Hit : search-hit;
  (= (head Page) (head (tail Hit))) : verified;
  =============================================
  [Page Hit] : grounded-source;)

(datatype research-summary
  Query : search-query;
  Sources : (list grounded-source);
  Response : ai-response;
  ===========================
  [Query Sources Response] : research-summary;)

\* --- UI component types --- *\

(datatype ui-component-type
  X : string;
  ==============
  X : ui-component-type;)

(datatype ui-text-block
  Content : string;
  CssClass : string;
  ====================
  [Content CssClass] : ui-text-block;)

(datatype ui-source-card
  Title : string;
  Url : url;
  Snippet : snippet;
  Relevance : number;
  (>= Relevance 0) : verified;
  (<= Relevance 1) : verified;
  ================================
  [Title Url Snippet Relevance] : ui-source-card;)

(datatype ui-search-bar
  Placeholder : string;
  Value : string;
  =====================
  [Placeholder Value] : ui-search-bar;)

\* --- UI layout types --- *\

(datatype ui-panel
  Id : string;
  Kind : ui-component-type;
  Children : (list string);
  ============================
  [Id Kind Children] : ui-panel;)

\* --- Safe render: can only render a summary that has grounded sources --- *\
\* The UI cannot display ungrounded AI-generated content *\

(datatype safe-render
  Summary : research-summary;
  Panel : ui-panel;
  ========================
  [Summary Panel] : safe-render;)

\* --- Pipeline state machine --- *\
\* Enforces the order: query -> search -> fetch -> generate -> render *\

(datatype pipeline-idle
  X : string;
  (= X "idle") : verified;
  ========================
  X : pipeline-idle;)

(datatype pipeline-searching
  Query : search-query;
  =======================
  Query : pipeline-searching;)

(datatype pipeline-fetching
  Result : search-result;
  ========================
  Result : pipeline-fetching;)

(datatype pipeline-generating
  Sources : (list grounded-source);
  Query : search-query;
  ==================================
  [Sources Query] : pipeline-generating;)

(datatype pipeline-complete
  Render : safe-render;
  ======================
  Render : pipeline-complete;)
