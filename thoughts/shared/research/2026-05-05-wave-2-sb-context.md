---
date: 2026-05-05T17:55:00Z
researcher: claude
git_commit: 6f367c8959a0400b9013f30844927ee087db9495
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Wave 2 — sb context"
tags: [research, sb-engine, wave-2, context, prompt-hydration, post-hoc]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Wave 2 — `sb context`

Post-hoc record of the Wave 2 work landed in commit `827d795`
(`sb: Wave 2 — context command + scaffolding/transparency`, 2026-04-11).

## What problem this wave solved

Wave 1 made the manifest the source of truth for gate topology. That
meant prompts and skills could no longer hard-code "the five gates" —
they had to ask the engine what the project's gate shape actually was.

The pre-Wave-2 prompts solved this by scraping the filesystem from
inside the Ralph harness: cat `sb.toml`, grep for guard type names in
`internal/shenguard/guards_gen.go`, infer the proof chain from the spec
file, etc. Three problems:

- **Drift.** Prompt logic and engine logic disagreed about how to
  classify a `[[Bal Tx]] : balance-checked` block. Two parsers, one
  source of truth, divergent answers.
- **Per-prompt re-implementation.** Every command (`/sb:loop`,
  `/sb:init`, `/sb:ralph-scaffold`, third-party commands) reinvented
  the same scrape.
- **No structured contract.** A Cursor harness, a Codex harness, a CI
  step, and a developer running `sb gates` interactively all needed
  the same information; they had no canonical channel for it.

The user-visible pain: editing the manifest and the spec did not
propagate into the agent's worldview unless the agent also re-scraped
correctly. The engine knew the right answer; nothing was emitting it.

## What was built

`sb context` reads the manifest, parses the Shen spec inline, and emits
a single normalised `ProjectContext` in either JSON (machine-readable)
or Markdown (prompt-injectable).

Key code paths:

- `cmd/sb/context.go:15-72` — `ProjectContext`, `ProjectInfo`,
  `TypeInfo`, `DeriveInfo`, `GateInfo`, `BackpressureInfo`. These are
  exported so `loop.go` can build and consume them in-process without
  shelling out (Wave 3).
- `cmd/sb/context.go:78` — `BuildContext(cfg)` is the single entry
  point used by both the CLI command and the loop.
- `cmd/sb/context.go:332` — `parseSpecTypes` is a self-contained Shen
  spec parser (~200 lines, stdlib only). It classifies every datatype
  block into one of `wrapper`, `constrained`, `composite`, `guarded`,
  `alias`, `sumtype`, and resolves field dependencies by intersecting
  field types with the known type set.
- `cmd/sb/context.go:243` — `proofChain` linearises the dependency DAG
  for the markdown render.
- `cmd/sb/context.go:183` — `RenderMarkdown` produces the block that
  gets injected into harness prompts (Wave 3).
- `cmd/sb/context.go:272` — the `cmdContext` CLI entry point with
  `--format json|markdown` and `--out FILE`.

The backpressure log integration (`BackpressureInfo`, fed from
`plans/backpressure.log`) closes the loop: a prompt asking "what
failed last iteration?" gets a deterministic answer from the same
file the loop wrote.

## What was rejected

- **JSON-only output.** Considered emitting only structured JSON and
  letting prompts template themselves. Rejected: every harness would
  re-implement the same go-template-style render, and the markdown
  rendering is the part that needs to look right inside an LLM
  prompt. We kept JSON for CI/diagnostics, markdown for prompts; one
  builder, two renderers.
- **Reusing the shengen parser.** shengen lives in `cmd/shengen/` as
  its own binary and module. Importing it into `sb` would have pulled
  the Go-codegen path into the engine and broken the "engine knows
  nothing about target languages" property. The duplicated parser is
  ~200 lines, stdlib only, and only does the classification work the
  context output needs — no codegen.
- **Letting prompts read `sb.toml` directly.** This was the existing
  shape. Rejected explicitly: the manifest format will keep evolving
  (Wave 1 just added `[[gates]]`, more is plausible), and pinning
  prompts to its surface meant every change forced a coordinated
  prompt update. `sb context` is the contract; the manifest is the
  implementation detail.

## What's open

- The embedded spec parser doesn't share a corpus with shengen's
  parser. Both have parsed every spec in `examples/` correctly, but
  there is no test ensuring the two never disagree on classification.
- `BackpressureInfo.Summary` is a one-line crib of the latest log
  block. Richer summaries (per-gate breakdown, trend across iterations)
  are a future direction.
- `--format json` does not emit a schema document. A `sb context
  --schema` mode that prints the JSONSchema for the output would let
  third-party harnesses validate without reading Go.
