import { test } from "node:test";
import assert from "node:assert/strict";

import { ResolveRawTagBlockChildren } from "./tag_resolver.js";
import { toTagRenderState } from "./tag_render_contract.js";

test("toTagRenderState maps signed-complete outcome to fully renderable signed state", () => {
  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body: "Root", childRefs: ["child-a"], signature: "sig-root" },
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
  assert.equal(state.signature, "sig-root");
});

test("toTagRenderState maps unsigned-complete outcome without missing refs", () => {
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
});

test("toTagRenderState maps missing refs to partial renderable state", () => {
  const outcome = ResolveRawTagBlockChildren(
    { id: "root", body: "Root", childRefs: ["child-a", "missing"], signature: "sig-root" },
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
});
