#!/usr/bin/env python3
"""shengen-py — Generate Python guard types from Shen sequent-calculus specs.

Architecture: Parse → Symbol Table → Resolve → Emit (mirrors Go/TS shengen).
Supports --mode standard|hardened.

Usage:
    python3 shengen.py <spec-file> --out <output-file> [--mode standard|hardened]
"""

import argparse
import re
import sys
from dataclasses import dataclass, field
from typing import Optional


# ---------------------------------------------------------------------------
# AST (shared with shengen-rs)
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
    fields: list[str]
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
    patterns: list[str]   # raw pattern tokens, e.g. ["_", "[]"] or ["Ref", "[[EntryRef _] | Rest]"]
    result: str           # raw s-expression or literal
    guard: str = ""       # raw where-clause s-expression (may be empty)


@dataclass
class Define:
    name: str
    clauses: list[DefineClause] = field(default_factory=list)
    # Type signature parsed from `{T1 --> T2 --> ... --> Tret}`. If empty the
    # define has no published signature in the source — params fall back to
    # untyped `Any`.
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
    py_name: str
    category: str
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

PRIMITIVES = {"string": "str", "number": "float", "symbol": "str", "boolean": "bool"}

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
    """Find all balanced-paren blocks starting with `prefix`. Mirrors Go's
    `extractBlocks` in cmd/shengen/main.go."""
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
    body = block[m.end():-1]  # strip trailing ')'
    rules = parse_rules(body)
    if not rules:
        return None
    return Datatype(name=name, rules=rules)


def parse_define(block: str) -> Optional[Define]:
    """Parse a (define name {sig?} body) block into a Define record.

    Mirrors `parseDefine` in cmd/shengen/main.go. The body is reduced to a
    single line and split on ` -> ` to yield alternating patterns/results.

    Also extracts an optional type signature `{T1 --> T2 --> ... --> Tret}`.
    """
    block = block.strip()
    if not block.startswith('(define '):
        return None
    block = block[len('(define '):]
    # The first whitespace token after `(define ` is the name.
    nl_idx = block.find('\n')
    if nl_idx == -1:
        return None
    name = block[:nl_idx].strip()
    body = block[nl_idx:].rstrip(' \t\n)')

    # Extract optional type signature on the first non-blank body line. Shen
    # signatures look like `{string --> (list tag-id) --> number --> boolean}`.
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

        # Clean up trailing parens / re-extract balanced expressions.
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
    """Tokenize a pattern string respecting bracket nesting.
    `[Med | Meds]` stays as one token; `[[X Y] | Rest]` stays as one token.
    Mirrors `splitPatterns` in cmd/shengen/main.go."""
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
    """Return (balanced-expr, index-past-end). Mirrors Go's `extractBalancedParen`."""
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
        if not line or '>>' in line:
            continue
        vm = re.match(r'(.+?)\s*:\s*verified\s*$', line)
        if vm:
            verified.append(VerifiedPremise(raw=vm.group(1).strip()))
            continue
        if line.startswith('if '):
            verified.append(VerifiedPremise(raw=line[3:].strip()))
            continue
        # Match either a simple type (e.g. `X : amount`) or a parametric
        # type like `Refs : (list tag-id)`. The (list X) form is the only
        # parametric construct supported today.
        tm = re.match(r'(\w+)\s*:\s*(\(list\s+[\w-]+\)|[\w-]+)\s*$', line)
        if tm:
            premises.append(Premise(var_name=tm.group(1), type_name=tm.group(2).strip()))
    return premises, verified

def parse_conclusion(text: str) -> Optional[Conclusion]:
    text = text.strip().rstrip(';').rstrip(')').strip()
    if not text or '>>' in text:
        return None
    cm = re.match(r'\[([^\]]+)\]\s*:\s*([\w-]+)', text)
    if cm:
        return Conclusion(fields=cm.group(1).split(), type_name=cm.group(2), is_composite=True)
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
            if dt.name != ctype and conc_count.get(ctype, 0) > 1:
                type_name = dt.name
                sum_types.setdefault(ctype, []).append(dt.name)
            else:
                type_name = ctype

            info = TypeInfo(shen_name=type_name, py_name=to_pascal(type_name), category=classify(rule))

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
        table[ctype] = TypeInfo(shen_name=ctype, py_name=to_pascal(ctype), category="sumtype", variants=variants)

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
# S-Expression Parser & Resolver (shared logic)
# ---------------------------------------------------------------------------

def parse_sexpr(text: str) -> SExpr:
    tokens = []
    i = 0
    while i < len(text):
        if text[i] in ' \t\n':
            i += 1
        elif text[i] in '()[]':
            tokens.append(text[i])
            i += 1
        else:
            j = i
            while j < len(text) and text[j] not in ' \t\n()[]':
                j += 1
            tokens.append(text[i:j])
            i = j

    def _parse(pos):
        if pos >= len(tokens):
            return SExpr(atom=""), pos
        if tokens[pos] == '(':
            children = []
            pos += 1
            while pos < len(tokens) and tokens[pos] != ')':
                child, pos = _parse(pos)
                children.append(child)
            return SExpr(children=children), pos + 1
        return SExpr(atom=tokens[pos]), pos + 1

    expr, _ = _parse(0)
    return expr

