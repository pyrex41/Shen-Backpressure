\* Leader election among three computers a, b and c, after the running
   example in Reasonable's "The internet discovers TLA+. Now what?".

   A state is [Roles Votes]. Roles[i] is "follower", "candidate" or
   "leader". Votes[i][j] is true when computer i has voted for j.

   Actions:
     start I     a follower that has not voted becomes a candidate and
                 votes for itself
     I votes J   a computer that has not voted votes for candidate J
     I wins      a candidate with a majority (2 of 3) becomes leader

     timeout     (only with timeouts? on) when the vote has split and no
                 one can win, everyone forgets their vote and starts over

   Safety:   [] (never two leaders)                  --inv one-leader
   Liveness: <> (someone is leader)                   --eventually has-leader
   CTL:      AG EF (an election can still be started) --possible can-start

   The constants below are the model's knobs; `--const name=expr`
   overrides one for a run, like a TLC model config. *\

(define names -> ["a" "b" "c"])
(define ids -> [0 1 2])
(define quorum -> 2)
(define double-vote? -> false)   \* the bug: a computer may vote twice *\
(define timeouts? -> false)      \* split votes are retried *\

(define init
  -> [[["follower" "follower" "follower"]
       [[false false false] [false false false] [false false false]]]])

\* --- list helpers --- *\

(define nth
  0 [X | _] -> X
  N [_ | Xs] -> (nth (- N 1) Xs))

(define set-nth
  0 V [_ | Xs] -> [V | Xs]
  N V [X | Xs] -> [X | (set-nth (- N 1) V Xs)])

(define any?
  Xs -> (foldr (lambda X (lambda Acc (or X Acc))) false Xs))

(define count
  X Xs -> (foldr (lambda Y (lambda Acc (if (= X Y) (+ Acc 1) Acc))) 0 Xs))

(define name I -> (nth I (names)))

\* --- votes --- *\

(define has-voted? I Votes -> (any? (nth I Votes)))

(define may-vote?
  I J Votes -> (if (double-vote?)
                   (not (nth J (nth I Votes)))
                   (not (has-voted? I Votes))))

(define votes-for
  J Votes -> (count true (map (lambda Row (nth J Row)) Votes)))

(define cast I J Votes -> (set-nth I (set-nth J true (nth I Votes)) Votes))

\* --- actions: each returns a list of labelled successors --- *\

(define start
  I [Roles Votes] -> [(@p (cn "start " (name I))
                          [(set-nth I "candidate" Roles) (cast I I Votes)])]
                     where (and (= (nth I Roles) "follower") (not (has-voted? I Votes)))
  _ _ -> [])

(define vote
  I J [Roles Votes] -> [(@p (cn (name I) (cn " votes " (name J)))
                            [Roles (cast I J Votes)])]
                       where (and (not (= I J))
                                  (and (= (nth J Roles) "candidate")
                                       (may-vote? I J Votes)))
  _ _ _ -> [])

(define win
  I [Roles Votes] -> [(@p (cn (name I) " wins") [(set-nth I "leader" Roles) Votes])]
                     where (and (= (nth I Roles) "candidate") (>= (votes-for I Votes) (quorum)))
  _ _ -> [])

(define all-voted? Votes -> (not (any? (map (lambda I (not (has-voted? I Votes))) (ids)))))

(define timeout
  [Roles Votes] -> (init)
                   where (and (timeouts?)
                              (and (all-voted? Votes)
                                   (= (for-each (lambda I (win I [Roles Votes]))) [])))
  _ -> [])

(define for-each
  F -> (concat (map F (ids))))

(define next
  S -> (concat [(for-each (lambda I (start I S)))
                (for-each (lambda I (for-each (lambda J (vote I J S)))))
                (for-each (lambda I (win I S)))
                (map (lambda S2 (@p "timeout" S2)) (timeout S))]))

\* --- properties --- *\

(define one-leader [Roles _] -> (<= (count "leader" Roles) 1))

(define has-leader [Roles _] -> (> (count "leader" Roles) 0))

(define can-start S -> (any? (map (lambda I (not (= (start I S) []))) (ids))))
