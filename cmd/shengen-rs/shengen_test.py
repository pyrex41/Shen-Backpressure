"""Smoke tests for the Rust shengen emitter — hygiene fixes from W1.5
plus the (list X) and (define …) coverage added in W2.2.

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
        datatypes, defines = shengen.parse_file_full(path)
        st = shengen.build_symbol_table(datatypes)
        return shengen.emit_rust(datatypes, st, path, "guards_gen", mode, defines)
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


def test_list_x_in_composite_field_renders_vec_typed_field():
    """Premises of the form `Xs : (list X)` lower to Rust `Vec<X>` for the
    struct field and `&[X]` for the public accessor — idiomatic borrow
    semantics matching what a hand-written impl would expose."""
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
    assert "tags: Vec<TagId>" in out, out
    assert "pub fn tags(&self) -> &[TagId]" in out, out
    assert "pub type TagSet = Vec<TagId>" in out, out


def test_define_simple_predicate_emits_for_loop_function():
    """Predicate-style defines that iterate a single list parameter with
    an `[]` base case render as a Rust `pub fn` with a `for` loop."""
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
    assert "pub fn has_positive(" in out, out
    assert "&[Amount]" in out, out
    assert "-> bool" in out, out
    assert "for elem in" in out, out
    assert "elem.val() > 0" in out, out
    assert "return true;" in out, out


def test_define_with_ref_keyword_field_is_marked_unsupported():
    """Destructure bindings that would invoke a Rust-keyword accessor
    (e.g. `.ref()`) are flagged rather than emitted, because they would
    fail to compile."""
    spec = """(datatype tag-id
  X : string;
  ==============
  X : tag-id;)

(datatype ref-entry
  Ref : tag-id;
  Body : string;
  =====================
  [Ref Body] : ref-entry;)

(define matching?
  {tag-id --> (list ref-entry) --> boolean}
  Needle [] -> false
  Needle [[Ref Body] | Rest] -> true where (= Needle Ref))
"""
    out = _emit(spec)
    assert "// unsupported: define 'matching?'" in out, out
    assert "Rust keyword" in out, out
    assert "pub fn matching(" not in out, out


def test_define_with_non_supported_return_type_is_marked_unsupported():
    """Defines whose return type isn't bool/number stay flagged — the
    emitter doesn't synthesise borrow-vs-clone strategy on its own."""
    spec = """(datatype tag-id
  X : string;
  ==============
  X : tag-id;)

(define collect
  {(list tag-id) --> (list tag-id)}
  [] -> []
  [Head | Tail] -> Head where (= Head Head))
"""
    out = _emit(spec)
    assert "// unsupported: define 'collect'" in out, out
    assert "(list tag-id)" in out, out
    assert "pub fn collect(" not in out, out


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