@dataclass
class Resolved:
    code: str
    typ: str = "unknown"
    is_multi: bool = False
    base_code: str = ""
    remaining: list[FieldInfo] = field(default_factory=list)

def resolve(expr: SExpr, var_map: dict, st: dict) -> Resolved:
    if expr.is_atom():
        a = expr.atom
        if a and (a[0].isdigit() or (a[0] == '-' and len(a) > 1)):
            return Resolved(code=a, typ="number")
        if a in var_map:
            return Resolved(code=to_snake(a), typ=var_map[a])
        if a and a[0] == '"':
            return Resolved(code=a, typ="string")
        return Resolved(code=a or "", typ="unknown")

    if not expr.is_call():
        return Resolved(code="True  # unresolved")

    op = expr.op()
    if op in ("head", "tail"):
        inner = resolve(expr.children[1], var_map, st)
        if inner.is_multi:
            fields = inner.remaining
        else:
            ti = st.get(inner.typ)
            fields = ti.fields if ti and ti.fields else []
        if not fields:
            return Resolved(code="True  # unresolved head/tail")
        if op == "head":
            f = fields[0]
            return Resolved(code=f"{inner.code if not inner.is_multi else inner.base_code}.{to_snake(f.shen_name)}()", typ=f.shen_type)
        remaining = fields[1:]
        if len(remaining) == 1:
            f = remaining[0]
            base = inner.code if not inner.is_multi else inner.base_code
            return Resolved(code=f"{base}.{to_snake(f.shen_name)}()", typ=f.shen_type)
        return Resolved(code=inner.code, is_multi=True, base_code=inner.base_code or inner.code, remaining=remaining)

    if op == "not":
        inner = resolve(expr.children[1], var_map, st)
        return Resolved(code=f"not ({inner.code})", typ="boolean")
    if op == "shen.mod":
        lhs = resolve(expr.children[1], var_map, st)
        rhs = resolve(expr.children[2], var_map, st)
        return Resolved(code=f"int({unwrap(lhs, st)}) % int({rhs.code})", typ="number")
    if op == "length":
        inner = resolve(expr.children[1], var_map, st)
        return Resolved(code=f"len({unwrap(inner, st)})", typ="number")
    if op == "element?":
        var_expr = resolve(expr.children[1], var_map, st)
        members = [f'"{c.atom.strip("[]")}"' for c in expr.children[2:] if c.is_atom() and c.atom.strip("[]")]
        return Resolved(code=f'{unwrap(var_expr, st)} in {{{", ".join(members)}}}', typ="boolean")
    return Resolved(code=f"True  # TODO: {expr}")

def unwrap(r: Resolved, st: dict) -> str:
    ti = st.get(r.typ)
    if ti and ti.category in ("wrapper", "constrained"):
        return f"{r.code}.val()"
    return r.code

def translate_verified(vp: VerifiedPremise, var_map: dict, st: dict) -> tuple[str, str]:
    expr = parse_sexpr(vp.raw)
    if not expr.is_call():
        return ("True  # TODO", vp.raw)
    op = expr.op()
    if op in (">=", "<=", ">", "<"):
        lhs = resolve(expr.children[1], var_map, st)
        rhs = resolve(expr.children[2], var_map, st)
        return (f"{unwrap(lhs, st)} {op} {unwrap(rhs, st)}", f"{lhs.code} must be {op} {rhs.code}")
    if op == "=":
        lhs = resolve(expr.children[1], var_map, st)
        rhs = resolve(expr.children[2], var_map, st)
        # Escape any embedded quotes so the message can be safely interpolated
        # into a Python string literal at emission time.
        return (
            f"{unwrap(lhs, st)} == {unwrap(rhs, st)}",
            f"{_msg_escape(lhs.code)} must equal {_msg_escape(rhs.code)}",
        )
    if op == "not":
        inner_expr = expr.children[1]
        # (not (= LHS RHS)) — prefer human-readable messages:
        #   (not (= X ""))  → "X must not be empty"
        #   (not (= X Y))   → "X must not equal Y"
        if inner_expr.is_call() and inner_expr.op() == "=" and len(inner_expr.children) == 3:
            lhs = resolve(inner_expr.children[1], var_map, st)
            rhs = resolve(inner_expr.children[2], var_map, st)
            code = f"not ({unwrap(lhs, st)} == {unwrap(rhs, st)})"
            if _is_empty_string_literal(rhs.code):
                return (code, f"{lhs.code} must not be empty")
            if _is_empty_string_literal(lhs.code):
                return (code, f"{rhs.code} must not be empty")
            return (code, f"{lhs.code} must not equal {rhs.code}")
        inner = resolve(inner_expr, var_map, st)
        return (f"not ({inner.code})", f"not {vp.raw}")
    if op == "element?":
        r = resolve(expr, var_map, st)
        return (r.code, "must be a valid member")
    return (f"True  # TODO: {vp.raw}", vp.raw)


def _is_empty_string_literal(code: str) -> bool:
    return code == '""'


def _msg_escape(s: str) -> str:
    """Escape characters that would break a Python string literal.

    Specifically: replace `"` with `\\\"` so a Shen-literal RHS like `"signed-complete"`
    survives interpolation into an emitted `raise ValueError("...")` line.
    """
    return s.replace('"', '\\"')


