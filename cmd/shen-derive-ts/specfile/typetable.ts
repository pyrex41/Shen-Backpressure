// Shen→TS type classifier. Mirrors shen-derive/specfile/typetable.go.
//
// Given the parsed datatypes from a .shen spec, classify each Shen type by
// its shape (wrapper, constrained, composite, guarded, alias, sumtype) and
// produce a lookup table used by the verifier and code generator.
//
// This file deliberately does NOT share code with cmd/shengen-ts. The two
// implementations are parallel, matching how the Go `specfile` package is
// parallel to `cmd/shengen/main.go`.

import type { Datatype } from "./parse.ts";

export type TypeCategory =
  | "wrapper"
  | "constrained"
  | "composite"
  | "guarded"
  | "alias"
  | "sumtype";

export type Field = {
  /** camelCase TS accessor name (e.g. "amount", "from") */
  name: string;
  /** original Shen variable name as written in the spec (e.g. "Amount") */
  shenName: string;
  /** Shen type name from the matching premise (e.g. "amount") */
  typeName: string;
  /** TS type string (e.g. "number", "Foo[]") */
  tsType: string;
};

export type TypeEntry = {
  shenName: string;
  /** PascalCase class name (e.g. "AccountId") */
  tsName: string;
  category: TypeCategory;
  /** Underlying primitive Shen type for wrappers/constrained; "" otherwise. */
  shenPrim: string;
  /** TS type string for the underlying primitive; "" otherwise. */
  tsPrimType: string;
  /** Raw verified predicate strings, e.g. "(>= X 0)". */
  verified: string[];
  /** Variable name used in verified predicates (e.g. "X"). */
  varName: string;
  /** Fields for composite/guarded types. Empty otherwise. */
  fields: Field[];
  /** Import specifier for the shengen-ts generated module. */
  importPath: string;
  /** Namespace alias used when qualifying imported types. */
  importAlias: string;
};

export type TypeTable = Map<string, TypeEntry>;

// --- Public API ---

/**
 * Classify every datatype rule and return a table keyed by Shen type name.
 * When multiple rules share a conclusion type, a synthetic `sumtype` entry is
 * registered under that conclusion name and the individual rules are stored
 * under their block names as sum variants.
 */
