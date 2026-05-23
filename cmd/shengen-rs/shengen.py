#!/usr/bin/env python3
"""shengen-rs — Generate Rust guard types from Shen sequent-calculus specs.

Architecture: Parse → Symbol Table → Resolve → Emit (mirrors Go/TS shengen).
Supports --mode standard|hardened.

Usage:
    python3 shengen.py <spec-file> --out <output-file> [--mode standard|hardened] [--mod shenguard]
"""

import argparse
import re
import sys
from dataclasses import dataclass, field
from typing import Optional


# ---------------------------------------------------------------------------
# AST
# ---------------------------------------------------------------------------

@dataclass
class Premise:
    var_name: str
    type_name: str

@dataclass
class VerifiedPremise:
    raw: str

@dataclass
class Conclusion:
    fields: list[str]  # empty for wrapped
    type_name: str
    is_composite: bool

@dataclass
class Rule:
    premises: list[Premise]
    verified: list[VerifiedPremise]
    conclusion: Conclusion

@dataclass
class Datatype:
    name: str
    rules: list[Rule]


@dataclass
class DefineClause:
    patterns: list[str]
    result: str
    guard: str = ""


@dataclass
class Define:
    name: str
    clauses: list[DefineClause] = field(default_factory=list)
    param_types: list[str] = field(default_factory=list)
    return_type: str = ""

@dataclass
class FieldInfo:
    index: int
    shen_name: str
    shen_type: str

@dataclass
class TypeInfo:
    shen_name: str
    rust_name: str
    category: str  # wrapper|constrained|composite|guarded|alias|sumtype
    fields: list[FieldInfo] = field(default_factory=list)
    wrapped_prim: Optional[str] = None
    wrapped_type: Optional[str] = None
    variants: list[str] = field(default_factory=list)

@dataclass
class SExpr:
    atom: Optional[str] = None
    children: Optional[list] = None

    def is_atom(self): return self.atom is not None
    def is_call(self): return self.children is not None and len(self.children) > 0
    def op(self): return self.children[0].atom if self.is_call() else None


# ---------------------------------------------------------------------------
# Parser
# ---------------------------------------------------------------------------

PRIMITIVES = {"string": "String", "number": "f64", "symbol": "String", "boolean": "bool"}

def parse_file(path: str) -> list[Datatype]:
    """Backwards-compatible entry point — returns only datatypes."""
    datatypes, _ = parse_file_full(path)
    return datatypes


def parse_file_full(path: str) -> tuple[list[Datatype], list[Define]]:
    """Parse a Shen spec, returning both datatype and define blocks."""
    with open(path) as f:
        text = f.read()
    text = re.sub(r'\\\*.*?\*\\', '', text, flags=re.DOTALL)
    datatypes = []
    defines = []
    for block in _extract_blocks(text, '(datatype '):
        dt = _parse_datatype_block(block)
        if dt is not None:
            datatypes.append(dt)
    for block in _extract_blocks(text, '(define '):
        df = parse_define(block)
        if df is not None:
            defines.append(df)
    return datatypes, defines


def _extract_blocks(text: str, prefix: str) -> list[str]:
    blocks = []
    remaining = text
    while True:
        idx = remaining.find(prefix)
        if idx == -1:
            break
        remaining = remaining[idx:]
        depth = 0
        end = -1
        for i, ch in enumerate(remaining):
            if ch == '(':
                depth += 1
            elif ch == ')':
                depth -= 1
                if depth == 0:
                    end = i + 1
                    break
        if end == -1:
            break
        blocks.append(remaining[:end])
        remaining = remaining[end:]
    return blocks


def _parse_datatype_block(block: str) -> Optional[Datatype]:
    m = re.match(r'\(datatype\s+([\w-]+)', block)
    if not m:
        return None
    name = m.group(1)
    body = block[m.end():-1]
    rules = parse_rules(body)
    if not rules:
        return None
    return Datatype(name=name, rules=rules)


def parse_define(block: str) -> Optional[Define]:
    """Parse a (define name {sig?} body) block. See shengen-py for the
    detailed algorithm — this Rust emitter port duplicates the parser
    logic verbatim because the two emitters do not share code."""
    block = block.strip()
    if not block.startswith('(define '):
        return None
    block = block[len('(define '):]
    nl_idx = block.find('\n')
    if nl_idx == -1:
        return None
    name = block[:nl_idx].strip()
    body = block[nl_idx:].rstrip(' \t\n)')

    param_types: list[str] = []
    return_type: str = ""
    body_one = ' '.join(body.split())
    sig_match = re.match(r'\s*\{(.+?)\}\s*(.*)', body_one)
    if sig_match:
        sig_inner = sig_match.group(1).strip()
        body_one = sig_match.group(2)
        sig_parts = [p.strip() for p in sig_inner.split(' --> ')]
        if len(sig_parts) >= 2:
            param_types = sig_parts[:-1]
            return_type = sig_parts[-1]

    segments = body_one.split(' -> ')
    if len(segments) < 2:
        return None

    define = Define(name=name, param_types=param_types, return_type=return_type)
    current_patterns = segments[0]

    for i in range(1, len(segments)):
        seg = segments[i]
        result = ""
        guard = ""
        next_patterns = ""

        where_idx = seg.find(' where ')
        if where_idx != -1:
            result = seg[:where_idx].strip()
            after_where = seg[where_idx + 7:].strip()
            if after_where.startswith('('):
                guard_expr, end_idx = _extract_balanced_paren(after_where)
                guard = guard_expr
                next_patterns = after_where[end_idx:].strip()
            else:
                guard = after_where
        else:
            seg = seg.strip()
            if seg.startswith('('):
                expr, end_idx = _extract_balanced_paren(seg)
                result = expr
                next_patterns = seg[end_idx:].strip()
            else:
                tokens = seg.split()
                result = tokens[0]
                if len(tokens) > 1:
                    next_patterns = ' '.join(tokens[1:])

        result = result.rstrip(')')
        result = result.strip()
        if result.startswith('('):
            r, _ = _extract_balanced_paren(result + ')')
            if r:
                result = r

        patterns = _split_patterns(current_patterns)
        if patterns:
            define.clauses.append(DefineClause(patterns=patterns, result=result, guard=guard))

        current_patterns = next_patterns

    if not define.clauses:
        return None
    return define


