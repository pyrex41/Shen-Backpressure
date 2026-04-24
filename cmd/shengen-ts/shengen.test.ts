// Tests ported from cmd/shengen/main_test.go.
//
// Go assertions that are specific to the Go emission path (e.g. "fmt" import,
// `struct{ v string }` shape, `tx.amount.Val()` title-case accessor) are
// translated to their TypeScript equivalents where the output model differs
// (class with `private readonly _v`, `static create`, lowercase `.val()`,
// accessor methods call-sited with `()`).

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  SymbolTable,
  parseFileString,
  parseSExpr,
  isCall,
  isAtom,
  op,
  resolveExpr,
  verifiedToTs,
  generateTs,
  isNumericLiteral,
  inferTargetFields,
  structuralMatchFallback,
  type VerifiedPremise,
  type FieldInfo,
} from "./shengen.ts";

// ============================================================================
// Parser Tests
// ============================================================================

test("parseWrapper: single-premise wrapped conclusion", () => {
  const spec = `(datatype account-id
  X : string;
  ==============
  X : account-id;)`;
  const types = parseFileString(spec);
  assert.equal(types.length, 1);
  const dt = types[0];
  assert.equal(dt.name, "account-id");
  assert.equal(dt.rules.length, 1);
  const r = dt.rules[0];
  assert.equal(r.premises.length, 1);
  assert.equal(r.premises[0].varName, "X");
  assert.equal(r.premises[0].typeName, "string");
  assert.equal(r.conc.isWrapped, true);
  assert.equal(r.conc.typeName, "account-id");
});

test("parseConstrained: verified premise is captured", () => {
  const spec = `(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)`;
  const types = parseFileString(spec);
  const r = types[0].rules[0];
  assert.equal(r.verified.length, 1);
  assert.equal(r.verified[0].raw, "(>= X 0)");
});

test("parseComposite: multi-field unwrapped conclusion", () => {
  const spec = `(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)`;
  const types = parseFileString(spec);
  const r = types[0].rules[0];
  assert.equal(r.conc.isWrapped, false);
  assert.deepEqual(r.conc.fields, ["Amount", "From", "To"]);
});

test("parseGuardedWithDifferentBlockName: conclusion type differs from block name", () => {
  const spec = `(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)`;
  const types = parseFileString(spec);
  assert.equal(types[0].name, "balance-invariant");
  assert.equal(types[0].rules[0].conc.typeName, "balance-checked");
});

test("parseSkipsAssumptionRules: >> rules are dropped", () => {
  const spec = `(datatype amount
  X : string;
  ==============
  X : amount;

  X : amount >> X : string;
  ==============
  X : amount;)`;
  const types = parseFileString(spec);
  assert.equal(types[0].rules.length, 1, ">> rule should be skipped");
});

test("parseSideCondition: `if (pred)` becomes verified premise", () => {
  const spec = `(datatype op-kind
  Op : string;
  if (element? Op [+ - * /])
  ==========================
  Op : op-kind;)`;
  const types = parseFileString(spec);
  const r = types[0].rules[0];
  assert.equal(r.verified.length, 1);
  assert.equal(r.verified[0].raw, "(element? Op [+ - * /])");
});

test("parseMultipleBlocks: two datatypes in one spec", () => {
  const spec = `\\* test spec *\\
(datatype foo
  X : string;
  ===========
  X : foo;)

(datatype bar
  X : number;
  ===========
  X : bar;)`;
  const types = parseFileString(spec);
  assert.equal(types.length, 2);
  assert.equal(types[0].name, "foo");
  assert.equal(types[1].name, "bar");
});

// ============================================================================
// Symbol Table Tests
// ============================================================================

const paymentSpec = `(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)

(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)`;

function buildPaymentSymbolTable(): SymbolTable {
  const types = parseFileString(paymentSpec);
  const st = new SymbolTable();
  st.build(types);
  return st;
}

test("symbolTable: classifies wrapper / constrained / composite / guarded", () => {
  const st = buildPaymentSymbolTable();
  const cases: Array<[string, string]> = [
    ["account-id", "wrapper"],
    ["amount", "constrained"],
    ["transaction", "composite"],
    ["balance-checked", "guarded"],
  ];
  for (const [name, category] of cases) {
    const info = st.lookup(name);
    assert.ok(info, `${name} should be present in symbol table`);
    assert.equal(info!.category, category, `${name} category`);
  }
});