def negate_py_expr(py_expr: str) -> str:
    """Boolean negation of a Python expression with a peephole.

    Generated checks wrap the verified predicate in `not (...)` for the
    violation branch. When the inner predicate already starts with `not (`,
    that produces `not (not (...))` — mathematically correct but ugly in the
    generated source. Strip the redundant outer `not` instead.
    """
    inner = _strip_outer_not_paren(py_expr)
    if inner is not None:
        return f"({inner})"
    return f"not ({py_expr})"


def _strip_outer_not_paren(py_expr: str) -> Optional[str]:
    if not py_expr.startswith("not (") or not py_expr.endswith(")"):
        return None
    depth = 1
    body = py_expr[5:-1]
    for ch in body:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                # The opening paren after `not ` closed early — outer `not`
                # does not bracket the whole expression.
                return None
    if depth != 1:
        return None
    return body


# ---------------------------------------------------------------------------
# Emitter — Standard Mode
# ---------------------------------------------------------------------------

def emit_standard(
    datatypes: list[Datatype],
    st: dict[str, TypeInfo],
    spec_path: str,
    defines: Optional[list[Define]] = None,
) -> str:
    lines = [
        f"# Code generated by shengen-py from {spec_path}. DO NOT EDIT.",
        "#",
        "# These types enforce Shen sequent-calculus invariants at the Python level.",
        "# Factory functions are the ONLY way to create these types — bypassing them",
        "# is a violation of the formal spec.",
        "",
        "from __future__ import annotations",
        "",
        "from dataclasses import dataclass",
        "",
    ]

    # Sum types as Protocols
    for name, info in st.items():
        if info.category == "sumtype":
            lines.append(f"from typing import Union")
            break

    for dt in datatypes:
        for rule in dt.rules:
            ctype = rule.conclusion.type_name
            if dt.name != ctype and st.get(ctype, TypeInfo("", "", "")).category == "sumtype":
                type_name = dt.name
            else:
                type_name = ctype
            info = st.get(type_name)
            if not info or info.category == "sumtype":
                continue
            lines.extend(emit_type_standard(info, rule, st))

    # Sum type aliases
    for name, info in st.items():
        if info.category == "sumtype":
            variants = " | ".join(to_pascal(v) for v in info.variants)
            lines.append(f"{info.py_name} = Union[{', '.join(to_pascal(v) for v in info.variants)}]")
            lines.append("")

    # Pure-function helpers translated from (define …) blocks.
    if defines:
        lines.extend(emit_defines(defines, st))

    return "\n".join(lines)

def emit_type_standard(info: TypeInfo, rule: Rule, st: dict) -> list[str]:
    lines = []
    cat = info.category
    lines.append("")
    lines.append(f"# --- {info.py_name} ---")
    lines.append(f"# Shen: (datatype {info.shen_name})")

    if cat == "alias":
        # Render the alias target through `field_py_type` so parametric
        # constructs like `(list ref-table-entry)` produce `list[RefTableEntry]`
        # rather than the malformed `(list refTableEntry)`.
        target = field_py_type(info.wrapped_type or "", st)
        lines.append(f"{info.py_name} = {target}")
        lines.append("")
        return lines

    lines.append("@dataclass(frozen=True, slots=True)")
    lines.append(f"class {info.py_name}:")

    if cat in ("wrapper", "constrained"):
        py_type = PRIMITIVES.get(info.wrapped_prim, "str")
        lines.append(f"    _v: {py_type}")
        lines.append("")
        if cat == "constrained":
            lines.append("    def __post_init__(self) -> None:")
            var_map = {rule.premises[0].var_name: rule.premises[0].type_name}
            for vp in rule.verified:
                code, msg = translate_verified(vp, var_map, st)
                # In __post_init__, the variable is self._v
                code = code.replace(to_snake(rule.premises[0].var_name), "self._v")
                lines.append(f"        if {negate_py_expr(code)}:")
                lines.append(f'            raise ValueError(f"{msg}: {{self._v}}")')
            lines.append("")
        lines.append(f"    def val(self) -> {py_type}:")
        lines.append("        return self._v")
    else:
        for fi in info.fields:
            py_type = field_py_type(fi.shen_type, st)
            lines.append(f"    _{to_snake(fi.shen_name)}: {py_type}")
        lines.append("")
        if cat == "guarded":
            var_map = {p.var_name: p.type_name for p in rule.premises}
            lines.append("    def __post_init__(self) -> None:")
            for vp in rule.verified:
                code, msg = translate_verified(vp, var_map, st)
                # Replace variable refs with self._field
                for p in rule.premises:
                    code = code.replace(to_snake(p.var_name) + ".", f"self._{to_snake(p.var_name)}.")
                    code = re.sub(rf'\b{to_snake(p.var_name)}\b(?!\.)', f"self._{to_snake(p.var_name)}", code)
                lines.append(f"        if {negate_py_expr(code)}:")
                lines.append(f'            raise ValueError("{msg}")')
            lines.append("")
        for fi in info.fields:
            py_type = field_py_type(fi.shen_type, st)
            accessor = to_snake(fi.shen_name)
            # Avoid Python keyword conflicts
            if accessor in ("from",):
                accessor = accessor + "_"
            lines.append(f"    def {accessor}(self) -> {py_type}:")
            lines.append(f"        return self._{to_snake(fi.shen_name)}")
            lines.append("")

    lines.append("")

    # Factory function
    fn_name = f"new_{to_snake(info.shen_name)}"
    if cat in ("wrapper", "constrained"):
        py_type = PRIMITIVES.get(info.wrapped_prim, "str")
        lines.append(f"def {fn_name}(x: {py_type}) -> {info.py_name}:")
        lines.append(f"    return {info.py_name}(_v=x)")
    else:
        params = []
        for fi in info.fields:
            py_type = field_py_type(fi.shen_type, st)
            pname = to_snake(fi.shen_name)
            if pname in ("from",):
                pname = pname + "_"
            params.append(f"{pname}: {py_type}")
        args = []
        for fi in info.fields:
            pname = to_snake(fi.shen_name)
            if pname in ("from",):
                pname = pname + "_"
            args.append(f"_{to_snake(fi.shen_name)}={pname}")
        lines.append(f"def {fn_name}({', '.join(params)}) -> {info.py_name}:")
        # Type checks for composite args. Primitive types and list types are
        # not isinstance-checked at construction time — primitives are caught
        # by the type annotation; lists are trusted (matching the Go emitter's
        # treatment of `[]T` parameters).
        for fi in info.fields:
            if fi.shen_type in PRIMITIVES or list_elem_type(fi.shen_type) is not None:
                continue
            pname = to_snake(fi.shen_name)
            if pname in ("from",):
                pname = pname + "_"
            expected = to_pascal(fi.shen_type)
            lines.append(f"    if not isinstance({pname}, {expected}):")
            lines.append(f'        raise TypeError(f"{pname} must be {expected}, got {{type({pname}).__name__}}")')
        lines.append(f"    return {info.py_name}({', '.join(args)})")

    lines.append("")
    return lines


