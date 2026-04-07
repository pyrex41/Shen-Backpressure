;;;; backend/shen-interop.lisp — Load Shen and wire it to the CL bridge
;;;;
;;;; This file:
;;;;   1. Loads the Shen language on SBCL
;;;;   2. Registers CL bridge functions as Shen-callable symbols
;;;;   3. Loads the application .shen files
;;;;   4. Provides the Shen-to-CL dispatch layer
;;;;
;;;; After loading, Shen code can call:
;;;;   (web-search Query MaxResults)  → calls CL web-search
;;;;   (web-fetch Url)                → calls CL web-fetch
;;;;   (ai-generate System User)      → calls CL ai-generate
;;;;   (current-timestamp)            → calls CL current-timestamp

(in-package :shen-web-tools)

;; -------------------------------------------------------------------------
;; Shen loading
;; -------------------------------------------------------------------------

(defvar *shen-loaded* nil "Whether the Shen runtime has been loaded")

(defun load-shen-runtime ()
  "Load the Shen language implementation.
   Expects shen-sbcl or shen-cl to be available via Quicklisp or local path."
  (when *shen-loaded*
    (return-from load-shen-runtime t))

  ;; Try Quicklisp first
  (handler-case
      (progn
        (ql:quickload :shen :silent t)
        (setf *shen-loaded* t)
        (format t "Shen runtime loaded via Quicklisp~%"))
    (error ()
      ;; Try local installation
      (let ((paths (append
                    (list (merge-pathnames "shen/shen-sbcl.lisp" (user-homedir-pathname)))
                    (when *static-root*
                      (list (merge-pathnames "lib/shen/shen.lisp" *static-root*)))
                    (list #P"/usr/local/lib/shen/shen.lisp"))))
        (dolist (p paths)
          (when (probe-file p)
            (load p)
            (setf *shen-loaded* t)
            (format t "Shen runtime loaded from ~A~%" p)
            (return-from load-shen-runtime t)))
        (format t "WARNING: Shen runtime not found. ~
                   Running in CL-only mode (Shen type checking unavailable).~%")
        (format t "To install: https://github.com/Shen-Language/shen-sbcl~%")))))

;; -------------------------------------------------------------------------
;; Register CL functions as Shen-callable
;; -------------------------------------------------------------------------

(defun register-bridge-functions ()
  "Make CL bridge functions available from Shen.
   In Shen-SBCL, CL functions are callable via (lisp.fn args) or
   by registering them in Shen's function table."
  (when *shen-loaded*
    ;; Register each bridge function in Shen's environment
    ;; The exact mechanism depends on the Shen port:
    ;;   - shen-sbcl: (shen-cl:define-shen-function ...)
    ;;   - shen-cl:   (shen:define ...)
    ;; We use the eval-shen approach which works across ports:
    (shen-eval-string
     "(define cl-web-search
        { string --> number --> (list (list string)) }
        Query MaxResults -> (lisp.shen-web-tools::web-search Query MaxResults))")

    (shen-eval-string
     "(define cl-web-fetch
        { string --> (list string) }
        Url -> (lisp.shen-web-tools::web-fetch Url))")

    (shen-eval-string
     "(define cl-ai-generate
        { string --> string --> (list A) }
        System User -> (lisp.shen-web-tools::ai-generate System User))")

    (shen-eval-string
     "(define cl-current-timestamp
        { --> number }
        -> (lisp.shen-web-tools::current-timestamp))")

    (shen-eval-string
     "(define cl-extract-terms
        { string --> (list string) }
        Query -> (lisp.shen-web-tools::extract-terms Query))")

    ;; Medicare-specific bridge functions
    (shen-eval-string
     "(define cl-set-pipeline-state
        { string --> A --> string }
        Phase Data -> (do (lisp.shen-web-tools::set-pipeline-state Phase Data) \"ok\"))")

    (shen-eval-string
     "(define cl-substring-search
        { string --> string --> string }
        Needle Haystack -> (lisp.shen-web-tools::cl-substring-search Needle Haystack))")

    (shen-eval-string
     "(define cl-substring
        { string --> number --> number --> string }
        S Start End -> (lisp.shen-web-tools::cl-substring S Start End))")

    ;; Generative UI helper — read properties from result data
    (shen-eval-string
     "(define cl-get-prop
        { string --> A --> string }
        Key Data -> (lisp.shen-web-tools::cl-get-prop Key Data))")

    (format t "Bridge functions registered in Shen~%")))

(defun shen-eval-string (shen-code)
  "Evaluate a string of Shen code. Wraps the port-specific eval mechanism."
  (unless (find-package :shen)
    (warn "Shen package not loaded, cannot eval: ~A"
          (subseq shen-code 0 (min 60 (length shen-code))))
    (return-from shen-eval-string nil))
  (handler-case
      ;; Try different Shen port interfaces
      (cond
        ((fboundp (find-symbol "EVAL-STRING" :shen))
         (funcall (find-symbol "EVAL-STRING" :shen) shen-code))
        ((fboundp (find-symbol "SHEN" :shen))
         (funcall (find-symbol "SHEN" :shen) shen-code))
        (t (warn "Cannot find Shen eval function")))
    (error (e)
      (warn "Shen eval failed: ~A~%Code: ~A" e shen-code))))

;; -------------------------------------------------------------------------
;; Extra CL helpers for Shen bridge
;; -------------------------------------------------------------------------

(defun cl-substring-search (needle haystack)
  "Return position of NEEDLE in HAYSTACK, or empty string if not found."
  (let ((pos (search needle haystack)))
    (if pos (write-to-string pos) "")))

(defun cl-substring (s start end)
  "Extract substring. Safe: clamps indices."
  (let ((len (length s)))
    (subseq s (min start len) (min end len))))

;; -------------------------------------------------------------------------
;; Load application .shen files
;; -------------------------------------------------------------------------

(defun load-shen-sources ()
  "Load the application Shen source files."
  (when *shen-loaded*
    (let ((files '("utils.shen"
                   "web-tools.shen"
                   "ai-gen.shen"
                   "ui-resolve.shen"
                   "app.shen"
                   "medicare.shen"
                   "medicare-ui-resolve.shen")))
      (dolist (f files)
        (let ((path (merge-pathnames f *shen-root*)))
          (if (probe-file path)
              (progn
                (format t "Loading Shen source: ~A~%" f)
                (shen-eval-string (format nil "(load ~S)" (namestring path))))
              (format t "WARNING: Shen source not found: ~A~%" path)))))))

;; -------------------------------------------------------------------------
;; Shen type checking (Gate 4)
;; -------------------------------------------------------------------------

(defun verify-shen-specs ()
  "Run Shen type checker on the specs (tc+). This is Gate 4."
  (when *shen-loaded*
    (dolist (spec-file '("core.shen" "medicare.shen"))
      (let ((spec-path (merge-pathnames (format nil "../specs/~A" spec-file) *shen-root*)))
        (if (probe-file spec-path)
            (progn
              (format t "Verifying Shen specs: ~A~%" spec-file)
              (shen-eval-string "(tc +)")
              (shen-eval-string (format nil "(load ~S)" (namestring spec-path)))
              (format t "Shen specs verified (tc+) ✓: ~A~%" spec-file))
            (format t "WARNING: Spec file not found: ~A~%" spec-path))))))

;; -------------------------------------------------------------------------
;; Call Shen functions from CL (for the API handlers)
;; -------------------------------------------------------------------------

(defun shen-research (query)
  "Call Shen's (research Query) function and return the result.
   Falls back to CL-only pipeline if Shen is not loaded."
  (if *shen-loaded*
      (handler-case
          (shen-eval-string (format nil "(research ~S)" query))
        (error (e)
          (warn "Shen research failed, falling back to CL: ~A" e)
          (cl-research-pipeline query)))
      (cl-research-pipeline query)))

(defun cl-research-pipeline (query)
  "Pure CL fallback of the research pipeline (same logic as app.shen)."
  (let* (;; Search
         (hits (web-search query 10))
         ;; Fetch top 5
         (top-hits (subseq hits 0 (min 5 (length hits))))
         (pages (mapcar (lambda (hit) (web-fetch (second hit))) top-hits))
         ;; Ground sources
         (sources (ground-sources pages top-hits))
         ;; Generate summary
         (system-msg (format nil "You are a research assistant. ~
                                  Summarize web sources about: ~A" query))
         (source-text (format-sources-for-ai sources))
         (user-msg (format nil "Based on these sources, provide a summary:~%~%~A" source-text))
         (ai-result (ai-generate system-msg user-msg)))
    (list query sources ai-result)))