test("symbolTable: composite field order is preserved", () => {
  const spec = `(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)`;
  const st = new SymbolTable();
  st.build(parseFileString(spec));
  const info = st.lookup("transaction");
  assert.ok(info);
  assert.equal(info!.fields.length, 3);
  const expected: Array<[string, string]> = [
    ["Amount", "amount"],
    ["From", "account-id"],
    ["To", "account-id"],
  ];
  for (let i = 0; i < expected.length; i++) {
    assert.equal(info!.fields[i].shenName, expected[i][0]);
    assert.equal(info!.fields[i].shenType, expected[i][1]);
  }
});

test("symbolTable: single non-primitive premise produces an alias", () => {
  const spec = `(datatype unknown-profile
  Id : user-id;
  Email : email-addr;
  ==========================
  [Id Email] : unknown-profile;)

(datatype prompt-required
  Profile : unknown-profile;
  ==========================
  Profile : prompt-required;)`;
  const st = new SymbolTable();
  st.build(parseFileString(spec));
  const info = st.lookup("prompt-required");
  assert.ok(info);
  assert.equal(info!.category, "alias");
  assert.equal(info!.wrappedType, "unknown-profile");
});

// ============================================================================
// S-Expression Parser Tests
// ============================================================================

test("parseSExpr: simple call", () => {
  const expr = parseSExpr("(>= X 10)");
  assert.ok(isCall(expr));
  assert.equal(op(expr), ">=");
  assert.equal(expr.children!.length, 3);
  assert.equal(expr.children![1].atom, "X");
  assert.equal(expr.children![2].atom, "10");
});

test("parseSExpr: nested call", () => {
  const expr = parseSExpr("(= 0 (shen.mod X 10))");
  assert.equal(op(expr), "=");
  const inner = expr.children![2];
  assert.ok(isCall(inner));
  assert.equal(op(inner), "shen.mod");
});

test("parseSExpr: atom", () => {
  const expr = parseSExpr("hello");
  assert.ok(isAtom(expr));
  assert.equal(expr.atom, "hello");
});

// ============================================================================
// Resolver Tests
// ============================================================================

test("resolveExpr: (head Tx) on composite returns first field accessor", () => {
  const st = buildPaymentSymbolTable();
  const varMap = new Map([["Tx", "transaction"]]);
  const expr = parseSExpr("(head Tx)");
  const resolved = resolveExpr(st, expr, varMap);
  assert.ok(resolved, "should resolve");
  // TS uses method accessors, so `.amount()` not `.amount` like Go.
  assert.equal(resolved!.code, "tx.amount()");
  assert.equal(resolved!.shenType, "amount");
});

test("resolveExpr: (tail P) on two-field composite returns the lone remaining field", () => {
  const spec = `(datatype pair
  A : string;
  B : number;
  ===================
  [A B] : pair;)`;
  const st = new SymbolTable();
  st.build(parseFileString(spec));
  const varMap = new Map([["P", "pair"]]);
  const expr = parseSExpr("(tail P)");
  const resolved = resolveExpr(st, expr, varMap);
  assert.ok(resolved);
  assert.equal(resolved!.code, "p.b()");
});

test("resolveExpr: (head (tail Tx)) returns second field accessor", () => {
  const st = buildPaymentSymbolTable();
  const varMap = new Map([["Tx", "transaction"]]);
  const expr = parseSExpr("(head (tail Tx))");
  const resolved = resolveExpr(st, expr, varMap);
  assert.ok(resolved);
  assert.equal(resolved!.code, "tx.from()");
});

test("verifiedToTs: balance-invariant resolves with wrapper unwrap", () => {
  const st = buildPaymentSymbolTable();
  const varMap = new Map([
    ["Bal", "number"],
    ["Tx", "transaction"],
  ]);
  const v: VerifiedPremise = { raw: "(>= Bal (head Tx))" };
  const [code] = verifiedToTs(st, v, varMap);
  // Mirrors Go's `bal >= tx.amount.Val()`, adjusted for TS method accessor
  // and lowercase `val()` convention.
  assert.equal(code, "bal >= tx.amount().val()");
});

test("verifiedToTs: (= 0 (shen.mod X 10)) emits Math.trunc modulo", () => {
  const spec = `(datatype decade
  X : number;
  (= 0 (shen.mod X 10)) : verified;
  ===================================
  X : decade;)`;
  const st = new SymbolTable();
  st.build(parseFileString(spec));
  const varMap = new Map([["X", "number"]]);
  const v: VerifiedPremise = { raw: "(= 0 (shen.mod X 10))" };
  const [code] = verifiedToTs(st, v, varMap);
  assert.ok(
    code.includes("Math.trunc(x) % 10"),
    `expected Math.trunc(x) %% 10 in ${code}`
  );
});