def _split_patterns(s: str) -> list[str]:
    patterns: list[str] = []
    current: list[str] = []
    depth = 0
    for ch in s:
        if ch == '[':
            depth += 1
            current.append(ch)
        elif ch == ']':
            depth -= 1
            current.append(ch)
            if depth == 0 and current:
                patterns.append(''.join(current))
                current = []
        elif ch in (' ', '\t'):
            if depth > 0:
                current.append(ch)
            elif current:
                patterns.append(''.join(current))
                current = []
        else:
            current.append(ch)
    if current:
        patterns.append(''.join(current))
    return patterns


def _extract_balanced_paren(s: str) -> tuple[str, int]:
    if not s or s[0] != '(':
        return "", 0
    depth = 0
    for i, ch in enumerate(s):
        if ch == '(':
            depth += 1
        elif ch == ')':
            depth -= 1
            if depth == 0:
                return s[:i + 1], i + 1
    return s, len(s)

def parse_rules(body: str) -> list[Rule]:
    # Split by inference line (3+ = or _)
    parts = re.split(r'\n\s*[=_]{3,}\s*\n', body)
    if len(parts) < 2:
        return []
    rules = []
    for i in range(0, len(parts) - 1, 2):
        premises_text = parts[i]
        conclusion_text = parts[i + 1] if i + 1 < len(parts) else ""
        premises, verified = parse_premises(premises_text)
        conclusion = parse_conclusion(conclusion_text)
        if conclusion:
            rules.append(Rule(premises=premises, verified=verified, conclusion=conclusion))
    return rules

def parse_premises(text: str) -> tuple[list[Premise], list[VerifiedPremise]]:
    premises = []
    verified = []
    for line in text.strip().split(';'):
        line = line.strip()
        if not line:
            continue
        if '>>' in line:
            continue
        # Verified premise: (expr) : verified
        vm = re.match(r'(.+?)\s*:\s*verified\s*$', line)
        if vm:
            verified.append(VerifiedPremise(raw=vm.group(1).strip()))
            continue
        # Side condition: if (expr)
        if line.startswith('if '):
            verified.append(VerifiedPremise(raw=line[3:].strip()))
            continue
        # Type judgment: Var : type — supports either a plain identifier
        # (e.g. `X : amount`) or the parametric list form
        # (e.g. `Refs : (list tag-id)`).
        tm = re.match(r'(\w+)\s*:\s*(\(list\s+[\w-]+\)|[\w-]+)\s*$', line)
        if tm:
            premises.append(Premise(var_name=tm.group(1), type_name=tm.group(2).strip()))
    return premises, verified

def parse_conclusion(text: str) -> Optional[Conclusion]:
    text = text.strip().rstrip(';').rstrip(')').strip()
    if not text or '>>' in text:
        return None
    # Composite: [A B C] : type-name
    cm = re.match(r'\[([^\]]+)\]\s*:\s*([\w-]+)', text)
    if cm:
        fields = cm.group(1).split()
        return Conclusion(fields=fields, type_name=cm.group(2), is_composite=True)
    # Wrapped: X : type-name
    wm = re.match(r'(\w+)\s*:\s*([\w-]+)', text)
    if wm:
        return Conclusion(fields=[], type_name=wm.group(2), is_composite=False)
    return None


# ---------------------------------------------------------------------------
# Symbol Table
# ---------------------------------------------------------------------------

def build_symbol_table(datatypes: list[Datatype]) -> dict[str, TypeInfo]:
    conc_count: dict[str, int] = {}
    for dt in datatypes:
        for rule in dt.rules:
            conc_count[rule.conclusion.type_name] = conc_count.get(rule.conclusion.type_name, 0) + 1

    table: dict[str, TypeInfo] = {}
    sum_types: dict[str, list[str]] = {}

    for dt in datatypes:
        for rule in dt.rules:
            ctype = rule.conclusion.type_name
            # Name resolution
            if dt.name != ctype and conc_count.get(ctype, 0) > 1:
                type_name = dt.name
                sum_types.setdefault(ctype, []).append(dt.name)
            else:
                type_name = ctype

            info = TypeInfo(shen_name=type_name, rust_name=to_pascal(type_name), category=classify(rule))

            if rule.conclusion.is_composite:
                prem_map = {p.var_name: p.type_name for p in rule.premises}
                for i, fname in enumerate(rule.conclusion.fields):
                    info.fields.append(FieldInfo(index=i, shen_name=fname, shen_type=prem_map.get(fname, "unknown")))

            if info.category in ("wrapper", "constrained"):
                if rule.premises:
                    info.wrapped_prim = rule.premises[0].type_name
            elif info.category == "alias":
                if rule.premises:
                    info.wrapped_type = rule.premises[0].type_name

            table[type_name] = info

    for ctype, variants in sum_types.items():
        st_info = TypeInfo(shen_name=ctype, rust_name=to_pascal(ctype), category="sumtype", variants=variants)
        table[ctype] = st_info

    return table

