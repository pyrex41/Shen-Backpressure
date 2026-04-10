// Package verify — sample generator for the TS verification harness.
//
// Ported from shen-derive/verify/samples.go. Produces deterministic
// boundary-value samples for Shen types, optionally extended with
// random draws from a seeded PRNG. Each sample carries both a runtime
// `Value` for the spec evaluator and a TS source expression for the
// emitted test file.

import {
  Env,
  boolVal,
  evalExpr,
  floatVal,
  intVal,
  listVal,
  stringVal,
  type Value,
} from "../core/eval.ts";
import { parseSexpr } from "../core/sexpr-parse.ts";
import {
  elemType,
  tsType,
  type TypeEntry,
  type TypeTable,
} from "../specfile/typetable.ts";

// --- Seeded PRNG (Mulberry32) ---
//
// We do NOT aim to match Go's math/rand byte-for-byte. The only goal
// is reproducibility within the TS harness for `--seed N`.

export class SeededRng {
  private state: number;

  constructor(seed: number) {
    // Coerce to unsigned 32-bit.
    this.state = seed >>> 0;
  }

  /** Returns a float in [0, 1). */
  nextFloat(): number {
    this.state = (this.state + 0x6d2b79f5) >>> 0;
    let t = this.state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }

  /** Returns an integer in [min, max] inclusive. */
  nextInt(min: number, max: number): number {
    if (max < min) throw new Error(`nextInt: max (${max}) < min (${min})`);
    const span = max - min + 1;
    return min + Math.floor(this.nextFloat() * span);
  }
}

// --- Public types ---

export type Sample = {
  /** Runtime value used by the spec evaluator. */
  value: Value;
  /** TS source expression used in the generated test file. */
  tsExpr: string;
};

export type SampleCtx = {
  tt: TypeTable;
  /** null → deterministic, boundary values only. */
  rand: SeededRng | null;
  /** Number of extra random draws for primitive number/string types. */
  randomDraws: number;
};

// --- Boundary pools. MUST match Go exactly. ---
//
// Number pool mixes ints and floats — 2.5 is the load-bearing
// fractional value that catches int-vs-float truncation bugs. The
// int/float tagging survives through to the runtime `Value`, so
// constrained-wrapper filters see the correct `kind`.

const NUMBER_RAW: ReadonlyArray<
  { kind: "int" | "float"; val: number; expr: string }
> = [
  { kind: "int", val: 0, expr: "0" },
  { kind: "int", val: 1, expr: "1" },
  { kind: "int", val: -1, expr: "-1" },
  { kind: "int", val: 5, expr: "5" },
  { kind: "float", val: 2.5, expr: "2.5" },
  { kind: "int", val: 100, expr: "100" },
];

const STRING_POOL: ReadonlyArray<string> = ["", "alice", "bob"];
const BOOL_POOL: ReadonlyArray<boolean> = [true, false];

// --- Entry point ---

export function genSamples(ctx: SampleCtx, shenType: string): Sample[] {
  const t = shenType.trim();

  // list types: (list T)
  const elem = elemType(t);
  if (elem !== "") {
    const elemSamples = genSamples(ctx, elem);
    return listSamples(ctx.tt, elem, elemSamples);
  }

  // primitives
  switch (t) {
    case "number": {
      const out = numberSamples();
      if (ctx.rand !== null && ctx.randomDraws > 0) {
        out.push(...randomNumberSamples(ctx.rand, ctx.randomDraws));
      }
      return out;
    }
    case "string":
    case "symbol": {
      const out: Sample[] = STRING_POOL.map((s) => ({
        value: stringVal(s),
        tsExpr: JSON.stringify(s),
      }));
      if (ctx.rand !== null && ctx.randomDraws > 0) {
        out.push(...randomStringSamples(ctx.rand, ctx.randomDraws));
      }
      return out;
    }
    case "boolean":
      return BOOL_POOL.map((b) => ({
        value: boolVal(b),
        tsExpr: b ? "true" : "false",
      }));
  }

  // declared type: look up in the TypeTable.
  const entry = ctx.tt.get(t);
  if (entry === undefined) {
    throw new Error(`unknown Shen type "${t}"`);
  }

  switch (entry.category) {
    case "wrapper":
    case "constrained":
      return wrapperSamples(ctx, entry);
    case "composite":
    case "guarded":
      return compositeSamples(ctx, entry);
    case "alias":
      throw new Error(`alias type "${t}" not supported in samples yet`);
    case "sumtype":
      throw new Error(`sum type "${t}" sampling not supported yet`);
    default: {
      // Exhaustive check.
      const _never: never = entry.category;
      throw new Error(`unhandled category for "${t}": ${String(_never)}`);
    }
  }
}

// --- Primitive number pool ---

function numberSamples(): Sample[] {
  return NUMBER_RAW.map((n) => ({
    value: n.kind === "int" ? intVal(n.val) : floatVal(n.val),
    tsExpr: n.expr,
  }));
}

// --- Wrapper / constrained types ---
//
// Sample the underlying primitive, drop any candidate that fails the
// entry's verified predicates, and wrap the tsExpr in a
// `shenguard.mustXxx(raw)` helper call. The evaluator value passes
// through unchanged — wrapping is invisible at eval time.

function wrapperSamples(ctx: SampleCtx, entry: TypeEntry): Sample[] {
  let primSamples = genSamples(ctx, entry.shenPrim);
  if (entry.category === "constrained") {
    primSamples = filterByConstraints(entry, primSamples);
  }
  const helper = `${entry.importAlias || "shenguard"}.must${entry.tsName}`;
  return primSamples.map((ps) => ({
    value: ps.value,
    tsExpr: `${helper}(${ps.tsExpr})`,
  }));
}

