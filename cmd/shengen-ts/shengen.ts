#!/usr/bin/env npx tsx
// shengen-ts — Generate TypeScript guard types from Shen sequent-calculus specs.
//
// Reads specs/core.shen, parses (datatype ...) blocks, and emits TypeScript
// classes with private fields and static factory methods that enforce invariants.
//
// Usage: npx tsx shengen.ts [spec-path] [--out file]
//
// Architecture mirrors cmd/shengen/main.go:
//   1. Parse: extract (datatype ...) blocks
//   2. Symbol table: classify each type
//   3. Resolve: translate verified premises via s-expression parser
//   4. Emit: TypeScript classes with guarded constructors

import { readFileSync, writeFileSync } from "fs";
import { fileURLToPath } from "url";

// ============================================================================
// AST
// ============================================================================

export interface Premise {
  varName: string;
  typeName: string;
}

export interface VerifiedPremise {
  raw: string;
}

export interface Conclusion {
  fields: string[];
  typeName: string;
  isWrapped: boolean;
}

export interface Rule {
  premises: Premise[];
  verified: VerifiedPremise[];
  conc: Conclusion;
}

export interface Datatype {
  name: string;
  rules: Rule[];
}

// --- (define ...) blocks ---
// A Shen define is a (possibly multi-clause, possibly guarded) helper function.
// In specs it appears as `(define name {t1 --> ... --> ret} pat1 pat2 -> body ...)`
// with clauses separated by ` -> ` transitions. See cmd/shengen/main.go:90-99
// for the Go reference shape.

export interface DefineClause {
  patterns: string[]; // raw pattern per parameter ("_", "X", "[]", "[H | T]")
  result: string; // raw result expression (atom or balanced s-expr source)
  guard: string; // raw guard source after `where`, or "" when absent
}

export interface Define {
  name: string; // Shen name, including any trailing `?`
  signature: string[]; // parsed `{t1 --> t2 --> ret}` into [t1, t2, …, ret]; empty if unparseable
  clauses: DefineClause[];
}

export interface Spec {
  datatypes: Datatype[];
  defines: Define[];
}

// ============================================================================
// Symbol Table
// ============================================================================

export interface FieldInfo {
  index: number;
  shenName: string;
  shenType: string;
}

export interface TypeInfo {
  shenName: string;
  tsName: string;
  category: "wrapper" | "constrained" | "composite" | "guarded" | "alias" | "sumtype";
  fields: FieldInfo[];
  wrappedPrim: string | null;
  wrappedType: string | null;
}

export class SymbolTable {
  types: Map<string, TypeInfo> = new Map();
  concCount: Map<string, number> = new Map();
  sumTypes: Map<string, string[]> = new Map();
  defines: Map<string, Define> = new Map();

  registerDefines(defs: Define[]): void {
    for (const d of defs) this.defines.set(d.name, d);
  }

  build(datatypes: Datatype[]): void {
    // Pass 1: count conclusion type producers
    for (const dt of datatypes) {
      for (const r of dt.rules) {
        this.concCount.set(
          r.conc.typeName,
          (this.concCount.get(r.conc.typeName) ?? 0) + 1
        );
      }
    }

    // Pass 2: build entries
    for (const dt of datatypes) {
      for (const r of dt.rules) {
        let typeName = r.conc.typeName;
        if (
          dt.name !== typeName &&
          (this.concCount.get(typeName) ?? 0) > 1
        ) {
          typeName = dt.name;
          // Track sum type: conclusion type → concrete block names
          const existing = this.sumTypes.get(r.conc.typeName) ?? [];
          existing.push(typeName);
          this.sumTypes.set(r.conc.typeName, existing);
        }

        const info: TypeInfo = {
          shenName: typeName,
          tsName: toPascalCase(typeName),
          category: "composite",
          fields: [],
          wrappedPrim: null,
          wrappedType: null,
        };

        const prems = r.premises;
        const verified = r.verified;

        // A rule is a sum-type variant when its conclusion type has more than
        // one producer — i.e. other (datatype ...) blocks also conclude it.
        // Variants must retain their own identity (class), not be erased into
        // a type alias of the single inner premise. Mirrors Go main.go:159.
        const isSumVariant =
          (this.concCount.get(r.conc.typeName) ?? 0) > 1;

        if (
          r.conc.isWrapped &&
          verified.length === 0 &&
          prems.length === 1 &&
          isPrimitive(prems[0].typeName)
        ) {
          info.category = "wrapper";
          info.wrappedPrim = prems[0].typeName;
        } else if (
          r.conc.isWrapped &&
          verified.length > 0 &&
          prems.length >= 1 &&
          isPrimitive(prems[0].typeName)
        ) {
          info.category = "constrained";
          info.wrappedPrim = prems[0].typeName;
        } else if (
          r.conc.isWrapped &&
          prems.length === 1 &&
          !isPrimitive(prems[0].typeName) &&
          !isSumVariant
        ) {
          info.category = "alias";
          info.wrappedType = prems[0].typeName;
        } else if (!r.conc.isWrapped && verified.length > 0) {
          info.category = "guarded";
        } else if (!r.conc.isWrapped) {
          info.category = "composite";
        }

        if (!r.conc.isWrapped) {
          const premMap = new Map(prems.map((p) => [p.varName, p.typeName]));
          for (let i = 0; i < r.conc.fields.length; i++) {
            const fieldName = r.conc.fields[i];
            info.fields.push({
              index: i,
              shenName: fieldName,
              shenType: premMap.get(fieldName) ?? "unknown",
            });
          }
        }

        this.types.set(typeName, info);
      }
    }

    // Pass 3: Register synthetic entries for sum types.
    for (const [concType] of this.sumTypes) {
      if (!this.types.has(concType)) {
        this.types.set(concType, {
          shenName: concType,
          tsName: toPascalCase(concType),
          category: "sumtype",
          fields: [],
          wrappedPrim: null,
          wrappedType: null,
        });
      }
    }
  }

  isSumType(name: string): boolean {
    return this.sumTypes.has(name);
  }

  lookup(name: string): TypeInfo | undefined {
    return this.types.get(name);
  }

  isWrapper(shenType: string): boolean {
    const info = this.lookup(shenType);
    return (
      info !== undefined &&
      (info.category === "wrapper" || info.category === "constrained")
    );
  }
}

// ============================================================================
// S-Expression Parser
// ============================================================================

export interface SExpr {
  atom: string | null;
  children: SExpr[] | null;
}

export function isAtom(e: SExpr): boolean {
  return e.atom !== null;
}
export function isCall(e: SExpr): boolean {
  return e.children !== null && e.children.length > 0;
}
export function op(e: SExpr): string {
  if (isCall(e) && e.children![0].atom) return e.children![0].atom;
  return "";
}
export function sexprToString(e: SExpr): string {
  if (!isCall(e)) return e.atom ?? "";
  return "(" + e.children!.map(sexprToString).join(" ") + ")";
}

function tokenize(s: string): string[] {
  const tokens: string[] = [];
  let cur = "";
  for (const ch of s) {
    if (ch === "(" || ch === ")") {
      if (cur) {
        tokens.push(cur);
        cur = "";
      }
      tokens.push(ch);
    } else if (ch === " " || ch === "\t" || ch === "\n") {
      if (cur) {
        tokens.push(cur);
        cur = "";
      }
    } else {
      cur += ch;
    }
  }
  if (cur) tokens.push(cur);
  return tokens;
}

