\* ====================================================================
   sb/flow/stdlib.shen — flow rules for Shen's embedded Prolog.

   This is the primary engine for flow premises: the rules that decide
   whether a (flow …) premise holds are written once, in Shen, over the
   fact vocabulary `sb index` emits. The same rule text runs against a
   Go tree and a TypeScript tree, because the facts are the resolved
   symbol graph and carry no language in them.

   USAGE

     (load "sb/flow/stdlib.shen")          \* rules + fact tables *\
     (load ".sb/facts.shen")               \* the generated facts *\
     (prolog? (unsanctioned-caller Ctor Caller))
     (prolog? (must-pass-through-violation Src Proof Sink Path))

   The fact file is a flat list of forms:

     (def sym file start-line start-col end-line end-col)
     (ref sym file line col enclosing-def-sym)
     (call caller-sym callee-sym)

   `def`, `ref` and `call` are defined below as ordinary Shen functions
   that push onto three globals, so *loading the fact file is asserting
   the facts*. That is why no parser ships with this file.

   ENGINE AVAILABILITY

   Evaluating these rules needs a Shen host (SBCL plus shen, i.e. the
   same `shen-sbcl` that gate 4's bin/shen-check.sh uses). Where no
   Shen host is installed, `sb flow` runs an equivalent evaluator
   written in Go (cmd/sb/flow), and says which engine ran in both its
   output and the discharge report's rationale. The two engines are
   two implementations of the rules below; docs/FLOW.md states the
   correspondence premise by premise and names it as a trust
   assumption.

   WHAT THE TESTS COVER

   cmd/sb/flow/stdlib_test.go checks this file structurally — balanced
   forms, every Prolog clause terminated, every predicate this file
   promises actually defined — and, when `shen-sbcl` is on PATH, loads
   it together with a hand-written fact fixture and checks the four
   query outcomes. In an environment with no Shen host the execution
   half skips and says so.

   TERMINATION

   `reaches/2` is right-recursive over the call graph and will not
   terminate on a cyclic graph without the visited-set argument, so the
   searching predicates below all thread one (`path-to-sink/5`). A
   monorepo-scale fact base will outgrow Shen's Prolog; the documented
   escape hatch is to emit Soufflé from this same rule text.
   ==================================================================== *\

\* --- Fact tables ---------------------------------------------------- *\

(set *flow-defs* [])
(set *flow-refs* [])
(set *flow-calls* [])

\* Loading .sb/facts.shen calls these. Each returns the fact it
   asserted so a fact file is also a readable transcript. *\

(define def
  Sym File L0 C0 L1 C1 -> (do (set *flow-defs* [[Sym File L0 C0 L1 C1] | (value *flow-defs*)])
                              [Sym File L0 C0 L1 C1]))

(define ref
  Sym File L C Encl -> (do (set *flow-refs* [[Sym File L C Encl] | (value *flow-refs*)])
                           [Sym File L C Encl]))

(define call
  Caller Callee -> (do (set *flow-calls* [[Caller Callee] | (value *flow-calls*)])
                       [Caller Callee]))

\* --- Symbol canonicalisation ---------------------------------------
   A SCIP symbol is "<scheme> <manager> <package> <version> <descriptors>".
   The four header fields carry a version hash, so a premise must never
   match against them: `canonical` drops them, strips the backtick
   quoting, and drops the trailing descriptor punctuation. Mirrors
   flow.Canonical in cmd/sb/flow/pattern.go. *\

(define canonical
  Sym -> (drop-descriptor-tail (strip-ticks (descriptor-part Sym 0))))

(define descriptor-part
  Sym 4 -> Sym
  Sym N -> Sym where (= Sym "")
  Sym N -> (descriptor-part (tlstr Sym) (+ N 1)) where (= (pos Sym 0) " ")
  Sym N -> (descriptor-part (tlstr Sym) N))

(define strip-ticks
  "" -> ""
  S -> (strip-ticks (tlstr S)) where (= (pos S 0) "`")
  S -> (@s (pos S 0) (strip-ticks (tlstr S))))

\* Drops a trailing "()." , "." , "#" or "/" — the descriptor suffix
   that says "method", "term", "type" or "namespace". *\
(define drop-descriptor-tail
  S -> (drop-descriptor-tail (butlast-str S)) where (ends-with? S "().")
  S -> (drop-descriptor-tail (butlast-str S)) where (ends-with? S ".")
  S -> (drop-descriptor-tail (butlast-str S)) where (ends-with? S "#")
  S -> (drop-descriptor-tail (butlast-str S)) where (ends-with? S "/")
  S -> S)

(define ends-with?
  S Suffix -> (str-suffix? S Suffix (- (str-len S) (str-len Suffix))))

(define str-suffix?
  S Suffix N -> false where (< N 0)
  S Suffix 0 -> (= S Suffix)
  S Suffix N -> (str-suffix? (tlstr S) Suffix (- N 1)))

(define str-len
  "" -> 0
  S -> (+ 1 (str-len (tlstr S))))

(define butlast-str
  S -> (take-str S (- (str-len S) 1)))

(define take-str
  S 0 -> ""
  S N -> (@s (pos S 0) (take-str (tlstr S) (- N 1))))

\* --- Pattern matching ----------------------------------------------
   A pattern matches a canonical symbol when it matches the whole
   symbol or a suffix of it beginning at a "/" or "#" boundary, with
   "*" matching any run of characters. Mirrors flow.Pattern.Matches. *\

(define matches?
  Pattern Sym -> (match-at-boundary? Pattern (canonical Sym) true))

(define match-at-boundary?
  Pattern S Start -> true where (and Start (glob? Pattern S))
  Pattern "" _ -> false
  Pattern S _ -> (match-at-boundary? Pattern (tlstr S) (boundary? (pos S 0))))

(define boundary?
  C -> (or (= C "/") (= C "#")))

\* Textbook backtracking glob: "*" consumes nothing or one character
   and recurses. *\
(define glob?
  "" "" -> true
  "*" _ -> true
  Pattern "" -> (glob? (tlstr Pattern) "") where (= (pos Pattern 0) "*")
  Pattern "" -> false
  Pattern S -> (or (glob? (tlstr Pattern) S) (glob? Pattern (tlstr S)))
               where (= (pos Pattern 0) "*")
  Pattern S -> (glob? (tlstr Pattern) (tlstr S))
               where (= (pos Pattern 0) (pos S 0))
  _ _ -> false)

\* --- Fact access as Prolog predicates -------------------------------
   memberp/2 is spelled out rather than taken from a library so this
   file loads into a bare Shen with no prelude beyond the kernel. *\

(defprolog memberp
  X [X | _] <--;
  X [_ | Rest] <-- (memberp X Rest);)

(defprolog flow-def
  Sym File L0 C0 L1 C1 <-- (is Ds (value *flow-defs*))
                           (memberp [Sym File L0 C0 L1 C1] Ds);)

(defprolog flow-ref
  Sym File L C Encl <-- (is Rs (value *flow-refs*))
                        (memberp [Sym File L C Encl] Rs);)

(defprolog flow-call
  Caller Callee <-- (is Cs (value *flow-calls*))
                    (memberp [Caller Callee] Cs);)

\* --- reaches/2 -------------------------------------------------------
   Reflexive-transitive closure of the call graph. The plain form is
   the statement of the relation; every search below uses the
   visited-set form so a cyclic call graph terminates. *\

(defprolog reaches
  X X <--;
  X Z <-- (reaches-acc X Z [X]);)

(defprolog reaches-acc
  X X _ <--;
  X Z Seen <-- (flow-call X Y)
               (fresh? Y Seen)
               (reaches-acc Y Z [Y | Seen]);)

(defprolog fresh?
  X Seen <-- (is Ok (not (element? X Seen))) (when Ok);)

\* --- unsanctioned-caller/2 ------------------------------------------
   The lowering of (constructor-only Ctor Allowed…): a reference to a
   symbol matching Ctor whose enclosing definition matches none of the
   allowed callers.

   `allowed-caller?` is supplied by the project — sb generates it from
   the premise's allowed-caller patterns before the query runs:

     (define allowed-caller?
       Ctor Caller -> (or (matches? "internal/verified/CheckTenantAccess" Caller)
                          (matches? "cmd/cedar-verify/computeGuardAllow" Caller)))

   `ctor-pattern` is likewise generated, holding the premise's
   constructor pattern. Keeping the allowlist a Shen function rather
   than a Prolog predicate keeps negation out of the search: the
   search is a pure conjunction, and "not allowed" is a boolean test.

   Note what this premise does *not* need: the spelling of the import.
   The reference in the fact base is already resolved to the
   constructor's symbol, so an aliased import and a direct one are the
   same fact. That is the whole difference from a grep gate. *\

(defprolog unsanctioned-caller
  Ctor Caller File Line <-- (flow-ref Sym File Line _ Caller)
                            (is Hit (matches? Ctor Sym)) (when Hit)
                            (is Bad (not (allowed-caller? Ctor Caller))) (when Bad);)

\* --- must-pass-through violation search -----------------------------
   The lowering of (must-pass-through Src Proof Sink): for every
   definition matching Src, no call path may reach a call of Sink
   without some definition on the path referencing Proof. With the
   guard constructor as the declassifier this is noninterference.

   The search prunes at a declassifier: once a definition on the path
   references Proof, everything downstream of it is sanctioned and is
   not explored. Path is returned innermost-last, so reversing it
   gives the shortest violating path the report prints. *\

(defprolog references-proof?
  Sym Proof <-- (flow-ref Ref _ _ _ Sym)
                (is Hit (matches? Proof Ref)) (when Hit);)

(defprolog source-def
  Src Sym <-- (flow-def Sym _ _ _ _ _)
              (is Hit (matches? Src Sym)) (when Hit)
              (is Callable (callable? Sym)) (when Callable);)

(defprolog must-pass-through-violation
  Src Proof Sink Path <-- (source-def Src Sym)
                          (unsanctioned-source Sym Proof)
                          (path-to-sink Sym Proof Sink [Sym] Path);)

\* A source that already holds the proof is sanctioned outright. *\
(defprolog unsanctioned-source
  Sym Proof <-- (is Held (not (prolog? (references-proof? Sym Proof)))) (when Held);)

\* Either this definition calls the sink directly, or it calls some
   unsanctioned callee that does. *\
(defprolog path-to-sink
  Sym _ Sink Seen Seen <-- (flow-ref Callee _ _ _ Sym)
                           (is Hit (matches? Sink Callee)) (when Hit);
  Sym Proof Sink Seen Path <-- (flow-call Sym Next)
                               (fresh? Next Seen)
                               (unsanctioned-source Next Proof)
                               (path-to-sink Next Proof Sink [Next | Seen] Path);)

\* `callable?` is the one syntactic test in the vocabulary: SCIP spells
   a method or function descriptor with a trailing "().", and that is
   part of the protocol rather than of any language. *\
(define callable?
  Sym -> (ends-with? Sym "()."))
