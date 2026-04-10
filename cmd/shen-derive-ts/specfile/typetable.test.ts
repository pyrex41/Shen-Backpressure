// Tests for the Shen→TS type classifier. Datatype objects are constructed
// inline so these tests don't depend on parse.ts being able to parse real
// .shen files — only on the shared type contract.

import { test } from "node:test";
import assert from "node:assert/strict";

import type { Datatype } from "./parse.ts";
import {
  buildTypeTable,
  tsType,
  elemType,
  type TypeEntry,
} from "./typetable.ts";

// --- Inline fixture mirroring examples/payment/specs/core.shen ---
//
// The payment spec contains:
//   - account-id : wrapper around string
//   - amount     : constrained number (>= 0)
//   - transaction: composite [Amount From To]
//   - balance-checked : guarded composite (verified invariant)
//
// These fixtures construct the post-parse shape directly.

const paymentDatatypes: Datatype[] = [
  {
    name: "account-id",
    rules: [
      {
        premises: [{ varName: "X", typeName: "string" }],
        verified: [],
        conclusion: { varName: "X", typeName: "account-id", fields: [] },
      },
    ],
  },
  {
    name: "amount",
    rules: [
      {
        premises: [{ varName: "X", typeName: "number" }],
        verified: [{ raw: "(>= X 0)", varName: "", expr: "(>= X 0)" }],
        conclusion: { varName: "X", typeName: "amount", fields: [] },
      },
    ],
  },
  {
    name: "transaction",
    rules: [
      {
        premises: [
          { varName: "Amount", typeName: "amount" },
          { varName: "From", typeName: "account-id" },
          { varName: "To", typeName: "account-id" },
        ],
        verified: [],
        conclusion: {
          varName: "",
          typeName: "transaction",
          fields: ["Amount", "From", "To"],
        },
      },
    ],
  },
  {
    name: "balance-invariant",
    rules: [
      {
        premises: [
          { varName: "Before", typeName: "number" },
          { varName: "Txs", typeName: "(list transaction)" },
          { varName: "After", typeName: "number" },
        ],
        verified: [
          {
            raw: "(= After (+ Before (sum Txs)))",
            varName: "",
            expr: "(= After (+ Before (sum Txs)))",
          },
        ],
        conclusion: {
          varName: "",
          typeName: "balance-checked",
          fields: ["Before", "Txs", "After"],
        },
      },
    ],
  },
];

function get(tt: Map<string, TypeEntry>, name: string): TypeEntry {
  const e = tt.get(name);
  assert.ok(e !== undefined, `expected entry for ${name}`);
  return e;
}

// --- buildTypeTable classification tests ---

test("account-id is classified as a wrapper around string", () => {
  const tt = buildTypeTable(paymentDatatypes, "./shenguard.ts", "shenguard");
  const e = get(tt, "account-id");
  assert.equal(e.category, "wrapper");
  assert.equal(e.tsName, "AccountId");
  assert.equal(e.shenPrim, "string");
  assert.equal(e.tsPrimType, "string");
  assert.equal(e.varName, "X");
  assert.deepEqual(e.verified, []);
  assert.deepEqual(e.fields, []);
  assert.equal(e.importPath, "./shenguard.ts");
  assert.equal(e.importAlias, "shenguard");
});

test("amount is classified as constrained with captured predicate", () => {
  const tt = buildTypeTable(paymentDatatypes, "./shenguard.ts", "shenguard");
  const e = get(tt, "amount");
  assert.equal(e.category, "constrained");
  assert.equal(e.tsName, "Amount");
  assert.equal(e.shenPrim, "number");
  assert.equal(e.tsPrimType, "number");
  assert.equal(e.varName, "X");
  assert.deepEqual(e.verified, ["(>= X 0)"]);
});

test("transaction is classified as composite with three fields", () => {
  const tt = buildTypeTable(paymentDatatypes, "./shenguard.ts", "shenguard");
  const e = get(tt, "transaction");
  assert.equal(e.category, "composite");
  assert.equal(e.tsName, "Transaction");
  assert.equal(e.fields.length, 3);
  assert.deepEqual(
    e.fields.map((f) => ({
      name: f.name,
      shenName: f.shenName,
      typeName: f.typeName,
      tsType: f.tsType,
    })),
    [
      { name: "amount", shenName: "Amount", typeName: "amount", tsType: "Amount" },
      { name: "from", shenName: "From", typeName: "account-id", tsType: "AccountId" },
      { name: "to", shenName: "To", typeName: "account-id", tsType: "AccountId" },
    ],
  );
});

