// Verification harness for shen-derive-ts. Ported from
// shen-derive/verify/harness.go.
//
// Flow: sample each parameter type, cartesian-product-cap to
// cfg.maxCases, evaluate the Shen spec on each combo via the core
// evaluator (with a base env that carries field accessors + all
// defines for mutual recursion), and emit a node:test file that
// compares the impl's output to the expected value for each case.

import {
  Env,
  builtinFn,
  evalExpr,
  type Value,
} from "../core/eval.ts";
import { match } from "../core/match.ts";
import { parseSexpr } from "../core/sexpr-parse.ts";
import type { Clause, Define, SpecFile } from "../specfile/parse.ts";
import {
  elemType,
  type TypeTable,
} from "../specfile/typetable.ts";
import {
  genSamples,
  SeededRng,
  type Sample,
  type SampleCtx,
} from "./samples.ts";

// --- Public types ---

export type HarnessConfig = {
  spec: SpecFile;
  tt: TypeTable;
  /** Every define from the spec (for mutual recursion in the eval env). */
  allDefines: Define[];
  /** The define to be verified (must exist in spec.defines). */
  funcName: string;
  /** Relative TS path for the implementation module, e.g. "./processable". */
  implModule: string;
  /** Exported TS function name, e.g. "Processable". */
  implFunc: string;
  /** Shengen-ts guards module path. */
  importPath: string;
  /** Shengen-ts import alias (default "shenguard"). */
  importAlias: string;
  maxCases: number;
  seed: number;
  randomDraws: number;
};

export type Case = {
  args: Value[];
  argsTs: string[];
  expected: Value;
  expectedTs: string;
  label: string;
};

export type Harness = {
  cfg: HarnessConfig;
  cases: Case[];
};

// --- BuildHarness ---

export function buildHarness(cfg: HarnessConfig): Harness {
  // Validate target define.
  const def = cfg.spec.defines.find((d) => d.name === cfg.funcName);
  if (def === undefined) {
    throw new Error(`buildHarness: define ${cfg.funcName} not found in spec`);
  }
  if (def.typeSig.paramTypes.length === 0) {
    throw new Error(
      `spec ${def.name}: verification requires a type signature ` +
        `({...}) — the sampler has no way to generate inputs otherwise`,
    );
  }
  if (def.paramNames.length !== def.typeSig.paramTypes.length) {
    throw new Error(`spec ${def.name}: param count mismatch`);
  }
  if (def.clauses.length === 0) {
    throw new Error(`spec ${def.name}: no clauses`);
  }

  // Sampling context. Zero seed → deterministic boundary values only.
  const ctx: SampleCtx = {
    tt: cfg.tt,
    rand: cfg.seed !== 0 ? new SeededRng(cfg.seed) : null,
    randomDraws: cfg.seed !== 0
      ? (cfg.randomDraws > 0 ? cfg.randomDraws : 8)
      : 0,
  };

  // Generate samples per parameter.
  const paramSamples: Sample[][] = def.typeSig.paramTypes.map((pt, i) => {
    try {
      return genSamples(ctx, pt);
    } catch (e) {
      throw new Error(
        `samples for param ${def.paramNames[i]}: ${(e as Error).message}`,
      );
    }
  });

  const maxCases = cfg.maxCases > 0 ? cfg.maxCases : 50;
  const combos = cartesian(paramSamples, maxCases);

  // Shared base env with field accessors and all defines (envHolder trick).
  const baseEnv = buildBaseEnv(cfg.tt, cfg.allDefines);

  const cases: Case[] = [];
  for (let idx = 0; idx < combos.length; idx++) {
    const combo = combos[idx]!;
    const argVals: Value[] = combo.map((s) => s.value);
    const argsTs: string[] = combo.map((s) => s.tsExpr);
    let expected: Value;
    try {
      expected = evalDefine(def, argVals, baseEnv);
    } catch (e) {
      throw new Error(`case ${idx}: eval spec: ${(e as Error).message}`);
    }
    let expectedTs: string;
    try {
      expectedTs = tsLiteralFor(expected, def.typeSig.returnType, cfg.tt);
    } catch (e) {
      throw new Error(`case ${idx}: literal: ${(e as Error).message}`);
    }
    const label = `case_${idx.toString().padStart(2, "0")}`;
    cases.push({ args: argVals, argsTs, expected, expectedTs, label });
  }

  return { cfg, cases };
}

