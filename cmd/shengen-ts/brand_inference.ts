// brand_inference.ts — infer phantom brand parameters (GDP brands) from
// the sequent-calculus sharing structure of a spec.
//
// Port of cmd/shengen/brand_inference.go; same three rules, same output
// shape, so the Go and TypeScript emitters bind proofs to values the
// same way and the golden tables can be compared across languages:
//
//  1. Each premise variable of wrapper type introduces a brand variable.
//  2. If two premises are related by a verified premise or by a nested
//     field path, their brand variables unify.
//  3. The conclusion type is parameterized by the brands of its
//     constituents that survive unification.
//
// Brandable types are the generated aggregates (composite, guarded,
// sum); primitive wrappers stay unparameterized. A brandable type that
// other rules consume but that inherits nothing mints its own brand —
// it is the head of a proof chain.
//
// One deliberate difference from the Go implementation: containment is
// computed from each rule's premise types rather than from
// SymbolTable.fields, because the TypeScript symbol table does not
// populate fields for sum-type variants with a wrapped conclusion
// (e.g. `human-principal`). Premises are the same information, read
// straight off the sequent.

import type { Datatype, Premise, Rule, SymbolTable } from "./shengen.ts";
import { classify, toPascalCase, tokenize } from "./shengen.ts";

export interface BrandInfo {
  shenName: string;
  tsName: string;
  /** Brand type-parameter names in declaration order, e.g. ["B"]. */
  params: string[];
  /** True when this type introduces its own brand (head of a chain). */
  minted: boolean;
  /** Brand arguments carried by premise i of the producing rule. */
  premiseArgs: string[][];
  /** True for the synthetic entry of a sum-type conclusion. */
  isSum: boolean;
}

export class BrandTable {
  types: Map<string, BrandInfo> = new Map();

  lookup(shenType: string): BrandInfo | undefined {
    return this.types.get(shenType);
  }

  params(shenType: string): string[] {
    return this.types.get(shenType)?.params ?? [];
  }

  isBranded(shenType: string): boolean {
    return this.params(shenType).length > 0;
  }

  /** Stable, diffable rendering — this is what the golden tests pin. */
  toString(): string {
    const names = [...this.types.keys()].sort();
    const out: string[] = [];
    for (const n of names) {
      const info = this.types.get(n)!;
      let kind = "inherited";
      if (info.params.length === 0) kind = "unbranded";
      else if (info.isSum) kind = "sum";
      else if (info.minted) kind = "minted";
      let sig = info.tsName;
      if (info.params.length > 0) sig += `<${info.params.join(", ")}>`;
      let line = `${n.padEnd(24)} ${kind.padEnd(10)} ${sig}`;
      if (info.premiseArgs.length > 0) {
        const parts = info.premiseArgs.map((args) =>
          args.length === 0 ? "-" : args.join("+")
        );
        line += `  premises(${parts.join(" ")})`;
      }
      out.push(line);
    }
    return out.join("\n") + (out.length > 0 ? "\n" : "");
  }
}

function brandableCategory(cat: string | undefined): boolean {
  return cat === "composite" || cat === "guarded" || cat === "sumtype";
}

function resolveAliases(st: SymbolTable, shenType: string): string {
  const seen = new Set<string>();
  let cur = shenType;
  for (;;) {
    const info = st.lookup(cur);
    if (!info || info.category !== "alias" || !info.wrappedType) return cur;
    if (seen.has(cur)) return cur;
    seen.add(cur);
    cur = info.wrappedType;
  }
}

function brandParamNames(n: number): string[] {
  if (n <= 0) return [];
  if (n === 1) return ["B"];
  return Array.from({ length: n }, (_, i) => `B${i + 1}`);
}

class UnionFind {
  private parent: number[];
  constructor(n: number) {
    this.parent = Array.from({ length: n }, (_, i) => i);
  }
  find(x: number): number {
    while (this.parent[x] !== x) {
      this.parent[x] = this.parent[this.parent[x]];
      x = this.parent[x];
    }
    return x;
  }
  union(a: number, b: number): void {
    const ra = this.find(a);
    const rb = this.find(b);
    if (ra === rb) return;
    if (ra < rb) this.parent[rb] = ra;
    else this.parent[ra] = rb;
  }
}

/** generatedShenName mirrors SymbolTable.build / classify name resolution. */
function generatedShenName(dt: Datatype, rule: Rule, st: SymbolTable): string {
  let typeName = rule.conc.typeName;
  if (dt.name !== typeName && (st.concCount.get(typeName) ?? 0) > 1) {
    typeName = dt.name;
  }
  return typeName;
}

