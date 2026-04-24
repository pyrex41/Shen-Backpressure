// Tests for cmd/shen-derive-ts/specfile/parse.ts.
// Ported 1:1 from shen-derive/specfile/parse_test.go where possible.

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import {
  parseFile,
  parseDefine,
  parseDatatype,
  parseTypeSig,
  parseDefineClauses,
  splitPatterns,
  extractBalancedParen,
  extractBlocks,
  stripShenComments,
  deriveParamNames,
  findDatatype,
  findDefine,
} from "./parse.ts";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "../../..");

// --- Ported from TestParsePaymentSpec ---

test("parseFile: payment spec datatypes + processable define", () => {
  const path = resolve(repoRoot, "examples/payment/specs/core.shen");
  const src = readFileSync(path, "utf-8");
  const sf = parseFile(src);

  assert.equal(sf.datatypes.length, 6, "datatype count");

  const wantNames = [
    "account-id",
    "amount",
    "transaction",
    "balance-invariant",
    "account-state",
    "safe-transfer",
  ];
  for (let i = 0; i < wantNames.length; i++) {
    assert.equal(sf.datatypes[i].name, wantNames[i], `datatype[${i}] name`);
  }

  // Spot-check amount.
  const amount = findDatatype(sf, "amount");
  assert.ok(amount, "amount datatype present");
  assert.equal(amount!.rules.length, 1);
  const r = amount!.rules[0];
  assert.equal(r.premises.length, 1);
  assert.equal(r.premises[0].typeName, "number");
  assert.equal(r.verified.length, 1);
  assert.equal(r.verified[0].expr, "(>= X 0)");
  // amount conclusion is a simple variable, not a tuple.
  assert.equal(r.conclusion.typeName, "amount");
  assert.equal(r.conclusion.fields.length, 0);
  assert.notEqual(r.conclusion.varName, "");

  // Spot-check transaction.
  const tx = findDatatype(sf, "transaction");
  assert.ok(tx, "transaction datatype present");
  assert.equal(tx!.rules.length, 1);
  const txr = tx!.rules[0];
  assert.equal(txr.conclusion.varName, "", "tuple conclusion has empty varName");
  assert.deepEqual(txr.conclusion.fields, ["Amount", "From", "To"]);

  // processable define.
  assert.equal(sf.defines.length, 1);
  const proc = findDefine(sf, "processable");
  assert.ok(proc, "processable found");
  assert.deepEqual(proc!.typeSig.paramTypes, ["amount", "(list transaction)"]);
  assert.equal(proc!.typeSig.returnType, "boolean");
  assert.deepEqual(proc!.paramNames, ["B0", "Txs"]);
});

// --- Ported from TestParseTypeSig ---

test("parseTypeSig: single arrow", () => {
  const sig = parseTypeSig("{number --> number}");
  assert.deepEqual(sig.paramTypes, ["number"]);
  assert.equal(sig.returnType, "number");
});

test("parseTypeSig: parameterized list type", () => {
  const sig = parseTypeSig("{amount --> (list transaction) --> boolean}");
  assert.deepEqual(sig.paramTypes, ["amount", "(list transaction)"]);
  assert.equal(sig.returnType, "boolean");
});

test("parseTypeSig: three params", () => {
  const sig = parseTypeSig("{string --> string --> number --> boolean}");
  assert.deepEqual(sig.paramTypes, ["string", "string", "number"]);
  assert.equal(sig.returnType, "boolean");
});

// --- Ported from TestParseDefine ---

test("parseDefine: processable (single-clause typed define)", () => {
  const block = `(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx))))) (val B0) Txs)))`;

  const def = parseDefine(block);
  assert.ok(def);
  assert.equal(def!.name, "processable");
  assert.deepEqual(def!.typeSig.paramTypes, ["amount", "(list transaction)"]);
  assert.equal(def!.typeSig.returnType, "boolean");
  assert.deepEqual(def!.paramNames, ["B0", "Txs"]);
  assert.equal(def!.clauses.length, 1);
  assert.ok(def!.clauses[0].body.length > 0);
  assert.equal(def!.clauses[0].guard, null);
  assert.equal(def!.clauses[0].patterns.length, 2);
});

// --- Ported from TestParseDefineMultiClauseWithGuards ---

test("parseDefine: pair-in-list? multi-clause with where guards", () => {
  const block = `(define pair-in-list?
  _ _ [] -> false
  A B [[X Y] | Rest] -> true  where (and (= A X) (= B Y))
  A B [[X Y] | Rest] -> true  where (and (= A Y) (= B X))
  A B [_ | Rest] -> (pair-in-list? A B Rest))`;

  const def = parseDefine(block);
  assert.ok(def);
  assert.equal(def!.name, "pair-in-list?");
  assert.equal(def!.clauses.length, 4);

  // Clause 0: `_ _ [] -> false`, no guard.
  assert.equal(def!.clauses[0].patterns.length, 3);
  assert.equal(def!.clauses[0].guard, null);
  assert.equal(def!.clauses[0].body, "false");

  // Clauses 1 and 2 have where guards.
  assert.ok(def!.clauses[1].guard);
  assert.ok(def!.clauses[1].guard!.includes("(= A X)"));
  assert.ok(def!.clauses[2].guard);
  assert.ok(def!.clauses[2].guard!.includes("(= A Y)"));

  // Clause 3 has a recursive call body, no guard.
  assert.equal(def!.clauses[3].guard, null);
  assert.equal(def!.clauses[3].body, "(pair-in-list? A B Rest)");

  // No type signature.
  assert.equal(def!.typeSig.paramTypes.length, 0);

  // Arity = first clause pattern count.
  assert.equal(def!.clauses[0].patterns.length, 3);

  // paramNames for wildcards should be p0/p1/p2.
  assert.deepEqual(def!.paramNames, ["p0", "p1", "p2"]);
});

