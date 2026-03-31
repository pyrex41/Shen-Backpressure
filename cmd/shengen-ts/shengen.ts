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

// ============================================================================
// AST
// ============================================================================

interface Premise {
  varName: string;
  typeName: string;
}

interface VerifiedPremise {
  raw: string;
}

interface Conclusion {
  fields: string[];
  typeName: string;
  isWrapped: boolean;
}

interface Rule {
  premises: Premise[];
  verified: VerifiedPremise[];
  conc: Conclusion;
}

interface Datatype {
  name: string;
  rules: Rule[];
}

// ============================================================================
// Symbol Table
// ============================================================================

interface FieldInfo {
  index: number;
  shenName: string;
  shenType: string;
}

interface TypeInfo {
  shenName: string;
  tsName: string;
  category: "wrapper" | "constrained" | "composite" | "guarded" | "alias" | "sumtype";
  fields: FieldInfo[];
  wrappedPrim: string | null;
  wrappedType: string | null;
}

class SymbolTable {
  types: Map<string, TypeInfo> = new Map();
  concCount: Map<string, number> = new Map();
  sumTypes: Map<string, string[]> = new Map();

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
          !isPrimitive(prems[0].typeName)
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

interface SExpr {
  atom: string | null;
  children: SExpr[] | null;
}

function isAtom(e: SExpr): boolean {
  return e.atom !== null;
}
function isCall(e: SExpr): boolean {
  return e.children !== null && e.children.length > 0;
}
function op(e: SExpr): string {
  if (isCall(e) && e.children![0].atom) return e.children![0].atom;
  return "";
}
function sexprToString(e: SExpr): string {
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

function parseSExpr(input: string): SExpr {
  const tokens = tokenize(input.trim());
  if (tokens.length === 0) return { atom: "", children: null };
  const [expr] = parseTokens(tokens, 0);
  return expr;
}

// ============================================================================
// Accessor Chain Resolution
// ============================================================================

interface ResolvedExpr {
  code: string;
  tsType: string;
  shenType: string;
  isMulti?: boolean;
  fields?: FieldInfo[];
}

function resolveExpr(
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

function verifiedToTs(
  st: SymbolTable,
  v: VerifiedPremise,
  varMap: Map<string, string>
): [string, string] {
  const expr = parseSExpr(v.raw);
  if (!isCall(expr)) return [`/* TODO: ${v.raw} */ true`, "unhandled"];

  switch (op(expr)) {
    case ">=":
    case "<=":
    case ">":
    case "<":
      return translateCmp(st, expr, varMap, op(expr));
    case "=":
      return translateEq(st, expr, varMap);
    case "not":
      return translateNot(st, expr, varMap);
    case "element?":
      return translateElement(st, expr, varMap);
  }
  return [`/* TODO: ${v.raw} */ true`, `unhandled op: ${op(expr)}`];
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

function structuralMatchFallback(
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
  if (!lhsInfo || !rhsInfo) return null;

  for (const lf of lhsInfo.fields) {
    for (const rf of rhsInfo.fields) {
      if (lf.shenType === rf.shenType && !isPrimitive(lf.shenType)) {
        const l = `${toCamelCase(lhsVar)}.${toCamelCase(lf.shenName)}()`;
        const r = `${toCamelCase(rhsVar)}.${toCamelCase(rf.shenName)}()`;
        return [
          `${l} === ${r}`,
          `${toCamelCase(lhsVar)}.${toCamelCase(lf.shenName)} must equal ${toCamelCase(rhsVar)}.${toCamelCase(rf.shenName)}`,
        ];
      }
    }
  }
  return null;
}

// ============================================================================
// Parser
// ============================================================================

function parseFile(path: string): Datatype[] {
  let content = readFileSync(path, "utf-8");
  const types: Datatype[] = [];

  while (true) {
    const idx = content.indexOf("(datatype ");
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

    const block = content.slice(0, end);
    content = content.slice(end);
    const dt = parseDatatype(block);
    if (dt) types.push(dt);
  }
  return types;
}

function parseDatatype(block: string): Datatype | null {
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

function shenTypeToTs(t: string): string {
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

function toPascalCase(s: string): string {
  return s
    .split(/[-_]/)
    .map((p) => (p.length > 0 ? p[0].toUpperCase() + p.slice(1) : ""))
    .join("");
}

function toCamelCase(s: string): string {
  const pc = toPascalCase(s);
  return pc.length > 0 ? pc[0].toLowerCase() + pc.slice(1) : pc;
}

function isPrimitive(t: string): boolean {
  return t === "string" || t === "number" || t === "boolean" || t === "symbol";
}

function isNumericLiteral(s: string): boolean {
  if (!s) return false;
  return !isNaN(parseFloat(s)) && isFinite(Number(s));
}

// ============================================================================
// TypeScript Code Generator
// ============================================================================

function generateTs(types: Datatype[], st: SymbolTable, specPath: string): string {
  const lines: string[] = [];
  lines.push(`// Code generated by shengen-ts from ${specPath}. DO NOT EDIT.`);
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
  return lines.join("\n") + "\n";
}

interface GeneratedType {
  name: string;
  tsName: string;
  category: string;
  rule: Rule;
}

function classify(dt: Datatype, st: SymbolTable): GeneratedType[] {
  const out: GeneratedType[] = [];
  for (const r of dt.rules) {
    let typeName = r.conc.typeName;
    if (dt.name !== typeName && (st.concCount.get(typeName) ?? 0) > 1) {
      typeName = dt.name;
    }
    const info = st.lookup(typeName);
    out.push({
      name: dt.name,
      tsName: toPascalCase(typeName),
      category: info?.category ?? "composite",
      rule: r,
    });
  }
  return out;
}

function genWrapper(gt: GeneratedType): string[] {
  const tsType = shenTypeToTs(gt.rule.premises[0].typeName);
  return [
    `export class ${gt.tsName} {`,
    `  private readonly _v: ${tsType};`,
    `  private constructor(v: ${tsType}) { this._v = v; }`,
    `  static create(x: ${tsType}): ${gt.tsName} { return new ${gt.tsName}(x); }`,
    `  val(): ${tsType} { return this._v; }`,
    ...(tsType === "string" ? [`  toString(): string { return this._v; }`] : []),
    `}`,
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
  return [
    `export class ${gt.tsName} {`,
    `  private readonly _v: ${tsType};`,
    `  private constructor(v: ${tsType}) { this._v = v; }`,
    `  static create(x: ${tsType}): ${gt.tsName} {`,
    ...checks,
    `    return new ${gt.tsName}(x);`,
    `  }`,
    `  val(): ${tsType} { return this._v; }`,
    `}`,
  ];
}

function genComposite(gt: GeneratedType): string[] {
  const fields = gt.rule.premises.map((p) => ({
    name: toCamelCase(p.varName),
    type: shenTypeToTs(p.typeName),
  }));
  const params = fields.map((f) => `${f.name}: ${f.type}`).join(", ");
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
    `  static create(${params}): ${gt.tsName} {`,
    `    return new ${gt.tsName}(${fields.map((f) => f.name).join(", ")});`,
    `  }`,
    ...accessors,
    `}`,
  ];
}

function genGuarded(gt: GeneratedType, st: SymbolTable): string[] {
  const fields = gt.rule.premises.map((p) => ({
    name: toCamelCase(p.varName),
    type: shenTypeToTs(p.typeName),
    shenType: p.typeName,
  }));
  const params = fields.map((f) => `${f.name}: ${f.type}`).join(", ");
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
    `  static create(${params}): ${gt.tsName} {`,
    ...checks,
    `    return new ${gt.tsName}(${fields.map((f) => f.name).join(", ")});`,
    `  }`,
    ...accessors,
    `}`,
  ];
}

function genAlias(gt: GeneratedType): string[] {
  return [
    `export type ${gt.tsName} = ${shenTypeToTs(gt.rule.premises[0].typeName)};`,
  ];
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

// CLI
const args = process.argv.slice(2);
let specPath = "specs/core.shen";
let outFile: string | null = null;
let dryRun = false;

for (let i = 0; i < args.length; i++) {
  if (args[i] === "--out" && i + 1 < args.length) {
    outFile = args[++i];
  } else if (args[i] === "--dry-run") {
    dryRun = true;
  } else if (!args[i].startsWith("--")) {
    specPath = args[i];
  }
}

const types = parseFile(specPath);
const st = new SymbolTable();
st.build(types);

printSymbolTable(types, st, specPath);

if (!dryRun) {
  const output = generateTs(types, st, specPath);
  if (outFile) {
    writeFileSync(outFile, output);
    process.stderr.write(`Generated ${outFile} from ${specPath}\n`);
  } else {
    process.stdout.write(output);
  }
}