export function buildTypeTable(
  datatypes: Datatype[],
  importPath: string,
  importAlias: string,
): TypeTable {
  const tt: TypeTable = new Map();

  // Pass 1: count how many rules produce each conclusion type name. A count
  // greater than one means "this conclusion is a sum type with N variants".
  const concCount = new Map<string, number>();
  for (const dt of datatypes) {
    for (const r of dt.rules) {
      concCount.set(
        r.conclusion.typeName,
        (concCount.get(r.conclusion.typeName) ?? 0) + 1,
      );
    }
  }

  // Pass 2: classify each rule into an entry.
  for (const dt of datatypes) {
    for (const r of dt.rules) {
      let typeName = r.conclusion.typeName;
      const isSumVariant =
        dt.name !== r.conclusion.typeName &&
        (concCount.get(r.conclusion.typeName) ?? 0) > 1;
      if (isSumVariant) {
        typeName = dt.name;
      }

      const entry: TypeEntry = {
        shenName: typeName,
        tsName: toPascalCase(typeName),
        category: "composite",
        shenPrim: "",
        tsPrimType: "",
        verified: [],
        varName: "",
        fields: [],
        importPath,
        importAlias,
      };

      const prems = r.premises;
      const verified = r.verified;
      const conc = r.conclusion;
      // "wrapped" conclusions have the shape `X : type` (single LHS
      // variable); tuple-shaped conclusions `[A B C] : type` populate
      // `fields` instead.
      const isWrapped = conc.fields.length === 0;

      if (
        isWrapped &&
        verified.length === 0 &&
        prems.length === 1 &&
        isPrimitive(prems[0].typeName)
      ) {
        entry.category = "wrapper";
        entry.shenPrim = prems[0].typeName;
        entry.tsPrimType = shenPrimToTs(entry.shenPrim);
        entry.varName = prems[0].varName;
      } else if (
        isWrapped &&
        verified.length > 0 &&
        prems.length >= 1 &&
        isPrimitive(prems[0].typeName)
      ) {
        entry.category = "constrained";
        entry.shenPrim = prems[0].typeName;
        entry.tsPrimType = shenPrimToTs(entry.shenPrim);
        entry.varName = prems[0].varName;
      } else if (
        isWrapped &&
        prems.length === 1 &&
        !isPrimitive(prems[0].typeName) &&
        !isSumVariant
      ) {
        entry.category = "alias";
      } else if (!isWrapped && verified.length > 0) {
        entry.category = "guarded";
      } else {
        entry.category = "composite";
      }

      if (entry.category === "constrained" || entry.category === "guarded") {
        for (const v of verified) {
          entry.verified.push(v.raw);
        }
      }

      // Fields for composites (and sum variants with wrapped conclusions,
      // for symmetry with shengen).
      if (!isWrapped || isSumVariant) {
        const premMap = new Map<string, string>();
        for (const p of prems) {
          premMap.set(p.varName, p.typeName);
        }
        for (const fieldName of conc.fields) {
          const shenType = premMap.get(fieldName) ?? "unknown";
          entry.fields.push({
            name: toCamelCase(fieldName),
            shenName: fieldName,
            typeName: shenType,
            tsType: "", // filled in after the table is fully populated
          });
        }
      }

      tt.set(typeName, entry);
    }
  }

  // Pass 3: register synthetic sumtype entries for conclusions shared by
  // multiple rules.
  for (const dt of datatypes) {
    for (const r of dt.rules) {
      const conc = r.conclusion.typeName;
      if ((concCount.get(conc) ?? 0) > 1 && !tt.has(conc)) {
        tt.set(conc, {
          shenName: conc,
          tsName: toPascalCase(conc),
          category: "sumtype",
          shenPrim: "",
          tsPrimType: "",
          verified: [],
          varName: "",
          fields: [],
          importPath,
          importAlias,
        });
      }
    }
  }

  // Pass 4: now that every declared type is in the table, resolve field
  // tsType strings. This is deferred so that forward references resolve.
  for (const entry of tt.values()) {
    for (const f of entry.fields) {
      f.tsType = tsType(tt, f.typeName);
    }
  }

  return tt;
}

/**
 * Return the TS type expression for a Shen type. Handles `(list T)` by
 * recursing on the element type. Primitives map to TS built-ins; declared
 * types resolve to their PascalCase class name.
 */
export function tsType(tt: TypeTable, shenType: string): string {
  const t = shenType.trim();
  if (t.startsWith("(list ") && t.endsWith(")")) {
    const inner = t.slice("(list ".length, t.length - 1).trim();
    return tsType(tt, inner) + "[]";
  }
  if (isPrimitive(t)) {
    return shenPrimToTs(t);
  }
  const e = tt.get(t);
  if (e !== undefined) {
    return e.tsName;
  }
  return toPascalCase(t);
}

/**
 * Extract the element type from `(list T)`. Returns `""` if `shenType` is not
 * a list.
 */
export function elemType(shenType: string): string {
  const t = shenType.trim();
  if (t.startsWith("(list ") && t.endsWith(")")) {
    return t.slice("(list ".length, t.length - 1).trim();
  }
  return "";
}

// --- Helpers (ported fresh; not shared with shengen-ts) ---

function isPrimitive(t: string): boolean {
  return t === "string" || t === "number" || t === "boolean" || t === "symbol";
}

function shenPrimToTs(t: string): string {
  switch (t) {
    case "number":
      return "number";
    case "string":
    case "symbol":
      return "string";
    case "boolean":
      return "boolean";
    default:
      return "unknown";
  }
}

function splitIdent(s: string): string[] {
  return s.split(/[-_]/).filter((p) => p.length > 0);
}

function toPascalCase(s: string): string {
  return splitIdent(s)
    .map((p) => p[0].toUpperCase() + p.slice(1))
    .join("");
}

function toCamelCase(s: string): string {
  const parts = splitIdent(s);
  if (parts.length === 0) return "";
  const [head, ...rest] = parts;
  return (
    head[0].toLowerCase() +
    head.slice(1) +
    rest.map((p) => p[0].toUpperCase() + p.slice(1)).join("")
  );
}
