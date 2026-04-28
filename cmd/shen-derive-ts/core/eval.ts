// Evaluator for shen-derive. Ported from shen-derive/core/eval.go.
//
// Pure, synchronous tree-walking interpreter over `Sexpr`. Owns the
// runtime `Value` discriminated union shared with the pattern matcher.
//
// Special forms: `lambda`, `let`, `if`, `@p` (tuple). Everything else
// goes through primitive application (curried via `PrimPartial`) or
// closure/builtin application.

import type { Sexpr } from "./sexpr.ts";
import { headSym, symName } from "./sexpr.ts";

// --- Value discriminated union ---
//
// A superset of what the pattern matcher needs. `int` and `float` are
// tagged so that mixed-mode arithmetic does not silently collapse into
// plain JS numbers (see `numBinOp`). Closures / primitive partials /
// builtins are not comparable by value (ref identity only).

export type IntVal = { kind: "int"; val: number };
export type FloatVal = { kind: "float"; val: number };
export type BoolVal = { kind: "bool"; val: boolean };
export type StringVal = { kind: "string"; val: string };
export type ListVal = { kind: "list"; elems: Value[] };
export type TupleVal = { kind: "tuple"; fst: Value; snd: Value };
export type ClosureVal = {
  kind: "closure";
  env: Env;
  param: string;
  body: Sexpr;
};
export type PrimPartialVal = {
  kind: "primPartial";
  op: string;
  args: Value[];
};
export type BuiltinVal = {
  kind: "builtin";
  name: string;
  fn: (arg: Value) => MaybePromise<Value>;
};

export type MaybePromise<T> = T | Promise<T>;

export type Value =
  | IntVal
  | FloatVal
  | BoolVal
  | StringVal
  | ListVal
  | TupleVal
  | ClosureVal
  | PrimPartialVal
  | BuiltinVal;

// --- Constructors ---

export function intVal(n: number | bigint): IntVal {
  const v = typeof n === "bigint" ? Number(n) : Math.trunc(n);
  return { kind: "int", val: v };
}

export function floatVal(f: number): FloatVal {
  return { kind: "float", val: f };
}

export function boolVal(b: boolean): BoolVal {
  return { kind: "bool", val: b };
}

export function stringVal(s: string): StringVal {
  return { kind: "string", val: s };
}

export function listVal(...elems: Value[]): ListVal {
  return { kind: "list", elems };
}

export function tupleVal(fst: Value, snd: Value): TupleVal {
  return { kind: "tuple", fst, snd };
}

export function closureVal(env: Env, param: string, body: Sexpr): ClosureVal {
  return { kind: "closure", env, param, body };
}

export function primPartial(op: string, args: Value[]): PrimPartialVal {
  return { kind: "primPartial", op, args };
}

export function builtinFn(
  name: string,
  fn: (arg: Value) => MaybePromise<Value>,
): BuiltinVal {
  return { kind: "builtin", name, fn };
}

// --- Env: linked list ---
//
// Null sentinel + immutable chain nodes. Shadowing is by chain depth,
// not by Map overwrite, so the harness's mutable `{env}` container can
// share a parent chain across extensions.

export class Env {
  private readonly name: string;
  private readonly val: Value;
  private readonly parent: Env | null;

  private constructor(name: string, val: Value, parent: Env | null) {
    this.name = name;
    this.val = val;
    this.parent = parent;
  }

  static empty(): Env {
    // Sentinel head node whose `parent` is null and whose name will
    // never be looked up. We use a marker frame instead of exporting
    // `Env | null` so callers have a concrete type.
    return new Env("\x00__empty__\x00", { kind: "bool", val: false }, null);
  }

  extend(name: string, val: Value): Env {
    return new Env(name, val, this);
  }

  lookup(name: string): Value | null {
    // Walk from this frame up to (but not including) the empty sentinel.
    // The sentinel's name is a value no user code can write.
    for (let cur: Env | null = this; cur !== null; cur = cur.parent) {
      if (cur.name === name) return cur.val;
    }
    return null;
  }
}

// --- Primitive table ---

