"""Smoke tests for the Python shengen emitter — hygiene fixes from W1.5
plus the (list X) and (define …) coverage added in W2.2.

Run with: python3 cmd/shengen-py/shengen_test.py
"""
import ast
import importlib.util
import os
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC_MOD = importlib.util.spec_from_file_location(
    "shengen_py", os.path.join(HERE, "shengen.py")
)
shengen = importlib.util.module_from_spec(SPEC_MOD)
SPEC_MOD.loader.exec_module(shengen)


def _emit(spec_src: str) -> str:
    """Emit Python code from a spec source string. Returns the output."""
    with tempfile.NamedTemporaryFile("w", suffix=".shen", delete=False) as f:
        f.write(spec_src)
        path = f.name
    try:
        datatypes, defines = shengen.parse_file_full(path)
        st = shengen.build_symbol_table(datatypes)
        return shengen.emit_standard(datatypes, st, path, defines)
    finally:
        os.unlink(path)


def _assert_valid_python(code: str) -> None:
    """Fail with a useful message if `code` is not parseable Python."""
    try:
        ast.parse(code)
    except SyntaxError as exc:
        # Give the assertion message enough context to debug from CI logs.
        lines = code.split("\n")
        ctx_start = max(0, (exc.lineno or 1) - 3)
        ctx_end = min(len(lines), (exc.lineno or 1) + 2)
        ctx = "\n".join(
            f"{i+1:>4}: {lines[i]}" for i in range(ctx_start, ctx_end)
        )
        raise AssertionError(f"emitter produced invalid Python:\n{ctx}\n{exc}") from exc


def test_negate_py_expr_peephole():
    assert shengen.negate_py_expr('not (x == "")') == '(x == "")'
    assert shengen.negate_py_expr("not (a == b)") == "(a == b)"
    assert shengen.negate_py_expr("x >= 0") == "not (x >= 0)"
    assert shengen.negate_py_expr("foo(x)") == "not (foo(x))"
    # Outer `not (...)` closes before the end → not redundant.
    assert shengen.negate_py_expr("not (a) and b") == "not (not (a) and b)"
    assert shengen.negate_py_expr("not ((a))") == "((a))"


def test_not_empty_string_emits_friendly_message_and_collapses_double_negation():
    spec = """(datatype secret
  X : string;
  (not (= X "")) : verified;
  ==========================
  X : secret;)
"""
    out = _emit(spec)
    assert "if not (not (self._v == \"\"))" not in out, out
    assert "if (self._v == \"\"):" in out, out
    # The message string references the Shen variable name (camelCased to `x`),
    # not the rewritten `self._v` form which is only used in the predicate.
    assert "x must not be empty" in out, out
    assert "not: " not in out, out


def test_not_equals_pair_emits_must_not_equal():
    spec = """(datatype distinct-pair
  A : string;
  B : string;
  (not (= A B)) : verified;
  =========================
  [A B] : distinct-pair;)
"""
    out = _emit(spec)
    assert "if not (not (self._a == self._b))" not in out, out
    assert "if (self._a == self._b):" in out, out
    # The error message string keeps the Shen variable names (lowercase),
    # not the rewritten `self._a` form which is only used in the predicate.
    assert "a must not equal b" in out, out


def test_positive_eq_empty_keeps_outer_negation():
    spec = """(datatype empty-string
  X : string;
  (= X "") : verified;
  ====================
  X : empty-string;)
"""
    out = _emit(spec)
    assert 'if not (self._v == ""):' in out, out


