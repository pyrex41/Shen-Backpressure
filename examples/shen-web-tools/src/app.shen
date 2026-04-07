\* src/app.shen - Main application logic *\
\* Orchestrates the full research pipeline, all in Shen *\
\* Pipeline: query -> search -> fetch -> ground -> generate -> render *\
\* Runs on SBCL via Shen-CL; CL bridge handles I/O *\

\* --- Main pipeline --- *\

(define research
  \* Execute the full research pipeline for a user query *\
  { string --> research-summary }
  RawQuery ->
    (let \* Step 1: Validate and refine the query *\
         QueryText (refine-query RawQuery)
         Query [QueryText 10]

         \* Step 2: Execute web search (CL bridge does I/O) *\
         SearchResult (search-and-collect QueryText 10)

         \* Step 3: Fetch top pages (CL bridge does I/O) *\
         Pages (fetch-top-n SearchResult 5)

         \* Step 4: Ground sources (pair pages with search hits) *\
         \* KEY INVARIANT: URL of fetched page must match URL of search hit *\
         Sources (ground-sources Pages (head (tail SearchResult)))

         \* Step 5: Generate AI summary from grounded sources *\
         Summary (summarize-with-sources Query Sources)

         \* Step 6: Build UI panel description *\
         Panels (assemble-research-view Summary)

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
    (let FollowupQuery [(cn (str (head OrigQuery)) (cn " " Aspect)) 5]
         SearchResult (search-and-collect (head FollowupQuery) 5)
         Pages (fetch-top-n SearchResult 3)
         AllSources (append OrigSources (ground-sources Pages (head (tail SearchResult))))
         Summary (summarize-with-sources FollowupQuery AllSources)
      Summary))

\* --- Compare sources --- *\

(define compare-sources
  \* Generate a comparison of multiple grounded sources *\
  { (list grounded-source) --> ai-response }
  Sources ->
    (let Prompt (make-comparison-prompt Sources)
      (ai-generate Prompt)))
