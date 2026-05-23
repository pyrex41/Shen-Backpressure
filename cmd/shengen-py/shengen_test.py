"""Smoke tests for the Python shengen emitter — specifically the hygiene
fixes that mirror the Go fixtures in cmd/shengen/main_test.go.

Run with: python3 cmd/shengen-py/shengen_test.py
"""
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
    with tempfile.NamedTemporaryFile("w", suffix=".shen", delete=False) as f:
        f.write(spec_src)
        path = f.name
    try:
        datatypes = shengen.parse_file(path)
        st = shengen.build_symbol_table(datatypes)
        return shengen.emit_standard(datatypes, st, path)
    finally:
        os.unlink(path)


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
