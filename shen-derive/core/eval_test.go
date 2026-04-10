package core

import (
	"testing"
)

func mustEval(t *testing.T, input string) Value {
	t.Helper()
	sexpr, err := ParseSexpr(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	v, err := Eval(EmptyEnv(), sexpr)
	if err != nil {
		t.Fatalf("eval %q: %v", input, err)
	}
	return v
}

func TestEvalArithmetic(t *testing.T) {
	v := mustEval(t, "(+ 1 2)")
	if v.String() != "3" {
		t.Fatalf("got %s, want 3", v)
	}

	v = mustEval(t, "(* (+ 2 3) (- 10 4))")
	if v.String() != "30" {
		t.Fatalf("got %s, want 30", v)
	}
}

func TestEvalFloatArithmetic(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"(+ 0.5 0.5)", "1"},
		{"(+ 1 0.5)", "1.5"},
		{"(- 1.5 0.5)", "1"},
		{"(* 2 2.5)", "5"},
		{"(/ 10 4)", "2"},       // int/int → int (2)
		{"(/ 10.0 4)", "2.5"},   // float promoted
		{"(>= 0.5 0)", "true"},
		{"(< 0.1 0.2)", "true"},
		{"(= 1 1.0)", "true"},
		{"(!= 1.0 2)", "true"},
	}
	for _, tc := range cases {
		v := mustEval(t, tc.in)
		if v.String() != tc.want {
			t.Errorf("%s = %s, want %s", tc.in, v.String(), tc.want)
		}
	}
}

func TestEvalBoolean(t *testing.T) {
	v := mustEval(t, "(and true (>= 5 3))")
	if v.String() != "true" {
		t.Fatalf("got %s, want true", v)
	}

	v = mustEval(t, "(not (< 5 3))")
	if v.String() != "true" {
		t.Fatalf("got %s, want true", v)
	}
}

func TestEvalLambda(t *testing.T) {
	v := mustEval(t, "((lambda X (+ X 1)) 5)")
	if v.String() != "6" {
		t.Fatalf("got %s, want 6", v)
	}
}

func TestEvalLet(t *testing.T) {
	v := mustEval(t, "(let Y (+ 3 4) (* Y 2))")
	if v.String() != "14" {
		t.Fatalf("got %s, want 14", v)
	}
}

func TestEvalIf(t *testing.T) {
	v := mustEval(t, "(if (> 3 2) 10 20)")
	if v.String() != "10" {
		t.Fatalf("got %s, want 10", v)
	}
}

func TestEvalTuple(t *testing.T) {
	v := mustEval(t, "(fst (@p 1 2))")
	if v.String() != "1" {
		t.Fatalf("got %s, want 1", v)
	}

	v = mustEval(t, "(snd (@p 1 2))")
	if v.String() != "2" {
		t.Fatalf("got %s, want 2", v)
	}
}

func TestEvalFoldl(t *testing.T) {
	// foldl (+) 0 [1 2 3 4 5] = 15
	v := mustEval(t, "(foldl (lambda Acc (lambda X (+ Acc X))) 0 (cons 1 (cons 2 (cons 3 (cons 4 (cons 5 nil))))))")
	if v.String() != "15" {
		t.Fatalf("got %s, want 15", v)
	}
}

func TestEvalFoldr(t *testing.T) {
	// foldr (\x acc -> if x >= 0 then acc else false) true [1 -1 2]
	v := mustEval(t, "(foldr (lambda X (lambda Acc (if (>= X 0) Acc false))) true (cons 1 (cons -1 (cons 2 nil))))")
	if v.String() != "false" {
		t.Fatalf("got %s, want false", v)
	}
}

func TestEvalMap(t *testing.T) {
	// map (\x -> x * x) [1 2 3]
	v := mustEval(t, "(map (lambda X (* X X)) (cons 1 (cons 2 (cons 3 nil))))")
	if v.String() != "[1, 4, 9]" {
		t.Fatalf("got %s, want [1, 4, 9]", v)
	}
}

func TestEvalFilter(t *testing.T) {
	// filter (\x -> x > 0) [-1 2 -3 4]
	v := mustEval(t, "(filter (lambda X (> X 0)) (cons -1 (cons 2 (cons -3 (cons 4 nil)))))")
	if v.String() != "[2, 4]" {
		t.Fatalf("got %s, want [2, 4]", v)
	}
}

func TestEvalScanl(t *testing.T) {
	// scanl (+) 0 [1 2 3] = [0 1 3 6]
	v := mustEval(t, "(scanl (lambda Acc (lambda X (+ Acc X))) 0 (cons 1 (cons 2 (cons 3 nil))))")
	if v.String() != "[0, 1, 3, 6]" {
		t.Fatalf("got %s, want [0, 1, 3, 6]", v)
	}
}

func TestEvalNegativeLiteral(t *testing.T) {
	v := mustEval(t, "(- 0 5)")
	if v.String() != "-5" {
		t.Fatalf("got %s, want -5", v)
	}
}

func TestEvalProcessablePattern(t *testing.T) {
	// The all-scanl-fusion integration test pattern:
	// snd(foldl step (@p b0 (>= b0 0)) txs)
	// where step = \state tx -> let next = (- (fst state) tx) in (@p next (and (snd state) (>= next 0)))
	expr := `(snd (foldl
		(lambda State (lambda Tx
			(let Next (- (fst State) Tx)
				(@p Next (and (snd State) (>= Next 0))))))
		(@p 100 (>= 100 0))
		(cons 30 (cons 20 (cons 10 nil)))))`
	v := mustEval(t, expr)
	if v.String() != "true" {
		t.Fatalf("got %s, want true", v)
	}

	// Overspend case
	expr2 := `(snd (foldl
		(lambda State (lambda Tx
			(let Next (- (fst State) Tx)
				(@p Next (and (snd State) (>= Next 0))))))
		(@p 100 (>= 100 0))
		(cons 30 (cons 50 (cons 40 nil)))))`
	v2 := mustEval(t, expr2)
	if v2.String() != "false" {
		t.Fatalf("got %s, want false", v2)
	}
}
