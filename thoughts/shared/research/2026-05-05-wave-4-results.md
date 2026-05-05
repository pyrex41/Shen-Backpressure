---
date: 2026-05-05T22:35:00Z
researcher: claude
git_commit: 8b1f37f
branch: claude/discharge-reports-audit-z9FNM
repository: pyrex41/Shen-Backpressure
topic: "Wave 4 — discharge reports results"
tags: [research, sb-engine, wave-4, discharge-reports, results]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Wave 4 results — discharge reports

Captured at the end of Wave 4 implementation. Compares against the
Phase 0 baseline (`2026-05-05-wave-4-baseline.md`).

## Gate output (Phase 8)

```
PASS [shengen] 10ms
PASS [test] 150ms
PASS [build] 252ms
FAIL [shen-check] 2ms      ← unchanged: no Shen runtime in this image
PASS [tcb-audit] 15ms
PASS [shen-derive] 235ms

  PASS  shengen        10ms
  PASS  test           150ms
  PASS  build          252ms
  FAIL  shen-check     2ms
  PASS  tcb-audit      15ms
  PASS  shen-derive    235ms

5/6 gates passed
```

Same `5/6` shape as the Phase 0 baseline. Wave 4 adds a side-effect
artifact (`.sb/discharge_report.json`) without changing the existing
gates' pass/fail semantics.

## New artefact

`examples/payment/.sb/discharge_report.json` summary on the canonical
implementation:

```json
{
  "rule_count": 7,
  "rules_discharged": 7,
  "rules_violated": 0,
  "rules_unproven": 0,
  "premises_total": 14,
  "premises_static": 13,
  "premises_runtime_sampled": 1,
  "premises_unproven": 0
}
```

Six datatypes (`account-id`, `amount`, `transaction`, `balance-invariant`,
`account-state`, `safe-transfer`) plus the `processable` define block
yield 14 premises: 13 statically discharged via guard types or guard
constructors, 1 runtime-sampled by shen-derive's 35-case oracle.

When `demo-shen-derive/run.sh` swaps in `bug2_b0_truncate.go.bak`, the
report flips: `rules_violated: 1`, `processable.oracle-spec-equiv`
gains a `case_25` counter-example with spec output `true` vs impl
output `false` and the reproduction command
`go test -run TestSpec_Processable/case_25`.

## sb context output (terse rendering)

`sb context` adds a Discharge Report section after the existing Derive
Coverage section, before any backpressure block. On the canonical pass
case:

```text
### Discharge Report

13 premises proven statically (via guard types)
1 premises sampled clean (deterministic seed)
7/7 rules discharged.

Latest report: .sb/discharge_report.json (full JSON for tooling)
```

On a violated state (after seeding a bug), the same section emits the
case-level counter-example with spec/impl outputs, impl file, and a
`go test -run …` reproduction line — a five-second skim suitable for
agent-loop backpressure.

## sb audit-report output (long-form rendering)

`sb audit-report` produces a 257-line Markdown document covering: a
header with project commit + spec hash + tool versions, a per-rule
section for each datatype/define block (plain-English description,
spec excerpt, premise table with discharge classification and
rationale, code references into `guards_gen.go`,
"continuously discharged since" pointer), counter-examples on any
violated rules, and a "How to read this report" appendix that
explains the static / runtime-sampled / unproven taxonomy.

The Markdown rendering passed an internal legibility check: a reader
unfamiliar with Shen-Backpressure can identify what is being verified,
what status each rule is in, and what evidence backs each claim.

## Phase-by-phase summary

- **Phase 0** — captured `5/6` baseline; mapped shen-derive's data
  flow; recorded existing files Wave 4 would touch. Output:
  `2026-05-05-wave-4-baseline.md`.
- **Phase 0.5** — read three relevant feature_ideas
  (compliance-audit-trail, counterexample-traces, mixed-evidence-
  report). They informed schema decisions and are quoted in the
  schema design doc.
- **Phase 1** — locked `schema_version: 1` with rationale in
  `2026-05-05-discharge-report-schema.md`. Built
  `shen-derive/report/` package: schema types, classifier, builder,
  guard-line lookup. Wrote `:doc` annotation parser in `specfile/`.
