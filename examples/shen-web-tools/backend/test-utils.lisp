(in-package :shen-web-tools)

(defvar *test-count* 0)
(defvar *test-pass* 0)
(defvar *test-fail* 0)

(defun reset-test-counts ()
  (setf *test-count* 0 *test-pass* 0 *test-fail* 0))

(defmacro assert-equal (expected actual &optional (description ""))
  `(progn
     (incf *test-count*)
     (let ((exp ,expected) (act ,actual))
       (if (equal exp act)
           (incf *test-pass*)
           (progn
             (incf *test-fail*)
             (format t "  FAIL: ~A~%    expected: ~S~%    actual:   ~S~%" ,description exp act))))))

(defmacro assert-true (expr &optional (description ""))
  `(assert-equal t (not (not ,expr)) ,description))

(defmacro assert-nil (expr &optional (description ""))
  `(assert-equal nil ,expr ,description))

(defun run-test-suite (name thunk)
  (format t "~%~A~%" name)
  (funcall thunk))

(defun test-summary ()
  (format t "~%========================================~%")
  (format t "Results: ~D/~D passed (~D failed)~%" *test-pass* *test-count* *test-fail*)
  (format t "========================================~%")
  (zerop *test-fail*))