function parseTokens(
  tokens: string[],
  pos: number
): [SExpr, number] {
  if (pos >= tokens.length) return [{ atom: "", children: null }, pos];
  if (tokens[pos] === "(") {
    pos++;
    const children: SExpr[] = [];
    while (pos < tokens.length && tokens[pos] !== ")") {
      const [child, np] = parseTokens(tokens, pos);
      children.push(child);
      pos = np;
    }
    if (pos < tokens.length) pos++;
    return [{ atom: null, children }, pos];
  }
  return [{ atom: tokens[pos], children: null }, pos + 1];
}

export function parseSExpr(input: string): SExpr {
  const tokens = tokenize(input.trim());
  if (tokens.length === 0) return { atom: "", children: null };
  const [expr] = parseTokens(tokens, 0);
  return expr;
}

// ============================================================================
// Accessor Chain Resolution
// ============================================================================

export interface ResolvedExpr {
  code: string;
  tsType: string;
  shenType: string;
  isMulti?: boolean;
  fields?: FieldInfo[];
}

export function resolveExpr(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): ResolvedExpr | null {
  if (!isCall(expr)) {
    return resolveAtom(st, expr.atom ?? "", varMap);
  }
  switch (op(expr)) {
    case "head":
    case "tail":
      return resolveHeadTail(st, expr, varMap, op(expr) === "head");
    case "shen.mod":
      return resolveBinOp(st, expr, varMap);
    case "length":
      return resolveLength(st, expr, varMap);
    case "not":
      return resolveNot(st, expr, varMap);
  }
  return null;
}

function resolveAtom(
  st: SymbolTable,
  atom: string,
  varMap: Map<string, string>
): ResolvedExpr | null {
  if (isNumericLiteral(atom)) {
    return { code: atom, tsType: "number", shenType: "number" };
  }
  if (atom === "[]") {
    return { code: "[]", tsType: "never[]", shenType: "list" };
  }
  const shenType = varMap.get(atom);
  if (shenType) {
    return {
      code: toCamelCase(atom),
      tsType: shenTypeToTs(shenType),
      shenType,
    };
  }
  return { code: atom, tsType: "unknown", shenType: "unknown" };
}

function resolveHeadTail(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>,
  isHead: boolean
): ResolvedExpr | null {
  if (!expr.children || expr.children.length !== 2) return null;
  const inner = resolveExpr(st, expr.children[1], varMap);
  if (!inner) return null;

  if (inner.isMulti && inner.fields) {
    return accessFields(inner.code, inner.fields, isHead);
  }

  const typeInfo = st.lookup(inner.shenType);
  if (!typeInfo || typeInfo.fields.length === 0) return null;
  return accessFields(inner.code, typeInfo.fields, isHead);
}

function accessFields(
  baseCode: string,
  fields: FieldInfo[],
  isHead: boolean
): ResolvedExpr | null {
  if (fields.length === 0) return null;

  if (isHead) {
    const f = fields[0];
    return {
      code: `${baseCode}.${toCamelCase(f.shenName)}()`,
      tsType: shenTypeToTs(f.shenType),
      shenType: f.shenType,
    };
  }

  const rest = fields.slice(1);
  if (rest.length === 0) return null;
  if (rest.length === 1) {
    const f = rest[0];
    return {
      code: `${baseCode}.${toCamelCase(f.shenName)}()`,
      tsType: shenTypeToTs(f.shenType),
      shenType: f.shenType,
    };
  }
  return { code: baseCode, tsType: "multi", shenType: "multi", isMulti: true, fields: rest };
}

function resolveBinOp(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): ResolvedExpr | null {
  if (!expr.children || expr.children.length !== 3) return null;
  const lhs = resolveExpr(st, expr.children[1], varMap);
  const rhs = resolveExpr(st, expr.children[2], varMap);
  if (!lhs || !rhs) return null;
  return {
    code: `Math.trunc(${unwrap(st, lhs)}) % ${rhs.code}`,
    tsType: "number",
    shenType: "number",
  };
}

function resolveLength(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): ResolvedExpr | null {
  if (!expr.children || expr.children.length !== 2) return null;
  const inner = resolveExpr(st, expr.children[1], varMap);
  if (!inner) return null;
  return {
    code: `${unwrap(st, inner)}.length`,
    tsType: "number",
    shenType: "number",
  };
}

function resolveNot(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): ResolvedExpr | null {
  if (!expr.children || expr.children.length !== 2) return null;
  const inner = resolveExpr(st, expr.children[1], varMap);
  if (!inner) return null;
  return {
    code: `!(${inner.code})`,
    tsType: "boolean",
    shenType: "boolean",
  };
}

function unwrap(st: SymbolTable, r: ResolvedExpr): string {
  if (st.isWrapper(r.shenType)) return `${r.code}.val()`;
  return r.code;
}

// ============================================================================
// Verified Premise → TypeScript
// ============================================================================

export function verifiedToTs(
  st: SymbolTable,
  v: VerifiedPremise,
  varMap: Map<string, string>
): [string, string] {
  const expr = parseSExpr(v.raw);
  if (!isCall(expr)) return [`/* TODO: ${v.raw} */ true`, "unhandled"];

  const fname = op(expr);
  switch (fname) {
    case ">=":
    case "<=":
    case ">":
    case "<":
      return translateCmp(st, expr, varMap, fname);
    case "=":
      return translateEq(st, expr, varMap);
    case "not":
      return translateNot(st, expr, varMap);
    case "element?":
      return translateElement(st, expr, varMap);
  }
  // User-defined predicate call via (define …). The resolved TS name mirrors
  // what generateDefineHelpers emits — `${toCamelCase(name sans ?)}(…args)`.
  if (st.defines.has(fname)) {
    const args = expr.children!.slice(1).map((c) =>
      translateDefineExpr(c, varMap, st)
    );
    const callee = definePascalName(fname);
    return [
      `${callee}(${args.join(", ")})`,
      `${fname} check failed`,
    ];
  }
  return [`/* TODO: ${v.raw} */ true`, `unhandled op: ${fname}`];
}

function translateCmp(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>,
  cmpOp: string
): [string, string] {
  if (!expr.children || expr.children.length !== 3)
    return ["/* bad arity */ true", "comparison needs 2 args"];
  const lhs = resolveExpr(st, expr.children[1], varMap);
  const rhs = resolveExpr(st, expr.children[2], varMap);
  if (!lhs || !rhs)
    return [`/* TODO: ${sexprToString(expr)} */ true`, "could not resolve"];
  return [
    `${unwrap(st, lhs)} ${cmpOp} ${unwrap(st, rhs)}`,
    `${lhs.code} must be ${cmpOp} ${rhs.code}`,
  ];
}

function translateEq(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): [string, string] {
  if (!expr.children || expr.children.length !== 3)
    return ["/* bad arity */ true", "equality needs 2 args"];
  const lhs = resolveExpr(st, expr.children[1], varMap);
  const rhs = resolveExpr(st, expr.children[2], varMap);

  if (!lhs || !rhs) {
    const fallback = structuralMatchFallback(st, expr, varMap);
    if (fallback) return fallback;
    return [`/* TODO: ${sexprToString(expr)} */ true`, "could not resolve"];
  }

  let goL = lhs.code;
  let goR = rhs.code;
  if (st.isWrapper(lhs.shenType) && isPrimitive(rhs.shenType))
    goL = unwrap(st, lhs);
  if (st.isWrapper(rhs.shenType) && isPrimitive(lhs.shenType))
    goR = unwrap(st, rhs);
  return [`${goL} === ${goR}`, `${lhs.code} must equal ${rhs.code}`];
}

