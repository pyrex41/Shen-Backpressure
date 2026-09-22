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
     (calls caller-sym callee-sym)

   `def`, `ref` and `calls` are defined below as ordinary Shen
   functions that push onto three globals, so *loading the fact file is
   asserting the facts*. That is why no parser ships with this file.

   WHY `calls` AND NOT `call` (W6)

   `call` is a Shen system function of arity 5. `(define call …)` here
   was therefore rejected outright — "call is a system function" — so
   this file could not be loaded at all and these rules had never
   executed, in any workstream, despite being described throughout the
   documentation as the primary engine. The predicate is `calls`
   everywhere now: here, in the facts `sb index` writes, in the Go
   engine, in the fixtures and in docs/FLOW.md. `sb index`'s reader
   still accepts a `(call …)` fact so a stale cached fact file loads.

   ENGINE AVAILABILITY

   Evaluating these rules needs a Shen host — any port. `make shen-go`
   at the repo root builds the Go port into bin/shen, which is what
   this repository's own runs use; shen-sbcl and shen-scheme work
   identically. `sb flow --engine` selects: `shen` runs these rules,
   `go` runs the equivalent evaluator in cmd/sb/flow, and `both` (the
   default when a host is present) runs each and fails if their
   verdicts differ. The discharge report records which ran in
   `flow_engine` and in each premise's rationale.

   WHAT THE TESTS COVER

   cmd/sb/flow/stdlib_test.go checks this file structurally — balanced
   forms, every Prolog clause terminated, every predicate this file
   promises actually defined — and, whenever a Shen host is
   resolvable, loads it together with a hand-written fact fixture and
   checks the query outcomes. That execution half used to be behind an
   SB_FLOW_SHEN opt-in, which is the other reason nobody noticed the
   file did not load; it now runs on the presence of a host and skips
   with a reason when there is none.

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

(define calls
  Caller Callee -> (do (set *flow-calls* [[Caller Callee] | (value *flow-calls*)])
                       [Caller Callee]))

\* --- Symbol canonicalisation ---------------------------------------
   A SCIP symbol is "<scheme> <manager> <package> <version> <descriptors>".
   The four header fields carry a version hash, so a premise must never
   match against them: `canonical` drops them, strips the backtick
   quoting, and drops the trailing descriptor punctuation. Mirrors
   flow.Canonical in cmd/sb/flow/pattern.go. *\

(define canonical
  Sym -> (drop-descriptor-tail (strip-ticks (descriptor-part Sym 0 false))))

\* Counts four unquoted spaces and returns what follows the fourth, so
   a package name containing a space inside backticks does not shift
   the field count. The tick flag is the third argument; dropping it
   was the third divergence from flow.descriptorPart that running this
   file exposed. *\
(define descriptor-part
  Sym 4 _ -> Sym
  Sym N T -> Sym where (= Sym "")
  Sym N T -> (descriptor-part (tlstr Sym) N (not T)) where (= (pos Sym 0) "`")
  Sym N true -> (descriptor-part (tlstr Sym) N true) where (= (pos Sym 0) " ")
  Sym N T -> (descriptor-part (tlstr Sym) (+ N 1) T) where (= (pos Sym 0) " ")
  Sym N T -> (descriptor-part (tlstr Sym) N T))

(define strip-ticks
  "" -> ""
  S -> (strip-ticks (tlstr S)) where (= (pos S 0) "`")
  S -> (@s (pos S 0) (strip-ticks (tlstr S))))

\* Drops the whole "()." method suffix, then any trailing run of "."
   "#" or "/" — the descriptor punctuation that says "term", "type" or
   "namespace". Mirrors flow.Canonical, which does exactly
   TrimSuffix("().") followed by TrimRight("./#").

   The pre-W6 version dropped one character per guard, so a method
   symbol canonicalised to "pkg/NewAccess()" instead of
   "pkg/NewAccess" and no premise pattern ever matched anything. Every
   Shen-engine verdict would have been a silent vacuous pass. That is
   the kind of bug an unexecuted primary engine hides. *\
(define drop-descriptor-tail
  S -> (drop-punctuation-tail (drop-call-suffix S)))

(define drop-call-suffix
  S -> (take-str S (- (str-len S) 3)) where (ends-with? S "().")
  S -> S)

(define drop-punctuation-tail
  "" -> ""
  S -> (drop-punctuation-tail (butlast-str S)) where (ends-with? S ".")
  S -> (drop-punctuation-tail (butlast-str S)) where (ends-with? S "#")
  S -> (drop-punctuation-tail (butlast-str S)) where (ends-with? S "/")
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
   and recurses.

   The `"" _ -> false` clause is not redundant with the last clause,
   and leaving it out is the second bug W6 found by running this file
   for the first time: with the pattern exhausted and input left over —
   `glob? "" "Foo"`, reached from the character-match clause whenever a
   pattern is a strict prefix of a symbol — the `*` clause's guard
   evaluated `(pos "" 0)` and the host aborted the whole run with
   "0 is not valid index for". Every real fact base hits that case. *\
(define glob?
  "" "" -> true
  "*" _ -> true
  "" _ -> false
  Pattern "" -> (glob? (tlstr Pattern) "") where (= (pos Pattern 0) "*")
  Pattern "" -> false
  Pattern S -> (or (glob? (tlstr Pattern) S) (glob? Pattern (tlstr S)))
               where (= (pos Pattern 0) "*")
  Pattern S -> (glob? (tlstr Pattern) (tlstr S))
               where (= (pos Pattern 0) (pos S 0))
  _ _ -> false)

\* --- Fact access as Prolog predicates -------------------------------
   memberp/2 is spelled out rather than taken from a library so this
   file loads into a bare Shen with no prelude beyond the kernel.

   A NOTE ON `when` (W6)

   Every side condition below is written `(when (f …))`, never
   `(is V (f …)) (when V)`. The second form — which is what this file
   used until W6 — looks equivalent and is not: the goal always fails,
   so every predicate guarded that way was unsatisfiable and every
   premise came back vacuous. `is` binds a Prolog variable; `when`
   wants a Shen expression. Keep them in that order and the two never
   meet. *\

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
  X Seen <-- (when (not (element? X Seen)));)

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

