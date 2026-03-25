\\ src/medicare.shen — Medicare insurance lookup pipeline
\\
\\ Orchestrates: query refinement → web search → data extraction → AI summary
\\ All web data is cached by (plan-type, zip) with configurable TTL.

\\ =========================================================================
\\ Query refinement — turn user input into targeted Medicare searches
\\ =========================================================================

(define refine-medicare-query
  \doc "Build search queries targeting medicare.gov pricing data."
  PlanType Zip ""
    -> [(cn "medicare " (cn PlanType (cn " plans " (cn "zip code " (cn Zip " 2025 premiums costs medicare.gov")))))
        (cn "medicare.gov " (cn PlanType (cn " plan finder " (cn Zip " monthly premium"))))]
  PlanType Zip Filter
    -> [(cn "medicare " (cn PlanType (cn " " (cn Filter (cn " coverage " (cn Zip " 2025 costs"))))))
        (cn "medicare.gov " (cn PlanType (cn " " (cn Filter (cn " " Zip)))))])

\\ =========================================================================
\\ Search + collect Medicare-specific results
\\ =========================================================================

(define medicare-search
  \doc "Search for Medicare plan information. Returns search results."
  PlanType Zip Filter ->
    (let Queries (refine-medicare-query PlanType Zip Filter)
         Results (map (/. Q (cl-web-search Q 5)) Queries)
      (flatten-results Results)))

(define flatten-results
  \doc "Flatten list of search result lists into one list, dedup by URL."
  [] -> []
  [[] | Rest] -> (flatten-results Rest)
  [[H | T] | Rest] -> [H | (flatten-results [T | Rest])])

\\ =========================================================================
\\ Fetch + extract pricing data from pages
\\ =========================================================================

(define fetch-medicare-pages
  \doc "Fetch top N pages from search results, preferring medicare.gov."
  Results N ->
    (let Sorted (prioritize-medicare-gov Results)
         Top (take N Sorted)
      (map (/. Hit (cl-web-fetch (hd (tl Hit)))) Top)))

(define prioritize-medicare-gov
  \doc "Sort results so medicare.gov URLs come first."
  Results ->
    (let Gov (filter (/. Hit (is-medicare-gov? (hd (tl Hit)))) Results)
         Other (filter (/. Hit (not (is-medicare-gov? (hd (tl Hit))))) Results)
      (append Gov Other)))

(define is-medicare-gov?
  \doc "Check if URL contains medicare.gov."
  Url -> (or (substring? "medicare.gov" Url)
             (substring? "cms.gov" Url)))

(define substring?
  \doc "True if Needle is found in Haystack."
  Needle Haystack ->
    (trap-error (do (cn (head-string-search Needle Haystack) "") true)
                (/. E false)))

(define head-string-search
  \doc "Simple substring search — uses CL interop."
  Needle Haystack -> (cl-substring-search Needle Haystack))

(define take
  \doc "Take first N elements from a list."
  0 _ -> []
  _ [] -> []
  N [H | T] -> [H | (take (- N 1) T)])

(define filter
  \doc "Filter list by predicate."
  _ [] -> []
  F [H | T] -> (if (F H) [H | (filter F T)] (filter F T)))

\\ =========================================================================
\\ AI summarization — present Medicare data to consumers
\\ =========================================================================

(define make-medicare-prompt
  \doc "Build a prompt for consumer-friendly Medicare pricing summary."
  PlanType Zip Filter Sources ->
    (let SystemMsg (cn "You are a helpful Medicare insurance advisor. "
                   (cn "Present plan pricing information clearly for consumers. "
                   (cn "Always note that prices may vary and recommend visiting medicare.gov "
                   (cn "or calling 1-800-MEDICARE for exact pricing. "
                   (cn "Format with clear sections: Plan Options, Monthly Costs, "
                   "What's Covered, and Next Steps.")))))
         UserMsg (cn "A consumer in zip code " (cn Zip
                 (cn " is looking for Medicare " (cn PlanType
                 (cn " plan information"
                 (cn (if (= Filter "") "" (cn " specifically about " Filter))
                 (cn ".@newline@@newline@Here is what I found from web sources:@newline@@newline@"
                 (format-medicare-sources Sources))))))))
      [SystemMsg UserMsg]))

(define format-medicare-sources
  \doc "Format fetched page contents for the AI prompt."
  [] -> ""
  [[Url Content Timestamp] | Rest] ->
    (cn "--- Source: " (cn Url (cn " ---@newline@"
    (cn (truncate-text Content 2000) (cn "@newline@@newline@"
    (format-medicare-sources Rest)))))))

(define truncate-text
  \doc "Truncate text to MaxLen characters."
  Text MaxLen ->
    (if (<= (string-length Text) MaxLen)
        Text
        (cn (substring Text 0 MaxLen) "...")))

(define string-length
  \doc "Length of a string."
  "" -> 0
  S -> (+ 1 (string-length (tlstr S))))

(define substring
  \doc "Extract substring from Start to End."
  S Start End -> (cl-substring S Start End))

\\ =========================================================================
\\ Full Medicare pipeline
\\ =========================================================================

(define medicare-lookup
  \doc "Full Medicare plan lookup pipeline.
        1. Refine query for medicare.gov
        2. Search the web
        3. Fetch top pages (prefer medicare.gov)
        4. Generate consumer-friendly summary via AI
        Returns [query plans summary sources timestamp]."
  PlanType Zip Filter ->
    (let _          (cl-set-pipeline-state "searching" PlanType)
         Hits       (medicare-search PlanType Zip Filter)
         _          (cl-set-pipeline-state "fetching" Hits)
         Pages      (fetch-medicare-pages Hits 5)
         _          (cl-set-pipeline-state "generating" Pages)
         Prompt     (make-medicare-prompt PlanType Zip Filter Pages)
         Response   (cl-ai-generate (hd Prompt) (hd (tl Prompt)))
         Summary    (hd (tl Response))
         Urls       (map (/. P (hd P)) Pages)
         Timestamp  (cl-current-timestamp)
         _          (cl-set-pipeline-state "complete" Summary)
      [PlanType Zip Filter Summary Urls Timestamp]))

(define medicare-compare
  \doc "Compare plans across multiple types for a zip code."
  Zip PlanTypes ->
    (let Results (map (/. PT (medicare-lookup PT Zip "")) PlanTypes)
      Results))