function translateNot(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): [string, string] {
  if (!expr.children || expr.children.length !== 2)
    return ["/* bad not */ true", "not needs 1 arg"];
  const inner = expr.children[1];
  if (isCall(inner) && op(inner) === "=") {
    const [code, msg] = translateEq(st, inner, varMap);
    return [`!(${code})`, `not: ${msg}`];
  }
  const resolved = resolveExpr(st, inner, varMap);
  if (!resolved)
    return [`/* TODO: ${sexprToString(expr)} */ true`, "could not resolve not"];
  return [`!(${resolved.code})`, `negation of ${resolved.code}`];
}

function translateElement(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): [string, string] {
  if (!expr.children || expr.children.length < 3)
    return ["/* TODO: element? */ true", "element? needs args"];
  const resolved = resolveExpr(st, expr.children[1], varMap);
  if (!resolved)
    return ["/* TODO: element? */ true", "could not resolve element?"];
  // Collect set elements from remaining children.
  // Shen list [a b c] tokenizes as atoms with brackets attached,
  // e.g. "[a", "b", "c]" — strip brackets to get clean element names.
  const elements: string[] = [];
  for (let i = 2; i < expr.children.length; i++) {
    let atom = expr.children[i].atom ?? "";
    atom = atom.replace(/^\[/, "").replace(/\]$/, "");
    if (atom) elements.push(atom);
  }
  if (elements.length > 0) {
    const val = unwrap(st, resolved);
    const setLiteral = `new Set([${elements.map((e) => `"${e}"`).join(", ")}])`;
    return [
      `${setLiteral}.has(${val})`,
      `${resolved.code} must be in the valid set`,
    ];
  }
  return [
    `/* TODO: element? ${resolved.code} */ true`,
    `${resolved.code} must be in the valid set`,
  ];
}

// ============================================================================
// Structural Match Fallback
// ============================================================================

function extractBaseVar(expr: SExpr): string | null {
  if (!isCall(expr)) {
    if (expr.atom && /^[A-Z]/.test(expr.atom)) return expr.atom;
    return null;
  }
  const o = op(expr);
  if ((o === "head" || o === "tail") && expr.children!.length === 2) {
    return extractBaseVar(expr.children![1]);
  }
  return null;
}

// inferTargetFields estimates which fields a head/tail chain is targeting
// by counting tail operations (which drop leading fields).
// Port of Go main.go:606-626.
export function inferTargetFields(
  expr: SExpr,
  fields: FieldInfo[]
): FieldInfo[] {
  let tailCount = 0;
  let current = expr;
  while (isCall(current) && current.children!.length === 2) {
    const o = op(current);
    if (o === "tail") tailCount++;
    else if (o !== "head") break;
    current = current.children![1];
  }
  if (fields.length === 0) return fields;
  if (tailCount >= fields.length) return fields.slice(fields.length - 1);
  return fields.slice(tailCount);
}

export function structuralMatchFallback(
  st: SymbolTable,
  expr: SExpr,
  varMap: Map<string, string>
): [string, string] | null {
  if (!expr.children || expr.children.length !== 3) return null;
  const lhsVar = extractBaseVar(expr.children[1]);
  const rhsVar = extractBaseVar(expr.children[2]);
  if (!lhsVar || !rhsVar) return null;

  const lhsType = varMap.get(lhsVar);
  const rhsType = varMap.get(rhsVar);
  if (!lhsType || !rhsType) return null;

  const lhsInfo = st.lookup(lhsType);
  const rhsInfo = st.lookup(rhsType);
  if (
    !lhsInfo ||
    !rhsInfo ||
    lhsInfo.fields.length === 0 ||
    rhsInfo.fields.length === 0
  ) {
    return null;
  }

  // Narrow each side to the fields the head/tail chain is actually targeting.
  // Without this step the scan picks the first shared-type pair by position,
  // which is wrong when the chain's tail-depth is pointing at a later field.
  const lhsTarget = inferTargetFields(expr.children[1], lhsInfo.fields);
  const rhsTarget = inferTargetFields(expr.children[2], rhsInfo.fields);

  const emit = (lf: FieldInfo, rf: FieldInfo): [string, string] => {
    const l = `${toCamelCase(lhsVar)}.${toCamelCase(lf.shenName)}()`;
    const r = `${toCamelCase(rhsVar)}.${toCamelCase(rf.shenName)}()`;
    return [
      `${l} === ${r}`,
      `${toCamelCase(lhsVar)}.${toCamelCase(lf.shenName)} must equal ${toCamelCase(rhsVar)}.${toCamelCase(rf.shenName)}`,
    ];
  };

  // First pass: only within the targeted subsets.
  for (const lf of lhsTarget) {
    for (const rf of rhsTarget) {
      if (lf.shenType === rf.shenType && !isPrimitive(lf.shenType)) {
        return emit(lf, rf);
      }
    }
  }

  // Last resort: any shared non-primitive field type anywhere.
  for (const lf of lhsInfo.fields) {
    for (const rf of rhsInfo.fields) {
      if (lf.shenType === rf.shenType && !isPrimitive(lf.shenType)) {
        return emit(lf, rf);
      }
    }
  }
  return null;
}

// ============================================================================
// Parser
// ============================================================================

export function extractBlocks(source: string, marker: string): string[] {
  const blocks: string[] = [];
  let content = source;
  while (true) {
    const idx = content.indexOf(marker);
    if (idx === -1) break;
    content = content.slice(idx);

    let depth = 0;
    let end = -1;
    for (let i = 0; i < content.length; i++) {
      if (content[i] === "(") depth++;
      else if (content[i] === ")") {
        depth--;
        if (depth === 0) {
          end = i + 1;
          break;
        }
      }
    }
    if (end === -1) break;

    blocks.push(content.slice(0, end));
    content = content.slice(end);
  }
  return blocks;
}

export function parseFileString(source: string): Datatype[] {
  const types: Datatype[] = [];
  for (const block of extractBlocks(source, "(datatype ")) {
    const dt = parseDatatype(block);
    if (dt) types.push(dt);
  }
  return types;
}

export function parseFile(path: string): Datatype[] {
  return parseFileString(readFileSync(path, "utf-8"));
}

// parseSpecString returns both datatype blocks and (define …) helpers. Callers
// that only need datatypes can keep using parseFileString.
export function parseSpecString(source: string): Spec {
  const datatypes = parseFileString(source);
  const defines: Define[] = [];
  for (const block of extractBlocks(source, "(define ")) {
    const def = parseDefine(block);
    if (def) defines.push(def);
  }
  return { datatypes, defines };
}

export function parseSpecFile(path: string): Spec {
  return parseSpecString(readFileSync(path, "utf-8"));
}

