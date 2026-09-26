\* Two processes share one lock. This is the spec the implementation in
   impl/ must refine: a behaviour is a sequence of states, init says where
   behaviours start, next says which steps are allowed, and the invariants
   say which states are acceptable.

   A state is [Pc1 Pc2 Lock]: each process's program counter and whether
   the lock is held. A process goes idle -> want -> crit -> idle; it may
   only enter crit by taking the lock in one atomic step. *\

(define init
  -> [["idle" "idle" false]])

\* One process's moves, given its pc and the lock: a list of
   (@p NewPc NewLock). *\
(define proc
  "idle" L -> [(@p "want" L)]
  "want" false -> [(@p "crit" true)]
  "want" true -> []
  "crit" _ -> [(@p "idle" false)])

\* Either process may move. Each successor is labelled with the process
   that moved, so a recorded trace can say who moved and be held to it. *\
(define next
  [P1 P2 L] -> (concat [(map (lambda R (@p "p1" [(fst R) P2 (snd R)])) (proc P1 L))
                        (map (lambda R (@p "p2" [P1 (fst R) (snd R)])) (proc P2 L))]))

\* Safety: never both in the critical section. *\
(define mutex
  ["crit" "crit" _] -> false
  _ -> true)

\* The lock is held exactly when someone is in the critical section. *\
(define lock-matches-crit
  [P1 P2 L] -> (= L (or (= P1 "crit") (= P2 "crit"))))

\* Step invariant: only the process leaving crit may release the lock. *\
(define release-by-holder
  [P1 P2 true] [Q1 Q2 false] -> (or (and (= P1 "crit") (= Q1 "idle"))
                                    (and (= P2 "crit") (= Q2 "idle")))
  _ _ -> true)
