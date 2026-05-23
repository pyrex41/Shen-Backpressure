// Renderer contract for tag-resolve outcomes.
//
// Phase W1.4 — the `signed-complete` outcome now carries a typed
// `TagProvenance` composite. The renderer-facing state exposes that
// provenance to UI code instead of just an opaque signature string,
// so downstream consumers can be more discerning (audit signer,
// inspect stamp, etc.) without re-deriving structure.

import * as shenguard from "./guards_gen.js";

export type TagRenderStatus = "signed-complete" | "unsigned-complete" | "partial";

export type RenderableTagBlock = {
  id: string;
  body: string;
  childRefs: string[];
  signature: string;
};

export type TagProvenanceView = {
  signer: string;
  stamp: number;
  signature: string;
};

export type TagRenderState = {
  status: TagRenderStatus;
  root: RenderableTagBlock;
  children: RenderableTagBlock[];
  missingRefs: string[];
  /** Raw signature string. "" when unsigned/partial. Kept for backward
   * compatibility with renderer code that read the legacy field. */
  signature: string;
  /** Structured provenance. Populated only for `signed-complete`.
   * `null` for `unsigned-complete` and `partial`. */
  provenance: TagProvenanceView | null;
};

export function toTagRenderState(
  outcome: shenguard.TagResolveOutcome,
): TagRenderState {
  if (outcome instanceof shenguard.SignedComplete) {
    const provenance = outcome.provenance();
    return {
      status: "signed-complete",
      root: renderableBlock(outcome.root()),
      children: outcome.children().map(renderableBlock),
      missingRefs: [],
      signature: provenance.signature().val(),
      provenance: {
        signer: provenance.signer().val(),
        stamp: provenance.stamp(),
        signature: provenance.signature().val(),
      },
    };
  }

  if (outcome instanceof shenguard.UnsignedComplete) {
    return {
      status: "unsigned-complete",
      root: renderableBlock(outcome.root()),
      children: outcome.children().map(renderableBlock),
      missingRefs: [],
      signature: "",
      provenance: null,
    };
  }

  if (outcome instanceof shenguard.Partial) {
    return {
      status: "partial",
      root: renderableBlock(outcome.root()),
      children: outcome.children().map(renderableBlock),
      missingRefs: outcome.missing().map((ref) => ref.val()),
      signature: "",
      provenance: null,
    };
  }

  throw new Error("unknown tag render outcome");
}

function renderableBlock(block: shenguard.TagBlock): RenderableTagBlock {
  return {
    id: block.id().val(),
    body: block.body(),
    childRefs: block.childRefs().map((ref) => ref.val()),
    signature: block.signature().val(),
  };
}
