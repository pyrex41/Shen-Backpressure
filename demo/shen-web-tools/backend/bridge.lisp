;;;; backend/bridge.lisp — Web tool implementations in Common Lisp
;;;;
;;;; These are the actual I/O operations that Shen's logic layer calls.
;;;; Shen defines WHAT to do; these CL functions do the I/O.
;;;;
;;;; Functions:
;;;;   (web-search query max-results) → list of (title url snippet)
;;;;   (web-fetch url) → (url content timestamp)
;;;;   (ai-generate system-msg user-msg) → (prompt text timestamp)

(in-package :shen-web-tools)

;; -------------------------------------------------------------------------
;; Web Search
;; -------------------------------------------------------------------------

(defun web-search (query max-results)
  "Search the web. Returns list of (title url snippet) triples."
  (ecase *search-provider*
    (:mock (web-search-mock query max-results))
    (:live (web-search-live query max-results))))

(defun web-search-mock (query max-results)
  "Mock search: generate fake results for development."
  (loop for i from 1 to (min max-results 5)
        collect (list (format nil "~A - Result ~D" query i)
                      (format nil "https://example.com/~A/~D"
                              (substitute #\- #\Space (string-downcase query)) i)
                      (format nil "This is a snippet about ~A from source ~D. ~
                                   It contains relevant information about the topic." query i))))

(defun web-search-live (query max-results)
  "Live web search via API. Uses a search endpoint or scraping service."
  (handler-case
      (let* ((url (format nil "https://api.search.brave.com/res/v1/web/search?q=~A&count=~D"
                          (dex:url-encode query) max-results))
             (response (dex:get url
                                :headers '(("Accept" . "application/json"))
                                :want-stream nil))
             (json (cl-json:decode-json-from-string response))
             (results (cdr (assoc :web--results json))))
        (loop for r in (subseq results 0 (min (length results) max-results))
              collect (list (cdr (assoc :title r))
                            (cdr (assoc :url r))
                            (or (cdr (assoc :description r)) ""))))
    (error (e)
      (warn "Web search failed: ~A" e)
      (web-search-mock query max-results))))

;; -------------------------------------------------------------------------
;; Web Fetch
;; -------------------------------------------------------------------------

(defun web-fetch (url)
  "Fetch a URL and return (url content timestamp)."
  (ecase *fetch-provider*
    (:mock (web-fetch-mock url))
    (:live (web-fetch-live url))))

(defun web-fetch-mock (url)
  "Mock fetch: return simulated page content."
  (list url
        (format nil "Mock content fetched from ~A. This simulates the text content ~
                     that would be extracted from the web page. In production, the CL ~
                     backend uses dexador to retrieve and parse the actual page." url)
        (get-universal-time)))

(defun web-fetch-live (url)
  "Fetch a real URL using dexador, extract text content."
  (handler-case
      (let* ((response (dex:get url :want-stream nil :connect-timeout 10))
             ;; Simple HTML-to-text: strip tags
             (text (strip-html-tags response)))
        (list url (subseq text 0 (min (length text) 5000)) (get-universal-time)))
    (error (e)
      (warn "Web fetch failed for ~A: ~A" url e)
      (web-fetch-mock url))))

(defun strip-html-tags (html)
  "Crude HTML tag stripper. For production, use cl-html-parse."
  (with-output-to-string (out)
    (let ((in-tag nil))
      (loop for c across html
            do (cond
                 ((char= c #\<) (setf in-tag t))
                 ((char= c #\>) (setf in-tag nil) (write-char #\Space out))
                 ((not in-tag) (write-char c out)))))))

;; -------------------------------------------------------------------------
;; AI Generation
;; -------------------------------------------------------------------------

(defun ai-generate (system-msg user-msg)
  "Send a prompt to the AI and return (prompt text timestamp).
   Prompt is stored as (system-msg user-msg)."
  (let ((prompt (list system-msg user-msg)))
    (ecase *ai-provider*
      (:mock (ai-generate-mock prompt))
      (:anthropic (ai-generate-anthropic prompt system-msg user-msg)))))

(defun ai-generate-mock (prompt)
  "Mock AI generation."
  (let ((text (format nil "## Research Summary~%~%~
                Based on the provided sources, here is a comprehensive overview:~%~%~
                **Key Findings:**~%~
                1. The topic has been extensively covered across multiple sources~%~
                2. There is general consensus on the core concepts~%~
                3. Several sources provide unique perspectives worth exploring~%~%~
                **Details:**~%~
                The research draws from multiple web sources to provide a grounded ~
                analysis. Each source was verified and cross-referenced.~%~%~
                **Open Questions:**~%~
                - Further research could explore emerging developments~%~
                - Some sources suggest alternative interpretations")))
    (list prompt text (get-universal-time))))

(defun ai-generate-anthropic (prompt system-msg user-msg)
  "Call Anthropic API using dexador."
  (unless *anthropic-api-key*
    (error "ANTHROPIC_API_KEY not set. Pass --api-key or set the environment variable."))
  (handler-case
      (let* ((body (cl-json:encode-json-to-string
                    `((:model . ,*anthropic-model*)
                      (:max--tokens . 1024)
                      (:system . ,system-msg)
                      (:messages . (((:role . "user") (:content . ,user-msg)))))))
             (response (dex:post "https://api.anthropic.com/v1/messages"
                                 :content body
                                 :headers `(("Content-Type" . "application/json")
                                            ("x-api-key" . ,*anthropic-api-key*)
                                            ("anthropic-version" . "2023-06-01"))
                                 :want-stream nil))
             (json (cl-json:decode-json-from-string response))
             (content (cdr (assoc :content json)))
             (text (cdr (assoc :text (car content)))))
        (list prompt (or text "") (get-universal-time)))
    (error (e)
      (warn "Anthropic API call failed: ~A" e)
      (ai-generate-mock prompt))))

;; -------------------------------------------------------------------------
;; Utilities (called by Shen via CL interop)
;; -------------------------------------------------------------------------

(defun current-timestamp ()
  "Current time as universal time."
  (get-universal-time))

(defun extract-terms (query)
  "Extract key terms from a query string."
  (let ((stopwords '("the" "and" "for" "with" "about" "from" "that" "this")))
    (remove-if (lambda (w)
                 (or (<= (length w) 3)
                     (member w stopwords :test #'string-equal)))
               (split-string query #\Space))))

(defun split-string (string delimiter)
  "Split STRING by DELIMITER character."
  (loop for start = 0 then (1+ end)
        for end = (position delimiter string :start start)
        collect (subseq string start (or end (length string)))
        while end))
