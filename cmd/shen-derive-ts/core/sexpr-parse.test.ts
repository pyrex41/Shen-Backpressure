// Tests for sexpr-parse.ts — ported from shen-derive/core/sexpr_test.go.

import { test } from "node:test";
import assert from "node:assert/strict";

import { parseSexpr, parseAllSexprs } from "./sexpr-parse.ts";
import {
  type Sexpr,
  sym,
  num,
  float_,
  str,
  bool_,
  sList,
  sexprEqual,
  headSym,
} from "./sexpr.ts";

function assertEqualSexpr(got: Sexpr, want: Sexpr, msg?: string): void {
  if (!sexprEqual(got, want)) {
    assert.fail(
      `${msg ?? "sexpr mismatch"}\n  got:  ${JSON.stringify(got)}\n  want: ${JSON.stringify(want)}`,
    );
  }
}

test("parseAtoms — int, negative int, float, bools, symbols, metavars, strings", () => {
  const cases: Array<[string, Sexpr]> = [
    ["42", num(42)],
    ["-7", num(-7)],
    ["3.14", float_(3.14)],
    ["true", bool_(true)],
    ["false", bool_(false)],
    ["foo", sym("foo")],
    ["+", sym("+")],
    [">=", sym(">=")],
    ["?f", sym("?f")],
    [`"hello"`, str("hello")],
    [`"with\\nnewline"`, str("with\nnewline")],
  ];
  for (const [input, want] of cases) {
    const got = parseSexpr(input);
    assertEqualSexpr(got, want, `parseSexpr(${JSON.stringify(input)})`);
  }
});

test("parseAtoms — string tab and escaped quote and backslash", () => {
  assertEqualSexpr(parseSexpr(`"a\\tb"`), str("a\tb"));
  assertEqualSexpr(parseSexpr(`"say \\"hi\\""`), str(`say "hi"`));
  assertEqualSexpr(parseSexpr(`"back\\\\slash"`), str("back\\slash"));
});

test("parseAtoms — symbol case is preserved", () => {
  assertEqualSexpr(parseSexpr("X"), sym("X"));
  assertEqualSexpr(parseSexpr("FooBar"), sym("FooBar"));
  assertEqualSexpr(parseSexpr("nil"), sym("nil"));
});

test("parseAtoms — int vs float distinction", () => {
  const five = parseSexpr("5");
  assert.equal(five.kind, "atom");
  if (five.kind === "atom") assert.equal(five.atomKind, "int");

  const fiveDot = parseSexpr("5.0");
  assert.equal(fiveDot.kind, "atom");
  if (fiveDot.kind === "atom") assert.equal(fiveDot.atomKind, "float");
});

test("parseLists — basic and nested", () => {
  const cases: Array<[string, Sexpr]> = [
    ["(+ 1 2)", sList(sym("+"), num(1), num(2))],
    [
      "(lambda X (+ X 1))",
      sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1))),
    ],
    ["()", sList()],
    ["(f)", sList(sym("f"))],
    ["(f (g x))", sList(sym("f"), sList(sym("g"), sym("x")))],
  ];
  for (const [input, want] of cases) {
    assertEqualSexpr(parseSexpr(input), want, `parseSexpr(${JSON.stringify(input)})`);
  }
});

test("parseSquareList — [1 2 3] desugars to cons chain", () => {
  const got = parseSexpr("[1 2 3]");
  const want = sList(
    sym("cons"),
    num(1),
    sList(sym("cons"), num(2), sList(sym("cons"), num(3), sym("nil"))),
  );
  assertEqualSexpr(got, want);
});

test("parseSquareList — [a | b] desugars to (cons a b)", () => {
  const got = parseSexpr("[a | b]");
  const want = sList(sym("cons"), sym("a"), sym("b"));
  assertEqualSexpr(got, want);
});

test("parseSquareList — [X | Xs] preserves uppercase symbols", () => {
  const got = parseSexpr("[X | Xs]");
  const want = sList(sym("cons"), sym("X"), sym("Xs"));
  assertEqualSexpr(got, want);
});

test("parseSquareList — [A B] desugars correctly", () => {
  const got = parseSexpr("[A B]");
  const want = sList(
    sym("cons"),
    sym("A"),
    sList(sym("cons"), sym("B"), sym("nil")),
  );
  assertEqualSexpr(got, want);
});