def classify(rule: Rule) -> str:
    c = rule.conclusion
    p = rule.premises
    v = rule.verified
    if not c.is_composite and len(v) == 0 and len(p) == 1 and p[0].type_name in PRIMITIVES:
        return "wrapper"
    if not c.is_composite and len(v) > 0 and len(p) >= 1 and p[0].type_name in PRIMITIVES:
        return "constrained"
    if not c.is_composite and len(p) == 1 and p[0].type_name not in PRIMITIVES:
        return "alias"
    if c.is_composite and len(v) > 0:
        return "guarded"
    return "composite"


# ---------------------------------------------------------------------------
# S-Expression Parser
# ---------------------------------------------------------------------------

def parse_sexpr(text: str) -> SExpr:
    tokens = tokenize_sexpr(text.strip())
    expr, _ = parse_sexpr_tokens(tokens, 0)
    return expr

def tokenize_sexpr(text: str) -> list[str]:
    tokens = []
    i = 0
    while i < len(text):
        if text[i] in ' \t\n':
            i += 1
        elif text[i] == '(':
            tokens.append('(')
            i += 1
        elif text[i] == ')':
            tokens.append(')')
            i += 1
        elif text[i] == '[':
            tokens.append('[')
            i += 1
        elif text[i] == ']':
            tokens.append(']')
            i += 1
        else:
            j = i
            while j < len(text) and text[j] not in ' \t\n()[]':
                j += 1
            tokens.append(text[i:j])
            i = j
    return tokens

def parse_sexpr_tokens(tokens: list[str], pos: int) -> tuple[SExpr, int]:
    if pos >= len(tokens):
        return SExpr(atom=""), pos
    if tokens[pos] == '(':
        children = []
        pos += 1
        while pos < len(tokens) and tokens[pos] != ')':
            child, pos = parse_sexpr_tokens(tokens, pos)
            children.append(child)
        if pos < len(tokens):
            pos += 1  # skip )
        return SExpr(children=children), pos
    return SExpr(atom=tokens[pos]), pos + 1


# ---------------------------------------------------------------------------
# Resolver
# ---------------------------------------------------------------------------

@dataclass
class Resolved:
    code: str
    typ: str = "unknown"
    is_multi: bool = False
    base_code: str = ""
    remaining: list[FieldInfo] = field(default_factory=list)

def resolve(expr: SExpr, var_map: dict[str, str], st: dict[str, TypeInfo]) -> Resolved:
    if expr.is_atom():
        a = expr.atom
        if a and a[0].isdigit() or (a and a[0] == '-' and len(a) > 1):
            return Resolved(code=a + (".0" if '.' not in a else ""), typ="number")
        if a in var_map:
            return Resolved(code=to_snake(a), typ=var_map[a])
        if a and a[0] == '"':
            return Resolved(code=a, typ="string")
        return Resolved(code=a or "", typ="unknown")

    if not expr.is_call():
        return Resolved(code="/* unresolved */", typ="unknown")

    op = expr.op()
    if op in ("head", "tail"):
        return resolve_head_tail(expr, var_map, st, op == "head")
    if op == "not":
        inner = resolve(expr.children[1], var_map, st)
        return Resolved(code=f"!({inner.code})", typ="boolean")
    if op == "shen.mod":
        lhs = resolve(expr.children[1], var_map, st)
        rhs = resolve(expr.children[2], var_map, st)
        return Resolved(code=f"({unwrap_num(lhs, st)} as i64) % ({rhs.code} as i64)", typ="number")
    if op == "length":
        inner = resolve(expr.children[1], var_map, st)
        return Resolved(code=f"{unwrap_str(inner, st)}.len()", typ="number")
    if op == "element?":
        var_expr = resolve(expr.children[1], var_map, st)
        members = []
        for c in expr.children[2:]:
            if c.is_atom():
                a = c.atom.strip('[]')
                if a:
                    members.append(f'"{a}"')
        code = f'[{", ".join(members)}].contains(&{unwrap_str(var_expr, st)})'
        return Resolved(code=code, typ="boolean")
    return Resolved(code="/* unresolved */", typ="unknown")

def resolve_head_tail(expr: SExpr, var_map: dict, st: dict, is_head: bool) -> Resolved:
    inner = resolve(expr.children[1], var_map, st)
    if inner.is_multi:
        fields = inner.remaining
        return access_fields(inner.base_code, fields, is_head, st)
    ti = st.get(inner.typ)
    if not ti or not ti.fields:
        return Resolved(code="/* unresolved head/tail */", typ="unknown")
    return access_fields(inner.code, ti.fields, is_head, st)

def access_fields(base: str, fields: list[FieldInfo], is_head: bool, st: dict) -> Resolved:
    if is_head:
        f = fields[0]
        accessor = to_snake(f.shen_name)
        return Resolved(code=f"{base}.{accessor}()", typ=f.shen_type)
    remaining = fields[1:]
    if len(remaining) == 0:
        return Resolved(code="/* empty tail */", typ="unknown")
    if len(remaining) == 1:
        f = remaining[0]
        accessor = to_snake(f.shen_name)
        return Resolved(code=f"{base}.{accessor}()", typ=f.shen_type)
    return Resolved(code=base, typ="multi", is_multi=True, base_code=base, remaining=remaining)

def unwrap_num(r: Resolved, st: dict) -> str:
    ti = st.get(r.typ)
    if ti and ti.category in ("wrapper", "constrained"):
        return f"{r.code}.val()"
    return r.code

def unwrap_str(r: Resolved, st: dict) -> str:
    ti = st.get(r.typ)
    if ti and ti.category in ("wrapper", "constrained"):
        return f"{r.code}.val()"
    return r.code