function primArity(op: string): number {
  switch (op) {
    case "not":
    case "fst":
    case "snd":
    case "concat":
    case "length":
    case "trim":
    case "head-char":
    case "tail-chars":
      return 1;
    case "+":
    case "-":
    case "*":
    case "/":
    case "%":
    case "shen.mod":
    case "=":
    case "!=":
    case "<":
    case "<=":
    case ">":
    case ">=":
    case "and":
    case "or":
    case "cons":
    case "map":
    case "filter":
    case "unfoldr":
    case "element?":
      return 2;
    case "foldr":
    case "foldl":
    case "scanl":
    case "compose":
    case "in-range?":
      return 3;
    default:
      return 0;
  }
}

function isBuiltin(name: string): boolean {
  return primArity(name) > 0;
}

// --- Eval ---

export function evalExpr(env: Env, s: Sexpr): Value {
  if (s.kind === "atom") {
    switch (s.atomKind) {
      case "int": {
        const n = Number.parseInt(s.val, 10);
        if (Number.isNaN(n)) throw new Error(`bad int literal: ${s.val}`);
        return { kind: "int", val: n };
      }
      case "float": {
        const f = Number.parseFloat(s.val);
        if (Number.isNaN(f)) throw new Error(`bad float literal: ${s.val}`);
        return { kind: "float", val: f };
      }
      case "bool":
        return { kind: "bool", val: s.val === "true" };
      case "string":
        return { kind: "string", val: s.val };
      case "symbol": {
        if (s.val === "nil") return { kind: "list", elems: [] };
        if (isBuiltin(s.val)) return { kind: "primPartial", op: s.val, args: [] };
        const v = env.lookup(s.val);
        if (v === null) throw new Error(`unbound variable "${s.val}"`);
        return v;
      }
    }
  }

  // list
  if (s.elems.length === 0) return { kind: "list", elems: [] };

  const head = headSym(s);

  // Special forms
  switch (head) {
    case "lambda": {
      if (s.elems.length !== 3) {
        throw new Error(`lambda: expected 3 elements, got ${s.elems.length}`);
      }
      const [name, ok] = symName(s.elems[1]!);
      if (!ok) throw new Error("lambda: param must be a symbol");
      return { kind: "closure", env, param: name, body: s.elems[2]! };
    }
    case "let": {
      if (s.elems.length !== 4) {
        throw new Error(`let: expected 4 elements, got ${s.elems.length}`);
      }
      const [name, ok] = symName(s.elems[1]!);
      if (!ok) throw new Error("let: name must be a symbol");
      const val = evalExpr(env, s.elems[2]!);
      return evalExpr(env.extend(name, val), s.elems[3]!);
    }
    case "if": {
      if (s.elems.length !== 4) {
        throw new Error(`if: expected 4 elements, got ${s.elems.length}`);
      }
      const cv = evalExpr(env, s.elems[1]!);
      if (cv.kind !== "bool") {
        throw new Error(`if: condition must be Bool, got ${cv.kind}`);
      }
      return cv.val
        ? evalExpr(env, s.elems[2]!)
        : evalExpr(env, s.elems[3]!);
    }
    case "@p": {
      if (s.elems.length !== 3) {
        throw new Error(`@p: expected 3 elements, got ${s.elems.length}`);
      }
      const fst = evalExpr(env, s.elems[1]!);
      const snd = evalExpr(env, s.elems[2]!);
      return { kind: "tuple", fst, snd };
    }
  }

  // Application (curried). Evaluate head, then each arg in turn.
  let result = evalExpr(env, s.elems[0]!);
  for (let i = 1; i < s.elems.length; i++) {
    const av = evalExpr(env, s.elems[i]!);
    result = applyVal(result, av);
  }
  return result;
}

