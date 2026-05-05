---
date: 2026-05-05T22:30:00Z
researcher: claude
git_commit: 8b1f37f
branch: claude/discharge-reports-audit-z9FNM
repository: pyrex41/Shen-Backpressure
topic: "Wave 4 — discharge report schema"
tags: [research, schema, wave-4, discharge-reports, audit]
status: locked-v1
last_updated: 2026-05-05
last_updated_by: claude
---

# Discharge report schema (v1)

This is the load-bearing schema document for Wave 4. The JSON shape
written here is `schema_version: 1`. Future evolution must be additive
unless the version field is bumped.

## Goal

Produce, on every gate run, a structured artifact that distinguishes
how each premise of each Shen rule was discharged. The artifact serves
two readers:

- **The agent loop**, via a terse Markdown rendering injected through
  `sb context`. Failed premises get concrete counter-examples that
  the agent can act on.
- **A human auditor**, via a long-form Markdown rendering produced by
  `sb audit-report`. The artifact answers "what was verified, against
  which spec, on which commit, with what kind of evidence" without
  the reader having to know Shen.

The same JSON file feeds both renderings; a single source of truth.

## Top-level shape

```json
{
  "schema_version": 1,
  "generated_at": "2026-05-05T22:30:00Z",
  "spec": {
    "files": [
      {"path": "specs/core.shen", "sha256": "..."}
    ],
    "rule_count": 7
  },
  "impl": {
    "git_commit": "8b1f37f...",
    "git_dirty": false,
    "target_languages": ["go"]
  },
  "tools": {
    "sb_version": "0.3.0",
    "shen_derive_version": "0.3.0",
    "shengen_version": "...",
    "shen_runtime": null,
    "shen_runtime_available": false
  },
  "rules": [ ... ],
  "summary": {
    "rule_count": 7,
    "rules_discharged": 7,
    "rules_violated": 0,
    "premises_total": 18,
    "premises_static": 14,
    "premises_runtime_sampled": 4,
    "premises_unproven": 0
  },
  "signature": null
}
```

### Required vs optional

| Field | Required | Notes |
|---|---|---|
| `schema_version` | required | Always `1` for this version. Bump only on breaking change. |
| `generated_at` | required | RFC3339 UTC. Excluded from byte-identity comparison. |
| `spec.files[]` | required | At least one entry. Multiple entries are reserved for future multi-spec projects. |
| `spec.files[].sha256` | required | Hex-encoded SHA-256 of the file's bytes. Lets a stored report be associated with a specific spec version. |
| `impl.git_commit` | optional | Null if the project is not in a git repo or the commit can't be resolved. |
| `impl.git_dirty` | optional | True if the working tree has uncommitted changes. Null when `git_commit` is null. |
| `impl.target_languages` | required | List of target languages this report covers (`go`, `ts`). Reserved for future multi-target reports. |
| `tools.shen_runtime` | optional | Null when no runtime is installed. The string when present is the runtime banner. |
| `rules[]` | required | One entry per Shen `(datatype …)` and `(define …)` block in the spec. May be empty if the spec is empty. |
| `summary` | required | Always present, never null; counts can be zero. |
| `signature` | optional | Reserved for v0+. Always `null` in v0. |

## Per-rule shape

```json
{
  "name": "balance-invariant",
  "kind": "guarded",
  "spec_file": "specs/core.shen",
  "spec_excerpt": "(datatype balance-invariant ...)",
  "human_description": "A BalanceChecked value must hold a non-negative balance covering all transaction amounts.",
  "human_description_source": "auto-generated",
  "premises": [ ... ],
  "status": "discharged",
  "discharged_since_commit": "abc123...",
  "counter_examples": []
}
```

| Field | Required | Notes |
|---|---|---|
| `name` | required | Shen name as written in the spec (`balance-invariant`, `processable`). |
| `kind` | required | One of: `wrapper`, `constrained`, `composite`, `guarded`, `alias`, `sumtype`, `define`. Mirrors `specfile.TypeCategory` plus `define`. |
| `spec_file` | required | Relative path to the spec file containing this rule. |
| `spec_excerpt` | required | Multi-line spec block, verbatim. |
| `human_description` | required | Plain-English summary. Required even when no `:doc` annotation exists. |
| `human_description_source` | required | One of: `:doc` (sourced from a `:doc "..."` annotation in the spec), `auto-generated` (synthesised). Auditors can tell whether a human signed off on the description. |
| `premises` | required | At least one entry. Empty rules are filtered out. |
| `status` | required | One of: `discharged`, `violated`, `unproven`. `unproven` means at least one premise is `unproven` and none is `violated`. |
| `discharged_since_commit` | optional | Null on the first run or when status is not `discharged`. Walks `.sb/history/` to find the last commit where status changed. |
| `counter_examples` | optional | Empty array when status is `discharged` or `unproven`. Non-empty when `violated`. |

