---
date: 2026-05-05T22:35:00Z
researcher: claude
git_commit: 8b1f37f
branch: claude/discharge-reports-audit-z9FNM
repository: pyrex41/Shen-Backpressure
topic: "Wave 4 — discharge reports as audit-grade verification artifacts"
tags: [research, sb-engine, wave-4, discharge-reports, audit, post-hoc]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Wave 4 — Discharge reports as audit-grade verification artifacts

Post-hoc record of the Wave 4 work landed on
`claude/discharge-reports-audit-z9FNM` against `8b1f37f` (the
post-PR-#11 contraction baseline). Phase results live in
`2026-05-05-wave-4-results.md`; the schema design lives in
`2026-05-05-discharge-report-schema.md`.

## What problem this wave solved

Waves 1-3 made the manifest the source of truth for gate topology and
piped a structured project context into the agent loop. Gate
pass/fail was the only verification signal, however: a successful run
produced a green "5/6 gates passed" line and nothing else durable
about *what* had been verified or *how*.

Two readers were under-served:

- **The agent loop.** `sb context` told the harness about guard types
  and gate names but said nothing about what the spec-equivalence
  oracle had checked, what passed, what failed, with what input. When
  shen-derive caught a divergence the agent only saw the raw
  `go test` output piped through `plans/backpressure.log` — not a
  structured pointer to which rule, which premise, which case.
- **A human auditor.** The same project ships into security reviews,
  SOC-2 evidence files, and customer compliance questionnaires. None
  of those readers run `sb gates`. They want a stable, reviewable
  artifact that answers: which Shen rules were checked, against which
  spec hash, on which commit, with what kind of evidence, with what
  rationale for each premise. Today there was no such artifact;
  evidence lived in transient log lines.

The user-visible pain: the engine knew enough to build a structured
verification record, but emitted the same opaque pass/fail it had
since Wave 1. The mixed-evidence-report and counterexample-traces
feature design prompts (under `thoughts/shared/research/`) had been
sitting unimplemented for a month.

## What was built

A new discharge-report system with three coupled components:

### shen-derive/report/ — schema + classifier (Phase 1-3)

- `shen-derive/report/schema.go` — `Report`, `Rule`, `Premise`, and
  `CounterExample` Go types matching the locked
  `schema_version: 1` JSON shape. Constants for `discharge`,
  `discharge_basis`, and `status` values. `signature` is a typed
  pointer that is always nil in v0 — reserved for a follow-up
  signing integration.
- `shen-derive/report/classify.go` — `ClassifyDatatypes` walks the
  parsed spec's `(datatype …)` blocks and emits one rule per block
  with one premise per value premise (`X : type`) and one premise per
  verified expression (`(>= X 0) : verified`). Value premises
  discharge via `guard-type-at-boundary`; verified premises discharge
  via `guard-constructor-validates`. `ClassifyDefine` emits a single
  runtime-sampled premise capturing the spec ≡ impl agreement.
- `shen-derive/report/build.go` — `Build()` finalises a partial
  report by hashing the spec file, sorting rules by name,
  recomputing the summary, and serialising as canonical JSON.
  `WriteFile()` writes atomically via tempfile + rename.
- `shen-derive/report/guardrefs.go` — `readGuardLines` scans a
  shengen-emitted Go file for `// Shen: (datatype NAME)` markers and
  builds a `name → line-of-type-decl` map used to populate
  `code_references` on each premise.

### shen-derive `:doc` annotation parser (Phase 1)

- `shen-derive/specfile/parse.go` — `extractDocAnnotations` scans the
  raw spec text (before comment stripping) for
  `\* :doc "..." *\` block-comment annotations and pairs each with
  the next `(datatype NAME` or `(define NAME`. New `Datatype.Doc`
  and `Define.Doc` fields hold the result, consumed by the
  classifier's `human_description` selector. Annotations are optional;
  rules without one fall back to a clearly-labelled
  auto-generated description (`human_description_source:
  "auto-generated"`).

### shen-derive verify --report-out (Phase 2)

- `shen-derive/main.go` — two new flags on `verify`:
  - `--report-out PATH` — write a per-spec discharge report (JSON)
    to PATH alongside the existing test file.
  - `--guard-file PATH` — point shen-derive at the
    shengen-emitted guards file so the report can include
    `code_references` for each guarded type.
  The existing test-file output is unchanged (backward compatible).

### sb derive aggregation, history, counter-examples (Phase 3-4)

- `cmd/sb/discharge.go` — schema mirror types matching
  shen-derive's report package byte-for-byte (the two are separate
  Go modules, so the JSON wire format is the contract). Helpers:
  `loadDischarge` / `writeDischarge`, `mergeDischargeReports`,
  `fillImplGit` (`git rev-parse HEAD` + `git status --porcelain`),
  `parseGoTestFailures` (two regexes: `=== RUN
  TestSpec_<Func>/case_NN` for the case context, then the body line
  `case_NN: spec says X, impl returned Y` for the divergence
  values), `applyCounterExamples`, `rotateDischargeHistory` with
  configurable retention (default 50), and
  `computeDischargedSinceCommit` walking the history newest-first.
- `cmd/sb/derive.go` — for every Go derive spec the engine now also
  asks shen-derive for a per-spec report, collects the partials,
  runs `go test -v` capturing combined output, parses failures,
  patches counter-examples in, fills git/tools metadata,
  recomputes the summary, writes
  `.sb/discharge_report.json`, and rotates a time-stamped copy into
  `.sb/history/`. The path-normalisation step
  (`normaliseDischargePaths`) rewrites the absolute paths
  shen-derive received into the relative paths the user wrote in
  `sb.toml`, so the artifact is repo-portable.

### sb context — terse rendering (Phase 5)

- `cmd/sb/context.go` — `ProjectContext.Discharge` is a new
  optional `*DischargeContextInfo` projected from the latest report.
  Rendered as a "Discharge Report" Markdown section between
  "Derive Coverage" and "Backpressure". Five-second skim format:
  premise counts split by static / sampled / unproven, rule-count
  pass/fail, per-violation case ID + spec/impl outputs + impl file
  + reproduction command, footer pointing at the JSON for tooling.
  When no report exists the section is omitted entirely (per
  Wave 4 prompt).

### sb audit-report — long-form rendering (Phase 6)

- `cmd/sb/audit_report.go` — new `sb audit-report` command. Reads
  `.sb/discharge_report.json` (or any historical report via
  `--in .sb/history/...`) and emits a Markdown document targeted at
  a security reviewer. Sections: header (commit + spec hash + tool
  versions), summary, per-rule sections (description with source
  marker, spec excerpt fenced as `shen`, premise table with
  rationale, sample stats, code references, counter-examples), and
  a "How to Read This Report" appendix that explains the static /
  runtime-sampled / unproven taxonomy and what the artifact
  explicitly does not claim. `--format=json` is a passthrough for
  piping into other tools.

### Demo + documentation (Phase 7-8)

- `examples/payment/README.md` — third walkthrough step
  demonstrating the discharge report in green and broken states,
  with expected `summary` JSON and the violated-state log line.
  Updated the "What's here" tree (`.sb/discharge_report.json`,
  `.sb/history/`) and tool-level entry-points table.
- `README.md` — new "Discharge Reports — Audit-Grade Verification
  Artifacts" section after the gate table, before Quick Start.
  Frames the dual purpose explicitly and calls out what
  audit-grade does not mean.

## What was rejected

- **Importing shen-derive's report package into cmd/sb.** They are
  separate Go modules. Cross-module imports would force one module
  to depend on the other's go.mod, breaking the long-standing
  property that `cmd/sb` is engine-agnostic and `shen-derive` is
  one of several pluggable verifiers. The duplicated schema mirror
  in `cmd/sb/discharge.go` is small, the JSON wire format is the
  contract, and a unit test (future work) can pin the two
  representations together.
- **Per-premise counter-example attribution.** The schema's
  `counter_examples` shape places witnesses at the rule level for
  v0. Per-premise attribution requires premise-level test
  instrumentation (the generated test would have to evaluate each
  premise of the spec individually). The schema reserves
  `counter_examples[].failed_premise_id` as an additive v1.x field;
  no instrumentation lands in v0.
- **A new top-level gate.** The Wave 4 prompt was explicit: the
  discharge-report functionality runs as part of the existing
  shen-derive gate. Adding a 7th gate would have changed pass/fail
  semantics; the engine still passes 5/6 (or 6/6 with shen-sbcl)
  because the report is a side effect.
- **Off-machine sync.** `.sb/history/` stays local in v0. Persisting
  evidence for compliance is a separate integration (S3 sync,
  attestation server, GitHub Actions artifact upload) and lives
  outside this wave.
- **Per-rule counter-example minimisation.** Running shrink loops on
  failing inputs would produce smaller witnesses for auditors. Out
  of scope for v0; the existing samples are deterministic and
  reproducible, which is sufficient.
- **A flow analyser.** The Phase 3 static-discharge classification
  is intentionally an approximation: it asserts that the
  shengen-emitted constructor + Go's type system enforces the
  premise. A real cross-function flow analyser is a future feature.
  The schema's `discharge_basis` field is forward-compatible with
  more rigorous bases (`flow-analysis`, `prover-z3`).

## What's open

- **TS path.** shen-derive-ts does not yet implement `--report-out`.
  Projects whose only derive specs are TypeScript get an aggregated
  report covering only their Go specs (none, in
  `examples/shen-web-tools/`'s case). Adding the flag to the TS
  port mirrors the Go change.
- **Schema test.** The schema mirror in `cmd/sb/discharge.go` and
  the canonical schema in `shen-derive/report/schema.go` are
  expected to round-trip identical bytes. A unit test that loads
  one and serialises through the other (and vice-versa) would lock
  the wire-format contract and catch silent drift.
- **Determinism.** `generated_at` is the only intentional
  non-determinism. A flag (`--frozen-time` or similar) for
  reproducible-build use cases is plausible but not implemented.
- **Counter-example input rendering.** The current `Input` map
  contains a single placeholder string pointing back at the
  generated test file. Auditors who want the raw inputs in the
  report itself need a richer marshaller — straightforward but not
  shipped.
- **`discharged_since_commit` audit semantics.** v0 scans local
  history. A team running gates only on CI will have an empty local
  history, so the field will say "since the current commit" on
  every fresh clone. The fix is off-machine sync (see "Off-machine
  sync" rejection above).