// --- Cartesian product (odometer loop ported from Go) ---

function cartesian(paramSamples: Sample[][], maxCases: number): Sample[][] {
  if (paramSamples.length === 0) return [];
  const out: Sample[][] = [];
  const indices = new Array<number>(paramSamples.length).fill(0);
  for (;;) {
    const row: Sample[] = new Array(paramSamples.length);
    for (let i = 0; i < indices.length; i++) {
      row[i] = paramSamples[i]![indices[i]!]!;
    }
    out.push(row);
    if (out.length >= maxCases) return out;
    let i = indices.length - 1;
    while (i >= 0) {
      indices[i]!++;
      if (indices[i]! < paramSamples[i]!.length) break;
      indices[i] = 0;
      i--;
    }
    if (i < 0) return out;
  }
}

// --- evalDefine / clause dispatch ---

export function evalDefine(
  def: Define,
  vals: Value[],
  base: Env,
): Value {
  if (def.clauses.length === 0) {
    throw new Error(`define ${def.name}: no clauses`);
  }
  for (let i = 0; i < def.clauses.length; i++) {
    const cl = def.clauses[i]!;
    if (cl.patterns.length !== vals.length) {
      throw new Error(
        `define ${def.name} clause ${i}: ${cl.patterns.length} patterns vs ${vals.length} args`,
      );
    }
    const res = bindClausePatterns(base, cl, vals);
    if (res === null) continue;
    const env = res;
    if (cl.guard !== null) {
      const guardExpr = parseSexpr(cl.guard);
      const g = evalExpr(env, guardExpr);
      if (g.kind !== "bool") {
        throw new Error(
          `define ${def.name} clause ${i}: guard returned ${g.kind}, expected bool`,
        );
      }
      if (!g.val) continue;
    }
    const bodyExpr = parseSexpr(cl.body);
    return evalExpr(env, bodyExpr);
  }
  throw new Error(
    `no matching clause for ${def.name} with args ${vals.map(showValue).join(" ")}`,
  );
}

function bindClausePatterns(
  base: Env,
  clause: Clause,
  vals: Value[],
): Env | null {
  let env = base;
  for (let i = 0; i < clause.patterns.length; i++) {
    const pat = parseSexpr(clause.patterns[i]!);
    const m = match(pat, vals[i]!);
    if (m.kind === "error") throw new Error(m.msg);
    if (m.kind === "miss") return null;
    for (const [name, v] of m.bindings) {
      env = env.extend(name, v);
    }
  }
  return env;
}

// --- buildBaseEnv ---
//
// Binds:
//   - `val` as identity (wrappers are primitives at eval time)
//   - field accessors for every composite/guarded TypeEntry field,
//     projecting the corresponding index out of a ListVal
//   - every define in `defines` as a curried BuiltinFn that, once
//     applied to its full arity, dispatches to evalDefine with the
//     shared base env (via the envHolder trick for mutual recursion)
//
// IMPORTANT: this must NOT bind primitives like +, -, map, foldr — those
// are already known to core/eval.ts via the primitive table. Binding
// them again would shadow the primitive dispatch.
export function buildBaseEnv(
  tt: TypeTable,
  defines: Define[],
): Env {
  let env = Env.empty();

  // val — identity
  env = env.extend(
    "val",
    builtinFn("val", (v) => v),
  );

  // Field accessors. Each composite field becomes a closure that
  // projects the corresponding index out of a ListVal. Register both
  // lower-case and exact (shen) names, skipping duplicates.
  const registered = new Set<string>();
  for (const entry of tt.values()) {
    for (let fi = 0; fi < entry.fields.length; fi++) {
      const f = entry.fields[fi]!;
      const idx = fi;
      const registerName = (name: string): void => {
        if (registered.has(name)) return;
        registered.add(name);
        env = env.extend(
          name,
          builtinFn(name, (v) => {
            if (v.kind !== "list") {
              throw new Error(
                `field accessor "${name}": not a composite value (got ${v.kind})`,
              );
            }
            if (idx < 0 || idx >= v.elems.length) {
              throw new Error(
                `field accessor "${name}": index ${idx} out of range`,
              );
            }
            return v.elems[idx]!;
          }),
        );
      };
      registerName(f.shenName.toLowerCase());
      registerName(f.shenName);
    }
  }

  // envHolder trick: every curried define closure reads `.env` from
  // this shared container, so all references resolve through the
  // fully-populated env after we finish extending it below.
  const holder: { env: Env } = { env };
  for (const d of defines) {
    env = env.extend(d.name, curriedDefineFn(d, holder));
  }
  holder.env = env;
  return env;
}

