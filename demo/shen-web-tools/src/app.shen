\* src/app.shen - Main application logic *\
\* Orchestrates the full research pipeline, all in Shen *\
\* Pipeline: query -> search -> fetch -> ground -> generate -> render *\

\* --- Application state --- *\
\* Shen manages all state transitions; the bridge just stores it *\

(define init-app
  \* Initialize the application in idle state *\
  { --> string }
  -> (let Panel (resolve-ui "idle" [])
      (js.call "bridge.render" ["idle" Panel []])
      "ready"))

\* --- Main pipeline --- *\

(define research
  \* Execute the full research pipeline for a user query *\
  { string --> research-summary }
  RawQuery ->
    (let \* Step 1: Validate and refine the query *\
         QueryText (refine-query RawQuery)
         Query [QueryText 10]

         \* Step 2: Update UI to searching state *\
         _ (js.call "bridge.render" ["searching" (resolve-ui "searching" Query) Query])

         \* Step 3: Execute web search *\
         SearchResult (search-and-collect QueryText 10)

         \* Step 4: Update UI to fetching state *\
         _ (js.call "bridge.render" ["fetching" (resolve-ui "fetching" SearchResult) SearchResult])

         \* Step 5: Fetch top pages *\
         Pages (fetch-top-n SearchResult 5)

         \* Step 6: Ground sources (pair pages with search hits) *\
         Sources (ground-sources Pages (head (tail SearchResult)))

         \* Step 7: Update UI to generating state *\
         _ (js.call "bridge.render" ["generating" (resolve-ui "generating" [Sources Query]) [Sources Query]])

         \* Step 8: Generate AI summary from grounded sources *\
         Summary (summarize-with-sources Query Sources)

         \* Step 9: Build safe render (requires grounded sources — enforced by type) *\
         Panels (assemble-research-view Summary)
         SafeRender [Summary ["root" "column" []]]

         \* Step 10: Update UI to complete state with full results *\
         _ (js.call "bridge.render" ["complete" (resolve-ui "complete" SafeRender) Summary])

      Summary))

\* --- Individual pipeline steps (exposed for incremental use) --- *\

(define do-search
  \* Just the search step *\
  { string --> search-result }
  RawQuery ->
    (let QueryText (refine-query RawQuery)
      (search-and-collect QueryText 10)))

(define do-fetch
  \* Fetch pages from search results *\
  { search-result --> (list fetched-page) }
  Result -> (fetch-top-n Result 5))

(define do-ground
  \* Ground fetched pages against search hits *\
  { (list fetched-page) --> search-result --> (list grounded-source) }
  Pages [Query Hits Ts] -> (ground-sources Pages Hits))

(define do-generate
  \* Generate summary from grounded sources *\
  { search-query --> (list grounded-source) --> research-summary }
  Query Sources -> (summarize-with-sources Query Sources))

\* --- Follow-up research --- *\

(define research-deeper
  \* Given an existing summary, do follow-up research on a specific aspect *\
  { research-summary --> string --> research-summary }
  [OrigQuery OrigSources OrigResponse] Aspect ->
    (let FollowupQuery [(cn (value->string (head OrigQuery)) (cn " " Aspect)) 5]
         SearchResult (search-and-collect (head FollowupQuery) 5)
         Pages (fetch-top-n SearchResult 3)
         AllSources (append OrigSources (ground-sources Pages (head (tail SearchResult))))
         Summary (summarize-with-sources FollowupQuery AllSources)
      Summary))

\* --- Event handlers --- *\
\* These are called by the TypeScript bridge when UI events occur *\

(define on-search-submit
  \* User submitted a search query *\
  { string --> string }
  Query -> (let Summary (research Query)
                Text (extract-summary-text (head (tail (tail Summary))))
             Text))

(define on-source-click
  \* User clicked on a source card *\
  { string --> string }
  Url -> (let Page (web-fetch Url)
              Content (head (tail Page))
           Content))

(define on-refine-click
  \* User wants to refine the current research *\
  { research-summary --> string --> string }
  Summary Aspect -> (let Deeper (research-deeper Summary Aspect)
                         Text (extract-summary-text (head (tail (tail Deeper))))
                      Text))

(define on-compare-click
  \* User wants to compare sources *\
  { (list grounded-source) --> string }
  Sources -> (let Prompt (make-comparison-prompt Sources)
                  Response (ai-generate Prompt)
                  [_ Text _] Response
               Text))
