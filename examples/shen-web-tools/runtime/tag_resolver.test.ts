import { test } from "node:test";
import assert from "node:assert/strict";

import * as shenguard from "./guards_gen.js";
import {
  ResolveRawTagBlockChildren,
  ResolveTagBlockChildren,
  toRefTable,
  toTagBlock,
} from "./tag_resolver.js";

test("ResolveTagBlockChildren returns signed-complete when all refs resolve and root is signed", () => {
  const root = toTagBlock({
    id: "root",
    body: "Root",
    childRefs: ["child-a"],
    signature: "sig-root",
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
  assert.equal(outcome.signature().val(), "sig-root");
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
