---
date: 2026-05-22T00:00:00Z
researcher: claude
git_commit: 81ddf671a482f8b325db11acd04c72ec4182af03
branch: claude/tag-resolver-finish
repository: Shen-Backpressure
topic: "Tag-block resolver — closure memo for W1.4 open questions"
tags: [research, shen-web-tools, tag-resolver, closure, w1.4]
status: complete
last_updated: 2026-05-22
last_updated_by: claude
---

# Tag-Block Resolver — Open Questions Closed

> Closure record for the W1.4 wave (worktree
> `claude/tag-resolver-finish`). Closes the three open questions in
> [`2026-05-05-tag-resolver-open-questions.md`](2026-05-05-tag-resolver-open-questions.md)
> and companion file
> [`2026-05-05-tag-resolver-finish-line.md`](2026-05-05-tag-resolver-finish-line.md).

## Summary

The tag-block resolver in `examples/shen-web-tools/` now carries a
strengthened spec contract that addresses all three open questions
from the May 5 memo. The change surface is intentionally narrow:

- Spec: `examples/shen-web-tools/specs/{core,tag-block-resolver}.shen`
- Generated guards: `examples/shen-web-tools/runtime/guards_gen.ts`
- Resolver impl: `examples/shen-web-tools/runtime/tag_resolver.ts`
- Renderer contract: `examples/shen-web-tools/runtime/tag_render_contract.ts`
- Tests: `examples/shen-web-tools/runtime/tag_resolver.test.ts`,
  `examples/shen-web-tools/runtime/tag_render_contract.test.ts`,
  `examples/shen-web-tools/runtime/tag_resolver.shen-derive.test.ts` (regenerated)
- README: `examples/shen-web-tools/README.md`

No engine code (`cmd/`, `shen-derive/`) is touched. The
`tag-block` input schema is unchanged so the existing fixture-row
generator in `cmd/shen-derive-ts/verify/samples.ts` continues to
work without modification.

## Decisions

### Q1. Signature validity as a real crypto predicate

**Status: Resolved with a structural stub.**

The previous predicate was `(> (length Signature) 0)` — a
non-emptiness check. The new predicate is a deterministic stub-HMAC:

```shen
(define hmac-stub-of-payload
  {string --> (list tag-id) --> number --> number}
  Body ChildRefs Secret ->
    (shen.mod (+ (* (length Body) 31) (+ (* (length ChildRefs) 17) Secret)) 256))

(define digest-of-string
  {string --> number}
  S -> (shen.mod (length S) 256))

(define signature-valid?
  {tag-signature --> string --> (list tag-id) --> boolean}
  Sig Body ChildRefs ->
    false where (= 0 (length Sig))
  Sig Body ChildRefs ->
    (= (digest-of-string Sig) (hmac-stub-of-payload Body ChildRefs (demo-secret-length 0))))
```

The signature is structurally valid iff its digest (length mod 256)
equals a deterministic function of (body length, child-ref count,
demo-secret length).

**This is NOT cryptography.** It is a structural stand-in
demonstrating where a real HMAC predicate would slot in. The point is
that the *spec* now carries a *relational* predicate tying signature
to payload, not just a presence check. A real HMAC slots in by
replacing the two helper defines (`digest-of-string` and
`hmac-stub-of-payload`) with calls to a `:runtime-via` cryptographic
primitive — Wave 3 work
([`this-is-great-let-s-crispy-stallman.md`](../../.claude/plans/this-is-great-let-s-crispy-stallman.md)
section W3.2).

**Choice rationale.** The shen-derive-ts evaluator's primitive set
covers arithmetic, comparison, list/string `length`,
`shen.mod`/`%`, and friends — but does not include character-code
extraction or string concatenation. A real digest function therefore
cannot be expressed in this evaluator. The constraint forced a
length-based stub. Documented explicitly in the spec comment block so
readers cannot mistake the predicate for cryptography.

**Worked example.** For `Body = "Root"` (length 4),
`ChildRefs = ["child-a"]` (length 1), `Secret = 24` (`demo-secret-length`):

- `hmac-stub = (4*31 + 1*17 + 24) mod 256 = 165`
- A signature of exactly 165 chars passes (e.g., `"v".repeat(165)`)
- `"sig-root"` (8 chars) fails: 8 ≠ 165 → classified `unsigned-complete`

This makes signature acceptance a structural function of payload, not a
trivial check. Test coverage in
`runtime/tag_resolver.test.ts` includes both passing and failing
signatures.

### Q2. Recursive descendant resolution

