// Pretty-printer for s-expressions in Shen surface syntax.
// Ported from shen-derive/core/sexpr_print.go.

import { type Sexpr, type SList, isSym } from "./sexpr.ts";

// prettyPrintSexpr renders an s-expression in Shen surface syntax.
export function prettyPrintSexpr(s: Sexpr): string {
  return ppSexpr(s);
}

function ppSexpr(s: Sexpr): string {
  if (s.kind === "atom") {
    if (s.atomKind === "string") {
      return quoteString(s.val);
    }
    return s.val;
  }

  // list
  if (s.elems.length === 0) {
    return "()";
  }

  // Try to render as [a b c] sugar for cons chains
  const desugared = desugarConsList(s);
  if (desugared !== null) {
    const { elems, tail } = desugared;
    const parts = elems.map(ppSexpr);
    if (isSym(tail, "nil")) {
      return "[" + parts.join(" ") + "]";
    }
    return "[" + parts.join(" ") + " | " + ppSexpr(tail) + "]";
  }

  const parts = s.elems.map(ppSexpr);
  return "(" + parts.join(" ") + ")";
}

// quoteString mirrors Go's fmt.Sprintf("%q", ...) for the common escape cases.
function quoteString(s: string): string {
  let out = '"';
  for (const ch of s) {
    switch (ch) {
      case "\\":
        out += "\\\\";
        break;
      case '"':
        out += '\\"';
        break;
      case "\n":
        out += "\\n";
        break;
      case "\r":
        out += "\\r";
        break;
      case "\t":
        out += "\\t";
        break;
      default: {
        const code = ch.charCodeAt(0);
        if (code < 0x20 || code === 0x7f) {
          out += "\\x" + code.toString(16).padStart(2, "0");
        } else {
          out += ch;
        }
      }
    }
  }
  out += '"';
  return out;
}

// desugarConsList checks if a list is a cons chain:
// (cons a (cons b (cons c nil))) -> { elems: [a, b, c], tail: nil }
// (cons a (cons b tail))         -> { elems: [a, b], tail }
function desugarConsList(l: SList): { elems: Sexpr[]; tail: Sexpr } | null {
  if (l.elems.length !== 3 || !isSym(l.elems[0], "cons")) {
    return null;
  }

  const elems: Sexpr[] = [l.elems[1]];
  let rest: Sexpr = l.elems[2];

  while (true) {
    if (
      rest.kind !== "list" ||
      rest.elems.length !== 3 ||
      !isSym(rest.elems[0], "cons")
    ) {
      return { elems, tail: rest };
    }
    elems.push(rest.elems[1]);
    rest = rest.elems[2];
  }
}
