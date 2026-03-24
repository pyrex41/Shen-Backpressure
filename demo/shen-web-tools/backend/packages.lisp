;;;; backend/packages.lisp — System definition for Shen Web Tools
;;;;
;;;; Loads Quicklisp dependencies:
;;;;   - hunchentoot  (HTTP server)
;;;;   - dexador      (HTTP client for web fetch/search)
;;;;   - cl-json      (JSON encode/decode)
;;;;   - bordeaux-threads (threading)

(in-package :cl-user)

;;; Ensure Quicklisp is loaded
(unless (find-package :ql)
  (let ((setup (merge-pathnames "quicklisp/setup.lisp" (user-homedir-pathname))))
    (if (probe-file setup)
        (load setup)
        (error "Quicklisp not found. Run: curl -O https://beta.quicklisp.org/quicklisp.lisp && sbcl --load quicklisp.lisp --eval '(quicklisp-quickstart:install)' --quit"))))

;;; Load dependencies
(ql:quickload '(:hunchentoot :dexador :cl-json :bordeaux-threads) :silent t)

;;; Define our package
(defpackage :shen-web-tools
  (:use :cl)
  (:export #:start-server
           #:stop-server
           #:web-search
           #:web-fetch
           #:ai-generate))

(in-package :shen-web-tools)

(defvar *server* nil "Hunchentoot acceptor instance")
(defvar *port* 3000 "Server port")
(defvar *static-root* nil "Path to static files")
(defvar *shen-root* nil "Path to Shen source files")

;;; Provider configuration
(defvar *search-provider* :mock "Search provider: :mock or :live")
(defvar *fetch-provider* :mock "Fetch provider: :mock or :live")
(defvar *ai-provider* :mock "AI provider: :mock or :anthropic")
(defvar *anthropic-api-key* nil "Anthropic API key")
(defvar *anthropic-model* "claude-sonnet-4-6" "Anthropic model ID")