// mergeSpecs concatenates specs parsed from multiple files and rejects
// redefinitions (both datatypes and defines) with a pointed error.
export function mergeSpecs(
  groups: Array<{ path: string; spec: Spec }>
): Spec {
  const dtGroups = groups.map((g) => ({
    path: g.path,
    datatypes: g.spec.datatypes,
  }));
  const datatypes = mergeDatatypeGroups(dtGroups);
  const seenDefine = new Map<string, string>();
  const defines: Define[] = [];
  for (const { path, spec } of groups) {
    for (const def of spec.defines) {
      const prev = seenDefine.get(def.name);
      if (prev !== undefined && prev !== path) {
        throw new Error(
          `define "${def.name}" declared in both ${prev} and ${path}; ` +
            `rename one or remove the duplicate`
        );
      }
      seenDefine.set(def.name, path);
      defines.push(def);
    }
  }
  return { datatypes, defines };
}

// mergeDatatypeGroups concatenates parsed datatype groups from multiple source
// files and rejects cross-file redefinitions. Within-file duplicates (if any)
// flow through unchanged — parseFile's behavior is preserved. This is the
// minimum viable multi-spec story (§2.4 of the parity handoff); a richer
// (import "other.shen") directive can come later.
export function mergeDatatypeGroups(
  groups: Array<{ path: string; datatypes: Datatype[] }>
): Datatype[] {
  const seen = new Map<string, string>();
  const all: Datatype[] = [];
  for (const { path, datatypes } of groups) {
    for (const dt of datatypes) {
      const prev = seen.get(dt.name);
      if (prev !== undefined && prev !== path) {
        throw new Error(
          `datatype "${dt.name}" declared in both ${prev} and ${path}; ` +
            `rename one or remove the duplicate`
        );
      }
      seen.set(dt.name, path);
      all.push(dt);
    }
  }
  return all;
}

export function parseDatatype(block: string): Datatype | null {
  block = block.replace(/^\(datatype /, "");
  const nlIdx = block.indexOf("\n");
  if (nlIdx === -1) return null;

  const name = block.slice(0, nlIdx).trim();
  const body = block.slice(nlIdx).replace(/[\s)]+$/, "");
  const lines = body.split("\n");

  const dt: Datatype = { name, rules: [] };
  let premLines: string[] = [];
  let concLines: string[] = [];
  let seenInf = false;

  const flush = () => {
    if (concLines.length === 0) return;
    const r = buildRule(premLines, concLines);
    if (r) dt.rules.push(r);
  };

  for (const line of lines) {
    const t = line.trim();
    if (!t) continue;
    if (t.length >= 3 && (/^=+$/.test(t) || /^_+$/.test(t))) {
      if (seenInf) {
        flush();
        premLines = [];
        concLines = [];
        seenInf = false;
      }
      seenInf = true;
      continue;
    }
    if (!seenInf) premLines.push(t);
    else concLines.push(t);
  }
  flush();
  return dt.rules.length > 0 ? dt : null;
}

// extractBalancedParen pulls the first balanced parenthesized expression
// from `s` (which must start with `(`), returning [expr, indexPastEnd].
// Port of Go main.go:1369-1385.
function extractBalancedParen(s: string): [string, number] {
  if (s.length === 0 || s[0] !== "(") return ["", 0];
  let depth = 0;
  for (let i = 0; i < s.length; i++) {
    if (s[i] === "(") depth++;
    else if (s[i] === ")") {
      depth--;
      if (depth === 0) return [s.slice(0, i + 1), i + 1];
    }
  }
  return [s, s.length];
}

// splitPatterns tokenizes a pattern string respecting bracket nesting, so
// "[Med | Meds]" stays one token and "[[X Y] | Rest]" stays one token.
// Port of Go main.go:1334-1365.
export function splitPatterns(s: string): string[] {
  const patterns: string[] = [];
  let current = "";
  let depth = 0;
  for (const ch of s) {
    if (ch === "[") {
      depth++;
      current += ch;
    } else if (ch === "]") {
      depth--;
      current += ch;
      if (depth === 0 && current.length > 0) {
        patterns.push(current);
        current = "";
      }
    } else if (ch === " " || ch === "\t") {
      if (depth > 0) {
        current += ch;
      } else if (current.length > 0) {
        patterns.push(current);
        current = "";
      }
    } else {
      current += ch;
    }
  }
  if (current.length > 0) patterns.push(current);
  return patterns;
}

// parseSignature turns `{t1 --> t2 --> ret}` into ["t1", "t2", "ret"].
// Parameterized types like `(list ...)` stay as a single element. Returns [] if
// the input is not a brace-wrapped arrow chain.
export function parseSignature(raw: string): string[] {
  const src = raw.trim();
  if (!src.startsWith("{") || !src.endsWith("}")) return [];
  const inner = src.slice(1, -1).trim();
  // Split on ` --> ` while respecting paren depth (for (list foo) etc.).
  const parts: string[] = [];
  let depth = 0;
  let cur = "";
  for (let i = 0; i < inner.length; i++) {
    const ch = inner[i];
    if (ch === "(") depth++;
    else if (ch === ")") depth--;
    if (depth === 0 && inner.startsWith(" --> ", i)) {
      parts.push(cur.trim());
      cur = "";
      i += 4; // skip over "--> " (loop's i++ consumes the leading space)
      continue;
    }
    cur += ch;
  }
  if (cur.trim()) parts.push(cur.trim());
  return parts;
}

// parseDefine ports cmd/shengen/main.go:1242-1330. Handles multi-clause
// `(define name {sig} pat... -> result [where guard] pat... -> result ...)`
// syntax with balanced-paren result expressions.
export function parseDefine(block: string): Define | null {
  // extractBlocks hands us the balanced `(define …)` with outer parens intact.
  // Strip exactly the `(define ` prefix and the single matching trailing `)`;
  // anything greedier corrupts bodies that legitimately end in `)))`.
  if (!block.startsWith("(define ") || !block.endsWith(")")) return null;
  let rest = block.slice("(define ".length, -1);
  const nlIdx = rest.indexOf("\n");
  if (nlIdx === -1) return null;
  const name = rest.slice(0, nlIdx).trim();
  // Trim trailing whitespace — any unbalanced parens would indicate a
  // malformed input upstream, not something we should silently absorb.
  const body = rest.slice(nlIdx).replace(/\s+$/, "");

  // Normalize whitespace to a single line so the arrow-splitter below
  // sees consistent ` -> ` separators.
  const bodyOneLine = body.split(/\s+/).filter(Boolean).join(" ");

  // Optional first part is a type signature `{t1 --> ... --> ret}`.
  let signature: string[] = [];
  let rhs = bodyOneLine;
  if (rhs.startsWith("{")) {
    const closeIdx = rhs.indexOf("}");
    if (closeIdx !== -1) {
      signature = parseSignature(rhs.slice(0, closeIdx + 1));
      rhs = rhs.slice(closeIdx + 1).trim();
    }
  }

  const segments = rhs.split(" -> ");
  if (segments.length < 2) return null;

  const clauses: DefineClause[] = [];
  let currentPatterns = segments[0];

  for (let i = 1; i < segments.length; i++) {
    let seg = segments[i];
    let result = "";
    let guard = "";
    let nextPatterns = "";
    // When result is harvested as a balanced paren expression we trust the
    // extractor's bookkeeping; only bare-atom results need later cleanup.
    let resultCameFromBalancedParen = false;

    const whereIdx = seg.indexOf(" where ");
    if (whereIdx !== -1) {
      result = seg.slice(0, whereIdx).trim();
      if (result.startsWith("(")) resultCameFromBalancedParen = true;
      const afterWhere = seg.slice(whereIdx + 7).trim();
      if (afterWhere.startsWith("(")) {
        const [g, endIdx] = extractBalancedParen(afterWhere);
        guard = g;
        nextPatterns = afterWhere.slice(endIdx).trim();
      } else {
        guard = afterWhere;
      }
    } else {
      seg = seg.trim();
      if (seg.startsWith("(")) {
        const [expr, endIdx] = extractBalancedParen(seg);
        result = expr;
        resultCameFromBalancedParen = true;
        nextPatterns = seg.slice(endIdx).trim();
      } else {
        const tokens = seg.split(/\s+/);
        result = tokens[0];
        if (tokens.length > 1) nextPatterns = tokens.slice(1).join(" ");
      }
    }

    // Bare-atom results (e.g. `true`, `42`, `"foo"`) may carry a trailing
    // close-paren from the enclosing (define …) frame's segment-joining;
    // trim that. Balanced s-exprs are already correct.
    if (!resultCameFromBalancedParen) {
      result = result.replace(/\)+$/, "").trim();
    }

    const patterns = splitPatterns(currentPatterns);
    if (patterns.length > 0) {
      clauses.push({ patterns, result, guard });
    }
    currentPatterns = nextPatterns;
  }

  if (clauses.length === 0) return null;
  return { name, signature, clauses };
}

