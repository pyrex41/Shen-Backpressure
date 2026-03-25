\\ src/medicare-ui-resolve.shen — Generative UI resolution for Medicare
\\
\\ Shen decides WHAT panels to render based on:
\\   1. Pipeline state (idle, searching, fetching, generating, complete)
\\   2. Data shape (what we actually have)
\\   3. LLM layout intent (what the LLM thinks the user wants to see)
\\   4. Conversation context (is this a follow-up? comparison request?)
\\
\\ The LLM generates a layout-intent. Shen validates it against the
\\ grounded data (backpressure) and produces a med-layout for Arrow.js.
\\ The frontend is a DUMB renderer — zero domain knowledge.

\\ =========================================================================
\\ Default layouts for each pipeline phase
\\ =========================================================================

(define resolve-medicare-ui
  \doc "Main resolver: pipeline state → panel list (as JSON-ready nested lists).
        Returns [[kind, [[key, val], ...]], ...] for the frontend."

  \\ Idle: show header + search form + welcome
  "idle" _ _ ->
    [["header"      [["title" "Medicare Plan Finder"]
                     ["subtitle" "Compare Medicare insurance plans and estimated costs in your area"]]]
     ["search-form" [["mode" "full"]]]
     ["chat-input"  [["placeholder" "Or ask a question: \"What Part D plans cover insulin in 33101?\""]
                     ["mode" "initial"]]]
     ["followup"    [["suggestions" "Medicare Advantage plans near me|Part D drug coverage costs|Compare Medigap plans|What does Original Medicare cover?"]]]]

  \\ Searching
  "searching" Data _ ->
    [["header"      [["title" "Medicare Plan Finder"]
                     ["subtitle" "Searching..."]]]
     ["search-form" [["mode" "compact"]]]
     ["progress"    [["phase" "searching"]
                     ["message" "Searching medicare.gov and trusted sources..."]]]]

  \\ Fetching
  "fetching" Data _ ->
    [["header"      [["title" "Medicare Plan Finder"]
                     ["subtitle" "Reading plan details..."]]]
     ["search-form" [["mode" "compact"]]]
     ["progress"    [["phase" "fetching"]
                     ["message" "Fetching plan details and pricing information..."]]]]

  \\ Generating
  "generating" Data _ ->
    [["header"      [["title" "Medicare Plan Finder"]
                     ["subtitle" "Preparing your summary..."]]]
     ["search-form" [["mode" "compact"]]]
     ["progress"    [["phase" "generating"]
                     ["message" "AI is summarizing plan options and costs for you..."]]]]

  \\ Complete — this is where generative UI kicks in
  \\ The LLM layout-intent tells us what panels to show
  "complete" Data LayoutIntent ->
    (resolve-complete-layout Data LayoutIntent))

\\ =========================================================================
\\ Complete layout resolution — LLM-guided + Shen-validated
\\ =========================================================================

(define resolve-complete-layout
  \doc "Build the final layout from data + LLM intent.
        LLM intent is [panels emphasis reasoning].
        We validate each requested panel against available data."
  Data [] ->
    \\ No LLM intent — use default layout
    (default-complete-layout Data)

  Data [layout-intent Panels Emphasis Reasoning] ->
    (let ValidPanels (validate-panels Panels Data)
         Header  [["header" [["title" "Medicare Plan Finder"]
                              ["subtitle" Emphasis]]]]
         Form    [["search-form" [["mode" "compact"]]]]
         Body    (build-panels ValidPanels Data Emphasis)
         Chat    [["chat-input" [["placeholder" "Ask a follow-up question..."]
                                 ["mode" "followup"]]]]
         Sources [["source-list" (extract-source-props Data)]]
         Disc    [["disclaimer" [["text" "Prices shown are estimates. Visit medicare.gov for official pricing."]]]]
      (append Header (append Form (append Body (append Chat (append Sources Disc)))))))

(define default-complete-layout
  \doc "Default layout when no LLM intent is provided."
  Data ->
    [["header"      [["title" "Medicare Plan Finder"]
                     ["subtitle" (get-prop "planLabel" Data)]]]
     ["search-form" [["mode" "compact"]]]
     ["filter-pills" (extract-filter-props Data)]
     ["summary"     (extract-summary-props Data)]
     ["source-list" (extract-source-props Data)]
     ["chat-input"  [["placeholder" "Ask about specific coverage, costs, or compare plans..."]
                     ["mode" "followup"]]]
     ["followup"    [["suggestions" (generate-followups Data)]]]
     ["disclaimer"  [["text" "Prices shown are estimates based on web search. Visit medicare.gov or call 1-800-MEDICARE for exact pricing."]]]])

