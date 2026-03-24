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
  "Return current pipeline state (for polling)."
  (json-response (get-pipeline-state)))

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

(defun create-static-dispatcher ()
  "Create a dispatcher that serves static files from the project root."
  (lambda (request)
    (let* ((uri (hunchentoot:request-uri request))
           (path (hunchentoot:url-decode (subseq uri 1))))
      (cond
        ;; Index
        ((or (string= uri "/") (string= uri "/index.html"))
         (lambda ()
           (hunchentoot:handle-static-file
            (merge-pathnames "index.html" *static-root*))))
        ;; Static assets
        ((and (>= (length path) 7) (string= (subseq path 0 7) "static/"))
         (lambda ()
           (hunchentoot:handle-static-file
            (merge-pathnames path *static-root*))))
        ;; Compiled JS
        ((and (>= (length path) 8) (string= (subseq path 0 8) "runtime/")
              (search ".js" path))
         (lambda ()
           (let ((dist-path (merge-pathnames (format nil "dist/~A" path) *static-root*)))
             (if (probe-file dist-path)
                 (hunchentoot:handle-static-file dist-path)
                 (progn
                   (setf (hunchentoot:return-code*) 404)
                   "Not found")))))
        ;; Shen source files (for inspection)
        ((and (>= (length path) 4) (string= (subseq path 0 4) "src/")
              (search ".shen" path))
         (lambda ()
           (setf (hunchentoot:content-type*) "text/plain; charset=utf-8")
           (hunchentoot:handle-static-file
            (merge-pathnames path *static-root*))))))))

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
  (format t "~%API endpoints:~%")
  (format t "  POST /api/research  — full pipeline (Shen orchestrates)~%")
  (format t "  POST /api/search    — search only~%")
  (format t "  POST /api/fetch     — fetch only~%")
  (format t "  POST /api/generate  — AI generation only~%")
  (format t "  GET  /api/state     — pipeline state (for polling)~%")
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