function buildRule(premLines: string[], concLines: string[]): Rule | null {
  const r: Rule = { premises: [], verified: [], conc: { fields: [], typeName: "", isWrapped: false } };

  for (let line of premLines) {
    line = line.replace(/;$/, "").trim();
    if (!line) continue;
    if (line.endsWith(": verified")) {
      r.verified.push({ raw: line.replace(/\s*:\s*verified$/, "").trim() });
      continue;
    }
    if (line.startsWith("if ")) {
      r.verified.push({ raw: line.slice(3).trim() });
      continue;
    }
    const parts = line.split(" : ");
    if (parts.length === 2) {
      r.premises.push({ varName: parts[0].trim(), typeName: parts[1].trim() });
    }
  }

  const concStr = concLines.join(" ").replace(/;$/, "").trim();
  if (concStr.includes(">>")) return null;
  const parts = concStr.split(" : ");
  if (parts.length !== 2) return null;

  const [lhs, rhs] = [parts[0].trim(), parts[1].trim()];
  r.conc.typeName = rhs;
  if (lhs.startsWith("[") && lhs.endsWith("]")) {
    r.conc.fields = lhs.slice(1, -1).trim().split(/\s+/);
  } else {
    r.conc.fields = [lhs];
    r.conc.isWrapped = true;
  }
  return r;
}

// ============================================================================
// Helpers
// ============================================================================

export function shenTypeToTs(t: string): string {
  // Handle parameterized types like (list search-hit) → SearchHit[]
  const listMatch = t.match(/^\(list\s+(.+)\)$/);
  if (listMatch) {
    return shenTypeToTs(listMatch[1]) + "[]";
  }
  switch (t) {
    case "string":
    case "symbol":
      return "string";
    case "number":
      return "number";
    case "boolean":
      return "boolean";
    case "":
      return "unknown";
    default:
      return toPascalCase(t);
  }
}

export function toPascalCase(s: string): string {
  return s
    .split(/[-_]/)
    .map((p) => (p.length > 0 ? p[0].toUpperCase() + p.slice(1) : ""))
    .join("");
}

export function toCamelCase(s: string): string {
  const pc = toPascalCase(s);
  return pc.length > 0 ? pc[0].toLowerCase() + pc.slice(1) : pc;
}

export function isPrimitive(t: string): boolean {
  return t === "string" || t === "number" || t === "boolean" || t === "symbol";
}

export function isNumericLiteral(s: string): boolean {
  if (!s) return false;
  return !isNaN(parseFloat(s)) && isFinite(Number(s));
}

// ============================================================================
// TypeScript Code Generator
// ============================================================================

export interface GenerateOptions {
  // Logical package name — e.g. "shenguard" or "guards". TypeScript has no
  // first-class package declaration, so this is documented in the header
  // comment only; consumers pick the import binding at their import site
  // (e.g. `import * as shenguard from "./guards.ts"`). See §2.3 of the
  // shengen-ts parity handoff for the design choice.
  pkg?: string;
}

export function generateTs(
  types: Datatype[],
  st: SymbolTable,
  specPath: string,
  options: GenerateOptions = {}
): string {
  const lines: string[] = [];
  lines.push(`// Code generated by shengen-ts from ${specPath}. DO NOT EDIT.`);
  if (options.pkg) {
    lines.push(`// Logical package: ${options.pkg}`);
    lines.push(`// (TS has no package declaration — import as \`import * as ${options.pkg} from "./…"\`.)`);
  }
  lines.push("//");
  lines.push("// These types enforce Shen sequent-calculus invariants at the TypeScript level.");
  lines.push("// Constructors are the ONLY way to create these types — bypassing them");
  lines.push("// is a violation of the formal spec.");
  lines.push("");

  // Generate sum type unions.
  const sumTypeVariants = new Set<string>();
  for (const [concType, variants] of st.sumTypes) {
    const tsIface = toPascalCase(concType);
    const variantTypes = variants.map((v) => toPascalCase(v));
    lines.push(`// --- ${tsIface} (sum type) ---`);
    lines.push(`// Multiple Shen datatype blocks produce this type.`);
    lines.push(`// Variants: ${variants.join(", ")}`);
    lines.push(`export type ${tsIface} = ${variantTypes.join(" | ")};`);
    lines.push("");
    for (const v of variants) sumTypeVariants.add(v);
  }

  for (const dt of types) {
    for (const gt of classify(dt, st)) {
      lines.push(`// --- ${gt.tsName} ---`);
      lines.push(`// Shen: (datatype ${gt.name})`);
      switch (gt.category) {
        case "wrapper":
          lines.push(...genWrapper(gt));
          break;
        case "constrained":
          lines.push(...genConstrained(gt, st));
          break;
        case "composite":
          lines.push(...genComposite(gt));
          break;
        case "guarded":
          lines.push(...genGuarded(gt, st));
          break;
        case "alias":
          lines.push(...genAlias(gt));
          break;
      }
      lines.push("");
    }
  }

  lines.push(...generateDefineHelpers(st));
  return lines.join("\n") + "\n";
}

export interface GeneratedType {
  // Block name — always `dt.name`. Kept for callers that care about provenance.
  name: string;
  // Post-rename Shen-canonical name: the one that tsName was built from.
  // For sum-type variants this is `dt.name`; for plain blocks it's the
  // conclusion type name. This is the right source of truth for naming
  // free-function helpers like `mustAmount`.
  shenName: string;
  tsName: string;
  category: string;
  rule: Rule;
}

export function classify(dt: Datatype, st: SymbolTable): GeneratedType[] {
  const out: GeneratedType[] = [];
  for (const r of dt.rules) {
    let typeName = r.conc.typeName;
    if (dt.name !== typeName && (st.concCount.get(typeName) ?? 0) > 1) {
      typeName = dt.name;
    }
    const info = st.lookup(typeName);
    out.push({
      name: dt.name,
      shenName: typeName,
      tsName: toPascalCase(typeName),
      category: info?.category ?? "composite",
      rule: r,
    });
  }
  return out;
}