function filterByConstraints(
  entry: TypeEntry,
  candidates: Sample[],
): Sample[] {
  if (entry.verified.length === 0 || entry.varName === "") {
    return candidates;
  }
  // Parse the verified predicates up front.
  const preds = [];
  for (const raw of entry.verified) {
    try {
      preds.push(parseSexpr(raw));
    } catch {
      // Unparseable predicate — be safe: drop everything.
      return [];
    }
  }

  const out: Sample[] = [];
  for (const s of candidates) {
    const env = Env.empty().extend(entry.varName, s.value);
    let ok = true;
    for (const p of preds) {
      try {
        const result = evalExpr(env, p);
        if (result.kind !== "bool" || !result.val) {
          ok = false;
          break;
        }
      } catch {
        ok = false;
        break;
      }
    }
    if (ok) out.push(s);
  }
  return out;
}

// --- Composite / guarded types ---
//
// One variation per field-sample index up to the longest field sample
// list, cycling shorter fields. Ensures "tricky" samples at higher
// indices (e.g. a fractional number) always land in at least one
// composite. Matches the Go implementation.

function compositeSamples(ctx: SampleCtx, entry: TypeEntry): Sample[] {
  if (entry.fields.length === 0) {
    throw new Error(`composite "${entry.shenName}" has no fields`);
  }

  const fieldSamples: Sample[][] = entry.fields.map((f) => {
    const fs = genSamples(ctx, f.typeName);
    if (fs.length === 0) {
      throw new Error(`field ${f.shenName}: no samples`);
    }
    return fs;
  });

  let maxVariations = 0;
  for (const fs of fieldSamples) {
    if (fs.length > maxVariations) maxVariations = fs.length;
  }

  const helper = `${entry.importAlias || "shenguard"}.must${entry.tsName}`;
  const out: Sample[] = [];
  for (let v = 0; v < maxVariations; v++) {
    const values: Value[] = new Array(entry.fields.length);
    const exprs: string[] = new Array(entry.fields.length);
    for (let i = 0; i < fieldSamples.length; i++) {
      const fs = fieldSamples[i]!;
      const idx = v % fs.length;
      values[i] = fs[idx]!.value;
      exprs[i] = fs[idx]!.tsExpr;
    }
    out.push({
      value: listVal(...values),
      tsExpr: `${helper}(${exprs.join(", ")})`,
    });
  }
  return out;
}

// --- List types ---
//
// Empty list + one singleton per elem sample (capped at 6) + one
// 3-element mix. "One singleton per elem sample" is the critical
// invariant that keeps tricky elem samples (e.g. a fractional number
// at index 4) from being silently excluded from every list.

function listSamples(
  tt: TypeTable,
  elemShenType: string,
  elemSamples: Sample[],
): Sample[] {
  const tsElemType = tsType(tt, elemShenType);
  const out: Sample[] = [
    { value: listVal(), tsExpr: `[] as ${tsElemType}[]` },
  ];

  if (elemSamples.length === 0) return out;

  const maxSingletons = 6;
  const n = Math.min(elemSamples.length, maxSingletons);
  for (let i = 0; i < n; i++) {
    const e = elemSamples[i]!;
    out.push({
      value: listVal(e.value),
      tsExpr: `[${e.tsExpr}]`,
    });
  }

  if (elemSamples.length >= 2) {
    const m = Math.min(elemSamples.length, 3);
    const vals: Value[] = [];
    const exprs: string[] = [];
    for (let i = 0; i < m; i++) {
      vals.push(elemSamples[i]!.value);
      exprs.push(elemSamples[i]!.tsExpr);
    }
    out.push({
      value: listVal(...vals),
      tsExpr: `[${exprs.join(", ")}]`,
    });
  }

  return out;
}

// --- Random draws (only invoked when ctx.rand !== null) ---
//
// Half ints, half 2-decimal-place floats in [-1000, 1000]. The
// fractional draws deliberately catch int-vs-float truncation bugs.

export function randomNumberSamples(rnd: SeededRng, n: number): Sample[] {
  const out: Sample[] = [];
  for (let i = 0; i < n; i++) {
    if (i % 2 === 0) {
      const v = rnd.nextInt(-1000, 1000);
      out.push({ value: intVal(v), tsExpr: v.toString() });
    } else {
      const cents = rnd.nextInt(-100000, 100000);
      const v = cents / 100;
      out.push({ value: floatVal(v), tsExpr: formatFloat(v) });
    }
  }
  return out;
}

export function randomStringSamples(rnd: SeededRng, n: number): Sample[] {
  const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789";
  const out: Sample[] = [];
  for (let i = 0; i < n; i++) {
    const length = 1 + rnd.nextInt(0, 7); // 1..8 inclusive
    let s = "";
    for (let j = 0; j < length; j++) {
      s += alphabet[rnd.nextInt(0, alphabet.length - 1)]!;
    }
    out.push({ value: stringVal(s), tsExpr: JSON.stringify(s) });
  }
  return out;
}

// formatFloat prints a float with no trailing zeros, matching Go's
// strconv.FormatFloat(v, 'f', -1, 64) closely enough for the literals
// we emit. Integer-valued floats still render as e.g. "3" — the
// evaluator sees the float kind via `value`, not via `tsExpr`.
function formatFloat(v: number): string {
  if (Number.isInteger(v)) return v.toString();
  return v.toString();
}
