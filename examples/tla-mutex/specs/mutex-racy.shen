\* The same system with the bug everyone writes first: check the lock,
   then take it, in two steps. `shen-derive check` finds the interleaving
   that puts both processes in crit. *\

(define init
  -> [["idle" "idle" false]])

(define proc
  "idle" L -> [(@p "want" L)]
  "want" false -> [(@p "saw-free" false)]
  "want" true -> [(@p "want" true)]
  "saw-free" _ -> [(@p "crit" true)]
  "crit" _ -> [(@p "idle" false)])

(define next
  [P1 P2 L] -> (concat [(map (lambda R (@p "p1" [(fst R) P2 (snd R)])) (proc P1 L))
                        (map (lambda R (@p "p2" [P1 (fst R) (snd R)])) (proc P2 L))]))

(define mutex
  ["crit" "crit" _] -> false
  _ -> true)
