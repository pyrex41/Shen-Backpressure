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
  parseSpecString,
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
  mergeDatatypeGroups,
  parseDefine,
  parseSignature,
  splitPatterns,
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

test("symbolTable: wrapped non-primitive with verified premise is constrained, not alias", () => {
  // Regression: bounded-base64url wraps base64url (non-primitive) plus a
  // length cap. Before the fix the classifier called this `alias` and the
  // runtime `length <= 100000` check silently disappeared.
  const spec = `(datatype base64url
  X : string;
  (>= (length X) 0) : verified;
  =========================
  X : base64url;)

(datatype bounded-base64url
  X : base64url;
  (<= (length X) 100000) : verified;
  ====================================
  X : bounded-base64url;)`;
  const st = new SymbolTable();
  st.build(parseFileString(spec));
  const info = st.lookup("bounded-base64url");
  assert.ok(info);
  assert.equal(info!.category, "constrained");
  assert.equal(info!.wrappedType, "base64url");
});

test("generateTs: wrapped-non-primitive constrained emits class with runtime check", () => {
  const spec = `(datatype base64url
  X : string;
  (>= (length X) 0) : verified;
  =========================
  X : base64url;)

(datatype bounded-base64url
  X : base64url;
  (<= (length X) 100000) : verified;
  ====================================
  X : bounded-base64url;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);
  const out = generateTs(types, st, "test.shen");
  assert.ok(
    out.includes("export class BoundedBase64url"),
    "BoundedBase64url must be emitted as a class, not a type alias"
  );
  assert.ok(
    out.includes("x.val().length <= 100000"),
    "length check must appear in the generated code"
  );
  assert.ok(
    !out.includes("export type BoundedBase64url"),
    "must not emit a type alias for bounded-base64url"
  );
});

test("verifiedToTs: element? with quoted string atoms emits singly-quoted set literal", () => {
  // Regression: source `(element? X ["s" "e" "m"])` used to produce
  // `new Set([""s"", ""e"", ""m""])` — invalid TS. Atoms that already carry
  // surrounding `"` chars must pass through the generator unchanged.
  const spec = `(datatype tag
  X : string;
  (element? X ["s" "e" "m"]) : verified;
  =========================================
  X : tag;)`;
  const types = parseFileString(spec);
  const st = new SymbolTable();
  st.build(types);
  const out = generateTs(types, st, "test.shen");
  assert.ok(
    out.includes('new Set(["s", "e", "m"])'),
    `set literal should be new Set(["s", "e", "m"]); got:\n${out}`
  );
  assert.ok(
    !out.includes('""s""'),
    "must not emit doubled quotes on element? atoms"
  );
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

// ============================================================================
// §2.4 regression: multi-file spec input via mergeDatatypeGroups.
// ============================================================================

test("mergeDatatypeGroups: concatenates distinct datatypes from multiple files", () => {
  const a = parseFileString(`(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)`);
  const b = parseFileString(`(datatype account-id
  X : string;
  ==============
  X : account-id;)`);

  const merged = mergeDatatypeGroups([
    { path: "a.shen", datatypes: a },
    { path: "b.shen", datatypes: b },
  ]);
  assert.equal(merged.length, 2);
  const names = merged.map((d) => d.name).sort();
  assert.deepEqual(names, ["account-id", "amount"]);

  // Both should land in the same symbol table cleanly.
  const st = new SymbolTable();
  st.build(merged);
  assert.equal(st.lookup("amount")!.category, "constrained");
  assert.equal(st.lookup("account-id")!.category, "wrapper");
});

test("mergeDatatypeGroups: rejects cross-file redefinitions", () => {
  const a = parseFileString(`(datatype amount
  X : number;
  ====================
  X : amount;)`);
  const b = parseFileString(`(datatype amount
  X : string;
  ====================
  X : amount;)`);
  assert.throws(
    () =>
      mergeDatatypeGroups([
        { path: "a.shen", datatypes: a },
        { path: "b.shen", datatypes: b },
      ]),
    /declared in both a\.shen and b\.shen/
  );
});

test("mergeDatatypeGroups: a file referenced twice is not a redefinition", () => {
  // The same source path appears twice — common when a wrapper script or
  // user accidentally duplicates a --spec flag. The validator should only
  // reject true cross-file conflicts, not harmless repeats.
  const a = parseFileString(`(datatype amount
  X : number;
  ====================
  X : amount;)`);
  const merged = mergeDatatypeGroups([
    { path: "a.shen", datatypes: a },
    { path: "a.shen", datatypes: a },
  ]);
  // Note: datatypes still appear twice in the merged array (no de-dup); the
  // check is purely about cross-file *conflicts*. Within-file semantics are
  // parseFile's responsibility, not mergeDatatypeGroups'.
  assert.equal(merged.length, 2);
});

// ============================================================================
// §2.1 — (define …) parser, translator, and helper emission.
// ============================================================================

test("parseSignature: arrow-chain signatures parse into ordered parts", () => {
  assert.deepEqual(parseSignature("{string --> boolean}"), ["string", "boolean"]);
  assert.deepEqual(
    parseSignature("{amount --> (list transaction) --> boolean}"),
    ["amount", "(list transaction)", "boolean"]
  );
  assert.deepEqual(parseSignature("garbage"), []);
  assert.deepEqual(parseSignature("{}"), []);
});

test("splitPatterns: respects bracket nesting", () => {
  assert.deepEqual(splitPatterns("X Y"), ["X", "Y"]);
  assert.deepEqual(splitPatterns("[Med | Meds]"), ["[Med | Meds]"]);
  assert.deepEqual(splitPatterns("X [[A B] | Rest]"), ["X", "[[A B] | Rest]"]);
});

test("parseDefine: single-clause, all-variable patterns", () => {
  const block = `(define roundtrip?
  {site-data --> boolean}
  S -> (= S (decode (encode S))))`;
  const def = parseDefine(block);
  assert.ok(def, "should parse");
  assert.equal(def!.name, "roundtrip?");
  assert.deepEqual(def!.signature, ["site-data", "boolean"]);
  assert.equal(def!.clauses.length, 1);
  assert.deepEqual(def!.clauses[0].patterns, ["S"]);
  assert.equal(def!.clauses[0].result, "(= S (decode (encode S)))");
  assert.equal(def!.clauses[0].guard, "");
});

test("parseDefine: multi-clause with literal and variable patterns", () => {
  const block = `(define base64url?
  {string --> boolean}
  "" -> true
  X -> (and (base64url-char? (head-char X))
            (base64url? (tail-chars X))))`;
  const def = parseDefine(block);
  assert.ok(def);
  assert.equal(def!.clauses.length, 2);
  assert.deepEqual(def!.clauses[0].patterns, [`""`]);
  assert.equal(def!.clauses[0].result, "true");
  assert.deepEqual(def!.clauses[1].patterns, ["X"]);
  assert.ok(
    def!.clauses[1].result.startsWith("(and"),
    `unexpected result: ${def!.clauses[1].result}`
  );
});

test("parseSpecString: returns both datatypes and defines", () => {
  const src = `(datatype amount
  X : number;
  ====================
  X : amount;)

(define positive?
  {number --> boolean}
  X -> (>= X 0))`;
  const spec = parseSpecString(src);
  assert.equal(spec.datatypes.length, 1);
  assert.equal(spec.datatypes[0].name, "amount");
  assert.equal(spec.defines.length, 1);
  assert.equal(spec.defines[0].name, "positive?");
});

test("generateTs: single-clause define emits an exported function", () => {
  const src = `(datatype site-data
  S : string;
  ==============
  S : site-data;)

(define roundtrip?
  {site-data --> boolean}
  S -> (= S (decode (encode S))))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");

  assert.ok(
    /export function roundtrip\(s: SiteData\): boolean \{\s*return \(s === decode\(encode\(s\)\)\);\s*\}/m.test(out),
    `expected roundtrip helper; got:\n${out}`
  );
});

test("generateTs: multi-clause define with \"\" literal emits if-chain", () => {
  const src = `(define base64url?
  {string --> boolean}
  "" -> true
  X -> (and (base64url-char? (head-char X))
            (base64url? (tail-chars X))))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");

  assert.ok(
    /export function base64url\(x: string\): boolean/.test(out),
    "missing base64url function signature"
  );
  assert.ok(
    /if \(x === ""\)/.test(out),
    "missing empty-string check"
  );
  // `head-char` and `tail-chars` are recognized primitives — they translate
  // to inline `x[0] ?? ""` / `x.slice(1)` rather than function calls.
  assert.ok(
    /base64urlChar\(\(x\[0\] \?\? ""\)\)/.test(out),
    `missing (head-char x) → (x[0] ?? "") translation; got:\n${out}`
  );
  assert.ok(
    /base64url\(x\.slice\(1\)\)/.test(out),
    `missing (tail-chars x) → x.slice(1) translation; got:\n${out}`
  );
});

test("translateDefineExpr: in-range? emits lexical bounds check", () => {
  // `(in-range? C A B)` is a char-range alphabet check (A-Z, 0-9, …).
  // TypeScript string comparison is lexicographic, which matches the
  // intended single-char semantics.
  const src = `(define upper-alpha?
  {string --> boolean}
  C -> (in-range? C "A" "Z"))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");
  assert.ok(
    /\(c >= "A" && c <= "Z"\)/.test(out),
    `expected lexical A..Z check; got:\n${out}`
  );
});

test("generateTs: filterUnreferencedDefines=true drops derivation-target defines", () => {
  // `roundtrip?` is a classic derive target — it calls the consumer's real
  // `encode`/`decode`, which don't exist in the spec. When the filter flag
  // is set, such defines must be skipped so the emitted TS stays compilable.
  const src = `(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(define roundtrip?
  {amount --> boolean}
  X -> (= X (decode (encode X))))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);

  const filtered = generateTs(spec.datatypes, st, "t.shen", {
    filterUnreferencedDefines: true,
  });
  assert.ok(
    !/export function roundtrip\b/.test(filtered),
    `roundtrip? should be dropped with filter on; got:\n${filtered}`
  );

  const unfiltered = generateTs(spec.datatypes, st, "t.shen");
  assert.ok(
    /export function roundtrip\b/.test(unfiltered),
    "roundtrip? should still emit without the filter (default behavior)"
  );
});

test("generateTs: filterUnreferencedDefines keeps transitively-reached defines", () => {
  // `base64url?` is reached from the `base64url` datatype's :verified premise;
  // `base64url-char?` is reached transitively through base64url?'s body. Both
  // must survive the filter.
  const src = `(define base64url-char?
  {string --> boolean}
  C -> (in-range? C "A" "Z"))

(define base64url?
  {string --> boolean}
  "" -> true
  X -> (and (base64url-char? (head-char X))
            (base64url? (tail-chars X))))

(datatype base64url
  X : string;
  (base64url? X) : verified;
  ============================
  X : base64url;)`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen", {
    filterUnreferencedDefines: true,
  });
  assert.ok(
    /export function base64url\b/.test(out),
    "base64url? (directly referenced) should survive filter"
  );
  assert.ok(
    /export function base64urlChar\b/.test(out),
    "base64url-char? (transitively referenced) should survive filter"
  );
});