export async function evalExprAsync(env: Env, s: Sexpr): Promise<Value> {
  if (s.kind === "atom") {
    switch (s.atomKind) {
      case "int": {
        const n = Number.parseInt(s.val, 10);
        if (Number.isNaN(n)) throw new Error(`bad int literal: ${s.val}`);
        return { kind: "int", val: n };
      }
      case "float": {
        const f = Number.parseFloat(s.val);
        if (Number.isNaN(f)) throw new Error(`bad float literal: ${s.val}`);
        return { kind: "float", val: f };
      }
      case "bool":
        return { kind: "bool", val: s.val === "true" };
      case "string":
        return { kind: "string", val: s.val };
      case "symbol": {
        if (s.val === "nil") return { kind: "list", elems: [] };
        if (isBuiltin(s.val)) return { kind: "primPartial", op: s.val, args: [] };
        const v = env.lookup(s.val);
        if (v === null) throw new Error(`unbound variable "${s.val}"`);
        return v;
      }
    }
  }

  if (s.elems.length === 0) return { kind: "list", elems: [] };

  const head = headSym(s);

  switch (head) {
    case "lambda": {
      if (s.elems.length !== 3) {
        throw new Error(`lambda: expected 3 elements, got ${s.elems.length}`);
      }
      const [name, ok] = symName(s.elems[1]!);
      if (!ok) throw new Error("lambda: param must be a symbol");
      return { kind: "closure", env, param: name, body: s.elems[2]! };
    }
    case "let": {
      if (s.elems.length !== 4) {
        throw new Error(`let: expected 4 elements, got ${s.elems.length}`);
      }
      const [name, ok] = symName(s.elems[1]!);
      if (!ok) throw new Error("let: name must be a symbol");
      const val = await evalExprAsync(env, s.elems[2]!);
      return evalExprAsync(env.extend(name, val), s.elems[3]!);
    }
    case "if": {
      if (s.elems.length !== 4) {
        throw new Error(`if: expected 4 elements, got ${s.elems.length}`);
      }
      const cv = await evalExprAsync(env, s.elems[1]!);
      if (cv.kind !== "bool") {
        throw new Error(`if: condition must be Bool, got ${cv.kind}`);
      }
      return cv.val
        ? evalExprAsync(env, s.elems[2]!)
        : evalExprAsync(env, s.elems[3]!);
    }
    case "@p": {
      if (s.elems.length !== 3) {
        throw new Error(`@p: expected 3 elements, got ${s.elems.length}`);
      }
      const fst = await evalExprAsync(env, s.elems[1]!);
      const snd = await evalExprAsync(env, s.elems[2]!);
      return { kind: "tuple", fst, snd };
    }
  }

  let result = await evalExprAsync(env, s.elems[0]!);
  for (let i = 1; i < s.elems.length; i++) {
    const av = await evalExprAsync(env, s.elems[i]!);
    result = await applyValAsync(result, av);
  }
  return result;
}

// --- Apply ---

export function applyVal(f: Value, arg: Value): Value {
  switch (f.kind) {
    case "closure":
      return evalExpr(f.env.extend(f.param, arg), f.body);
    case "primPartial": {
      const newArgs = f.args.slice();
      newArgs.push(arg);
      const arity = primArity(f.op);
      if (newArgs.length < arity) {
        return { kind: "primPartial", op: f.op, args: newArgs };
      }
      return execPrim(f.op, newArgs);
    }
    case "builtin":
      return expectSyncValue(f.name, f.fn(arg));
    default:
      throw new Error(`cannot apply non-function value: ${f.kind}`);
  }
}

export async function applyValAsync(f: Value, arg: Value): Promise<Value> {
  switch (f.kind) {
    case "closure":
      return evalExprAsync(f.env.extend(f.param, arg), f.body);
    case "primPartial": {
      const newArgs = f.args.slice();
      newArgs.push(arg);
      const arity = primArity(f.op);
      if (newArgs.length < arity) {
        return { kind: "primPartial", op: f.op, args: newArgs };
      }
      return execPrimAsync(f.op, newArgs);
    }
    case "builtin":
      return f.fn(arg);
    default:
      throw new Error(`cannot apply non-function value: ${f.kind}`);
  }
}

// --- Primitive execution ---

