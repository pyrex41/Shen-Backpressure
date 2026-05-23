---
date: 2026-05-22T23:00:00-05:00
researcher: claude
git_commit: 81ddf671a482f8b325db11acd04c72ec4182af03
branch: claude/sb-context-polish
repository: Shen-Backpressure
topic: "Additive sidecars introduced by sb context polish (schema v1 untouched)"
tags: [research, discharge-schema, context, gate-results, additive]
status: complete
last_updated: 2026-05-22
last_updated_by: claude
---

# Schema v1 additions — sidecar artifacts introduced by `sb context` polish

**Date**: 2026-05-22
**Researcher**: claude (W1.3 worktree, `claude/sb-context-polish`)
**Git Commit**: 81ddf671a482f8b325db11acd04c72ec4182af03 (base)
**Repository**: Shen-Backpressure

## Question

The W1.3 worktree adds three surface improvements to `sb context` and
`sb audit-report`. One of them — surfacing the most recent gate
PASS/FAIL outcome inside `sb context` — needs persistent storage that
the v1 discharge report schema does not provide. The discharge schema
is locked
(`thoughts/shared/research/2026-05-05-discharge-report-schema.md`);
any field addition would be a schema break.

How is this resolved without breaking schema v1?

## Resolution

`schema_version` of `.sb/discharge_report.json` is **unchanged** at 1.
No fields were added, removed, or repurposed inside that artifact.

The gate-result surface is provided by a **separate sibling file** at
`.sb/gates_last_run.json`, with its own independent schema versioned
from 1. Adding sibling files under `.sb/` is not a discharge-schema
change — the locked schema speaks only to the format of one
particular artifact, not the directory layout of `.sb/`. Consumers
that only know the discharge schema continue to behave correctly:
they read `discharge_report.json` and ignore the sibling.

## What was added

### `.sb/gates_last_run.json` (new sidecar)

Wire format:

```json
{
  "schema_version": 1,
  "generated_at": "2026-05-22T22:48:00Z",
  "gates": [
    {"name": "shengen",    "passed": true,  "exit_code": 0, "duration_ms": 140},
    {"name": "test",       "passed": true,  "exit_code": 0, "duration_ms": 2310},
    {"name": "tcb-audit",  "passed": false, "exit_code": 1, "duration_ms": 60}
  ]
}
```

- **Written by**: `sb gates` (best-effort, non-fatal on write failure)
- **Read by**: `sb context` (defensive, treats absence/parse error as
  "no information")
- **Gitignored**: yes, under the existing `.sb/` glob (line 29-31 of
  `.gitignore`)
- **History rotation**: none today. Only the most recent run is
  persisted; older outcomes live in the discharge-report history if
  they were captured during a `sb derive`/`sb gates` cycle.

### In-memory additions to `GateInfo`

`cmd/sb/context.go`'s `GateInfo` struct grew three optional fields:
`LastResult` (`"pass"` | `"fail"` | `""`), `LastDurationMs` (int64),
`LastExitCode` (int). All carry `omitempty` so the JSON form of `sb
context` only emits them when a sidecar is present. The Markdown
rendering shows `[—]` for gates without recorded outcomes.

### `EvidenceSummary` (read-only projection)

`sb context --evidence` is a read-only view over the locked
`discharge_report.json`. Its `EvidenceSummary` type is a derived
projection — no information is added, only re-aggregated. The
Markdown output is a Markdown table; the JSON output is a small
`EvidenceSummary` struct that consumers can parse with their own
schema if they want.

## Why a sibling and not a schema bump

Three reasons:

1. **Decoupled cadence.** Gate-result persistence has different
   lifetime requirements than the discharge report. We care about the
   most recent gate run only; we care about the full history of
   discharge reports for the "discharged_since_commit" audit walk.
   Two artifacts let each evolve at its own pace.

2. **Optional consumption.** Tools that depend on `discharge_report.json`
   are not forced to learn a new field. A future tool that wants gate
   outcomes opts in explicitly by reading the sibling.

3. **Reversibility.** If we later find a better gate-result format,
   we can replace the sibling without touching the locked discharge
   schema. A bumped-and-reverted discharge schema is much costlier.

## What is NOT changed

- `.sb/discharge_report.json` schema: untouched.
- `.sb/history/<timestamp>-<commit>.json` schema: untouched (entries
  are still copies of discharge reports, same shape).
- Any field in `DischargeReport`, `DischargeRule`, `DischargePremise`,
  `DischargeCounter`, `DischargeSummary`, or `DischargeSignature`:
  untouched.
- The discharge report's `schema_version`: still 1.

## Forward compatibility

A future agent that wants to consume `.sb/gates_last_run.json` should
check `schema_version` and tolerate unknown fields (`encoding/json`
in Go does this automatically; TS consumers should use
`additionalProperties: true` if validating). A schema_version of 0 or
absent should be treated as "stale or malformed; no information",
consistent with how `readGatesLastRun` behaves today.

## References

- `cmd/sb/gates_run.go` — wire format + reader/writer
- `cmd/sb/context.go` — `GateInfo` overlay logic + markdown rendering
- `cmd/sb/evidence.go` — read-only `EvidenceSummary` projection
- `cmd/sb/context_polish_test.go` — tests covering sidecar
  read/missing/malformed paths
- `thoughts/shared/research/2026-05-05-discharge-report-schema.md` —
  the v1 schema this sidecar deliberately does NOT touch
- `thoughts/shared/research/2026-05-22-hn-feedback-next-steps.md`
  §R5 — the HN-derived motivation for surfacing latest gate
  PASS/FAIL in `sb context`
