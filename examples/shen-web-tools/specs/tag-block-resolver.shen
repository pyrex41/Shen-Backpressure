\* tag-block-resolver.shen — product resolver contract for tag/ref tables *\
\* The datatypes mirror specs/core.shen so shen-derive-ts can build a local
   type table while the generated test imports runtime/guards_gen.ts.

   --- Phase note (W1.4) -----------------------------------------------
   This phase strengthens the resolver contract beyond the original
   "non-empty signature string" notion. Three open questions from
   2026-05-05-tag-resolver-open-questions.md are addressed here:

   Q1. Signature validity becomes a deterministic structural predicate
       — `signature-valid?` checks that the signature's *digest* matches
       a stub HMAC computed from the body, child-ref count, and a
       hardcoded demo secret. This is NOT cryptography — it is a
       structural stub demonstrating where a real HMAC predicate would
       slot in. See `hmac-stub-of-payload` and `digest-of-string` below.

   Q2. Recursive descendant resolution. Deferred for this phase: full
       multi-level recursion needs cycle detection that is out of scope
       for a stub. A `resolve-descendants-onelevel` clause is provided
       below to demonstrate the recursion shape without committing the
       public resolver to it. The public `resolve-tag-block-children`
       remains one-level.

   Q3. Stable provenance metadata. The `signed-complete` outcome now
       carries a `tag-provenance` composite (signer-id + stamp +
       signature) instead of a raw signature string. Downstream
       consumers see who signed and when, not just the opaque
       signature bytes.

   The `tag-block` input schema is unchanged so existing fixture-row
   sample generation in cmd/shen-derive-ts/verify/samples.ts continues
   to work. The change surface is the resolver output and the
   validity predicate. *\

(datatype tag-id
  X : string;
  (> (length X) 0) : verified;
  ==============
  X : tag-id;)

(datatype tag-signature
  X : string;
  ==============
  X : tag-signature;)

(datatype signer-id
  X : string;
  (> (length X) 0) : verified;
  ==============
  X : signer-id;)

(datatype tag-provenance
  Signer : signer-id;
  Stamp : number;
  Signature : tag-signature;
  (> Stamp 0) : verified;
  ==================================
  [Signer Stamp Signature] : tag-provenance;)

(datatype tag-block
  Id : tag-id;
  Body : string;
  ChildRefs : (list tag-id);
  Signature : tag-signature;
  ===================================
  [Id Body ChildRefs Signature] : tag-block;)

(datatype ref-table-entry
  Ref : tag-id;
  Block : tag-block;
  =========================
  [Ref Block] : ref-table-entry;)

(datatype ref-table
  Entries : (list ref-table-entry);
  (>= (length Entries) 0) : verified;
  ================================
  Entries : ref-table;)

(datatype signed-complete
  Kind : string;
  Root : tag-block;
  Children : (list tag-block);
  Provenance : tag-provenance;
  (= Kind "signed-complete") : verified;
  =====================================
  [Kind Root Children Provenance] : tag-resolve-outcome;)

(datatype unsigned-complete
  Kind : string;
  Root : tag-block;
  Children : (list tag-block);
  (= Kind "unsigned-complete") : verified;
  ================================
  [Kind Root Children] : tag-resolve-outcome;)

(datatype partial
  Kind : string;
  Root : tag-block;
  Children : (list tag-block);
  Missing : (list tag-id);
  (= Kind "partial") : verified;
  =================================
  [Kind Root Children Missing] : tag-resolve-outcome;)

(define ref-present?
  {tag-id --> ref-table --> boolean}
  _ [] -> false
  Ref [[EntryRef _] | Rest] -> true where (= Ref EntryRef)
  Ref [_ | Rest] -> (ref-present? Ref Rest))

(define lookup-ref
  {tag-id --> ref-table --> tag-block}
  Ref [[EntryRef Block] | Rest] -> Block where (= Ref EntryRef)
  Ref [_ | Rest] -> (lookup-ref Ref Rest))

(define resolve-children
  {(list tag-id) --> ref-table --> (list tag-block)}
  [] _ -> []
  [Ref | Rest] RefTable -> (cons (lookup-ref Ref RefTable) (resolve-children Rest RefTable)) where (ref-present? Ref RefTable)
  [Ref | Rest] RefTable -> (resolve-children Rest RefTable))

(define missing-refs
  {(list tag-id) --> ref-table --> (list tag-id)}
  [] _ -> []
  [Ref | Rest] RefTable -> (missing-refs Rest RefTable) where (ref-present? Ref RefTable)
  [Ref | Rest] RefTable -> (cons Ref (missing-refs Rest RefTable)))

