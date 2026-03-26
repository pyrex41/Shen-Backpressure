\\ src/utils.shen — Shared utility functions
\\ Loaded FIRST, before all other .shen files

(define take
  \doc "Take first N elements from a list."
  0 _ -> []
  _ [] -> []
  N [H | T] -> [H | (take (- N 1) T)])

(define filter
  \doc "Filter list by predicate."
  _ [] -> []
  F [H | T] -> (if (F H) [H | (filter F T)] (filter F T)))

(define string-length
  \doc "Length of a string."
  "" -> 0
  S -> (+ 1 (string-length (tlstr S))))

(define substring
  \doc "Extract substring from Start to End via CL interop."
  S Start End -> (cl-substring S Start End))

(define truncate-text
  \doc "Truncate text to MaxLen characters."
  Text MaxLen ->
    (if (<= (string-length Text) MaxLen)
        Text
        (cn (substring Text 0 MaxLen) "...")))
