---
date: 2026-05-05T18:30:00Z
researcher: claude
git_commit: 448d048
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Contraction pass — Phase 7 verification + summary of changes"
tags: [research, contraction, results, demo-readiness, post-pass]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Contraction pass — Phase 7 verification

Companion to `2026-05-05-contraction-baseline.md`. Records what
changed across phases 0–6 and confirms nothing that worked in the
baseline regressed.

## Phase-by-phase outcome

| Phase | Result |
|---|---|
| 0. Baseline | Captured pre-contraction gate output and pre-contraction repo state. Three baseline failures per example are environment-level (missing `shen-sbcl`, missing helper shell scripts) and not in scope. |
| 1. Archive examples | `git mv` of `category-showcase/`, `order-state-machine/`, `shenguard-bolt-on/` into `examples/.archive/`. `examples/` now holds exactly four entries (payment, multi-tenant-api, shen-web-tools, .archive). `examples/.archive/README.md` updated with a one-line description per archived directory. |
| 2. Skill bundle dedup | Already resolved on this branch: `sb/` and `cmd/sb/skilldata/` are byte-equal, `cmd/sb/build.sh` rebuilds the mirror before every binary build, `make check-skilldata` enforces equality. Made `sync-skilldata` portable (dropped rsync dependency) and added a comment block at the top of `cmd/sb/build.sh` documenting the chosen pattern (mirror checked in, drift caught in CI). |
| 3. Root markdown cleanup | Already done in earlier work: `EXPLORATION.md`, `heavy_analysis.md`, `heavy_analysis_2.md`, `temp_prompt.md`, and the shen-bend feasibility prompt all already live under `thoughts/shared/research/` with appropriate dated filenames. Repo root contains only `README.md`, `Makefile`, `.gitignore`, `skm.toml`, and the standard subdirectories. No edits needed. |
| 4. Wave research memos | Wrote three memos in `thoughts/shared/research/` (Wave 1 manifest gates, Wave 2 `sb context`, Wave 3 prompt hydration). Each ~400–550 words, follows existing front-matter format, references the relevant commits and code paths, records what was rejected and what is open. |
| 5. README rewrite | Rewrote `README.md` to lead with the AI-coding-loop framing. Polyglot and multi-language stories are now under "What Else This Does." Removed enumerations of archived examples; the only examples named are the three retained ones. Added a `/sb:derive` row to the commands table and a third install option (`sb init`). Linked to the three new wave memos under "Engine Architecture." All concrete commands and file paths in the rewritten README resolve. |
| 6. Payment example tutorial | Rewrote `examples/payment/README.md`. Removed the obsolete `/tmp/shen-go` install instructions and the pre-Wave Ralph orchestrator output. New structure: link to top README, "what you'll see" overview, prerequisites, two-step five-minute walkthrough (canonical pass, then `demo-shen-derive/run.sh` for the bug-finding demo), the spec-and-codegen story, "what's here" tree, and a tool-entry-point reference table. |

## Phase 7 verification

Re-ran `sb gates` in each retained example. Pass/fail set is identical
to the Phase 0 baseline:

| Example | Baseline | Post-contraction |
|---|---|---|
| `examples/payment/` | 2/6 (build, shen-derive) | 2/6 (build, shen-derive) |
| `examples/multi-tenant-api/` | 2/5 (test, build) | 2/5 (test, build) |
| `examples/shen-web-tools/` | 3/6 (test, build, shen-derive) | 3/6 (test, build, shen-derive) |

All baseline failures recur verbatim and remain environment-level
(missing `shen-sbcl` runtime, missing `bin/shengen-codegen.sh` and
`bin/shenguard-audit.sh` shell scripts in the example bin/
directories). No regressions introduced by the contraction.

`sb init` in a fresh temporary directory installs the canonical skill
bundle: every file under `.claude/commands/sb/` and
`.claude/skills/shen-backpressure/` is byte-equal to its `sb/`
counterpart.

Internal links in `README.md`, `examples/payment/README.md`,
`examples/shen-web-tools/README.md`, and `examples/.archive/README.md`
all resolve to existing files.

## What's still in scope for future passes

Out of scope for this contraction; flagged for the next pass:

- `examples/payment/` ships a default `bin/` that is missing
  `shengen-codegen.sh` and `shenguard-audit.sh`. Either restore them
  (so `sb gates` passes 5/6 instead of 2/6 in a fresh clone) or
  document explicitly that those gates require `sb init`-style
  scaffolding.
- `examples/payment/reference/guards_gen_test.go` declares
  `package shengen/payment` which is not in the example's go module —
  it fails `go test ./...` with a setup error. Either fix the import
  or move the file under a `reference/.disabled/` directory so it
  doesn't drag down the test gate.
- The two retained example manifests (`examples/payment/sb.toml`,
  `examples/shen-web-tools/sb.toml`) still use the legacy
  `[commands]` shape rather than `[[gates]]`. Adding a `[[gates]]`
  block in at least one would dogfood the new format.
- `examples/multi-tenant-api/` has no `sb.toml` and no `README.md`.
  The 2026-04-16 readiness doc treated it as Tier-A on the strength
  of `demo.md` and `transcript/`; a thin README that points at those
  artifacts would help first-time readers.
- The Wave-1 memo notes the duplicated decode helpers between
  `tomlConfigNew` and `tomlConfigLegacy`. Collapsing the two parsers
  is mechanical and small.

None of the above is required for the contraction itself to be
considered done.

## Changed files (high-level)

- `examples/`: 3 directories archived; `examples/.archive/README.md`
  rewritten as inventory.
- `Makefile`, `cmd/sb/build.sh`: portability + comment.
- `README.md`: full rewrite.
- `examples/payment/README.md`: full rewrite.
- `thoughts/shared/research/`: 4 new memos
  (`2026-05-05-contraction-baseline.md`,
  `-wave-1-manifest-driven-gates.md`,
  `-wave-2-sb-context.md`,
  `-wave-3-prompt-hydration.md`,
  plus this file).

No engine code (`cmd/sb/`, `cmd/shengen*/`, `shen-derive/`) was
modified. No example application code was modified. No new features
or abstractions were added.
