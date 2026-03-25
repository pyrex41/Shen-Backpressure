;;;; backend/load.lisp — Bootstrap: load everything and start the server
;;;;
;;;; Usage:
;;;;   sbcl --load backend/load.lisp
;;;;
;;;; With options:
;;;;   sbcl --load backend/load.lisp --eval '(shen-web-tools::configure :ai :anthropic :api-key "sk-...")'
;;;;   sbcl --load backend/load.lisp --eval '(shen-web-tools::configure :port 8080)'

(in-package :cl-user)

;;; Resolve project root (directory containing this file's parent)
(defvar *project-root*
  (make-pathname :directory (butlast (pathname-directory (or *load-truename*
                                                             *default-pathname-defaults*))))
  "Root directory of the shen-web-tools project.")

(format t "~%========================================~%")
(format t "  Shen Web Tools — CL Backend~%")
(format t "========================================~%~%")
(format t "Project root: ~A~%" *project-root*)

;;; Load system components in order
(let ((*default-pathname-defaults* (merge-pathnames "backend/" *project-root*)))
  (load "packages.lisp")
  (load "bridge.lisp")
  (load "server.lisp")
  (load "medicare.lisp")
  (load "shen-interop.lisp"))

;;; Configure and start
(in-package :shen-web-tools)

(defun configure (&key (port nil port-p) (search nil search-p)
                       (fetch nil fetch-p) (ai nil ai-p)
                       (api-key nil key-p) (model nil model-p)
                       (rho-bin nil rho-p))
  "Configure providers and server options."
  (when port-p (setf *port* port))
  (when search-p (setf *search-provider* search))
  (when fetch-p (setf *fetch-provider* fetch))
  (when ai-p (setf *ai-provider* ai))
  (when key-p (setf *anthropic-api-key* api-key))
  (when model-p (setf *anthropic-model* model))
  (when rho-p (setf *rho-binary* rho-bin))
  (format t "Configuration updated.~%"))

(defun boot ()
  "Full boot sequence: load Shen, register bridge, load sources, start server."
  ;; Check for env vars
  (let ((key (uiop:getenv "ANTHROPIC_API_KEY")))
    (when (and key (> (length key) 0))
      (setf *anthropic-api-key* key)
      (setf *ai-provider* :anthropic)
      (format t "Using Anthropic API (key from environment)~%")))

  ;; Auto-detect rho-cli on PATH
  (when (uiop:run-program (list "which" *rho-binary*) :ignore-error-status t :output nil)
    (format t "rho-cli detected on PATH~%")
    ;; Use rho for search/fetch if no explicit provider set
    (when (eq *search-provider* :mock)
      (setf *search-provider* :duckduckgo)
      (format t "  Auto-selected :duckduckgo for search (same engine as rho)~%"))
    (when (eq *fetch-provider* :mock)
      (setf *fetch-provider* :duckduckgo)
      (format t "  Auto-selected :duckduckgo for fetch~%")))

  ;; Load Shen runtime
  (load-shen-runtime)

  ;; Register CL bridge functions in Shen
  (register-bridge-functions)

  ;; Load application .shen files
  (load-shen-sources)

  ;; Verify specs (Gate 4)
  (verify-shen-specs)

  ;; Start HTTP server
  (start-server :port *port* :root *project-root*)

  ;; Keep SBCL alive
  (format t "~%Server ready. Press Ctrl+C to stop.~%")
  (handler-case
      (bt:join-thread
       (find-if (lambda (th) (search "hunchentoot" (bt:thread-name th)))
                (bt:all-threads)))
    (interrupt-condition ()
      (stop-server))))

;;; Auto-boot
(boot)