# ---------------------------------------------------------------------------
# Emitter — Hardened Mode
# ---------------------------------------------------------------------------

def emit_hardened(
    datatypes: list[Datatype],
    st: dict[str, TypeInfo],
    spec_path: str,
    defines: Optional[list[Define]] = None,
) -> str:
    lines = [
        f"# Code generated by shengen-py from {spec_path}. DO NOT EDIT.",
        "#",
        "# These types enforce Shen sequent-calculus invariants at the Python level.",
        "# Factory functions are the ONLY way to create these types — bypassing them",
        "# is a violation of the formal spec.",
        "#",
        "# HARDENED MODE: Closure-vault pattern with HMAC provenance tokens,",
        "# __init_subclass__ prevention, WeakSet instance registry, and",
        "# recursive HMAC chains for composite types.",
        "",
        "from __future__ import annotations",
        "",
        "import hashlib",
        "import hmac",
        "import os",
        "import weakref",
        "from typing import Any, Optional",
        "",
        "",
        "# --- Internal HMAC machinery ---",
        "",
        "_GUARD_SECRET = os.urandom(32)",
        "",
        "",
        "def _hmac_tag(label: str, *parts: bytes) -> str:",
        '    """Produce an HMAC provenance token from a label and constituent parts."""',
        "    h = hmac.new(_GUARD_SECRET, label.encode(), hashlib.sha256)",
        "    for p in parts:",
        "        h.update(p)",
        "    return h.hexdigest()",
        "",
        "",
        "def _bytes_of(value: Any) -> bytes:",
        '    """Canonical byte representation for HMAC inputs."""',
        "    if isinstance(value, str):",
        '        return value.encode("utf-8")',
        "    if isinstance(value, (int, float)):",
        '        return str(value).encode("utf-8")',
        "    if isinstance(value, bytes):",
        "        return value",
        '    return repr(value).encode("utf-8")',
        "",
    ]

    for dt in datatypes:
        for rule in dt.rules:
            ctype = rule.conclusion.type_name
            if dt.name != ctype and st.get(ctype, TypeInfo("", "", "")).category == "sumtype":
                type_name = dt.name
            else:
                type_name = ctype
            info = st.get(type_name)
            if not info or info.category == "sumtype":
                continue
            lines.extend(emit_type_hardened(info, rule, st))

    # Pure-function helpers translated from (define …) blocks.
    if defines:
        lines.extend(emit_defines(defines, st))

    return "\n".join(lines)