test("balance-checked is classified as guarded with verified predicate", () => {
  const tt = buildTypeTable(paymentDatatypes, "./shenguard.ts", "shenguard");
  const e = get(tt, "balance-checked");
  assert.equal(e.category, "guarded");
  assert.equal(e.tsName, "BalanceChecked");
  assert.equal(e.verified.length, 1);
  assert.match(e.verified[0], /^\(= After/);
  assert.equal(e.fields.length, 3);
  // Field with a list type should resolve its tsType to "Transaction[]".
  const txField = e.fields.find((f) => f.shenName === "Txs");
  assert.ok(txField !== undefined);
  assert.equal(txField.tsType, "Transaction[]");
});

test("unknown premise references resolve to 'unknown' shen type", () => {
  const dts: Datatype[] = [
    {
      name: "broken",
      rules: [
        {
          premises: [{ varName: "A", typeName: "number" }],
          verified: [],
          conclusion: { varName: "", typeName: "broken", fields: ["A", "Missing"] },
        },
      ],
    },
  ];
  const tt = buildTypeTable(dts, "", "");
  const e = get(tt, "broken");
  assert.equal(e.fields[1].typeName, "unknown");
});

test("sum types produce a synthetic sumtype entry", () => {
  const dts: Datatype[] = [
    {
      name: "circle",
      rules: [
        {
          premises: [{ varName: "R", typeName: "number" }],
          verified: [],
          // Conclusion type "shape" is shared with the square rule below →
          // this becomes a sum variant named "circle".
          conclusion: { varName: "", typeName: "shape", fields: ["R"] },
        },
      ],
    },
    {
      name: "square",
      rules: [
        {
          premises: [{ varName: "S", typeName: "number" }],
          verified: [],
          conclusion: { varName: "", typeName: "shape", fields: ["S"] },
        },
      ],
    },
  ];
  const tt = buildTypeTable(dts, "", "");
  const shape = get(tt, "shape");
  assert.equal(shape.category, "sumtype");
  assert.equal(shape.tsName, "Shape");
  const circle = get(tt, "circle");
  assert.equal(circle.category, "composite");
  assert.equal(circle.fields.length, 1);
});

// --- tsType tests ---

test("tsType maps primitives to TS built-ins", () => {
  const tt = buildTypeTable([], "", "");
  assert.equal(tsType(tt, "number"), "number");
  assert.equal(tsType(tt, "string"), "string");
  assert.equal(tsType(tt, "boolean"), "boolean");
  assert.equal(tsType(tt, "symbol"), "string");
});

test("tsType handles nested list types", () => {
  const tt = buildTypeTable(paymentDatatypes, "", "");
  assert.equal(tsType(tt, "(list transaction)"), "Transaction[]");
  assert.equal(tsType(tt, "(list (list number))"), "number[][]");
  assert.equal(tsType(tt, "(list amount)"), "Amount[]");
});

test("tsType resolves declared types to their PascalCase class name", () => {
  const tt = buildTypeTable(paymentDatatypes, "", "");
  assert.equal(tsType(tt, "amount"), "Amount");
  assert.equal(tsType(tt, "account-id"), "AccountId");
  assert.equal(tsType(tt, "transaction"), "Transaction");
});

test("tsType falls back to PascalCase for unknown types", () => {
  const tt = buildTypeTable([], "", "");
  assert.equal(tsType(tt, "unknown-thing"), "UnknownThing");
});

// --- elemType tests ---

test("elemType extracts element type from (list T)", () => {
  assert.equal(elemType("(list int)"), "int");
  assert.equal(elemType("(list Foo)"), "Foo");
  assert.equal(elemType("(list (list number))"), "(list number)");
});

test("elemType returns empty string for non-list types", () => {
  assert.equal(elemType("number"), "");
  assert.equal(elemType("amount"), "");
  assert.equal(elemType(""), "");
});
