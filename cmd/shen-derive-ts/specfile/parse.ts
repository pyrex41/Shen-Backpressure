// Package specfile parses .shen specification files for shen-derive.
//
// Ported from shen-derive/specfile/parse.go. Intentionally does NOT share
// code with cmd/shengen-ts/shengen.ts — parallel implementations are the
// whole point of the dual-tool design.
//
// Unlike the Go version this parser keeps clause patterns/guards/bodies as
// raw strings; downstream code in cmd/shen-derive-ts re-parses them via
// core/sexpr.ts when needed.

// --- Shared type contract ---

export type Premise = {
  varName: string;
  typeName: string;
};

export type VerifiedPremise = {
  raw: string;      // the full premise text as it appeared in the source
  varName: string;  // bound variable name if the premise was "X : verified"; empty otherwise
  expr: string;     // the predicate expression as a string
};

export type Conclusion = {
  varName: string;  // LHS symbol name, or empty if conclusion is a tuple pattern
  typeName: string; // RHS type
  fields: string[]; // tuple-shaped conclusions like [A B C]; empty otherwise
};

export type Rule = {
  premises: Premise[];
  verified: VerifiedPremise[];
  conclusion: Conclusion;
};

export type Datatype = {
  name: string;
  rules: Rule[];
};

export type TypeSig = {
  paramTypes: string[];
  returnType: string;
};

export type Clause = {
  patterns: string[];   // one per parameter, as raw strings
  guard: string | null; // the `where EXPR` text, or null
  body: string;         // the RHS expression
};

export type Define = {
  name: string;
  typeSig: TypeSig;
  clauses: Clause[];
  paramNames: string[]; // derived from the first clause
};

export type SpecFile = {
  datatypes: Datatype[];
  defines: Define[];
};

// --- Public API ---

export function parseFile(src: string): SpecFile {
  const content = stripShenComments(src);
  const datatypes: Datatype[] = [];
  const defines: Define[] = [];

  for (const block of extractBlocks(content, "(datatype ")) {
    const dt = parseDatatype(block);
    if (dt) datatypes.push(dt);
  }

  for (const block of extractBlocks(content, "(define ")) {
    const def = parseDefine(block);
    if (def) defines.push(def);
  }

  return { datatypes, defines };
}

export function findDefine(spec: SpecFile, name: string): Define | null {
  for (const d of spec.defines) {
    if (d.name === name) return d;
  }
  return null;
}

export function findDatatype(spec: SpecFile, name: string): Datatype | null {
  for (const d of spec.datatypes) {
    if (d.name === name) return d;
  }
  return null;
}

// --- Comment stripping ---

// Removes `\* ... *\` block comments from Shen source. Matches Go's
// stripShenComments byte-by-byte.
export function stripShenComments(s: string): string {
  let out = "";
  let i = 0;
  while (i < s.length) {
    if (i + 1 < s.length && s[i] === "\\" && s[i + 1] === "*") {
      const end = s.indexOf("*\\", i + 2);
      if (end === -1) break; // unterminated — drop rest
      i = end + 2;
      continue;
    }
    out += s[i];
    i++;
  }
  return out;
}

// --- Balanced-paren block extraction ---

export function extractBlocks(content: string, prefix: string): string[] {
  const blocks: string[] = [];
  let remaining = content;
  while (true) {
    const idx = remaining.indexOf(prefix);
    if (idx === -1) break;
    remaining = remaining.slice(idx);
    let depth = 0;
    let end = -1;
    for (let i = 0; i < remaining.length; i++) {
      const ch = remaining[i];
      if (ch === "(") {
        depth++;
      } else if (ch === ")") {
        depth--;
        if (depth === 0) {
          end = i + 1;
          break;
        }
      }
    }
    if (end === -1) break;
    blocks.push(remaining.slice(0, end));
    remaining = remaining.slice(end);
  }
  return blocks;
}

// --- Datatype parser ---

function allChar(s: string, ch: string): boolean {
  if (s.length === 0) return false;
  for (const c of s) {
    if (c !== ch) return false;
  }
  return true;
}