test("verifiedToTs: (define …) calls dispatch as function calls in guards", () => {
  // A constrained type that uses a user-defined predicate in its :verified
  // premise. The helper emitter proves the predicate exists; verifiedToTs
  // routes the guard to the helper's TS name.
  const src = `(define positive?
  {number --> boolean}
  X -> (>= X 0))

(datatype amount
  X : number;
  (positive? X) : verified;
  ====================
  X : amount;)`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");

  // The constrained amount's createOrThrow should call `positive(...)`, not
  // emit a `/* TODO */` placeholder.
  assert.ok(
    !/TODO:.*positive/.test(out),
    "amount should not emit a TODO for positive? check"
  );
  assert.ok(
    /if \(!\(positive\(x\)\)\) throw/.test(out),
    `amount should guard with positive(x); got:\n${out}`
  );
});

// ============================================================================
// §2.1 follow-up: destructuring patterns + `where` guards.
// ============================================================================

test("generateTs: [H | T] cons destructure binds head and tail", () => {
  const src = `(define all-positive?
  {(list number) --> boolean}
  [] -> true
  [H | T] -> (and (>= H 0) (all-positive? T)))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");

  // No top-level variable across clauses, so the param falls back to arg0.
  assert.ok(
    /export function allPositive\(arg0: number\[\]\): boolean/.test(out),
    `expected allPositive signature with arg0 fallback; got:\n${out}`
  );
  assert.ok(/if \(arg0\.length === 0\) \{\s+return true;/m.test(out));
  // Head bound as h, tail bound as t (from the pattern variable), both
  // visible to the recursive body.
  assert.ok(out.includes("const h = arg0[0];"));
  assert.ok(out.includes("const t = arg0.slice(1);"));
  assert.ok(
    /return \(\(h >= 0\) && allPositive\(t\)\);/.test(out),
    `recursive tail call should use the bound t; got:\n${out}`
  );
});

test("generateTs: [[A B] | Rest] nested destructure binds composite fields", () => {
  const src = `(datatype contraindication
  DrugA : string;
  DrugB : string;
  =====================
  [DrugA DrugB] : contraindication;)

(define contra-of?
  {string --> (list contraindication) --> boolean}
  _ [] -> false
  D [[A B] | Rest] -> (or (= D A) (= D B))
  D [_ | Rest] -> (contra-of? D Rest))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");

  // Param 0 is `d` from the top-level D binder. Param 1 has no top-level
  // variable across clauses (all destructures), so it falls back to arg1.
  assert.ok(
    /export function contraOf\(d: string, arg1: Contraindication\[\]\): boolean/.test(out),
    `expected contraOf signature; got:\n${out}`
  );
  assert.ok(
    /if \(arg1\.length === 0\) \{\s+return false;/m.test(out),
    `expected empty-list clause; got:\n${out}`
  );
  assert.ok(
    /const __p1Head = arg1\[0\];/.test(out),
    `expected head binding; got:\n${out}`
  );
  assert.ok(
    /const a = __p1Head\.drugA\(\);/.test(out),
    `expected A to bind to drugA(); got:\n${out}`
  );
  assert.ok(
    /const b = __p1Head\.drugB\(\);/.test(out),
    `expected B to bind to drugB(); got:\n${out}`
  );
  assert.ok(
    /return \(\(d === a\) \|\| \(d === b\)\);/.test(out),
    `expected body expansion; got:\n${out}`
  );
});

test("generateTs: `where` guards turn into an inner if-condition", () => {
  // Same shape as contra-of?, but now the matching logic lives in a where
  // guard so the clause only fires when the guard passes.
  const src = `(datatype contraindication
  DrugA : string;
  DrugB : string;
  =====================
  [DrugA DrugB] : contraindication;)

(define contra-of?
  {string --> (list contraindication) --> boolean}
  _ [] -> false
  D [[A B] | Rest] where (or (= D A) (= D B)) -> true
  D [_ | Rest] -> (contra-of? D Rest))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");

  // Guard should show up as an inner if. `true` body.
  assert.ok(
    /if \(\(\(d === a\) \|\| \(d === b\)\)\) \{\s+return true;/m.test(out),
    `expected guard-wrapped true-return; got:\n${out}`
  );
  // Fallthrough clause recurses on the sliced tail (bound as `rest`).
  assert.ok(
    /const rest = arg1\.slice\(1\);[\s\S]*?return contraOf\(d, rest\)/.test(out),
    `expected fallthrough recursion; got:\n${out}`
  );
});

test("generateTs: `val` in define bodies routes through defensive __val helper", async () => {
  // Specs in the wild sometimes apply `(val X)` to values that might already
  // be primitives (e.g. a scanl accumulator seeded with `(val B0)` that then
  // re-unwraps `B` inside the body). Emit `__val(x)` so primitives flow
  // through unchanged instead of crashing at runtime.
  const src = `(datatype amount
  X : number;
  ====================
  X : amount;)

(define double-unwrap?
  {amount --> boolean}
  A -> (>= (val A) 0))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "t.shen");

  assert.ok(
    /return \(__val\(a\) >= 0\);/.test(out),
    `expected __val(a) wrapping in body; got:\n${out}`
  );
  assert.ok(/function __val\(x: any\): any/.test(out), "missing __val helper");

  // Runtime: passing a wrapper unwraps; passing a primitive passes through.
  const { writeFileSync, mkdtempSync } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const dir = mkdtempSync(join(tmpdir(), "shengen-ts-val-"));
  const file = join(dir, "val-defensive.ts");
  writeFileSync(file, out);
  const mod: {
    mustAmount(x: number): { val(): number };
    doubleUnwrap(a: unknown): boolean;
  } = await import(file);
  assert.equal(mod.doubleUnwrap(mod.mustAmount(5)), true);
  assert.equal(mod.doubleUnwrap(5), true, "primitive should pass through __val");
  assert.equal(mod.doubleUnwrap(mod.mustAmount(-1)), false);
});