function curriedDefineFn(
  def: Define,
  holder: { env: Env },
): Value {
  const arity = def.clauses[0]?.patterns.length ?? 0;
  const build = (collected: Value[]): Value =>
    builtinFn(def.name, (v) => {
      const next = collected.slice();
      next.push(v);
      if (next.length === arity) {
        return evalDefine(def, next, holder.env);
      }
      return build(next);
    });
  return build([]);
}

// --- tsLiteralFor ---

export function tsLiteralFor(
  v: Value,
  shenType: string,
  tt: TypeTable,
): string {
  const t = shenType.trim();

  // list type
  const elem = elemType(t);
  if (elem !== "") {
    if (v.kind !== "list") {
      throw new Error(`expected list for ${t}, got ${v.kind}`);
    }
    const parts = v.elems.map((e) => tsLiteralFor(e, elem, tt));
    return `[${parts.join(", ")}]`;
  }

  // primitives
  switch (t) {
    case "number": {
      if (v.kind === "int") return formatIntLiteral(v.val);
      if (v.kind === "float") return formatFloatLiteral(v.val);
      throw new Error(`cannot produce TS literal for number value ${v.kind}`);
    }
    case "string":
    case "symbol": {
      if (v.kind === "string") return JSON.stringify(v.val);
      throw new Error(`cannot produce TS literal for string value ${v.kind}`);
    }
    case "boolean": {
      if (v.kind === "bool") return v.val ? "true" : "false";
      throw new Error(`cannot produce TS literal for boolean value ${v.kind}`);
    }
  }

  // declared type
  const entry = tt.get(t);
  if (entry !== undefined) {
    if (entry.category === "wrapper" || entry.category === "constrained") {
      // Emit the underlying primitive literal (comparison unwraps via .val()).
      return tsLiteralFor(v, entry.shenPrim, tt);
    }
    if (entry.category === "composite" || entry.category === "guarded") {
      if (v.kind !== "list") {
        throw new Error(
          `expected composite list for ${t}, got ${v.kind}`,
        );
      }
      if (v.elems.length !== entry.fields.length) {
        throw new Error(
          `composite ${t}: value has ${v.elems.length} fields, type has ${entry.fields.length}`,
        );
      }
      const helper = `${entry.importAlias || "shenguard"}.must${entry.tsName}`;
      const parts = entry.fields.map((f, i) =>
        tsLiteralFor(v.elems[i]!, f.typeName, tt),
      );
      return `${helper}(${parts.join(", ")})`;
    }
  }

  throw new Error(`cannot produce TS literal for ${t} value ${v.kind}`);
}

function formatIntLiteral(n: number): string {
  return String(Math.trunc(n));
}

// formatFloatLiteral mirrors Go's formatFloatLiteral: integer-valued
// floats get a trailing ".0" so readers and downstream tooling can tell
// them apart from ints.
function formatFloatLiteral(f: number): string {
  let s: string;
  if (Number.isInteger(f)) {
    s = `${f}.0`;
  } else {
    s = String(f);
  }
  return s;
}

// --- Emit ---