**Status: Deferred with a documented sketch.**

The public resolver (`resolve-tag-block-children`) remains one-level:
it resolves the *root's direct children* against the ref-table and
classifies the outcome. Multi-level recursion is **not** wired into
the public path.

A `resolve-descendants-onelevel` clause is provided in the spec to
demonstrate the shape of one additional descent level:

```shen
(define resolve-descendants-onelevel
  {(list tag-block) --> ref-table --> (list (list tag-block))}
  [] _ -> []
  [Block | Rest] RefTable ->
    (cons (resolve-children (ChildRefs Block) RefTable)
      (resolve-descendants-onelevel Rest RefTable)))
```

This is documented but **not called** from the public resolver.

**Choice rationale.** A production recursive resolver needs cycle
detection (a tag block can reference itself directly or via a longer
loop). Cycle detection requires either:

1. An explicit "visited set" parameter threaded through recursion, OR
2. A depth budget.

Neither is in scope for a stub phase, and either would require
non-trivial spec gymnastics in Shen (no mutable state, immutable
lists for visited sets). Adding either would also bloat the generated
test surface — every recursion path becomes a sampling axis.

The sketch clause carries the shape so a future agent can see what a
recursion clause looks like in this codebase and pick up the work
with full context. The README and the spec comment block both note
the deferral and the reason.

### Q3. Stable provenance metadata beyond the raw signature

**Status: Resolved with a `tag-provenance` composite.**

New datatype `tag-provenance` is added to both `specs/core.shen` and
`specs/tag-block-resolver.shen`:

```shen
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
```

The `signed-complete` outcome's fourth field changed from
`Signature : tag-signature` to `Provenance : tag-provenance`. The
generated TypeScript guard reflects this: `SignedComplete.provenance()`
returns a `TagProvenance` instance with `signer()`, `stamp()`, and
`signature()` accessors.

**Choice rationale.** Downstream consumers (renderer, audit log)
previously had to look at *both* the outcome's signature *and* recover
"who signed" by convention. The composite makes that structural: a
signed outcome carries (1) the signer's identity, (2) when it was
sealed, (3) the signature bytes. Consumers can make policy decisions
("only render if `signer == "did:demo:tag-signer"`", or
"reject stamps older than X") without re-deriving structure.

**Demo values.** Since the spec must be runnable in shen-derive's
evaluator, the provenance is built from deterministic stub values:

- `signer = "did:demo:tag-signer"` (hardcoded constant)
- `stamp = (+ 1 (length Body))` (deterministic int, > 0)
- `signature = Signature Block` (carried through from the block)

In production these would come from the signing party. The structural
commitment is *that downstream code receives a typed composite*, not
the specific demo values.

The renderer contract (`runtime/tag_render_contract.ts`) exposes a
`TagProvenanceView` shape on `TagRenderState`:

```ts
provenance: { signer: string; stamp: number; signature: string } | null
```

`null` for `unsigned-complete` and `partial`; populated for
`signed-complete`. The legacy `signature` top-level field on
`TagRenderState` is preserved for backward compatibility but is
redundant with `provenance.signature` when provenance is non-null.

## Spec — Before / After

### `signed-complete` constructor

**Before:**
```shen
(datatype signed-complete
  Kind : string;
  Root : tag-block;
  Children : (list tag-block);
  Signature : tag-signature;
  (= Kind "signed-complete") : verified;
  =====================================
  [Kind Root Children Signature] : tag-resolve-outcome;)
```

**After:**
```shen
(datatype signed-complete
  Kind : string;
  Root : tag-block;
  Children : (list tag-block);
  Provenance : tag-provenance;
  (= Kind "signed-complete") : verified;
  =====================================
  [Kind Root Children Provenance] : tag-resolve-outcome;)
```

### Resolver dispatch

**Before:**
```shen
(define has-signature?
  {tag-block --> boolean}
  Block -> (> (length (Signature Block)) 0))

(define resolve-tag-block-children
  ...
  Block RefTable ->
    (cons "signed-complete"
      (cons Block
        (cons (resolve-children (ChildRefs Block) RefTable)
          (cons (Signature Block) nil))))
    where (has-signature? Block)
  ...)
```

