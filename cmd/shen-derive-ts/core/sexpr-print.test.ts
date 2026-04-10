import { test } from "node:test";
import assert from "node:assert/strict";

import {
  type Sexpr,
  sym,
  num,
  float_,
  str,
  bool_,
  sList,
  sexprEqual,
} from "./sexpr.ts";
import { prettyPrintSexpr } from "./sexpr-print.ts";

test("prettyPrint: atoms", () => {
  assert.equal(prettyPrintSexpr(num(42)), "42");
  assert.equal(prettyPrintSexpr(num(-7)), "-7");
  assert.equal(prettyPrintSexpr(sym("foo")), "foo");
  assert.equal(prettyPrintSexpr(sym("+")), "+");
  assert.equal(prettyPrintSexpr(sym("?f")), "?f");
  assert.equal(prettyPrintSexpr(bool_(true)), "true");
  assert.equal(prettyPrintSexpr(bool_(false)), "false");
  assert.equal(prettyPrintSexpr(float_(3.14)), "3.14");
});

test("prettyPrint: strings with escapes", () => {
  assert.equal(prettyPrintSexpr(str("hello")), `"hello"`);
  assert.equal(prettyPrintSexpr(str("hello world")), `"hello world"`);
  assert.equal(prettyPrintSexpr(str("with\nnewline")), `"with\\nnewline"`);
  assert.equal(prettyPrintSexpr(str('a"b')), `"a\\"b"`);
  assert.equal(prettyPrintSexpr(str("a\\b")), `"a\\\\b"`);
  assert.equal(prettyPrintSexpr(str("tab\there")), `"tab\\there"`);
});

test("prettyPrint: empty list", () => {
  assert.equal(prettyPrintSexpr(sList()), "()");
});

test("prettyPrint: simple list", () => {
  assert.equal(
    prettyPrintSexpr(sList(sym("+"), num(1), num(2))),
    "(+ 1 2)",
  );
});

test("prettyPrint: nested lists", () => {
  const expr = sList(
    sym("lambda"),
    sym("X"),
    sList(sym("+"), sym("X"), num(1)),
  );
  assert.equal(prettyPrintSexpr(expr), "(lambda X (+ X 1))");
});

test("prettyPrint: cons-list sugar [1 2 3]", () => {
  // (cons 1 (cons 2 (cons 3 nil))) -> [1 2 3]
  const expr = sList(
    sym("cons"),
    num(1),
    sList(
      sym("cons"),
      num(2),
      sList(sym("cons"), num(3), sym("nil")),
    ),
  );
  assert.equal(prettyPrintSexpr(expr), "[1 2 3]");
});

test("prettyPrint: cons with non-nil tail [a | b]", () => {
  // (cons a b) -> [a | b]
  const expr = sList(sym("cons"), sym("a"), sym("b"));
  assert.equal(prettyPrintSexpr(expr), "[a | b]");
});

test("prettyPrint: cons chain with non-nil tail", () => {
  // (cons 1 (cons 2 tail)) -> [1 2 | tail]
  const expr = sList(
    sym("cons"),
    num(1),
    sList(sym("cons"), num(2), sym("tail")),
  );
  assert.equal(prettyPrintSexpr(expr), "[1 2 | tail]");
});

test("prettyPrint: non-cons 3-list stays in parens", () => {
  assert.equal(
    prettyPrintSexpr(sList(sym("foo"), num(1), num(2))),
    "(foo 1 2)",
  );
});

// --- Structural round-trip via print twice.
// sexpr-parse.ts does not yet exist in this tree, so we cannot do a
// parse->print->parse round-trip. Instead we verify that printing is
// deterministic and that structurally equal inputs produce identical output.
test("roundTrip: deterministic print (proxy for round-trip)", () => {
  const inputs: Sexpr[] = [
    num(42),
    sList(sym("+"), num(1), num(2)),
    sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1))),
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
    sList(
      sym("compose"),
      sList(sym("map"), sym("?f")),
      sList(sym("map"), sym("?g")),
    ),
    str("hello world"),
    bool_(true),
    bool_(false),
  ];
  for (const expr of inputs) {
    const printed = prettyPrintSexpr(expr);
    // printing again yields the same string
    assert.equal(prettyPrintSexpr(expr), printed);
    // structural equality with itself (sanity)
    assert.ok(sexprEqual(expr, expr));
  }
});

test("roundTrip: cons-list sugar prints to [1 2 3]", () => {
  const expr = sList(
    sym("cons"),
    num(1),
    sList(
      sym("cons"),
      num(2),
      sList(sym("cons"), num(3), sym("nil")),
    ),
  );
  assert.equal(prettyPrintSexpr(expr), "[1 2 3]");
});
