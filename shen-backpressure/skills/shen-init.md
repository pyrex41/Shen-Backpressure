---
name: shen-init
description: Generate Shen sequent-calculus type specifications from a natural language description of domain invariants
---

# Shen Init — Generate Type Specs

You are generating `specs/core.shen` — a Shen sequent-calculus type specification file that formally encodes domain invariants described in plain English.

## Activation

Trigger when the user says: "generate Shen types", "create type specs", "Shen spec from description", "write sequent rules", or asks to formalize invariants in Shen.

## Workflow

### Step 1: Gather Domain Description

Ask the user to describe:
1. **Domain entities** — What are the key types? (accounts, items, states, resources, etc.)
2. **Invariants** — What must ALWAYS be true? Describe in plain English.
3. **Operations** — What transitions or mutations exist? What preconditions do they require?

### Step 2: Generate specs/core.shen

Create `specs/core.shen` translating each invariant into Shen sequent-calculus datatypes.

**Shen datatype syntax:**

```shen
(datatype rule-name
  premise1;
  premise2;
  ============
  conclusion;)
```

Premises and conclusions are type judgments of the form `Expression : Type`. The line of `=` separates premises from conclusion — if all premises hold, the conclusion follows.

**Common patterns:**

1. **Wrapper type** (string → domain type):
```shen
(datatype account-id
  X : string;
  ==============
  X : account-id;)
```

2. **Constrained value** (number with bounds):
```shen
(datatype quantity
  X : number;
  (>= X 0) : verified;
  ====================
  X : quantity;)
```

3. **Composite type** (struct/record):
```shen
(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)
```

4. **Guarded operation** (precondition check):
```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

5. **Ownership/authorization**:
```shen
(datatype safe-free
  Alloc : allocated;
  Requester : process-id;
  (= (tail Alloc) Requester) : verified;
  =========================================
  [Alloc Requester] : safe-free;)
```

6. **Non-empty constraint**:
```shen
(datatype live-state
  S : state;
  Transitions : (list transition);
  (not (= Transitions [])) : verified;
  =====================================
  [S Transitions] : live-state;)
```

7. **Proof-carrying type** (operation + its proof):
```shen
(datatype safe-transfer
  Tx : transaction;
  Check : balance-checked;
  =============================
  [Tx Check] : safe-transfer;)
```

### Step 3: Add Comments

Use Shen comment syntax `\* comment *\` to document each section:

```shen
\* specs/core.shen - Formal type specifications *\
\* Domain: [user's domain] *\

\* --- Basic types --- *\
...

\* --- Key invariant: [description] --- *\
...
```

### Step 4: Verify

If `bin/shen-check.sh` and `bin/shen` exist, run the type check:

```bash
./bin/shen-check.sh
```

The output should end with `RESULT: PASS`. If there's a type error, fix the rules until they pass.

### Step 5: Report

Tell the user what types and invariants were encoded, and explain how each Shen rule maps to their domain requirements.

## Guidelines

- Start simple — basic types first, then invariants, then proof-carrying types
- Every `verified` premise corresponds to a runtime check the Go code must enforce
- Keep rules small and composable — prefer multiple small datatypes over one giant rule
- Shen comments use `\* ... *\` syntax (backslash-star)
- List types use `(list element-type)` syntax
- Composite values are encoded as Shen lists: `[A B C]`
- Access list elements with `(head X)` and `(tail X)`
