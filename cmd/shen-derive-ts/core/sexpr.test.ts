import { test } from "node:test";
import assert from "node:assert/strict";
import {
  sym,
  num,
  float_,
  str,
  bool_,
  sList,
  lambda,
  sApply,
  isSym,
  isMetaVar,
  headSym,
  listElems,
  atomVal,
  sexprIntVal,
  sexprFloatVal,
  sexprBoolVal,
  symName,
  sexprEqual,
  deepCopy,
  type Sexpr,
} from "./sexpr.ts";

// --- Constructor / equality tests (mirror TestParseAtoms cases) ---

test("atom constructors build equal values", () => {
  const cases: Array<[Sexpr, Sexpr]> = [
    [num(42), num(42)],
    [num(-7), num(-7)],
    [float_(3.14), float_(3.14)],
    [bool_(true), bool_(true)],
    [bool_(false), bool_(false)],
    [sym("foo"), sym("foo")],
    [sym("+"), sym("+")],
    [sym(">="), sym(">=")],
    [sym("?f"), sym("?f")],
    [str("hello"), str("hello")],
    [str("with\nnewline"), str("with\nnewline")],
  ];
  for (const [a, b] of cases) {
    assert.ok(sexprEqual(a, b), `expected equal: ${JSON.stringify(a)} vs ${JSON.stringify(b)}`);
  }
});

test("atoms of differing kind are not equal", () => {
  // int 42 and float 42 must be distinguishable at the atom level.
  assert.ok(!sexprEqual(num(42), float_(42)));
  assert.ok(!sexprEqual(num(1), bool_(true)));
  assert.ok(!sexprEqual(sym("true"), bool_(true)));
  assert.ok(!sexprEqual(sym("hello"), str("hello")));
});

test("sym preserves case", () => {
  const [name, ok] = symName(sym("Foo"));
  assert.ok(ok);
  assert.strictEqual(name, "Foo");
  assert.ok(!sexprEqual(sym("Foo"), sym("foo")));
});

// --- List constructors (mirror TestParseLists cases) ---

test("list constructors build equal values", () => {
  const cases: Array<[Sexpr, Sexpr]> = [
    [sList(sym("+"), num(1), num(2)), sList(sym("+"), num(1), num(2))],
    [
      sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1))),
      sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1))),
    ],
    [sList(), sList()],
    [sList(sym("f")), sList(sym("f"))],
    [
      sList(sym("f"), sList(sym("g"), sym("x"))),
      sList(sym("f"), sList(sym("g"), sym("x"))),
    ],
  ];
  for (const [a, b] of cases) {
    assert.ok(sexprEqual(a, b));
  }
});

test("lists of differing length or element are not equal", () => {
  assert.ok(!sexprEqual(sList(sym("+"), num(1)), sList(sym("+"), num(1), num(2))));
  assert.ok(!sexprEqual(sList(sym("+"), num(1), num(2)), sList(sym("-"), num(1), num(2))));
});

test("atom and list are never equal", () => {
  assert.ok(!sexprEqual(sym("foo"), sList(sym("foo"))));
  assert.ok(!sexprEqual(sList(), num(0)));
});

// --- lambda / sApply convenience constructors ---

test("lambda builds (lambda Param Body)", () => {
  const got = lambda("X", sList(sym("+"), sym("X"), num(1)));
  const want = sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1)));
  assert.ok(sexprEqual(got, want));
});

test("sApply builds (f arg1 arg2 ...)", () => {
  const got = sApply(sym("foldl"), sym("+"), num(0), sym("Xs"));
  const want = sList(sym("foldl"), sym("+"), num(0), sym("Xs"));
  assert.ok(sexprEqual(got, want));
});

test("sApply with no args", () => {
  const got = sApply(sym("f"));
  assert.ok(sexprEqual(got, sList(sym("f"))));
});

// --- isSym ---

test("isSym matches symbol name", () => {
  assert.ok(isSym(sym("foo"), "foo"));
  assert.ok(!isSym(sym("foo"), "bar"));
  assert.ok(!isSym(num(42), "42"));
  assert.ok(!isSym(str("foo"), "foo"));
  assert.ok(!isSym(sList(sym("foo")), "foo"));
});

// --- TestIsMetaVar ---

test("IsMetaVar: ?f is a metavar", () => {
  const [name, ok] = isMetaVar(sym("?f"));
  assert.ok(ok);
  assert.strictEqual(name, "?f");
});

test("IsMetaVar: f is not a metavar", () => {
  const [, ok] = isMetaVar(sym("f"));
  assert.ok(!ok);
});

test("IsMetaVar: 42 is not a metavar", () => {
  const [, ok] = isMetaVar(num(42));
  assert.ok(!ok);
});

test("IsMetaVar: bare ? is not a metavar (length > 1 requirement)", () => {
  const [, ok] = isMetaVar(sym("?"));
  assert.ok(!ok);
});

// --- TestHeadSym ---