export function emit(h: Harness): string {
  const cfg = h.cfg;
  const tt = cfg.tt;

  // Determine how to compare the return value.
  const retType = cfg.spec.defines.find((d) => d.name === cfg.funcName)!
    .typeSig.returnType;
  const retEntry = tt.get(retType.trim());
  const retCategory = retEntry?.category ?? "";
  const isWrappedReturn =
    retCategory === "wrapper" || retCategory === "constrained";
  const isListReturn = elemType(retType) !== "";
  const useDeepEqual = isListReturn || retCategory === "composite" ||
    retCategory === "guarded";

  const lines: string[] = [];
  lines.push(`// Code generated by shen-derive-ts. DO NOT EDIT.`);
  if (cfg.seed !== 0) {
    lines.push(`// seed: ${cfg.seed}`);
    lines.push(
      `// Sampling seed: ${cfg.seed} (--seed ${cfg.seed} --random-draws ${
        cfg.randomDraws > 0 ? cfg.randomDraws : 8
      })`,
    );
  }
  lines.push(`//`);
  lines.push(
    `// This file checks that ${cfg.implFunc} matches the Shen spec`,
  );
  lines.push(
    `// by evaluating the spec on sampled inputs and comparing outputs.`,
  );
  lines.push(``);
  lines.push(`import { test } from "node:test";`);
  lines.push(`import assert from "node:assert/strict";`);
  lines.push(
    `import * as ${cfg.importAlias || "shenguard"} from "${cfg.importPath}";`,
  );
  lines.push(
    `import { ${cfg.implFunc} } from "${cfg.implModule}";`,
  );
  lines.push(``);

  // Helper constructors referenced by the cases as "shenguard.mustXxx(...)".
  // The alias already qualifies them — nothing extra to emit for wrappers.
  // But for test-source substring checks we inline each helper name as a
  // comment so that tests can grep for "mustAmount" etc. without depending
  // on the shengen-ts module surface.
  const helperNames = collectHelperNames(h);
  if (helperNames.length > 0) {
    lines.push(`// helpers referenced: ${helperNames.join(", ")}`);
    lines.push(``);
  }

  for (const c of h.cases) {
    const call = `${cfg.implFunc}(${c.argsTs.join(", ")})`;
    const wantExpr = c.expectedTs;
    const gotExpr = isWrappedReturn ? "got.val()" : "got";
    const assertFn = useDeepEqual ? "deepStrictEqual" : "strictEqual";
    lines.push(`test(${JSON.stringify(c.label)}, () => {`);
    lines.push(`  const got = ${call};`);
    lines.push(`  const want = ${wantExpr};`);
    lines.push(
      `  assert.${assertFn}(${gotExpr}, want, \`${cfg.funcName}: spec says \${JSON.stringify(want)}, impl returned \${JSON.stringify(got)}\`);`,
    );
    lines.push(`});`);
  }

  return lines.join("\n") + "\n";
}

// collectHelperNames walks the generated argument and expected literals,
// grabbing every `${alias}.mustXxx` reference so we can surface them in a
// header comment (and so tests have stable substrings to grep).
function collectHelperNames(h: Harness): string[] {
  const seen = new Set<string>();
  const re = /must[A-Z][A-Za-z0-9_]*/g;
  const walk = (s: string): void => {
    const matches = s.match(re);
    if (matches === null) return;
    for (const m of matches) seen.add(m);
  };
  for (const c of h.cases) {
    for (const a of c.argsTs) walk(a);
    walk(c.expectedTs);
  }
  return [...seen].sort();
}

// --- tiny helper ---

function showValue(v: Value): string {
  switch (v.kind) {
    case "int":
    case "float":
      return String(v.val);
    case "bool":
      return v.val ? "true" : "false";
    case "string":
      return JSON.stringify(v.val);
    case "list":
      return `[${v.elems.map(showValue).join(" ")}]`;
    case "tuple":
      return `(@p ${showValue(v.fst)} ${showValue(v.snd)})`;
    case "closure":
    case "primPartial":
    case "builtin":
      return `<${v.kind}>`;
  }
}

