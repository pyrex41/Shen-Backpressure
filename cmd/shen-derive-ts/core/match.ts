// Pattern matcher for shen-derive. Ported from shen-derive/core/match.go.
//
// Matches a pattern (represented as a `Sexpr`) against a runtime `Value`
// produced by the evaluator. The `Value` union and its constructors live
// in `./eval.ts`; this module re-exports the handful of constructors
// pattern-matching consumers typically need so existing test imports
// keep working.

import type { Sexpr } from "./sexpr.ts";
import {
  type Value,
  type ListVal,
  valEqual,
} from "./eval.ts";

export {
  type Value,
  type IntVal,
  type FloatVal,
  type BoolVal,
  type StringVal,
  type ListVal,
  intVal,
  floatVal,
  boolVal,
  stringVal,
  listVal,
  valEqual,
} from "./eval.ts";

// --- Match result ---

export type MatchResult =
  | { kind: "matched"; bindings: Map<string, Value> }
  | { kind: "miss" }
  | { kind: "error"; msg: string };

// --- Match entry point ---
//
// Supported patterns:
//   - symbol "_"                — wildcard, binds nothing
//   - symbol "Uppercase..."     — variable binding
//   - symbol "nil"              — matches an empty list
//   - int / float / bool / str  — literal; matches by valEqual
//   - (cons head tail)          — cons pattern; matches a non-empty list
//
// Fixed-length list patterns like `[A B]` desugar to
// `(cons A (cons B nil))`, so the cons case is the only list form needed.
export function match(pat: Sexpr, v: Value): MatchResult {
  const bindings = new Map<string, Value>();
  const err = matchInto(pat, v, bindings);
  if (err === null) return { kind: "matched", bindings };
  if (err === NO_MATCH) return { kind: "miss" };
  return { kind: "error", msg: err };
}

// Internal sentinel distinct from any real error string.
const NO_MATCH = "\x00__no_match__\x00";

function matchInto(pat: Sexpr, v: Value, out: Map<string, Value>): string | null {
  if (pat.kind === "atom") return matchAtom(pat, v, out);
  return matchList(pat, v, out);
}

function isUpper(ch: string): boolean {
  return ch.length > 0 && ch >= "A" && ch <= "Z";
}

function matchAtom(
  p: Extract<Sexpr, { kind: "atom" }>,
  v: Value,
  out: Map<string, Value>,
): string | null {
  switch (p.atomKind) {
    case "symbol": {
      if (p.val === "_") return null;
      if (p.val === "nil") {
        if (v.kind === "list" && v.elems.length === 0) return null;
        return NO_MATCH;
      }
      if (p.val.length > 0 && isUpper(p.val[0]!)) {
        // Variable binding. Last-one-wins for repeated names (matches Go).
        out.set(p.val, v);
        return null;
      }
      return `unsupported symbol pattern "${p.val}" (only _, nil, and uppercase vars are allowed)`;
    }
    case "int": {
      const n = Number.parseInt(p.val, 10);
      if (Number.isNaN(n)) {
        return `bad int literal in pattern: ${p.val}`;
      }
      const lit: Value = { kind: "int", val: n };
      return valEqual(lit, v) ? null : NO_MATCH;
    }
    case "float": {
      const f = Number.parseFloat(p.val);
      if (Number.isNaN(f)) {
        return `bad float literal in pattern: ${p.val}`;
      }
      const lit: Value = { kind: "float", val: f };
      return valEqual(lit, v) ? null : NO_MATCH;
    }
    case "bool": {
      const want = p.val === "true";
      if (v.kind !== "bool" || v.val !== want) return NO_MATCH;
      return null;
    }
    case "string": {
      if (v.kind !== "string" || v.val !== p.val) return NO_MATCH;
      return null;
    }
  }
  return `unsupported atom kind in pattern: ${(p as { atomKind: string }).atomKind}`;
}

function matchList(
  p: Extract<Sexpr, { kind: "list" }>,
  v: Value,
  out: Map<string, Value>,
): string | null {
  const head = p.elems[0];
  const isCons =
    p.elems.length === 3 &&
    head !== undefined &&
    head.kind === "atom" &&
    head.atomKind === "symbol" &&
    head.val === "cons";
  if (!isCons) {
    return `unsupported list pattern (only cons patterns are supported)`;
  }
  if (v.kind !== "list" || v.elems.length === 0) return NO_MATCH;
  // head pattern -> first element
  const headErr = matchInto(p.elems[1]!, v.elems[0]!, out);
  if (headErr !== null) return headErr;
  // tail pattern -> rest of list as a ListVal
  const tail: ListVal = { kind: "list", elems: v.elems.slice(1) };
  return matchInto(p.elems[2]!, tail, out);
}
