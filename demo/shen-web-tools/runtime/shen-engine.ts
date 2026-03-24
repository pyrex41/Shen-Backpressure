/**
 * runtime/shen-engine.ts — Lightweight Shen evaluator for the web tools demo.
 *
 * This implements enough of Shen's semantics to run the application logic
 * defined in src/*.shen. It handles:
 *   - S-expression parsing
 *   - Pattern matching (define with multiple clauses)
 *   - List operations (head, tail, cons, map, append)
 *   - Arithmetic and comparison
 *   - String operations (cn, length)
 *   - Lambda expressions (/. X Body)
 *   - Let bindings
 *   - js.call dispatch to the TypeScript bridge
 *   - if/cond expressions
 *
 * This is NOT a full Shen implementation — it's the minimal subset needed
 * for the web tools demo. A production system would use shen-js or similar.
 */

import { bridge } from "./bridge.js";

// ---------------------------------------------------------------------------
// S-Expression Parser
// ---------------------------------------------------------------------------

export type SExpr = string | number | boolean | SExpr[];

export function parse(source: string): SExpr[] {
  const tokens = tokenize(source);
  const exprs: SExpr[] = [];
  let pos = 0;

  function tokenize(src: string): string[] {
    const toks: string[] = [];
    let i = 0;
    while (i < src.length) {
      // Skip whitespace
      if (/\s/.test(src[i])) { i++; continue; }
      // Skip comments \* ... *\
      if (src[i] === "\\" && src[i + 1] === "*") {
        i += 2;
        while (i < src.length - 1 && !(src[i] === "*" && src[i + 1] === "\\")) i++;
        i += 2;
        continue;
      }
      // Brackets
      if (src[i] === "(" || src[i] === ")") { toks.push(src[i]); i++; continue; }
      if (src[i] === "[" || src[i] === "]") { toks.push(src[i]); i++; continue; }
      // String literal
      if (src[i] === '"') {
        let s = '"';
        i++;
        while (i < src.length && src[i] !== '"') {
          if (src[i] === "\\" && i + 1 < src.length) { s += src[i] + src[i + 1]; i += 2; }
          else { s += src[i]; i++; }
        }
        s += '"';
        i++; // skip closing quote
        toks.push(s);
        continue;
      }
      // Atom (symbol, number)
      let atom = "";
      while (i < src.length && !/[\s()\[\]]/.test(src[i])) { atom += src[i]; i++; }
      if (atom) toks.push(atom);
    }
    return toks;
  }

  function parseExpr(): SExpr {
    const tok = tokens[pos];
    if (tok === "(") {
      pos++;
      const list: SExpr[] = [];
      while (pos < tokens.length && tokens[pos] !== ")") {
        list.push(parseExpr());
      }
      pos++; // skip )
      return list;
    }
    if (tok === "[") {
      pos++;
      const list: SExpr[] = ["list"];
      while (pos < tokens.length && tokens[pos] !== "]") {
        if (tokens[pos] === "|") {
          pos++; // skip |
          list.push(parseExpr());
          (list as any).__hasTail = true;
          break;
        }
        list.push(parseExpr());
      }
      pos++; // skip ]
      return list;
    }
    pos++;
    // Number
    if (/^-?\d+(\.\d+)?$/.test(tok)) return parseFloat(tok);
    // String
    if (tok.startsWith('"') && tok.endsWith('"')) return tok.slice(1, -1);
    // Boolean
    if (tok === "true") return true;
    if (tok === "false") return false;
    // Symbol
    return tok;
  }

  while (pos < tokens.length) {
    exprs.push(parseExpr());
  }
  return exprs;
}

// ---------------------------------------------------------------------------
// Environment
// ---------------------------------------------------------------------------

type Env = Map<string, any>;

function envNew(parent?: Env): Env {
  const e = new Map<string, any>();
  if (parent) parent.forEach((v, k) => e.set(k, v));
  return e;
}

// ---------------------------------------------------------------------------
// Shen Engine
// ---------------------------------------------------------------------------