def emit_type_hardened(info: TypeInfo, rule: Rule, st: dict) -> list[str]:
    lines = []
    cat = info.category
    lines.append("")
    lines.append(f"# --- {info.py_name} ---")
    lines.append(f"# Shen: (datatype {info.shen_name})")

    if cat == "alias":
        # Render the alias target through `field_py_type` so parametric
        # constructs like `(list ref-table-entry)` produce `list[RefTableEntry]`
        # rather than the malformed `(list refTableEntry)`.
        target = field_py_type(info.wrapped_type or "", st)
        lines.append(f"{info.py_name} = {target}")
        lines.append("")
        return lines

    # Class with __slots__ and __init_subclass__ prevention
    if cat in ("wrapper", "constrained"):
        slots = '("_v", "_tag", "__weakref__")'
    else:
        field_slots = ", ".join(f'"_{to_snake(fi.shen_name)}"' for fi in info.fields)
        slots = f'({field_slots}, "_tag", "__weakref__")'

    lines.append(f"class {info.py_name}:")
    lines.append(f"    __slots__ = {slots}")
    lines.append(f'    _registry: weakref.WeakSet["{info.py_name}"] = weakref.WeakSet()')
    lines.append("")
    lines.append(f"    def __init_subclass__(cls, **kwargs: Any) -> None:")
    lines.append(f'        raise TypeError("{info.py_name} cannot be subclassed")')
    lines.append("")
    lines.append(f"    def __init__(self) -> None:")
    lines.append(f'        raise TypeError("Use new_{to_snake(info.shen_name)}() to create {info.py_name} instances")')
    lines.append("")

    # Accessors
    if cat in ("wrapper", "constrained"):
        py_type = PRIMITIVES.get(info.wrapped_prim, "str")
        lines.append(f"    def val(self) -> {py_type}:")
        lines.append(f"        return self._v")
    else:
        for fi in info.fields:
            py_type = field_py_type(fi.shen_type, st)
            accessor = to_snake(fi.shen_name)
            if accessor in ("from",):
                accessor += "_"
            lines.append(f"    def {accessor}(self) -> {py_type}:")
            lines.append(f"        return self._{to_snake(fi.shen_name)}")

    lines.append("")
    lines.append("")

    # Verify function
    fn_verify = f"verify_{to_snake(info.shen_name)}"
    lines.append(f"def {fn_verify}(obj: {info.py_name}) -> bool:")
    lines.append(f'    """Verify that a {info.py_name} has valid HMAC provenance."""')

    if cat in ("wrapper", "constrained"):
        lines.append(f'    expected = _hmac_tag("{info.py_name}", _bytes_of(obj._v))')
    else:
        tag_parts = []
        for fi in info.fields:
            if fi.shen_type in PRIMITIVES:
                tag_parts.append(f"_bytes_of(obj._{to_snake(fi.shen_name)})")
            elif list_elem_type(fi.shen_type) is not None:
                # Lists are tagged by their length only — element-level
                # provenance hashing is deferred (see factory TODO).
                tag_parts.append(f'_bytes_of(len(obj._{to_snake(fi.shen_name)}))')
            else:
                tag_parts.append(f'obj._{to_snake(fi.shen_name)}._tag.encode("utf-8")')
        lines.append(f'    expected = _hmac_tag("{info.py_name}", {", ".join(tag_parts)})')

    lines.append("    return hmac.compare_digest(obj._tag, expected)")
    lines.append("")
    lines.append("")

    # Factory function
    fn_name = f"new_{to_snake(info.shen_name)}"
    if cat in ("wrapper", "constrained"):
        py_type = PRIMITIVES.get(info.wrapped_prim, "str")
        lines.append(f"def {fn_name}(x: {py_type}) -> {info.py_name}:")
        if cat == "constrained":
            var_map = {rule.premises[0].var_name: rule.premises[0].type_name}
            for vp in rule.verified:
                code, msg = translate_verified(vp, var_map, st)
                lines.append(f"    if {negate_py_expr(code)}:")
                lines.append(f'        raise ValueError(f"{msg}: {{x}}")')
        lines.append(f"    obj = object.__new__({info.py_name})")
        lines.append(f'    object.__setattr__(obj, "_v", x)')
        lines.append(f'    object.__setattr__(obj, "_tag", _hmac_tag("{info.py_name}", _bytes_of(x)))')
        lines.append(f"    {info.py_name}._registry.add(obj)")
        lines.append("    return obj")
    else:
        params = []
        for fi in info.fields:
            py_type = field_py_type(fi.shen_type, st)
            pname = to_snake(fi.shen_name)
            if pname in ("from",):
                pname += "_"
            params.append(f"{pname}: {py_type}")

        lines.append(f"def {fn_name}({', '.join(params)}) -> {info.py_name}:")

        # Type checks. Lists are trusted (no per-element provenance check
        # because list element provenance would change the recursive HMAC
        # shape — emit a TODO and skip).
        for fi in info.fields:
            if fi.shen_type in PRIMITIVES:
                continue
            if list_elem_type(fi.shen_type) is not None:
                pname = to_snake(fi.shen_name)
                if pname in ("from",):
                    pname += "_"
                lines.append(f"    # TODO: list element provenance not yet verified for {pname}")
                continue
            pname = to_snake(fi.shen_name)
            if pname in ("from",):
                pname += "_"
            expected = to_pascal(fi.shen_type)
            lines.append(f"    if not isinstance({pname}, {expected}):")
            lines.append(f'        raise TypeError(f"{pname} must be {expected}, got {{type({pname}).__name__}}")')
            lines.append(f"    if not verify_{to_snake(fi.shen_type)}({pname}):")
            lines.append(f'        raise ValueError("{pname} has invalid provenance (possible tampering)")')

        # Guarded checks
        if cat == "guarded":
            var_map = {p.var_name: p.type_name for p in rule.premises}
            for vp in rule.verified:
                code, msg = translate_verified(vp, var_map, st)
                lines.append(f"    if {negate_py_expr(code)}:")
                lines.append(f'        raise ValueError("{msg}")')

        lines.append(f"    obj = object.__new__({info.py_name})")
        for fi in info.fields:
            pname = to_snake(fi.shen_name)
            if pname in ("from",):
                pname += "_"
            lines.append(f'    object.__setattr__(obj, "_{to_snake(fi.shen_name)}", {pname})')

        # HMAC tag incorporating field tokens
        tag_parts = []
        for fi in info.fields:
            pname = to_snake(fi.shen_name)
            if pname in ("from",):
                pname += "_"
            if fi.shen_type in PRIMITIVES:
                tag_parts.append(f"_bytes_of({pname})")
            elif list_elem_type(fi.shen_type) is not None:
                tag_parts.append(f"_bytes_of(len({pname}))")
            else:
                tag_parts.append(f'{pname}._tag.encode("utf-8")')
        lines.append(f'    object.__setattr__(obj, "_tag", _hmac_tag("{info.py_name}", {", ".join(tag_parts)}))')
        lines.append(f"    {info.py_name}._registry.add(obj)")
        lines.append("    return obj")

    lines.append("")
    return lines


