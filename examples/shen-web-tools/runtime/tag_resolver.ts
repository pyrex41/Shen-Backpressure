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

  if (missing.length > 0) {
    return shenguard.mustPartial("partial", block, children, missing);
  }

  const signature = block.signature();
  if (signature.val().length > 0) {
    return shenguard.mustSignedComplete(
      "signed-complete",
      block,
      children,
      signature,
    );
  }

  return shenguard.mustUnsignedComplete("unsigned-complete", block, children);
}