export function execPrim(op: string, args: Value[]): Value {
  switch (op) {
    case "+":
      return numBinOp(args, "+", (a, b) => a + b, (a, b) => a + b);
    case "-":
      return numBinOp(args, "-", (a, b) => a - b, (a, b) => a - b);
    case "*":
      return numBinOp(args, "*", (a, b) => a * b, (a, b) => a * b);
    case "/":
      return numBinOp(
        args,
        "/",
        (a, b) => {
          if (b === 0) throw new Error("/: division by zero");
          // Go int division truncates toward zero.
          return Math.trunc(a / b);
        },
        (a, b) => {
          if (b === 0) throw new Error("/: division by zero");
          return a / b;
        },
      );
    case "%":
    case "shen.mod": {
      const a = asInt(args[0]!, "%");
      const b = asInt(args[1]!, "%");
      if (b === 0) throw new Error("%: modulo by zero");
      // Go int % truncates toward zero; JS `%` already matches.
      return { kind: "int", val: a % b };
    }

    case "=":
      return { kind: "bool", val: valEqual(args[0]!, args[1]!) };
    case "!=":
      return { kind: "bool", val: !valEqual(args[0]!, args[1]!) };
    case "<":
      return numCmp(args, "<", (a, b) => a < b);
    case "<=":
      return numCmp(args, "<=", (a, b) => a <= b);
    case ">":
      return numCmp(args, ">", (a, b) => a > b);
    case ">=":
      return numCmp(args, ">=", (a, b) => a >= b);

    case "and": {
      const a = asBool(args[0]!, "and");
      const b = asBool(args[1]!, "and");
      return { kind: "bool", val: a && b };
    }
    case "or": {
      const a = asBool(args[0]!, "or");
      const b = asBool(args[1]!, "or");
      return { kind: "bool", val: a || b };
    }
    case "not": {
      const a = asBool(args[0]!, "not");
      return { kind: "bool", val: !a };
    }

    case "cons": {
      const xs = asList(args[1]!, "cons");
      return { kind: "list", elems: [args[0]!, ...xs] };
    }
    case "concat": {
      const xss = asList(args[0]!, "concat");
      const out: Value[] = [];
      for (const x of xss) {
        const inner = asList(x, "concat");
        for (const y of inner) out.push(y);
      }
      return { kind: "list", elems: out };
    }
    case "fst": {
      const t = asTuple(args[0]!, "fst");
      return t.fst;
    }
    case "snd": {
      const t = asTuple(args[0]!, "snd");
      return t.snd;
    }

    case "map": {
      const f = args[0]!;
      const xs = asList(args[1]!, "map");
      const out: Value[] = new Array(xs.length);
      for (let i = 0; i < xs.length; i++) {
        out[i] = applyVal(f, xs[i]!);
      }
      return { kind: "list", elems: out };
    }

    case "foldr": {
      const f = args[0]!;
      const e = args[1]!;
      const xs = asList(args[2]!, "foldr");
      let acc = e;
      for (let i = xs.length - 1; i >= 0; i--) {
        const partial = applyVal(f, xs[i]!);
        acc = applyVal(partial, acc);
      }
      return acc;
    }

    case "foldl": {
      const f = args[0]!;
      const e = args[1]!;
      const xs = asList(args[2]!, "foldl");
      let acc = e;
      for (const x of xs) {
        const partial = applyVal(f, acc);
        acc = applyVal(partial, x);
      }
      return acc;
    }

    case "scanl": {
      const f = args[0]!;
      const e = args[1]!;
      const xs = asList(args[2]!, "scanl");
      const out: Value[] = [e];
      let acc = e;
      for (const x of xs) {
        const partial = applyVal(f, acc);
        acc = applyVal(partial, x);
        out.push(acc);
      }
      return { kind: "list", elems: out };
    }

    case "filter": {
      const p = args[0]!;
      const xs = asList(args[1]!, "filter");
      const out: Value[] = [];
      for (const x of xs) {
        const pv = applyVal(p, x);
        if (asBool(pv, "filter")) out.push(x);
      }
      return { kind: "list", elems: out };
    }

    case "unfoldr": {
      const f = args[0]!;
      let seed = args[1]!;
      const out: Value[] = [];
      // Go caps iterations at 10000 to prevent infinite loops; match.
      for (let i = 0; i < 10000; i++) {
        const pair = applyVal(f, seed);
        const tp = asTuple(pair, "unfoldr");
        const cont = asBool(tp.fst, "unfoldr");
        if (!cont) break;
        const inner = asTuple(tp.snd, "unfoldr");
        out.push(inner.fst);
        seed = inner.snd;
      }
      return { kind: "list", elems: out };
    }

    case "compose": {
      const f = args[0]!;
      const g = args[1]!;
      const x = args[2]!;
      return applyVal(f, applyVal(g, x));
    }

    // String primitives — mirror shengen-ts's translateDefineExpr emission
    // (length → x.length; trim → x.trim(); head-char → x[0] ?? "";
    // tail-chars → x.slice(1); in-range? → lex compare; element? → set
    // membership).
    case "length": {
      const v = args[0]!;
      if (v.kind === "string") return { kind: "int", val: v.val.length };
      if (v.kind === "list") return { kind: "int", val: v.elems.length };
      throw new Error(`length: expected String or List, got ${v.kind}`);
    }
    case "trim": {
      const s = asString(args[0]!, "trim");
      return { kind: "string", val: s.trim() };
    }
    case "head-char": {
      const s = asString(args[0]!, "head-char");
      return { kind: "string", val: s.length === 0 ? "" : s[0]! };
    }
    case "tail-chars": {
      const s = asString(args[0]!, "tail-chars");
      return { kind: "string", val: s.slice(1) };
    }
    case "in-range?": {
      const c = asString(args[0]!, "in-range?");
      const lo = asString(args[1]!, "in-range?");
      const hi = asString(args[2]!, "in-range?");
      return { kind: "bool", val: c >= lo && c <= hi };
    }
    case "element?": {
      const x = args[0]!;
      const xs = asList(args[1]!, "element?");
      for (const y of xs) {
        if (valEqual(x, y)) return { kind: "bool", val: true };
      }
      return { kind: "bool", val: false };
    }
  }
  throw new Error(`unknown primitive: ${op}`);
}