/** premiseTypesOf maps each generated type to the premise types of its rule. */
function premiseTypesOf(
  types: Datatype[],
  st: SymbolTable
): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const dt of types) {
    for (const gt of classify(dt, st)) {
      const name = generatedShenName(dt, gt.rule, st);
      out.set(
        name,
        gt.rule.premises.map((p) => resolveAliases(st, p.typeName))
      );
    }
  }
  return out;
}

/**
 * typeContains reports whether `outer` reaches `inner` along a nested
 * premise path of length >= 1. Strict: a type does not contain itself,
 * so two premises of the same type keep two brands.
 */
function typeContains(
  st: SymbolTable,
  prems: Map<string, string[]>,
  outer: string,
  inner: string
): boolean {
  if (!outer || !inner) return false;
  const visited = new Set<string>();
  const walk = (cur: string, depth: number): boolean => {
    if (depth > 16 || visited.has(cur)) return false;
    visited.add(cur);
    const info = st.lookup(cur);
    if (info?.category === "sumtype") {
      for (const v of st.sumTypes.get(cur) ?? []) {
        if (v === inner || walk(v, depth + 1)) return true;
      }
    }
    for (const raw of prems.get(cur) ?? []) {
      const t = resolveAliases(st, listElem(raw) ?? raw);
      if (t === inner) return true;
      if (walk(t, depth + 1)) return true;
    }
    return false;
  };
  return walk(outer, 0);
}

function listElem(shenType: string): string | null {
  const t = shenType.trim();
  if (!t.startsWith("(list ") || !t.endsWith(")")) return null;
  return t.slice("(list ".length, -1).trim();
}

/** verifiedMentions reports whether a raw premise mentions a Shen variable. */
function verifiedMentions(raw: string, varName: string): boolean {
  if (!raw || !varName) return false;
  for (const tok of tokenize(raw)) {
    if (tok.replace(/^[[(]+|[\])]+$/g, "") === varName) return true;
  }
  return false;
}

function premisesRelated(
  st: SymbolTable,
  prems: Map<string, string[]>,
  rule: Rule,
  a: Premise,
  b: Premise
): boolean {
  const at = resolveAliases(st, a.typeName);
  const bt = resolveAliases(st, b.typeName);
  if (
    typeContains(st, prems, at, bt) ||
    typeContains(st, prems, bt, at)
  ) {
    return true;
  }
  for (const v of rule.verified) {
    if (verifiedMentions(v.raw, a.varName) && verifiedMentions(v.raw, b.varName)) {
      return true;
    }
  }
  return false;
}

function consumedTypes(types: Datatype[], st: SymbolTable): Set<string> {
  const out = new Set<string>();
  for (const dt of types) {
    for (const r of dt.rules) {
      for (const p of r.premises) {
        const target = resolveAliases(st, listElem(p.typeName) ?? p.typeName);
        if (brandableCategory(st.lookup(target)?.category)) {
          out.add(target);
          for (const v of st.sumTypes.get(target) ?? []) out.add(v);
        }
      }
    }
  }
  return out;
}

/** inferBrands computes the brand table for a parsed spec. */
export function inferBrands(types: Datatype[], st: SymbolTable): BrandTable {
  const bt = new BrandTable();
  for (const [name, info] of [...st.types.entries()].sort((a, b) =>
    a[0] < b[0] ? -1 : 1
  )) {
    bt.types.set(name, {
      shenName: name,
      tsName: toPascalCase(name),
      params: [],
      minted: false,
      premiseArgs: [],
      isSum: info.category === "sumtype",
    });
  }

  const prems = premiseTypesOf(types, st);
  const consumed = consumedTypes(types, st);

  for (let pass = 0; pass < 16; pass++) {
    let changed = false;

    for (const dt of types) {
      for (const gt of classify(dt, st)) {
        const shenName = generatedShenName(dt, gt.rule, st);
        const info = bt.types.get(shenName);
        if (!info) continue;
        if (!brandableCategory(st.lookup(shenName)?.category)) continue;
        if (
          inferRuleBrands(bt, st, prems, gt.rule, consumed.has(shenName), info)
        ) {
          changed = true;
        }
      }
    }

    // A sum-type union carries the widest variant's brands, and every
    // variant is padded to it so each is assignable to the union.
    for (const [concType, variants] of st.sumTypes) {
      const info = bt.types.get(concType);
      if (!info) continue;
      let widest = 0;
      for (const v of variants) widest = Math.max(widest, bt.params(v).length);
      if (widest > info.params.length) {
        info.params = brandParamNames(widest);
        changed = true;
      }
      for (const v of variants) {
        const vi = bt.types.get(v);
        if (vi && vi.params.length < widest) {
          vi.params = brandParamNames(widest);
          changed = true;
        }
      }
    }

    // Aliases are transparent.
    for (const [name, info] of bt.types) {
      const ti = st.lookup(name);
      if (!ti || ti.category !== "alias" || !ti.wrappedType) continue;
      const target = bt.params(resolveAliases(st, ti.wrappedType));
      if (target.length > info.params.length) {
        info.params = [...target];
        changed = true;
      }
    }

    if (!changed) break;
  }

  return bt;
}