test("verifiedToTs: (length X) emits .length", () => {
  const spec = `(datatype us-state
  X : string;
  (= 2 (length X)) : verified;
  =============================
  X : us-state;)`;
  const st = new SymbolTable();
  st.build(parseFileString(spec));
  const varMap = new Map([["X", "string"]]);
  const v: VerifiedPremise = { raw: "(= 2 (length X))" };
  const [code] = verifiedToTs(st, v, varMap);
  assert.ok(code.includes("x.length"), `expected x.length in ${code}`);
});

// ============================================================================
// isNumericLiteral
// ============================================================================

test("isNumericLiteral: accepts ints, floats, and signed variants; rejects garbage", () => {
  const cases: Array<[string, boolean]> = [
    ["0", true],
    ["42", true],
    ["-5", true],
    ["3.14", true],
    ["-0.5", true],
    ["", false],
    ["abc", false],
    ["--5", false],
    ["..", false],
    ["5.3.2", false],
    ["-", false],
    [".", false],
  ];
  for (const [input, want] of cases) {
    assert.equal(
      isNumericLiteral(input),
      want,
      `isNumericLiteral(${JSON.stringify(input)})`
    );
  }
});

// ============================================================================
// Integration Tests (TS output shape)
// ============================================================================

test("generateTs: payment spec produces classes with the expected shape", () => {
  const st = buildPaymentSymbolTable();
  const types = parseFileString(paymentSpec);
  const output = generateTs(types, st, "specs/core.shen");

  assert.ok(
    output.includes("// Code generated by shengen-ts from specs/core.shen. DO NOT EDIT."),
    "missing or wrong header"
  );

  // Wrapper: class with private readonly _v, static create, val().
  assert.ok(
    output.includes("export class AccountId {"),
    "missing AccountId class"
  );
  assert.ok(
    output.includes("private readonly _v: string;"),
    "AccountId should store a private string"
  );

  // Constrained: static createOrThrow for Amount taking number.
  assert.ok(
    output.includes("static createOrThrow(x: number): Amount"),
    "missing Amount static createOrThrow"
  );

  // Composite: private fields with underscore prefix, accessor methods.
  assert.ok(
    output.includes("private readonly _amount: Amount;"),
    "Transaction should have private _amount field"
  );
  assert.ok(
    output.includes("amount(): Amount { return this._amount; }"),
    "missing amount() accessor on Transaction"
  );
  assert.ok(
    output.includes("from(): AccountId { return this._from; }"),
    "missing from() accessor on Transaction"
  );

  // Guarded: the balance check should unwrap Amount via method chain.
  assert.ok(
    output.includes("tx.amount().val()"),
    "guarded check should reach through accessor + unwrap"
  );

  // Guarded class (BalanceChecked) has accessor methods for its fields.
  assert.ok(
    output.includes("bal(): number { return this._bal; }"),
    "missing bal() accessor on BalanceChecked"
  );
});

// ============================================================================
// §3.1 regression: sum-type variants must not be misclassified as aliases.
// ============================================================================

test("symbolTable: sum-type variant wrapping a non-primitive is not an alias", () => {
  // Two datatype blocks both conclude `tx-outcome` — so tx-outcome is a sum type.
  // `transaction-success` has a single non-primitive premise (transaction).
  // Before the isSumVariant guard landed, it was misclassified as an alias and
  // the generator emitted `export type TransactionSuccess = Transaction`,
  // erasing the variant tag.
  const spec = `(datatype transaction-success
  Tx : transaction;
  ==========================
  Tx : tx-outcome;)

(datatype transaction-failure
  Err : string;
  ==========================
  Err : tx-outcome;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);

  const success = st.lookup("transaction-success");
  assert.ok(success, "transaction-success should be in the symbol table");
  assert.notEqual(
    success!.category,
    "alias",
    "sum-type variant must not be classified as alias"
  );
});

test("generateTs: sum-type variant emits a class, not a type alias", () => {
  const spec = `(datatype transaction-success
  Tx : transaction;
  ==========================
  Tx : tx-outcome;)

(datatype transaction-failure
  Err : string;
  ==========================
  Err : tx-outcome;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);
  const out = generateTs(types, st, "test.shen");

  assert.ok(
    !out.includes("export type TransactionSuccess ="),
    `variant TransactionSuccess should not emit as type alias; output was:\n${out}`
  );
  assert.ok(
    out.includes("export class TransactionSuccess"),
    `variant TransactionSuccess should emit as a class; output was:\n${out}`
  );
});