async function execPrimAsync(op: string, args: Value[]): Promise<Value> {
  switch (op) {
    case "map": {
      const f = args[0]!;
      const xs = asList(args[1]!, "map");
      const out: Value[] = new Array(xs.length);
      for (let i = 0; i < xs.length; i++) {
        out[i] = await applyValAsync(f, xs[i]!);
      }
      return { kind: "list", elems: out };
    }

    case "foldr": {
      const f = args[0]!;
      const e = args[1]!;
      const xs = asList(args[2]!, "foldr");
      let acc = e;
      for (let i = xs.length - 1; i >= 0; i--) {
        const partial = await applyValAsync(f, xs[i]!);
        acc = await applyValAsync(partial, acc);
      }
      return acc;
    }

    case "foldl": {
      const f = args[0]!;
      const e = args[1]!;
      const xs = asList(args[2]!, "foldl");
      let acc = e;
      for (const x of xs) {
        const partial = await applyValAsync(f, acc);
        acc = await applyValAsync(partial, x);
      }
      return acc;
    }

    case "scanl": {
      const f = args[0]!;
      const e = args[1]!;
      const xs = asList(args[2]!, "scanl");
      const out: Value[] = [e];
      let acc = e;
      for (const x of xs) {
        const partial = await applyValAsync(f, acc);
        acc = await applyValAsync(partial, x);
        out.push(acc);
      }
      return { kind: "list", elems: out };
    }

    case "filter": {
      const p = args[0]!;
      const xs = asList(args[1]!, "filter");
      const out: Value[] = [];
      for (const x of xs) {
        const pv = await applyValAsync(p, x);
        if (asBool(pv, "filter")) out.push(x);
      }
      return { kind: "list", elems: out };
    }

    case "unfoldr": {
      const f = args[0]!;
      let seed = args[1]!;
      const out: Value[] = [];
      for (let i = 0; i < 10000; i++) {
        const pair = await applyValAsync(f, seed);
        const tp = asTuple(pair, "unfoldr");
        const cont = asBool(tp.fst, "unfoldr");
        if (!cont) break;
        const inner = asTuple(tp.snd, "unfoldr");
        out.push(inner.fst);
        seed = inner.snd;
      }
      return { kind: "list", elems: out };
    }

    case "compose": {
      const f = args[0]!;
      const g = args[1]!;
      const x = args[2]!;
      return applyValAsync(f, await applyValAsync(g, x));
    }

    default:
      return execPrim(op, args);
  }
}

