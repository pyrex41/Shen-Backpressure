---
name: derive
description: Configure and run the optional shen-derive spec-equivalence gate. Use when the project has Shen `(define ...)` functions that should act as an oracle for handwritten Go implementations, or when `sb.toml` already contains `[[derive.specs]]` entries and the user wants to run, regenerate, or debug the derive gate.
---

# Shen Derive — Spec-Equivalence Gate

You configure or run the optional `shen-derive` verification gate for Shen `(define ...)` functions.

This gate is complementary to shengen:

- **shengen** turns Shen datatype rules into opaque guard types
- **shen-derive** turns Shen `(define ...)` functions into committed table-driven Go tests

Use it when the user has a pure function whose obvious-correct behavior can be written in Shen and whose handwritten implementation should stay aligned with that spec.

## Tooling Conventions

Prefer the current toolset explicitly:

- Use `ReadFile` for file reads, `rg` for content search, `Glob` for locating files, and `Shell` for commands like `sb derive`, `go test`, and `go run`.
- Use `ApplyPatch` for precise `sb.toml` or docs edits.
- Use `multi_tool_use.parallel` when independent reads or searches can happen together.

## Step 1: Detect Current State

Check:

1. Does `sb.toml` exist?
2. Does it already contain a `[derive]` section or any `[[derive.specs]]` entries?
3. Does the repo have a `shen-derive` module available (commonly `../../shen-derive`)?
4. Which `.shen` file contains the `(define ...)` function to verify?

If `sb.toml` already contains derive entries, show the user what is configured before changing anything.

## Step 2: Gather Missing Inputs

If the derive gate is not fully configured, ask the user for:

1. **Spec file path** — the `.shen` file containing the `(define ...)` block
2. **Shen function name** — the exact `define` name, e.g. `processable`
3. **Implementation package** — Go import path of the handwritten implementation
4. **Implementation function** — exported Go function name, e.g. `Processable`
5. **Guard package** — Go import path for the generated shengen guards
6. **Output file** — where the committed generated test should live
7. **Optional seed** — only if the user wants seeded random draws in addition to deterministic boundary cases

If the user already has an example to follow, point them at `examples/payment/sb.toml`.

## Step 3: Configure `sb.toml`

Add or update the derive configuration:

```toml
[derive]
dir = "../../shen-derive"

[[derive.specs]]
path      = "specs/core.shen"
func      = "processable"
impl_pkg  = "your-module/internal/derived"
impl_func = "Processable"
guard_pkg = "your-module/internal/shenguard"
out_file  = "internal/derived/processable_spec_test.go"
# seed    = 42
```

Rules:

- Keep one `[[derive.specs]]` entry per `(define ...)` function being verified
- Use committed output files inside the project, not temp paths
- Only add `seed` when the user wants seeded random draws; otherwise keep the default deterministic behavior

## Step 4: Explain What `sb derive` Actually Does

Tell the user how the gate works:

1. `sb derive` loads `[[derive.specs]]` from `sb.toml`
2. For each entry, it runs `shen-derive verify` via `go run` in the configured derive module
3. It writes regenerated output to a temp file and diffs it against the committed `out_file`
4. Drift is treated as failure
5. After all drift checks pass, it runs `go test` on each referenced implementation package

Useful flags:

- `sb derive --regen` — rewrite the committed generated tests in place
- `sb derive --skip-test` — skip the `go test` phase after drift checking

## Step 5: Run the Gate

Once configured, run:

```bash
sb derive
```

If the committed generated file does not exist yet, or the user intentionally changed the spec/sampling strategy, regenerate:

```bash
sb derive --regen
```

Then re-run `sb derive` to confirm the gate is clean.

## Step 6: Connect It to the Full Pipeline

Explain the integration clearly:

- `sb derive` is a standalone gate the user can run directly
- When `sb.toml` contains any `[[derive.specs]]` entries, `sb gates` automatically appends `shen-derive` after the core five gates
- Ralph loops that call `sb gates` pick this up automatically once the project is configured

## Step 7: Report Back

Tell the user:

- What derive entries were added or updated in `sb.toml`
- Which `(define ...)` functions are now covered
- Where the committed generated test files live
- Whether `sb derive` passed, drifted, or required `--regen`
- That `sb gates` will now include the optional `shen-derive` gate automatically
