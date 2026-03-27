---
name: init
description: Add Shen backpressure to any project. Generates formal type specs from domain description, builds shengen, produces guard types in your target language, sets up verification gates. Works with any workflow — Ralph loops, CI pipelines, manual dev, or custom orchestrators.
---

# Shen Init — Add Formal Backpressure to Your Project

You add Shen sequent-calculus backpressure to the user's project. This means:
1. Formal type specs (`specs/core.shen`) that prove domain invariants
2. Generated guard types (Go, TypeScript, etc.) with opaque constructors that enforce those invariants at compile time
3. Verification gates that can be run manually, in CI, in a Ralph loop, or however the user wants

You do NOT assume any particular workflow or orchestrator. You set up the foundation — the user decides how to run it.

### How enforcement works

Guard types use the target language's **module-private fields** so the **compiler itself** enforces invariants — not the LLM, not a linter, not a runtime assertion. In Go, struct fields are unexported (lowercase); in TypeScript, fields are `private`; in Rust, fields are non-pub. There is no syntax for constructing a guard type except through its generated constructor, which validates the Shen spec's preconditions.

When a function requires a guard type as input (e.g., `ResourceAccess`), the caller must have produced it through the constructor chain. If an LLM writes code that skips a step, `go build` or `tsc` fails in Gate 3. The error feeds back as backpressure. The compiler checks invariants; the LLM just writes code that has to satisfy the compiler.

## Step 1: Gather Requirements

Ask the user:

1. **Domain description** — What are the key entities, invariants, and operations? Plain English is fine.

2. **Target language** — What language are the guard types for?
   - Go — uses `cmd/shengen` → generates `.go` with unexported struct fields
   - TypeScript — uses `cmd/shengen-ts` → generates `.ts` with private class fields
   - Other — use `/sb:create-shengen` to build a codegen tool for their language

3. **Project layout** — Where should files go? Defaults:
   - `specs/core.shen` — Shen type specifications
   - `bin/shen-check.sh` — Shen verification wrapper (uses shen-sbcl)
   - `bin/shengen` or `bin/shengen-codegen.sh` — codegen tooling
   - Generated guard types go wherever is idiomatic for the target language

## Step 2: Draft specs/core.shen

Translate the user's domain into Shen sequent-calculus datatypes.

**Patterns** (each maps to a specific guard type output):

Wrapper (domain-specific string/number, no validation):
```shen
(datatype account-id
  X : string;
  ==============
  X : account-id;)
```

Constrained (validated value):
```shen
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)
```

Composite (structured type):
```shen
(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)
```

Guarded (invariant proof — the key pattern):
```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

Proof chain (requires prior proof):
```shen
(datatype safe-transfer
  Tx : transaction;
  Check : balance-checked;
  =============================
  [Tx Check] : safe-transfer;)
```

Use `\* comment *\` to document sections.

## Step 3: Present for Confirmation

**Before writing anything**, show the complete `specs/core.shen` to the user. Explain:
- Each datatype and what invariant it encodes
- Each `verified` premise and what runtime check it becomes in the generated code
- The proof chain: which types require which proofs, and why

**Wait for the user to confirm.** Revise if requested. Do not proceed until confirmed.

## Step 4: Install Tooling

### Shen Runtime (for Gate 4: type checking)

Gate 4 runs Shen's type checker (`tc+`) on the spec. **Any Shen port works** — the spec is pure Shen, independent of what language the guard types target. Use **shen-sbcl** (Shen on SBCL/Common Lisp):

```bash
# Check if shen-sbcl is available
command -v shen-sbcl || command -v sbcl
```

- **If SBCL is installed**: install shen-sbcl via `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl`
- **If neither**: `brew install sbcl` then install shen-sbcl as above

Do NOT use shen-go — it has known memory allocation crash bugs and hangs during cold bootstrap.

### shengen (codegen tool)

**shengen is NOT a Shen interpreter.** It's a standalone parser/codegen that reads `.shen` files as text and emits guard types. Check if it exists:

- Go: `bin/shengen` or `cmd/shengen/main.go` — build with `cd cmd/shengen && go build -o ../../bin/shengen .`
- TypeScript: `cmd/shengen-ts/shengen.ts` — runs via `npx tsx`

If neither exists and the project is based on the Shen-Backpressure repo, check `../../cmd/shengen/` (the shared shengen in the repo root).

### shen-check.sh

Create `bin/shen-check.sh` using shen-sbcl. The script must:
- Accept a spec path argument (default: `specs/core.shen`)
- Enable type checking (`(tc +)`)
- Load the spec file
- Exit 0 with `RESULT: PASS` on success
- Exit 1 with `RESULT: FAIL` on type error
- Include a timeout (30 seconds) to prevent hangs

**For shen-sbcl:**
```bash
#!/bin/bash
set -euo pipefail
SPEC="${1:-specs/core.shen}"
timeout 30 shen-sbcl -q -e "(tc +)" -l "$SPEC" 2>&1 || { echo "RESULT: FAIL"; exit 1; }
echo "RESULT: PASS"
```

### shengen-codegen.sh

Create `bin/shengen-codegen.sh` wrapper. Make executable.

## Step 5: Write Specs and Generate Guard Types

Write `specs/core.shen` with the confirmed content.

Generate guard types using whichever shengen matches the target language:
```bash
./bin/shengen-codegen.sh specs/core.shen <package-name> <output-path>
```

Show the user the generated types — explain how each Shen type maps to a guard type with a validated constructor.

## Step 6: Verify

Run the Shen type check:
```bash
./bin/shen-check.sh
```

Output should end with `RESULT: PASS`. Fix and regenerate if there's a type error.

If shen-check.sh times out or crashes, verify shen-sbcl is installed and working: `shen-sbcl -q -e "(+ 1 1)"`

## Step 7: Report

Tell the user:
- What specs were created and what invariants they encode
- What guard types were generated and how constructors enforce invariants
- The proof chain and how to use it (wrap at boundary, trust internally)
- The three verification gates they now have:
  1. `shengen` — regenerate guard types (catches spec drift)
  2. `shen-check` — verify spec consistency (`tc +`)
  3. Build/test — compile and test against generated types

Then suggest next steps based on their workflow:
- **Ralph loop**: "Run `/sb:loop` to set up an autonomous coding loop with these gates"
- **CI**: "Add these as CI steps: `make shengen && make test && make build && make shen-check`"
- **Manual dev**: "Run `make all` after changing specs or domain code to verify everything holds"
- **Custom orchestrator**: "Wire the three gates into your build system in order: shengen first, then test+build, then shen-check"
