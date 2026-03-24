---
name: init
description: Generate Shen sequent-calculus type specs from natural language, run shengen to produce Go guard types, install shen-go. Presents specs and generated types for confirmation before writing.
---

# Shen Init — Generate Specs and Guard Types

You generate `specs/core.shen` (Shen sequent-calculus type rules) and then run shengen to produce `internal/shenguard/guards_gen.go` (Go guard types that enforce those rules at compile time).

You do NOT implement domain code — Ralph and the harness do that.

## Step 1: Gather Domain Description

Ask the user:
1. **Domain entities** — key types (accounts, items, states, resources, etc.)
2. **Invariants** — what must ALWAYS be true, in plain English
3. **Operations** — transitions or mutations and their preconditions

## Step 2: Draft specs/core.shen

Translate invariants into Shen sequent-calculus datatypes. Use `\* comment *\` syntax.

**Patterns:**

Wrapper (string/number → domain type, no validation):
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

Composite (struct):
```shen
(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)
```

Guarded (invariant proof):
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

## Step 3: Present Specs for Confirmation

**Before writing anything**, show the complete `specs/core.shen` to the user. Explain:
- Each datatype and what invariant it encodes
- Each `verified` premise and what runtime check it becomes
- The proof chain: which types require which proofs

**Wait for confirmation.** Revise if requested.

## Step 4: Install Shen-Go

Check if `bin/shen` exists. If not:

```bash
mkdir -p bin
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp /tmp/shen-go/shen bin/shen
rm -rf /tmp/shen-go
```

Ensure `bin/shen-check.sh` exists and is executable.

## Step 5: Build shengen

Check if `bin/shengen` exists. If not, build from source:

```bash
# If cmd/shengen/main.go exists locally
cd cmd/shengen && go build -o ../../bin/shengen .

# Or if using the shared repo
cd /path/to/Shen-Backpressure/cmd/shengen && go build -o bin/shengen .
```

Ensure `bin/shengen-codegen.sh` exists and is executable.

## Step 6: Write Specs and Generate Guard Types

Write `specs/core.shen` with the confirmed content. Then generate guard types:

```bash
./bin/shengen-codegen.sh specs/core.shen shenguard internal/shenguard/guards_gen.go
```

Show the user the generated `guards_gen.go` — explain how each Shen type maps to a Go type with a guarded constructor.

## Step 7: Verify

Run the Shen type check:
```bash
./bin/shen-check.sh
```

Output should end with `RESULT: PASS`. If there's a type error, fix the spec and regenerate.

## Step 8: Report

Tell the user:
- What types and invariants were encoded in the spec
- What Go guard types were generated
- How constructors enforce the invariants (which ones return errors)
- The proof chain: which types must be constructed before others
- These are now gates in the Ralph loop — every iteration, the harness's changes must satisfy both the Shen proofs and the Go type system