# ---------------------------------------------------------------------------
# Define Emission
# ---------------------------------------------------------------------------

class DefineEmitError(Exception):
    """Raised when a define block uses constructs the Python emitter cannot
    handle. Callers should emit a clear comment in the output and continue
    with the rest of the spec rather than corrupt the file silently."""


def define_py_name(shen_name: str) -> str:
    """Convert a Shen define name like `ref-present?` to Python `ref_present`.

    The trailing `?` (predicate convention) is stripped; hyphens map to
    underscores so the name slots into Python's snake_case style.
    """
    name = shen_name.rstrip('?')
    return to_snake(name)


def _extract_destructure_bindings(pattern: str) -> list[str]:
    """Parse a destructuring pattern and return the element's binding names.

    `[Med | Meds]`        → ["Med"]            (simple head binding)
    `[[X Y] | Rest]`      → ["X", "Y"]         (record-shape destructure)
    `[_ | Rest]`          → ["_"]              (ignore element)
    Anything else         → []
    """
    # Strip exactly one leading `[` (not all of them — `[[X Y] | Rest]`
    # contains a nested bracket pair that must survive).
    if not pattern.startswith('['):
        return []
    inner = pattern[1:]
    pipe_idx = inner.find('|')
    if pipe_idx == -1:
        return []
    inner = inner[:pipe_idx].strip()
    if inner.startswith('['):
        # Nested destructure [[X Y]] → strip exactly one bracket pair.
        if not inner.endswith(']'):
            return []
        inner = inner[1:-1].strip()
        return inner.split()
    parts = inner.split()
    if len(parts) == 1:
        return parts
    return []


def _analyze_define(define: Define) -> tuple[int, Optional[str]]:
    """Find (loop_param_idx, base_result).

    `loop_param_idx` is the index of the parameter that is destructured via
    `[X | Rest]` in at least one clause. Mirrors `analyzeDefine` in the Go
    emitter. Returns -1 if no list iteration pattern is found.
    `base_result` is the Python translation of the first clause whose
    loop-param pattern is exactly `[]` — the recursion base case. Returns
    None when no explicit base case is present, so callers can decide
    whether to default it (`False` for predicates) or refuse to emit.
    """
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
                base_result = _result_to_py(clause.result)
                break
    return loop_idx, base_result


def _result_to_py(result: str) -> str:
    """Translate a clause result token to Python syntax."""
    r = result.strip()
    if r == 'true':
        return 'True'
    if r == 'false':
        return 'False'
    # S-expression results — best-effort: only handle a few atomic forms.
    return r


def emit_defines(defines: list[Define], st: dict[str, TypeInfo]) -> list[str]:
    """Emit Python functions for the given define blocks.

    Conservative scope: handle defines that iterate over exactly one list
    parameter (the loop-param), with a base case `[]` and where-guarded
    clauses whose guard and result expressions translate cleanly to Python.

    Anything outside this shape is emitted as a top-level comment naming the
    define and the reason it was skipped. Never emits invalid Python.
    """
    if not defines:
        return []
    lines: list[str] = [
        "",
        "",
        "# --- Pure functions translated from (define …) blocks ---",
        "#",
        "# Only defines that recurse over a single list parameter are emitted.",
        "# Anything richer (multi-list recursion, non-list pattern matching,",
        "# arithmetic-heavy bodies) is flagged below and left for a future",
        "# emitter pass — the Python emitter prefers a clear gap to a quietly",
        "# corrupted helper.",
        "",
    ]
    for define in defines:
        try:
            block = _emit_one_define(define, st)
        except DefineEmitError as exc:
            block = [
                f"# unsupported: define {define.name!r} — {exc}",
                "",
            ]
        lines.extend(block)
    return lines