export class ShenEngine {
  private defs: Map<string, { params: string[][]; guards: SExpr[][]; bodies: SExpr[] }> = new Map();
  private globals: Env = new Map();

  constructor() {
    this.registerBuiltins();
  }

  /** Load and evaluate Shen source code */
  async loadSource(source: string): Promise<void> {
    const exprs = parse(source);
    for (const expr of exprs) {
      await this.evalTop(expr);
    }
  }

  /** Call a Shen-defined function */
  async call(name: string, ...args: any[]): Promise<any> {
    const def = this.defs.get(name);
    if (!def) throw new Error(`Undefined function: ${name}`);
    return this.applyDef(def, args);
  }

  // -------------------------------------------------------------------------
  // Top-level evaluation
  // -------------------------------------------------------------------------

  private async evalTop(expr: SExpr): Promise<void> {
    if (!Array.isArray(expr)) return;
    const [head, ...rest] = expr;

    // (datatype name ...) — skip, these are spec-only (handled by shengen)
    if (head === "datatype") return;

    // (define name { type } clause1 clause2 ...)
    if (head === "define") {
      const name = rest[0] as string;
      this.registerDefine(name, rest.slice(1));
      return;
    }
  }

  private registerDefine(name: string, body: SExpr[]): void {
    const clauses: { params: string[]; guard: SExpr | null; body: SExpr }[] = [];

    let i = 0;
    // Skip type signature { ... }
    if (body[i] === "{" || (Array.isArray(body[i]) && (body[i] as SExpr[])[0] === "{")) {
      // Find closing }
      while (i < body.length && body[i] !== "}") i++;
      if (body[i] === "}") i++;
    }
    // Also handle inline type sig like { type --> type }
    if (typeof body[i] === "string" && (body[i] as string).startsWith("{")) {
      while (i < body.length && !(typeof body[i] === "string" && (body[i] as string).endsWith("}"))) i++;
      i++;
    }

    // Parse clauses: pattern1 pattern2 ... -> body
    while (i < body.length) {
      const params: SExpr[] = [];
      // Collect patterns until ->
      while (i < body.length && body[i] !== "->") {
        params.push(body[i]);
        i++;
      }
      i++; // skip ->
      // Body is the next expression
      const clauseBody = body[i];
      i++;
      clauses.push({ params: params as any, guard: null, body: clauseBody });
    }

    const paramArrays = clauses.map((c) => c.params as any as string[]);
    const guards = clauses.map((c) => c.guard ? [c.guard] : []);
    const bodies = clauses.map((c) => c.body);

    this.defs.set(name, { params: paramArrays, guards, bodies });
  }

  // -------------------------------------------------------------------------
  // Evaluation
  // -------------------------------------------------------------------------