export function parseDatatype(block: string): Datatype | null {
  if (!block.startsWith("(datatype ")) return null;
  block = block.slice("(datatype ".length);
  const nlIdx = block.indexOf("\n");
  if (nlIdx === -1) return null;
  const name = block.slice(0, nlIdx).trim();
  // Strip trailing " \t\n)" characters.
  let end = block.length;
  while (end > nlIdx) {
    const c = block[end - 1];
    if (c === " " || c === "\t" || c === "\n" || c === ")") end--;
    else break;
  }
  const body = block.slice(nlIdx, end);

  const lines = body.split("\n");
  const rules: Rule[] = [];
  let premLines: string[] = [];
  let concLines: string[] = [];
  let seenInf = false;

  const flush = () => {
    if (concLines.length === 0) return;
    const r = buildRule(premLines, concLines);
    if (r) rules.push(r);
  };

  for (const line of lines) {
    const t = line.trim();
    if (t === "") continue;
    if (t.length >= 3 && (allChar(t, "=") || allChar(t, "_"))) {
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

  if (rules.length === 0) return null;
  return { name, rules };
}

function buildRule(premLines: string[], concLines: string[]): Rule | null {
  const premises: Premise[] = [];
  const verified: VerifiedPremise[] = [];

  for (const rawLine of premLines) {
    let line = rawLine.trim();
    if (line.endsWith(";")) line = line.slice(0, -1).trim();
    if (line === "") continue;

    if (line.endsWith(": verified")) {
      const expr = line.slice(0, line.length - ": verified".length).trim();
      // raw holds just the stripped expression — downstream consumers
      // (typetable/samples/harness) parse it directly via parseSexpr.
      verified.push({ raw: expr, varName: "", expr });
      continue;
    }
    if (line.startsWith("if ")) {
      const expr = line.slice(3).trim();
      verified.push({ raw: expr, varName: "", expr });
      continue;
    }
    const idx = line.indexOf(" : ");
    if (idx !== -1) {
      premises.push({
        varName: line.slice(0, idx).trim(),
        typeName: line.slice(idx + 3).trim(),
      });
    }
  }

  let concStr = concLines.join(" ").trim();
  if (concStr.endsWith(";")) concStr = concStr.slice(0, -1).trim();
  if (concStr.includes(">>")) return null;
  const idx = concStr.indexOf(" : ");
  if (idx === -1) return null;
  const lhs = concStr.slice(0, idx).trim();
  const rhs = concStr.slice(idx + 3).trim();

  const conclusion: Conclusion = { varName: "", typeName: rhs, fields: [] };
  if (lhs.startsWith("[") && lhs.endsWith("]")) {
    const inner = lhs.slice(1, -1).trim();
    conclusion.fields = inner === "" ? [] : inner.split(/\s+/);
  } else {
    conclusion.varName = lhs;
  }
  return { premises, verified, conclusion };
}

// --- Define parser ---

export function parseDefine(block: string): Define | null {
  if (!block.startsWith("(define ")) return null;
  let inner = block.slice("(define ".length);
  if (inner.endsWith(")")) inner = inner.slice(0, -1);
  inner = inner.trim();

  // First token is the name.
  const nameEnd = firstWhitespace(inner);
  if (nameEnd === -1) {
    throw new Error("define: missing body");
  }
  const name = inner.slice(0, nameEnd).trim();
  let rest = inner.slice(nameEnd).trim();

  // Optional {...} type signature.
  let sig: TypeSig = { paramTypes: [], returnType: "" };
  if (rest.startsWith("{")) {
    const sigEnd = rest.indexOf("}");
    if (sigEnd === -1) {
      throw new Error(`define ${name}: unterminated type signature`);
    }
    sig = parseTypeSig(rest.slice(0, sigEnd + 1));
    rest = rest.slice(sigEnd + 1).trim();
  }

  const clauses = parseDefineClauses(rest);
  if (clauses.length === 0) {
    throw new Error(`define ${name}: no clauses`);
  }

  const paramNames = deriveParamNames(clauses[0].patterns);

  if (sig.paramTypes.length > 0 && paramNames.length !== sig.paramTypes.length) {
    throw new Error(
      `define ${name}: ${paramNames.length} params in first clause, ${sig.paramTypes.length} in type sig`,
    );
  }

  return { name, typeSig: sig, clauses, paramNames };
}

function firstWhitespace(s: string): number {
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c === " " || c === "\t" || c === "\n") return i;
  }
  return -1;
}

