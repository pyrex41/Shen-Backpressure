---
date: 2026-05-05T22:19:00Z
researcher: claude
git_commit: 8b1f37f
branch: claude/discharge-reports-audit-z9FNM
repository: pyrex41/Shen-Backpressure
topic: "Wave 4 — discharge reports baseline"
tags: [research, sb-engine, wave-4, discharge-reports, baseline]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Wave 4 baseline

Captured before any Wave 4 code lands, against `8b1f37f` (post-PR-#11
merge). Re-running `phase-0` after the wave should reproduce the
"before" column of the comparison memo.

## `git status`

```
On branch claude/discharge-reports-audit-z9FNM
nothing to commit, working tree clean
```

## `sb gates` in `examples/payment/`

```
PASS [shengen] 423ms
PASS [test] 3.072s
PASS [build] 321ms
FAIL [shen-check] 3ms
PASS [tcb-audit] 16ms
PASS [shen-derive] 371ms

  PASS  shengen        423ms
  PASS  test           3.072s
  PASS  build          321ms
  FAIL  shen-check     3ms
  PASS  tcb-audit      16ms
  PASS  shen-derive    371ms

5/6 gates passed

--- FAIL [shen-check] ---
ERROR: no Shen runtime found. Install shen-sbcl or shen-scheme, or set $SHEN.
  brew tap Shen-Language/homebrew-shen && brew install shen-sbcl
```

`5/6` is the documented post-contraction baseline (Gate 4 fails because
no Shen runtime is installed in this CI image). This is the same shape
the contraction-results memo records.

## Filesystem before Wave 4

`examples/payment/` does not have a `.sb/` directory. After Wave 4 it
will, holding the current `discharge_report.json` plus a `history/`
subdir (both gitignored).

## shen-derive harness data flow today

```
specs/core.shen
       │
       ▼  specfile.ParseFile → []Define + []Datatype
       │
       ▼  specfile.BuildTypeTable → TypeTable
       │
       ▼  verify.BuildHarness → Harness{Cases}
       │
       ▼  Harness.Emit → Go test source
       │
       ▼  sb derive → go test on impl pkg
```

Wave 4 hooks:

- After `BuildHarness` finishes, extract per-rule premise structure from
  `Spec` + `TypeTable` and write a partial discharge report next to the
  test file.
- `sb derive` aggregates per-spec reports into one project-level
  `.sb/discharge_report.json`, runs `go test`, parses failures, and
  patches in counter-examples.

## Files Wave 4 will touch

- `shen-derive/main.go` — new `--report-out` flag on `verify`.
- `shen-derive/report/` (new package) — schema types, classifier,
  emitter.
- `shen-derive/specfile/parse.go` — `:doc` annotation support.
- `cmd/sb/derive.go` — invoke shen-derive with `--report-out`,
  aggregate, run `go test`, parse failures, write final report and a
  history copy.
- `cmd/sb/context.go` — render terse Markdown summary of
  `.sb/discharge_report.json`.
- `cmd/sb/audit_report.go` (new) — `sb audit-report` command.
- `cmd/sb/main.go` — register `audit-report`.
- `examples/payment/README.md` — third walkthrough step + `What's here`
  update.
- `README.md` — short discharge-reports section.
- `.gitignore` — ignore `.sb/`.
