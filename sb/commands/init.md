---
name: init
description: Generate Shen sequent-calculus type specifications from natural language invariants. Presents specs for confirmation, installs shen-go, and verifies types pass.
---

# Shen Init — Generate Type Specs

You are generating `specs/core.shen` — Shen sequent-calculus type rules that formally encode domain invariants. These specs become one of the three gates in the Ralph loop: every iteration, the harness's code changes must pass `(tc +)`.

You generate and verify specs. You do NOT implement domain code — the Ralph loop and its harness do that.

## Workflow

### Step 1: Gather Domain Description

Ask the user to describe:
1. **Domain entities** — What are the key types? (accounts, items, states, resources, etc.)
2. **Invariants** — What must ALWAYS be true? Plain English is fine.
3. **Operations** — What transitions or mutations exist? What preconditions do they require?

### Step 2: Draft specs/core.shen

Translate invariants into Shen sequent-calculus datatypes.

**Shen datatype syntax:**

```shen
(datatype rule-name
  premise1;
  premise2;
  ============
  conclusion;)
```

Premises and conclusions are type judgments: `Expression : Type`. The `=` line separates premises from conclusion — if all premises hold, the conclusion follows.

**Common patterns:**

Wrapper type (string → domain type):
```shen
(datatype account-id
  X : string;
  ==============
  X : account-id;)
```

Constrained value (number with bounds):
```shen
(datatype quantity
  X : number;
  (>= X 0) : verified;
  ====================
  X : quantity;)
```

Composite type (struct/record):
```shen
(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)
```

Guarded operation (precondition check):
```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

Ownership/authorization:
```shen
(datatype safe-free
  Alloc : allocated;
  Requester : process-id;
  (= (tail Alloc) Requester) : verified;
  =========================================
  [Alloc Requester] : safe-free;)
```

Non-empty constraint:
```shen
(datatype live-state
  S : state;
  Transitions : (list transition);
  (not (= Transitions [])) : verified;
  =====================================
  [S Transitions] : live-state;)
```

Proof-carrying type (operation + its proof):
```shen
(datatype safe-transfer
  Tx : transaction;
  Check : balance-checked;
  =============================
  [Tx Check] : safe-transfer;)
```

Use Shen comment syntax `\* comment *\` to document sections.

### Step 3: Present Specs for Confirmation

**Before writing any files**, present the complete `specs/core.shen` content to the user. Explain:
- Each datatype rule and what invariant it encodes
- How each `verified` premise maps to a runtime check the harness will need to implement
- Any simplifications or assumptions

**Wait for the user to confirm.** If they request changes, revise and present again. Do not proceed until confirmed.

### Step 4: Install Shen-Go Binary

After confirmation, check if `bin/shen` exists. If not, install it:

```bash
mkdir -p bin
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp /tmp/shen-go/shen bin/shen
rm -rf /tmp/shen-go
```

Also ensure `bin/shen-check.sh` exists and is executable. If not, tell the user to run `/sb:setup` first to generate the check wrapper.

Add `bin/shen` to `.gitignore` if not already there.

### Step 5: Write and Verify

Write `specs/core.shen` with the confirmed content. Then run the type check:

```bash
./bin/shen-check.sh
```

Output should end with `RESULT: PASS`. If there's a type error, fix the rules and re-run until they pass.

### Step 6: Report

Tell the user:
- What types and invariants were encoded
- That the Shen type check passes
- These specs are now a gate in the Ralph loop — every iteration, the harness's changes must satisfy these proofs

## Guidelines

- Start simple — basic types first, then invariants, then proof-carrying types
- Every `verified` premise = a runtime check the harness must implement
- Keep rules small and composable — multiple small datatypes over one giant rule
- Shen comments: `\* ... *\`
- List types: `(list element-type)`
- Composite values: `[A B C]`
- Access elements: `(head X)` and `(tail X)`
