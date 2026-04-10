import { test } from "node:test";
import assert from "node:assert/strict";
import { sym, num, bool_, sList, type Sexpr } from "./sexpr.ts";
import {
  match,
  intVal,
  boolVal,
  stringVal,
  listVal,
  type Value,
  type MatchResult,
} from "./match.ts";

// --- Helpers ---

// `(cons head tail)` pattern builder.
function cons(head: Sexpr, tail: Sexpr): Sexpr {
  return sList(sym("cons"), head, tail);
}

// `[A B]` desugars to (cons A (cons B nil)).
function fixedList(...items: Sexpr[]): Sexpr {
  let acc: Sexpr = sym("nil");
  for (let i = items.length - 1; i >= 0; i--) {
    acc = cons(items[i]!, acc);
  }
  return acc;
}

function expectMatched(r: MatchResult): Map<string, Value> {
  if (r.kind !== "matched") {
    assert.fail(`expected matched, got ${r.kind}${r.kind === "error" ? ": " + r.msg : ""}`);
  }
  return r.bindings;
}

// --- Tests ported from match_test.go ---

test("wildcard `_` matches anything and binds nothing", () => {
  const r = match(sym("_"), intVal(42));
  const b = expectMatched(r);
  assert.equal(b.size, 0);
});

test("uppercase variable binds the value", () => {
  const r = match(sym("X"), intVal(42));
  const b = expectMatched(r);
  const x = b.get("X");
  assert.ok(x);
  assert.deepEqual(x, intVal(42));
});

test("`nil` matches the empty list", () => {
  const r = match(sym("nil"), listVal());
  const b = expectMatched(r);
  assert.equal(b.size, 0);
});

test("`nil` does NOT match a non-empty list", () => {
  const r = match(sym("nil"), listVal(intVal(1)));
  assert.equal(r.kind, "miss");
});

test("cons pattern `[X | Xs]` binds head and tail", () => {
  const pat = cons(sym("X"), sym("Xs"));
  const lst = listVal(intVal(1), intVal(2), intVal(3));
  const b = expectMatched(match(pat, lst));
  assert.deepEqual(b.get("X"), intVal(1));
  const xs = b.get("Xs");
  assert.ok(xs && xs.kind === "list");
  assert.equal(xs.elems.length, 2);
  assert.deepEqual(xs.elems[0], intVal(2));
  assert.deepEqual(xs.elems[1], intVal(3));
});

test("cons pattern does not match empty list", () => {
  const pat = cons(sym("X"), sym("Xs"));
  assert.equal(match(pat, listVal()).kind, "miss");
});

test("fixed-length list `[A B]` matches a 2-element list", () => {
  const pat = fixedList(sym("A"), sym("B"));
  const b = expectMatched(match(pat, listVal(stringVal("x"), stringVal("y"))));
  assert.deepEqual(b.get("A"), stringVal("x"));
  assert.deepEqual(b.get("B"), stringVal("y"));
});

test("fixed-length `[A B]` does not match 1-element list", () => {
  const pat = fixedList(sym("A"), sym("B"));
  assert.equal(match(pat, listVal(stringVal("x"))).kind, "miss");
});

test("fixed-length `[A B]` does not match 3-element list", () => {
  const pat = fixedList(sym("A"), sym("B"));
  const v = listVal(stringVal("x"), stringVal("y"), stringVal("z"));
  assert.equal(match(pat, v).kind, "miss");
});

test("nested cons pattern binds inner and outer variables", () => {
  // [[X Y] | Rest]
  const pat = cons(fixedList(sym("X"), sym("Y")), sym("Rest"));
  const inner = listVal(stringVal("a"), stringVal("b"));
  const outer = listVal(inner, intVal(99));
  const b = expectMatched(match(pat, outer));
  assert.deepEqual(b.get("X"), stringVal("a"));
  assert.deepEqual(b.get("Y"), stringVal("b"));
  const rest = b.get("Rest");
  assert.ok(rest && rest.kind === "list");
  assert.equal(rest.elems.length, 1);
});

test("nested cons pattern misses when inner element wrong shape", () => {
  const pat = cons(fixedList(sym("X"), sym("Y")), sym("Rest"));
  const bad = listVal(listVal(stringVal("a")), intVal(99));
  assert.equal(match(pat, bad).kind, "miss");
});

test("boolean literal `true` matches BoolVal(true)", () => {
  assert.equal(match(bool_(true), boolVal(true)).kind, "matched");
  assert.equal(match(bool_(true), boolVal(false)).kind, "miss");
});

test("integer literal `42` matches IntVal(42)", () => {
  assert.equal(match(num(42), intVal(42)).kind, "matched");
  assert.equal(match(num(42), intVal(41)).kind, "miss");
});

test("cons pattern against int is a structural miss (not error)", () => {
  const pat = cons(sym("X"), sym("Xs"));
  const r = match(pat, intVal(1));
  assert.equal(r.kind, "miss");
});

test("malformed pattern: lowercase non-special symbol is an error", () => {
  const r = match(sym("foo"), intVal(1));
  assert.equal(r.kind, "error");
});

test("malformed pattern: non-cons list head is an error", () => {
  // (bogus X Y) should be rejected as a list pattern, not a miss.
  const pat = sList(sym("bogus"), sym("X"), sym("Y"));
  const r = match(pat, listVal(intVal(1), intVal(2)));
  assert.equal(r.kind, "error");
});