def translate_verified(vp: VerifiedPremise, var_map: dict, st: dict) -> tuple[str, str]:
    expr = parse_sexpr(vp.raw)
    if not expr.is_call():
        return ("true /* TODO */", vp.raw)
    op = expr.op()
    if op in (">=", "<=", ">", "<"):
        lhs = resolve(expr.children[1], var_map, st)
        rhs = resolve(expr.children[2], var_map, st)
        code = f"{unwrap_num(lhs, st)} {op} {unwrap_num(rhs, st)}"
        msg = f"{lhs.code} must be {op} {rhs.code}"
        return (code, msg)
    if op == "=":
        lhs = resolve(expr.children[1], var_map, st)
        rhs = resolve(expr.children[2], var_map, st)
        code = f"{unwrap_num(lhs, st)} == {unwrap_num(rhs, st)}"
        msg = f"{lhs.code} must equal {rhs.code}"
        return (code, msg)
    if op == "not":
        inner_expr = expr.children[1]
        # (not (= LHS RHS)) — prefer human-readable messages:
        #   (not (= X ""))  → "X must not be empty"
        #   (not (= X Y))   → "X must not equal Y"
        if inner_expr.is_call() and inner_expr.op() == "=" and len(inner_expr.children) == 3:
            lhs = resolve(inner_expr.children[1], var_map, st)
            rhs = resolve(inner_expr.children[2], var_map, st)
            code = f"!({unwrap_num(lhs, st)} == {unwrap_num(rhs, st)})"
            if _is_empty_string_literal(rhs.code):
                return (code, f"{lhs.code} must not be empty")
            if _is_empty_string_literal(lhs.code):
                return (code, f"{rhs.code} must not be empty")
            return (code, f"{lhs.code} must not equal {rhs.code}")
        inner = resolve(inner_expr, var_map, st)
        return (f"!({inner.code})", f"not {vp.raw}")
    if op == "element?":
        r = resolve(expr, var_map, st)
        return (r.code, f"must be a valid member")
    return ("true /* TODO: " + vp.raw + " */", vp.raw)


def _is_empty_string_literal(code: str) -> bool:
    # Rust string literal form. The resolver emits `""` for the empty literal
    # in both source positions; matching exactly avoids partial-string false
    # positives like `b""`.
    return code == '""'


def negate_rust_expr(rust_expr: str) -> str:
    """Boolean negation of a Rust expression with a peephole.

    The gate site wraps the verified predicate in `!(...)` for the violation
    branch. When the inner predicate already starts with `!(`, that produces
    `!(!(...))` — mathematically correct but ugly in the generated source.
    Strip the redundant outer `!` instead.
    """
    inner = _strip_outer_not_paren(rust_expr)
    if inner is not None:
        return f"({inner})"
    return f"!({rust_expr})"


def _strip_outer_not_paren(rust_expr: str) -> Optional[str]:
    if not rust_expr.startswith("!(") or not rust_expr.endswith(")"):
        return None
    depth = 1
    body = rust_expr[2:-1]
    for ch in body:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                # The opening `(` after `!` closed before the end — outer `!`
                # does not bracket the whole expression.
                return None
    if depth != 1:
        return None
    return body


# ---------------------------------------------------------------------------
# Emitter
# ---------------------------------------------------------------------------

def emit_rust(
    datatypes: list[Datatype],
    st: dict[str, TypeInfo],
    spec_path: str,
    mod_name: str,
    mode: str,
    defines: Optional[list[Define]] = None,
) -> str:
    lines = []
    lines.append(f"// Code generated by shengen-rs from {spec_path}. DO NOT EDIT.")
    lines.append("//")
    lines.append("// These types enforce Shen sequent-calculus invariants at the Rust level.")
    lines.append("// Constructors are the ONLY way to create these types — bypassing them")
    lines.append("// is a violation of the formal spec.")
    if mode == "hardened":
        lines.append("//")
        lines.append("// HARDENED MODE: No Clone/Copy on guarded types, #[non_exhaustive],")
        lines.append("// no Deserialize — use from_json() instead.")
    lines.append("")
    lines.append(f"mod {mod_name} {{")
    lines.append("    use std::fmt;")
    lines.append("")
    lines.append("    #[derive(Debug, Clone)]")
    lines.append("    pub struct GuardError {")
    lines.append("        pub message: String,")
    lines.append("    }")
    lines.append("")
    lines.append("    impl fmt::Display for GuardError {")
    lines.append('        fn fmt(&self, f: &mut fmt::Formatter<\'_>) -> fmt::Result {')
    lines.append('            write!(f, "GuardError: {}", self.message)')
    lines.append("        }")
    lines.append("    }")
    lines.append("")
    lines.append("    impl std::error::Error for GuardError {}")
    lines.append("")

    # Emit sum types first (trait definitions)
    for name, info in st.items():
        if info.category == "sumtype":
            lines.extend(emit_sumtype(info, mode))

    # Emit regular types
    for dt in datatypes:
        for rule in dt.rules:
            ctype = rule.conclusion.type_name
            if dt.name != ctype and st.get(ctype, TypeInfo(shen_name="", rust_name="", category="")).category == "sumtype":
                type_name = dt.name
            else:
                type_name = ctype
            info = st.get(type_name)
            if not info or info.category == "sumtype":
                continue
            lines.extend(emit_type(info, rule, st, mode))

    # Pure-function helpers translated from (define …) blocks.
    if defines:
        lines.extend(emit_defines(defines, st))

    lines.append("}")
    lines.append("")
    return "\n".join(lines)