function inferRuleBrands(
  bt: BrandTable,
  st: SymbolTable,
  prems: Map<string, string[]>,
  rule: Rule,
  isConsumed: boolean,
  info: BrandInfo
): boolean {
  // Rule 1.
  const premSlots: number[][] = [];
  let slotCount = 0;
  for (const p of rule.premises) {
    const target = resolveAliases(st, listElem(p.typeName) ?? p.typeName);
    const n = bt.params(target).length;
    const slots: number[] = [];
    for (let j = 0; j < n; j++) slots.push(slotCount++);
    premSlots.push(slots);
  }

  // Rule 2.
  const uf = new UnionFind(slotCount);
  for (let i = 0; i < rule.premises.length; i++) {
    for (let j = i + 1; j < rule.premises.length; j++) {
      if (premSlots[i].length === 0 || premSlots[j].length === 0) continue;
      if (!premisesRelated(st, prems, rule, rule.premises[i], rule.premises[j])) {
        continue;
      }
      const n = Math.min(premSlots[i].length, premSlots[j].length);
      for (let k = 0; k < n; k++) uf.union(premSlots[i][k], premSlots[j][k]);
    }
  }

  // Rule 3.
  const classIndex = new Map<number, number>();
  for (let s = 0; s < slotCount; s++) {
    const root = uf.find(s);
    if (!classIndex.has(root)) classIndex.set(root, classIndex.size);
  }

  let params = brandParamNames(classIndex.size);
  let minted = false;
  if (params.length === 0 && isConsumed) {
    params = brandParamNames(1);
    minted = true;
  }

  const premiseArgs: string[][] = rule.premises.map((_, i) =>
    premSlots[i].map((s) => params[classIndex.get(uf.find(s))!])
  );

  let changed = false;
  if (params.length > info.params.length) {
    info.params = params;
    info.minted = minted;
    changed = true;
  } else if (
    params.length === info.params.length &&
    params.length > 0 &&
    info.minted !== minted
  ) {
    info.minted = minted;
    changed = true;
  }
  if (JSON.stringify(info.premiseArgs) !== JSON.stringify(premiseArgs)) {
    info.premiseArgs = premiseArgs;
    changed = true;
  }
  return changed;
}

// ============================================================================
// Emission helpers
// ============================================================================

/** `<B>` for a declaration; "" when unbranded. */
export function brandDeclSuffix(params: string[]): string {
  return params.length === 0 ? "" : `<${params.join(", ")}>`;
}

/** `<B>` for a type reference; "" when unbranded. */
export function brandArgSuffix(args: string[]): string {
  return args.length === 0 ? "" : `<${args.join(", ")}>`;
}

/**
 * tsTypeWithBrands renders a Shen type as a TypeScript type with the
 * brand arguments the inference unified it into. `args` comes from the
 * producing rule's premiseArgs.
 */
export function tsTypeWithBrands(
  st: SymbolTable,
  bt: BrandTable | null,
  shenType: string,
  args: string[],
  shenTypeToTs: (t: string) => string
): string {
  if (!bt) return shenTypeToTs(shenType);
  const elem = listElem(shenType);
  if (elem !== null) {
    return `${tsTypeWithBrands(st, bt, elem, args, shenTypeToTs)}[]`;
  }
  const base = shenTypeToTs(shenType);
  const target = resolveAliases(st, shenType);
  const params = bt.params(target);
  if (params.length === 0) return base;
  let use = args.length === 0 ? params : args;
  if (use.length > params.length) use = use.slice(0, params.length);
  return base + brandArgSuffix(use);
}

/** True when a Shen type lowers to a generated class carrying a witness. */
export function hasWitness(st: SymbolTable, shenType: string): boolean {
  if (listElem(shenType) !== null) return false;
  const info = st.lookup(resolveAliases(st, shenType));
  if (!info) return false;
  return (
    info.category === "wrapper" ||
    info.category === "constrained" ||
    info.category === "composite" ||
    info.category === "guarded"
  );
}

export { resolveAliases, listElem };
