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

test("eval: shen.mod aliases modulo", () => {
  assertInt(run("(shen.mod 10 3)"), 1);
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

// --- String primitives (mirror shengen-ts's translateDefineExpr) ---

test("eval: length on string", () => {
  assertInt(run('(length "hello")'), 5);
});

test("eval: length on empty string is 0", () => {
  assertInt(run('(length "")'), 0);
});

test("eval: length on list", () => {
  assertInt(run("(length (cons 1 (cons 2 (cons 3 nil))))"), 3);
});

test("eval: trim removes leading and trailing whitespace", () => {
  const v = run('(trim "  hello  ")');
  assert.equal(v.kind, "string");
  if (v.kind === "string") assert.equal(v.val, "hello");
});

test("eval: trim of all-whitespace yields empty", () => {
  const v = run('(trim "   ")');
  assert.equal(v.kind, "string");
  if (v.kind === "string") assert.equal(v.val, "");
});

test("eval: head-char of non-empty string", () => {
  const v = run('(head-char "abc")');
  assert.equal(v.kind, "string");
  if (v.kind === "string") assert.equal(v.val, "a");
});

test("eval: head-char of empty string is empty", () => {
  const v = run('(head-char "")');
  assert.equal(v.kind, "string");
  if (v.kind === "string") assert.equal(v.val, "");
});

test("eval: tail-chars drops first character", () => {
  const v = run('(tail-chars "abc")');
  assert.equal(v.kind, "string");
  if (v.kind === "string") assert.equal(v.val, "bc");
});

test("eval: tail-chars of single-char string is empty", () => {
  const v = run('(tail-chars "x")');
  assert.equal(v.kind, "string");
  if (v.kind === "string") assert.equal(v.val, "");
});

test("eval: in-range? accepts in-range char", () => {
  assertBool(run('(in-range? "M" "A" "Z")'), true);
});

test("eval: in-range? rejects out-of-range char", () => {
  assertBool(run('(in-range? "a" "A" "Z")'), false);
});

test("eval: in-range? boundary lo is inclusive", () => {
  assertBool(run('(in-range? "A" "A" "Z")'), true);
});

test("eval: in-range? boundary hi is inclusive", () => {
  assertBool(run('(in-range? "Z" "A" "Z")'), true);
});

test("eval: element? finds member in string list", () => {
  assertBool(run('(element? "b" (cons "a" (cons "b" (cons "c" nil))))'), true);
});

test("eval: element? rejects non-member", () => {
  assertBool(run('(element? "z" (cons "a" (cons "b" nil)))'), false);
});

test("eval: element? on empty list is false", () => {
  assertBool(run('(element? "x" nil)'), false);
});

// --- non-blank? round-trip — composes length + trim + > ---

test("eval: non-blank? define body accepts non-blank string", () => {
  // Mirrors event-schema.shen: X -> (> (length (trim X)) 0)
  assertBool(run('(> (length (trim "  hello  ")) 0)'), true);
});

test("eval: non-blank? define body rejects whitespace-only", () => {
  assertBool(run('(> (length (trim "   ")) 0)'), false);
});

test("eval: non-blank? define body rejects empty string", () => {
  assertBool(run('(> (length (trim "")) 0)'), false);
});

// --- base64url-char? define body — composes in-range? + or + = ---

test("eval: base64url-char? accepts uppercase letter", () => {
  const src = `(or (in-range? "M" "A" "Z")
                  (or (in-range? "M" "a" "z")
                      (or (in-range? "M" "0" "9")
                          (or (= "M" "-") (= "M" "_")))))`;
  assertBool(run(src), true);
});

test("eval: base64url-char? rejects punctuation", () => {
  const src = `(or (in-range? "!" "A" "Z")
                  (or (in-range? "!" "a" "z")
                      (or (in-range? "!" "0" "9")
                          (or (= "!" "-") (= "!" "_")))))`;
  assertBool(run(src), false);
});
