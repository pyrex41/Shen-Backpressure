\* tag-block-resolver.shen — product resolver contract for tag/ref tables *\
\* The datatypes mirror specs/core.shen so shen-derive-ts can build a local
   type table while the generated test imports runtime/guards_gen.ts. *\

(datatype tag-id
  X : string;
  (> (length X) 0) : verified;
  ==============
  X : tag-id;)

(datatype tag-signature
  X : string;
  ==============
  X : tag-signature;)

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
  Signature : tag-signature;
  (= Kind "signed-complete") : verified;
  =====================================
  [Kind Root Children Signature] : tag-resolve-outcome;)

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

(define has-signature?
  {tag-block --> boolean}
  Block -> (> (length (Signature Block)) 0))

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
          (cons (Signature Block) nil))))
    where (has-signature? Block)

  Block RefTable ->
    (cons "unsigned-complete"
      (cons Block
        (cons (resolve-children (ChildRefs Block) RefTable) nil))))
