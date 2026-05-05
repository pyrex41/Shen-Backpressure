---
date: 2026-05-05T17:55:00Z
researcher: claude
git_commit: 6f367c8959a0400b9013f30844927ee087db9495
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Wave 1 — manifest-driven gates"
tags: [research, sb-engine, wave-1, gates, manifest, post-hoc]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Wave 1 — Manifest-driven gates

Post-hoc record of the Wave 1 engine work landed in commit `588eec1`
(`sb+shen-derive: Wave 1 — manifest-driven gates + parser hardening`,
2026-04-10) and the follow-up `sb: add lang field to derive specs for
Go/TS dispatch` (`ce84129`).

## What problem this wave solved

The pre-Wave engine ran a fixed five-gate pipeline: shengen → test →
build → shen-check → tcb-audit. The pipeline lived in code, with
parallelism toggled by a single `[gates] relaxed = true` boolean. That
worked for the payment example but did not generalise:

- Polyglot examples (`shen-web-tools`) needed a different gate shape:
  `npx tsc --noEmit` instead of `go build`, `npm test` instead of `go
  test`, no `tcb-audit` shell script.
- shen-derive could not become a sixth gate without hard-coding it into
  the engine — and even then there was no way to express "run this
  gate only after build passes" or "run these two in parallel."
- Projects with a custom audit step (e.g. running `git diff --exit-code
  generated/`) had nowhere to declare it. The only escape hatch was
  rewriting the five `[commands]` to point at user shell scripts that
  fanned out internally — which removed the engine's per-gate timing
  and pass/fail reporting.

The user-visible pain: the engine's promise was "ship a deterministic
gate runner you can trust"; in practice every project had to bend to
five gates exactly, in one fixed order, with one fan-out point.

## What was built

A new `[[gates]]` array of tables in `sb.toml` lets a manifest declare
its own gate topology. Each entry has `name`, `run`, optional `kind`
(defaults to `command`; `derive` is reserved for the shen-derive gate),
and optional `parallel_group`. Gates with the same `parallel_group`
that appear contiguously in the array fan out via a `WaitGroup`; the
next non-matching gate waits for the group to finish.

Key code paths:

- `cmd/sb/config.go:22-27` — `GateDef` and the `GateKind` enum.
- `cmd/sb/config.go:60` — `Config.HasManifestGates()` is the toggle the
  rest of the engine branches on.
- `cmd/sb/config.go:166-227` — two-pass TOML decode: try the new
  `[engine]` + `[[gates]]` shape first; on empty `Gates` slice, fall
  back to the legacy `[gates]` table + `[commands]` fields.
  `applyProjectPaths`/`applyCommands`/`applyDerive`/`applyLoop` are
  the small helpers shared between both passes.
- `cmd/sb/gates.go:107-159` — `buildGateList(cfg)` produces the unified
  `[]gate` slice from either format and unconditionally appends the
  `shen-derive` gate when `[[derive.specs]]` is non-empty.
- `cmd/sb/gates.go:198` — `runGateList` walks the slice, batching
  contiguous same-`parallel_group` entries into a `WaitGroup`.
- `cmd/sb/derive.go:113` — the `lang` field on `[[derive.specs]]` (added
  in `ce84129`) selects between Go and TS shen-derive backends.

`gateResult` was enriched with separate `stdout`, `stderr`, and
`exitCode` so the loop's prompt-hydration step can splice the right
stream into backpressure feedback.

## What was rejected

- **Full DAG with explicit dependencies** (`depends_on = ["test"]`).
  Rejected as overkill. Linear declaration order with same-group fan-out
  expresses every shape we cared about (sequential pipeline, "fast
  group" of mutually independent gates) without the parsing or display
  complexity. If a project ever needs a real DAG, the manifest can be
  extended; nothing in the array form blocks that.
- **Removing the legacy `[commands]` block in the same change.**
  Existing projects (`payment/`, `shen-web-tools/`) would have had to
  migrate immediately. Keeping both formats in the parser and letting
  the new format win when present made migration opt-in, which is what
  the README's "Migration Guide" section now describes.
- **Embedding the shen-derive gate as just another `[[gates]]` entry
  the user has to add.** Rejected because every shen-derive project
  already declares `[[derive.specs]]`, and there is exactly one right
  way to wire the gate. `buildGateList` auto-appends the `derive` gate
  whenever `cfg.DeriveSpecs` is non-empty, regardless of whether the
  manifest uses the legacy or new gate format
  (`cmd/sb/gates.go:147-156`).

## What's open

- Neither retained example uses the new `[[gates]]` format. Both still
  declare paths and let the engine synthesise the legacy five-gate
  pipeline. The new format works (it has tests) but is not
  dogfooded in the curated tree.
- The two-pass parser duplicates field decoding between
  `tomlConfigNew` and `tomlConfigLegacy`. The duplication is small
  enough to live with, but it is real and a future merge of the
  formats could collapse it.
- `parallel_group` is a string with no validation — typos silently
  produce sequential execution. A linter pass over `buildGateList`
  could warn when a group has only one member or when the same group
  string appears non-contiguously.