// --- Coercion helpers ---

function asInt(v: Value, ctx: string): number {
  if (v.kind !== "int") {
    throw new Error(`${ctx}: expected Int, got ${v.kind}`);
  }
  return v.val;
}

function asString(v: Value, ctx: string): string {
  if (v.kind !== "string") {
    throw new Error(`${ctx}: expected String, got ${v.kind}`);
  }
  return v.val;
}

function asNum(v: Value): { ok: true; val: number } | { ok: false } {
  if (v.kind === "int" || v.kind === "float") {
    return { ok: true, val: v.val };
  }
  return { ok: false };
}

function asBool(v: Value, ctx: string): boolean {
  if (v.kind !== "bool") {
    throw new Error(`${ctx}: expected Bool, got ${v.kind}`);
  }
  return v.val;
}

function asList(v: Value, ctx: string): Value[] {
  if (v.kind !== "list") {
    throw new Error(`${ctx}: expected List, got ${v.kind}`);
  }
  return v.elems;
}

function asTuple(v: Value, ctx: string): TupleVal {
  if (v.kind !== "tuple") {
    throw new Error(`${ctx}: expected Tuple, got ${v.kind}`);
  }
  return v;
}

function expectSyncValue(name: string, value: MaybePromise<Value>): Value {
  if (isPromiseLike(value)) {
    throw new Error(`builtin ${name}: async result requires evalExprAsync`);
  }
  return value;
}

function isPromiseLike(value: unknown): value is Promise<Value> {
  return typeof value === "object" && value !== null && "then" in value;
}

// numBinOp: int+int → int; anything-with-float → float. DO NOT merge
// kinds silently — the `kind` tag on the result is load-bearing for
// downstream Go/TS emission symmetry.
function numBinOp(
  args: Value[],
  name: string,
  intOp: (a: number, b: number) => number,
  floatOp: (a: number, b: number) => number,
): Value {
  const a = args[0]!;
  const b = args[1]!;
  if (a.kind === "int" && b.kind === "int") {
    return { kind: "int", val: intOp(a.val, b.val) };
  }
  const ax = asNum(a);
  if (!ax.ok) throw new Error(`${name}: expected number, got ${a.kind}`);
  const bx = asNum(b);
  if (!bx.ok) throw new Error(`${name}: expected number, got ${b.kind}`);
  return { kind: "float", val: floatOp(ax.val, bx.val) };
}

function numCmp(
  args: Value[],
  name: string,
  pred: (a: number, b: number) => boolean,
): Value {
  const a = asNum(args[0]!);
  if (!a.ok) {
    throw new Error(`${name}: expected number, got ${args[0]!.kind}`);
  }
  const b = asNum(args[1]!);
  if (!b.ok) {
    throw new Error(`${name}: expected number, got ${args[1]!.kind}`);
  }
  return { kind: "bool", val: pred(a.val, b.val) };
}

// valEqual: structural equality. int(1) numerically equals float(1.0).
// Strings/bools compare by value. Lists/tuples recurse. Closures /
// primPartial / builtins are not comparable and always return false.
export function valEqual(a: Value, b: Value): boolean {
  const ax = asNum(a);
  const bx = asNum(b);
  if (ax.ok && bx.ok) return ax.val === bx.val;
  if (a.kind !== b.kind) return false;
  switch (a.kind) {
    case "bool":
      return a.val === (b as BoolVal).val;
    case "string":
      return a.val === (b as StringVal).val;
    case "list": {
      const bl = b as ListVal;
      if (a.elems.length !== bl.elems.length) return false;
      for (let i = 0; i < a.elems.length; i++) {
        if (!valEqual(a.elems[i]!, bl.elems[i]!)) return false;
      }
      return true;
    }
    case "tuple": {
      const bt = b as TupleVal;
      return valEqual(a.fst, bt.fst) && valEqual(a.snd, bt.snd);
    }
    default:
      // int / float handled above via asNum; closures / primPartial /
      // builtins are not structurally comparable.
      return false;
  }
}