// ============================================================================
// §3.3 regression: inferTargetFields + precise structuralMatchFallback.
// ============================================================================

test("inferTargetFields: no head/tail leaves all fields", () => {
  const fields: FieldInfo[] = [
    { index: 0, shenName: "A", shenType: "alpha" },
    { index: 1, shenName: "B", shenType: "beta" },
    { index: 2, shenName: "C", shenType: "gamma" },
  ];
  const expr = parseSExpr("X");
  assert.deepEqual(inferTargetFields(expr, fields), fields);
});

test("inferTargetFields: tail drops the first field", () => {
  const fields: FieldInfo[] = [
    { index: 0, shenName: "A", shenType: "alpha" },
    { index: 1, shenName: "B", shenType: "beta" },
    { index: 2, shenName: "C", shenType: "gamma" },
  ];
  const result = inferTargetFields(parseSExpr("(tail X)"), fields);
  assert.equal(result.length, 2);
  assert.equal(result[0].shenName, "B");
  assert.equal(result[1].shenName, "C");
});

test("inferTargetFields: two tails drop two fields; head preserves the index", () => {
  const fields: FieldInfo[] = [
    { index: 0, shenName: "A", shenType: "alpha" },
    { index: 1, shenName: "B", shenType: "beta" },
    { index: 2, shenName: "C", shenType: "gamma" },
  ];
  const result = inferTargetFields(parseSExpr("(head (tail (tail X)))"), fields);
  assert.equal(result.length, 1);
  assert.equal(result[0].shenName, "C");
});

test("inferTargetFields: tail depth beyond field count falls back to last field", () => {
  const fields: FieldInfo[] = [
    { index: 0, shenName: "A", shenType: "alpha" },
    { index: 1, shenName: "B", shenType: "beta" },
  ];
  const result = inferTargetFields(parseSExpr("(tail (tail (tail X)))"), fields);
  assert.equal(result.length, 1);
  assert.equal(result[0].shenName, "B");
});

