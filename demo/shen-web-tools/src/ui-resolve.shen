\* src/ui-resolve.shen - UI layout resolution in Shen *\
\* Shen's Prolog engine resolves WHAT to render based on pipeline state *\
\* Arrow.js handles the HOW of rendering *\

\* --- UI resolution rules (Prolog-style in Shen) --- *\
\* Given the current pipeline state, resolve which UI components to show *\

(define resolve-ui
  \* Main UI resolver: maps pipeline state to a UI panel tree *\
  { string --> A --> ui-panel }

  \* Idle state: show search bar only *\
  "idle" _ ->
    ["root" "column" ["search-bar" "welcome-msg"]]

  \* Searching: show search bar + loading indicator *\
  "searching" Query ->
    ["root" "column" ["search-bar" "loading-search"]]

  \* Fetching: show search bar + search results + fetch progress *\
  "fetching" Result ->
    (let HitCount (length (head (tail Result)))
         Children (append ["search-bar" "result-header"]
                          (append (make-hit-ids HitCount)
                                  ["fetch-progress"]))
      ["root" "column" Children])

  \* Generating: show search bar + sources + generation progress *\
  "generating" [Sources Query] ->
    (let SourceCount (length Sources)
         Children (append ["search-bar" "sources-header"]
                          (append (make-source-ids SourceCount)
                                  ["gen-progress"]))
      ["root" "column" Children])

  \* Complete: show full research results *\
  "complete" Render ->
    ["root" "column" ["search-bar" "summary-panel" "sources-panel" "actions-panel"]])

\* --- Component-specific resolvers --- *\

(define resolve-search-bar
  \* Resolve search bar component with current query *\
  { string --> ui-search-bar }
  CurrentQuery -> ["Search any topic..." CurrentQuery])

(define resolve-source-cards
  \* Turn grounded sources into renderable source cards *\
  { (list grounded-source) --> (list ui-source-card) }
  Sources -> (resolve-source-cards-h Sources 1))

(define resolve-source-cards-h
  { (list grounded-source) --> number --> (list ui-source-card) }
  [] _ -> []
  [[Page Hit] | Rest] N ->
    (let Title (head Hit)
         Url (head (tail Hit))
         Snip (head (tail (tail Hit)))
         Relevance (compute-relevance N (length Rest))
      [[Title Url Snip Relevance] | (resolve-source-cards-h Rest (+ N 1))]))

(define compute-relevance
  \* Simple relevance score: higher rank = more relevant *\
  { number --> number --> number }
  Rank Total -> (/ 1.0 (+ 1.0 (* 0.5 (- Rank 1)))))

\* --- Layout helpers --- *\

(define make-hit-ids
  \* Generate component IDs for N search hits *\
  { number --> (list string) }
  0 -> []
  N -> [(cn "hit-" (value->string N)) | (make-hit-ids (- N 1))])

(define make-source-ids
  \* Generate component IDs for N source cards *\
  { number --> (list string) }
  0 -> []
  N -> [(cn "source-" (value->string N)) | (make-source-ids (- N 1))])

\* --- Generative UI decisions --- *\
\* Shen decides which UI components to create based on content analysis *\

(define should-show-comparison
  \* Show comparison view if we have 2+ sources with differing content *\
  { (list grounded-source) --> boolean }
  [] -> false
  [_] -> false
  _ -> true)

(define should-show-followup
  \* Show follow-up suggestions if the query could be refined *\
  { search-query --> search-result --> boolean }
  _ [_ Hits _] -> (> (length Hits) 3))

(define select-layout-mode
  \* Choose between compact and expanded layout based on content *\
  { research-summary --> string }
  [Query Sources Response] ->
    (if (> (length Sources) 5)
        "expanded"
        (if (> (length (extract-summary-text Response)) 1000)
            "expanded"
            "compact")))

\* --- Panel assembly --- *\
\* Shen assembles the final UI description that Arrow will render *\

(define assemble-research-view
  \* Build the complete UI description for a finished research task *\
  { research-summary --> (list ui-panel) }
  Summary ->
    (let Mode (select-layout-mode Summary)
         [Query Sources Response] Summary
         MainPanel ["summary" "article" ["summary-text"]]
         SourcesPanel ["sources" "grid" (make-source-ids (length Sources))]
         ActionsPanel ["actions" "row"
           (append ["new-search" "refine"]
                   (if (should-show-comparison Sources)
                       ["compare"]
                       []))]
      [MainPanel SourcesPanel ActionsPanel]))