// mustName converts a Shen datatype name into its free-function helper name,
// matching the Go shen-derive emission (`mustAmount`, `mustAccountId`, …).
function mustName(shenName: string): string {
  return "must" + toPascalCase(shenName);
}

// genTryCreateBody is the common factory-shape both tryCreate and createOrThrow
// share: tryCreate is the error-returning variant, createOrThrow throws.
function genTryCreate(className: string, paramsStr: string, argNames: string[]): string[] {
  const argList = argNames.join(", ");
  return [
    `  static tryCreate(${paramsStr}): ${className} | Error {`,
    `    try { return ${className}.createOrThrow(${argList}); }`,
    `    catch (e) { return e instanceof Error ? e : new Error(String(e)); }`,
    `  }`,
  ];
}

// genMust emits the free-function helper that wraps createOrThrow — the entry
// point shen-derive-ts's generated tests reach for via `shenguard.mustXxx(...)`.
function genMust(
  shenName: string,
  className: string,
  paramsStr: string,
  argNames: string[]
): string[] {
  const argList = argNames.join(", ");
  return [
    `export function ${mustName(shenName)}(${paramsStr}): ${className} {`,
    `  return ${className}.createOrThrow(${argList});`,
    `}`,
  ];
}

function genWrapper(gt: GeneratedType): string[] {
  const tsType = shenTypeToTs(gt.rule.premises[0].typeName);
  const paramsStr = `x: ${tsType}`;
  return [
    `export class ${gt.tsName} {`,
    `  private readonly _v: ${tsType};`,
    `  private constructor(v: ${tsType}) { this._v = v; }`,
    `  static createOrThrow(x: ${tsType}): ${gt.tsName} { return new ${gt.tsName}(x); }`,
    ...genTryCreate(gt.tsName, paramsStr, ["x"]),
    `  val(): ${tsType} { return this._v; }`,
    ...(tsType === "string" ? [`  toString(): string { return this._v; }`] : []),
    `}`,
    ...genMust(gt.shenName, gt.tsName, paramsStr, ["x"]),
  ];
}

function genConstrained(gt: GeneratedType, st: SymbolTable): string[] {
  const tsType = shenTypeToTs(gt.rule.premises[0].typeName);
  const varMap = new Map(gt.rule.premises.map((p) => [p.varName, p.typeName]));
  const checks: string[] = [];
  for (const v of gt.rule.verified) {
    const [code, msg] = verifiedToTs(st, v, varMap);
    checks.push(`    if (!(${code})) throw new Error(\`${msg.replace(/`/g, "\\`")}: \${x}\`);`);
  }
  const paramsStr = `x: ${tsType}`;
  return [
    `export class ${gt.tsName} {`,
    `  private readonly _v: ${tsType};`,
    `  private constructor(v: ${tsType}) { this._v = v; }`,
    `  static createOrThrow(x: ${tsType}): ${gt.tsName} {`,
    ...checks,
    `    return new ${gt.tsName}(x);`,
    `  }`,
    ...genTryCreate(gt.tsName, paramsStr, ["x"]),
    `  val(): ${tsType} { return this._v; }`,
    `}`,
    ...genMust(gt.shenName, gt.tsName, paramsStr, ["x"]),
  ];
}

function genComposite(gt: GeneratedType): string[] {
  const fields = gt.rule.premises.map((p) => ({
    name: toCamelCase(p.varName),
    type: shenTypeToTs(p.typeName),
  }));
  const params = fields.map((f) => `${f.name}: ${f.type}`).join(", ");
  const argNames = fields.map((f) => f.name);
  const assigns = fields.map((f) => `    this._${f.name} = ${f.name};`);
  const accessors = fields.map(
    (f) => `  ${f.name}(): ${f.type} { return this._${f.name}; }`
  );
  return [
    `export class ${gt.tsName} {`,
    ...fields.map((f) => `  private readonly _${f.name}: ${f.type};`),
    `  private constructor(${params}) {`,
    ...assigns,
    `  }`,
    `  static createOrThrow(${params}): ${gt.tsName} {`,
    `    return new ${gt.tsName}(${argNames.join(", ")});`,
    `  }`,
    ...genTryCreate(gt.tsName, params, argNames),
    ...accessors,
    `}`,
    ...genMust(gt.shenName, gt.tsName, params, argNames),
  ];
}

function genGuarded(gt: GeneratedType, st: SymbolTable): string[] {
  const fields = gt.rule.premises.map((p) => ({
    name: toCamelCase(p.varName),
    type: shenTypeToTs(p.typeName),
    shenType: p.typeName,
  }));
  const params = fields.map((f) => `${f.name}: ${f.type}`).join(", ");
  const argNames = fields.map((f) => f.name);
  const assigns = fields.map((f) => `    this._${f.name} = ${f.name};`);
  const accessors = fields.map(
    (f) => `  ${f.name}(): ${f.type} { return this._${f.name}; }`
  );
  const varMap = new Map(gt.rule.premises.map((p) => [p.varName, p.typeName]));
  const checks: string[] = [];
  for (const v of gt.rule.verified) {
    const [code, msg] = verifiedToTs(st, v, varMap);
    checks.push(`    if (!(${code})) throw new Error(\`${msg.replace(/`/g, "\\`")}\`);`);
  }
  return [
    `export class ${gt.tsName} {`,
    ...fields.map((f) => `  private readonly _${f.name}: ${f.type};`),
    `  private constructor(${params}) {`,
    ...assigns,
    `  }`,
    `  static createOrThrow(${params}): ${gt.tsName} {`,
    ...checks,
    `    return new ${gt.tsName}(${argNames.join(", ")});`,
    `  }`,
    ...genTryCreate(gt.tsName, params, argNames),
    ...accessors,
    `}`,
    ...genMust(gt.shenName, gt.tsName, params, argNames),
  ];
}

function genAlias(gt: GeneratedType): string[] {
  return [
    `export type ${gt.tsName} = ${shenTypeToTs(gt.rule.premises[0].typeName)};`,
  ];
}

// ============================================================================
// (define …) body translation
// ============================================================================
//
// Scope of this port (minimum viable, §2.1 of the parity handoff):
//   - Single-clause defines with all-variable patterns emit cleanly.
//   - Multi-clause defines with literal patterns (string, number, bool, "")
//     plus a fallthrough variable clause emit an if-chain.
//   - Destructuring patterns (`[H | T]`, `[[X Y] | Rest]`) are not yet
//     translated — they emit a TODO stub.
//   - Guards (`where (pred ...)`) are not yet translated either.
//
// Body translation covers: comparisons (>=, <=, >, <, =, !=), arithmetic
// (+, -, *, /, %, shen.mod), boolean (and, or, not, if), list ops (head,
// tail, length, cons, concat, empty?), wrapper unwrap (val), higher-order
// (lambda, foldr, foldl, scanl, map, filter). User-defined calls dispatch
// to generated helpers (camelCase, `?` trimmed) or user-provided impl
// functions (same shape — users must define any unreferenced symbols).

function definePascalName(shenName: string): string {
  return toCamelCase(shenName.replace(/\?$/, ""));
}

function translateDefineBodyRaw(
  raw: string,
  varMap: Map<string, string>,
  st: SymbolTable
): string {
  const trimmed = raw.trim();
  // Raw atoms bypass parseSExpr so literal strings keep their quotes verbatim.
  if (!trimmed.startsWith("(")) {
    return translateDefineAtom(trimmed, varMap, st);
  }
  const expr = parseSExpr(raw);
  return translateDefineExpr(expr, varMap, st);
}