// --- Ported from TestStripShenComments ---

test("stripShenComments: block comments removed", () => {
  const input = `\\* line comment *\\
(datatype foo
  X : number;
  ============
  X : foo;)
\\* another *\\`;
  const out = stripShenComments(input);
  assert.ok(!out.includes("\\*"), "opening marker removed");
  assert.ok(!out.includes("*\\"), "closing marker removed");
  assert.ok(out.includes("datatype foo"));
  assert.ok(out.includes("X : number"));
});

// --- Additional unit tests for the ported helpers ---

test("extractBlocks: finds two datatype blocks", () => {
  const src = `(datatype a
  X : number;
  ====
  X : a;)

(datatype b
  Y : string;
  ====
  Y : b;)`;
  const blocks = extractBlocks(src, "(datatype ");
  assert.equal(blocks.length, 2);
  assert.ok(blocks[0].startsWith("(datatype a"));
  assert.ok(blocks[1].startsWith("(datatype b"));
});

test("splitPatterns: nested brackets stay as one token", () => {
  assert.deepEqual(splitPatterns("_ _ []"), ["_", "_", "[]"]);
  assert.deepEqual(splitPatterns("A B [[X Y] | Rest]"), ["A", "B", "[[X Y] | Rest]"]);
  assert.deepEqual(splitPatterns("Drug [Med | Meds] Pairs"), [
    "Drug",
    "[Med | Meds]",
    "Pairs",
  ]);
});

test("extractBalancedParen: handles nesting", () => {
  const [expr, end] = extractBalancedParen("(and (= A X) (= B Y)) rest");
  assert.equal(expr, "(and (= A X) (= B Y))");
  assert.equal(end, 21);
});

test("extractBalancedParen: returns ['', 0] for non-paren", () => {
  assert.deepEqual(extractBalancedParen("foo"), ["", 0]);
});

test("deriveParamNames: uppercase symbols kept, wildcards become pN", () => {
  assert.deepEqual(deriveParamNames(["B0", "Txs"]), ["B0", "Txs"]);
  assert.deepEqual(deriveParamNames(["_", "_", "[]"]), ["p0", "p1", "p2"]);
  assert.deepEqual(deriveParamNames(["A", "B", "[_ | Rest]"]), ["A", "B", "p2"]);
});

test("parseDatatype: returns null for blocks with no rules", () => {
  const dt = parseDatatype("(datatype empty\n)");
  assert.equal(dt, null);
});

test("parseDefineClauses: bare multi-clause without type sig", () => {
  const clauses = parseDefineClauses("_ [] -> true\n  X [H | T] -> (f X T)");
  assert.equal(clauses.length, 2);
  assert.deepEqual(clauses[0].patterns, ["_", "[]"]);
  assert.equal(clauses[0].body, "true");
  assert.deepEqual(clauses[1].patterns, ["X", "[H | T]"]);
  assert.equal(clauses[1].body, "(f X T)");
});

// --- Integration: dosage-calculator ---

// The dosage-calculator example was removed in the demo-readiness cleanup
// (commit eb3be68). Skipping until a replacement integration fixture lands.
test.skip("integration: dosage-calculator core.shen parses with expected defines", () => {
  const path = resolve(repoRoot, "examples/dosage-calculator/specs/core.shen");
  const src = readFileSync(path, "utf-8");
  const sf = parseFile(src);

  const pairIn = findDefine(sf, "pair-in-list?");
  assert.ok(pairIn, "pair-in-list? found");
  assert.equal(pairIn!.clauses.length, 4, "pair-in-list? clause count");

  const drugClear = findDefine(sf, "drug-clear-of-list?");
  assert.ok(drugClear, "drug-clear-of-list? found");
  assert.equal(drugClear!.clauses.length, 3, "drug-clear-of-list? clause count");

  // Spot-check: guards present on clauses that have `where`.
  assert.ok(pairIn!.clauses[1].guard, "pair-in-list? clause 1 has guard");
  assert.ok(pairIn!.clauses[2].guard, "pair-in-list? clause 2 has guard");
  assert.ok(drugClear!.clauses[1].guard, "drug-clear-of-list? clause 1 has guard");

  // Contraindication datatype should parse as a composite.
  const contra = findDatatype(sf, "contraindication");
  assert.ok(contra, "contraindication datatype present");
  assert.equal(contra!.rules.length, 1);
  assert.deepEqual(contra!.rules[0].conclusion.fields, ["DrugA", "DrugB"]);
});
