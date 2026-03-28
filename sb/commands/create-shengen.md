---
name: create_shengen
description: Build a shengen codegen tool that compiles Shen sequent-calculus specs into guard types for any target language. Provides the complete algorithm, grammar, symbol table schema, accessor resolution rules, and per-language enforcement strategies. Use when bootstrapping shengen for a new target language (Go, Rust, Python, TypeScript, Java, etc.) or when extending an existing shengen to support new Shen patterns.
---

# Create Shengen — Build a Spec-to-Guards Compiler for Any Language

You are building `shengen`: a codegen tool that reads Shen sequent-calculus type specifications and emits guard types in a target language. The generated types enforce domain invariants through opaque constructors — the ONLY way to create a value of a guard type is through its constructor, which validates the Shen spec's preconditions at runtime.

**shengen is NOT a Shen interpreter.** It does not execute Shen code. It parses `(datatype ...)` blocks, extracts the proof rules, and translates them into target-language types whose constructors mirror those rules. Shen itself runs separately as a deductive proof checker on the spec.

---

## 1. The Contract

The generated code must guarantee three properties:

### 1a. Construction exclusivity

Values of guard types can ONLY be created through generated constructor functions. No other path. This means:

| Language | Enforcement mechanism |
|---|---|
| Go | Struct with unexported fields (`type Amount struct{ v float64 }`) — only the `shenguard` package can access `v` |
| Rust | Struct with private fields in a module — `pub` constructor, fields are `pub(crate)` or private |
| TypeScript | Class with `private` fields + static factory method, or branded types with a unique symbol |
| Python | `__init__` that runs validation; `__slots__` to prevent attribute injection; or `@dataclass(frozen=True)` with `__post_init__` validation |
| Java | Final class with private fields, public static factory method, no public constructor |
| C# | Record or class with `internal`/`private` fields, public static `Create` method returning `Result<T>` |
| Swift | Struct with `private(set)` stored properties, `public init` that throws on validation failure |
| Kotlin | Data class with `private constructor`, companion object factory that returns `Result<T>` |

The enforcement mechanism varies, but the invariant is universal: **you cannot hold a value of a guard type without having passed through the constructor's validation.**

### 1b. Validation faithfulness

Every `verified` premise in the Shen spec becomes a runtime check in the constructor. If the check fails, construction fails (returns error, throws exception, returns Result/Option — whatever is idiomatic). The checks must be semantically equivalent to the Shen expressions.

### 1c. Type propagation

Composite and guarded types accept other guard types as arguments, not raw primitives. This means the target language's type system prevents passing unchecked values deep into the domain. Validation happens once at the boundary; the types carry proof internally.

---

## 2. Input Grammar

shengen parses a subset of Shen's `(datatype ...)` form. Here is the complete grammar it must handle:

### 2a. File structure

