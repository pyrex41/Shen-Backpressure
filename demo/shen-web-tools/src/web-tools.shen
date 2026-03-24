\* src/web-tools.shen - Web tool definitions in Shen *\
\* These functions define the LOGIC of web operations. *\
\* Actual I/O is delegated to the TypeScript bridge via foreign calls. *\

\* --- Foreign function declarations --- *\
\* These bind to the TypeScript bridge at runtime *\

(define web-search
  \* Execute a web search query, return list of search hits *\
  { search-query --> (list search-hit) }
  Query -> (js.call "bridge.webSearch" Query))

(define web-fetch
  \* Fetch a URL and return the page content *\
  { fetch-request --> fetched-page }
  Req -> (js.call "bridge.webFetch" Req))

(define ai-generate
  \* Send a prompt to the AI model and return the response *\
  { ai-prompt --> ai-response }
  Prompt -> (js.call "bridge.aiGenerate" Prompt))

(define current-timestamp
  \* Get the current timestamp *\
  { --> timestamp }
  -> (js.call "bridge.now"))

\* --- Web tool combinators --- *\
\* Pure Shen functions that compose web tools into pipelines *\

(define search-and-collect
  \* Search for a query and return structured results *\
  { query-text --> number --> search-result }
  Text MaxResults ->
    (let Query [Text MaxResults]
         Hits  (web-search Query)
         Ts    (current-timestamp)
      [Query Hits Ts]))

(define fetch-top-n
  \* Fetch the top N pages from search results *\
  { search-result --> number --> (list fetched-page) }
  [Query Hits Ts] N ->
    (map (/. Hit (web-fetch (head (tail Hit)))) (take N Hits)))

(define take
  \* Take the first N elements from a list *\
  { number --> (list A) --> (list A) }
  0 _ -> []
  _ [] -> []
  N [X | Xs] -> [X | (take (- N 1) Xs)])

\* --- Source grounding --- *\
\* Pairs fetched pages with their original search hits *\
\* This is the key safety operation: ensures AI has real sources *\

(define ground-sources
  \* Create grounded sources by pairing pages with hits *\
  { (list fetched-page) --> (list search-hit) --> (list grounded-source) }
  [] _ -> []
  _ [] -> []
  [Page | Pages] [Hit | Hits] ->
    (if (= (head Page) (head (tail Hit)))
        [[Page Hit] | (ground-sources Pages Hits)]
        (ground-sources Pages [Hit | Hits])))

\* --- Query refinement --- *\
\* Shen logic for expanding/refining user queries *\

(define refine-query
  \* Add context to a bare query for better search results *\
  { string --> string }
  Q -> (cn Q " overview explanation summary"))

(define extract-key-terms
  \* Pull key terms from a query for follow-up searches *\
  { string --> (list string) }
  Q -> (js.call "bridge.extractTerms" Q))

(define build-followup-queries
  \* Given initial results, build follow-up queries for deeper research *\
  { search-result --> (list search-query) }
  [Query Hits Ts] ->
    (map (/. Hit [(cn "details about " (head Hit)) 5]) (take 3 Hits)))