test("generated code: destructure + guard define executes correctly at runtime", async () => {
  // End-to-end: compile the spec, import the generated module, and exercise
  // the emitted function. This is the runtime contract the string-matching
  // tests can only hint at.
  const { writeFileSync, mkdtempSync } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");

  const src = `(datatype contraindication
  DrugA : string;
  DrugB : string;
  =====================
  [DrugA DrugB] : contraindication;)

(define contra-of?
  {string --> (list contraindication) --> boolean}
  _ [] -> false
  D [[A B] | Rest] where (or (= D A) (= D B)) -> true
  D [_ | Rest] -> (contra-of? D Rest))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const source = generateTs(spec.datatypes, st, "contra.shen");

  const dir = mkdtempSync(join(tmpdir(), "shengen-ts-contra-"));
  const file = join(dir, "contra.ts");
  writeFileSync(file, source);

  type ContraMod = {
    Contraindication: {
      createOrThrow(a: string, b: string): { drugA(): string; drugB(): string };
    };
    mustContraindication(
      a: string,
      b: string
    ): { drugA(): string; drugB(): string };
    contraOf(d: string, list: Array<{ drugA(): string; drugB(): string }>): boolean;
  };
  const mod: ContraMod = await import(file);

  const c1 = mod.mustContraindication("warfarin", "aspirin");
  const c2 = mod.mustContraindication("metformin", "insulin");

  // Drug that appears as drugA in a pair.
  assert.equal(mod.contraOf("warfarin", [c1, c2]), true);
  // Drug that appears as drugB.
  assert.equal(mod.contraOf("insulin", [c1, c2]), true);
  // Drug that doesn't appear.
  assert.equal(mod.contraOf("acetaminophen", [c1, c2]), false);
  // Empty list.
  assert.equal(mod.contraOf("warfarin", []), false);
});

// ============================================================================
// Tokenizer list-sugar + premise-var alias regressions.
// ============================================================================

test("parseSExpr: `[H | T]` body expression parses as (__cons H T)", () => {
  const expr = parseSExpr("[H | T]");
  assert.ok(isCall(expr), "[H | T] should parse as a call");
  assert.equal(op(expr), "__cons");
  assert.equal(expr.children!.length, 3);
  assert.equal(expr.children![1].atom, "H");
  assert.equal(expr.children![2].atom, "T");
});

test("parseSExpr: `[a b c]` parses as (__list a b c)", () => {
  const expr = parseSExpr("[a b c]");
  assert.ok(isCall(expr));
  assert.equal(op(expr), "__list");
  assert.deepEqual(
    expr.children!.slice(1).map((c) => c.atom),
    ["a", "b", "c"]
  );
});

test("parseSExpr: `[]` parses as (__nil)", () => {
  const expr = parseSExpr("[]");
  assert.ok(isCall(expr));
  assert.equal(op(expr), "__nil");
});

test("generateTs: define body using `[H | Visited]` translates to cons spread", () => {
  // Regression: the previous tokenizer treated `[`, `]`, `|` as regular
  // characters, so `[(content-address-of-node Root) | Visited]` in a define
  // body emitted `[, contentAddressOfNode(root), |, visited]` — invalid TS.
  const src = `(datatype site-node
  X : string;
  ==================
  X : site-node;)

(define note-cons?
  {site-node --> (list string) --> boolean}
  Root Visited -> (member? Root [Root | Visited]))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "test.shen");
  assert.ok(
    /\[root, \.\.\.visited\]/.test(out),
    `expected cons-sugar body to emit [root, ...visited]; got:\n${out}`
  );
  assert.ok(
    !/\[,\s+root,\s+\|,\s+visited\]/.test(out),
    "raw [ | ] tokens must not leak into the output"
  );
});