def _emit_one_define(define: Define, st: dict[str, TypeInfo]) -> list[str]:
    py_name = define_py_name(define.name)
    loop_idx, base_result = _analyze_define(define)
    if loop_idx < 0:
        raise DefineEmitError(
            "no list-iteration clause (Python emitter only handles defines "
            "that destructure a single list parameter)"
        )
    if base_result is None:
        # No explicit `[]` clause. For boolean predicates we can default to
        # `False`; for any other return type we refuse rather than fabricate.
        if define.return_type == "boolean":
            base_result = "False"
        else:
            raise DefineEmitError(
                "no `[]` base case clause and return type is not boolean — "
                "emitter cannot synthesise a fallback value"
            )

    # Resolve param types from the signature if present.
    param_py_types: list[str] = []
    for i in range(len(define.param_types)):
        shen_t = define.param_types[i]
        elem = list_elem_type(shen_t)
        if elem is not None:
            param_py_types.append(f"list[{field_py_type(elem, st)}]")
        elif shen_t in PRIMITIVES:
            param_py_types.append(PRIMITIVES[shen_t])
        else:
            param_py_types.append(to_pascal(shen_t))

    # Build parameter names: prefer a non-wildcard pattern name from the
    # first clause where each position has a usable variable, falling back
    # to a snake-cased version of the type signature (e.g. `ref-table` →
    # `ref_table`) so generated code doesn't carry through `arg1`.
    arity = max(len(c.patterns) for c in define.clauses) if define.clauses else 0
    if arity == 0:
        raise DefineEmitError("no clauses")
    param_names: list[str] = []
    for i in range(arity):
        name: Optional[str] = None
        for clause in define.clauses:
            if i >= len(clause.patterns):
                continue
            pat = clause.patterns[i]
            if pat in ('_', '[]') or pat.startswith('['):
                continue
            name = to_snake(pat)
            break
        if name is None and i < len(define.param_types):
            # Fall back to the type-name (without (list ...) wrapper).
            shen_t = define.param_types[i]
            elem = list_elem_type(shen_t)
            label = elem if elem is not None else shen_t
            name = to_snake(label) + "s" if elem is not None else to_snake(label)
        if name is None:
            name = f"arg{i}"
        # Avoid Python keyword collisions.
        if name in ("from",):
            name += "_"
        param_names.append(name)

    # Typed signature.
    sig_parts = []
    for i, pn in enumerate(param_names):
        if i < len(param_py_types):
            sig_parts.append(f"{pn}: {param_py_types[i]}")
        else:
            sig_parts.append(pn)
    return_py = "bool"
    if define.return_type:
        if define.return_type in PRIMITIVES:
            return_py = PRIMITIVES[define.return_type]
        else:
            elem = list_elem_type(define.return_type)
            if elem is not None:
                return_py = f"list[{field_py_type(elem, st)}]"
            else:
                return_py = to_pascal(define.return_type)

    # The non-base, non-guarded "fall-through" clause is the recursive call
    # in tail position. We emit a `for` loop that handles the guarded clauses
    # eagerly, then returns `base_result` after the loop — semantically
    # equivalent for predicate-style defines (the recursive call sees the
    # tail of the list, which is what the next loop iteration handles).
    # Verify each clause that we emit translates cleanly *before* emitting
    # anything. This way a failure mid-define still keeps the output clean.
    emitted_clauses: list[tuple[str, str]] = []  # (guard_py, result_py)
    loop_shen_type = define.param_types[loop_idx] if loop_idx < len(define.param_types) else ""
    list_elem_shen = list_elem_type(loop_shen_type) or ""
    # If the loop parameter is an alias for `(list X)`, resolve through.
    if not list_elem_shen and loop_shen_type:
        ti = st.get(loop_shen_type)
        if ti is not None and ti.category == "alias" and ti.wrapped_type:
            list_elem_shen = list_elem_type(ti.wrapped_type) or ""
    elem_var = "elem"

    for clause in define.clauses:
        if loop_idx >= len(clause.patterns):
            continue
        loop_pat = clause.patterns[loop_idx]
        if loop_pat == '[]':
            continue
        if clause.guard == "":
            # Unguarded recursive tail — implicit by the for-loop. Skip.
            continue
        bindings = _extract_destructure_bindings(loop_pat)
        local_var_map: dict[str, str] = {}
        # Replacement map: source identifier (in either Shen or snake-case
        # form) → target Python expression. We then do a single regex pass
        # to avoid the chained-replacement bug where a target string contains
        # a token that's itself a key.
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
                    f"destructure {loop_pat!r} does not match element type {list_elem_shen!r}"
                )
            for j, varname in enumerate(bindings):
                if varname == '_':
                    continue
                f = elem_info.fields[j]
                local_var_map[varname] = f.shen_type
                accessor = f"{elem_var}.{to_snake(f.shen_name)}()"
                _add_replacement(varname, accessor)
        else:
            raise DefineEmitError(
                f"could not bind destructure {loop_pat!r} (no type signature?)"
            )

        # Bind non-loop pattern variables to their parameter names.
        for i, pat in enumerate(clause.patterns):
            if i == loop_idx or pat in ('_', '[]') or pat.startswith('['):
                continue
            if i < len(define.param_types):
                local_var_map[pat] = define.param_types[i]
            _add_replacement(pat, param_names[i])

        # Drop replacements where the target *is* the source (self-mapping)
        # to keep the regex small.
        repl_map = {k: v for k, v in repl_map.items() if k != v}

        guard_py = _guard_to_py(clause.guard, local_var_map, st)
        if "TODO" in guard_py:
            raise DefineEmitError(
                f"guard {clause.guard!r} contains constructs the emitter cannot translate"
            )

        result_py = _result_to_py(clause.result)
        # If the result is itself an s-expression call (`(cons ...)`, recursion),
        # we can't translate it cleanly. Reject the whole define.
        if result_py.startswith('('):
            raise DefineEmitError(
                f"result {clause.result!r} is an s-expression — only literal results supported"
            )
        guard_py = _apply_replacements_once(guard_py, repl_map)
        result_py = _apply_replacements_once(result_py, repl_map)
        emitted_clauses.append((guard_py, result_py))

    if not emitted_clauses:
        raise DefineEmitError(
            "no translatable guarded clauses (only the base case + unguarded "
            "recursion were present)"
        )

    lines: list[str] = []
    lines.append(f"def {py_name}({', '.join(sig_parts)}) -> {return_py}:")
    lines.append(f'    """Generated from Shen define {define.name}."""')
    loop_param_name = param_names[loop_idx]
    lines.append(f"    for {elem_var} in {loop_param_name}:")
    for guard_py, result_py in emitted_clauses:
        lines.append(f"        if {guard_py}:")
        lines.append(f"            return {result_py}")
    lines.append(f"    return {base_result}")
    lines.append("")
    return lines


