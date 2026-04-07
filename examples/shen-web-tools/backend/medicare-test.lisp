(in-package :shen-web-tools)

(defun run-medicare-tests ()
  (run-test-suite "Medicare Tests"
    (lambda ()
      ;; cache-key
      (assert-equal "advantage:33101:" (cache-key "advantage" "33101" "") "cache-key: basic")
      (assert-equal "part-d:10001:insulin" (cache-key "Part-D" "10001" "Insulin") "cache-key: downcases")

      ;; cache-put / cache-get
      (cache-clear)
      (cache-put "advantage" "33101" "" '((:test . "data")))
      (assert-true (cache-get "advantage" "33101" "") "cache-get: hit after put")
      (assert-nil (cache-get "part-d" "33101" "") "cache-get: miss for wrong key")

      ;; cache-clear
      (cache-put "part-d" "10001" "" '((:test . "more")))
      (cache-clear)
      (assert-nil (cache-get "advantage" "33101" "") "cache-clear: empties cache")
      (assert-nil (cache-get "part-d" "10001" "") "cache-clear: empties all")

      ;; plan-type-label
      (assert-equal "Medicare Advantage (Part C)" (plan-type-label "advantage") "plan-label: advantage")
      (assert-equal "Medicare Part D (Prescription Drug)" (plan-type-label "part-d") "plan-label: part-d")
      (assert-equal "Medicare unknown" (plan-type-label "unknown") "plan-label: unknown type")

      ;; extract-json-from-response — balanced brace matching
      (assert-equal "{\"key\":\"val\"}" (extract-json-from-response "Here is JSON: {\"key\":\"val\"} done") "extract-json: basic")
      (assert-equal "{\"a\":{\"b\":1}}" (extract-json-from-response "{\"a\":{\"b\":1}}") "extract-json: nested")
      (assert-equal "{}" (extract-json-from-response "no json here") "extract-json: no json")
      (assert-equal "{\"key\":\"val\"}" (extract-json-from-response "```json
{\"key\":\"val\"}
```
More text}") "extract-json: markdown fenced")
      (assert-equal "{\"x\":\"a}b\"}" (extract-json-from-response "{\"x\":\"a}b\"}") "extract-json: brace in string")
      )))

(run-medicare-tests)
