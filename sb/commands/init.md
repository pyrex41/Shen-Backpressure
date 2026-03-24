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

## Step 1: Gather Requirements

Ask the user:

1. **Domain description** — What are the key entities, invariants, and operations? Plain English is fine.

2. **Target language** — What language are the guard types for?
   - Go (default) — uses `cmd/shengen` → `internal/shenguard/guards_gen.go`
   - TypeScript — uses `cmd/shengen-ts` → `internal/shenguard/guards.ts`
   - Other — use `/sb:create-shengen` to build a codegen tool for their language

3. **Project layout** — Where should files go? Defaults:
   - `specs/core.shen` — Shen type specifications
   - `bin/shen` — Shen-Go binary
   - `bin/shen-check.sh` — Shen verification wrapper
   - `bin/shengen` or `bin/shengen-codegen.sh` — codegen tooling
   - `internal/shenguard/` — generated guard types

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

**Shen-Go** — check if `bin/shen` exists. If not:
```bash
mkdir -p bin
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp /tmp/shen-go/shen bin/shen
rm -rf /tmp/shen-go
```

**shen-check.sh** — create `bin/shen-check.sh` if missing (wraps shen-go's EOF behavior). Make executable.

**shengen** — build or install based on target language:
- Go: `cd cmd/shengen && go build -o ../../bin/shengen .`
- TypeScript: ensure `cmd/shengen-ts/shengen.ts` is available and `npx tsx` works

**shengen-codegen.sh** — create `bin/shengen-codegen.sh` wrapper if missing. Make executable.

## Step 5: Write Specs and Generate Guard Types

Write `specs/core.shen` with the confirmed content.

Generate guard types:
```bash
# Go
./bin/shengen-codegen.sh specs/core.shen shenguard internal/shenguard/guards_gen.go

# TypeScript
npx tsx cmd/shengen-ts/shengen.ts specs/core.shen --out internal/shenguard/guards.ts
```

Show the user the generated types — explain how each Shen type maps to a guard type with a validated constructor.

## Step 6: Verify

Run the Shen type check:
```bash
./bin/shen-check.sh
```

Output should end with `RESULT: PASS`. Fix and regenerate if there's a type error.

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
