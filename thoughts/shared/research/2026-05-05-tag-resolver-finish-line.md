---
date: 2026-05-05T19:00:00Z
researcher: claude
git_commit: 399c00a
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Tag-block resolver finish line — post-hoc record"
tags: [research, shen-web-tools, tag-resolver, shen-derive-ts, finish-line]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Finish-Line Roadmap

> Post-hoc record of the tag-block resolver phase that shipped on
> main in commit `c565543` (`shen-web-tools: prove tag resolver
> outcomes`). Originally lived at `notes/finish-line-roadmap.md`;
> moved into `thoughts/shared/research/` during the post-contraction
> cleanup. Companion memo:
> [`2026-05-05-tag-resolver-open-questions.md`](2026-05-05-tag-resolver-open-questions.md).

## Finish Line

Tag-block child references now resolve into one of three typed outcomes:

- `signed-complete`: all child refs are present and the root tag block carries a signature.
- `unsigned-complete`: all child refs are present, but the root tag block has no signature.
- `partial`: at least one child ref is missing, while available children remain renderable.

The contract is represented in Shen datatypes, generated TypeScript guards, a product resolver wrapper, a renderer-facing adapter, correlated fixtures, and a generated `shen-derive-ts` parity test.

## Current Baseline

- Product datatypes live in `examples/shen-web-tools/specs/core.shen`.
- Resolver semantics live in `examples/shen-web-tools/specs/tag-block-resolver.shen`.
- Generated guards live in `examples/shen-web-tools/runtime/guards_gen.ts`.
- The implementation wrapper lives in `examples/shen-web-tools/runtime/tag_resolver.ts`.
- Renderer normalization lives in `examples/shen-web-tools/runtime/tag_render_contract.ts`.
- The derive gate is `examples/shen-web-tools/runtime/tag_resolver.shen-derive.test.ts`.

## Guardrails

- The renderer consumes `TagRenderState`, not raw ref tables.
- Missing child refs must not produce `signed-complete`.
- Invalid raw ids fail at guard construction.
- Correlated signed, unsigned, and partial rows are fixture-backed so generic sampling cannot miss the phase-critical cases.

## Completed Milestones

1. Semantic tag/ref-table validation: product datatypes and tri-state sum variants are defined in Shen.
2. Product resolver wrapper: raw payloads normalize through generated guard constructors before resolving.
3. Renderer contract integration: all outcomes map through a single renderer-facing state function.
4. Spec/derive retag: the example derive gate now targets `resolve-tag-block-children`.
5. Small product fixture set: signed, unsigned, and partial rows are prepended to generated cases.
6. Notes cleanup: this file and [`2026-05-05-tag-resolver-open-questions.md`](2026-05-05-tag-resolver-open-questions.md) describe the shipped finish line.

## Non-Goals

- Cryptographic signature verification is represented by signature presence only in this phase.
- Ref lookup is in-memory and deterministic; no network/database lookup is introduced.
- Recursive rendering of grandchildren is not part of this finish line.
- The older smoke derive fixture remains in the tree for reference, but it is no longer the active example gate.