- **Phase 2** — wired `shen-derive verify --report-out` so each
  per-spec verify run emits a JSON report covering classified
  datatype premises plus the define's runtime-sample premise.
- **Phase 3** — static-discharge classification: value premises
  typed at the function boundary discharge via
  `guard-type-at-boundary`; verified premises
  (`(>= X 0) : verified`) discharge via
  `guard-constructor-validates`. The classification is faithful to
  what shengen-emitted Go does; not a separate analyser.
- **Phase 4** — `sb derive` aggregates per-spec partial reports,
  runs `go test`, parses failures with two regexes
  (`=== RUN ...case_NN` and the test body's `case_NN: spec says X,
  impl returned Y`), patches counter-examples, fills git
  commit/dirty flags, walks `.sb/history/` for
  `discharged_since_commit`, writes `.sb/discharge_report.json`,
  and rotates a time-stamped copy into `.sb/history/`. Retention
  default is 50 entries; `.sb/` was added to `.gitignore`.
- **Phase 5** — `sb context`'s `ProjectContext` gained an optional
  `Discharge` field; `RenderMarkdown` emits the terse Discharge
  Report section between Derive Coverage and Backpressure (omitted
  entirely when no report exists, per Wave 4 spec).
- **Phase 6** — `sb audit-report` reads
  `.sb/discharge_report.json` (or any
  `.sb/history/<timestamp>-<commit>.json` via `--in PATH`) and
  emits long-form Markdown (default) or passthrough JSON.
- **Phase 7** — extended `examples/payment/README.md` with a third
  walkthrough step demonstrating the discharge report on the green
  case and on the broken-impl case. Updated the "What's here" tree
  and tool-level entry-points table.
- **Phase 8** — wrote the README intro, this results memo, and the
  Wave 4 memo (`2026-05-05-wave-4-discharge-reports.md`).

## Files added or changed

New:

- `shen-derive/report/schema.go` — locked v1 schema types.
- `shen-derive/report/classify.go` — datatype + define classifier.
- `shen-derive/report/build.go` — report builder, hash, JSON write.
- `shen-derive/report/guardrefs.go` — guards file line lookup.
- `cmd/sb/discharge.go` — schema mirror, aggregation, history,
  go-test failure parsing.
- `cmd/sb/audit_report.go` — `sb audit-report` command.
- `thoughts/shared/research/2026-05-05-discharge-report-schema.md`
- `thoughts/shared/research/2026-05-05-wave-4-baseline.md`
- `thoughts/shared/research/2026-05-05-wave-4-discharge-reports.md`
- `thoughts/shared/research/2026-05-05-wave-4-results.md`

Changed:

- `shen-derive/main.go` — `--report-out` and `--guard-file` flags
  on `verify`.
- `shen-derive/specfile/parse.go` — `Datatype.Doc` and
  `Define.Doc`; `:doc "..."` annotation parser.
- `cmd/sb/derive.go` — invoke shen-derive with `--report-out`,
  collect per-spec reports, parse failures, finalise project-level
  report and history.
- `cmd/sb/context.go` — `DischargeContextInfo`,
  `readDischargeContext`, terse Markdown section.
- `cmd/sb/main.go` — register `audit-report`.
- `README.md` — discharge-reports section after the gate table,
  before Quick Start.
- `examples/payment/README.md` — third walkthrough step + What's
  here update.
- `.gitignore` — `.sb/`.

## Known limitations (carried into the wave-4-discharge-reports memo)

- TS specs (shen-derive-ts) don't yet emit a partial report. Wave 4
  reports for TS-only projects are skipped silently. Adding the
  `--report-out` flag to shen-derive-ts is a small follow-up.
- Counter-example inputs are stringified placeholders that point
  back to the generated test file rather than reproducing the full
  Go expression. Per-premise attribution and richer input rendering
  are reserved for v1.x.
- `discharged_since_commit` is currently the most-recent commit on
  any history entry where the rule was non-discharged; the schema
  promises this is "best-effort" and recomputed every run from the
  visible history.
- `examples/payment/internal/shenguard/guards_gen.go` carries a
  pre-existing path-comment drift (`demo/payment/specs/core.shen`
  vs `specs/core.shen`) that gets corrected the moment Gate 1
  regenerates it. Out of scope for Wave 4; not introduced by Wave
  4.