A `.shen` file contains zero or more `(datatype ...)` blocks interspersed with Shen comments (`\* ... *\`). Everything outside a `(datatype ...)` block is ignored.

### 2b. Datatype block

```
(datatype BLOCK_NAME
  RULE_1
  RULE_2
  ...
  RULE_N)
```

`BLOCK_NAME` is a hyphenated lowercase identifier (e.g., `balance-invariant`, `user-id`, `copy-delivery`). A block contains one or more rules separated by blank lines.

### 2c. Rule structure

A rule has premises above an inference line, and a conclusion below it:

```
PREMISE_1;
PREMISE_2;
...
PREMISE_N;
INFERENCE_LINE
CONCLUSION;
```

The **inference line** is a row of 3+ identical characters:
- `====...====` (equals signs) — **LR rule**: defines both construction (R) and deconstruction (L). This is the common case and is almost always what specs use.
- `____...____` (underscores) — **R rule only**: defines construction but not deconstruction. Rarely used in practice; treat the same as LR for codegen purposes.

### 2d. Premise types

A premise is one of:

**Type judgment**: `VARIABLE : TYPE;`
- `VARIABLE` is a capitalized identifier (e.g., `X`, `Amount`, `Profile`)
- `TYPE` is a Shen type expression: either a primitive (`string`, `number`, `symbol`, `boolean`), a user-defined type (`amount`, `known-profile`), or a parameterised type (`(list A)`)
- Examples: `X : string;`, `Amount : amount;`, `Tx : transaction;`

**Verified premise**: `SHEN_EXPRESSION : verified;`
- `SHEN_EXPRESSION` is an s-expression that must evaluate to true at runtime
- Examples: `(>= X 0) : verified;`, `(= (head A) (head B)) : verified;`

**Side condition**: `if SHEN_EXPRESSION`
- Equivalent to a verified premise but uses `if` syntax instead of `: verified`
- Example: `if (element? Op [+ - * /])`

### 2e. Conclusion types

A conclusion is one of:

**Composite conclusion**: `[FIELD_1 FIELD_2 ... FIELD_N] : TYPE_NAME;`
- Defines a structured type with positional fields
- Field names correspond to variables from the premises
- Example: `[Amount From To] : transaction;` — a transaction has three fields

**Wrapped conclusion**: `VARIABLE : TYPE_NAME;`
- Defines a type that wraps a single value
- Example: `X : email-addr;` — email-addr wraps whatever type X has

**Assumption conclusion (skip)**: anything containing `>>`
- These are left-rule assumptions used in Shen's proof engine
- shengen should **skip** these entirely — they are handled by the LR (`====`) rule

### 2f. Block name vs conclusion type name

**These can differ.** The block name is `(datatype BLOCK_NAME ...)`. The conclusion type name is the type after `:` in the conclusion. Example:

```shen
(datatype balance-invariant      \* block name *\
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)  \* conclusion type name *\
```

The generated type should be named after the **conclusion type** (`BalanceChecked`), not the block name (`BalanceInvariant`), because other types reference the conclusion type name.

**Collision rule**: When multiple blocks produce the same conclusion type (sum types / alternative constructors), use the **block name** for each concrete type to avoid name collisions. Detect this with a pre-pass that counts producers per conclusion type.

### 2g. Sum types (multiple blocks → same conclusion type)

When two or more `(datatype ...)` blocks have different block names but the same conclusion type name, they form a **sum type**. Each block becomes a concrete variant; the conclusion type becomes the abstract sum type.

```shen
(datatype human-principal          \* block name → concrete type *\
  User : authenticated-user;
  =============================
  User : authenticated-principal;) \* conclusion type → sum type *\

(datatype service-principal        \* block name → concrete type *\
  Cred : service-credential;
  =============================
  Cred : authenticated-principal;) \* same conclusion type *\
```

Detection: in the symbol table pre-pass, if `ConcCount[type_name] > 1`, that type is a sum type. Each variant is named after its **block name** (not the conclusion type) to avoid collisions.

### 2h. `(define ...)` blocks (Go shengen only)

Shen `define` blocks define helper functions using pattern matching with `->` as clause separator and optional `where` guards:

```shen
(define has-required-role
  [] _ -> false
  [[Role _] | Rest] Target -> true where (= Role Target)
  [_ | Rest] Target -> (has-required-role Rest Target))
```

shengen can parse these and emit iterative Go functions. This is optional — not all target languages need this.

---

## 3. Classification Algorithm

After parsing, classify each rule into one of five categories. The category determines what code to generate.

```
classify(rule):
  C = rule.conclusion
  P = rule.premises (type judgments only)
  V = rule.verified_premises

  if C.is_wrapped AND |V| == 0 AND |P| == 1 AND P[0].type is primitive:
    → WRAPPER

  if C.is_wrapped AND |V| > 0 AND |P| >= 1 AND P[0].type is primitive:
    → CONSTRAINED

  if C.is_wrapped AND |P| == 1 AND P[0].type is NOT primitive:
    → ALIAS

  if C.is_composite AND |V| > 0:
    → GUARDED

  if C.is_composite AND |V| == 0:
    → COMPOSITE

  else:
    → COMPOSITE (default)
```

Where "primitive" means `string`, `number`, `symbol`, or `boolean`.

### Category descriptions

| Category | Premises | Verified | Conclusion | Generated output |
|---|---|---|---|---|
| **WRAPPER** | 1 primitive | none | wrapped | Opaque type wrapping a primitive. Constructor takes the primitive, returns the wrapper. No validation. |
| **CONSTRAINED** | 1 primitive | 1+ checks | wrapped | Opaque type wrapping a primitive. Constructor takes the primitive, validates, returns wrapper or error. |
| **COMPOSITE** | N fields | none | composite | Struct/record with N typed fields. Constructor takes guard types, returns the composite. No validation. |
| **GUARDED** | N fields | 1+ checks | composite | Struct/record with N typed fields. Constructor takes guard types, validates preconditions, returns composite or error. |
| **ALIAS** | 1 custom type | none | wrapped | Type alias to another guard type. No constructor needed. |
| **SUMTYPE** | (synthetic) | — | — | Interface/union over variants that share a conclusion type. Generated in a separate pass. |

### Sum type detection

Sum types are NOT detected by the classifier. They are detected in the **symbol table pre-pass** (§4b) by counting how many blocks produce each conclusion type. When `ConcCount[type] > 1`:
- Each variant block is classified normally (typically COMPOSITE) and named after the **block name**
- A synthetic SUMTYPE entry is added for the conclusion type name
- The sum type entry lists which variants implement it

---

## 4. Symbol Table

The symbol table is the core data structure that enables verified premise translation. Build it in a pre-pass before generating any code.

### 4a. Schema

```
SymbolTable = map<shen_type_name, TypeInfo>

TypeInfo:
  shen_name:     string          # e.g. "transaction"
  target_name:   string          # e.g. "Transaction" (PascalCase for Go, etc.)
  category:      enum            # WRAPPER | CONSTRAINED | COMPOSITE | GUARDED | ALIAS
  fields:        list<FieldInfo> # only for COMPOSITE and GUARDED
  wrapped_prim:  string | null   # only for WRAPPER and CONSTRAINED (e.g. "string", "number")
  wrapped_type:  string | null   # only for ALIAS (e.g. "unknown-profile")

FieldInfo:
  index:     int                 # positional index (0-based)
  shen_name: string              # variable name from conclusion (e.g. "Amount")
  shen_type: string              # type from premise (e.g. "amount")
```

### 4b. Construction algorithm

```
function build_symbol_table(datatypes):
  # Pass 1: count conclusion type producers (for collision detection)
  conc_count = {}
  for dt in datatypes:
    for rule in dt.rules:
      conc_count[rule.conclusion.type_name] += 1

  # Pass 2: build entries
  table = {}
  for dt in datatypes:
    for rule in dt.rules:
      type_name = rule.conclusion.type_name

      # Collision resolution: if multiple blocks produce this conclusion type,
      # use block name to avoid generating duplicate type names
      if dt.name != type_name AND conc_count[type_name] > 1:
        type_name = dt.name

      info = new TypeInfo(shen_name=type_name, ...)
      info.category = classify(rule)

      # Build field list for composites/guarded from conclusion field order
      if rule.conclusion.is_composite:
        prem_map = {p.var_name: p.type_name for p in rule.premises}
        for i, field_name in enumerate(rule.conclusion.fields):
          info.fields.append(FieldInfo(
            index=i,
            shen_name=field_name,
            shen_type=prem_map[field_name]
          ))

      table[type_name] = info

  return table
```

### 4c. What the symbol table enables

Without the symbol table, the expression `(>= Bal (head Tx))` is opaque — you don't know what `(head Tx)` means. With the symbol table:

1. Look up `Tx` in the variable map → its type is `transaction`
2. Look up `transaction` in the symbol table → fields are `[Amount:amount, From:account-id, To:account-id]`
3. `(head Tx)` = field 0 = `Amount` of type `amount`
4. `amount` is a CONSTRAINED wrapper around `number` → need `.value()` to unwrap
5. Result: `bal >= tx.amount.value()`

---

## 5. S-Expression Parser

Verified premises are s-expressions. You need a parser that produces an AST.

### 5a. Grammar

```
sexpr   ::= atom | '(' sexpr* ')'
atom    ::= number | string | symbol
number  ::= ['-'] digit+ ['.' digit+]
symbol  ::= identifier characters (letters, digits, hyphens, underscores, ?, !, .)
```

### 5b. AST

```
SExpr:
  atom:     string | null     # non-null for atoms
  children: list<SExpr> | null  # non-null for lists

  is_atom():  atom != null
  is_call():  children != null AND len(children) > 0
  op():       children[0].atom if is_call() else null
```

### 5c. Examples

| Input | AST |
|---|---|
| `(>= X 10)` | `Call(>=, Atom(X), Atom(10))` |
| `(= 0 (shen.mod X 10))` | `Call(=, Atom(0), Call(shen.mod, Atom(X), Atom(10)))` |
| `(>= Bal (head Tx))` | `Call(>=, Atom(Bal), Call(head, Atom(Tx)))` |
| `(= (tail (tail (head P))) (tail C))` | `Call(=, Call(tail, Call(tail, Call(head, Atom(P)))), Call(tail, Atom(C)))` |

---

## 6. Accessor Chain Resolution

This is the algorithm that resolves `(head X)` and `(tail X)` to concrete field accesses using the symbol table.

### 6a. Core concept

Shen represents composite types as nested cons cells (linked lists). The conclusion `[A B C] : thing` means `(cons A (cons B (cons C nil)))`. Therefore:

| Expression | Meaning | Result |
|---|---|---|
| `(head X)` | first element of X's list | field at index 0 |
| `(tail X)` | everything after the first element | fields at index 1+ |
| `(head (tail X))` | second element | field at index 1 |
| `(tail (tail X))` | everything after second element | fields at index 2+ |
| `(head (tail (tail X)))` | third element | field at index 2 |

When `(tail X)` leaves exactly one field remaining, resolve directly to that field (not a sub-list).

### 6b. Resolution algorithm

```
function resolve(expr, var_map, symbol_table) -> ResolvedExpr:

  # Base case: atom
  if expr.is_atom():
    if expr.atom is a numeric literal:
      return ResolvedExpr(code=expr.atom, type="number")

    if expr.atom is in var_map:
      shen_type = var_map[expr.atom]
      return ResolvedExpr(
        code = to_target_name(expr.atom),  # e.g. camelCase
        type = shen_type
      )

    return ResolvedExpr(code=expr.atom, type="unknown")

  # Recursive case: function call
  op = expr.op()

  if op == "head" or op == "tail":
    return resolve_head_tail(expr, var_map, symbol_table, is_head=(op == "head"))

  if op == "shen.mod":
    lhs = resolve(expr.children[1], var_map, symbol_table)
    rhs = resolve(expr.children[2], var_map, symbol_table)
    return ResolvedExpr(code=modulo(unwrap_numeric(lhs), rhs.code), type="number")

  if op == "length":
    inner = resolve(expr.children[1], var_map, symbol_table)
    return ResolvedExpr(code=length_of(unwrap_string(inner)), type="number")

  if op == "not":
    inner = resolve(expr.children[1], var_map, symbol_table)
    return ResolvedExpr(code=negate(inner.code), type="boolean")

  return UNRESOLVED


function resolve_head_tail(expr, var_map, symbol_table, is_head) -> ResolvedExpr:
  inner = resolve(expr.children[1], var_map, symbol_table)

  # If inner resolved to a multi-field intermediate (from a prior tail), use those fields
  if inner.is_multi_field:
    return access_fields(inner.base_code, inner.remaining_fields, is_head)

  # Otherwise look up the type's field layout in the symbol table
  type_info = symbol_table.lookup(inner.type)
  if type_info is null or type_info.fields is empty:
    return UNRESOLVED

  return access_fields(inner.code, type_info.fields, is_head)


function access_fields(base_code, fields, is_head) -> ResolvedExpr:
  if is_head:
    # Head = first field
    f = fields[0]
    return ResolvedExpr(
      code = base_code + field_accessor(f.shen_name),  # e.g. ".Amount" in Go
      type = f.shen_type
    )

  # Tail = drop first field
  remaining = fields[1:]
  if len(remaining) == 0:
    return UNRESOLVED

  if len(remaining) == 1:
    # Single field remaining — resolve directly to that field
    f = remaining[0]
    return ResolvedExpr(
      code = base_code + field_accessor(f.shen_name),
      type = f.shen_type
    )

  # Multiple fields remaining — return multi-field intermediate
  return ResolvedExpr(
    base_code = base_code,
    is_multi_field = true,
    remaining_fields = remaining
  )
```

### 6c. Unwrapping

When a resolved expression has a type that is a WRAPPER or CONSTRAINED type, and the context requires a primitive (e.g., numeric comparison), emit an unwrap call:

```
function unwrap_numeric(resolved) -> string:
  if symbol_table.is_wrapper(resolved.type):
    return resolved.code + value_accessor()  # e.g. ".Val()" in Go, ".value" in TS
  return resolved.code
```

The `value_accessor()` function is target-language-specific:

| Language | Accessor |
|---|---|
| Go | `.Val()` |
| Rust | `.value()` or `.0` (tuple struct) |
| TypeScript | `.value` (private getter) |
| Python | `.value` (property) |
| Java | `.getValue()` |

### 6d. Structural match fallback

When head/tail resolution fails (the accessor chain doesn't cleanly traverse the symbol table), fall back to **structural matching**: find the base variable names on both sides of an equality, look up their types, and find fields with matching non-primitive types.

```
function structural_match_fallback(equality_expr, var_map, symbol_table):
  lhs_var = extract_deepest_variable(equality_expr.children[1])
  rhs_var = extract_deepest_variable(equality_expr.children[2])

  lhs_type_info = symbol_table.lookup(var_map[lhs_var])
  rhs_type_info = symbol_table.lookup(var_map[rhs_var])

  # Find fields with matching non-primitive types
  for lf in lhs_type_info.fields:
    for rf in rhs_type_info.fields:
      if lf.shen_type == rf.shen_type AND NOT is_primitive(lf.shen_type):
        return (
          code = target_field(lhs_var, lf) + " == " + target_field(rhs_var, rf),
          message = lhs_var + "." + lf + " must equal " + rhs_var + "." + rf
        )

  return UNRESOLVED
```

This handles cases like `(= (tail (tail (head Profile))) (tail Copy))` where `Profile` is a `known-profile` with fields `[Id, Email, Demo]` and `Copy` is `copy-content` with fields `[Body, Demo]`. The shared non-primitive type is `demographics`, found in `Demo` on both sides → `profile.demo == copy.demo`.

---

## 7. Verified Premise Translation

Each verified premise produces a boolean check in the constructor. The translation uses the expression resolver from §6.

### 7a. Top-level dispatch

```
function translate_verified(premise, var_map, symbol_table) -> (code, error_message):
  expr = parse_sexpr(premise.raw)
  op = expr.op()

  if op in [">=", "<=", ">", "<"]:
    return translate_comparison(expr, op, var_map, symbol_table)

  if op == "=":
    return translate_equality(expr, var_map, symbol_table)

  if op == "not":
    return translate_negation(expr, var_map, symbol_table)

  if op == "element?":
    return translate_membership(expr, var_map, symbol_table)

  return FALLBACK_TODO(premise.raw)
```

### 7e. Set membership (`element?`)

```
function translate_membership(expr, var_map, st):
  var_expr = expr.children[1]  # the variable being tested
  # Children 2+ are bracket-wrapped atoms: "[a", "b", "c]"
  # Strip leading [ and trailing ] from each atom
  members = parse_list_literal(expr.children[2:])
  var_code = resolve(var_expr, var_map, st)

  # Language-specific set membership:
  # Go:         map[string]bool{"a": true, "b": true}[varCode]
  # TypeScript: new Set(["a", "b", "c"]).has(varCode)
  # Rust:       ["a", "b", "c"].contains(&var_code)
  # Python:     var_code in {"a", "b", "c"}
  return (
    code = set_contains(members, unwrap(var_code)),
    msg  = var_code.code + " must be one of " + members
  )
```

### 7b. Comparison (`>=`, `<=`, `>`, `<`)

```
function translate_comparison(expr, op, var_map, st):
  lhs = resolve(expr.children[1], var_map, st)
  rhs = resolve(expr.children[2], var_map, st)
  return (
    code = unwrap_numeric(lhs) + " " + op + " " + unwrap_numeric(rhs),
    msg  = lhs.code + " must be " + op + " " + rhs.code
  )
```

### 7c. Equality (`=`)

```
function translate_equality(expr, var_map, st):
  lhs = resolve(expr.children[1], var_map, st)
  rhs = resolve(expr.children[2], var_map, st)

  if lhs is UNRESOLVED or rhs is UNRESOLVED:
    # Try structural match fallback
    result = structural_match_fallback(expr, var_map, st)
    if result is not UNRESOLVED:
      return result
    return FALLBACK_TODO(expr)

  # Unwrap if comparing wrapper to primitive
  lhs_code = maybe_unwrap(lhs, rhs.type, st)
  rhs_code = maybe_unwrap(rhs, lhs.type, st)
  return (
    code = lhs_code + " == " + rhs_code,
    msg  = lhs.code + " must equal " + rhs.code
  )
```

### 7d. Supported patterns (complete list)

| Shen expression | Resolved target code (Go-like) | Notes |
|---|---|---|
| `(>= X 10)` | `x >= 10` | Simple numeric comparison |
| `(<= X 100)` | `x <= 100` | |
| `(> X 0)` | `x > 0` | |
| `(= 0 (shen.mod X 10))` | `int(x) % 10 == 0` | Divisibility check |
| `(= 2 (length X))` | `len(x) == 2` | String/list length |
| `(not (= X []))` | `len(x) > 0` | Non-empty check |
| `(>= Bal (head Tx))` | `bal >= tx.amount.value()` | Cross-type field access via symbol table |
| `(= (tail X) (tail Y))` | `x.fieldN == y.fieldN` | Structural match on shared field type |
| `(= (head X) Y)` | `x.field0 == y` | Direct field access + variable comparison |
| `(element? Op [a b c])` | Go: `map[string]bool{"a":true,...}[op]`; TS: `new Set(["a","b","c"]).has(op)` | Set membership check |

### 7f. Fallback

When a verified premise uses a pattern shengen cannot translate, emit a language-appropriate TODO marker:

| Language | Fallback output |
|---|---|
| Go | `/* TODO: translate verified premise: (original shen) */ true` |
| Rust | `/* TODO: translate verified premise: (original shen) */ true` |
| TypeScript | `/* TODO: translate verified premise: (original shen) */ true as boolean` |
| Python | `True  # TODO: translate verified premise: (original shen)` |

The `true` ensures the generated code compiles/runs while alerting the developer that a manual check is needed.

---

## 8. Code Generation

### 8a. Output structure

Generate a single file containing:
- A header comment: `// Code generated by shengen. DO NOT EDIT.`
- Module/package declaration
- Imports (error handling, fmt, etc.)
- One type + constructor per classified rule

### 8b. Per-category templates

#### WRAPPER

```
# Input:  X : string; ==== X : email-addr;
# Output: Opaque type wrapping string, no validation

type EmailAddr:
  private value: string

  constructor(x: string) -> EmailAddr:
    return EmailAddr(value=x)

  accessor value() -> string:
    return self.value
```

#### CONSTRAINED

```
# Input:  X : number; (>= X 0) : verified; ==== X : amount;
# Output: Opaque type wrapping number, with validation

type Amount:
  private value: float

  constructor(x: float) -> Result<Amount, Error>:
    if NOT (x >= 0):
      return Error("x must be >= 0")
    return Ok(Amount(value=x))

  accessor value() -> float:
    return self.value
```

#### COMPOSITE

```
# Input:  Amount : amount; From : account-id; To : account-id;
#         ==== [Amount From To] : transaction;
# Output: Record with typed fields, no validation

type Transaction:
  field amount: Amount      # guard type, not raw float
  field from:   AccountId   # guard type, not raw string
  field to:     AccountId

  constructor(amount: Amount, from: AccountId, to: AccountId) -> Transaction:
    return Transaction(amount=amount, from=from, to=to)
```

#### GUARDED

```
# Input:  Bal : number; Tx : transaction;
#         (>= Bal (head Tx)) : verified;
#         ==== [Bal Tx] : balance-checked;
# Output: Record with typed fields + validation

type BalanceChecked:
  field bal: float
  field tx:  Transaction

  constructor(bal: float, tx: Transaction) -> Result<BalanceChecked, Error>:
    if NOT (bal >= tx.amount.value()):    # ← resolved via symbol table
      return Error("bal must be >= tx.amount")
    return Ok(BalanceChecked(bal=bal, tx=tx))
```

#### ALIAS

```
# Input:  Profile : unknown-profile; ==== Profile : prompt-required;
# Output: Type alias

type PromptRequired = UnknownProfile
```

#### SUMTYPE (generated from sum type detection)

```
# Input: Multiple blocks (human-principal, service-principal) → authenticated-principal
# Output: Abstract type + concrete variants with marker

# Go:
type AuthenticatedPrincipal interface {
    isAuthenticatedPrincipal()   # private marker method — lowercase = unexported
}
# Each variant (HumanPrincipal, ServicePrincipal) implements this interface
func (t HumanPrincipal) isAuthenticatedPrincipal() {}

# TypeScript:
export type AuthenticatedPrincipal = HumanPrincipal | ServicePrincipal;

# Rust:
pub enum AuthenticatedPrincipal {
    Human(HumanPrincipal),
    Service(ServicePrincipal),
}
```

The enforcement mechanism: any function accepting `AuthenticatedPrincipal` can only receive a value constructed through one of the variant constructors. In Go, the private marker method prevents external implementations.

### 8c. Naming conventions

Convert Shen's hyphenated names to the target language's convention:

| Language | `balance-checked` | `account-id` | `get-from-env` |
|---|---|---|---|
| Go | `BalanceChecked` | `AccountId` | `GetFromEnv` |
| Rust | `BalanceChecked` | `AccountId` | `get_from_env` |
| TypeScript | `BalanceChecked` | `AccountId` | `getFromEnv` |
| Python | `BalanceChecked` | `AccountId` | `get_from_env` |
| Java | `BalanceChecked` | `AccountId` | `getFromEnv` |

Constructor names follow language convention: `NewBalanceChecked` (Go), `BalanceChecked::new` (Rust), `BalanceChecked.create` (TS/Python), `BalanceChecked.of` (Java).

### 8d. Error handling in constructors

Use the target language's idiomatic error type:

| Language | Return type | Failure |
|---|---|---|
| Go | `(T, error)` | `return T{}, fmt.Errorf(...)` |
| Rust | `Result<T, String>` or custom error | `Err(format!(...))` |
| TypeScript | `T` (throws) or `{ ok: true, value: T } \| { ok: false, error: string }` | `throw new Error(...)` or return error variant |
| Python | `T` (raises) | `raise ValueError(...)` |
| Java | `T` (throws) or `Optional<T>` | `throw new IllegalArgumentException(...)` |

### 8e. Escaping in error messages

Generated error messages may contain `%` characters (from modulo expressions). Escape them appropriately for the target language's string formatting:

| Language | Escape |
|---|---|
| Go | `%%` in `fmt.Errorf` |
| Rust | `{{` and `}}` in `format!` |
| Python | `%%` in `%`-formatting, or just use f-strings |
| Others | Generally not needed |

---

## 9. Diagnostic Output

shengen should print the symbol table to stderr (or a diagnostic channel) so the user can verify the parse:

```
Parsed 6 datatypes from specs/core.shen

Symbol table:
  account-id                   [wrapper    ] wraps=string
  amount                       [constrained] wraps=number
  transaction                  [composite  ] {Amount:amount, From:account-id, To:account-id}
  balance-checked (block: balance-invariant) [guarded] {Bal:number, Tx:transaction}
  account-state                [composite  ] {Id:account-id, Balance:amount}
  safe-transfer                [composite  ] {Tx:transaction, Check:balance-checked}
```

Note the `(block: balance-invariant)` annotation when the block name differs from the conclusion type. This helps debug name resolution issues.

---

## 10. CLI Interface

```
shengen [OPTIONS] SPEC_FILE [PACKAGE_NAME]

Arguments:
  SPEC_FILE      Path to the .shen spec file (e.g., specs/core.shen)
  PACKAGE_NAME   Name for the generated package/module (default: "shenguard")

Options:
  --lang=LANG         Target language: go, rust, typescript, python, java (default: go)
  --out=FILE          Output file path (default: stdout)
  --dry-run           Parse and show symbol table only, don't generate code
  --db-wrappers=FILE  Also generate scoped DB wrappers (proof-carrying query builders)

Output:
  Generated code to stdout (or --out file)
  Symbol table and diagnostics to stderr
```

---

## 11. Testing Strategy

### 11a. Unit tests for the parser

Test that each Shen datatype pattern parses correctly:
- Wrapper, constrained, composite, guarded, alias
- Multiple rules per block
- Block name ≠ conclusion type
- Side conditions (`if` syntax)
- Nested head/tail in verified premises

### 11b. Unit tests for the symbol table

Test that field layouts are correct:
- Field ordering matches conclusion field order
- Types are resolved from premises
- Collision detection works for sum types

### 11c. Unit tests for accessor resolution

Test that head/tail chains resolve through the symbol table:
- `(head X)` where X is a 3-field composite → field 0
- `(tail X)` where X is a 3-field composite → fields 1-2
- `(tail X)` where X is a 2-field composite → field 1 directly (not a sub-list)
- `(head (tail X))` → field 1
- `(>= Bal (head Tx))` where Tx is a transaction → `bal >= tx.amount.value()`
- Structural match fallback for deeply nested chains

### 11d. Integration tests for generated code

**These are the most important tests.** Generate code from a known spec and verify:

1. **Valid construction succeeds**: `NewAmount(50)` returns a valid Amount
2. **Invalid construction fails**: `NewAmount(-10)` returns an error
3. **Cross-type guards work**: `NewBalanceChecked(100, tx{amount:50})` succeeds
4. **Cross-type guards reject**: `NewBalanceChecked(30, tx{amount:50})` fails
5. **Proof-carrying types require proofs**: `NewSafeTransfer(tx, proof)` requires a `BalanceChecked` — you can't pass a raw struct
6. **Constructor bypass is impossible**: verify that creating a guard type without the constructor is a compile-time error (in statically typed languages) or a runtime error (in dynamic languages)

---

## 12. Implementation Checklist

Use this as your task list when building shengen:

- [ ] **Parser**: Extract `(datatype ...)` blocks from a `.shen` file
- [ ] **Parser**: Split blocks into rules by inference lines (`====` / `____`)
- [ ] **Parser**: Parse premises (type judgments, verified premises, side conditions)
- [ ] **Parser**: Parse conclusions (composite `[A B C] : type` vs wrapped `X : type`)
- [ ] **Parser**: Skip assumption rules (containing `>>`)
- [ ] **Symbol table**: Count conclusion type producers (collision detection pre-pass)
- [ ] **Symbol table**: Build TypeInfo entries with field layouts from conclusion field order
- [ ] **Symbol table**: Handle block name ≠ conclusion type name
- [ ] **Symbol table**: Handle sum type collisions (multiple blocks → same conclusion type)
- [ ] **Symbol table**: Register synthetic SUMTYPE entries for sum type conclusion types
- [ ] **S-expression parser**: Tokenize Shen expressions
- [ ] **S-expression parser**: Parse into nested AST
- [ ] **Resolver**: Resolve atoms (variables via var_map, numeric literals)
- [ ] **Resolver**: Resolve `(head X)` via symbol table field lookup
- [ ] **Resolver**: Resolve `(tail X)` — single remaining field vs multi-field intermediate
- [ ] **Resolver**: Chain resolution: `(head (tail (tail X)))` → field 2
- [ ] **Resolver**: Unwrap WRAPPER/CONSTRAINED types for numeric/string comparison
- [ ] **Resolver**: Resolve `(shen.mod X N)` → modulo operation
- [ ] **Resolver**: Resolve `(length X)` → length function
- [ ] **Resolver**: Resolve `(not ...)` → negation
- [ ] **Resolver**: Structural match fallback for unresolvable equality expressions
- [ ] **Translator**: Comparison operators (`>=`, `<=`, `>`, `<`)
- [ ] **Translator**: Equality with resolved operands
- [ ] **Translator**: Equality with structural match fallback
- [ ] **Translator**: Negated equality
- [ ] **Translator**: `element?` set membership with list literal parsing
- [ ] **Translator**: Fallback TODO for unrecognized patterns
- [ ] **Generator**: Emit WRAPPER type + constructor + accessor
- [ ] **Generator**: Emit CONSTRAINED type + validating constructor + accessor
- [ ] **Generator**: Emit COMPOSITE type + constructor
- [ ] **Generator**: Emit GUARDED type + validating constructor
- [ ] **Generator**: Emit ALIAS type
- [ ] **Generator**: Emit SUMTYPE interface/union + variant marker methods
- [ ] **Generator**: Escape `%` and other format characters in error messages
- [ ] **Generator (optional)**: Emit scoped DB wrappers for `--db-wrappers` flag
- [ ] **Parser (optional)**: Parse `(define ...)` blocks with pattern matching and `where` guards
- [ ] **Generator**: Header comment with "DO NOT EDIT"
- [ ] **CLI**: Accept spec file path and package name arguments
- [ ] **CLI**: Print symbol table to stderr
- [ ] **Tests**: Parser tests for each rule pattern
- [ ] **Tests**: Symbol table tests for field resolution
- [ ] **Tests**: Accessor chain tests (head/tail through nested types)
- [ ] **Tests**: Integration tests with real spec → generated code → invariant enforcement
