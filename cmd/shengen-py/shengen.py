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
    with open(path) as f:
        text = f.read()
    text = re.sub(r'\\\*.*?\*\\', '', text, flags=re.DOTALL)
    datatypes = []
    i = 0
    while i < len(text):
        m = re.search(r'\(datatype\s+([\w-]+)', text[i:])
        if not m:
            break
        start = i + m.start()
        name = m.group(1)
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
        tm = re.match(r'(\w+)\s*:\s*([\w-]+(?:\s*\(.*?\))?)\s*$', line)
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
        return (f"{unwrap(lhs, st)} == {unwrap(rhs, st)}", f"{lhs.code} must equal {rhs.code}")
    if op == "not":
        inner = resolve(expr.children[1], var_map, st)
        return (f"not ({inner.code})", f"not {vp.raw}")
    if op == "element?":
        r = resolve(expr, var_map, st)
        return (r.code, "must be a valid member")
    return (f"True  # TODO: {vp.raw}", vp.raw)


# ---------------------------------------------------------------------------
# Emitter — Standard Mode
# ---------------------------------------------------------------------------

def emit_standard(datatypes: list[Datatype], st: dict[str, TypeInfo], spec_path: str) -> str:
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

    return "\n".join(lines)

def emit_type_standard(info: TypeInfo, rule: Rule, st: dict) -> list[str]:
    lines = []
    cat = info.category
    lines.append("")
    lines.append(f"# --- {info.py_name} ---")
    lines.append(f"# Shen: (datatype {info.shen_name})")

    if cat == "alias":
        target = to_pascal(info.wrapped_type or "")
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
                lines.append(f"        if not ({code}):")
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
                lines.append(f"        if not ({code}):")
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
        # Type checks for composite args
        for fi in info.fields:
            if fi.shen_type not in PRIMITIVES:
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

def emit_hardened(datatypes: list[Datatype], st: dict[str, TypeInfo], spec_path: str) -> str:
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

    return "\n".join(lines)

def emit_type_hardened(info: TypeInfo, rule: Rule, st: dict) -> list[str]:
    lines = []
    cat = info.category
    lines.append("")
    lines.append(f"# --- {info.py_name} ---")
    lines.append(f"# Shen: (datatype {info.shen_name})")

    if cat == "alias":
        target = to_pascal(info.wrapped_type or "")
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
                lines.append(f"    if not ({code}):")
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

        # Type checks
        for fi in info.fields:
            if fi.shen_type not in PRIMITIVES:
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
                lines.append(f"    if not ({code}):")
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
            else:
                tag_parts.append(f'{pname}._tag.encode("utf-8")')
        lines.append(f'    object.__setattr__(obj, "_tag", _hmac_tag("{info.py_name}", {", ".join(tag_parts)}))')
        lines.append(f"    {info.py_name}._registry.add(obj)")
        lines.append("    return obj")

    lines.append("")
    return lines


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def field_py_type(shen_type: str, st: dict) -> str:
    if shen_type in PRIMITIVES:
        return PRIMITIVES[shen_type]
    return to_pascal(shen_type)

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

    datatypes = parse_file(args.spec)
    st = build_symbol_table(datatypes)
    print_symbol_table(st, args.spec)

    if args.mode == "hardened":
        code = emit_hardened(datatypes, st, args.spec)
    else:
        code = emit_standard(datatypes, st, args.spec)

    if args.out:
        with open(args.out, 'w') as f:
            f.write(code)
        print(f"Generated {args.out}", file=sys.stderr)
    else:
        print(code)


if __name__ == "__main__":
    main()
