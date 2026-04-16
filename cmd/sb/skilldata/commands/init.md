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

## Tooling Conventions

Prefer the current toolset explicitly while carrying out this command:

- Use `ReadFile` for file reads, `rg` for content search, `Glob` for path discovery, and `Shell` for command execution.
- Use `ApplyPatch` for focused file edits and scripts only for clearly mechanical or generated updates.
- Prefer `sb gates` and `sb derive` over ad hoc verification commands once the project is configured.

### Why this works — compiler enforcement, not LLM policing

Guard types use the target language's **module-private fields** so the **compiler itself** enforces invariants — not the LLM, not a linter, not a runtime assertion. In Go, struct fields are unexported (lowercase); in TypeScript, fields are `private`; in Rust, fields are non-pub. There is no syntax for constructing a guard type except through its generated constructor, which validates the Shen spec's preconditions.

When a function requires a guard type as input (e.g., `ResourceAccess`), the caller must have produced it through the constructor chain. If an LLM writes code that skips a step, `go build` or `tsc` fails in Gate 3. The error feeds back as backpressure. **The compiler checks the invariants, not the LLM.**

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

4. **Optional shen-derive coverage** — Are there pure `(define ...)` functions that should become spec-equivalence drift gates?
   - If yes, capture the function name, impl package, impl function, guard package, and desired generated test path for a future `sb derive` setup

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

Gate 4 runs Shen's type checker (`tc+`) on the spec. **Any Shen port works** — the spec is pure Shen, independent of what language the guard types target. Two recommended backends:

| Backend | Install | Startup | Compute | Best for |
|---------|---------|---------|---------|----------|
| **shen-sbcl** (shen-cl on SBCL) | `brew tap Shen-Language/homebrew-shen && brew install shen-sbcl` | **0.06s** | 1x | Gate loops, CI (startup-dominated) |
| **shen-scheme** (Chez Scheme) | Build from [shen-scheme](https://github.com/Shen-Language/shen-scheme) | 0.44s | **1.6x faster** | Large specs, heavy typechecking |

The `bin/shen-check.sh` script auto-detects whichever is installed (prefers shen-sbcl). Override with `SHEN=/path/to/binary`.

```bash
# Check what's available
command -v shen-sbcl || command -v shen-scheme || command -v shen
```

Do NOT use shen-go — it has known memory allocation crash bugs and hangs during cold bootstrap.

### shengen (codegen tool)

**shengen is NOT a Shen interpreter.** It's a standalone parser/codegen that reads `.shen` files as text and emits guard types. It does not execute Shen code — that's only Gate 4's job.

Check if shengen exists:
- Go: `bin/shengen` or `cmd/shengen/main.go` — build with `cd cmd/shengen && go build -o ../../bin/shengen .`
- TypeScript: `cmd/shengen-ts/shengen.ts` — runs via `npx tsx`

If neither exists and the project is based on the Shen-Backpressure repo, check `../../cmd/shengen/` (the shared shengen in the repo root).

### shen-check.sh

Copy `bin/shen-check.sh` from the Shen-Backpressure repo. It auto-detects the Shen backend (`shen-sbcl` > `shen-scheme` > `shen`) and can be overridden with `$SHEN`. It:
- Accepts a spec path argument (default: `specs/core.shen`)
- Enables type checking (`(tc +)`)
- Loads the spec file
- Exits 0 with `RESULT: PASS` on success
- Exits 1 with `RESULT: FAIL` on type error
- Includes a timeout (30 seconds) to prevent hangs

Make executable: `chmod +x bin/shen-check.sh`

### shengen-codegen.sh

Create `bin/shengen-codegen.sh` wrapper. Make executable.

## Step 5: Write Specs and Generate Guard Types

Write `specs/core.shen` with the confirmed content.

Generate guard types using whichever shengen matches the target language:
```bash
./bin/shengen-codegen.sh specs/core.shen <package-name> <output-path>
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

If shen-check.sh times out or crashes, verify shen-sbcl is installed and working: `shen-sbcl -q -e "(+ 1 1)"`

## Step 6c (Optional): Configure `shen-derive`

If the user wants spec-equivalence checks for pure `(define ...)` functions, add derive config to `sb.toml`:

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
```

Then initialize the committed generated test once:

```bash
sb derive --regen
```

After that, `sb derive` becomes a drift gate, and `sb gates` will automatically append `shen-derive` after the core five gates whenever `[[derive.specs]]` is configured.

## Step 7: Verify

Run all configured gates:

```bash
sb gates
```

If you have not wired `sb gates` yet, the underlying core pipeline is still:

```bash
./bin/shengen-codegen.sh specs/core.shen ...  # Gate 1: regenerate
# Gate 2: test (go test, npm test, cargo test, etc.)
# Gate 3: build (go build, tsc, cargo build, etc.)
./bin/shen-check.sh                            # Gate 4: type check
./bin/shenguard-audit.sh                       # Gate 5: TCB audit
```

All configured gates must pass. Fix and regenerate if there are errors.

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
- If they configured derive coverage, the optional sixth gate:
  6. `sb derive` / `shen-derive` — regenerate spec-derived tests, fail on drift, then run `go test` on referenced impl packages

Then suggest next steps based on their workflow:
- **Ralph loop**: "Run `/sb:loop` or `sb loop` to launch an autonomous coding loop with these gates"
- **CI**: "Add `sb gates` as a CI step, or run the individual scripts from `bin/`"
- **Manual dev**: "Run `sb gates` after changing specs or domain code to verify everything holds"
- **Spec-equivalence**: "If you have pure `(define ...)` functions to pin against Go implementations, add `[[derive.specs]]` and run `/sb:derive` or `sb derive`"
- **Custom orchestrator**: "Wire the core five gate scripts (`bin/`) into your build system in order, and add `sb derive` when derive coverage is configured"
