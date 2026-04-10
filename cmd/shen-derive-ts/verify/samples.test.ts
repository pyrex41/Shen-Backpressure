// Tests for the sample generator. Ported (case-for-case) from
// shen-derive/verify/harness_test.go's sampling-related checks plus a
// few TS-specific ones (int/float tagging, PRNG reproducibility).

import { test } from "node:test";
import assert from "node:assert/strict";

import type { Datatype } from "../specfile/parse.ts";
import { buildTypeTable } from "../specfile/typetable.ts";
import { genSamples, SeededRng, type SampleCtx } from "./samples.ts";

// --- Fixture datatypes ---

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
];

function makeCtx(rand: SeededRng | null = null, draws = 0): SampleCtx {
  const tt = buildTypeTable(paymentDatatypes, "./shenguard.ts", "shenguard");
  return { tt, rand, randomDraws: draws };
}

// --- Primitive pools ---

test("primitive number samples preserve int/float tagging", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "number");
  assert.equal(s.length, 6);

  const kinds = s.map((x) => x.value.kind);
  // Order must match NUMBER_POOL: 0,1,-1,5 (int), 2.5 (float), 100 (int).
  assert.deepEqual(kinds, ["int", "int", "int", "int", "float", "int"]);

  const exprs = s.map((x) => x.tsExpr);
  assert.deepEqual(exprs, ["0", "1", "-1", "5", "2.5", "100"]);

  // Numeric values.
  const vals = s.map((x) =>
    x.value.kind === "int" || x.value.kind === "float" ? x.value.val : NaN
  );
  assert.deepEqual(vals, [0, 1, -1, 5, 2.5, 100]);
});

test("primitive string samples cover empty / alice / bob", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "string");
  assert.equal(s.length, 3);
  assert.deepEqual(
    s.map((x) => x.tsExpr),
    [`""`, `"alice"`, `"bob"`],
  );
  for (const x of s) assert.equal(x.value.kind, "string");
});

test("primitive boolean samples cover true and false", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "boolean");
  assert.equal(s.length, 2);
  assert.deepEqual(s.map((x) => x.tsExpr), ["true", "false"]);
  assert.deepEqual(
    s.map((x) => (x.value.kind === "bool" ? x.value.val : null)),
    [true, false],
  );
});

// --- Lists ---

test("(list number) produces empty + one singleton per elem + 3-elem mix", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "(list number)");

  // 6 elem samples → 1 empty + 6 singletons + 1 triple = 8.
  assert.equal(s.length, 1 + 6 + 1);

  // First sample is the empty list.
  assert.equal(s[0]!.value.kind, "list");
  assert.equal(
    s[0]!.value.kind === "list" ? s[0]!.value.elems.length : -1,
    0,
  );

  // Next six are singletons, one per NUMBER_POOL elem, IN ORDER.
  const expectedExprs = ["0", "1", "-1", "5", "2.5", "100"];
  for (let i = 0; i < 6; i++) {
    const sample = s[1 + i]!;
    assert.equal(sample.tsExpr, `[${expectedExprs[i]}]`);
    assert.equal(sample.value.kind, "list");
    if (sample.value.kind === "list") {
      assert.equal(sample.value.elems.length, 1);
    }
  }

  // Last is a 3-element mix of the first three elem samples.
  const mix = s[7]!;
  assert.equal(mix.tsExpr, "[0, 1, -1]");
  assert.equal(mix.value.kind, "list");
  if (mix.value.kind === "list") assert.equal(mix.value.elems.length, 3);
});

test("list singleton for fractional elem is not dropped (bug 2 regression guard)", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "(list number)");
  // Singleton "[2.5]" must be present.
  const fracSingleton = s.find((x) => x.tsExpr === "[2.5]");
  assert.ok(fracSingleton, "expected [2.5] singleton in list samples");
  assert.equal(fracSingleton!.value.kind, "list");
  if (fracSingleton!.value.kind === "list") {
    const elem = fracSingleton!.value.elems[0]!;
    assert.equal(elem.kind, "float");
  }
});

// --- Wrapper + constraint filtering ---

