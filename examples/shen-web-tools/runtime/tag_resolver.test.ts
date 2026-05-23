import { test } from "node:test";
import assert from "node:assert/strict";

import * as shenguard from "./guards_gen.js";
import {
  digestOfString,
  hmacStubOfPayload,
  isBlockSignatureValid,
  isSignatureValid,
  provenanceOfBlock,
  ResolveRawTagBlockChildren,
  ResolveTagBlockChildren,
  toRefTable,
  toTagBlock,
} from "./tag_resolver.js";

// Construct a signature whose length matches the stub-HMAC digest for
// the given payload, so the predicate accepts it.
function validSignatureFor(body: string, childRefsCount: number): string {
  return "v".repeat(hmacStubOfPayload(body, childRefsCount, /* secret */ 24));
}

test("ResolveTagBlockChildren returns signed-complete when all refs resolve and signature passes stub-HMAC", () => {
  const body = "Root";
  const sig = validSignatureFor(body, 1);
  const root = toTagBlock({
    id: "root",
    body,
    childRefs: ["child-a"],
    signature: sig,
  });
  const refTable = toRefTable([
    {
      ref: "child-a",
      block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
    },
  ]);

  const outcome = ResolveTagBlockChildren(root, refTable);

  assert.ok(outcome instanceof shenguard.SignedComplete);
  assert.equal(outcome.root(), root);
  assert.equal(outcome.children().length, 1);
  // Provenance is a typed composite, not a raw string.
  const prov = outcome.provenance();
  assert.equal(prov.signer().val(), "did:demo:tag-signer");
  assert.equal(prov.stamp(), body.length + 1);
  assert.equal(prov.signature().val(), sig);
});

test("ResolveRawTagBlockChildren returns unsigned-complete when all refs resolve without signature", () => {
  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body: "Root", childRefs: ["child-a"], signature: "" },
    [
      {
        ref: "child-a",
        block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
      },
    ],
  );

  assert.ok(outcome instanceof shenguard.UnsignedComplete);
  assert.equal(outcome.children().length, 1);
});

test("ResolveRawTagBlockChildren falls back to unsigned-complete when stub-HMAC rejects the signature", () => {
  // A non-empty signature that does NOT structurally match the body
  // must classify as unsigned-complete, not signed-complete. This is
  // the W1.4 strengthening over the previous "non-empty" predicate.
  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body: "Root", childRefs: ["child-a"], signature: "sig-root" },
    [
      {
        ref: "child-a",
        block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
      },
    ],
  );

  assert.ok(outcome instanceof shenguard.UnsignedComplete);
});

test("ResolveRawTagBlockChildren returns partial for missing child refs", () => {
  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body: "Root", childRefs: ["child-a", "missing"], signature: "sig-root" },
    [
      {
        ref: "child-a",
        block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
      },
    ],
  );

  assert.ok(outcome instanceof shenguard.Partial);
  assert.equal(outcome.children().length, 1);
  assert.deepEqual(outcome.missing().map((ref) => ref.val()), ["missing"]);
});

test("raw wrapper rejects invalid tag ids before resolving", () => {
  assert.throws(
    () => ResolveRawTagBlockChildren(
      { id: "", body: "Root", childRefs: [], signature: "" },
      [],
    ),
    /length must be > 0|must be > 0/,
  );
});

// --- Stub-HMAC predicate unit tests ----------------------------------

test("digestOfString and hmacStubOfPayload are deterministic", () => {
  assert.equal(digestOfString("abcdef"), 6);
  assert.equal(digestOfString(""), 0);
  // (length "Root" * 31) + (1 * 17) + 24 = 124 + 17 + 24 = 165
  assert.equal(hmacStubOfPayload("Root", 1, 24), 165);
  // (0 * 31) + (0 * 17) + 24 = 24
  assert.equal(hmacStubOfPayload("", 0, 24), 24);
  // (12 * 31) + (3 * 17) + 24 = 372 + 51 + 24 = 447 → 447 mod 256 = 191
  assert.equal(hmacStubOfPayload("123456789012", 3, 24), 191);
});

test("isSignatureValid rejects empty signatures regardless of payload", () => {
  assert.equal(isSignatureValid("", "anything", 0), false);
  assert.equal(isSignatureValid("", "", 0), false);
});

test("isSignatureValid accepts only signatures whose digest matches payload digest", () => {
  // For body="" and childRefsCount=0, hmac-stub = 24. A 24-char signature
  // validates; a 23-char one does not.
  assert.equal(isSignatureValid("v".repeat(24), "", 0), true);
  assert.equal(isSignatureValid("v".repeat(23), "", 0), false);
  assert.equal(isSignatureValid("v".repeat(25), "", 0), false);
});

test("isBlockSignatureValid reads body and childRefs from the block", () => {
  const validBlock = toTagBlock({
    id: "x",
    body: "Root",
    childRefs: ["c"],
    signature: "v".repeat(165),
  });
  assert.equal(isBlockSignatureValid(validBlock), true);

  const invalidBlock = toTagBlock({
    id: "x",
    body: "Root",
    childRefs: ["c"],
    signature: "sig-root",
  });
  assert.equal(isBlockSignatureValid(invalidBlock), false);
});

test("provenanceOfBlock encodes the signer, stamp, and raw signature", () => {
  // For body "Root" (length 4), no children, secret 24:
  //   hmac-stub = (4*31) + 0 + 24 = 148
  const block = toTagBlock({
    id: "x",
    body: "Root",
    childRefs: [],
    signature: "v".repeat(148),
  });
  const prov = provenanceOfBlock(block);
  assert.equal(prov.signer().val(), "did:demo:tag-signer");
  assert.equal(prov.stamp(), 5);
  assert.equal(prov.signature().val(), "v".repeat(148));
});
