---
date: 2026-05-05T19:00:00Z
researcher: claude
git_commit: 399c00a
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Tag-block resolver — open questions for the next phase"
tags: [research, shen-web-tools, tag-resolver, open-questions]
status: open
last_updated: 2026-05-05
last_updated_by: claude
---

# Open Questions

> Companion to
> [`2026-05-05-tag-resolver-finish-line.md`](2026-05-05-tag-resolver-finish-line.md).
> Originally lived at `notes/open-questions.md`; moved into
> `thoughts/shared/research/` during the post-contraction cleanup.

The tag/ref-table phase is now tracked in
[`2026-05-05-tag-resolver-finish-line.md`](2026-05-05-tag-resolver-finish-line.md).

## Resolved For This Phase

- Tag-block child refs resolve to signed complete, unsigned complete, or partial outcomes.
- Partial outcomes remain renderable and carry the missing refs.
- The renderer contract is centralized in `runtime/tag_render_contract.ts`.
- The product resolver is covered by generated `shen-derive-ts` cases plus focused unit tests.

## Still Open

- Whether signature validity should become a real crypto predicate instead of signature presence.
- Whether ref tables should support recursive descendant resolution.
- Whether product payloads should carry stable provenance metadata beyond the current signature string.