export function parseDefineClauses(body: string): Clause[] {
  body = body.trim();
  if (body === "") {
    throw new Error("empty define body");
  }

  // Collapse whitespace and split on " -> ".
  const bodyOneLine = body.split(/\s+/).filter((x) => x.length > 0).join(" ");
  const segments = bodyOneLine.split(" -> ");
  if (segments.length < 2) {
    throw new Error("missing '->' in define body");
  }

  const clauses: Clause[] = [];
  let currentPatterns = segments[0];

  for (let i = 1; i < segments.length; i++) {
    const seg = segments[i];
    let resultStr = "";
    let guardStr: string | null = null;
    let nextPatterns = "";

    const whereIdx = seg.indexOf(" where ");
    if (whereIdx !== -1) {
      resultStr = seg.slice(0, whereIdx).trim();
      const afterWhere = seg.slice(whereIdx + " where ".length).trim();
      if (afterWhere.startsWith("(")) {
        const [expr, endIdx] = extractBalancedParen(afterWhere);
        guardStr = expr;
        nextPatterns = afterWhere.slice(endIdx).trim();
      } else {
        const toks = afterWhere.split(/\s+/).filter((x) => x.length > 0);
        if (toks.length === 0) {
          throw new Error(`empty \`where\` guard in clause ${i}`);
        }
        guardStr = toks[0];
        nextPatterns = toks.slice(1).join(" ");
      }
    } else {
      const trimmed = seg.trim();
      if (trimmed.startsWith("(")) {
        const [expr, endIdx] = extractBalancedParen(trimmed);
        resultStr = expr;
        nextPatterns = trimmed.slice(endIdx).trim();
      } else {
        const toks = trimmed.split(/\s+/).filter((x) => x.length > 0);
        if (toks.length === 0) {
          throw new Error(`empty clause result at segment ${i}`);
        }
        resultStr = toks[0];
        if (toks.length > 1) {
          nextPatterns = toks.slice(1).join(" ");
        }
      }
    }

    const patternStrs = splitPatterns(currentPatterns);
    if (patternStrs.length === 0) {
      throw new Error(`clause ${clauses.length}: no patterns`);
    }

    clauses.push({
      patterns: patternStrs,
      guard: guardStr,
      body: resultStr,
    });
    currentPatterns = nextPatterns;
  }

  // Sanity: same arity across clauses.
  const want = clauses[0].patterns.length;
  for (let i = 0; i < clauses.length; i++) {
    if (clauses[i].patterns.length !== want) {
      throw new Error(
        `clause ${i} has ${clauses[i].patterns.length} patterns, expected ${want}`,
      );
    }
  }

  return clauses;
}

// splitPatterns tokenizes a pattern string respecting bracket nesting.
// "[Med | Meds]" stays as one token; "[[X Y] | Rest]" stays as one token.
export function splitPatterns(s: string): string[] {
  const patterns: string[] = [];
  let current = "";
  let depth = 0;
  const flush = () => {
    if (current.length > 0) {
      patterns.push(current);
      current = "";
    }
  };
  for (const ch of s) {
    if (ch === "[") {
      depth++;
      current += ch;
    } else if (ch === "]") {
      depth--;
      current += ch;
      if (depth === 0) flush();
    } else if (ch === " " || ch === "\t") {
      if (depth > 0) current += ch;
      else flush();
    } else {
      current += ch;
    }
  }
  flush();
  return patterns;
}

// extractBalancedParen returns the balanced parenthesized expression at the
// start of s and the index just past its end. Returns ["", 0] if s does not
// start with '('.
export function extractBalancedParen(s: string): [string, number] {
  if (s.length === 0 || s[0] !== "(") return ["", 0];
  let depth = 0;
  for (let i = 0; i < s.length; i++) {
    const ch = s[i];
    if (ch === "(") depth++;
    else if (ch === ")") {
      depth--;
      if (depth === 0) return [s.slice(0, i + 1), i + 1];
    }
  }
  return [s, s.length];
}

// deriveParamNames inspects the first clause's patterns. Simple
// uppercase-symbol patterns keep their names; everything else becomes p0,
// p1, ...
export function deriveParamNames(patterns: string[]): string[] {
  const out: string[] = [];
  for (let i = 0; i < patterns.length; i++) {
    const p = patterns[i];
    let name = `p${i}`;
    if (isSimpleUpperSymbol(p)) name = p;
    out.push(name);
  }
  return out;
}

function isSimpleUpperSymbol(s: string): boolean {
  if (s.length === 0) return false;
  const first = s[0];
  if (first < "A" || first > "Z") return false;
  // Must be a symbol-ish token (no brackets, parens, or whitespace).
  for (const c of s) {
    if (c === "[" || c === "]" || c === "(" || c === ")" || c === " " || c === "\t") {
      return false;
    }
  }
  return true;
}

// parseTypeSig parses "{a --> b --> c}" into paramTypes=[a, b], returnType=c.
export function parseTypeSig(sig: string): TypeSig {
  sig = sig.trim();
  if (!sig.startsWith("{") || !sig.endsWith("}")) {
    throw new Error("type sig must be wrapped in {...}");
  }
  const inner = sig.slice(1, -1).trim();

  const parts = splitArrow(inner);
  if (parts.length < 2) {
    throw new Error(`type sig needs at least one arrow: ${JSON.stringify(sig)}`);
  }
  const trimmed = parts.map((p) => p.trim());
  return {
    paramTypes: trimmed.slice(0, trimmed.length - 1),
    returnType: trimmed[trimmed.length - 1],
  };
}

// splitArrow splits on " --> " outside of parentheses.
function splitArrow(s: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c === "(") depth++;
    else if (c === ")") depth--;
    if (depth === 0 && i + 4 < s.length && s.slice(i, i + 5) === " --> ") {
      parts.push(s.slice(start, i));
      start = i + 5;
      i += 4;
    }
  }
  parts.push(s.slice(start));
  return parts;
}
