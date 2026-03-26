;;;; backend/bridge.lisp — Web tool implementations in Common Lisp
;;;;
;;;; These are the actual I/O operations that Shen's logic layer calls.
;;;; Shen defines WHAT to do; these CL functions do the I/O.
;;;;
;;;; Providers:
;;;;   :mock       — fake data for dev (no network)
;;;;   :duckduckgo — DuckDuckGo HTML scraping (no API key, same as rho-cli)
;;;;   :rho        — shell out to rho-cli binary for search/fetch/AI
;;;;   :live       — Brave Search API (needs BRAVE_API_KEY)
;;;;   :anthropic  — Anthropic Messages API (needs ANTHROPIC_API_KEY)
;;;;
;;;; Functions:
;;;;   (web-search query max-results) → list of (title url snippet)
;;;;   (web-fetch url) → (url content timestamp)
;;;;   (ai-generate system-msg user-msg) → (prompt text timestamp)

(in-package :shen-web-tools)

;; =========================================================================
;; Web Search
;; =========================================================================

(defun web-search (query max-results)
  "Search the web. Returns list of (title url snippet) triples."
  (demo-log "search" (format nil "~A [provider: ~A, max: ~D]" query *search-provider* max-results))
  (let ((results (ecase *search-provider*
                   (:mock       (web-search-mock query max-results))
                   (:duckduckgo (web-search-ddg query max-results))
                   (:rho        (rho-web-search query max-results))
                   (:live       (web-search-brave query max-results)))))
    (demo-log "search" (format nil "Got ~D results" (length results))
              (format nil "~{~A~^~%~}" (mapcar #'first (subseq results 0 (min 3 (length results))))))
    results))

;; --- Mock ---

(defun web-search-mock (query max-results)
  "Mock search: generate fake results for development."
  (loop for i from 1 to (min max-results 5)
        collect (list (format nil "~A - Result ~D" query i)
                      (format nil "https://example.com/~A/~D"
                              (substitute #\- #\Space (string-downcase query)) i)
                      (format nil "This is a snippet about ~A from source ~D. ~
                                   It contains relevant information about the topic." query i))))

;; --- DuckDuckGo (same approach as rho-cli) ---
;; POST to https://html.duckduckgo.com/html/ with form body q=QUERY
;; Parse <a class="result__a"> for title+url, <a class="result__snippet"> for snippet

(defun web-search-ddg (query max-results)
  "Search DuckDuckGo HTML endpoint. No API key required."
  (handler-case
      (let* ((body (format nil "q=~A&b=&kl=" (url-encode-form query)))
             (response (dex:post "https://html.duckduckgo.com/html/"
                                 :content body
                                 :headers '(("Content-Type" . "application/x-www-form-urlencoded")
                                            ("Referer" . "https://duckduckgo.com/")
                                            ("User-Agent" . "Mozilla/5.0 (compatible; ShenWebTools/1.0)"))
                                 :want-stream nil
                                 :connect-timeout 15))
             (results (parse-ddg-results response max-results)))
        (or results (web-search-mock query max-results)))
    (error (e)
      (warn "DuckDuckGo search failed: ~A" e)
      (web-search-mock query max-results))))

(defun parse-ddg-results (html max-results)
  "Parse DuckDuckGo HTML for search results.
   Extracts links from <a class=\"result__a\"> and snippets from result__snippet."
  (let ((results nil)
        (pos 0)
        (len (length html)))
    (loop while (and (< pos len) (< (length results) max-results))
          do (let ((link-start (search "class=\"result__a\"" html :start2 pos)))
               (if (null link-start)
                   (return)
                   (let* (;; Find href in this <a> tag
                          (a-open (search-backward "<a" html link-start))
                          (href-start (search "href=\"" html :start2 (or a-open link-start)))
                          (href-begin (when href-start (+ href-start 6)))
                          (href-end (when href-begin (position #\" html :start href-begin)))
                          (url (when (and href-begin href-end)
                                 (subseq html href-begin href-end)))
                          ;; Find title (text inside the <a> tag)
                          (text-start (search ">" html :start2 link-start))
                          (text-begin (when text-start (1+ text-start)))
                          (text-end (when text-begin (search "</a>" html :start2 text-begin)))
                          (title-raw (when (and text-begin text-end)
                                       (subseq html text-begin text-end)))
                          (title (when title-raw (strip-html-tags title-raw)))
                          ;; Find snippet (next result__snippet after this link)
                          (snip-marker (search "result__snippet" html :start2 (or text-end link-start)))
                          (snip-start (when snip-marker (search ">" html :start2 snip-marker)))
                          (snip-begin (when snip-start (1+ snip-start)))
                          (snip-end (when snip-begin (search "</a>" html :start2 snip-begin)))
                          (snippet-raw (when (and snip-begin snip-end)
                                         (subseq html snip-begin snip-end)))
                          (snippet (when snippet-raw (strip-html-tags snippet-raw))))
                     (when (and url title (> (length url) 0) (> (length title) 0))
                       (push (list (string-trim '(#\Space #\Newline #\Tab) title)
                                   (decode-ddg-url url)
                                   (or (when snippet (string-trim '(#\Space #\Newline #\Tab) snippet))
                                       ""))
                             results))
                     (setf pos (or text-end (1+ link-start)))))))
    (nreverse results)))

(defun decode-ddg-url (url)
  "DuckDuckGo wraps URLs in redirects. Extract the real URL if present."
  (let ((uddg (search "uddg=" url)))
    (if uddg
        (let* ((start (+ uddg 5))
               (end (or (position #\& url :start start) (length url))))
          (url-decode (subseq url start end)))
        url)))

(defun search-backward (needle haystack end)
  "Search backward from END in HAYSTACK for NEEDLE."
  (loop for i from (max 0 (- end (length needle) 200)) below end
        when (and (<= (+ i (length needle)) (length haystack))
                  (string= needle haystack :start2 i :end2 (+ i (length needle))))
          return i))

;; --- Brave Search API ---

(defun web-search-brave (query max-results)
  "Live web search via Brave Search API."
  (handler-case
      (let* ((api-key (or (uiop:getenv "BRAVE_API_KEY") ""))
             (url (format nil "https://api.search.brave.com/res/v1/web/search?q=~A&count=~D"
                          (url-encode-form query) max-results))
             (response (dex:get url
                                :headers `(("Accept" . "application/json")
                                           ("X-Subscription-Token" . ,api-key))
                                :want-stream nil))
             (json (cl-json:decode-json-from-string response))
             (results (cdr (assoc :web--results json))))
        (loop for r in (subseq results 0 (min (length results) max-results))
              collect (list (cdr (assoc :title r))
                            (cdr (assoc :url r))
                            (or (cdr (assoc :description r)) ""))))
    (error (e)
      (warn "Brave search failed: ~A — falling back to DuckDuckGo" e)
      (web-search-ddg query max-results))))

;; =========================================================================
;; Web Fetch
;; =========================================================================

(defun web-fetch (url)
  "Fetch a URL and return (url content timestamp)."
  (demo-log "fetch" (format nil "~A [~A]" url *fetch-provider*))
  (let ((result (ecase *fetch-provider*
                  (:mock       (web-fetch-mock url))
                  (:duckduckgo (web-fetch-dex url))
                  (:rho        (rho-web-fetch url))
                  (:live       (web-fetch-dex url)))))
    (demo-log "fetch" (format nil "Got ~D chars from ~A" (length (second result)) url))
    result))

(defun web-fetch-mock (url)
  "Mock fetch: return simulated page content."
  (list url
        (format nil "Mock content fetched from ~A. This simulates the text content ~
                     that would be extracted from the web page. In production, the CL ~
                     backend uses dexador to retrieve and parse the actual page." url)
        (get-universal-time)))

(defun web-fetch-dex (url)
  "Fetch a real URL using dexador, extract text content."
  (handler-case
      (let* ((response (dex:get url
                                :want-stream nil
                                :connect-timeout 15
                                :headers '(("User-Agent" . "Mozilla/5.0 (compatible; ShenWebTools/1.0)"))))
             (text (strip-html-tags response))
             (clean (collapse-whitespace text))
             (truncated (if (> (length clean) 5000)
                            (subseq clean 0 5000)
                            clean)))
        (list url truncated (get-universal-time)))
    (error (e)
      (warn "Web fetch failed for ~A: ~A" url e)
      (web-fetch-mock url))))

;; =========================================================================
;; AI Generation
;; =========================================================================

(defun ai-generate (system-msg user-msg)
  "Send a prompt to the AI and return (prompt text timestamp).
   Prompt is stored as (system-msg user-msg)."
  (demo-log "ai" (format nil "Generating [~A~A]"
                         *ai-provider*
                         (if *rho-model* (format nil ", model: ~A" *rho-model*) ""))
            (format nil "System: ~A~%~%User: ~A"
                    (subseq system-msg 0 (min 200 (length system-msg)))
                    (subseq user-msg 0 (min 300 (length user-msg)))))
  (let* ((prompt (list system-msg user-msg))
         (result (ecase *ai-provider*
                   (:mock      (ai-generate-mock prompt))
                   (:anthropic (ai-generate-anthropic prompt system-msg user-msg))
                   (:rho       (rho-ai-generate prompt system-msg user-msg)))))
    (demo-log "ai" (format nil "Response: ~D chars" (length (second result)))
              (subseq (second result) 0 (min 500 (length (second result)))))
    result))

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

;; =========================================================================
;; Rho-CLI subprocess provider
;; =========================================================================
;; Shells out to `rho` binary for search, fetch, and AI generation.
;; rho-cli uses DuckDuckGo for search, direct HTTP for fetch, and
;; Anthropic/OpenAI for AI — all with no extra API keys for web tools.
;;
;; Usage: set *search-provider* :rho, *fetch-provider* :rho, *ai-provider* :rho
;; Requires: rho binary on PATH (cargo install from pyrex41/rho)

(defun rho-run (prompt &key (tools nil) (model nil))
  "Run rho-cli in one-shot mode, return stdout as string."
  (let* ((cmd (list *rho-binary*))
         (cmd (if tools
                  (append cmd (list "--tools" (format nil "~{~A~^,~}" tools)))
                  cmd))
         (cmd (if model
                  (append cmd (list "--model" model))
                  cmd))
         (cmd (append cmd (list "--output-format" "text" prompt))))
    (handler-case
        (let ((output (uiop:run-program cmd
                                        :output :string
                                        :error-output :string
                                        :ignore-error-status t)))
          (string-trim '(#\Space #\Newline #\Tab) output))
      (error (e)
        (warn "rho-cli failed: ~A" e)
        nil))))

(defun rho-web-search (query max-results)
  "Use rho-cli to perform a web search."
  (let* ((prompt (format nil "Search the web for: ~A~%~
                              Return exactly ~D results as a numbered list. ~
                              For each result, output on separate lines:~%~
                              TITLE: <title>~%URL: <url>~%SNIPPET: <snippet>~%---"
                         query max-results))
         (output (rho-run prompt :tools '("web_search"))))
    (if output
        (parse-rho-search-results output max-results)
        (web-search-mock query max-results))))

(defun parse-rho-search-results (text max-results)
  "Parse rho-cli search output into (title url snippet) triples."
  (let ((results nil)
        (lines (split-string text #\Newline))
        (title nil) (url nil) (snippet nil))
    (dolist (line lines)
      (let ((trimmed (string-trim '(#\Space #\Tab) line)))
        (cond
          ((and (>= (length trimmed) 7) (string= "TITLE: " (subseq trimmed 0 7)))
           (setf title (subseq trimmed 7)))
          ((and (>= (length trimmed) 5) (string= "URL: " (subseq trimmed 0 5)))
           (setf url (subseq trimmed 5)))
          ((and (>= (length trimmed) 9) (string= "SNIPPET: " (subseq trimmed 0 9)))
           (setf snippet (subseq trimmed 9)))
          ((string= "---" trimmed)
           (when (and title url)
             (push (list title url (or snippet "")) results)
             (setf title nil url nil snippet nil))))))
    ;; Catch last result if no trailing ---
    (when (and title url)
      (push (list title url (or snippet "")) results))
    (let ((r (nreverse results)))
      (subseq r 0 (min (length r) max-results)))))

(defun rho-web-fetch (url)
  "Use rho-cli to fetch a URL."
  (let* ((prompt (format nil "Fetch this URL and return the text content: ~A~%~
                              Output ONLY the page text content, no commentary." url))
         (output (rho-run prompt :tools '("web_fetch"))))
    (if output
        (list url (if (> (length output) 5000) (subseq output 0 5000) output) (get-universal-time))
        (web-fetch-mock url))))

(defun rho-ai-generate (prompt system-msg user-msg)
  "Use rho-cli for AI generation. Uses *rho-model* if set."
  (let ((output (rho-run (format nil "~A~%~%~A" system-msg user-msg)
                         :model *rho-model*)))
    (if output
        (list prompt output (get-universal-time))
        (ai-generate-mock prompt))))

;; =========================================================================
;; URL encoding/decoding utilities
;; =========================================================================

(defun char-to-utf8-bytes (c)
  "Convert a character to its UTF-8 byte sequence as a list of integers."
  #+sbcl (coerce (sb-ext:string-to-octets (string c) :external-format :utf-8) 'list)
  #-sbcl (let ((code (char-code c)))
           (cond
             ((<= code #x7F) (list code))
             ((<= code #x7FF)
              (list (logior #xC0 (ash code -6))
                    (logior #x80 (logand code #x3F))))
             ((<= code #xFFFF)
              (list (logior #xE0 (ash code -12))
                    (logior #x80 (logand (ash code -6) #x3F))
                    (logior #x80 (logand code #x3F))))
             (t
              (list (logior #xF0 (ash code -18))
                    (logior #x80 (logand (ash code -12) #x3F))
                    (logior #x80 (logand (ash code -6) #x3F))
                    (logior #x80 (logand code #x3F)))))))

(defun url-encode-form (string)
  "Percent-encode a string for use in URL form data. Spaces become +.
   Non-ASCII characters are encoded as UTF-8 bytes."
  (with-output-to-string (out)
    (loop for c across string
          do (cond
               ((char= c #\Space) (write-char #\+ out))
               ((or (and (alphanumericp c) (<= (char-code c) 127))
                    (find c "-_.~"))
                (write-char c out))
               (t (dolist (byte (char-to-utf8-bytes c))
                    (format out "%~2,'0X" byte)))))))

(defun url-decode (string)
  "Decode a percent-encoded string."
  (with-output-to-string (out)
    (let ((i 0) (len (length string)))
      (loop while (< i len)
            do (let ((c (char string i)))
                 (cond
                   ((char= c #\%)
                    (when (< (+ i 2) len)
                      (write-char (code-char (parse-integer string
                                                            :start (1+ i)
                                                            :end (+ i 3)
                                                            :radix 16))
                                  out))
                    (incf i 3))
                   ((char= c #\+)
                    (write-char #\Space out)
                    (incf i))
                   (t
                    (write-char c out)
                    (incf i))))))))

;; =========================================================================
;; HTML / text utilities
;; =========================================================================

(defun strip-html-tags (html)
  "Strip HTML tags, decode common entities. Like rho's html_to_text."
  (let ((text (with-output-to-string (out)
                (let ((in-tag nil) (in-script nil))
                  (loop with i = 0 and len = (length html)
                        while (< i len)
                        do (let ((c (char html i)))
                             (cond
                               ;; Skip <script> and <style> blocks entirely
                               ((and (not in-script)
                                     (< (+ i 7) len)
                                     (or (string-equal "<script" (subseq html i (+ i 7)))
                                         (string-equal "<style" (subseq html i (min (+ i 6) len)))))
                                (setf in-script t)
                                (incf i))
                               ((and in-script
                                     (or (search "</script>" html :start2 i :end2 (min (+ i 9) len))
                                         (search "</style>" html :start2 i :end2 (min (+ i 8) len))))
                                (setf in-script nil)
                                (setf i (or (search ">" html :start2 i) len))
                                (incf i))
                               (in-script (incf i))
                               ;; Normal tag handling
                               ((char= c #\<) (setf in-tag t) (incf i))
                               ((char= c #\>) (setf in-tag nil) (write-char #\Space out) (incf i))
                               ((not in-tag) (write-char c out) (incf i))
                               (t (incf i)))))))))
    ;; Decode HTML entities
    (setf text (replace-all text "&amp;" "&"))
    (setf text (replace-all text "&lt;" "<"))
    (setf text (replace-all text "&gt;" ">"))
    (setf text (replace-all text "&quot;" "\""))
    (setf text (replace-all text "&nbsp;" " "))
    (setf text (replace-all text "&#39;" "'"))
    text))

(defun collapse-whitespace (text)
  "Collapse runs of whitespace into single spaces, trim leading/trailing."
  (string-trim '(#\Space #\Newline #\Tab)
               (with-output-to-string (out)
                 (let ((last-was-space nil))
                   (loop for c across text
                         do (if (member c '(#\Space #\Newline #\Tab #\Return))
                                (unless last-was-space
                                  (write-char #\Space out)
                                  (setf last-was-space t))
                                (progn
                                  (write-char c out)
                                  (setf last-was-space nil))))))))

(defun replace-all (string old new)
  "Replace all occurrences of OLD with NEW in STRING."
  (let ((old-len (length old)))
    (with-output-to-string (out)
      (loop with i = 0 and len = (length string)
            while (< i len)
            do (let ((found (search old string :start2 i)))
                 (if found
                     (progn
                       (write-string (subseq string i found) out)
                       (write-string new out)
                       (setf i (+ found old-len)))
                     (progn
                       (write-string (subseq string i) out)
                       (setf i len))))))))

;; =========================================================================
;; Shared utilities (called by Shen via CL interop)
;; =========================================================================

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