def emit_sumtype(info: TypeInfo, mode: str) -> list[str]:
    lines = []
    if mode == "hardened":
        lines.append("    mod sealed {")
        lines.append("        pub trait Sealed {}")
        lines.append("    }")
        lines.append("")
        lines.append(f"    pub trait {info.rust_name}: sealed::Sealed {{")
    else:
        lines.append(f"    pub trait {info.rust_name} {{")
    lines.append(f"        fn is_{to_snake(info.shen_name)}(&self);")
    lines.append("    }")
    lines.append("")
    return lines

def emit_type(info: TypeInfo, rule: Rule, st: dict, mode: str) -> list[str]:
    lines = []
    cat = info.category
    lines.append(f"    // --- {info.rust_name} ---")
    lines.append(f"    // Shen: (datatype {info.shen_name})")

    # Derive attributes
    non_exhaustive = "    #[non_exhaustive]\n" if mode == "hardened" else ""
    if cat in ("wrapper", "constrained", "alias"):
        derive = "    #[derive(Clone)]\n" if mode == "hardened" else ""
    elif cat in ("guarded",):
        derive = ""  # No Clone on guarded types in hardened mode
        if mode == "standard":
            derive = ""
    else:
        derive = ""

    if cat == "alias":
        # Resolve the alias target through `field_rust_type` so parametric
        # constructs like `(list ref-table-entry)` render as `Vec<RefTableEntry>`
        # rather than the malformed `(list refTableEntry)`.
        target_type = field_rust_type(info.wrapped_type or "", st)
        lines.append(f"    pub type {info.rust_name} = {target_type};")
        lines.append("")
        return lines

    # Struct
    lines.append(non_exhaustive.rstrip() if non_exhaustive.strip() else "")
    if derive.strip():
        lines.append(derive.rstrip())
    lines = [l for l in lines if l != ""]  # clean empty
    lines.append(f"    pub struct {info.rust_name} {{")

    if cat in ("wrapper", "constrained"):
        rust_type = PRIMITIVES.get(info.wrapped_prim, "String")
        lines.append(f"        v: {rust_type},")
    else:
        for fi in info.fields:
            rust_type = field_rust_type(fi.shen_type, st)
            lines.append(f"        {to_snake(fi.shen_name)}: {rust_type},")
    lines.append("    }")
    lines.append("")

    # Constructor
    lines.append(f"    impl {info.rust_name} {{")
    if cat == "wrapper":
        rust_type = PRIMITIVES.get(info.wrapped_prim, "String")
        param = f"x: {rust_type}"
        if rust_type == "String":
            param = "x: String"
        lines.append(f"        pub fn new({param}) -> Self {{")
        lines.append(f"            {info.rust_name} {{ v: x }}")
        lines.append("        }")
    elif cat == "constrained":
        rust_type = PRIMITIVES.get(info.wrapped_prim, "String")
        lines.append(f"        pub fn new(x: {rust_type}) -> Result<Self, GuardError> {{")
        var_map = {rule.premises[0].var_name: rule.premises[0].type_name}
        for vp in rule.verified:
            code, msg = translate_verified(vp, var_map, st)
            lines.append(f"            if {negate_rust_expr(code)} {{")
            lines.append(f'                return Err(GuardError {{ message: format!("{msg}: {{}}", x) }});')
            lines.append("            }")
        lines.append(f"            Ok({info.rust_name} {{ v: x }})")
        lines.append("        }")
    elif cat in ("composite", "guarded"):
        var_map = {p.var_name: p.type_name for p in rule.premises}
        params = []
        for fi in info.fields:
            rust_type = field_rust_type(fi.shen_type, st)
            params.append(f"{to_snake(fi.shen_name)}: {rust_type}")
        param_str = ", ".join(params)
        returns_result = cat == "guarded"

        if returns_result:
            lines.append(f"        pub fn new({param_str}) -> Result<Self, GuardError> {{")
            for vp in rule.verified:
                code, msg = translate_verified(vp, var_map, st)
                lines.append(f"            if {negate_rust_expr(code)} {{")
                lines.append(f'                return Err(GuardError {{ message: "{msg}".to_string() }});')
                lines.append("            }")
            field_inits = ", ".join(to_snake(fi.shen_name) for fi in info.fields)
            lines.append(f"            Ok({info.rust_name} {{ {field_inits} }})")
        else:
            lines.append(f"        pub fn new({param_str}) -> Self {{")
            field_inits = ", ".join(to_snake(fi.shen_name) for fi in info.fields)
            lines.append(f"            {info.rust_name} {{ {field_inits} }}")
        lines.append("        }")

    lines.append("")

    # Accessors
    if cat in ("wrapper", "constrained"):
        rust_type = PRIMITIVES.get(info.wrapped_prim, "String")
        ret_type = f"&str" if rust_type == "String" else rust_type
        ret_expr = "&self.v" if rust_type == "String" else "self.v"
        lines.append(f"        pub fn val(&self) -> {ret_type} {{")
        lines.append(f"            {ret_expr}")
        lines.append("        }")
    else:
        for fi in info.fields:
            rust_type = field_rust_type(fi.shen_type, st)
            accessor = to_snake(fi.shen_name)
            elem = list_elem_type(fi.shen_type)
            if fi.shen_type in PRIMITIVES:
                prim_rust = PRIMITIVES[fi.shen_type]
                ret_type = f"&str" if prim_rust == "String" else prim_rust
                ret_expr = f"&self.{accessor}" if prim_rust == "String" else f"self.{accessor}"
            elif elem is not None:
                # Return a slice borrow rather than `&Vec<T>` for idiomatic Rust.
                elem_rust = field_rust_type(elem, st)
                ret_type = f"&[{elem_rust}]"
                ret_expr = f"&self.{accessor}"
            else:
                ret_type = f"&{rust_type}"
                ret_expr = f"&self.{accessor}"
            lines.append(f"        pub fn {accessor}(&self) -> {ret_type} {{")
            lines.append(f"            {ret_expr}")
            lines.append("        }")
            lines.append("")

    lines.append("    }")
    lines.append("")
    return lines

