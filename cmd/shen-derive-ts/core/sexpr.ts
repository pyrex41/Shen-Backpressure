// Package core provides the s-expression representation for shen-derive.
//
// All terms in shen-derive are represented as s-expressions — the same
// representation used by Shen itself. Ported from shen-derive/core/sexpr.go.

export type AtomKind = "symbol" | "int" | "float" | "string" | "bool";

export type Atom = { kind: "atom"; atomKind: AtomKind; val: string };
export type SList = { kind: "list"; elems: Sexpr[] };
export type Sexpr = Atom | SList;

// --- Helper constructors ---

// sym creates a symbol atom. Does NOT lowercase — uppercase-var pattern
// binding depends on case preservation.
export function sym(s: string): Atom {
  return { kind: "atom", atomKind: "symbol", val: s };
}

// num creates an integer atom. Stored as a string to preserve fidelity and
// keep int/float tagging independent of JS number semantics.
export function num(n: number | bigint): Atom {
  return { kind: "atom", atomKind: "int", val: typeof n === "bigint" ? n.toString() : Math.trunc(n).toString() };
}

// float_ creates a float atom. Trailing underscore avoids clashing with any
// future `Float` global and mirrors the Go `Float` helper.
export function float_(f: number): Atom {
  // Match Go's strconv.FormatFloat(f, 'f', -1, 64) shortest-round-trip style.
  let s = Number.isInteger(f) ? f.toFixed(1) : String(f);
  // JS `String(1e-7)` yields "1e-7"; Go's 'f' format would yield "0.0000001".
  // For the cases this module handles (test literals like 3.14) the default
  // stringification is fine.
  return { kind: "atom", atomKind: "float", val: s };
}

export function str(s: string): Atom {
  return { kind: "atom", atomKind: "string", val: s };
}

export function bool_(b: boolean): Atom {
  return { kind: "atom", atomKind: "bool", val: b ? "true" : "false" };
}

export function sList(...elems: Sexpr[]): SList {
  return { kind: "list", elems };
}

// --- Convenience constructors for common Shen forms ---

export function lambda(param: string, body: Sexpr): SList {
  return sList(sym("lambda"), sym(param), body);
}

export function sApply(f: Sexpr, ...args: Sexpr[]): SList {
  return { kind: "list", elems: [f, ...args] };
}

// --- Inspection helpers ---

export function isSym(s: Sexpr, name: string): boolean {
  return s.kind === "atom" && s.atomKind === "symbol" && s.val === name;
}

// isMetaVar returns [name, true] if s is a symbol beginning with '?'.
export function isMetaVar(s: Sexpr): [string, boolean] {
  if (s.kind === "atom" && s.atomKind === "symbol" && s.val.length > 1 && s.val[0] === "?") {
    return [s.val, true];
  }
  return ["", false];
}

export function headSym(s: Sexpr): string {
  if (s.kind !== "list" || s.elems.length === 0) return "";
  const h = s.elems[0];
  if (h.kind !== "atom" || h.atomKind !== "symbol") return "";
  return h.val;
}

export function listElems(s: Sexpr): Sexpr[] | null {
  return s.kind === "list" ? s.elems : null;
}

export function atomVal(s: Sexpr): [string, AtomKind, boolean] {
  if (s.kind !== "atom") return ["", "symbol", false];
  return [s.val, s.atomKind, true];
}

export function sexprIntVal(s: Sexpr): [number, boolean] {
  if (s.kind !== "atom" || s.atomKind !== "int") return [0, false];
  const n = Number.parseInt(s.val, 10);
  if (Number.isNaN(n)) return [0, false];
  return [n, true];
}

export function sexprFloatVal(s: Sexpr): [number, boolean] {
  if (s.kind !== "atom" || (s.atomKind !== "float" && s.atomKind !== "int")) {
    return [0, false];
  }
  const f = Number.parseFloat(s.val);
  if (Number.isNaN(f)) return [0, false];
  return [f, true];
}

export function sexprBoolVal(s: Sexpr): [boolean, boolean] {
  if (s.kind !== "atom" || s.atomKind !== "bool") return [false, false];
  return [s.val === "true", true];
}

export function symName(s: Sexpr): [string, boolean] {
  if (s.kind !== "atom" || s.atomKind !== "symbol") return ["", false];
  return [s.val, true];
}

// sexprEqual is recursive structural equality. It is NOT Object.is — the
// matcher and rewrite engine depend on structural comparison.
export function sexprEqual(a: Sexpr, b: Sexpr): boolean {
  if (a.kind === "atom" && b.kind === "atom") {
    return a.atomKind === b.atomKind && a.val === b.val;
  }
  if (a.kind === "list" && b.kind === "list") {
    if (a.elems.length !== b.elems.length) return false;
    for (let i = 0; i < a.elems.length; i++) {
      if (!sexprEqual(a.elems[i], b.elems[i])) return false;
    }
    return true;
  }
  return false;
}

// deepCopy recursively clones s. Helper functions must not mutate their
// input; the matcher and harness rely on this invariant.
export function deepCopy(s: Sexpr): Sexpr {
  if (s.kind === "atom") {
    return { kind: "atom", atomKind: s.atomKind, val: s.val };
  }
  return { kind: "list", elems: s.elems.map(deepCopy) };
}
