package core

import (
	"testing"
)

// helper: parse, eval in empty env, return value
func evalString(t *testing.T, input string) Value {
	t.Helper()
	term, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := Eval(EmptyEnv(), term)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	return val
}

func expectInt(t *testing.T, input string, want int64) {
	t.Helper()
	v := evalString(t, input)
	iv, ok := v.(IntVal)
	if !ok {
		t.Fatalf("expected IntVal, got %T (%s)", v, v)
	}
	if int64(iv) != want {
		t.Errorf("got %d, want %d", iv, want)
	}
}

func expectBool(t *testing.T, input string, want bool) {
	t.Helper()
	v := evalString(t, input)
	bv, ok := v.(BoolVal)
	if !ok {
		t.Fatalf("expected BoolVal, got %T (%s)", v, v)
	}
	if bool(bv) != want {
		t.Errorf("got %v, want %v", bv, want)
	}
}

func expectStr(t *testing.T, input string, want string) {
	t.Helper()
	v := evalString(t, input)
	sv, ok := v.(StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T (%s)", v, v)
	}
	if string(sv) != want {
		t.Errorf("got %q, want %q", sv, want)
	}
}

func expectList(t *testing.T, input string, want string) {
	t.Helper()
	v := evalString(t, input)
	got := v.String()
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- Tests covering each primitive ---

func TestEvalLiterals(t *testing.T) {
	expectInt(t, "42", 42)
	expectBool(t, "True", true)
	expectBool(t, "False", false)
	expectStr(t, `"hello"`, "hello")
}

func TestEvalArithmetic(t *testing.T) {
	expectInt(t, "3 + 4", 7)
	expectInt(t, "10 - 3", 7)
	expectInt(t, "6 * 7", 42)
	expectInt(t, "17 / 3", 5)
	expectInt(t, "17 % 3", 2)
	expectInt(t, "-5", -5)
	expectInt(t, "2 + 3 * 4", 14) // precedence: 2 + (3*4)
}

func TestEvalComparison(t *testing.T) {
	expectBool(t, "3 == 3", true)
	expectBool(t, "3 == 4", false)
	expectBool(t, "3 /= 4", true)
	expectBool(t, "3 < 4", true)
	expectBool(t, "4 < 3", false)
	expectBool(t, "3 <= 3", true)
	expectBool(t, "4 > 3", true)
	expectBool(t, "3 >= 4", false)
}

func TestEvalBoolean(t *testing.T) {
	expectBool(t, "True && False", false)
	expectBool(t, "True && True", true)
	expectBool(t, "True || False", true)
	expectBool(t, "False || False", false)
	expectBool(t, "not True", false)
	expectBool(t, "not False", true)
}

func TestEvalList(t *testing.T) {
	expectList(t, "[]", "[]")
	expectList(t, "[1, 2, 3]", "[1, 2, 3]")
	expectList(t, "cons 1 [2, 3]", "[1, 2, 3]")
	expectList(t, "cons 0 []", "[0]")
}

func TestEvalTuple(t *testing.T) {
	v := evalString(t, "(1, True)")
	tv, ok := v.(*TupleVal)
	if !ok {
		t.Fatalf("expected TupleVal, got %T", v)
	}
	if int64(tv.Fst.(IntVal)) != 1 {
		t.Errorf("fst: got %v, want 1", tv.Fst)
	}
	if bool(tv.Snd.(BoolVal)) != true {
		t.Errorf("snd: got %v, want True", tv.Snd)
	}
}

func TestEvalFstSnd(t *testing.T) {
	expectInt(t, "fst (1, 2)", 1)
	expectInt(t, "snd (1, 2)", 2)
}

func TestEvalIf(t *testing.T) {
	expectInt(t, "if True then 1 else 2", 1)
	expectInt(t, "if False then 1 else 2", 2)
	expectInt(t, "if 3 > 2 then 10 else 20", 10)
}

func TestEvalLambda(t *testing.T) {
	expectInt(t, "(\\x -> x + 1) 5", 6)
	expectInt(t, "(\\x y -> x + y) 3 4", 7)
}

func TestEvalLet(t *testing.T) {
	expectInt(t, "let x = 5 in x + 1", 6)
	expectInt(t, "let double = \\x -> x * 2 in double 21", 42)
}

func TestEvalMap(t *testing.T) {
	expectList(t, "map (\\x -> x * 2) [1, 2, 3]", "[2, 4, 6]")
	expectList(t, "map (\\x -> x + 1) []", "[]")
}

func TestEvalFoldr(t *testing.T) {
	// foldr (+) 0 [1,2,3,4,5] = 15
	expectInt(t, "foldr (+) 0 [1, 2, 3, 4, 5]", 15)
	// foldr cons [] [1,2,3] = [1,2,3]
	expectList(t, "foldr cons [] [1, 2, 3]", "[1, 2, 3]")
	// foldr (*) 1 [1,2,3,4] = 24
	expectInt(t, "foldr (*) 1 [1, 2, 3, 4]", 24)
}

func TestEvalFoldl(t *testing.T) {
	// foldl (+) 0 [1,2,3] = 6
	expectInt(t, "foldl (+) 0 [1, 2, 3]", 6)
	// foldl (-) 10 [1, 2, 3] = ((10-1)-2)-3 = 4
	expectInt(t, "foldl (-) 10 [1, 2, 3]", 4)
}

func TestEvalScanl(t *testing.T) {
	// scanl (+) 0 [1,2,3] = [0, 1, 3, 6]
	expectList(t, "scanl (+) 0 [1, 2, 3]", "[0, 1, 3, 6]")
}

func TestEvalFilter(t *testing.T) {
	expectList(t, "filter (\\x -> x > 2) [1, 2, 3, 4, 5]", "[3, 4, 5]")
	expectList(t, "filter (\\x -> x == 0) [1, 2, 3]", "[]")
}

func TestEvalConcat(t *testing.T) {
	expectList(t, "concat [[1, 2], [3, 4], [5]]", "[1, 2, 3, 4, 5]")
	expectList(t, "concat [[], [1], []]", "[1]")
}

func TestEvalCompose(t *testing.T) {
	// (f . g) x = f (g x)
	// compose (\x -> x + 1) (\x -> x * 2) 3 = (3*2)+1 = 7
	expectInt(t, "let f = \\x -> x + 1 in let g = \\x -> x * 2 in (f . g) 3", 7)
}

func TestEvalComposeChain(t *testing.T) {
	// (f . g . h) x = f(g(h(x)))
	expectInt(t, "let f = \\x -> x + 1 in let g = \\x -> x * 2 in let h = \\x -> x + 10 in (f . g . h) 3", 27)
	// (3+10)*2 + 1 = 27
}

func TestEvalHigherOrder(t *testing.T) {
	// map (foldr (+) 0) [[1,2], [3,4], [5]]
	expectList(t, "map (foldr (+) 0) [[1, 2], [3, 4], [5]]", "[3, 7, 5]")
}

func TestEvalNestedLet(t *testing.T) {
	expectInt(t, "let x = 1 in let y = 2 in x + y", 3)
}

func TestEvalPartialApplication(t *testing.T) {
	// map (+1) is map partially applied to (+1)
	// We need to write this differently since (+1) isn't valid syntax
	expectList(t, "let inc = \\x -> x + 1 in map inc [1, 2, 3]", "[2, 3, 4]")
}

func TestEvalUnfoldr(t *testing.T) {
	// unfoldr (\n -> (n >= 5, (n, n+1))) 0 should give [0, 1, 2, 3, 4]
	// The function returns (done?, (value, nextSeed))
	// When done? is False, stop (wait, our convention is: True = continue)
	// Let me recheck: in our ExecPrim, True means continue, False means stop
	// Actually, looking at the code: if !asBool(tp.Fst) { break }
	// So True = continue, False = stop
	expectList(t,
		`unfoldr (\n -> if n < 5 then (True, (n, n + 1)) else (False, (0, 0))) 0`,
		"[0, 1, 2, 3, 4]")
}