  async eval(expr: SExpr, env: Env): Promise<any> {
    // Atoms
    if (typeof expr === "number" || typeof expr === "boolean") return expr;
    if (typeof expr === "string") {
      // Variable lookup (uppercase first letter = variable)
      if (/^[A-Z]/.test(expr) && env.has(expr)) return env.get(expr);
      if (expr === "_") return undefined;
      if (env.has(expr)) return env.get(expr);
      return expr; // symbol/string literal
    }

    if (!Array.isArray(expr)) return expr;
    if (expr.length === 0) return [];

    const [head, ...args] = expr;

    // List literal [a b c] or [a | b]
    if (head === "list") {
      const items: any[] = [];
      for (let i = 1; i < args.length; i++) {
        items.push(await this.eval(args[i - 1], env));
      }
      // Check for tail cons
      if ((expr as any).__hasTail && args.length > 0) {
        const tail = await this.eval(args[args.length - 1], env);
        const rest = [];
        for (let i = 0; i < args.length - 1; i++) {
          rest.push(await this.eval(args[i], env));
        }
        return [...rest, ...(Array.isArray(tail) ? tail : [tail])];
      }
      const result: any[] = [];
      for (const a of args) {
        result.push(await this.eval(a, env));
      }
      return result;
    }

    // Special forms
    if (head === "let") return this.evalLet(args, env);
    if (head === "if") return this.evalIf(args, env);
    if (head === "cn") return this.evalCn(args, env);
    if (head === "/.") return this.evalLambda(args, env);
    if (head === "lambda") return this.evalLambda(args, env);

    // Arithmetic
    if (head === "+" || head === "-" || head === "*" || head === "/") {
      const a = await this.eval(args[0], env);
      const b = await this.eval(args[1], env);
      switch (head) {
        case "+": return a + b;
        case "-": return a - b;
        case "*": return a * b;
        case "/": return a / b;
      }
    }

    // Comparison
    if (head === ">" || head === "<" || head === ">=" || head === "<=" || head === "=") {
      const a = await this.eval(args[0], env);
      const b = await this.eval(args[1], env);
      switch (head) {
        case ">": return a > b;
        case "<": return a < b;
        case ">=": return a >= b;
        case "<=": return a <= b;
        case "=": return a === b || JSON.stringify(a) === JSON.stringify(b);
      }
    }

    // Boolean
    if (head === "and") return (await this.eval(args[0], env)) && (await this.eval(args[1], env));
    if (head === "or") return (await this.eval(args[0], env)) || (await this.eval(args[1], env));
    if (head === "not") return !(await this.eval(args[0], env));

    // List ops
    if (head === "head") { const v = await this.eval(args[0], env); return v[0]; }
    if (head === "tail") { const v = await this.eval(args[0], env); return v.slice(1); }
    if (head === "cons") {
      const h = await this.eval(args[0], env);
      const t = await this.eval(args[1], env);
      return [h, ...(Array.isArray(t) ? t : [t])];
    }
    if (head === "length") {
      const v = await this.eval(args[0], env);
      if (typeof v === "string") return v.length;
      if (Array.isArray(v)) return v.length;
      return 0;
    }
    if (head === "append") {
      const a = await this.eval(args[0], env);
      const b = await this.eval(args[1], env);
      return [...a, ...b];
    }

    // Map
    if (head === "map") {
      const fn = await this.eval(args[0], env);
      const lst = await this.eval(args[1], env);
      const result: any[] = [];
      for (const item of lst) {
        result.push(await this.applyFn(fn, [item], env));
      }
      return result;
    }

    // js.call — bridge dispatch
    if (head === "js.call") {
      const fnName = await this.eval(args[0], env);
      const fnArgs = args.length > 1 ? await this.eval(args[1], env) : undefined;
      return this.jsBridgeCall(fnName as string, fnArgs);
    }

    // value->string (special)
    if (head === "value->string") {
      const v = await this.eval(args[0], env);
      return bridge.toString(v);
    }

    // User-defined function call
    if (typeof head === "string" && this.defs.has(head)) {
      const evalArgs: any[] = [];
      for (const a of args) {
        evalArgs.push(await this.eval(a, env));
      }
      return this.applyDef(this.defs.get(head)!, evalArgs);
    }

    // If head evaluates to a function, apply it
    const headVal = await this.eval(head, env);
    if (typeof headVal === "function") {
      const evalArgs: any[] = [];
      for (const a of args) {
        evalArgs.push(await this.eval(a, env));
      }
      return headVal(...evalArgs);
    }

    // Default: return as list
    const result: any[] = [];
    for (const e of expr) {
      result.push(await this.eval(e, env));
    }
    return result;
  }

  private async evalLet(args: SExpr[], env: Env): Promise<any> {
    const newEnv = envNew(env);
    let i = 0;
    while (i < args.length - 1) {
      const name = args[i] as string;
      const val = await this.eval(args[i + 1], newEnv);
      newEnv.set(name, val);
      i += 2;
    }
    return this.eval(args[args.length - 1], newEnv);
  }

  private async evalIf(args: SExpr[], env: Env): Promise<any> {
    const cond = await this.eval(args[0], env);
    if (cond) return this.eval(args[1], env);
    return args.length > 2 ? this.eval(args[2], env) : undefined;
  }

  private async evalCn(args: SExpr[], env: Env): Promise<string> {
    const a = await this.eval(args[0], env);
    const b = await this.eval(args[1], env);
    return String(a) + String(b);
  }