# ---------------------------------------------------------------------------
# Define Emission
# ---------------------------------------------------------------------------


class DefineEmitError(Exception):
    """Raised when a (define …) block uses constructs the Rust emitter cannot
    safely translate. The caller emits an `// unsupported` comment instead of
    invalid Rust."""


def define_rust_name(shen_name: str) -> str:
    """`ref-present?` → `ref_present` (Rust snake_case predicate convention).

    The trailing `?` is stripped; remaining hyphens map to underscores via
    `to_snake`. Hyphenated word-segments become single tokens.
    """
    return to_snake(shen_name.rstrip('?'))


def _extract_destructure_bindings(pattern: str) -> list[str]:
    """Mirror shengen-py's destructure parser."""
    if not pattern.startswith('['):
        return []
    inner = pattern[1:]
    pipe_idx = inner.find('|')
    if pipe_idx == -1:
        return []
    inner = inner[:pipe_idx].strip()
    if inner.startswith('['):
        if not inner.endswith(']'):
            return []
        inner = inner[1:-1].strip()
        return inner.split()
    parts = inner.split()
    if len(parts) == 1:
        return parts
    return []


def _analyze_define(define: Define) -> tuple[int, Optional[str]]:
    loop_idx = -1
    for clause in define.clauses:
        for j, pat in enumerate(clause.patterns):
            if '|' in pat:
                loop_idx = j
                break
        if loop_idx >= 0:
            break
    base_result: Optional[str] = None
    if loop_idx >= 0:
        for clause in define.clauses:
            if loop_idx < len(clause.patterns) and clause.patterns[loop_idx] == '[]':
                base_result = _result_to_rust(clause.result)
                break
    return loop_idx, base_result


def _result_to_rust(result: str) -> str:
    r = result.strip()
    if r == 'true':
        return 'true'
    if r == 'false':
        return 'false'
    return r


def _apply_replacements_once(text: str, repl_map: dict[str, str]) -> str:
    if not repl_map:
        return text
    keys = sorted(repl_map.keys(), key=len, reverse=True)
    pattern = re.compile(r'\b(' + '|'.join(re.escape(k) for k in keys) + r')\b')
    return pattern.sub(lambda m: repl_map[m.group(1)], text)


def _guard_to_rust(guard: str, var_map: dict[str, str], st: dict) -> str:
    if not guard.strip():
        return "true"
    expr = parse_sexpr(guard)
    if not expr.is_call():
        return "true"
    return _guard_expr_to_rust(expr, var_map, st)


def _guard_expr_to_rust(expr: SExpr, var_map: dict, st: dict) -> str:
    if expr.is_atom():
        a = expr.atom
        if a and (a[0].isdigit() or (a[0] == '-' and len(a) > 1)):
            return a
        if a and a[0] == '"':
            return a
        if a in var_map:
            r = Resolved(code=to_snake(a), typ=var_map[a])
            return unwrap_str(r, st)
        if a == 'true':
            return 'true'
        if a == 'false':
            return 'false'
        return to_snake(a) if a else ""

    op = expr.op()
    if op in ('and', 'or') and len(expr.children) == 3:
        l = _guard_expr_to_rust(expr.children[1], var_map, st)
        r = _guard_expr_to_rust(expr.children[2], var_map, st)
        rust_op = '&&' if op == 'and' else '||'
        return f"({l}) {rust_op} ({r})"
    if op == 'not' and len(expr.children) == 2:
        inner = _guard_expr_to_rust(expr.children[1], var_map, st)
        return f"!({inner})"
    if op == '=' and len(expr.children) == 3:
        l = _guard_expr_to_rust(expr.children[1], var_map, st)
        r = _guard_expr_to_rust(expr.children[2], var_map, st)
        return f"{l} == {r}"
    if op in ('>=', '<=', '>', '<') and len(expr.children) == 3:
        l = _guard_expr_to_rust(expr.children[1], var_map, st)
        r = _guard_expr_to_rust(expr.children[2], var_map, st)
        return f"{l} {op} {r}"
    if op == 'length' and len(expr.children) == 2:
        inner = _guard_expr_to_rust(expr.children[1], var_map, st)
        return f"{inner}.len()"
    return "true /* TODO: unsupported guard form */"


def emit_defines(defines: list[Define], st: dict[str, TypeInfo]) -> list[str]:
    if not defines:
        return []
    lines: list[str] = [
        "",
        "    // --- Pure functions translated from (define …) blocks ---",
        "    //",
        "    // The Rust emitter is intentionally conservative: it emits only",
        "    // predicate-style defines that recurse over a single list",
        "    // parameter and have a `[]` base case. Anything more complex is",
        "    // flagged below — Rust's borrow rules make silent corruption far",
        "    // worse than a TODO comment.",
        "",
    ]
    for define in defines:
        try:
            block = _emit_one_define(define, st)
        except DefineEmitError as exc:
            block = [f"    // unsupported: define {define.name!r} — {exc}", ""]
        lines.extend(block)
    return lines


