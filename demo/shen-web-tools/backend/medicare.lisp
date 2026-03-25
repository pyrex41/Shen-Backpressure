;;;; backend/medicare.lisp — Medicare plan lookup with caching
;;;;
;;;; CL-side implementation of Medicare-specific functionality:
;;;;   1. Query cache (keyed by plan-type + zip + filter)
;;;;   2. Medicare-focused search query construction
;;;;   3. Consumer-friendly AI prompt building
;;;;   4. CL fallback pipeline (when Shen not loaded)
;;;;   5. API endpoint handlers

(in-package :shen-web-tools)

;; =========================================================================
;; Cache layer
;; =========================================================================
;; In-memory hash table keyed by (plan-type zip filter).
;; Each entry stores the full result + timestamp.
;; Default TTL: 1 hour (Medicare plans don't change minute-to-minute).

(defvar *medicare-cache* (make-hash-table :test 'equal)
  "Cache: (plan-type zip filter) → (result . timestamp)")

(defvar *medicare-cache-lock* (bt:make-lock "medicare-cache"))

(defvar *medicare-cache-ttl* 3600
  "Cache TTL in seconds. Default 1 hour.")

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
;; Medicare-specific search queries
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
    ;; Add filter-specific query if provided
    (if (and filter (> (length filter) 0))
        (cons (format nil "medicare ~A ~A coverage ~A 2025 cost"
                      plan-type filter zip)
              base-queries)
        base-queries)))

;; =========================================================================
;; Medicare AI prompt
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
- Format response with clear sections: Plan Overview, Estimated Costs, What's Covered, and Next Steps
- Use bullet points and clear headings for readability
- If data is limited, be honest about it and point to authoritative sources")

(defun build-medicare-user-prompt (plan-type zip filter sources)
  "Build the user message with fetched source data."
  (format nil "A consumer in zip code ~A is looking for ~A information~A.~%~%~
               Please provide a clear, consumer-friendly summary of their options and estimated costs.~%~%~
               Here is what I found from web sources:~%~%~A"
          zip
          (plan-type-label plan-type)
          (if (and filter (> (length filter) 0))
              (format nil ", specifically about ~A" filter)
              "")
          (format-medicare-sources sources)))

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
;; Medicare pipeline (CL implementation)
;; =========================================================================

(defun medicare-lookup-cl (plan-type zip filter)
  "Full Medicare plan lookup. Checks cache first, then searches + generates.
   Returns an alist with query info, summary, sources, and timestamp."
  ;; Check cache
  (let ((cached (cache-get plan-type zip filter)))
    (when cached
      (return-from medicare-lookup-cl cached)))

  ;; Cache miss — run the pipeline
  (set-pipeline-state "searching" `((:plan-type . ,plan-type) (:zip . ,zip)))

  ;; Step 1: Search with multiple queries
  (let* ((queries (build-medicare-queries plan-type zip filter))
         (all-hits (loop for q in queries
                         append (handler-case (web-search q 5)
                                  (error () nil))))
         ;; Deduplicate by URL
         (seen (make-hash-table :test 'equal))
         (unique-hits (loop for hit in all-hits
                            unless (gethash (second hit) seen)
                              collect (progn (setf (gethash (second hit) seen) t) hit)))
         ;; Prioritize medicare.gov URLs
         (sorted-hits (sort (copy-list unique-hits)
                            (lambda (a b)
                              (let ((a-gov (or (search "medicare.gov" (second a))
                                               (search "cms.gov" (second a))))
                                    (b-gov (or (search "medicare.gov" (second b))
                                               (search "cms.gov" (second b)))))
                                (and a-gov (not b-gov)))))))

    ;; Step 2: Fetch top pages
    (set-pipeline-state "fetching" `((:hits . ,(length sorted-hits))
                                     (:plan-type . ,plan-type)))
    (let* ((top-hits (subseq sorted-hits 0 (min 5 (length sorted-hits))))
           (pages (loop for hit in top-hits
                        collect (handler-case (web-fetch (second hit))
                                  (error () (list (second hit) "" 0))))))

      ;; Step 3: Generate consumer-friendly summary
      (set-pipeline-state "generating" `((:sources . ,(length pages))
                                         (:plan-type . ,plan-type)))
      (let* ((system-msg (build-medicare-system-prompt))
             (user-msg (build-medicare-user-prompt plan-type zip filter pages))
             (ai-result (ai-generate system-msg user-msg))
             (summary (second ai-result))
             ;; Build result
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

        ;; Step 4: Cache and return
        (set-pipeline-state "complete" result)
        (cache-put plan-type zip filter result)
        result))))

;; =========================================================================
;; API endpoint handlers
;; =========================================================================

(hunchentoot:define-easy-handler (api-medicare :uri "/api/medicare") ()
  "Medicare plan lookup endpoint.
   POST body: { planType, zip, filter? }
   Returns: { planType, planLabel, zip, filter, summary, sources, timestamp, cached }"
  (let* ((params (read-json-body))
         (plan-type (or (cdr (assoc :plan-type params))
                        (cdr (assoc :plan--type params))
                        "advantage"))
         (zip (or (cdr (assoc :zip params)) ""))
         (filter (or (cdr (assoc :filter params)) "")))
    ;; Validate zip
    (when (or (< (length zip) 5)
              (not (every #'digit-char-p zip)))
      (return-from api-medicare
        (json-response '((:error . "Please enter a valid 5-digit zip code")) :status 400)))
    ;; Validate plan type
    (unless (assoc plan-type *plan-type-labels* :test #'string-equal)
      (return-from api-medicare
        (json-response `((:error . ,(format nil "Unknown plan type: ~A. Valid types: advantage, part-d, supplement, original, part-a, part-b" plan-type)))
                       :status 400)))
    ;; Run pipeline (or return cached)
    (handler-case
        (json-response (medicare-lookup-cl plan-type zip filter))
      (error (e)
        (json-response `((:error . ,(format nil "Medicare lookup failed: ~A" e)))
                       :status 500)))))

(hunchentoot:define-easy-handler (api-medicare-compare :uri "/api/medicare/compare") ()
  "Compare multiple plan types for a zip code.
   POST body: { zip, planTypes: ['advantage', 'part-d', ...] }
   Returns: { zip, comparisons: [...] }"
  (let* ((params (read-json-body))
         (zip (or (cdr (assoc :zip params)) ""))
         (plan-types (or (cdr (assoc :plan-types params))
                         (cdr (assoc :plan--types params))
                         '("advantage" "part-d" "supplement"))))
    (when (or (< (length zip) 5)
              (not (every #'digit-char-p zip)))
      (return-from api-medicare-compare
        (json-response '((:error . "Please enter a valid 5-digit zip code")) :status 400)))
    (handler-case
        (let ((results (loop for pt in plan-types
                             collect (medicare-lookup-cl pt zip ""))))
          (json-response `((:zip . ,zip)
                           (:comparisons . ,results)
                           (:timestamp . ,(get-universal-time)))))
      (error (e)
        (json-response `((:error . ,(format nil "Comparison failed: ~A" e)))
                       :status 500)))))

(hunchentoot:define-easy-handler (api-medicare-cache :uri "/api/medicare/cache") ()
  "Cache management endpoint.
   GET  → cache stats
   POST { action: 'clear' } → clear cache"
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
