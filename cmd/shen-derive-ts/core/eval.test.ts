// Tests for the shen-derive evaluator. Ported 1:1 from
// `shen-derive/core/eval_test.go`. The Go tests compare `v.String()`
// output; here we inspect the discriminated-union `kind` and `val`
// fields directly because TS Values don't carry a String() method.

import { test } from "node:test";
import assert from "node:assert/strict";

import { parseSexpr } from "./sexpr-parse.ts";
import {
  Env,
  evalExpr,
  type Value,
} from "./eval.ts";

// --- helpers ---

function run(src: string): Value {
  return evalExpr(Env.empty(), parseSexpr(src));
}

function assertInt(v: Value, want: number) {
  assert.equal(v.kind, "int", `expected int, got ${v.kind}`);
  if (v.kind === "int") assert.equal(v.val, want);
}

function assertFloat(v: Value, want: number) {
  assert.equal(v.kind, "float", `expected float, got ${v.kind}`);
  if (v.kind === "float") assert.equal(v.val, want);
}

function assertBool(v: Value, want: boolean) {
  assert.equal(v.kind, "bool", `expected bool, got ${v.kind}`);
  if (v.kind === "bool") assert.equal(v.val, want);
}

function assertIntList(v: Value, want: number[]) {
  assert.equal(v.kind, "list", `expected list, got ${v.kind}`);
  if (v.kind !== "list") return;
  assert.equal(v.elems.length, want.length);
  for (let i = 0; i < want.length; i++) {
    assertInt(v.elems[i]!, want[i]!);
  }
}

// --- Literals ---

test("eval: integer literal", () => {
  assertInt(run("42"), 42);
});

test("eval: negative integer literal parses", () => {
  assertInt(run("(- 0 5)"), -5);
});

test("eval: float literal", () => {
  assertFloat(run("3.14"), 3.14);
});

test("eval: bool literal true", () => {
  assertBool(run("true"), true);
});

test("eval: bool literal false", () => {
  assertBool(run("false"), false);
});

test("eval: string literal", () => {
  const v = run('"hello"');
  assert.equal(v.kind, "string");
  if (v.kind === "string") assert.equal(v.val, "hello");
});

test("eval: nil is the empty list", () => {
  const v = run("nil");
  assert.equal(v.kind, "list");
  if (v.kind === "list") assert.equal(v.elems.length, 0);
});

// --- Arithmetic ---

test("eval: (+ 1 2) = 3 (int)", () => {
  assertInt(run("(+ 1 2)"), 3);
});

test("eval: nested int arithmetic", () => {
  assertInt(run("(* (+ 2 3) (- 10 4))"), 30);
});

test("eval: int/int division truncates toward zero", () => {
  // (/ 10 4) → 2, not 2.5
  assertInt(run("(/ 10 4)"), 2);
});

test("eval: (/ 10.0 4) → 2.5 (float promoted)", () => {
  assertFloat(run("(/ 10.0 4)"), 2.5);
});

test("eval: float + float", () => {
  assertFloat(run("(+ 0.5 0.5)"), 1);
});

test("eval: int + float promotes to float", () => {
  assertFloat(run("(+ 1 0.5)"), 1.5);
});

test("eval: float - float", () => {
  assertFloat(run("(- 1.5 0.5)"), 1);
});

test("eval: int * float promotes to float", () => {
  assertFloat(run("(* 2 2.5)"), 5);
});

test("eval: modulo int only", () => {
  assertInt(run("(% 10 3)"), 1);
});

test("eval: division by zero throws (int)", () => {
  assert.throws(() => run("(/ 5 0)"), /division by zero/);
});

test("eval: division by zero throws (float)", () => {
  assert.throws(() => run("(/ 5.0 0.0)"), /division by zero/);
});

test("eval: modulo by zero throws", () => {
  assert.throws(() => run("(% 5 0)"), /modulo by zero/);
});

// --- Comparisons ---

test("eval: (>= 0.5 0) = true", () => {
  assertBool(run("(>= 0.5 0)"), true);
});

test("eval: (< 0.1 0.2) = true", () => {
  assertBool(run("(< 0.1 0.2)"), true);
});

test("eval: (= 1 1.0) = true (cross-kind numeric equality)", () => {
  assertBool(run("(= 1 1.0)"), true);
});

test("eval: (!= 1.0 2) = true", () => {
  assertBool(run("(!= 1.0 2)"), true);
});

test("eval: (> 3 2) = true", () => {
  assertBool(run("(> 3 2)"), true);
});

test("eval: (<= 3 3) = true", () => {
  assertBool(run("(<= 3 3)"), true);
});

// --- Boolean ---

test("eval: (and true (>= 5 3)) = true", () => {
  assertBool(run("(and true (>= 5 3))"), true);
});

test("eval: (not (< 5 3)) = true", () => {
  assertBool(run("(not (< 5 3))"), true);
});

test("eval: (or false true) = true", () => {
  assertBool(run("(or false true)"), true);
});