### Premise shape

```json
{
  "id": "bal-is-number",
  "expression": "Bal : number",
  "discharge": "static",
  "discharge_basis": "guard-type-at-boundary",
  "rationale": "Bal field of BalanceChecked is typed float64 in guards_gen.go; Go's type system prevents non-number values from reaching this premise.",
  "code_references": ["internal/shenguard/guards_gen.go:64"],
  "samples_passed": 0,
  "samples_failed": 0,
  "sample_seed": null
}
```

| Field | Required | Notes |
|---|---|---|
| `id` | required | Stable identifier scoped to the rule. Format: `rule-name.premise-slug`. Stable across runs unless the spec changes. |
| `expression` | required | Spec text of the premise: `Bal : number`, `(>= Bal (head Tx)) : verified`, `processable B0 Txs ≡ impl(B0, Txs)`. |
| `discharge` | required | One of: `static`, `runtime-sample`, `unproven`. Schema reserves `runtime-assertion` and `prover` for future use; v0 emits only the three above. |
| `discharge_basis` | required | A short stable token describing how `discharge` was determined. v0 values: `guard-type-at-boundary`, `guard-constructor-validates`, `shen-derive-sampled`, `not-discharged`. Future values: `flow-analysis`, `prover-z3`, etc. |
| `rationale` | required | One-sentence English justification for `discharge` + `discharge_basis`. |
| `code_references` | optional | Array of `path:line` pointers into the impl that justify the discharge. Empty for `runtime-sample` and `unproven`. |
| `samples_passed` | required when `discharge=runtime-sample` | Count of sampled inputs where spec and impl agreed. |
| `samples_failed` | required when `discharge=runtime-sample` | Count of sampled inputs where spec and impl disagreed. Non-zero implies parent rule status is `violated`. |
| `sample_seed` | required when `discharge=runtime-sample` | The seed used. `"deterministic-default"` when the boundary pool was used without a seed; otherwise the integer seed as a string. |

### Counter-example shape (rule-level, v0)

```json
{
  "case_id": "case_07",
  "input": {"b0": "100.5", "txs": "[Transaction{amount=50.5, …}]"},
  "spec_output": "true",
  "impl_output": "false",
  "impl_function": "Processable",
  "impl_file": "internal/derived/processable.go",
  "impl_line_hint": null,
  "first_seen_commit": "abc123...",
  "rationale": "Spec evaluates processable to true on this input; impl returned false."
}
```

| Field | Required | Notes |
|---|---|---|
| `case_id` | required | Matches the Go test name (`case_07`) so a reviewer can re-run a single case via `go test -run TestSpec_Processable/case_07`. |
| `input` | required | Map from param name to a stringified Go expression. Strings are intentionally lossy; the canonical input lives in the generated test file. |
| `spec_output` | required | Stringified Go literal for what the Shen spec produced. |
| `impl_output` | required | Stringified Go literal for what the impl produced. |
| `impl_function` | required | Symbol name. |
| `impl_file` | optional | Relative path to the impl source file. Best-effort. |
| `impl_line_hint` | optional | Best-effort line number. Null when not resolvable. |
| `first_seen_commit` | optional | When the violation first appeared, computed by walking `.sb/history/`. Null on first run. |
| `rationale` | required | One-sentence description of the divergence. v0 generates a generic message; richer "likely cause" analysis is reserved for future work. |

## Schema design decisions

### Per-premise vs. per-rule discharge classification