def _emit_one_define(define: Define, st: dict[str, TypeInfo]) -> list[str]:
    rust_name = define_rust_name(define.name)
    loop_idx, base_result = _analyze_define(define)
    if loop_idx < 0:
        raise DefineEmitError(
            "no list-iteration clause (Rust emitter only handles defines that "
            "destructure a single list parameter)"
        )
    if base_result is None:
        if define.return_type == "boolean":
            base_result = "false"
        else:
            raise DefineEmitError(
                "no `[]` base case and return type is not boolean — emitter "
                "cannot synthesise a fallback value"
            )

    # Only emit defines that return Copy types. Rust's borrow rules turn
    # owned-return defines (returning structs, lists, ...) into a clone-vs-
    # reference design decision the emitter is too conservative to make.
    if define.return_type not in ("", "boolean", "number"):
        raise DefineEmitError(
            f"return type {define.return_type!r} not yet supported (only "
            "boolean/number return types)"
        )

    arity = max(len(c.patterns) for c in define.clauses) if define.clauses else 0
    if arity == 0:
        raise DefineEmitError("no clauses")

    # Build parameter names. Escape Rust keywords with a trailing underscore.
    param_names: list[str] = []
    for i in range(arity):
        name: Optional[str] = None
        for clause in define.clauses:
            if i >= len(clause.patterns):
                continue
            pat = clause.patterns[i]
            if pat in ('_', '[]') or pat.startswith('['):
                continue
            name = rust_ident(pat)
            break
        if name is None and i < len(define.param_types):
            shen_t = define.param_types[i]
            elem = list_elem_type(shen_t)
            label = elem if elem is not None else shen_t
            name = rust_ident(label) + "s" if elem is not None else rust_ident(label)
        if name is None:
            name = f"arg{i}"
        param_names.append(name)

    # Build parameter type expressions. Rust takes lists as `&[T]` borrows.
    param_rust_types: list[str] = []
    for i in range(arity):
        if i >= len(define.param_types):
            param_rust_types.append("/* unknown */")
            continue
        shen_t = define.param_types[i]
        elem = list_elem_type(shen_t)
        if elem is not None:
            param_rust_types.append(f"&[{field_rust_type(elem, st)}]")
        elif shen_t in PRIMITIVES:
            # Take primitive non-string by value, &str for strings.
            rust = PRIMITIVES[shen_t]
            param_rust_types.append("&str" if rust == "String" else rust)
        else:
            # Composite: take by reference.
            ti = st.get(shen_t)
            if ti is not None and ti.category == "alias" and ti.wrapped_type:
                # Alias for `(list X)` → slice borrow.
                ae = list_elem_type(ti.wrapped_type)
                if ae is not None:
                    param_rust_types.append(f"&[{field_rust_type(ae, st)}]")
                    continue
            param_rust_types.append(f"&{to_pascal(shen_t)}")

    return_rust = "bool"
    if define.return_type == "number":
        return_rust = "f64"

    # Resolve the loop parameter's element type, possibly through an alias.
    loop_shen = define.param_types[loop_idx] if loop_idx < len(define.param_types) else ""
    list_elem_shen = list_elem_type(loop_shen) or ""
    if not list_elem_shen and loop_shen:
        ti = st.get(loop_shen)
        if ti is not None and ti.category == "alias" and ti.wrapped_type:
            list_elem_shen = list_elem_type(ti.wrapped_type) or ""

    emitted_clauses: list[tuple[str, str]] = []
    elem_var = "elem"

    for clause in define.clauses:
        if loop_idx >= len(clause.patterns):
            continue
        loop_pat = clause.patterns[loop_idx]
        if loop_pat == '[]':
            continue
        if clause.guard == "":
            continue
        bindings = _extract_destructure_bindings(loop_pat)
        local_var_map: dict[str, str] = {}
        repl_map: dict[str, str] = {}

        def _add_replacement(src: str, dst: str) -> None:
            repl_map[src] = dst
            snake = to_snake(src)
            if snake != src:
                repl_map[snake] = dst

        if len(bindings) == 1 and bindings[0] != '_':
            local_var_map[bindings[0]] = list_elem_shen
            _add_replacement(bindings[0], elem_var)
        elif len(bindings) > 1 and list_elem_shen:
            elem_info = st.get(list_elem_shen)
            if elem_info is None or len(elem_info.fields) < len(bindings):
                raise DefineEmitError(
                    f"destructure {loop_pat!r} does not match element type "
                    f"{list_elem_shen!r}"
                )
            for j, varname in enumerate(bindings):
                if varname == '_':
                    continue
                f = elem_info.fields[j]
                local_var_map[varname] = f.shen_type
                # The accessor name must match what was emitted on the
                # struct impl. The existing Rust emitter uses bare
                # `to_snake` for accessors, so we mirror that here even
                # when the result aliases a keyword — flag and bail rather
                # than emit a `.ref()` call that won't compile.
                accessor_name = to_snake(f.shen_name)
                if accessor_name in _RUST_KEYWORDS:
                    raise DefineEmitError(
                        f"destructure binding `{varname}` would invoke "
                        f"`{accessor_name}()` which is a Rust keyword; "
                        "regenerate the impl's accessor with an escaped name"
                    )
                accessor = f"{elem_var}.{accessor_name}()"
                _add_replacement(varname, accessor)
        else:
            raise DefineEmitError(
                f"could not bind destructure {loop_pat!r} (no type signature?)"
            )

        for i, pat in enumerate(clause.patterns):
            if i == loop_idx or pat in ('_', '[]') or pat.startswith('['):
                continue
            if i < len(define.param_types):
                local_var_map[pat] = define.param_types[i]
            _add_replacement(pat, param_names[i])

        repl_map = {k: v for k, v in repl_map.items() if k != v}

        guard_rust = _guard_to_rust(clause.guard, local_var_map, st)
        if "TODO" in guard_rust:
            raise DefineEmitError(
                f"guard {clause.guard!r} contains constructs the emitter cannot "
                "translate"
            )

        result_rust = _result_to_rust(clause.result)
        if result_rust.startswith('('):
            raise DefineEmitError(
                f"result {clause.result!r} is an s-expression — only literal "
                "results supported"
            )
        guard_rust = _apply_replacements_once(guard_rust, repl_map)
        result_rust = _apply_replacements_once(result_rust, repl_map)
        emitted_clauses.append((guard_rust, result_rust))

    if not emitted_clauses:
        raise DefineEmitError(
            "no translatable guarded clauses (only the base case and unguarded "
            "recursion were present)"
        )

    sig_parts = []
    for i, pn in enumerate(param_names):
        if i < len(param_rust_types):
            sig_parts.append(f"{pn}: {param_rust_types[i]}")
        else:
            sig_parts.append(pn)

    lines: list[str] = []
    lines.append(
        f"    /// Generated from Shen define {define.name}."
    )
    lines.append(f"    pub fn {rust_name}({', '.join(sig_parts)}) -> {return_rust} {{")
    loop_param_name = param_names[loop_idx]
    lines.append(f"        for {elem_var} in {loop_param_name} {{")
    for guard_rust, result_rust in emitted_clauses:
        lines.append(f"            if {guard_rust} {{")
        lines.append(f"                return {result_rust};")
        lines.append("            }")
    lines.append("        }")
    lines.append(f"        {base_result}")
    lines.append("    }")
    lines.append("")
    return lines


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def field_rust_type(shen_type: str, st: dict) -> str:
    # Parametric list types: (list X) → Vec<X>. Rust's borrow rules mean
    # the field-owning form is Vec<T>; accessors return &[T] (handled in
    # the accessor emitter). Constructor parameters likewise take Vec<T>
    # by value, matching the Go emitter's `[]T` treatment.
    elem = list_elem_type(shen_type)
    if elem is not None:
        return f"Vec<{field_rust_type(elem, st)}>"
    if shen_type in PRIMITIVES:
        return PRIMITIVES[shen_type]
    return to_pascal(shen_type)