test("HeadSym: (foldl 1) -> foldl", () => {
  assert.strictEqual(headSym(sList(sym("foldl"), num(1))), "foldl");
});

test("HeadSym: non-list -> empty", () => {
  assert.strictEqual(headSym(num(42)), "");
});

test("HeadSym: empty list -> empty", () => {
  assert.strictEqual(headSym(sList()), "");
});

test("HeadSym: list headed by non-symbol -> empty", () => {
  assert.strictEqual(headSym(sList(num(1), num(2))), "");
});

// --- listElems ---

test("listElems returns elements for list", () => {
  const elems = listElems(sList(sym("a"), sym("b")));
  assert.ok(elems !== null);
  assert.strictEqual(elems!.length, 2);
  assert.ok(sexprEqual(elems![0], sym("a")));
});

test("listElems returns null for atom", () => {
  assert.strictEqual(listElems(sym("foo")), null);
});

// --- atomVal ---

test("atomVal unpacks atom", () => {
  const [val, kind, ok] = atomVal(num(42));
  assert.ok(ok);
  assert.strictEqual(val, "42");
  assert.strictEqual(kind, "int");
});

test("atomVal returns false for list", () => {
  const [, , ok] = atomVal(sList());
  assert.ok(!ok);
});

// --- sexprIntVal ---

test("sexprIntVal reads int atom", () => {
  const [n, ok] = sexprIntVal(num(42));
  assert.ok(ok);
  assert.strictEqual(n, 42);
});

test("sexprIntVal rejects float atom", () => {
  const [, ok] = sexprIntVal(float_(3.14));
  assert.ok(!ok);
});

test("sexprIntVal rejects symbol", () => {
  const [, ok] = sexprIntVal(sym("foo"));
  assert.ok(!ok);
});

// --- sexprFloatVal ---

test("sexprFloatVal reads float atom", () => {
  const [f, ok] = sexprFloatVal(float_(3.14));
  assert.ok(ok);
  assert.ok(Math.abs(f - 3.14) < 1e-9);
});

test("sexprFloatVal accepts int atom (promotion)", () => {
  const [f, ok] = sexprFloatVal(num(5));
  assert.ok(ok);
  assert.strictEqual(f, 5);
});

test("sexprFloatVal rejects bool", () => {
  const [, ok] = sexprFloatVal(bool_(true));
  assert.ok(!ok);
});

// --- sexprBoolVal ---

test("sexprBoolVal reads bool atoms", () => {
  const [t, okT] = sexprBoolVal(bool_(true));
  assert.ok(okT);
  assert.strictEqual(t, true);
  const [f, okF] = sexprBoolVal(bool_(false));
  assert.ok(okF);
  assert.strictEqual(f, false);
});

test("sexprBoolVal rejects non-bool", () => {
  const [, ok] = sexprBoolVal(sym("true"));
  assert.ok(!ok);
});

// --- symName ---

test("symName reads symbol", () => {
  const [name, ok] = symName(sym("foo"));
  assert.ok(ok);
  assert.strictEqual(name, "foo");
});

test("symName rejects int", () => {
  const [, ok] = symName(num(42));
  assert.ok(!ok);
});

// --- TestDeepCopy (direct port) ---

test("DeepCopy: copy equals original", () => {
  const orig = sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1)));
  const cp = deepCopy(orig);
  assert.ok(sexprEqual(orig, cp));
});

test("DeepCopy: mutating copy does not affect original", () => {
  const orig = sList(sym("lambda"), sym("X"), sList(sym("+"), sym("X"), num(1)));
  const cp = deepCopy(orig);
  assert.strictEqual(cp.kind, "list");
  if (cp.kind !== "list") return;
  cp.elems[1] = sym("Y");
  assert.strictEqual(orig.kind, "list");
  if (orig.kind !== "list") return;
  const second = orig.elems[1];
  assert.strictEqual(second.kind, "atom");
  if (second.kind !== "atom") return;
  assert.strictEqual(second.val, "X");
});

test("DeepCopy: mutating nested list in copy does not affect original", () => {
  const orig = sList(sym("f"), sList(sym("g"), num(1)));
  const cp = deepCopy(orig);
  assert.strictEqual(cp.kind, "list");
  if (cp.kind !== "list") return;
  const nested = cp.elems[1];
  assert.strictEqual(nested.kind, "list");
  if (nested.kind !== "list") return;
  nested.elems[0] = sym("h");
  assert.strictEqual(orig.kind, "list");
  if (orig.kind !== "list") return;
  const origNested = orig.elems[1];
  assert.strictEqual(origNested.kind, "list");
  if (origNested.kind !== "list") return;
  const head = origNested.elems[0];
  assert.strictEqual(head.kind, "atom");
  if (head.kind !== "atom") return;
  assert.strictEqual(head.val, "g");
});

test("DeepCopy: atom copy is a fresh object", () => {
  const a = sym("X");
  const b = deepCopy(a);
  assert.ok(sexprEqual(a, b));
  assert.notStrictEqual(a, b);
});
