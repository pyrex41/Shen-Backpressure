---
date: 2026-05-05T17:52:00Z
researcher: claude
git_commit: 6f367c8959a0400b9013f30844927ee087db9495
branch: claude/contract-shen-backpressure-kZwSZ
repository: pyrex41/Shen-Backpressure
topic: "Contraction pass — Phase 0 baseline gate output"
tags: [research, contraction, baseline, gates, demo-readiness]
status: complete
last_updated: 2026-05-05
last_updated_by: claude
---

# Phase 0 Baseline — Gates Across the Three Retained Examples

Captured before the contraction pass begins, so the post-contraction
verification can compare against a known starting point.

Environment notes that affect what passes:

- `shen-sbcl` and `shen-scheme` are NOT installed in this sandbox. Every
  `shen-check` gate fails with a missing-runtime error.
- The example `bin/` directories ship `shen-check.sh` (and in the case of
  `payment/` a pre-built `shengen` binary) but DO NOT include
  `bin/shengen-codegen.sh` or `bin/shenguard-audit.sh`. These are the
  default `gen` and `audit` gate commands per `cmd/sb/config.go:248,172`,
  so those gates fail with "binary not found".
- `npm install` was run inside `examples/shen-web-tools/` to provide the
  `tsx` package needed by the TS shen-derive gate.

## examples/payment/

Command: `cd examples/payment && /home/user/Shen-Backpressure/bin/sb gates`

```
FAIL  shengen        0s            (./bin/shengen-codegen.sh missing)
FAIL  test           153ms         (reference/guards_gen_test.go imports
                                    package shengen/payment that is not
                                    in the payment go module)
PASS  build          266ms
FAIL  shen-check     2ms           (./bin/shen binary missing)
FAIL  tcb-audit      0s            (./bin/shenguard-audit.sh missing)
PASS  shen-derive    180ms

2/6 gates passed
```

Gates that work end-to-end on payment in this sandbox: `build`, `shen-derive`.
`sb gen` invoked directly succeeds and parses 6 datatypes from
`specs/core.shen`, regenerating `internal/shenguard/guards_gen.go`.

## examples/multi-tenant-api/

Command: `cd examples/multi-tenant-api && /home/user/Shen-Backpressure/bin/sb gates`

```
FAIL  shengen        4ms           (no shengen binary; legacy path lookup
                                    looks at cmd/shengen/main.go relative
                                    to cwd)
PASS  test           45.273s
PASS  build          1.02s
FAIL  shen-check     2ms           (shen-sbcl missing)
FAIL  tcb-audit      0s            (./bin/shenguard-audit.sh missing)

2/5 gates passed
```

There is no `sb.toml` in this example, so the engine runs the legacy
five-gate pipeline against the convention defaults. The Go test suite
(JWT proof chain) runs and passes. No shen-derive gate.

## examples/shen-web-tools/

Command: `cd examples/shen-web-tools && /home/user/Shen-Backpressure/bin/sb gates`

```
FAIL  shengen        0s            (./bin/shengen-codegen.sh missing)
PASS  test           981ms
PASS  build          1.897s
FAIL  shen-check     0s            (shen-sbcl missing)
FAIL  tcb-audit      0s            (./bin/shenguard-audit.sh missing)
PASS  shen-derive    1.191s

3/6 gates passed
```

After `npm install` the TS shen-derive gate passes
(`runtime/sum_nonneg.shen-derive.test.ts`).

## What this tells us

All baseline failures are environment-level, not contraction-level:
missing Shen runtime and missing helper shell scripts in
`examples/*/bin/`. None of the contraction work in this pass touches
those failure paths. Phase 7 verification will re-run the same gate set
under the same environment and compare.

## Pre-contraction repo state (snapshot)

- `examples/` top level entries: `payment`, `multi-tenant-api`,
  `shen-web-tools`, `category-showcase`, `order-state-machine`,
  `shenguard-bolt-on`, `.archive`
- `examples/.archive/` already contains the bulk of the prior
  scatter (24 example directories plus
  `FRAMEWORK_EXAMPLES.md`, `INFRA_COMPARISON.md`, `README.md`)
- Skill bundles `sb/` and `cmd/sb/skilldata/` are byte-equal at HEAD
  (`diff -r` reports no differences). `Makefile`'s
  `sync-skilldata` and `check-skilldata` targets exist; `cmd/sb/build.sh`
  copies from `sb/` before `go build`.
- Repo root contains only `README.md`, `Makefile`, `.gitignore`,
  `skm.toml`, plus the standard project subdirectories. No loose
  research markdown.
- `thoughts/shared/research/` already has dated counterparts for
  `EXPLORATION.md`, `heavy_analysis.md`, `heavy_analysis_2.md`, and the
  shen-bend feasibility prompt. No memo for Wave 1, Wave 2, or Wave 3.