\\ =========================================================================
\\ Panel validation — backpressure gate
\\ =========================================================================

(define validate-panels
  \doc "Filter panel list to only those we can actually render from data.
        This is the backpressure: LLM can request anything, but Shen
        only allows panels whose data requirements are met."
  [] _ -> []
  [P | Rest] Data ->
    (if (can-render? P Data)
        [P | (validate-panels Rest Data)]
        (validate-panels Rest Data)))

(define can-render?
  \doc "Check if we have sufficient data to render this panel kind."
  "summary"     Data -> (has-prop? "summary" Data)
  "cost-table"  Data -> (has-prop? "summary" Data)
  "plan-cards"  Data -> (has-prop? "summary" Data)
  "comparison"  Data -> (has-prop? "comparisons" Data)
  "detail"      Data -> (has-prop? "summary" Data)
  "chart"       Data -> (has-prop? "summary" Data)
  "source-list" Data -> (has-prop? "sources" Data)
  "filter-pills" Data -> true
  "followup"    Data -> true
  "disclaimer"  Data -> true
  "header"      Data -> true
  "search-form" Data -> true
  "chat-input"  Data -> true
  "progress"    Data -> true
  "error"       Data -> true
  _             _    -> false)

\\ =========================================================================
\\ Panel builders — turn data into panel props
\\ =========================================================================

(define build-panels
  \doc "Build prop lists for each validated panel."
  [] _ _ -> []
  [Kind | Rest] Data Emphasis ->
    [[Kind (panel-props Kind Data Emphasis)] | (build-panels Rest Data Emphasis)])

(define panel-props
  \doc "Extract props for a specific panel kind from result data."
  "summary"     Data _ -> (extract-summary-props Data)
  "cost-table"  Data _ -> (extract-summary-props Data)
  "plan-cards"  Data _ -> (extract-summary-props Data)
  "comparison"  Data _ -> [["content" (get-prop "summary" Data)]]
  "detail"      Data E -> [["content" (get-prop "summary" Data)] ["emphasis" E]]
  "chart"       Data _ -> [["content" (get-prop "summary" Data)]]
  "filter-pills" Data _ -> (extract-filter-props Data)
  \\ Follow-up suggestions now come from the LLM layout intent,
  \\ not hardcoded rules. The frontend reads layout.followups directly.
  "followup"    Data _ -> []
  _             _    _ -> [])

(define extract-summary-props
  \doc "Build props for summary panel."
  Data -> [["content" (get-prop "summary" Data)]
           ["planType" (get-prop "planType" Data)]
           ["zip" (get-prop "zip" Data)]])

(define extract-source-props
  \doc "Build props for source list panel."
  Data -> [["sources" (get-prop "sources" Data)]])

(define extract-filter-props
  \doc "Build filter pill props."
  Data ->
    (let PlanType (get-prop "planType" Data)
         Zip (get-prop "zip" Data)
         Filter (get-prop "filter" Data)
      [["planType" PlanType] ["zip" Zip] ["filter" Filter]]))

\\ =========================================================================
\\ Follow-up generation — now LLM-driven
\\ =========================================================================
\\ Follow-up suggestions are generated by the LLM as part of the layout
\\ intent. The LLM knows what the user asked and what data we have, so
\\ it generates better suggestions than hardcoded rules ever could.
\\
\\ The layout-intent now carries a Followups field:
\\   [layout-intent Panels Emphasis Reasoning Followups]
\\
\\ The frontend reads layout.followups directly. Shen's role is to
\\ validate that the layout intent is well-formed, not to generate
\\ the follow-up content.

\\ =========================================================================
\\ Data access helpers
\\ =========================================================================
\\ Result data comes as a CL alist serialized to a Shen list.
\\ These helpers extract values by key.

(define get-prop
  \doc "Get a property from the result data. Returns empty string if missing."
  Key Data -> (cl-get-prop Key Data))

(define has-prop?
  \doc "Check if a property exists and is non-empty in the data."
  Key Data -> (not (= "" (get-prop Key Data))))
