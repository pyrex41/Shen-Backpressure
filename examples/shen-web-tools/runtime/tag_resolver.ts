// Tag-block resolver implementation.
//
// Phase W1.4 — strengthened to mirror the spec in
// `specs/tag-block-resolver.shen`:
//
//   - Signature validity is no longer "non-empty string". The spec
//     defines a deterministic stub-HMAC predicate: a signature is
//     structurally valid iff its digest (length mod 256) equals
//     `hmac-stub-of-payload(body, childRefs, demoSecretLength)`. This
//     is NOT cryptography — it is a structural stand-in showing where
//     a real HMAC predicate would slot in. See the spec for details.
//
//   - `signed-complete` carries a typed `TagProvenance` composite
//     (signer-id, stamp, signature), not a raw signature string.
//     Downstream consumers receive structured provenance.
//
// The shen-derive-ts gate (`runtime/tag_resolver.shen-derive.test.ts`)
// pins this implementation to the spec value-by-value. If the spec or
// the implementation drift, that test fails.

import * as shenguard from "./guards_gen.js";

export type RawTagBlock = {
  id: string;
  body: string;
  childRefs: string[];
  signature?: string;
};

export type RawRefTableEntry = {
  ref: string;
  block: RawTagBlock;
};

// --- Stub predicate / provenance helpers (mirror the spec) -----------

// Mirrors `(define demo-secret-length _ -> 24)`.
const DEMO_SECRET_LENGTH = 24;

// Mirrors `(define demo-signer-id _ -> "did:demo:tag-signer")`.
const DEMO_SIGNER_ID = "did:demo:tag-signer";

// Mirrors `(define digest-of-string S -> (shen.mod (length S) 256))`.
export function digestOfString(s: string): number {
  return s.length % 256;
}

// Mirrors `(define hmac-stub-of-payload Body ChildRefs Secret ->
//   (shen.mod (+ (* (length Body) 31) (+ (* (length ChildRefs) 17) Secret)) 256))`.
export function hmacStubOfPayload(
  body: string,
  childRefsCount: number,
  secretLength: number,
): number {
  return (body.length * 31 + childRefsCount * 17 + secretLength) % 256;
}

// Mirrors `(define signature-valid? Sig Body ChildRefs ...)`.
export function isSignatureValid(
  signature: string,
  body: string,
  childRefsCount: number,
): boolean {
  if (signature.length === 0) return false;
  return digestOfString(signature) ===
    hmacStubOfPayload(body, childRefsCount, DEMO_SECRET_LENGTH);
}

// Mirrors `(define block-signature-valid? Block -> ...)`.
export function isBlockSignatureValid(block: shenguard.TagBlock): boolean {
  return isSignatureValid(
    block.signature().val(),
    block.body(),
    block.childRefs().length,
  );
}

// Mirrors `(define stamp-from-body Body -> (+ 1 (length Body)))`.
export function stampFromBody(body: string): number {
  return 1 + body.length;
}

// Mirrors `(define provenance-of-block Block -> ...)`.
export function provenanceOfBlock(
  block: shenguard.TagBlock,
): shenguard.TagProvenance {
  return shenguard.mustTagProvenance(
    shenguard.mustSignerId(DEMO_SIGNER_ID),
    stampFromBody(block.body()),
    block.signature(),
  );
}

// --- Raw → guard adapters --------------------------------------------

export function toTagBlock(raw: RawTagBlock): shenguard.TagBlock {
  return shenguard.mustTagBlock(
    shenguard.mustTagId(raw.id),
    raw.body,
    raw.childRefs.map((ref) => shenguard.mustTagId(ref)),
    shenguard.mustTagSignature(raw.signature ?? ""),
  );
}

export function toRefTable(entries: RawRefTableEntry[]): shenguard.RefTable {
  return shenguard.mustRefTable(
    entries.map((entry) =>
      shenguard.mustRefTableEntry(
        shenguard.mustTagId(entry.ref),
        toTagBlock(entry.block),
      )
    ),
  );
}

export function ResolveRawTagBlockChildren(
  block: RawTagBlock,
  refTable: RawRefTableEntry[],
): shenguard.TagResolveOutcome {
  return ResolveTagBlockChildren(toTagBlock(block), toRefTable(refTable));
}

// --- Resolver --------------------------------------------------------

export function ResolveTagBlockChildren(
  block: shenguard.TagBlock,
  refTable: shenguard.RefTable,
): shenguard.TagResolveOutcome {
  const entries = refTable.val();
  const children: shenguard.TagBlock[] = [];
  const missing: shenguard.TagId[] = [];

  for (const ref of block.childRefs()) {
    const found = entries.find((entry) => entry.ref().val() === ref.val());
    if (found) {
      children.push(found.block());
    } else {
      missing.push(ref);
    }
  }

  // Clause 1: any missing ref → partial.
  if (missing.length > 0) {
    return shenguard.mustPartial("partial", block, children, missing);
  }

  // Clause 2: signature passes the stub-HMAC predicate → signed-complete
  // with provenance composite.
  if (isBlockSignatureValid(block)) {
    return shenguard.mustSignedComplete(
      "signed-complete",
      block,
      children,
      provenanceOfBlock(block),
    );
  }

  // Clause 3: otherwise unsigned-complete.
  return shenguard.mustUnsignedComplete("unsigned-complete", block, children);
}