test("generateTs: constrained class aliases the Shen premise variable onto x", () => {
  // Regression: a spec like `E : encoded-fragment; (has-no-refs? E) : verified`
  // used to emit `if (!(hasNoRefs(e))) ...` against a constructor whose param
  // was `x` — leaving `e` unbound. Now the generator inserts `const e = x;`
  // when the premise variable's camelCase differs from `x`.
  const src = `(datatype encoded-fragment
  X : string;
  ==================
  X : encoded-fragment;)

(define has-no-refs?
  {encoded-fragment --> boolean}
  E -> (>= 0 0))

(datatype leaf-node
  E : encoded-fragment;
  (has-no-refs? E) : verified;
  ==========================
  E : leaf-node;)`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "test.shen");
  // The constrained class should alias e := x.
  assert.ok(
    /static createOrThrow\(x: EncodedFragment\): LeafNode \{[\s\S]*?const e = x;[\s\S]*?if \(!\(hasNoRefs\(e\)\)\) throw/.test(out),
    `expected premise-alias + call against e; got:\n${out}`
  );
});

test("translateDefineExpr: member? emits .includes()", () => {
  const src = `(define contains-self?
  {string --> (list string) --> boolean}
  X L -> (member? X L))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "test.shen");
  assert.ok(
    /return l\.includes\(x\);/.test(out),
    `expected member? to translate to .includes; got:\n${out}`
  );
});

// ============================================================================
// Type inference inside fold/scan/map/filter curried lambdas.
// ============================================================================

test("translateDefineExpr: scanl element type propagates into inner lambda (accessor dispatch)", () => {
  // Spec: fold over a (list transaction) with a curried `(lambda Acc (lambda Tx …))`.
  // The inner lambda's Tx must resolve to `transaction`, so `(amount Tx)` —
  // which is an accessor on the transaction composite — emits `tx.amount()`,
  // not the user-function-call fallback `amount(tx)`.
  const src = `(datatype account-id
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

(define total-out
  {(list transaction) --> number}
  Txs -> (foldr (lambda Tx (lambda Acc (+ (val (amount Tx)) Acc)))
                0
                Txs))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "test.shen");
  assert.ok(
    /tx\.amount\(\)/.test(out),
    `foldr element type should dispatch (amount Tx) to tx.amount(); got:\n${out}`
  );
  assert.ok(
    !/\bamount\(tx\)\b/.test(out),
    `should NOT fall back to amount(tx); got:\n${out}`
  );
});