function translateDefineAtom(
  atom: string,
  varMap: Map<string, string>,
  _st: SymbolTable
): string {
  if (atom === "true" || atom === "false") return atom;
  if (atom === "[]") return "[]";
  if (isNumericLiteral(atom)) return atom;
  if (/^".*"$/.test(atom)) return atom;
  if (varMap.has(atom)) return toCamelCase(atom);
  // Unknown identifier — emit as-is (user-provided dep or later-registered name).
  return toCamelCase(atom);
}

function translateDefineExpr(
  expr: SExpr,
  varMap: Map<string, string>,
  st: SymbolTable
): string {
  if (!isCall(expr)) {
    return translateDefineAtom(expr.atom ?? "", varMap, st);
  }
  const fname = op(expr);
  const args = expr.children!.slice(1);
  const tx = (e: SExpr): string => translateDefineExpr(e, varMap, st);

  switch (fname) {
    case ">=":
    case "<=":
    case ">":
    case "<":
      return `(${tx(args[0])} ${fname} ${tx(args[1])})`;
    case "=":
      return `(${tx(args[0])} === ${tx(args[1])})`;
    case "!=":
      return `(${tx(args[0])} !== ${tx(args[1])})`;
    case "and":
      return `(${tx(args[0])} && ${tx(args[1])})`;
    case "or":
      return `(${tx(args[0])} || ${tx(args[1])})`;
    case "not":
      return `!(${tx(args[0])})`;
    case "+":
    case "-":
    case "*":
    case "/":
    case "%":
      return `(${tx(args[0])} ${fname} ${tx(args[1])})`;
    case "shen.mod":
      return `(Math.trunc(${tx(args[0])}) % ${tx(args[1])})`;
    case "if":
      return `(${tx(args[0])} ? ${tx(args[1])} : ${tx(args[2])})`;
    case "head":
      return `${tx(args[0])}[0]`;
    case "tail":
      return `${tx(args[0])}.slice(1)`;
    case "length":
      return `${tx(args[0])}.length`;
    case "cons":
      return `[${tx(args[0])}, ...${tx(args[1])}]`;
    case "concat":
      return `${tx(args[0])}.concat(${tx(args[1])})`;
    case "empty?":
      return `(${tx(args[0])}.length === 0)`;
    case "val":
      return `${tx(args[0])}.val()`;
    case "lambda": {
      // (lambda X body) → (x: any) => <body>
      if (args.length !== 2 || !isAtom(args[0])) {
        return "/* bad lambda */ () => undefined";
      }
      const param = args[0].atom ?? "";
      const inner = new Map(varMap);
      inner.set(param, "any"); // propagate binding so body translator recognizes it
      const body = translateDefineExpr(args[1], inner, st);
      return `(${toCamelCase(param)}: any) => ${body}`;
    }
    case "foldr":
      // Shen foldr is right-fold with curried f: f(x)(acc). Wrap the f
      // translation in parens so an inline lambda doesn't bind to the outer
      // arrow's RHS.
      return `${tx(args[2])}.reduceRight((acc: any, x: any) => (${tx(args[0])})(x)(acc), ${tx(args[1])})`;
    case "foldl":
      return `${tx(args[2])}.reduce((acc: any, x: any) => (${tx(args[0])})(acc)(x), ${tx(args[1])})`;
    case "scanl":
      return `__scanl(${tx(args[0])}, ${tx(args[1])}, ${tx(args[2])})`;
    case "map":
      return `${tx(args[1])}.map((x: any) => (${tx(args[0])})(x))`;
    case "filter":
      return `${tx(args[1])}.filter((x: any) => (${tx(args[0])})(x))`;
  }

  // User-defined predicate / function call, or an accessor-style call like
  // (amount Tx) meaning Tx.amount(). Accessor dispatch: if there's exactly one
  // argument, that argument is a known-type variable, and fname matches a
  // field accessor on that type — emit method-call form.
  if (args.length === 1 && isAtom(args[0])) {
    const argAtom = args[0].atom ?? "";
    const argType = varMap.get(argAtom);
    if (argType) {
      const info = st.lookup(argType);
      if (info) {
        const target = info.fields.find(
          (f) => toCamelCase(f.shenName) === toCamelCase(fname)
        );
        if (target) {
          return `${toCamelCase(argAtom)}.${toCamelCase(target.shenName)}()`;
        }
      }
    }
  }

  // Otherwise: emit a function call. Both user-defined helpers and unresolved
  // references share this shape; at type-check time, unresolved names turn
  // into TS errors that the user satisfies by providing the impl.
  const callee = definePascalName(fname);
  return `${callee}(${args.map(tx).join(", ")})`;
}

// generateDefineHelpers emits one TS function per (define …) in the symbol
// table's registry. Returns the helper source plus a `__scanl` prelude when
// any define uses scanl.
function generateDefineHelpers(st: SymbolTable): string[] {
  if (st.defines.size === 0) return [];
  const lines: string[] = [];
  const bodies: string[] = [];
  let needsScanl = false;

  for (const [, def] of st.defines) {
    const emitted = generateOneDefine(def, st);
    if (emitted.usesScanl) needsScanl = true;
    bodies.push(...emitted.lines);
  }

  lines.push("// --- helpers derived from (define …) blocks ---");
  if (needsScanl) {
    lines.push(
      "// scanl: left-scan matching Shen semantics. Emits intermediate",
      "// accumulator values, returning [init, f(init, l[0]), f(f(init, l[0]), l[1]), …].",
      "function __scanl<A, T>(f: (a: A) => (t: T) => A, init: A, list: T[]): A[] {",
      "  const out: A[] = [init];",
      "  let acc: A = init;",
      "  for (const x of list) { acc = f(acc)(x); out.push(acc); }",
      "  return out;",
      "}"
    );
  }
  lines.push("");
  lines.push(...bodies);
  return lines;
}

interface DefineEmission {
  lines: string[];
  usesScanl: boolean;
}