def list_elem_type(shen_type: str) -> Optional[str]:
    """Extract element type from `(list X)`, or None if not a list type."""
    if shen_type.startswith("(list ") and shen_type.endswith(")"):
        return shen_type[len("(list "):-1].strip()
    return None


# ---------------------------------------------------------------------------
# Name Conversion
# ---------------------------------------------------------------------------

def to_pascal(name: str) -> str:
    return "".join(w.capitalize() for w in name.split("-"))

def to_snake(name: str) -> str:
    # Convert PascalCase or camelCase to snake_case
    if "-" in name:
        return name.replace("-", "_").lower()
    result = re.sub(r'([A-Z])', r'_\1', name).lower().lstrip('_')
    return result if result else name.lower()


# Rust strict keywords that cannot be used as identifiers (full list per
# the Rust reference). When `to_snake` produces one of these, append an
# underscore so the emitted Rust still compiles.
_RUST_KEYWORDS = {
    "as", "break", "const", "continue", "crate", "else", "enum", "extern",
    "false", "fn", "for", "if", "impl", "in", "let", "loop", "match", "mod",
    "move", "mut", "pub", "ref", "return", "self", "Self", "static", "struct",
    "super", "trait", "true", "type", "unsafe", "use", "where", "while",
    "async", "await", "dyn", "abstract", "become", "box", "do", "final",
    "macro", "override", "priv", "typeof", "unsized", "virtual", "yield",
    "try", "union",
}


def rust_ident(name: str) -> str:
    """Snake-case `name` and escape Rust keywords with a trailing underscore."""
    s = to_snake(name)
    if s in _RUST_KEYWORDS:
        return s + "_"
    return s


# ---------------------------------------------------------------------------
# Diagnostics
# ---------------------------------------------------------------------------

def print_symbol_table(st: dict[str, TypeInfo], spec_path: str):
    print(f"Parsed from {spec_path}", file=sys.stderr)
    print("", file=sys.stderr)
    print("Symbol table:", file=sys.stderr)
    for name, info in st.items():
        cat = f"[{info.category:12s}]"
        extra = ""
        if info.category in ("wrapper", "constrained"):
            extra = f" wraps={info.wrapped_prim}"
        elif info.category == "alias":
            extra = f" = {info.wrapped_type}"
        elif info.category == "sumtype":
            extra = f" variants={info.variants}"
        elif info.fields:
            fields_str = ", ".join(f"{fi.shen_name}:{fi.shen_type}" for fi in info.fields)
            extra = f" {{{fields_str}}}"
        print(f"  {name:30s} {cat}{extra}", file=sys.stderr)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Generate Rust guard types from Shen specs")
    parser.add_argument("spec", help="Path to .shen spec file")
    parser.add_argument("--out", help="Output file (default: stdout)")
    parser.add_argument("--mode", choices=["standard", "hardened"], default="standard")
    parser.add_argument("--mod", dest="mod_name", default="shenguard", help="Rust module name")
    args = parser.parse_args()

    datatypes, defines = parse_file_full(args.spec)
    st = build_symbol_table(datatypes)
    print_symbol_table(st, args.spec)

    code = emit_rust(datatypes, st, args.spec, args.mod_name, args.mode, defines)

    if args.out:
        with open(args.out, 'w') as f:
            f.write(code)
        print(f"Generated {args.out}", file=sys.stderr)
    else:
        print(code)


if __name__ == "__main__":
    main()