test("constrained amount drops negative primitives via verified predicate", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "amount");

  // NUMBER_POOL has one negative (-1); the (>= X 0) predicate drops it.
  assert.equal(s.length, 5);
  for (const x of s) {
    assert.ok(x.value.kind === "int" || x.value.kind === "float");
    const n = (x.value as { val: number }).val;
    assert.ok(n >= 0, `sample ${x.tsExpr} has negative value ${n}`);
  }
  // Fractional 2.5 survives (it's >= 0) — critical for bug 2 demo.
  assert.ok(s.some((x) => x.tsExpr.includes("2.5")));
  // All tsExprs are wrapped in the helper call.
  for (const x of s) {
    assert.ok(
      x.tsExpr.startsWith("shenguard.mustAmount("),
      `expected mustAmount wrapper, got ${x.tsExpr}`,
    );
  }
});

test("wrapper account-id wraps every string primitive", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "account-id");
  assert.equal(s.length, 3);
  for (const x of s) {
    assert.ok(x.tsExpr.startsWith("shenguard.mustAccountId("));
    assert.equal(x.value.kind, "string");
  }
});

// --- Composite ---

test("transaction composite produces one variation per longest-field index", () => {
  const ctx = makeCtx();
  const s = genSamples(ctx, "transaction");

  // Fields: amount (5 after filter), account-id (3), account-id (3).
  // Longest = 5 → 5 variations.
  assert.equal(s.length, 5);
  for (const x of s) {
    assert.ok(x.tsExpr.startsWith("shenguard.mustTransaction("));
    assert.ok(x.tsExpr.includes("shenguard.mustAmount("));
    assert.ok(x.tsExpr.includes("shenguard.mustAccountId("));
    // Value is a list of three field values.
    assert.equal(x.value.kind, "list");
    if (x.value.kind === "list") {
      assert.equal(x.value.elems.length, 3);
    }
  }
});

// --- Seeded PRNG reproducibility ---

test("SeededRng produces reproducible sequences for the same seed", () => {
  const a = new SeededRng(42);
  const b = new SeededRng(42);
  const seqA: number[] = [];
  const seqB: number[] = [];
  for (let i = 0; i < 20; i++) {
    seqA.push(a.nextInt(-1000, 1000));
    seqB.push(b.nextInt(-1000, 1000));
  }
  assert.deepEqual(seqA, seqB);

  const c = new SeededRng(43);
  const seqC: number[] = [];
  for (let i = 0; i < 20; i++) seqC.push(c.nextInt(-1000, 1000));
  assert.notDeepEqual(seqA, seqC);
});

test("random draws extend number samples and stay in range", () => {
  const ctx = makeCtx(new SeededRng(7), 10);
  const s = genSamples(ctx, "number");
  // 6 boundary + 10 random = 16.
  assert.equal(s.length, 16);
  for (let i = 6; i < 16; i++) {
    const v = s[i]!.value;
    assert.ok(v.kind === "int" || v.kind === "float");
    const n = v.val;
    assert.ok(n >= -1000 && n <= 1000, `random draw ${n} out of range`);
  }
  // Half ints, half floats in the random portion.
  const intCount = s.slice(6).filter((x) => x.value.kind === "int").length;
  const floatCount = s.slice(6).filter((x) => x.value.kind === "float").length;
  assert.equal(intCount, 5);
  assert.equal(floatCount, 5);
});

test("random string draws are lowercase alnum of length 1..8", () => {
  const ctx = makeCtx(new SeededRng(11), 5);
  const s = genSamples(ctx, "string");
  assert.equal(s.length, 3 + 5);
  for (let i = 3; i < s.length; i++) {
    const v = s[i]!.value;
    assert.equal(v.kind, "string");
    if (v.kind === "string") {
      assert.ok(v.val.length >= 1 && v.val.length <= 8);
      assert.ok(/^[a-z0-9]+$/.test(v.val), `bad random string ${v.val}`);
    }
  }
});

// --- Unknown type ---

test("unknown Shen type throws", () => {
  const ctx = makeCtx();
  assert.throws(() => genSamples(ctx, "nonesuch"), /unknown Shen type/);
});
