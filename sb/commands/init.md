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

### Why this works — compiler enforcement, not LLM policing

Guard types use the target language's **module-private fields** to make the compiler enforce invariants. In Go, struct fields are unexported (lowercase); in TypeScript, class fields are `private`; in Rust, fields are non-pub. This means there is no syntax for constructing a guard type except through its generated constructor — the compiler rejects any other path.

When a function signature requires a guard type (e.g., `TenantAccess`), the caller must have produced it through the constructor chain. If an LLM tries to skip a step, `go build` or `tsc` fails. The error feeds back as backpressure. **The compiler checks the invariants, not the LLM.**

## Step 1: Gather Requirements

Ask the user:

1. **Domain description** — What are the key entities, invariants, and operations? Plain English is fine.

2. **Target language** — What language are the guard types for?
   - Go (default) — uses `cmd/shengen` → `internal/shenguard/guards_gen.go`
   - TypeScript — uses `cmd/shengen-ts` → `internal/shenguard/guards.ts`
   - Other — use `/sb:create-shengen` to build a codegen tool for their language

3. **Project layout** — Where should files go? Defaults:
   - `specs/core.shen` — Shen type specifications
   - `bin/shen-check.sh` — Shen verification wrapper (uses shen-sbcl)
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

Sum type (alternative constructors — multiple blocks with the same conclusion type):
```shen
(datatype human-principal
  User : authenticated-user;
  =============================
  User : authenticated-principal;)

(datatype service-principal
  Cred : service-credential;
  =============================
  Cred : authenticated-principal;)
```
In Go this produces an `AuthenticatedPrincipal` interface with a private marker method, and `HumanPrincipal`/`ServicePrincipal` concrete structs. In TypeScript it produces a union type.

Set membership (`element?`):
```shen
(datatype role-check
  Role : string;
  (element? Role [admin owner member]) : verified;
  ================================================
  Role : valid-role;)
```
Go generates `map[string]bool{...}[val]`; TypeScript generates `new Set([...]).has(val)`.

Use `\* comment *\` to document sections.

## Step 3: Present for Confirmation

**Before writing anything**, show the complete `specs/core.shen` to the user. Explain:
- Each datatype and what invariant it encodes
- Each `verified` premise and what runtime check it becomes in the generated code
- The proof chain: which types require which proofs, and why

**Wait for the user to confirm.** Revise if requested. Do not proceed until confirmed.

## Step 4: Install Tooling

### Shen Runtime (for Gate 4: type checking)

Gate 4 runs Shen's type checker (`tc+`) on the spec. **Any Shen port works** — the spec is pure Shen, independent of what language the guard types target. Use shen-sbcl (Shen on SBCL/Common Lisp):

```bash
# Check if shen-sbcl is available
command -v shen-sbcl || command -v sbcl
```

- **If SBCL is installed**: install shen-sbcl via `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl`
- **If neither is installed**: `brew install sbcl` then install shen-sbcl as above

Do NOT use shen-go. It has known memory allocation crash bugs and hangs during cold bootstrap on macOS.

### shengen (codegen tool)

**shengen is NOT a Shen interpreter.** It's a standalone parser/codegen that reads `.shen` files as text and emits guard types. It does not execute Shen code — that's only Gate 4's job.

Check if shengen exists:
- Go: `bin/shengen` or `cmd/shengen/main.go` — build with `cd cmd/shengen && go build -o ../../bin/shengen .`
- TypeScript: `cmd/shengen-ts/shengen.ts` — runs via `npx tsx`

If neither exists and the project is based on the Shen-Backpressure repo, check `../../cmd/shengen/` (the shared shengen in the repo root).

### shen-check.sh

Create `bin/shen-check.sh` using shen-sbcl:

```bash
#!/bin/bash
set -euo pipefail
SPEC="${1:-specs/core.shen}"
[ -f "$SPEC" ] || { echo "ERROR: $SPEC not found"; exit 1; }
timeout 30 shen-sbcl -q -e "(tc +)" -l "$SPEC" 2>&1 || { echo "RESULT: FAIL"; exit 1; }
echo "RESULT: PASS"
```

Make executable: `chmod +x bin/shen-check.sh`

### shengen-codegen.sh

Create `bin/shengen-codegen.sh` wrapper. Make executable.

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

## Step 6: Set Up TCB Audit (Gate 5)

Create `bin/shenguard-audit.sh` — re-runs shengen, diffs output against committed file, and rejects any unexpected `.go` files in the shenguard package (only `guards_gen.go` and optionally `db_scoped_gen.go` are allowed). This ensures the forgery boundary contains only generated code.

Make executable: `chmod +x bin/shenguard-audit.sh`

## Step 6b (Optional): Scoped DB Wrappers

If the domain has ID types that should scope database queries, generate scoped DB wrappers:
```bash
./bin/shengen-codegen.sh specs/core.shen shenguard internal/shenguard/guards_gen.go --db-wrappers internal/shenguard/db_scoped_gen.go
```

This produces `<ProofType>DB` structs (e.g., `TenantAccessDB`) that capture the verified ID at construction time. `ScopedID()` returns the proven value — use it in all queries to auto-scope without forgetting `WHERE tenant_id = ?`.

## Step 7: Verify

Run all gates:
```bash
./bin/shengen-codegen.sh specs/core.shen ...  # Gate 1: regenerate
go test ./... (or npm test)                    # Gate 2: test
go build ./... (or tsc)                        # Gate 3: build
./bin/shen-check.sh                            # Gate 4: type check
./bin/shenguard-audit.sh                       # Gate 5: TCB audit
```

All gates must pass. Fix and regenerate if there are errors.

## Step 8: Report

Tell the user:
- What specs were created and what invariants they encode
- What guard types were generated and how constructors enforce invariants
- The proof chain and how to use it (wrap at boundary, trust internally)
- The five verification gates they now have:
  1. `shengen` — regenerate guard types (catches spec drift)
  2. Test — run tests against generated types
  3. Build — compile against regenerated types
  4. `shen-check` — verify spec consistency (`tc +`)
  5. `shenguard-audit` — TCB audit (catches tampering and unexpected files)

Then suggest next steps based on their workflow:
- **Ralph loop**: "Run `/sb:loop` to set up an autonomous coding loop with these gates"
- **CI**: "Add these as CI steps: `make shengen && make test && make build && make shen-check && make audit`"
- **Manual dev**: "Run `make all` after changing specs or domain code to verify everything holds"
- **Custom orchestrator**: "Wire the five gates into your build system in order"
