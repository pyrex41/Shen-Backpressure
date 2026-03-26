;;;; backend/medicare.lisp — Medicare plan lookup with generative UI
;;;;
;;;; Architecture:
;;;;   1. User sends natural language query OR structured form input
;;;;   2. LLM interprets intent → extracts plan-type, zip, filter, action
;;;;   3. CL pipeline: search + fetch + cache
;;;;   4. LLM generates layout-intent: which panels, what emphasis, why
;;;;   5. Shen validates layout against grounded data (backpressure)
;;;;   6. Frontend renders the panel tree — zero domain knowledge
;;;;
;;;; The LLM is in the loop TWICE:
;;;;   a) Query interpretation: "what Part D plans cover insulin near 33101?"
;;;;      → { planType: "part-d", zip: "33101", filter: "insulin" }
;;;;   b) Layout generation: given the result data, what panels to show?
;;;;      → { panels: ["summary", "cost-table", "source-list"], emphasis: "insulin coverage" }

(in-package :shen-web-tools)

;; =========================================================================
;; Cache layer
;; =========================================================================

(defvar *medicare-cache* (make-hash-table :test 'equal)
  "Cache: (plan-type zip filter) → (result . timestamp)")

(defvar *medicare-cache-lock* (bt:make-lock "medicare-cache"))

(defvar *medicare-cache-ttl* 3600
  "Cache TTL in seconds. Default 1 hour.")

(defvar *conversation-cache* (make-hash-table :test 'equal)
  "Conversation cache: session-id → list of (role . message)")

(defvar *conversation-lock* (bt:make-lock "conversation"))

(defun cache-key (plan-type zip filter)
  "Build a cache key from Medicare query parameters."
  (format nil "~A:~A:~A" (string-downcase plan-type) zip (string-downcase filter)))

(defun cache-get (plan-type zip filter)
  "Look up a cached Medicare result. Returns NIL if miss or expired."
  (bt:with-lock-held (*medicare-cache-lock*)
    (let* ((key (cache-key plan-type zip filter))
           (entry (gethash key *medicare-cache*)))
      (when entry
        (let ((result (car entry))
              (cached-at (cdr entry)))
          (if (< (- (get-universal-time) cached-at) *medicare-cache-ttl*)
              (progn
                (format t "Cache HIT: ~A~%" key)
                (demo-log "cache" (format nil "HIT ~A" key))
                result)
              (progn
                (format t "Cache EXPIRED: ~A~%" key)
                (demo-log "cache" (format nil "EXPIRED ~A" key))
                (remhash key *medicare-cache*)
                nil)))))))

(defun cache-put (plan-type zip filter result)
  "Store a Medicare result in the cache."
  (bt:with-lock-held (*medicare-cache-lock*)
    (let ((key (cache-key plan-type zip filter)))
      (setf (gethash key *medicare-cache*)
            (cons result (get-universal-time)))
      (format t "Cache STORE: ~A~%" key)))
  result)

(defun cache-clear ()
  "Clear the entire Medicare cache."
  (bt:with-lock-held (*medicare-cache-lock*)
    (clrhash *medicare-cache*)
    (format t "Medicare cache cleared.~%")))

(defun cache-stats ()
  "Return cache statistics as an alist."
  (bt:with-lock-held (*medicare-cache-lock*)
    (let ((count (hash-table-count *medicare-cache*))
          (entries nil))
      (maphash (lambda (key entry)
                 (push `((:key . ,key)
                         (:cached-at . ,(cdr entry))
                         (:age-seconds . ,(- (get-universal-time) (cdr entry))))
                       entries))
               *medicare-cache*)
      `((:count . ,count)
        (:ttl-seconds . ,*medicare-cache-ttl*)
        (:entries . ,(nreverse entries))))))

;; =========================================================================
;; Conversation state
;; =========================================================================

(defun conversation-get (session-id)
  "Get conversation history for a session."
  (bt:with-lock-held (*conversation-lock*)
    (gethash session-id *conversation-cache*)))

(defun conversation-push (session-id role content)
  "Add a turn to the conversation."
  (bt:with-lock-held (*conversation-lock*)
    (let ((history (gethash session-id *conversation-cache*)))
      (setf (gethash session-id *conversation-cache*)
            (append history (list (cons role content)))))))

(defun conversation-clear (session-id)
  "Clear conversation for a session."
  (bt:with-lock-held (*conversation-lock*)
    (remhash session-id *conversation-cache*)))

;; =========================================================================
;; Plan type labels
;; =========================================================================

(defvar *plan-type-labels*
  '(("advantage"  . "Medicare Advantage (Part C)")
    ("part-d"     . "Medicare Part D (Prescription Drug)")
    ("supplement" . "Medicare Supplement (Medigap)")
    ("original"   . "Original Medicare (Parts A & B)")
    ("part-a"     . "Medicare Part A (Hospital)")
    ("part-b"     . "Medicare Part B (Medical)"))
  "Human-readable labels for plan types.")

(defun plan-type-label (plan-type)
  "Get human-readable label for a plan type."
  (or (cdr (assoc plan-type *plan-type-labels* :test #'string-equal))
      (format nil "Medicare ~A" plan-type)))

;; =========================================================================
;; LLM-in-the-loop #1: Query interpretation
;; =========================================================================
;; Given natural language, extract structured Medicare query parameters.

(defun interpret-query-prompt (user-message conversation-history)
  "Build the prompt for LLM query interpretation."
  (let ((system "You are a Medicare query parser. Given a user's natural language question about Medicare insurance, extract structured query parameters.

Respond with ONLY a JSON object (no markdown, no explanation):
{
  \"planType\": \"advantage\" | \"part-d\" | \"supplement\" | \"original\" | \"part-a\" | \"part-b\",
  \"zip\": \"XXXXX\" or \"\" if not mentioned,
  \"filter\": \"specific drug, service, or coverage area\" or \"\",
  \"action\": \"lookup\" | \"compare\" | \"detail\" | \"followup\",
  \"needsZip\": true if zip code is required but not provided
}

Plan type mapping:
- Medicare Advantage, MA, Part C → \"advantage\"
- Drug coverage, prescription, Part D, formulary → \"part-d\"
- Medigap, supplemental, supplement → \"supplement\"
- Original Medicare, traditional → \"original\"
- Hospital, Part A → \"part-a\"
- Doctor, medical, Part B → \"part-b\"
- If asking to compare multiple types → action: \"compare\"
- If asking for details on specific aspect → action: \"detail\"

If the user's message is a follow-up to a previous conversation, use context to fill in missing parameters.")
        (context (when conversation-history
                   (format nil "~%~%Previous conversation:~%~{~A: ~A~%~}"
                           (loop for (role . msg) in (last conversation-history 6)
                                 collect role collect msg)))))
    (values system (concatenate 'string user-message (or context "")))))

(defun interpret-user-query (user-message session-id)
  "Use LLM to interpret a natural language Medicare query.
   Returns an alist with planType, zip, filter, action."
  (let ((history (conversation-get session-id)))
    (multiple-value-bind (system-msg user-msg)
        (interpret-query-prompt user-message history)
      (let* ((ai-result (ai-generate system-msg user-msg))
             (response-text (second ai-result))
             ;; Parse JSON response
             (parsed (handler-case
                         (cl-json:decode-json-from-string
                          (extract-json-from-response response-text))
                       (error () nil))))
        (if parsed
            ;; Return extracted parameters
            `((:plan-type . ,(or (cdr (assoc :plan-type parsed))
                                 (cdr (assoc :plan--type parsed))
                                 "advantage"))
              (:zip . ,(or (cdr (assoc :zip parsed)) ""))
              (:filter . ,(or (cdr (assoc :filter parsed)) ""))
              (:action . ,(or (cdr (assoc :action parsed)) "lookup"))
              (:needs-zip . ,(cdr (assoc :needs-zip parsed)))
              (:raw-query . ,user-message))
            ;; Fallback: treat as filter text
            `((:plan-type . "advantage")
              (:zip . "")
              (:filter . ,user-message)
              (:action . "lookup")
              (:needs-zip . t)
              (:raw-query . ,user-message)))))))

(defun extract-json-from-response (text)
  "Extract the first balanced JSON object from an LLM response.
   Handles nested braces and string literals correctly."
  (let ((start (position #\{ text)))
    (if (null start)
        "{}"
        (let ((depth 0)
              (in-string nil)
              (escape nil))
          (loop for i from start below (length text)
                for c = (char text i)
                do (cond
                     (escape (setf escape nil))
                     ((char= c #\\) (when in-string (setf escape t)))
                     ((char= c #\") (setf in-string (not in-string)))
                     ((not in-string)
                      (cond
                        ((char= c #\{) (incf depth))
                        ((char= c #\})
                         (decf depth)
                         (when (zerop depth)
                           (return-from extract-json-from-response
                             (subseq text start (1+ i))))))))
                finally (return "{}"))))))

;; =========================================================================
;; LLM-in-the-loop #2: Layout generation with lazy panel constraints
;; =========================================================================
;; Given result data + conversation context, decide what UI panels to show.
;; Inspired by Anthropic's read_me pattern: instead of dumping all panel
;; docs into context, we load only the constraints relevant to the data.
;;
;; The LLM also generates follow-up suggestions — it knows what the user
;; asked and what would be useful next, so it does better than hardcoded rules.

;; ---- Panel constraint specs (lazy-loaded per data shape) ----

(defvar *panel-constraints*
  '(("summary"
     :requires (:summary)
     :description "Main AI-generated summary in markdown"
     :best-for "Any query — always include as the primary content panel"
     :pairs-with ("filter-pills" "source-list"))

    ("cost-table"
     :requires (:summary)
     :description "Tabular cost breakdown with premiums, deductibles, OOP max"
     :best-for "Pricing questions, 'how much does X cost', premium comparisons"
     :needs "Summary should contain numeric pricing data (dollar amounts)"
     :pairs-with ("filter-pills" "chart"))

    ("plan-cards"
     :requires (:summary)
     :description "Grid of individual plan cards for browsing"
     :best-for "Broad searches where user wants to browse multiple options"
     :needs "Summary should describe 3+ distinct plans"
     :pairs-with ("filter-pills" "summary"))

    ("comparison"
     :requires (:comparisons)
     :description "Side-by-side comparison of multiple plan types"
     :best-for "When user asks to compare plan types or says 'vs', 'difference'"
     :needs "Multiple plan type results (requires /compare pipeline)"
     :pairs-with ("source-list" "disclaimer"))

    ("detail"
     :requires (:summary)
     :description "Deep-dive panel focused on a specific aspect"
     :best-for "Follow-up questions about specific coverage, formulary, network"
     :needs "User asked about a specific topic within a plan type"
     :pairs-with ("source-list" "followup"))

    ("chart"
     :requires (:summary)
     :description "Cost range visualization bar chart"
     :best-for "When showing price ranges across plans or tiers"
     :needs "Summary contains numeric ranges or multiple price points"
     :pairs-with ("cost-table" "summary"))

    ("source-list"
     :requires (:sources)
     :description "Attributed source links with medicare.gov badges"
     :best-for "Always include — grounds the response in real data"
     :pairs-with ("summary" "disclaimer"))

    ("followup"
     :requires ()
     :description "Suggested follow-up question chips"
     :best-for "After any result — helps user explore further"
     :note "LLM generates the actual suggestions in the followups field"
     :pairs-with ("summary" "disclaimer"))

    ("filter-pills"
     :requires ()
     :description "Active filter tags showing plan type, zip, filter"
     :best-for "When filters are active — helps user see what's applied"
     :pairs-with ("summary" "cost-table"))

    ("disclaimer"
     :requires ()
     :description "Legal/accuracy disclaimer with medicare.gov link"
     :best-for "Always include as the last panel"
     :pairs-with ("source-list")))
  "Panel constraint specifications. Each panel has:
   - Data requirements (:requires)
   - Description and best-use guidance
   - Pairing suggestions
   Loaded lazily into the LLM prompt based on available data.")

(defun available-panels-for-data (result)
  "Return panel constraints for panels whose data requirements are met.
   This is the lazy loading: only show the LLM panels it can actually use."
  (loop for spec in *panel-constraints*
        for name = (first spec)
        for requires = (getf (rest spec) :requires)
        when (every (lambda (key)
                      (let ((val (cdr (assoc key result))))
                        (and val (if (stringp val)
                                     (> (length val) 0)
                                     t))))
                    requires)
          collect spec))

(defun format-panel-constraints (available-panels)
  "Format available panel constraints as text for the LLM prompt."
  (with-output-to-string (s)
    (format s "## Available Panels~%~%")
    (format s "These panels are AVAILABLE based on the data we have. ")
    (format s "Only panels listed here can be used — Shen will reject others.~%~%")
    (loop for spec in available-panels
          for name = (first spec)
          for plist = (rest spec)
          do (format s "### ~A~%" name)
             (format s "~A~%" (getf plist :description))
             (format s "Best for: ~A~%" (getf plist :best-for))
             (when (getf plist :needs)
               (format s "Needs: ~A~%" (getf plist :needs)))
             (when (getf plist :note)
               (format s "Note: ~A~%" (getf plist :note)))
             (format s "Pairs well with: ~{~A~^, ~}~%~%"
                     (getf plist :pairs-with)))))

(defun generate-layout-intent (result conversation-history user-query)
  "Use LLM to decide which UI panels to render, what to emphasize,
   and what follow-up questions to suggest.

   The LLM sees only panels whose data requirements are met (lazy loading).
   It also generates follow-up suggestions — replacing hardcoded Shen rules."
  (demo-log "layout" "LLM generating layout intent..."
            (format nil "Available panels for this data shape"))
  (let* ((available (available-panels-for-data result))
         (panel-docs (format-panel-constraints available))
         (system (format nil "You are a UI layout planner for a Medicare insurance tool.
Given the data available and the user's question, decide:
1. Which panels to show (from the available list only)
2. What to emphasize (short phrase for the header subtitle)
3. 3-4 follow-up questions the user might want to ask next

~A
## Rules
- Always include \"source-list\" and \"disclaimer\"
- Include \"filter-pills\" when filters are active
- Use \"cost-table\" for pricing questions, \"detail\" for specific coverage questions
- Use \"comparison\" ONLY when comparison data is available
- Follow-up questions should be specific to what was asked and what data shows
- Follow-ups should help the user dig deeper or compare alternatives

Respond with ONLY a JSON object:
{
  \"panels\": [\"panel-kind\", ...],
  \"emphasis\": \"Short phrase for header subtitle\",
  \"reasoning\": \"Why this layout\",
  \"followups\": [\"Question 1?\", \"Question 2?\", \"Question 3?\", \"Question 4?\"]
}" panel-docs))
         (user-msg (format nil "User asked: ~A~%~%Data:~%- Plan type: ~A (~A)~%- Zip: ~A~%- Filter: ~A~%- Summary: ~D chars~%- Sources: ~D~%- Has comparisons: ~A~%~A"
                           user-query
                           (cdr (assoc :plan-type result))
                           (or (cdr (assoc :plan-label result)) "")
                           (cdr (assoc :zip result))
                           (cdr (assoc :filter result))
                           (length (or (cdr (assoc :summary result)) ""))
                           (length (cdr (assoc :sources result)))
                           (if (cdr (assoc :comparisons result)) "yes" "no")
                           (if conversation-history
                               (format nil "~%Conversation turn ~D. Previous topics: ~{~A~^, ~}"
                                       (length conversation-history)
                                       (loop for (role . msg) in (last conversation-history 4)
                                             when (string= role "user")
                                               collect (subseq msg 0 (min 60 (length msg)))))
                               "")))
         (ai-result (ai-generate system user-msg))
         (response-text (second ai-result))
         (parsed (handler-case
                     (cl-json:decode-json-from-string
                      (extract-json-from-response response-text))
                   (error () nil)))
         ;; Validate panels against available set
         (available-names (mapcar #'first available))
         (validated-panels (when (cdr (assoc :panels parsed))
                             (remove-if-not (lambda (p) (member p available-names :test #'string=))
                                            (cdr (assoc :panels parsed))))))
    (let ((layout-result
            (if parsed
                `((:panels . ,(or validated-panels '("summary" "source-list" "disclaimer")))
                  (:emphasis . ,(or (cdr (assoc :emphasis parsed)) (cdr (assoc :plan-label result))))
                  (:reasoning . ,(or (cdr (assoc :reasoning parsed)) "default layout"))
                  (:followups . ,(or (cdr (assoc :followups parsed))
                                     (default-followups result))))
                ;; Fallback layout
                `((:panels . ("summary" "source-list" "followup" "disclaimer"))
                  (:emphasis . ,(or (cdr (assoc :plan-label result)) "Medicare Plans"))
                  (:reasoning . "default layout (LLM parse failed)")
                  (:followups . ,(default-followups result))))))
      (demo-log "layout"
                (format nil "Panels: ~{~A~^, ~}" (cdr (assoc :panels layout-result)))
                (cdr (assoc :reasoning layout-result)))
      layout-result)))

(defun default-followups (result)
  "Fallback follow-up suggestions when LLM doesn't generate them."
  (let ((plan-type (or (cdr (assoc :plan-type result)) "")))
    (cond
      ((string-equal plan-type "part-d")
       '("What drugs are in the formulary?"
         "What's the coverage gap (donut hole)?"
         "Compare Part D plans by premium"
         "Does this cover insulin?"))
      ((string-equal plan-type "advantage")
       '("What's the out-of-pocket maximum?"
         "Do these plans include dental and vision?"
         "Compare with Original Medicare"
         "Which plan has the lowest premium?"))
      ((string-equal plan-type "supplement")
       '("What's the difference between Plan F and Plan G?"
         "Which Medigap plan has the lowest premium?"
         "What does Medigap NOT cover?"
         "Compare with Medicare Advantage"))
      (t '("What are the 2025 premiums?"
           "Compare plan types"
           "What's covered under this plan?"
           "Show me the cheapest options")))))

;; =========================================================================
;; Medicare search queries
;; =========================================================================

(defun build-medicare-queries (plan-type zip filter)
  "Build search queries targeting Medicare pricing data."
  (let ((base-queries
          (list
           (format nil "medicare ~A plans zip code ~A 2025 premiums costs site:medicare.gov"
                   plan-type zip)
           (format nil "medicare.gov plan finder ~A ~A monthly premium deductible"
                   plan-type zip)
           (format nil "medicare ~A plans ~A costs coverage 2025"
                   plan-type zip))))
    (if (and filter (> (length filter) 0))
        (cons (format nil "medicare ~A ~A coverage ~A 2025 cost"
                      plan-type filter zip)
              base-queries)
        base-queries)))

;; =========================================================================
;; Medicare AI summary prompt
;; =========================================================================

(defun build-medicare-system-prompt ()
  "System prompt for consumer-friendly Medicare advice."
  "You are a helpful Medicare insurance advisor helping consumers understand their plan options and costs. Present information clearly and simply.

IMPORTANT GUIDELINES:
- Present pricing in easy-to-read format with monthly premiums, deductibles, and out-of-pocket maximums
- Always note that prices vary by location, health status, and specific plan
- Recommend visiting medicare.gov or calling 1-800-MEDICARE (1-800-633-4227) for exact pricing
- Mention the Medicare Plan Finder tool at medicare.gov/plan-compare
- Note the Annual Enrollment Period (Oct 15 - Dec 7) and Special Enrollment Periods
- Do NOT provide specific medical advice
- Format response with clear sections using ## headers
- Use bullet points for readability
- If data is limited, be honest and point to authoritative sources")

(defun build-medicare-user-prompt (plan-type zip filter sources)
  "Build the user message with fetched source data."
  (format nil "A consumer in zip code ~A is looking for ~A information~A.~%~%~
               Provide a clear, consumer-friendly summary with costs and coverage details.~%~%~
               Web sources:~%~%~A"
          zip
          (plan-type-label plan-type)
          (if (and filter (> (length filter) 0))
              (format nil ", specifically about ~A" filter)
              "")
          (format-medicare-sources sources)))

(defun build-followup-prompt (user-query previous-summary plan-type zip)
  "Build prompt for a follow-up question using existing data."
  (format nil "The consumer previously asked about ~A plans in zip ~A.~%~%~
               Previous summary:~%~A~%~%~
               Their follow-up question: ~A~%~%~
               Answer their follow-up question using the information above. ~
               If the existing information doesn't cover their question, say so ~
               and suggest they check medicare.gov."
          (plan-type-label plan-type) zip previous-summary user-query))

(defun format-medicare-sources (sources)
  "Format source data for the AI prompt."
  (with-output-to-string (s)
    (loop for source in sources
          for i from 1
          do (let ((url (first source))
                   (content (second source)))
               (format s "--- Source ~D: ~A ---~%~A~%~%"
                       i url
                       (subseq content 0 (min 2000 (length content))))))))

;; =========================================================================
;; Medicare pipeline
;; =========================================================================

(defun medicare-lookup-cl (plan-type zip filter)
  "Full Medicare plan lookup. Checks cache first, then searches + generates."
  (let ((cached (cache-get plan-type zip filter)))
    (when cached
      (return-from medicare-lookup-cl cached)))

  (set-pipeline-state "searching" `((:plan-type . ,plan-type) (:zip . ,zip)))

  (let* ((queries (build-medicare-queries plan-type zip filter))
         (all-hits (loop for q in queries
                         append (handler-case (web-search q 5)
                                  (error () nil))))
         (seen (make-hash-table :test 'equal))
         (unique-hits (loop for hit in all-hits
                            unless (gethash (second hit) seen)
                              collect (progn (setf (gethash (second hit) seen) t) hit)))
         (sorted-hits (sort (copy-list unique-hits)
                            (lambda (a b)
                              (let ((a-gov (or (search "medicare.gov" (second a))
                                               (search "cms.gov" (second a))))
                                    (b-gov (or (search "medicare.gov" (second b))
                                               (search "cms.gov" (second b)))))
                                (and a-gov (not b-gov)))))))

    (set-pipeline-state "fetching" `((:hits . ,(length sorted-hits))
                                     (:plan-type . ,plan-type)))
    (let* ((top-hits (subseq sorted-hits 0 (min 5 (length sorted-hits))))
           (pages (loop for hit in top-hits
                        collect (handler-case (web-fetch (second hit))
                                  (error () (list (second hit) "" 0))))))

      (set-pipeline-state "generating" `((:sources . ,(length pages))
                                         (:plan-type . ,plan-type)))
      (let* ((system-msg (build-medicare-system-prompt))
             (user-msg (build-medicare-user-prompt plan-type zip filter pages))
             (ai-result (ai-generate system-msg user-msg))
             (summary (second ai-result))
             (result `((:plan-type . ,plan-type)
                       (:plan-label . ,(plan-type-label plan-type))
                       (:zip . ,zip)
                       (:filter . ,(or filter ""))
                       (:summary . ,summary)
                       (:sources . ,(loop for page in pages
                                          for hit in top-hits
                                          collect `((:url . ,(first page))
                                                    (:title . ,(first hit))
                                                    (:snippet . ,(third hit))
                                                    (:is-medicare-gov
                                                     . ,(if (or (search "medicare.gov" (second hit))
                                                                (search "cms.gov" (second hit)))
                                                            t nil)))))
                       (:timestamp . ,(get-universal-time))
                       (:cached . nil))))

        (set-pipeline-state "complete" result)
        (cache-put plan-type zip filter result)
        result))))

;; =========================================================================
;; Conversational pipeline — the main entry point
;; =========================================================================

(defun medicare-converse (user-message session-id &key zip plan-type)
  "Main conversational entry point. Handles both initial queries and follow-ups.
   Returns the full response including layout intent for generative UI.

   Flow:
   1. LLM interprets user intent (or use provided structured params)
   2. Fetch data if needed (or use cache)
   3. LLM generates UI layout intent
   4. Return data + layout for Shen validation + Arrow.js rendering"

  (demo-log "pipeline" (format nil "User: ~A" user-message)
            (format nil "session=~A zip=~A planType=~A" session-id (or zip "") (or plan-type "")))

  ;; Record user message in conversation
  (conversation-push session-id "user" user-message)

  ;; Step 1: Interpret the query
  (let* ((intent (if (and zip plan-type)
                     ;; Structured input — skip LLM interpretation
                     `((:plan-type . ,plan-type)
                       (:zip . ,zip)
                       (:filter . ,user-message)
                       (:action . "lookup")
                       (:needs-zip . nil)
                       (:raw-query . ,user-message))
                     ;; Natural language — LLM interprets
                     (interpret-user-query user-message session-id)))
         (i-plan-type (cdr (assoc :plan-type intent)))
         (i-zip (cdr (assoc :zip intent)))
         (i-filter (cdr (assoc :filter intent)))
         (i-action (cdr (assoc :action intent)))
         (needs-zip (cdr (assoc :needs-zip intent)))
         (history (conversation-get session-id)))

    ;; If we need a zip code and don't have one, ask
    (when (and needs-zip (or (null i-zip) (zerop (length i-zip))))
      (let ((response `((:type . "needs-input")
                        (:field . "zip")
                        (:message . "I'd be happy to help! What's your zip code so I can find plans in your area?")
                        (:intent . ,intent)
                        (:layout . ,(idle-layout-with-prompt
                                     "I'd be happy to help! What's your zip code?")))))
        (conversation-push session-id "assistant" (cdr (assoc :message response)))
        (return-from medicare-converse response)))

    ;; Step 2: Get data (cached or fresh)
    (let* ((action-type (or i-action "lookup"))
           (result
             (cond
               ;; Follow-up: use existing data + generate new summary
               ((string-equal action-type "followup")
                (handle-followup user-message session-id i-plan-type i-zip))

               ;; Comparison: fetch multiple plan types
               ((string-equal action-type "compare")
                (handle-comparison i-zip session-id))

               ;; Detail: deeper dive on existing data
               ((string-equal action-type "detail")
                (handle-detail user-message session-id i-plan-type i-zip i-filter))

               ;; Standard lookup
               (t (medicare-lookup-cl i-plan-type i-zip i-filter)))))

      ;; Step 3: LLM generates layout intent
      (let* ((layout-intent (generate-layout-intent result history user-message))
             ;; Build full response
             (response `((:type . "result")
                         (:data . ,result)
                         (:layout . ,layout-intent)
                         (:intent . ,intent)
                         (:session . ,session-id))))

        ;; Record assistant response in conversation
        (conversation-push session-id "assistant"
                           (or (cdr (assoc :summary result)) ""))

        response))))

;; =========================================================================
;; Action handlers
;; =========================================================================

(defun handle-followup (user-query session-id plan-type zip)
  "Handle a follow-up question using existing cached data."
  (let* ((cached (cache-get plan-type zip ""))
         (previous-summary (when cached (cdr (assoc :summary cached)))))
    (if previous-summary
        ;; Have cached data — generate follow-up answer
        (let* ((system-msg (build-medicare-system-prompt))
               (user-msg (build-followup-prompt user-query previous-summary plan-type zip))
               (ai-result (ai-generate system-msg user-msg))
               (followup-summary (second ai-result)))
          ;; Return a result with the follow-up summary
          `((:plan-type . ,plan-type)
            (:plan-label . ,(plan-type-label plan-type))
            (:zip . ,zip)
            (:filter . ,user-query)
            (:summary . ,followup-summary)
            (:sources . ,(when cached (cdr (assoc :sources cached))))
            (:timestamp . ,(get-universal-time))
            (:cached . nil)
            (:is-followup . t)))
        ;; No cached data — do a fresh lookup with the question as filter
        (medicare-lookup-cl (or plan-type "advantage") (or zip "") user-query))))

(defun handle-comparison (zip session-id)
  "Handle a comparison request across plan types."
  (let ((results (loop for pt in '("advantage" "part-d" "supplement")
                       collect (medicare-lookup-cl pt zip ""))))
    `((:plan-type . "comparison")
      (:plan-label . "Plan Type Comparison")
      (:zip . ,zip)
      (:filter . "")
      (:summary . ,(build-comparison-summary results))
      (:comparisons . ,results)
      (:sources . ,(loop for r in results
                         append (cdr (assoc :sources r))))
      (:timestamp . ,(get-universal-time))
      (:cached . nil))))

(defun build-comparison-summary (results)
  "Build a comparison summary from multiple plan type results."
  (format nil "~{~A~%~%~}"
          (loop for r in results
                collect (format nil "## ~A~%~A"
                                (cdr (assoc :plan-label r))
                                (let ((s (cdr (assoc :summary r))))
                                  (if (> (length s) 500)
                                      (subseq s 0 500)
                                      s))))))

(defun handle-detail (user-query session-id plan-type zip filter)
  "Handle a detail request — deeper search with specific filter."
  (medicare-lookup-cl (or plan-type "advantage") zip
                      (if (and filter (> (length filter) 0))
                          filter
                          user-query)))

(defun idle-layout-with-prompt (message)
  "Layout intent for when we need more info from the user."
  `((:panels . ("header" "search-form" "chat-input"))
    (:emphasis . ,message)
    (:reasoning . "Need more information from user")))

;; =========================================================================
;; CL helper for Shen
;; =========================================================================

(defun cl-get-prop (key data)
  "Get a property from a result alist by string key."
  (let ((sym (intern (string-upcase key) :keyword)))
    (or (cdr (assoc sym data)) "")))

;; =========================================================================
;; API endpoints
;; =========================================================================

(hunchentoot:define-easy-handler (api-medicare :uri "/api/medicare") ()
  "Medicare plan lookup — structured input.
   POST: { planType, zip, filter? }"
  (let* ((params (read-json-body))
         (plan-type (or (cdr (assoc :plan-type params))
                        (cdr (assoc :plan--type params))
                        "advantage"))
         (zip (or (cdr (assoc :zip params)) ""))
         (filter (or (cdr (assoc :filter params)) "")))
    (when (or (< (length zip) 5) (not (every #'digit-char-p zip)))
      (return-from api-medicare
        (json-response '((:error . "Please enter a valid 5-digit zip code")) :status 400)))
    (unless (assoc plan-type *plan-type-labels* :test #'string-equal)
      (return-from api-medicare
        (json-response `((:error . ,(format nil "Unknown plan type: ~A" plan-type)))
                       :status 400)))
    (handler-case
        (json-response (medicare-lookup-cl plan-type zip filter))
      (error (e)
        (json-response `((:error . ,(format nil "Lookup failed: ~A" e))) :status 500)))))

(hunchentoot:define-easy-handler (api-medicare-chat :uri "/api/medicare/chat") ()
  "Conversational Medicare endpoint — natural language + generative UI.
   POST: { message, sessionId?, zip?, planType? }
   Returns: { type, data, layout, intent, session }"
  (let* ((params (read-json-body))
         (message (or (cdr (assoc :message params)) ""))
         (session-id (or (cdr (assoc :session-id params))
                         (cdr (assoc :session--id params))
                         (format nil "s-~A" (get-universal-time))))
         (zip (cdr (assoc :zip params)))
         (plan-type (or (cdr (assoc :plan-type params))
                        (cdr (assoc :plan--type params)))))
    (when (= (length message) 0)
      (return-from api-medicare-chat
        (json-response '((:error . "Message is required")) :status 400)))
    (handler-case
        (json-response
         (medicare-converse message session-id
                           :zip (when (and zip (>= (length zip) 5)) zip)
                           :plan-type plan-type))
      (error (e)
        (json-response `((:error . ,(format nil "Chat failed: ~A" e))) :status 500)))))

(hunchentoot:define-easy-handler (api-medicare-compare :uri "/api/medicare/compare") ()
  "Compare multiple plan types for a zip code.
   POST: { zip, planTypes? }"
  (let* ((params (read-json-body))
         (zip (or (cdr (assoc :zip params)) ""))
         (plan-types (or (cdr (assoc :plan-types params))
                         (cdr (assoc :plan--types params))
                         '("advantage" "part-d" "supplement"))))
    (when (or (< (length zip) 5) (not (every #'digit-char-p zip)))
      (return-from api-medicare-compare
        (json-response '((:error . "Please enter a valid 5-digit zip code")) :status 400)))
    (handler-case
        (let ((results (loop for pt in plan-types
                             collect (medicare-lookup-cl pt zip ""))))
          (json-response `((:zip . ,zip)
                           (:comparisons . ,results)
                           (:timestamp . ,(get-universal-time)))))
      (error (e)
        (json-response `((:error . ,(format nil "Comparison failed: ~A" e))) :status 500)))))

(hunchentoot:define-easy-handler (api-medicare-cache :uri "/api/medicare/cache") ()
  "Cache management. GET → stats, POST { action: 'clear' } → clear."
  (ecase (hunchentoot:request-method*)
    (:get (json-response (cache-stats)))
    (:post (let* ((params (read-json-body))
                  (action (cdr (assoc :action params))))
             (cond
               ((string-equal action "clear")
                (cache-clear)
                (json-response '((:status . "cache cleared"))))
               (t (json-response '((:error . "unknown action")) :status 400)))))))

(hunchentoot:define-easy-handler (api-medicare-plans :uri "/api/medicare/plans") ()
  "Return available plan types and their labels."
  (json-response
   `((:plan-types . ,(loop for (id . label) in *plan-type-labels*
                           collect `((:id . ,id) (:label . ,label)))))))
