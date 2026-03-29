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
    with open(path) as f:
        text = f.read()
    # Strip comments
    text = re.sub(r'\\\*.*?\*\\', '', text, flags=re.DOTALL)
    datatypes = []
    # Find (datatype name ...) blocks with balanced parens
    i = 0
    while i < len(text):
        m = re.search(r'\(datatype\s+([\w-]+)', text[i:])
        if not m:
            break
        start = i + m.start()
        name = m.group(1)
        # Find matching closing paren
        depth = 0
        j = start
        while j < len(text):
            if text[j] == '(':
                depth += 1
            elif text[j] == ')':
                depth -= 1
                if depth == 0:
                    break
            j += 1
        body = text[start + m.end() - m.start():j]
        rules = parse_rules(body)
        if rules:
            datatypes.append(Datatype(name=name, rules=rules))
        i = j + 1
    return datatypes

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
        # Type judgment: Var : type
        tm = re.match(r'(\w+)\s*:\s*([\w-]+(?:\s*\(.*?\))?)\s*$', line)
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
        inner = resolve(inner_expr, var_map, st)
        return (f"!({inner.code})", f"not {vp.raw}")
    if op == "element?":
        r = resolve(expr, var_map, st)
        return (r.code, f"must be a valid member")
    return ("true /* TODO: " + vp.raw + " */", vp.raw)


# ---------------------------------------------------------------------------
# Emitter
# ---------------------------------------------------------------------------

def emit_rust(datatypes: list[Datatype], st: dict[str, TypeInfo], spec_path: str, mod_name: str, mode: str) -> str:
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
        target_type = PRIMITIVES.get(info.wrapped_type, to_pascal(info.wrapped_type or ""))
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
            lines.append(f"            if !({code}) {{")
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
                lines.append(f"            if !({code}) {{")
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
            if fi.shen_type in PRIMITIVES:
                prim_rust = PRIMITIVES[fi.shen_type]
                ret_type = f"&str" if prim_rust == "String" else prim_rust
                ret_expr = f"&self.{accessor}" if prim_rust == "String" else f"self.{accessor}"
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

def field_rust_type(shen_type: str, st: dict) -> str:
    if shen_type in PRIMITIVES:
        return PRIMITIVES[shen_type]
    return to_pascal(shen_type)


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

    datatypes = parse_file(args.spec)
    st = build_symbol_table(datatypes)
    print_symbol_table(st, args.spec)

    code = emit_rust(datatypes, st, args.spec, args.mod_name, args.mode)

    if args.out:
        with open(args.out, 'w') as f:
            f.write(code)
        print(f"Generated {args.out}", file=sys.stderr)
    else:
        print(code)


if __name__ == "__main__":
    main()
