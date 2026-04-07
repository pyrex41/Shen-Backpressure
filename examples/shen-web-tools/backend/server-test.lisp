(in-package :shen-web-tools)

(defun run-server-tests ()
  (run-test-suite "Server Tests"
    (lambda ()
      ;; ensure-trailing-slash
      (assert-equal #P"/tmp/foo/" (ensure-trailing-slash "/tmp/foo") "trailing-slash: adds /")
      (assert-equal #P"/tmp/foo/" (ensure-trailing-slash "/tmp/foo/") "trailing-slash: already has /")

      ;; safe-static-path — use /tmp/ as root for testing
      ;; Traversal attempt should return nil
      (assert-nil (safe-static-path "../etc/passwd" #P"/tmp/") "safe-path: traversal blocked")
      (assert-nil (safe-static-path "../../etc/passwd" #P"/tmp/") "safe-path: double traversal blocked")
      ;; Note: valid paths that actually exist would return the resolved path,
      ;; but we can't test with guaranteed existing files. The nil on nonexistent is fine.
      )))

(run-server-tests)