test("parseSquareList — [] desugars to nil", () => {
  assertEqualSexpr(parseSexpr("[]"), sym("nil"));
});

test("parseComments — Shen-style \\\\ line comments", () => {
  const got = parseSexpr(`
    \\\\ this is a comment
    (+ 1 2)
  `);
  assertEqualSexpr(got, sList(sym("+"), num(1), num(2)));
});

test("parseComments — -- line comments", () => {
  const got = parseSexpr(`
    -- this is a comment
    42
  `);
  assertEqualSexpr(got, num(42));
});

test("parseComments — comment after expression is tolerated by skipWhitespace", () => {
  const got = parseSexpr("(+ 1 2) \\\\ trailing");
  assertEqualSexpr(got, sList(sym("+"), num(1), num(2)));
});

test("parseAllSexprs — multiple top-level forms", () => {
  const input = `
    (define sum {(list number) --> number}
      Xs -> (foldl + 0 Xs))

    (derive sum
      (rewrite foldl-map-fusion))
  `;
  const results = parseAllSexprs(input);
  assert.equal(results.length, 2);
  assert.equal(headSym(results[0]), "define");
  assert.equal(headSym(results[1]), "derive");
});

test("parseAllSexprs — empty input yields empty array", () => {
  assert.deepEqual(parseAllSexprs(""), []);
  assert.deepEqual(parseAllSexprs("   \n\t  "), []);
  assert.deepEqual(parseAllSexprs("\\\\ just a comment\n"), []);
});

test("round-trip structural — parse then re-parse of equivalent text", () => {
  // Since no pretty-printer is ported yet, we verify that the parser is
  // stable by parsing canonical forms directly.
  const inputs: Array<[string, Sexpr]> = [
    ["42", num(42)],
    ["(+ 1 2)", sList(sym("+"), num(1), num(2))],
    [
      "(lambda X (+ X 1))",
      sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1))),
    ],
    [
      "(foldl (lambda X (lambda Acc (+ X Acc))) 0 Xs)",
      sList(
        sym("foldl"),
        sList(
          sym("lambda"),
          sym("X"),
          sList(sym("lambda"), sym("Acc"), sList(sym("+"), sym("X"), sym("Acc"))),
        ),
        num(0),
        sym("Xs"),
      ),
    ],
    [
      "(compose (map ?f) (map ?g))",
      sList(
        sym("compose"),
        sList(sym("map"), sym("?f")),
        sList(sym("map"), sym("?g")),
      ),
    ],
    [`"hello world"`, str("hello world")],
    ["true", bool_(true)],
    ["false", bool_(false)],
  ];
  for (const [input, want] of inputs) {
    assertEqualSexpr(parseSexpr(input), want, `round-trip ${JSON.stringify(input)}`);
  }
});

test("whitespace — leading, trailing, and internal", () => {
  assertEqualSexpr(parseSexpr("   42   "), num(42));
  assertEqualSexpr(parseSexpr("\n\t(+ 1\n  2)\n"), sList(sym("+"), num(1), num(2)));
});

test("floats — negative and positive", () => {
  const neg = parseSexpr("-3.14");
  assert.equal(neg.kind, "atom");
  if (neg.kind === "atom") {
    assert.equal(neg.atomKind, "float");
    assert.equal(neg.val, "-3.14");
  }
  const pos = parseSexpr("+2.5");
  assert.equal(pos.kind, "atom");
  if (pos.kind === "atom") {
    assert.equal(pos.atomKind, "float");
  }
});

test("errors — unclosed paren throws", () => {
  assert.throws(() => parseSexpr("(+ 1 2"), /unclosed/);
});

test("errors — unclosed square bracket throws", () => {
  assert.throws(() => parseSexpr("[1 2 3"), /unclosed/);
});

test("errors — unclosed string throws", () => {
  assert.throws(() => parseSexpr(`"no end`), /unclosed string/);
});

test("errors — stray closing paren throws", () => {
  assert.throws(() => parseSexpr(")"), /unexpected/);
});

test("errors — trailing garbage after expression throws", () => {
  assert.throws(() => parseSexpr("(+ 1 2) extra"), /unexpected input/);
});
