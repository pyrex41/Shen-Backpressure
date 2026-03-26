;;;; backend/server.lisp — HTTP server for Shen Web Tools
;;;;
;;;; Hunchentoot serves:
;;;;   GET  /                → index.html
;;;;   GET  /static/*        → CSS assets
;;;;   GET  /runtime/*.js    → compiled JS (Arrow.js frontend)
;;;;   GET  /src/*.shen      → Shen source files (for inspection)
;;;;   POST /api/research    → full research pipeline (Shen orchestrates)
;;;;   POST /api/search      → just the search step
;;;;   POST /api/fetch       → just the fetch step
;;;;   POST /api/generate    → just the AI generation step
;;;;   GET  /api/state       → current pipeline state

(in-package :shen-web-tools)

;; -------------------------------------------------------------------------
;; Pipeline state (Shen writes this; frontend reads it)
;; -------------------------------------------------------------------------

(defvar *pipeline-state*
  '((:phase . "idle") (:data . nil))
  "Current pipeline state, updated by Shen logic.")

(defvar *pipeline-lock* (bt:make-lock "pipeline"))

(defun set-pipeline-state (phase data)
  "Update pipeline state (called by Shen during execution)."
  (bt:with-lock-held (*pipeline-lock*)
    (setf *pipeline-state*
          `((:phase . ,phase) (:data . ,data)))))

(defun get-pipeline-state ()
  "Read current pipeline state."
  (bt:with-lock-held (*pipeline-lock*)
    *pipeline-state*))

;; -------------------------------------------------------------------------
;; Demo log stream — captures pipeline activity for the split-pane UI
;; -------------------------------------------------------------------------

(defvar *demo-log* nil "Ring buffer of log entries (newest first)")
(defvar *demo-log-lock* (bt:make-lock "demo-log"))
(defvar *demo-log-max* 200 "Max log entries to keep")
(defvar *demo-log-seq* 0 "Monotonic sequence number for log entries")

(defun demo-log (category message &optional detail)
  "Add a log entry. Category: search, fetch, ai, cache, pipeline, layout."
  (bt:with-lock-held (*demo-log-lock*)
    (incf *demo-log-seq*)
    (push `((:seq . ,*demo-log-seq*)
            (:ts . ,(get-universal-time))
            (:cat . ,category)
            (:msg . ,message)
            ,@(when detail `((:detail . ,detail))))
          *demo-log*)
    (when (> (length *demo-log*) *demo-log-max*)
      (setf *demo-log* (subseq *demo-log* 0 *demo-log-max*)))))

(defun demo-log-since (seq)
  "Return log entries with seq > SEQ, oldest first."
  (bt:with-lock-held (*demo-log-lock*)
    (nreverse (remove-if (lambda (e) (<= (cdr (assoc :seq e)) seq))
                         (copy-list *demo-log*)))))

(defun demo-log-current-seq ()
  (bt:with-lock-held (*demo-log-lock*) *demo-log-seq*))

;; -------------------------------------------------------------------------
;; JSON helpers
;; -------------------------------------------------------------------------

(defun json-response (data &key (status 200))
  "Set response headers and return JSON-encoded DATA."
  (setf (hunchentoot:content-type*) "application/json; charset=utf-8")
  (setf (hunchentoot:return-code*) status)
  (cl-json:encode-json-to-string data))

(defun read-json-body ()
  "Parse JSON from the request body."
  (let ((body (hunchentoot:raw-post-data :force-text t)))
    (when (and body (> (length body) 0))
      (cl-json:decode-json-from-string body))))

;; -------------------------------------------------------------------------
;; API handlers
;; -------------------------------------------------------------------------

(hunchentoot:define-easy-handler (api-research :uri "/api/research") ()
  "Full research pipeline. Shen orchestrates: search → fetch → ground → generate."
  (let* ((params (read-json-body))
         (query (cdr (assoc :query params))))
    (unless query
      (return-from api-research
        (json-response '((:error . "missing 'query' field")) :status 400)))

    ;; Step 1: Search
    (set-pipeline-state "searching" `((:query . ,query)))
    (let* ((max-results 10)
           (hits (web-search query max-results))
           (search-result `((:query . ,query)
                            (:max-results . ,max-results)
                            (:hits . ,(mapcar #'hit->alist hits))
                            (:timestamp . ,(current-timestamp)))))

      ;; Step 2: Fetch top pages
      (set-pipeline-state "fetching" search-result)
      (let* ((top-hits (subseq hits 0 (min 5 (length hits))))
             (pages (mapcar (lambda (hit)
                              (web-fetch (second hit)))
                            top-hits))
             ;; Step 3: Ground sources (pair pages with hits)
             (sources (ground-sources pages top-hits)))

        ;; Step 4: Generate AI summary
        (set-pipeline-state "generating" `((:sources . ,(length sources))
                                           (:query . ,query)))
        (let* ((system-msg (format nil "You are a research assistant. ~
                                        Summarize the following web sources about: ~A" query))
               (user-msg (format nil "Based on these sources, provide a clear summary:~%~%~A"
                                 (format-sources-for-ai sources)))
               (ai-result (ai-generate system-msg user-msg))
               (summary-text (second ai-result))
               ;; Build final result
               (result `((:query . ,query)
                         (:summary . ,summary-text)
                         (:sources . ,(mapcar #'source->alist sources))
                         (:timestamp . ,(current-timestamp)))))

          ;; Step 5: Complete
          (set-pipeline-state "complete" result)
          (json-response result))))))

(hunchentoot:define-easy-handler (api-search :uri "/api/search") ()
  "Search-only endpoint."
  (let* ((params (read-json-body))
         (query (cdr (assoc :query params)))
         (max-results (or (cdr (assoc :max-results params)) 10))
         (hits (web-search query max-results)))
    (json-response `((:query . ,query)
                     (:hits . ,(mapcar #'hit->alist hits))
                     (:timestamp . ,(current-timestamp))))))

(hunchentoot:define-easy-handler (api-fetch :uri "/api/fetch") ()
  "Fetch-only endpoint."
  (let* ((params (read-json-body))
         (url (cdr (assoc :url params)))
         (page (web-fetch url)))
    (json-response `((:url . ,(first page))
                     (:content . ,(second page))
                     (:timestamp . ,(third page))))))

(hunchentoot:define-easy-handler (api-generate :uri "/api/generate") ()
  "AI generation endpoint."
  (let* ((params (read-json-body))
         (system-msg (cdr (assoc :system params)))
         (user-msg (cdr (assoc :user params)))
         (result (ai-generate system-msg user-msg)))
    (json-response `((:text . ,(second result))
                     (:timestamp . ,(third result))))))

(hunchentoot:define-easy-handler (api-state :uri "/api/state") ()
  "Return current pipeline state (for polling fallback)."
  (json-response (get-pipeline-state)))

;; -------------------------------------------------------------------------
;; SSE streaming endpoint — replaces polling
;; -------------------------------------------------------------------------
;; Client opens EventSource("/api/stream"). We hold the connection open,
;; sending state transitions as they happen. When the pipeline reaches
;; "complete" or "idle", we send the final event and close.

(hunchentoot:define-easy-handler (api-stream :uri "/api/stream") ()
  "Server-Sent Events stream for pipeline state changes.
   Sends events: phase, layout, result, done, error."
  (setf (hunchentoot:content-type*) "text/event-stream; charset=utf-8")
  (setf (hunchentoot:header-out :cache-control) "no-cache")
  (setf (hunchentoot:header-out :connection) "keep-alive")
  ;; Disable chunked encoding — SSE needs raw flushing
  (setf (hunchentoot:reply-external-format*) :utf-8)
  (let ((stream (hunchentoot:send-headers))
        (last-phase "")
        (ticks 0)
        (max-ticks 600)) ; 5 min timeout at 500ms sleep
    (handler-case
        (loop
          (let* ((state (get-pipeline-state))
                 (phase (cdr (assoc :phase state)))
                 (data (cdr (assoc :data state))))
            ;; Send phase change
            (when (not (string= phase last-phase))
              (sse-write stream "phase"
                         (cl-json:encode-json-to-string
                          `((:phase . ,phase)
                            (:timestamp . ,(get-universal-time)))))
              (setf last-phase phase))
            ;; On complete/idle → send final data and close
            (when (string= phase "complete")
              (sse-write stream "result"
                         (cl-json:encode-json-to-string data))
              (sse-write stream "done" "{}")
              (force-output stream)
              (return))
            (when (and (string= phase "idle") (> ticks 2))
              (sse-write stream "done" "{}")
              (force-output stream)
              (return))
            ;; Heartbeat every ~5s to keep connection alive
            (when (zerop (mod ticks 10))
              (sse-write stream "heartbeat"
                         (format nil "{\"tick\":~D}" ticks)))
            ;; Timeout
            (incf ticks)
            (when (>= ticks max-ticks)
              (sse-write stream "error" "{\"message\":\"timeout\"}")
              (force-output stream)
              (return))
            (sleep 0.5)))
      (error (e)
        (handler-case
            (progn
              (sse-write stream "error"
                         (format nil "{\"message\":\"~A\"}" e))
              (force-output stream))
          (error () nil))))))

(defun sse-write (stream event data)
  "Write a single SSE event to the stream."
  (format stream "event: ~A~%data: ~A~%~%" event data)
  (force-output stream))

;; -------------------------------------------------------------------------
;; Demo log SSE — streams backend activity to the split-pane UI
;; -------------------------------------------------------------------------

(hunchentoot:define-easy-handler (api-logs :uri "/api/logs") (since)
  "Poll-based log endpoint. Returns new log entries since ?since=N.
   Response: { seq: N, entries: [...] }"
  (let* ((since-seq (or (when since (parse-integer since :junk-allowed t)) 0))
         (entries (demo-log-since since-seq))
         (current-seq (demo-log-current-seq)))
    (json-response `((:seq . ,current-seq) (:entries . ,entries)))))

;; -------------------------------------------------------------------------
;; Source grounding (CL implementation of the Shen logic)
;; -------------------------------------------------------------------------

(defun ground-sources (pages hits)
  "Pair fetched pages with their search hits, verifying URL match.
   This enforces the grounded-source invariant from the Shen spec:
   (= (head Page) (head (tail Hit))) : verified"
  (loop for page in pages
        for hit in hits
        when (string= (first page) (second hit))
          collect (list page hit)))

(defun format-sources-for-ai (sources)
  "Format grounded sources into text for the AI prompt."
  (with-output-to-string (s)
    (loop for (page hit) in sources
          for i from 1
          do (format s "[~D] ~A (~A)~%~A~%~%"
                     i (first hit) (second hit)
                     (subseq (second page) 0 (min 500 (length (second page))))))))

;; -------------------------------------------------------------------------
;; Serialization helpers
;; -------------------------------------------------------------------------

(defun hit->alist (hit)
  "Convert a search hit (title url snippet) to an alist."
  `((:title . ,(first hit))
    (:url . ,(second hit))
    (:snippet . ,(third hit))))

(defun source->alist (source)
  "Convert a grounded source ((url content ts) (title url snippet)) to alist."
  (let ((page (first source))
        (hit (second source)))
    `((:page-url . ,(first page))
      (:title . ,(first hit))
      (:url . ,(second hit))
      (:snippet . ,(third hit))
      (:grounded . t))))

;; -------------------------------------------------------------------------
;; Static file serving
;; -------------------------------------------------------------------------

(defun safe-static-path (relative-path root)
  "Resolve RELATIVE-PATH under ROOT. Returns NIL if the resolved path
   escapes ROOT (path traversal attack)."
  (let* ((merged (merge-pathnames relative-path root))
         (resolved (handler-case (truename merged) (error () nil))))
    (when resolved
      (let ((resolved-str (namestring resolved))
            (root-str (namestring root)))
        (when (and (>= (length resolved-str) (length root-str))
                   (string= root-str (subseq resolved-str 0 (length root-str))))
          resolved)))))

(defun create-static-dispatcher ()
  "Create a dispatcher that serves static files from the project root."
  (lambda (request)
    (let* ((uri (hunchentoot:request-uri request))
           (path (hunchentoot:url-decode (subseq uri 1))))
      (cond
        ;; Path traversal guard
        ((search ".." path)
         (lambda ()
           (setf (hunchentoot:return-code*) 403)
           "Forbidden"))
        ;; Medicare (default landing page)
        ((or (string= uri "/") (string= uri "/medicare") (string= uri "/medicare.html"))
         (lambda ()
           (hunchentoot:handle-static-file
            (merge-pathnames "medicare.html" *static-root*))))
        ;; Research tool
        ((or (string= uri "/research") (string= uri "/index.html"))
         (lambda ()
           (hunchentoot:handle-static-file
            (merge-pathnames "index.html" *static-root*))))
        ;; Static assets
        ((and (>= (length path) 7) (string= (subseq path 0 7) "static/"))
         (let ((safe (safe-static-path path *static-root*)))
           (if safe
               (lambda () (hunchentoot:handle-static-file safe))
               (lambda () (setf (hunchentoot:return-code*) 403) "Forbidden"))))
        ;; Compiled JS
        ((and (>= (length path) 8) (string= (subseq path 0 8) "runtime/")
              (search ".js" path))
         (let ((safe (safe-static-path (format nil "dist/~A" path) *static-root*)))
           (if safe
               (lambda () (hunchentoot:handle-static-file safe))
               (lambda () (setf (hunchentoot:return-code*) 404) "Not found"))))
        ;; Shen source files (for inspection)
        ((and (>= (length path) 4) (string= (subseq path 0 4) "src/")
              (search ".shen" path))
         (let ((safe (safe-static-path path *static-root*)))
           (if safe
               (lambda ()
                 (setf (hunchentoot:content-type*) "text/plain; charset=utf-8")
                 (hunchentoot:handle-static-file safe))
               (lambda () (setf (hunchentoot:return-code*) 403) "Forbidden"))))))))

;; -------------------------------------------------------------------------
;; Server lifecycle
;; -------------------------------------------------------------------------

(defun start-server (&key (port *port*) (root (truename ".")))
  "Start the Hunchentoot web server."
  (setf *static-root* (ensure-trailing-slash root))
  (setf *shen-root* (merge-pathnames "src/" *static-root*))

  (when *server*
    (hunchentoot:stop *server*)
    (setf *server* nil))

  (setf *server*
        (make-instance 'hunchentoot:easy-acceptor :port port))

  ;; Add static file dispatcher
  (push (create-static-dispatcher) hunchentoot:*dispatch-table*)

  (hunchentoot:start *server*)

  (format t "~%Shen Web Tools server running on http://localhost:~D~%" port)
  (format t "~%Architecture:~%")
  (format t "  Backend:  Common Lisp (SBCL) + Shen~%")
  (format t "  Shen src: ~A~%" *shen-root*)
  (format t "  Frontend: Arrow.js (served from ~A)~%" *static-root*)
  (format t "~%Providers:~%")
  (format t "  Search: ~A~%" *search-provider*)
  (format t "  Fetch:  ~A~%" *fetch-provider*)
  (format t "  AI:     ~A~%" *ai-provider*)
  (format t "~%Pages:~%")
  (format t "  /              — Medicare Plan Finder~%")
  (format t "  /research      — General Research Assistant~%")
  (format t "~%API endpoints:~%")
  (format t "  POST /api/medicare         — Medicare plan lookup (structured)~%")
  (format t "  POST /api/medicare/chat    — Conversational + generative UI~%")
  (format t "  POST /api/medicare/compare — Compare plan types~%")
  (format t "  GET  /api/medicare/plans   — Available plan types~%")
  (format t "  GET  /api/medicare/cache   — Cache stats~%")
  (format t "  POST /api/research         — full research pipeline~%")
  (format t "  POST /api/search           — search only~%")
  (format t "  POST /api/fetch            — fetch only~%")
  (format t "  POST /api/generate         — AI generation only~%")
  (format t "  GET  /api/state            — pipeline state (polling fallback)~%")
  (format t "  GET  /api/stream           — SSE pipeline stream~%")
  *server*)

(defun stop-server ()
  "Stop the web server."
  (when *server*
    (hunchentoot:stop *server*)
    (setf *server* nil)
    (format t "Server stopped.~%")))

(defun ensure-trailing-slash (path)
  "Ensure pathname ends with /."
  (let ((s (namestring path)))
    (if (char= (char s (1- (length s))) #\/)
        (pathname s)
        (pathname (concatenate 'string s "/")))))