function generateOneDefine(def: Define, st: SymbolTable): DefineEmission {
  const lines: string[] = [];
  const tsName = definePascalName(def.name);

  // Param count is the maximum across clauses. For well-formed defines it
  // matches the signature length - 1 (return is the last entry).
  const paramCount = Math.max(...def.clauses.map((c) => c.patterns.length), 0);
  const sigParams = def.signature.slice(0, Math.max(def.signature.length - 1, 0));
  const returnShen =
    def.signature.length > 0 ? def.signature[def.signature.length - 1] : "";
  const returnTs = returnShen ? shenTypeToTs(returnShen) : "any";

  // Choose a canonical param name per index: first clause whose pattern there
  // is a plain variable (uppercase binder). Fall back to arg<i>.
  const paramNames: string[] = [];
  for (let i = 0; i < paramCount; i++) {
    let chosen: string | null = null;
    for (const c of def.clauses) {
      const pat = c.patterns[i];
      if (!pat) continue;
      if (
        pat !== "_" &&
        !pat.startsWith("[") &&
        pat !== "[]" &&
        !/^".*"$/.test(pat) &&
        pat !== "true" &&
        pat !== "false" &&
        !isNumericLiteral(pat)
      ) {
        chosen = pat;
        break;
      }
    }
    paramNames.push(chosen ? toCamelCase(chosen) : `arg${i}`);
  }

  const paramSig = paramNames
    .map((p, i) => `${p}: ${sigParams[i] ? shenTypeToTs(sigParams[i]) : "any"}`)
    .join(", ");

  lines.push(`// ${tsName} is generated from Shen define ${def.name}`);
  lines.push(`export function ${tsName}(${paramSig}): ${returnTs} {`);

  let usesScanl = false;

  // If every clause has all-variable patterns, fuse them into a single body —
  // they're semantically identical as long as the variable names align.
  const allVarPatterns = def.clauses.every((c) =>
    c.patterns.every(
      (p) =>
        p === "_" ||
        (!p.startsWith("[") &&
          !/^".*"$/.test(p) &&
          p !== "true" &&
          p !== "false" &&
          !isNumericLiteral(p))
    )
  );

  if (allVarPatterns && def.clauses.length === 1) {
    const clause = def.clauses[0];
    const varMap = new Map<string, string>();
    for (let i = 0; i < clause.patterns.length; i++) {
      const pat = clause.patterns[i];
      if (pat === "_") continue;
      const shenTy = sigParams[i] ?? "any";
      varMap.set(pat, shenTy);
      // Alias the Shen-cased name to the canonical TS param so body references
      // translate cleanly. Only needed when the names diverge after camelCase.
      if (toCamelCase(pat) !== paramNames[i]) {
        lines.push(`  const ${toCamelCase(pat)} = ${paramNames[i]};`);
      }
    }
    const bodySrc = clause.result;
    if (/scanl/.test(bodySrc)) usesScanl = true;
    const body = translateDefineBodyRaw(bodySrc, varMap, st);
    lines.push(`  return ${body};`);
  } else {
    // Multi-clause: emit an if-chain guarded by literal pattern checks. If a
    // clause's pattern for any position isn't a recognized literal/variable,
    // fall back to a TODO stub.
    let fellThrough = false;
    for (const clause of def.clauses) {
      const checks: string[] = [];
      const varMap = new Map<string, string>();
      let supported = true;

      for (let i = 0; i < clause.patterns.length; i++) {
        const pat = clause.patterns[i];
        const paramName = paramNames[i];
        if (pat === "_" ) continue;
        if (pat.startsWith("[") || pat === "[]") {
          supported = false;
          break;
        }
        if (
          /^".*"$/.test(pat) ||
          pat === "true" ||
          pat === "false" ||
          isNumericLiteral(pat)
        ) {
          checks.push(`${paramName} === ${pat}`);
          continue;
        }
        // Variable pattern — alias if distinct from param.
        const shenTy = sigParams[i] ?? "any";
        varMap.set(pat, shenTy);
        if (toCamelCase(pat) !== paramName) {
          // Surfaced as a `const` below.
        }
      }

      if (!supported) {
        lines.push(
          `  // TODO: destructuring clause — patterns=${JSON.stringify(clause.patterns)} result=${JSON.stringify(clause.result)}`
        );
        fellThrough = true;
        continue;
      }

      // Emit aliases for variable bindings whose name differs from the param.
      const aliases: string[] = [];
      for (let i = 0; i < clause.patterns.length; i++) {
        const pat = clause.patterns[i];
        const paramName = paramNames[i];
        if (
          pat === "_" ||
          pat.startsWith("[") ||
          /^".*"$/.test(pat) ||
          pat === "true" ||
          pat === "false" ||
          isNumericLiteral(pat)
        ) {
          continue;
        }
        if (toCamelCase(pat) !== paramName) {
          aliases.push(`    const ${toCamelCase(pat)} = ${paramName};`);
        }
      }

      const bodySrc = clause.result;
      if (/scanl/.test(bodySrc)) usesScanl = true;
      const body = translateDefineBodyRaw(bodySrc, varMap, st);

      if (checks.length === 0) {
        // Variable-only (catch-all) clause.
        lines.push(...aliases);
        lines.push(`  return ${body};`);
        fellThrough = false;
        break;
      }
      lines.push(`  if (${checks.join(" && ")}) {`);
      lines.push(...aliases);
      lines.push(`    return ${body};`);
      lines.push(`  }`);
    }
    if (fellThrough) {
      // If the only clauses that fit were guarded, emit a throw so the
      // function is well-typed and surfaces the gap at runtime.
      lines.push(
        `  throw new Error("shengen-ts: unhandled pattern in ${def.name} (destructuring not yet translated)");`
      );
    }
  }

  lines.push(`}`);
  lines.push("");
  return { lines, usesScanl };
}

// ============================================================================
// Main
// ============================================================================

function printSymbolTable(types: Datatype[], st: SymbolTable, path: string): void {
  process.stderr.write(`Parsed ${types.length} datatypes from ${path}\n\n`);
  process.stderr.write("Symbol table:\n");
  for (const dt of types) {
    for (const r of dt.rules) {
      let typeName = r.conc.typeName;
      if (dt.name !== typeName && (st.concCount.get(typeName) ?? 0) > 1) {
        typeName = dt.name;
      }
      const info = st.lookup(typeName);
      if (!info) continue;
      const label =
        dt.name !== typeName
          ? `${typeName} (block: ${dt.name})`
          : typeName;
      let line = `  ${label.padEnd(28)} [${info.category.padEnd(11)}]`;
      if (info.fields.length > 0) {
        const fs = info.fields.map((f) => `${f.shenName}:${f.shenType}`).join(", ");
        line += ` {${fs}}`;
      }
      if (info.wrappedPrim) line += ` wraps=${info.wrappedPrim}`;
      if (info.wrappedType) line += ` alias=${info.wrappedType}`;
      process.stderr.write(line + "\n");
    }
  }
  process.stderr.write("\n");
}

// CLI — only run when invoked as the entry script (not when imported from tests).
function main(): void {
  const args = process.argv.slice(2);
  const specPaths: string[] = [];
  let outFile: string | null = null;
  let dryRun = false;
  let pkg: string | undefined;

  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--spec" && i + 1 < args.length) {
      specPaths.push(args[++i]);
    } else if (args[i] === "--out" && i + 1 < args.length) {
      outFile = args[++i];
    } else if (args[i] === "--pkg" && i + 1 < args.length) {
      pkg = args[++i];
    } else if (args[i] === "--dry-run") {
      dryRun = true;
    } else if (!args[i].startsWith("--")) {
      specPaths.push(args[i]);
    }
  }
  if (specPaths.length === 0) specPaths.push("specs/core.shen");

  const specGroups = specPaths.map((path) => ({
    path,
    spec: parseSpecFile(path),
  }));
  const spec = mergeSpecs(specGroups);
  const types = spec.datatypes;
  const st = new SymbolTable();
  st.build(types);
  st.registerDefines(spec.defines);

  const originLabel = specPaths.length === 1 ? specPaths[0] : specPaths.join(", ");
  printSymbolTable(types, st, originLabel);

  if (!dryRun) {
    const output = generateTs(types, st, originLabel, { pkg });
    if (outFile) {
      writeFileSync(outFile, output);
      process.stderr.write(`Generated ${outFile} from ${originLabel}\n`);
    } else {
      process.stdout.write(output);
    }
  }
}

const entry = process.argv[1] ? fileURLToPath(import.meta.url) === process.argv[1] : false;
if (entry) {
  main();
}