(define all-refs-present?
  {(list tag-id) --> ref-table --> boolean}
  Refs RefTable -> (= 0 (length (missing-refs Refs RefTable))))

\* --- Signature validity: stub HMAC predicate ---

   `hmac-stub-of-payload` is a structural stand-in for a real HMAC.
   Given body length B, child-ref count C, and secret length S, it
   returns a deterministic digest in [0, 255]:

       digest = (B * 31 + C * 17 + S) mod 256

   A signature is structurally valid when its own digest matches —
   here `digest-of-string Sig` is `(length Sig) mod 256`, so the
   predicate decides only on the signature's *length*. This is
   intentionally simple: the point is to demonstrate that the spec
   carries a *relational* predicate between signature and payload,
   not that the predicate is cryptographic. A real HMAC slots in by
   replacing these two defines with calls to a :runtime-via
   cryptographic primitive (Wave 3 work).

   The demo secret is hardcoded via `demo-secret-length`. A real
   implementation would carry it via a bound parameter or a
   runtime-via gate. *\

(define demo-secret-length
  {number --> number}
  _ -> 24)

(define digest-of-string
  {string --> number}
  S -> (shen.mod (length S) 256))

(define hmac-stub-of-payload
  {string --> (list tag-id) --> number --> number}
  Body ChildRefs Secret ->
    (shen.mod (+ (* (length Body) 31) (+ (* (length ChildRefs) 17) Secret)) 256))

(define signature-valid?
  {tag-signature --> string --> (list tag-id) --> boolean}
  Sig Body ChildRefs ->
    false where (= 0 (length Sig))
  Sig Body ChildRefs ->
    (= (digest-of-string Sig) (hmac-stub-of-payload Body ChildRefs (demo-secret-length 0))))

(define block-signature-valid?
  {tag-block --> boolean}
  Block -> (signature-valid? (Signature Block) (Body Block) (ChildRefs Block)))

\* --- Provenance construction ---

   Provenance carries (signer, stamp, signature). For demo purposes:
   - signer is a hardcoded "did:demo:tag-signer" identifier
   - stamp is a deterministic int derived from the body length
     (positive — body length is +1 to satisfy (> Stamp 0))
   - signature is the raw signature from the block

   In production this would be supplied by the signing party and
   carried alongside the tag block. The structural commitment here is
   that downstream consumers receive a typed composite, not an
   opaque string. *\

(define demo-signer-id
  {number --> signer-id}
  _ -> "did:demo:tag-signer")

(define stamp-from-body
  {string --> number}
  Body -> (+ 1 (length Body)))

(define provenance-of-block
  {tag-block --> tag-provenance}
  Block ->
    (cons (demo-signer-id 0)
      (cons (stamp-from-body (Body Block))
        (cons (Signature Block) nil))))

\* --- Recursive descendants (Q2 sketch, not wired into resolver) ---

   This clause demonstrates how a one-step deeper recursion would
   look. It is intentionally NOT called from
   resolve-tag-block-children — a production recursion needs cycle
   detection that is out of scope for this stub phase. Documented
   here so the shape is auditable. *\

(define resolve-descendants-onelevel
  {(list tag-block) --> ref-table --> (list (list tag-block))}
  [] _ -> []
  [Block | Rest] RefTable ->
    (cons (resolve-children (ChildRefs Block) RefTable)
      (resolve-descendants-onelevel Rest RefTable)))

\* --- Public resolver ---

   Classifies a root tag-block + ref-table into one of three outcomes.
   The order of clauses matters:

   1. If any child ref is missing → partial.
   2. Else if the block signature passes `block-signature-valid?` →
      signed-complete with a typed provenance composite.
   3. Else → unsigned-complete.

   The Q1 strengthening is in step 2: validity is no longer "non-empty
   string". The Q3 strengthening is the provenance composite carried
   in step 2's output. *\

(define resolve-tag-block-children
  {tag-block --> ref-table --> tag-resolve-outcome}
  Block RefTable ->
    (cons "partial"
      (cons Block
        (cons (resolve-children (ChildRefs Block) RefTable)
          (cons (missing-refs (ChildRefs Block) RefTable) nil))))
    where (not (all-refs-present? (ChildRefs Block) RefTable))

  Block RefTable ->
    (cons "signed-complete"
      (cons Block
        (cons (resolve-children (ChildRefs Block) RefTable)
          (cons (provenance-of-block Block) nil))))
    where (block-signature-valid? Block)

  Block RefTable ->
    (cons "unsigned-complete"
      (cons Block
        (cons (resolve-children (ChildRefs Block) RefTable) nil))))
