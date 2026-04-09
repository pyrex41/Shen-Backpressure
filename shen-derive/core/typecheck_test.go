package core

import (
	"strings"
	"testing"
)

// --- Successful type checking ---

func TestCheckLiterals(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"42", "Int"},
		{"True", "Bool"},
		{`"hello"`, "String"},
	}
	for _, tt := range tests {
		term, _ := Parse(tt.input)
		ty, err := CheckTerm(term)
		if err != nil {
			t.Errorf("CheckTerm(%q): %v", tt.input, err)
			continue
		}
		if ty.String() != tt.want {
			t.Errorf("CheckTerm(%q) = %s, want %s", tt.input, ty, tt.want)
		}
	}
}

func TestCheckArithmetic(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"3 + 4", "Int"},
		{"3 * 4 + 1", "Int"},
		{"10 - 3", "Int"},
		{"negate 5", "Int"},
	}
	for _, tt := range tests {
		term, _ := Parse(tt.input)
		ty, err := CheckTerm(term)
		if err != nil {
			t.Errorf("CheckTerm(%q): %v", tt.input, err)
			continue
		}
		if ty.String() != tt.want {
			t.Errorf("CheckTerm(%q) = %s, want %s", tt.input, ty, tt.want)
		}
	}
}

func TestCheckComparison(t *testing.T) {
	term, _ := Parse("3 < 4")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Bool" {
		t.Errorf("got %s, want Bool", ty)
	}
}

func TestCheckAnnotatedLambda(t *testing.T) {
	term, _ := Parse(`\(x : Int) -> x + 1`)
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Int -> Int" {
		t.Errorf("got %s, want Int -> Int", ty)
	}
}

func TestCheckMultiParamLambda(t *testing.T) {
	term, _ := Parse(`\(x : Int) (y : Int) -> x + y`)
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Int -> Int -> Int" {
		t.Errorf("got %s, want Int -> Int -> Int", ty)
	}
}

func TestCheckList(t *testing.T) {
	term, _ := Parse("[1, 2, 3]")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "[Int]" {
		t.Errorf("got %s, want [Int]", ty)
	}
}

func TestCheckEmptyList(t *testing.T) {
	// Empty list should get a polymorphic type [a]
	term, _ := Parse("[]")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	// Should be a list of some type variable
	if _, ok := ty.(*TList); !ok {
		t.Errorf("got %s (%T), want [a]", ty, ty)
	}
}

func TestCheckTuple(t *testing.T) {
	term, _ := Parse("(1, True)")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "(Int, Bool)" {
		t.Errorf("got %s, want (Int, Bool)", ty)
	}
}

func TestCheckFstSnd(t *testing.T) {
	term, _ := Parse("fst (1, True)")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Int" {
		t.Errorf("fst: got %s, want Int", ty)
	}

	term2, _ := Parse("snd (1, True)")
	ty2, err := CheckTerm(term2)
	if err != nil {
		t.Fatalf("CheckTerm snd: %v", err)
	}
	if ty2.String() != "Bool" {
		t.Errorf("snd: got %s, want Bool", ty2)
	}
}

func TestCheckIf(t *testing.T) {
	term, _ := Parse("if True then 1 else 2")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Int" {
		t.Errorf("got %s, want Int", ty)
	}
}

func TestCheckLet(t *testing.T) {
	term, _ := Parse("let x = 5 in x + 1")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Int" {
		t.Errorf("got %s, want Int", ty)
	}
}

// --- Polymorphic primitives ---

func TestCheckMapType(t *testing.T) {
	term, _ := Parse("map (\\(x : Int) -> x > 0) [1, 2, 3]")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "[Bool]" {
		t.Errorf("got %s, want [Bool]", ty)
	}
}

func TestCheckFoldrType(t *testing.T) {
	term, _ := Parse("foldr (+) 0 [1, 2, 3]")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Int" {
		t.Errorf("got %s, want Int", ty)
	}
}

func TestCheckFilterType(t *testing.T) {
	term, _ := Parse("filter (\\(x : Int) -> x > 0) [1, 2, 3]")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "[Int]" {
		t.Errorf("got %s, want [Int]", ty)
	}
}

func TestCheckConsType(t *testing.T) {
	term, _ := Parse("cons 1 [2, 3]")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "[Int]" {
		t.Errorf("got %s, want [Int]", ty)
	}
}

func TestCheckScanlType(t *testing.T) {
	term, _ := Parse("scanl (+) 0 [1, 2, 3]")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "[Int]" {
		t.Errorf("got %s, want [Int]", ty)
	}
}

func TestCheckComposeType(t *testing.T) {
	// not . (>= 0)  should be Int -> Bool
	// Actually: compose not (\x -> x >= 0) is (Int -> Bool)
	// But compose in our system has arity 3, so compose not (\x -> x >= 0)
	// is still waiting for the third arg. Type: Int -> Bool
	term, _ := Parse("let geq0 = \\(x : Int) -> x >= 0 in (not . geq0) 5")
	ty, err := CheckTerm(term)
	if err != nil {
		t.Fatalf("CheckTerm: %v", err)
	}
	if ty.String() != "Bool" {
		t.Errorf("got %s, want Bool", ty)
	}
}

// --- Type errors ---

func TestCheckTypeMismatch(t *testing.T) {
	// Adding a bool to an int should fail
	term, _ := Parse("1 + True")
	_, err := CheckTerm(term)
	if err == nil {
		t.Error("expected type error for 1 + True")
	}
}

func TestCheckIfBranchMismatch(t *testing.T) {
	term, _ := Parse("if True then 1 else False")
	_, err := CheckTerm(term)
	if err == nil {
		t.Error("expected type error for if with mismatched branches")
	}
}

func TestCheckIfCondNotBool(t *testing.T) {
	term, _ := Parse("if 1 then 2 else 3")
	_, err := CheckTerm(term)
	if err == nil {
		t.Error("expected type error for non-Bool condition")
	}
}

func TestCheckListMixedTypes(t *testing.T) {
	// [1, True] should fail — mixed types in list
	term := &ListLit{Elems: []Term{MkInt(1), MkBool(true)}}
	_, err := CheckTerm(term)
	if err == nil {
		t.Error("expected type error for mixed-type list")
	}
}

func TestCheckUnboundVariable(t *testing.T) {
	term, _ := Parse("x + 1")
	_, err := CheckTerm(term)
	if err == nil {
		t.Error("expected error for unbound variable")
	}
	if !strings.Contains(err.Error(), "unbound") {
		t.Errorf("expected 'unbound' in error, got: %v", err)
	}
}

func TestCheckApplyNonFunction(t *testing.T) {
	// 5 3 — applying an Int to an Int
	term := MkApp(MkInt(5), MkInt(3))
	_, err := CheckTerm(term)
	if err == nil {
		t.Error("expected type error for applying non-function")
	}
}
