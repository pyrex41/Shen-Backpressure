# Open Questions

The tag/ref-table phase is now tracked in `notes/finish-line-roadmap.md`.

## Resolved For This Phase

- Tag-block child refs resolve to signed complete, unsigned complete, or partial outcomes.
- Partial outcomes remain renderable and carry the missing refs.
- The renderer contract is centralized in `runtime/tag_render_contract.ts`.
- The product resolver is covered by generated `shen-derive-ts` cases plus focused unit tests.

## Still Open

- Whether signature validity should become a real crypto predicate instead of signature presence.
- Whether ref tables should support recursive descendant resolution.
- Whether product payloads should carry stable provenance metadata beyond the current signature string.
