import * as shenguard from "./guards_gen.js";

export type TagRenderStatus = "signed-complete" | "unsigned-complete" | "partial";

export type RenderableTagBlock = {
  id: string;
  body: string;
  childRefs: string[];
  signature: string;
};

export type TagRenderState = {
  status: TagRenderStatus;
  root: RenderableTagBlock;
  children: RenderableTagBlock[];
  missingRefs: string[];
  signature: string;
};

export function toTagRenderState(
  outcome: shenguard.TagResolveOutcome,
): TagRenderState {
  if (outcome instanceof shenguard.SignedComplete) {
    return {
      status: "signed-complete",
      root: renderableBlock(outcome.root()),
      children: outcome.children().map(renderableBlock),
      missingRefs: [],
      signature: outcome.signature().val(),
    };
  }

  if (outcome instanceof shenguard.UnsignedComplete) {
    return {
      status: "unsigned-complete",
      root: renderableBlock(outcome.root()),
      children: outcome.children().map(renderableBlock),
      missingRefs: [],
      signature: "",
    };
  }

  if (outcome instanceof shenguard.Partial) {
    return {
      status: "partial",
      root: renderableBlock(outcome.root()),
      children: outcome.children().map(renderableBlock),
      missingRefs: outcome.missing().map((ref) => ref.val()),
      signature: "",
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