**After:**
```shen
(define signature-valid?
  {tag-signature --> string --> (list tag-id) --> boolean}
  Sig Body ChildRefs ->
    false where (= 0 (length Sig))
  Sig Body ChildRefs ->
    (= (digest-of-string Sig) (hmac-stub-of-payload Body ChildRefs (demo-secret-length 0))))

(define block-signature-valid?
  {tag-block --> boolean}
  Block -> (signature-valid? (Signature Block) (Body Block) (ChildRefs Block)))

(define resolve-tag-block-children
  ...
  Block RefTable ->
    (cons "signed-complete"
      (cons Block
        (cons (resolve-children (ChildRefs Block) RefTable)
          (cons (provenance-of-block Block) nil))))
    where (block-signature-valid? Block)
  ...)
```

## Test surface

- `runtime/tag_resolver.test.ts` (11 cases): unit tests for the
  resolver and the stub-HMAC helpers. Includes:
  - signed-complete path (with `validSignatureFor` constructing a
    signature of the right length)
  - unsigned-complete path (empty signature and digest-mismatch
    signature both fall back to unsigned)
  - partial path (missing refs trump signature validity)
  - per-function tests for `digestOfString`, `hmacStubOfPayload`,
    `isSignatureValid`, `isBlockSignatureValid`, `provenanceOfBlock`.
- `runtime/tag_render_contract.test.ts` (4 cases): renderer contract
  tests, including the signed-complete path with provenance
  population and the digest-mismatch fallback.
- `runtime/tag_resolver.shen-derive.test.ts` (50 cases, regenerated):
  spec-equivalence over fixtures (3) + random samples (47). Note:
  with the strengthened predicate, none of the random samples
  produce `signed-complete` — the sampler can't trivially construct
  signatures that match the stub-HMAC formula. This is **expected
  and acceptable**: the shen-derive gate verifies spec/impl
  agreement, and both agree that random signatures fail validity.
  The signed-complete path coverage lives in the hand-written
  `tag_resolver.test.ts` cases that explicitly construct valid
  signatures.

`npm test` reports **90 passing, 0 failing**.

## Sampler note (out of scope for this worktree)

The fixture-row generator in
`cmd/shen-derive-ts/verify/samples.ts:260-294` hardcodes
`signature: "sig-root"` for the "signed root" fixture. With the new
predicate, that signature does NOT validate against body "Signed
root" — so the fixture is classified `unsigned-complete` rather
than `signed-complete`. The shen-derive test still passes (because
the impl matches the spec), but the fixture names in the sampler
are now mildly misleading.

A future engine-touching follow-up could update
`tagBlockResolverFixtureRows` to construct signatures via the same
formula as the spec, so the random sampling exercises the signed
path. That work is **out of scope here** (engine code), and is
noted as a deferred item.

## Deferred items

1. **Real cryptography**: replace the stub-HMAC with a real
   `:runtime-via` HMAC-SHA256 call. Tracked under W3.2 in the HN
   follow-up plan.
2. **Multi-level recursive descendant resolution**: needs cycle
   detection. Sketch clause `resolve-descendants-onelevel` shipped
   for shape; not wired into the public resolver.
3. **Sampler valid-signature fixture**: engine-side change to make
   the random sampler hit the signed path. Not in the W1.4 scope
   (engine code).

## Verification commands

```bash
# From repo root:
cd examples/shen-web-tools

npm install                    # install tsx + typescript
npm run shengen                # regenerate guards_gen.ts (drift)
npm run shen-derive            # regenerate tag_resolver.shen-derive.test.ts (drift)
npm test                       # node --test runtime/*.test.ts
npx tsc --noEmit               # type-check
npm run check                  # full pipeline: shengen + shen-derive-check + build + test
```

All commands above pass after the W1.4 changes.

## Code References

- Spec strengthening: `examples/shen-web-tools/specs/tag-block-resolver.shen:1-180`
- Datatype additions: `examples/shen-web-tools/specs/core.shen:194-260`
- Resolver impl: `examples/shen-web-tools/runtime/tag_resolver.ts:36-159`
- Renderer contract: `examples/shen-web-tools/runtime/tag_render_contract.ts:32-83`
- Regenerated guards: `examples/shen-web-tools/runtime/guards_gen.ts` (auto)
- Regenerated derive test: `examples/shen-web-tools/runtime/tag_resolver.shen-derive.test.ts` (auto)
- README walkthrough: `examples/shen-web-tools/README.md:28-58`

## Related Research

- [`2026-05-05-tag-resolver-open-questions.md`](2026-05-05-tag-resolver-open-questions.md) — original open-questions memo
- [`2026-05-05-tag-resolver-finish-line.md`](2026-05-05-tag-resolver-finish-line.md) — post-hoc finish-line record
- [`2026-05-22-hn-feedback-next-steps.md`](2026-05-22-hn-feedback-next-steps.md) — wave plan parent
