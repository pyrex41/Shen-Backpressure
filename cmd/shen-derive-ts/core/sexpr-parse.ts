// Package core — s-expression parser for shen-derive.
//
// Character-at-a-time LL(1) parser ported from shen-derive/core/sexpr_parse.go.
// Handles (lists), [cons-sugar], "strings" with escapes, atoms
// (symbols / ints / floats / true / false / nil), and line comments
// starting with `\\ ` or `-- `.

import { type Sexpr, sym, str, bool_, sList } from "./sexpr.ts";

// parseSexpr parses a single s-expression from input. Throws on failure.
export function parseSexpr(input: string): Sexpr {
  const p = new SexprParser(input);
  p.skipWhitespace();
  const expr = p.parse();
  p.skipWhitespace();
  if (p.pos < p.input.length) {
    throw p.error(`unexpected input after expression`, p.pos);
  }
  return expr;
}

// parseAllSexprs parses all top-level s-expressions from input.
export function parseAllSexprs(input: string): Sexpr[] {
  const p = new SexprParser(input);
  const result: Sexpr[] = [];
  for (;;) {
    p.skipWhitespace();
    if (p.pos >= p.input.length) break;
    result.push(p.parse());
  }
  return result;
}

class SexprParser {
  input: string;
  pos: number;

  constructor(input: string) {
    this.input = input;
    this.pos = 0;
  }

  error(msg: string, at: number): Error {
    const start = Math.max(0, at - 10);
    const end = Math.min(this.input.length, at + 10);
    const snippet = this.input.slice(start, end).replace(/\n/g, "\\n");
    return new Error(`parse error at position ${at}: ${msg} (near: "${snippet}")`);
  }

  parse(): Sexpr {
    this.skipWhitespace();
    if (this.pos >= this.input.length) {
      throw this.error("unexpected end of input", this.pos);
    }
    const ch = this.input[this.pos];
    if (ch === "(") return this.parseList();
    if (ch === "[") return this.parseSquareList();
    if (ch === '"') return this.parseString();
    if (ch === ")" || ch === "]") {
      throw this.error(`unexpected '${ch}'`, this.pos);
    }
    return this.parseAtom();
  }

  // parseList parses (elem1 elem2 ...)
  parseList(): Sexpr {
    this.pos++; // consume '('
    const elems: Sexpr[] = [];
    for (;;) {
      this.skipWhitespace();
      if (this.pos >= this.input.length) {
        throw this.error("unclosed '(' — expected ')'", this.pos);
      }
      if (this.input[this.pos] === ")") {
        this.pos++;
        return { kind: "list", elems };
      }
      elems.push(this.parse());
    }
  }

  // parseSquareList parses [e1 e2 ... | tail] or [e1 e2 ...].
  // [a b c] desugars to (cons a (cons b (cons c nil)))
  // [a b | c] desugars to (cons a (cons b c))
  parseSquareList(): Sexpr {
    this.pos++; // consume '['
    const elems: Sexpr[] = [];
    let tail: Sexpr | null = null;

    for (;;) {
      this.skipWhitespace();
      if (this.pos >= this.input.length) {
        throw this.error("unclosed '[' — expected ']'", this.pos);
      }
      if (this.input[this.pos] === "]") {
        this.pos++;
        break;
      }
      if (this.input[this.pos] === "|") {
        this.pos++; // consume '|'
        this.skipWhitespace();
        tail = this.parse();
        this.skipWhitespace();
        if (this.pos >= this.input.length || this.input[this.pos] !== "]") {
          throw this.error("expected ']' after tail in cons notation", this.pos);
        }
        this.pos++;
        break;
      }
      elems.push(this.parse());
    }

    // Build cons chain from right
    let result: Sexpr = tail ?? sym("nil");
    for (let i = elems.length - 1; i >= 0; i--) {
      result = sList(sym("cons"), elems[i], result);
    }
    return result;
  }

  // parseString parses "..." with escape handling.
  parseString(): Sexpr {
    this.pos++; // consume opening '"'
    let out = "";
    while (this.pos < this.input.length) {
      const ch = this.input[this.pos];
      if (ch === '"') {
        this.pos++;
        return str(out);
      }
      if (ch === "\\" && this.pos + 1 < this.input.length) {
        this.pos++;
        const esc = this.input[this.pos];
        switch (esc) {
          case "n":
            out += "\n";
            break;
          case "t":
            out += "\t";
            break;
          case "\\":
            out += "\\";
            break;
          case '"':
            out += '"';
            break;
          default:
            out += "\\" + esc;
            break;
        }
        this.pos++;
        continue;
      }
      out += ch;
      this.pos++;
    }
    throw this.error("unclosed string literal", this.pos);
  }

  // parseAtom parses a symbol, number, or boolean.
  parseAtom(): Sexpr {
    const start = this.pos;
    while (this.pos < this.input.length) {
      const ch = this.input[this.pos];
      if (
        ch === "(" ||
        ch === ")" ||
        ch === "[" ||
        ch === "]" ||
        ch === '"' ||
        isWhitespace(ch)
      ) {
        break;
      }
      this.pos++;
    }
    if (this.pos === start) {
      throw this.error("expected atom", this.pos);
    }
    const tok = this.input.slice(start, this.pos);

    if (tok === "true") return bool_(true);
    if (tok === "false") return bool_(false);

    if (isIntLiteral(tok)) {
      return { kind: "atom", atomKind: "int", val: tok };
    }
    if (isFloatLiteral(tok)) {
      return { kind: "atom", atomKind: "float", val: tok };
    }
    return sym(tok);
  }

  skipWhitespace(): void {
    while (this.pos < this.input.length) {
      const ch = this.input[this.pos];
      if (isWhitespace(ch)) {
        this.pos++;
        continue;
      }
      // Shen-style \\ line comment
      if (
        ch === "\\" &&
        this.pos + 1 < this.input.length &&
        this.input[this.pos + 1] === "\\"
      ) {
        this.pos += 2;
        while (this.pos < this.input.length && this.input[this.pos] !== "\n") {
          this.pos++;
        }
        continue;
      }
      // -- line comment (shen-derive v1 syntax)
      if (
        ch === "-" &&
        this.pos + 1 < this.input.length &&
        this.input[this.pos + 1] === "-"
      ) {
        this.pos += 2;
        while (this.pos < this.input.length && this.input[this.pos] !== "\n") {
          this.pos++;
        }
        continue;
      }
      break;
    }
  }
}

function isWhitespace(ch: string): boolean {
  return ch === " " || ch === "\t" || ch === "\n" || ch === "\r";
}

function isDigit(ch: string): boolean {
  return ch >= "0" && ch <= "9";
}

function isIntLiteral(s: string): boolean {
  if (s.length === 0) return false;
  let start = 0;
  if (s[0] === "-" || s[0] === "+") {
    if (s.length === 1) return false; // bare +/- is a symbol
    start = 1;
  }
  for (let i = start; i < s.length; i++) {
    if (!isDigit(s[i])) return false;
  }
  return true;
}

function isFloatLiteral(s: string): boolean {
  if (s.length === 0) return false;
  let hasDot = false;
  let start = 0;
  if (s[0] === "-" || s[0] === "+") {
    if (s.length === 1) return false;
    start = 1;
  }
  for (let i = start; i < s.length; i++) {
    if (s[i] === ".") {
      if (hasDot) return false;
      hasDot = true;
      continue;
    }
    if (!isDigit(s[i])) return false;
  }
  return hasDot;
}