test("translateDefineExpr: map element type propagates into lambda", () => {
  const src = `(datatype account-id
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

(define amounts-of
  {(list transaction) --> (list amount)}
  Txs -> (map (lambda Tx (amount Tx)) Txs))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "test.shen");
  assert.ok(
    /txs\.map\(\(x: any\) => \(\(tx: any\) => tx\.amount\(\)\)\(x\)\)/.test(out),
    `map's element type should propagate into the lambda so (amount Tx) → tx.amount(); got:\n${out}`
  );
});

test("translateDefineExpr: foldl/scanl accumulator stays untyped while element resolves", () => {
  // Regression check: scanl and foldl curry as f(acc)(x). Only the INNER
  // lambda (x) should get the element type; the outer lambda (acc) stays
  // untyped since we can't reliably infer it from spec context.
  const src = `(datatype item
  N : number;
  =============
  N : item;)

(define scan-doubles
  {(list item) --> (list number)}
  Xs -> (scanl (lambda Acc (lambda I (+ Acc (val I)))) 0 Xs))`;
  const spec = parseSpecString(src);
  const st = new SymbolTable();
  st.build(spec.datatypes);
  st.registerDefines(spec.defines);
  const out = generateTs(spec.datatypes, st, "test.shen");
  // `(val I)` on a primitive-wrapped `item` emits `__val(i)` — the __val
  // helper handles either side, so we just verify the lambda structure is
  // there and doesn't fall through to user-function dispatch on `val`.
  assert.ok(
    /__scanl\(\(acc: any\) => \(i: any\) => \(acc \+ __val\(i\)\)/.test(out),
    `scanl lambda shape wrong; got:\n${out}`
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