def _apply_replacements_once(text: str, repl_map: dict[str, str]) -> str:
    """Substitute identifiers in `text` using `repl_map` in a single pass.

    A `re.sub` callback is used so that each match position is consumed
    exactly once — preventing the cascade bug where the substituted token
    (e.g. `elem.block()`) contains another key (`block`) that would re-match.
    """
    if not repl_map:
        return text
    # Sort keys by length (longest first) so a key like `EntryRef` wins over
    # `Entry` if both are registered.
    keys = sorted(repl_map.keys(), key=len, reverse=True)
    pattern = re.compile(r'\b(' + '|'.join(re.escape(k) for k in keys) + r')\b')
    return pattern.sub(lambda m: repl_map[m.group(1)], text)


def _guard_to_py(guard: str, var_map: dict[str, str], st: dict) -> str:
    """Translate a where-clause s-expression to Python. Supports `=`, `>=`,
    `<=`, `>`, `<`, `not`, `and`, `or`, and `length`. Unsupported forms
    return `True  # TODO`."""
    if not guard.strip():
        return "True"
    expr = parse_sexpr(guard)
    if not expr.is_call():
        return "True"
    return _guard_expr_to_py(expr, var_map, st)


def _guard_expr_to_py(expr: SExpr, var_map: dict, st: dict) -> str:
    if expr.is_atom():
        a = expr.atom
        if a and (a[0].isdigit() or (a[0] == '-' and len(a) > 1)):
            return a
        if a and a[0] == '"':
            return a
        if a in var_map:
            r = Resolved(code=to_snake(a), typ=var_map[a])
            return unwrap(r, st)
        if a == 'true':
            return 'True'
        if a == 'false':
            return 'False'
        return to_snake(a) if a else ""

    op = expr.op()
    if op in ('and', 'or') and len(expr.children) == 3:
        l = _guard_expr_to_py(expr.children[1], var_map, st)
        r = _guard_expr_to_py(expr.children[2], var_map, st)
        py_op = 'and' if op == 'and' else 'or'
        return f"({l}) {py_op} ({r})"
    if op == 'not' and len(expr.children) == 2:
        inner = _guard_expr_to_py(expr.children[1], var_map, st)
        return f"not ({inner})"
    if op in ('=', '>=', '<=', '>', '<') and len(expr.children) == 3:
        l = _guard_expr_to_py(expr.children[1], var_map, st)
        r = _guard_expr_to_py(expr.children[2], var_map, st)
        py_op = '==' if op == '=' else op
        return f"{l} {py_op} {r}"
    if op == 'length' and len(expr.children) == 2:
        inner = _guard_expr_to_py(expr.children[1], var_map, st)
        return f"len({inner})"
    return "True  # TODO: unsupported guard form"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def field_py_type(shen_type: str, st: dict) -> str:
    # Handle parametric list types: (list X) → list[X-py-type]
    elem = list_elem_type(shen_type)
    if elem is not None:
        return f"list[{field_py_type(elem, st)}]"
    if shen_type in PRIMITIVES:
        return PRIMITIVES[shen_type]
    return to_pascal(shen_type)


def list_elem_type(shen_type: str) -> Optional[str]:
    """Extract the element type from `(list X)`, or None if not a list type."""
    if shen_type.startswith("(list ") and shen_type.endswith(")"):
        return shen_type[len("(list "):-1].strip()
    return None

def to_pascal(name: str) -> str:
    return "".join(w.capitalize() for w in name.split("-"))

def to_snake(name: str) -> str:
    if "-" in name:
        return name.replace("-", "_").lower()
    result = re.sub(r'([A-Z])', r'_\1', name).lower().lstrip('_')
    return result if result else name.lower()

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
            extra = " {" + ", ".join(f"{fi.shen_name}:{fi.shen_type}" for fi in info.fields) + "}"
        print(f"  {name:30s} {cat}{extra}", file=sys.stderr)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Generate Python guard types from Shen specs")
    parser.add_argument("spec", help="Path to .shen spec file")
    parser.add_argument("--out", help="Output file (default: stdout)")
    parser.add_argument("--mode", choices=["standard", "hardened"], default="standard")
    args = parser.parse_args()

    datatypes, defines = parse_file_full(args.spec)
    st = build_symbol_table(datatypes)
    print_symbol_table(st, args.spec)

    if args.mode == "hardened":
        code = emit_hardened(datatypes, st, args.spec, defines)
    else:
        code = emit_standard(datatypes, st, args.spec, defines)

    if args.out:
        with open(args.out, 'w') as f:
            f.write(code)
        print(f"Generated {args.out}", file=sys.stderr)
    else:
        print(code)


if __name__ == "__main__":
    main()