test("structuralMatchFallback: picks the tail-targeted pair, not the first shared non-primitive", () => {
  // Two composites sharing `user-ref` at field 0 AND `demographics` at field 2.
  // (= (tail (tail L)) (tail (tail R))) clearly points at field 2 on each side.
  // Without inferTargetFields the scan picked L.user == R.owner (the user-ref
  // pair at index 0). With the fix it targets L.demo == R.info.
  const spec = `(datatype lhs-struct
  User : user-ref;
  Name : string;
  Demo : demographics;
  ==========================
  [User Name Demo] : lhs-struct;)

(datatype rhs-struct
  Owner : user-ref;
  Label : string;
  Info : demographics;
  ==========================
  [Owner Label Info] : rhs-struct;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);

  const varMap = new Map([
    ["L", "lhs-struct"],
    ["R", "rhs-struct"],
  ]);
  const expr = parseSExpr("(= (tail (tail L)) (tail (tail R)))");
  const result = structuralMatchFallback(st, expr, varMap);
  assert.ok(result, "fallback should return a result");
  const [code] = result!;
  assert.ok(
    code.includes("l.demo") && code.includes("r.info"),
    `expected tail-targeted pair l.demo === r.info, got ${code}`
  );
  assert.ok(
    !(code.includes("l.user") || code.includes("r.owner")),
    `fallback should not pick the field-0 user-ref pair; got ${code}`
  );
});

// ============================================================================
// §3.2 + §2.2 regressions: createOrThrow/tryCreate + must* exports
// ============================================================================

test("generateTs: emits createOrThrow + tryCreate (not the old `create`) and mustX free functions", () => {
  const types = parseFileString(paymentSpec);
  const st = new SymbolTable();
  st.build(types);
  const out = generateTs(types, st, "specs/core.shen");

  // Old infallible `static create` must be gone across all categories.
  assert.ok(
    !out.match(/static create\(/),
    `legacy static create(...) signature should no longer appear; output was:\n${out}`
  );

  // Each category carries the new API pair.
  assert.ok(
    out.includes("static createOrThrow(x: number): Amount"),
    "Amount.createOrThrow missing"
  );
  assert.ok(
    out.includes("static tryCreate(x: number): Amount | Error"),
    "Amount.tryCreate should return Amount | Error"
  );
  assert.ok(
    out.includes("static createOrThrow(x: string): AccountId"),
    "AccountId.createOrThrow missing"
  );
  assert.ok(
    out.includes("static tryCreate(x: string): AccountId | Error"),
    "AccountId.tryCreate should return AccountId | Error"
  );
  assert.ok(
    /static createOrThrow\(amount: Amount, from: AccountId, to: AccountId\): Transaction/.test(out),
    "Transaction.createOrThrow should take positional composite fields"
  );
  assert.ok(
    /static tryCreate\(amount: Amount, from: AccountId, to: AccountId\): Transaction \| Error/.test(out),
    "Transaction.tryCreate should return Transaction | Error"
  );

  // Free-function must* exports at module level.
  assert.ok(
    out.includes("export function mustAmount(x: number): Amount"),
    "mustAmount free function missing"
  );
  assert.ok(
    out.includes("export function mustAccountId(x: string): AccountId"),
    "mustAccountId free function missing"
  );
  assert.ok(
    /export function mustTransaction\(amount: Amount, from: AccountId, to: AccountId\): Transaction/.test(out),
    "mustTransaction should take positional composite fields"
  );
  assert.ok(
    /export function mustBalanceChecked\(bal: number, tx: Transaction\): BalanceChecked/.test(out),
    "mustBalanceChecked should take guarded fields"
  );
});

test("generated code: createOrThrow throws on failing constraint; tryCreate returns Error", async () => {
  // Compile a tiny spec to TS, write it to a temp file, and import it. This is
  // the closest-to-real check that the runtime contract actually holds.
  const { writeFileSync, mkdtempSync } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const spec = `(datatype positive
  X : number;
  (>= X 0) : verified;
  =====================
  X : positive;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);
  const source = generateTs(types, st, "positive.shen");

  const dir = mkdtempSync(join(tmpdir(), "shengen-ts-test-"));
  const file = join(dir, "positive.ts");
  writeFileSync(file, source);

  const mod: { Positive: { createOrThrow(x: number): unknown; tryCreate(x: number): unknown }; mustPositive(x: number): unknown } = await import(file);
  // Happy path: createOrThrow returns a Positive.
  const v = mod.Positive.createOrThrow(5);
  assert.ok(v, "createOrThrow(5) should return a value");
  // Failing path: createOrThrow throws.
  assert.throws(() => mod.Positive.createOrThrow(-1));
  // tryCreate returns an Error for invalid inputs, not throws.
  const err = mod.Positive.tryCreate(-1);
  assert.ok(err instanceof Error, "tryCreate(-1) should return an Error instance");
  // mustPositive is createOrThrow in disguise — throws on invalid.
  assert.throws(() => mod.mustPositive(-1));
  assert.ok(mod.mustPositive(7));
});

// ============================================================================
// §2.3 regression: --pkg annotates the header, does not change class emission.
// ============================================================================

test("generateTs: --pkg option annotates the header comment only", () => {
  const spec = `(datatype name
  X : string;
  ===========
  X : name;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);

  const withoutPkg = generateTs(types, st, "t.shen");
  const withPkg = generateTs(types, st, "t.shen", { pkg: "guards" });

  assert.ok(!withoutPkg.includes("Logical package:"));
  assert.ok(withPkg.includes("// Logical package: guards"));
  assert.ok(
    withPkg.includes('import as `import * as guards from "./…"`'),
    `header should hint how to import — got:\n${withPkg.slice(0, 500)}`
  );

  // pkg must NOT leak into emitted class/function signatures.
  const stripHeader = (s: string): string =>
    s
      .split("\n")
      .filter((line) => !line.startsWith("//"))
      .join("\n");
  assert.equal(
    stripHeader(withoutPkg),
    stripHeader(withPkg),
    "--pkg should only affect the header comment, not code emission"
  );
});

test("generateTs: wrapper-only spec has no constrained/guarded checks or imports", () => {
  const spec = `(datatype name
  X : string;
  ===========
  X : name;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);
  const output = generateTs(types, st, "test.shen");

  // TS analogue of Go's "no fmt import" check: a wrapper-only spec should not
  // import anything (generator currently emits no imports at all, but pin the
  // behavior so regressions in codegen that add stray imports get caught).
  assert.ok(
    !output.match(/^import\s+/m),
    "wrapper-only spec should not emit imports"
  );
  // And no throw statements — wrappers never fail.
  assert.ok(
    !output.includes("throw new Error"),
    "wrapper-only spec should have no throw statements"
  );
});