\* Two references are exempt by construction rather than by
   allowlist, and `ctor-reference` is where that is stated: the
   constructor's own definition site, and any reference from inside a
   file that defines a symbol matching Ctor — the generated guard
   package names its own constructor in its own helpers and doc
   comments. flow.evalConstructorOnly (cmd/sb/flow/engine.go) applies
   exactly this exemption; the two engines have to agree on it or
   `sb flow --engine both` fails on every real fact base. *\

\* `defines-ctor-in-file?` is a Shen FUNCTION over the fact table, not
   a Prolog predicate, and so is `references-proof?` below. The reason
   is a sharp edge worth stating (W6): a `when` guard cannot call back
   into `prolog?` and have the inner goal see the clause's own
   bindings. `prolog?` reads every uppercase symbol in its goal as a
   FRESH variable, so `(when (not (prolog? (p Ctor File))))` hands the
   helper two unbound variables — which reached `matches?` and aborted
   the run with "mustString". Negation in these rules therefore has to
   be a boolean test on a Shen function, which is also what keeps the
   search a pure conjunction. *\

(define defines-ctor-in-file?
  Ctor File -> (any-def-in-file? Ctor File (value *flow-defs*)))

(define any-def-in-file?
  _ _ [] -> false
  Ctor File [[Sym F | _] | _] -> true where (and (= F File) (matches? Ctor Sym))
  Ctor File [_ | Rest] -> (any-def-in-file? Ctor File Rest))

(defprolog ctor-reference
  Ctor Caller File Line <-- (flow-ref Sym File Line _ Caller)
                            (when (matches? Ctor Sym))
                            (when (not (defines-ctor-in-file? Ctor File)));)

(defprolog unsanctioned-caller
  Ctor Caller File Line <-- (ctor-reference Ctor Caller File Line)
                            (when (not (allowed-caller? Ctor Caller)));)

\* --- must-pass-through violation search -----------------------------
   The lowering of (must-pass-through Src Proof Sink): for every
   definition matching Src, no call path may reach a call of Sink
   without some definition on the path referencing Proof. With the
   guard constructor as the declassifier this is noninterference.

   The search prunes at a declassifier: once a definition on the path
   references Proof, everything downstream of it is sanctioned and is
   not explored. Path is returned innermost-last, so reversing it
   gives the shortest violating path the report prints. *\

\* "Definition Sym references something matching Proof" — the
   declassifier test. A function for the same reason
   `defines-ctor-in-file?` is one: it is used under negation. Mirrors
   flow.refsMatching, which likewise skips a reference with no
   enclosing definition. *\
(define references-proof?
  Sym Proof -> (any-ref-from? Sym Proof (value *flow-refs*)))

(define any-ref-from?
  _ _ [] -> false
  Sym Proof [[Ref _ _ _ Encl] | _] -> true where (and (and (not (= Encl ""))
                                                           (= Encl Sym))
                                                      (matches? Proof Ref))
  Sym Proof [_ | Rest] -> (any-ref-from? Sym Proof Rest))

(defprolog source-def
  Src Sym <-- (flow-def Sym _ _ _ _ _)
              (when (matches? Src Sym))
              (when (callable? Sym));)

(defprolog must-pass-through-violation
  Src Proof Sink Path <-- (source-def Src Sym)
                          (unsanctioned-source Sym Proof)
                          (path-to-sink Sym Proof Sink [Sym] Path);)

\* A source that already holds the proof is sanctioned outright. *\
(defprolog unsanctioned-source
  Sym Proof <-- (when (not (references-proof? Sym Proof)));)

\* Either this definition calls the sink directly, or it calls some
   unsanctioned callee that does. *\
(defprolog path-to-sink
  Sym _ Sink Seen Seen <-- (flow-ref Callee _ _ _ Sym)
                           (when (matches? Sink Callee));
  Sym Proof Sink Seen Path <-- (flow-call Sym Next)
                               (fresh? Next Seen)
                               (unsanctioned-source Next Proof)
                               (path-to-sink Next Proof Sink [Next | Seen] Path);)

\* `callable?` is the one syntactic test in the vocabulary: SCIP spells
   a method or function descriptor with a trailing "().", and that is
   part of the protocol rather than of any language. *\
(define callable?
  Sym -> (ends-with? Sym "()."))