test("eval: (and true false) = false", () => {
  assertBool(run("(and true false)"), false);
});

// --- Lambda / let / if ---

test("eval: ((lambda X (+ X 1)) 5) = 6", () => {
  assertInt(run("((lambda X (+ X 1)) 5)"), 6);
});

test("eval: (let Y (+ 3 4) (* Y 2)) = 14", () => {
  assertInt(run("(let Y (+ 3 4) (* Y 2))"), 14);
});

test("eval: (if (> 3 2) 10 20) = 10", () => {
  assertInt(run("(if (> 3 2) 10 20)"), 10);
});

test("eval: closures capture environment", () => {
  // ((let Y 10 (lambda X (+ X Y))) 5) = 15
  assertInt(run("((let Y 10 (lambda X (+ X Y))) 5)"), 15);
});

test("eval: if with non-bool condition throws", () => {
  assert.throws(() => run("(if 1 2 3)"), /condition must be Bool/);
});

// --- Tuples ---

test("eval: (fst (@p 1 2)) = 1", () => {
  assertInt(run("(fst (@p 1 2))"), 1);
});

test("eval: (snd (@p 1 2)) = 2", () => {
  assertInt(run("(snd (@p 1 2))"), 2);
});

test("eval: @p is a special form (not a primitive)", () => {
  // (@p (+ 1 2) (* 3 4)) should evaluate its children
  const v = run("(@p (+ 1 2) (* 3 4))");
  assert.equal(v.kind, "tuple");
  if (v.kind === "tuple") {
    assertInt(v.fst, 3);
    assertInt(v.snd, 12);
  }
});

// --- List primitives ---

test("eval: (cons 1 nil) = [1]", () => {
  assertIntList(run("(cons 1 nil)"), [1]);
});

test("eval: (cons 1 (cons 2 (cons 3 nil))) = [1 2 3]", () => {
  assertIntList(run("(cons 1 (cons 2 (cons 3 nil)))"), [1, 2, 3]);
});

test("eval: foldl (+) 0 [1..5] = 15", () => {
  const src =
    "(foldl (lambda Acc (lambda X (+ Acc X))) 0 (cons 1 (cons 2 (cons 3 (cons 4 (cons 5 nil))))))";
  assertInt(run(src), 15);
});

test("eval: foldr short-circuit-style over [1 -1 2]", () => {
  // foldr (\x acc -> if x >= 0 then acc else false) true [1 -1 2] = false
  const src =
    "(foldr (lambda X (lambda Acc (if (>= X 0) Acc false))) true (cons 1 (cons -1 (cons 2 nil))))";
  assertBool(run(src), false);
});

test("eval: map (* X X) over [1 2 3] = [1 4 9]", () => {
  const src = "(map (lambda X (* X X)) (cons 1 (cons 2 (cons 3 nil))))";
  assertIntList(run(src), [1, 4, 9]);
});

test("eval: filter (> X 0) over [-1 2 -3 4] = [2 4]", () => {
  const src =
    "(filter (lambda X (> X 0)) (cons -1 (cons 2 (cons -3 (cons 4 nil)))))";
  assertIntList(run(src), [2, 4]);
});

test("eval: scanl (+) 0 [1 2 3] = [0 1 3 6]", () => {
  const src =
    "(scanl (lambda Acc (lambda X (+ Acc X))) 0 (cons 1 (cons 2 (cons 3 nil))))";
  assertIntList(run(src), [0, 1, 3, 6]);
});

test("eval: compose: (compose f g x) = f (g x)", () => {
  // compose (\x -> + x 1) (\x -> * x 2) 3 = ((3*2)+1) = 7
  const src =
    "(compose (lambda X (+ X 1)) (lambda X (* X 2)) 3)";
  assertInt(run(src), 7);
});

test("eval: concat [[1 2] [3]] = [1 2 3]", () => {
  const src =
    "(concat (cons (cons 1 (cons 2 nil)) (cons (cons 3 nil) nil)))";
  assertIntList(run(src), [1, 2, 3]);
});

test("eval: unbound variable throws", () => {
  assert.throws(() => run("Foo"), /unbound variable/);
});

// --- Processable pattern: scanl-fusion integration test ---

test("eval: processable balance pattern (allowed)", () => {
  const src = `(snd (foldl
    (lambda State (lambda Tx
      (let Next (- (fst State) Tx)
        (@p Next (and (snd State) (>= Next 0))))))
    (@p 100 (>= 100 0))
    (cons 30 (cons 20 (cons 10 nil)))))`;
  assertBool(run(src), true);
});

test("eval: processable balance pattern (overspend)", () => {
  const src = `(snd (foldl
    (lambda State (lambda Tx
      (let Next (- (fst State) Tx)
        (@p Next (and (snd State) (>= Next 0))))))
    (@p 100 (>= 100 0))
    (cons 30 (cons 50 (cons 40 nil)))))`;
  assertBool(run(src), false);
});