  private evalLambda(args: SExpr[], env: Env): (x: any) => Promise<any> {
    const param = args[0] as string;
    const body = args[1];
    return async (x: any) => {
      const newEnv = envNew(env);
      newEnv.set(param, x);
      return this.eval(body, newEnv);
    };
  }

  // -------------------------------------------------------------------------
  // Function application with pattern matching
  // -------------------------------------------------------------------------

  private async applyDef(
    def: { params: string[][]; guards: SExpr[][]; bodies: SExpr[] },
    args: any[]
  ): Promise<any> {
    for (let i = 0; i < def.bodies.length; i++) {
      const env = envNew(this.globals);
      const patterns = def.params[i];
      if (this.matchPatterns(patterns, args, env)) {
        return this.eval(def.bodies[i], env);
      }
    }
    throw new Error(`No matching clause for args: ${JSON.stringify(args)}`);
  }

  private matchPatterns(patterns: SExpr[], args: any[], env: Env): boolean {
    if (patterns.length !== args.length) return false;
    for (let i = 0; i < patterns.length; i++) {
      if (!this.matchPattern(patterns[i], args[i], env)) return false;
    }
    return true;
  }

  private matchPattern(pattern: SExpr, value: any, env: Env): boolean {
    // Wildcard
    if (pattern === "_") return true;

    // Variable (uppercase)
    if (typeof pattern === "string" && /^[A-Z]/.test(pattern)) {
      if (env.has(pattern)) return env.get(pattern) === value;
      env.set(pattern, value);
      return true;
    }

    // Literal match
    if (typeof pattern === "number") return pattern === value;
    if (typeof pattern === "string") return pattern === value;
    if (typeof pattern === "boolean") return pattern === value;

    // List pattern [X | Xs]
    if (Array.isArray(pattern) && pattern[0] === "list") {
      if (!Array.isArray(value)) return false;
      const elems = pattern.slice(1);
      if ((pattern as any).__hasTail) {
        // [X | Xs] pattern
        if (value.length < elems.length - 1) return false;
        for (let i = 0; i < elems.length - 1; i++) {
          if (!this.matchPattern(elems[i], value[i], env)) return false;
        }
        return this.matchPattern(elems[elems.length - 1], value.slice(elems.length - 1), env);
      }
      if (value.length !== elems.length) return false;
      for (let i = 0; i < elems.length; i++) {
        if (!this.matchPattern(elems[i], value[i], env)) return false;
      }
      return true;
    }

    // Nested list/tuple pattern
    if (Array.isArray(pattern)) {
      if (!Array.isArray(value)) return false;
      if (pattern.length !== value.length) return false;
      for (let i = 0; i < pattern.length; i++) {
        if (!this.matchPattern(pattern[i], value[i], env)) return false;
      }
      return true;
    }

    return false;
  }

  private async applyFn(fn: any, args: any[], env: Env): Promise<any> {
    if (typeof fn === "function") return fn(...args);
    if (typeof fn === "string" && this.defs.has(fn)) {
      return this.applyDef(this.defs.get(fn)!, args);
    }
    throw new Error(`Cannot apply: ${JSON.stringify(fn)}`);
  }

  // -------------------------------------------------------------------------
  // Bridge dispatch
  // -------------------------------------------------------------------------

  private async jsBridgeCall(name: string, args: any): Promise<any> {
    const parts = name.split(".");
    let obj: any = globalThis;
    for (const p of parts) {
      obj = obj?.[p];
    }
    if (typeof obj !== "function") {
      throw new Error(`Bridge function not found: ${name}`);
    }
    return obj(args);
  }

  // -------------------------------------------------------------------------
  // Built-in functions
  // -------------------------------------------------------------------------

  private registerBuiltins(): void {
    // Register built-in functions that aren't special forms
    // These are available as regular function calls in Shen code
  }
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

export async function createEngine(sources: string[]): Promise<ShenEngine> {
  const engine = new ShenEngine();
  for (const src of sources) {
    await engine.loadSource(src);
  }
  return engine;
}
