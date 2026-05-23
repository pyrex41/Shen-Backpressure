import { test } from "node:test";
import assert from "node:assert/strict";

import {
  hmacStubOfPayload,
  ResolveRawTagBlockChildren,
} from "./tag_resolver.js";
import { toTagRenderState } from "./tag_render_contract.js";

// --- Helper: build a signature of exactly the right length to pass
// the stub-HMAC predicate for the given body + childRefs count.
// Mirrors the spec's `(length Sig) mod 256 == hmac-stub-of-payload(...)`.
function validSignatureFor(body: string, childRefsCount: number): string {
  const targetLen = hmacStubOfPayload(body, childRefsCount, /* secret */ 24);
  return "v".repeat(targetLen);
}

test("toTagRenderState maps signed-complete outcome to fully renderable signed state with provenance", () => {
  const body = "Root";
  const childRefs = ["child-a"];
  const sig = validSignatureFor(body, childRefs.length);

  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body, childRefs, signature: sig },
    [
      {
        ref: "child-a",
        block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
      },
    ],
  );

  const state = toTagRenderState(outcome);

  assert.equal(state.status, "signed-complete");
  assert.equal(state.root.id, "root");
  assert.equal(state.children.length, 1);
  assert.deepEqual(state.missingRefs, []);
  assert.equal(state.signature, sig);
  assert.ok(state.provenance, "provenance must be populated for signed-complete");
  assert.equal(state.provenance!.signer, "did:demo:tag-signer");
  assert.equal(state.provenance!.stamp, body.length + 1);
  assert.equal(state.provenance!.signature, sig);
});

test("toTagRenderState maps unsigned-complete outcome without missing refs and null provenance", () => {
  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body: "Root", childRefs: ["child-a"], signature: "" },
    [
      {
        ref: "child-a",
        block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
      },
    ],
  );

  const state = toTagRenderState(outcome);

  assert.equal(state.status, "unsigned-complete");
  assert.equal(state.children.length, 1);
  assert.deepEqual(state.missingRefs, []);
  assert.equal(state.signature, "");
  assert.equal(state.provenance, null);
});

test("toTagRenderState classifies a signature whose digest does NOT match payload as unsigned-complete", () => {
  // Phase W1.4: non-empty signature alone is no longer enough — the
  // signature must structurally relate to the payload (stub-HMAC). A
  // signature of arbitrary length almost always fails the predicate.
  const outcome = ResolveRawTagBlockChildren(
    {
      id: "root",
      body: "Root",
      childRefs: ["child-a"],
      // 8 chars; hmac-stub for body "Root" (4) + 1 child + secret 24
      // = (4*31 + 17 + 24) mod 256 = 165 ≠ 8.
      signature: "sig-root",
    },
    [
      {
        ref: "child-a",
        block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
      },
    ],
  );

  const state = toTagRenderState(outcome);

  assert.equal(state.status, "unsigned-complete");
  assert.deepEqual(state.missingRefs, []);
  assert.equal(state.signature, "");
  assert.equal(state.provenance, null);
});

test("toTagRenderState maps missing refs to partial renderable state regardless of signature validity", () => {
  const body = "Root";
  const childRefs = ["child-a", "missing"];
  const sig = validSignatureFor(body, childRefs.length);

  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body, childRefs, signature: sig },
    [
      {
        ref: "child-a",
        block: { id: "child-a", body: "Child A", childRefs: [], signature: "" },
      },
    ],
  );

  const state = toTagRenderState(outcome);

  assert.equal(state.status, "partial");
  assert.equal(state.children.length, 1);
  assert.deepEqual(state.missingRefs, ["missing"]);
  assert.notEqual(state.status, "signed-complete");
  assert.equal(state.provenance, null);
});
