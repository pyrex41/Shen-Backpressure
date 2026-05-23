"""Smoke tests for the Rust shengen emitter — specifically the hygiene
fixes that mirror the Go fixtures in cmd/shengen/main_test.go.

Run with: python3 cmd/shengen-rs/shengen_test.py
"""
import importlib.util
import os
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC_MOD = importlib.util.spec_from_file_location(
    "shengen_rs", os.path.join(HERE, "shengen.py")
)
shengen = importlib.util.module_from_spec(SPEC_MOD)
SPEC_MOD.loader.exec_module(shengen)


def _emit(spec_src: str, mode: str = "standard") -> str:
    with tempfile.NamedTemporaryFile("w", suffix=".shen", delete=False) as f:
        f.write(spec_src)
        path = f.name
    try:
        datatypes = shengen.parse_file(path)
        st = shengen.build_symbol_table(datatypes)
        return shengen.emit_rust(datatypes, st, path, "guards_gen", mode)
    finally:
        os.unlink(path)


def test_negate_rust_expr_peephole():
    assert shengen.negate_rust_expr('!(x == "")') == '(x == "")'
    assert shengen.negate_rust_expr("!(a == b)") == "(a == b)"
    assert shengen.negate_rust_expr("x >= 0.0") == "!(x >= 0.0)"
    assert shengen.negate_rust_expr("foo(x)") == "!(foo(x))"
    assert shengen.negate_rust_expr("!(a) && b") == "!(!(a) && b)"
    assert shengen.negate_rust_expr("!((a))") == "((a))"


def test_not_empty_string_emits_friendly_message_and_collapses_double_negation():
    spec = """(datatype secret
  X : string;
  (not (= X "")) : verified;
  ==========================
  X : secret;)
"""
    out = _emit(spec)
    assert 'if !(!(x == ""))' not in out, out
    assert 'if (x == "")' in out, out
    assert "x must not be empty" in out, out
    assert 'message: format!("not:' not in out, out


def test_not_equals_pair_emits_must_not_equal():
    spec = """(datatype distinct-pair
  A : string;
  B : string;
  (not (= A B)) : verified;
  =========================
  [A B] : distinct-pair;)
"""
    out = _emit(spec)
    assert "if !(!(a == b))" not in out, out
    assert "if (a == b)" in out, out
    assert "a must not equal b" in out, out


def test_positive_eq_empty_keeps_outer_negation():
    spec = """(datatype empty-string
  X : string;
  (= X "") : verified;
  ====================
  X : empty-string;)
"""
    out = _emit(spec)
    assert 'if !(x == "")' in out, out


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