def test_list_x_in_composite_field_renders_list_typed_field():
    """Premises of the form `Xs : (list X)` lower to Python `list[X]`.

    The Go and TS emitters have long supported this; W2.2 brings the Py
    emitter to parity. Test covers the parser regex, the field-type
    renderer, and the factory's choice to skip isinstance checks for lists.
    """
    spec = """(datatype tag-id
  X : string;
  ==============
  X : tag-id;)

(datatype tag-set
  Tags : (list tag-id);
  ====================
  Tags : tag-set;)

(datatype tag-bundle
  Id : tag-id;
  Tags : (list tag-id);
  ========================
  [Id Tags] : tag-bundle;)
"""
    out = _emit(spec)
    _assert_valid_python(out)
    # Composite field renders as list[TagId].
    assert "_tags: list[TagId]" in out, out
    assert "def tags(self) -> list[TagId]:" in out, out
    # Factory parameter typed as list[TagId]; no `isinstance(tags, list[TagId])`
    # check is emitted (parametric generics aren't isinstance-compatible).
    assert "tags: list[TagId]" in out, out
    assert "isinstance(tags, list[TagId])" not in out, out
    # Alias resolution: `tag-set` aliases `(list tag-id)` and should render
    # as a `list[TagId]` type alias, not a malformed `(list tagId)` literal.
    assert "TagSet = list[TagId]" in out, out


def test_define_simple_predicate_emits_loop_function():
    """A define that iterates over a list with a `[]` base case and a
    guarded clause should emit a Python function with a for-loop body.

    This is the smallest define the emitter must handle: predicate-style
    recursion over a single list parameter.
    """
    spec = """(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(define has-positive?
  {(list amount) --> boolean}
  [] -> false
  [Amt | Rest] -> true where (> Amt 0))
"""
    out = _emit(spec)
    _assert_valid_python(out)
    # Function emitted with the expected signature shape.
    assert "def has_positive(" in out, out
    assert ": list[Amount]" in out, out
    assert "-> bool:" in out, out
    # The loop body comes from the guarded clause.
    assert "for elem in" in out, out
    # `Amt` in the guard rewrites to `elem` (single-binding destructure)
    # and the `>` predicate translates directly.
    assert "elem.val() > 0" in out, out
    assert "return True" in out, out
    # Base case `[]` → false → False fallback after the loop.
    assert "return False" in out, out


def test_define_with_destructure_emits_field_accessor_calls():
    """A define that destructures the list element via `[[X Y] | Rest]`
    must use the composite's field accessors in the guard, not bare
    pattern variable names."""
    spec = """(datatype tag-id
  X : string;
  ==============
  X : tag-id;)

(datatype tag-block
  Body : string;
  Sig : tag-id;
  =================
  [Body Sig] : tag-block;)

(datatype block-table
  Entries : (list tag-block);
  =============================
  Entries : block-table;)

(define has-body?
  {string --> block-table --> boolean}
  Needle [] -> false
  Needle [[Body Sig] | Rest] -> true where (= Needle Body))
"""
    out = _emit(spec)
    _assert_valid_python(out)
    # Destructured fields rewrite to method calls on the loop variable.
    assert "needle == elem.body()" in out, out
    # Pattern variable `Needle` becomes the named parameter `needle`,
    # not a stray `arg0`.
    assert "def has_body(needle: str" in out, out


def test_define_with_no_base_case_for_non_bool_is_marked_unsupported():
    """Defines without an `[]` base case must not silently fabricate a
    return value when the return type isn't `boolean`. The Python emitter
    flags them as unsupported and emits valid Python that does not include
    the broken function."""
    spec = """(datatype tag-id
  X : string;
  ==============
  X : tag-id;)

(define lookup
  {tag-id --> (list tag-id) --> tag-id}
  Ref [Match | Rest] -> Match where (= Ref Match))
"""
    out = _emit(spec)
    _assert_valid_python(out)
    assert "# unsupported: define 'lookup'" in out, out
    assert "def lookup(" not in out, out


def main():
    failures = []
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"PASS {name}")
            except AssertionError as exc:
                failures.append((name, exc))
                print(f"FAIL {name}\n  {exc}")
    if failures:
        sys.exit(1)


if __name__ == "__main__":
    main()
