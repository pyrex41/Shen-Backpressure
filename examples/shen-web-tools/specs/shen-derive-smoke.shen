\\ shen-derive-smoke.shen — minimal pure spec used by the shen-derive-ts
\\ smoke test. This file exists to exercise the verification-gate tool
\\ end-to-end against a real TS project; it does not participate in the
\\ shen-web-tools runtime.

(define sum-nonneg
  {(list number) --> number}
  [] -> 0
  [X | Xs] -> (if (>= X 0) (+ X (sum-nonneg Xs)) (sum-nonneg Xs)))
