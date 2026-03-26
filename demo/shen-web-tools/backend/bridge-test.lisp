(in-package :shen-web-tools)

(defun run-bridge-tests ()
  (run-test-suite "Bridge Tests"
    (lambda ()
      ;; url-encode-form
      (assert-equal "hello+world" (url-encode-form "hello world") "url-encode: spaces become +")
      (assert-equal "a%26b%3Dc" (url-encode-form "a&b=c") "url-encode: special chars")
      (assert-equal "hello" (url-encode-form "hello") "url-encode: no encoding needed")
      (assert-equal "" (url-encode-form "") "url-encode: empty string")
      ;; UTF-8 encoding: e-acute (U+00E9) should become %C3%A9
      (assert-equal "caf%C3%A9"
                    (url-encode-form (format nil "caf~A" (string (code-char #x00E9))))
                    "url-encode: non-ASCII UTF-8")

      ;; url-decode
      (assert-equal "hello world" (url-decode "hello+world") "url-decode: + to space")
      (assert-equal "a&b" (url-decode "a%26b") "url-decode: percent decode")
      (assert-equal "hello" (url-decode "hello") "url-decode: no decoding needed")

      ;; strip-html-tags
      (assert-equal " bold " (strip-html-tags "<b>bold</b>") "strip-html: basic tags")
      (assert-equal "a & b" (strip-html-tags "a &amp; b") "strip-html: entity decode &amp;")
      (assert-equal "a < b" (strip-html-tags "a &lt; b") "strip-html: entity decode &lt;")

      ;; collapse-whitespace
      (assert-equal "a b c" (collapse-whitespace "a   b   c") "collapse-ws: multiple spaces")
      (assert-equal "hello" (collapse-whitespace "  hello  ") "collapse-ws: trim")
      (assert-equal "" (collapse-whitespace "   ") "collapse-ws: all whitespace")

      ;; replace-all
      (assert-equal "b-b-b" (replace-all "a-a-a" "a" "b") "replace-all: multiple")
      (assert-equal "hello" (replace-all "hello" "xyz" "abc") "replace-all: no match")
      (assert-equal "" (replace-all "" "a" "b") "replace-all: empty string")

      ;; split-string
      (assert-equal '("a" "b" "c") (split-string "a,b,c" #\,) "split-string: basic")
      (assert-equal '("hello") (split-string "hello" #\,) "split-string: no delimiter")

      ;; char-to-utf8-bytes
      (assert-equal '(97) (char-to-utf8-bytes #\a) "utf8-bytes: ASCII")
      (assert-equal '(195 169) (char-to-utf8-bytes (code-char #x00E9)) "utf8-bytes: e-acute")
      )))

(run-bridge-tests)