**Decision: per-premise.** Quoted from the prompt: *"the whole point of
mixed-evidence is that some premises are static while others are
runtime; collapsing this loses the feature."* This matches the
mixed-evidence-report feature_idea ("what did the tool prove
structurally, what did it check by generated samples?"). The
classifier classifies each premise; the rule's status is a roll-up.

### Counter-example placement: rule level for v0

**Decision: rule-level.** Per-premise attribution requires premise-level
test instrumentation (the generated test would have to evaluate each
premise of the spec separately and report which premise failed). v0
emits counter-examples on the parent rule. The schema's `counter_example`
shape is forward-compatible — adding `failed_premise_id` as an optional
field in v1.x is additive.

### Hashing strategy

**Decision: SHA-256 of spec file bytes only.** Generated files
(`guards_gen.go`, `*_spec_test.go`) are intentionally not hashed in
this report — `tcb-audit` already covers drift on those. The discharge
report is about *what was verified*, not *what was generated*. A
follow-up could add `impl.source_hashes` covering the impl files; out
of scope for v0.

### `discharged_since_commit` source

**Decision: walk `.sb/history/` backward.** When a report is being
emitted, look at the previous N history entries; for each rule, find
the most recent commit where its `status` differs from the current
status. That commit's *successor* is `discharged_since_commit`. If
there is no history (first run, or all history shows the same status)
then this field is the current commit (or null when the current commit
is unresolvable).

### Versioning policy

**Decision: `schema_version: 1`.** Additive evolution only. Adding new
optional fields is permitted under v1. Adding required fields, renaming
fields, or changing field semantics requires `schema_version: 2`.
Consumers that don't recognise an optional field must ignore it.

### `human_description` is required

**Decision: required, with `human_description_source` to disclose
authorship.** Audit-grade output reads to a non-Shen reader. The
schema cannot ship without descriptions. Sources:

- `:doc` — the spec author wrote a description as a `:doc "..."`
  annotation. Treat as canonical.
- `auto-generated` — synthesised from the rule's structure (kind,
  fields, premise count). Auditors should treat these as raw, not as
  approved language.

A `:doc` annotation precedes a (datatype …) or (define …) block:

```shen
\* :doc "An Amount is a non-negative number representing money." *\
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)
```

The annotation is a Shen block comment with `:doc "..."` as its first
line. Easy to parse, doesn't need new spec syntax.

### What's reserved for future versions

These fields appear in the schema's *type* but are always null in v0:

- `signature`: cryptographic signature over the report's canonical
  byte form. Reserved for a follow-up signing integration. Documented
  null.
- `tools.shen_runtime` may be a string when a runtime is detected; the
  feature is wired but optional.
- Per-premise counter-example attribution. Schema reserves the
  optional field name `counter_examples[].failed_premise_id` (v1.x
  additive).

### What's NOT reserved (intentionally)

- `evidence_id`, `commands.log`, `manifest.lock.json`, `evidence-diff`:
  the compliance-audit-trail feature_idea proposes these as part of a
  larger evidence package. They live in a separate, larger feature; a
  discharge report alone is not the full audit-trail bundle. Wave 4
  builds the foundation.
- Differential reporting (`base/head` pairs from
  feature-differential-verification.md). Out of scope for v0; the
  history mechanism leaves room.

## Precedents from feature_ideas

The Wave 4 schema synthesises three earlier design prompts:

- **feature-mixed-evidence-report.md** establishes the
  evidence-kind taxonomy. "What did the tool prove structurally, what
  did it check by generated samples?" Wave 4's `discharge` field is
  the operational answer: `static` ≡ structural, `runtime-sample` ≡
  sampled.
- **feature-counterexample-traces.md** specifies the counter-example
  fields. Wave 4 honours `case_id`, `input`, `expected`/`actual` (as
  `spec_output`/`impl_output`), and `impl_function`. The
  `reproduction_command` field is implicit: `go test
  -run TestSpec_<Func>/<case_id>` reproduces.
- **feature-compliance-audit-trail.md** defines audit-grade. Wave 4
  produces the `evidence.json`-equivalent (the report itself) and the
  `evidence.md` equivalent (`sb audit-report` output). The full
  `verification/audit/` directory layout (commands.log, environment.json,
  hashes.json) is a future extension; Wave 4 is the foundation.

## Determinism

Re-running the verifier against the same spec, same commit, same
impl, same tools should produce the same report bytes — except for
`generated_at` and (transitively) `discharged_since_commit` when
history changes. Determinism enables diffability across runs.

Determinism rules:

- JSON keys are emitted in the order shown above. Map types are
  alphabetised before serialisation.
- `rules[]` is sorted by `name` for the project-level report; per-spec
  reports preserve spec-order.
- `premises[]` within a rule preserves spec order. Field/Verified
  premises come before any synthesised oracle-equality premise.
- `counter_examples[]` is sorted by `case_id`.
- All file paths are repo-relative POSIX paths.

## Open questions deferred to follow-up

- **Per-premise counter-example attribution.** Adding instrumented
  generated tests that report which premise failed (instead of
  rule-level pass/fail) is the obvious v1 step.
- **Cryptographic signing.** v0 leaves `signature: null`. minisign or
  cosign integration is a separate feature.
- **Off-machine sync.** v0 keeps `.sb/history/` local. Compliance
  evidence persistence (S3, attestation server, GitHub Actions
  artifact) is a separate integration.
