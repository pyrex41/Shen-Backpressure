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
                result)
              (progn
                (format t "Cache EXPIRED: ~A~%" key)
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
  "Extract the first JSON object from an LLM response (strip markdown fences etc)."
  (let* ((start (position #\{ text))
         (end (when start (position #\} text :from-end t))))
    (if (and start end (> end start))
        (subseq text start (1+ end))
        "{}")))

;; =========================================================================
;; LLM-in-the-loop #2: Layout generation
;; =========================================================================
;; Given result data + conversation context, decide what UI panels to show.

(defun generate-layout-intent (result conversation-history user-query)
  "Use LLM to decide which UI panels to render and what to emphasize.
   Returns a layout-intent alist."
  (let* ((system "You are a UI layout planner for a Medicare insurance information tool. Given the data we have about Medicare plans, decide which UI panels to show and what to emphasize.

Available panel types:
- \"summary\": Main AI-generated summary (always good to include)
- \"cost-table\": Tabular cost breakdown (good for pricing questions)
- \"plan-cards\": Grid of individual plan cards (good for comparison)
- \"comparison\": Side-by-side comparison (when comparing plan types)
- \"detail\": Deep-dive panel (when user asks about specific aspect)
- \"chart\": Cost visualization (when showing price ranges)
- \"source-list\": Attributed sources (always include)
- \"followup\": Suggested follow-up questions
- \"filter-pills\": Active filter display
- \"disclaimer\": Legal disclaimer (always include)

Respond with ONLY a JSON object:
{
  \"panels\": [\"panel-kind\", ...],
  \"emphasis\": \"Short phrase describing what to highlight for the user\",
  \"reasoning\": \"Brief explanation of why this layout\"
}")
         (user-msg (format nil "User asked: ~A~%~%Data available:~%- Plan type: ~A~%- Zip: ~A~%- Filter: ~A~%- Summary length: ~D chars~%- Number of sources: ~D~%~A"
                           user-query
                           (cdr (assoc :plan-type result))
                           (cdr (assoc :zip result))
                           (cdr (assoc :filter result))
                           (length (or (cdr (assoc :summary result)) ""))
                           (length (cdr (assoc :sources result)))
                           (if conversation-history
                               (format nil "~%This is a follow-up question (turn ~D in conversation)."
                                       (length conversation-history))
                               "")))
         (ai-result (ai-generate system user-msg))
         (response-text (second ai-result))
         (parsed (handler-case
                     (cl-json:decode-json-from-string
                      (extract-json-from-response response-text))
                   (error () nil))))
    (if parsed
        `((:panels . ,(or (cdr (assoc :panels parsed)) '("summary" "source-list" "disclaimer")))
          (:emphasis . ,(or (cdr (assoc :emphasis parsed)) (cdr (assoc :plan-label result))))
          (:reasoning . ,(or (cdr (assoc :reasoning parsed)) "default layout")))
        ;; Fallback layout
        `((:panels . ("summary" "source-list" "followup" "disclaimer"))
          (:emphasis . ,(or (cdr (assoc :plan-label result)) "Medicare Plans"))
          (:reasoning . "default layout (LLM response parse failed)")))))

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
    (when (and needs-zip (or (null i-zip) (= (length i-zip) 0)))
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
