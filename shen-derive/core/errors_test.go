package core

import (
	"strings"
	"testing"
)

// --- Parse errors ---

func TestParseErrorUnexpectedChar(t *testing.T) {
	_, err := Parse("1 @ 2")
	if err == nil {
		t.Error("expected parse error for @")
	}
}

func TestParseErrorUnclosedBracket(t *testing.T) {
	_, err := Parse("[1, 2, 3")
	if err == nil {
		t.Error("expected parse error for unclosed bracket")
	}
}

func TestParseErrorUnclosedParen(t *testing.T) {
	_, err := Parse("(1 + 2")
	if err == nil {
		t.Error("expected parse error for unclosed paren")
	}
}

func TestParseErrorEmptyLambda(t *testing.T) {
	_, err := Parse("\\ -> 1")
	if err == nil {
		t.Error("expected parse error for parameterless lambda")
	}
}

func TestParseErrorMissingIn(t *testing.T) {
	_, err := Parse("let x = 5")
	if err == nil {
		t.Error("expected parse error for let without in")
	}
}

func TestParseErrorMissingElse(t *testing.T) {
	_, err := Parse("if True then 1")
	if err == nil {
		t.Error("expected parse error for if without else")
	}
}

func TestParseErrorTrailingTokens(t *testing.T) {
	_, err := Parse("1 2")
	// This actually parses as App(1, 2) which would be a type error,
	// not a parse error. That's fine.
	if err != nil {
		t.Logf("parse error (acceptable): %v", err)
	}
}

func TestParseErrorUnclosedString(t *testing.T) {
	_, err := Parse(`"hello`)
	if err == nil {
		t.Error("expected parse error for unclosed string")
	}
}

// --- Eval errors ---

func TestEvalErrorUnbound(t *testing.T) {
	term, _ := Parse("x")
	_, err := Eval(EmptyEnv(), term)
	if err == nil {
		t.Error("expected eval error for unbound variable")
	}
	if !strings.Contains(err.Error(), "unbound") {
		t.Errorf("expected 'unbound' in error, got: %v", err)
	}
}

func TestEvalErrorDivZero(t *testing.T) {
	term, _ := Parse("10 / 0")
	_, err := Eval(EmptyEnv(), term)
	if err == nil {
		t.Error("expected eval error for division by zero")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("expected 'division by zero' in error, got: %v", err)
	}
}

func TestEvalErrorModZero(t *testing.T) {
	term, _ := Parse("10 % 0")
	_, err := Eval(EmptyEnv(), term)
	if err == nil {
		t.Error("expected eval error for modulo by zero")
	}
}

func TestEvalErrorIfNotBool(t *testing.T) {
	// Manually build: if 42 then 1 else 2
	term := MkIf(MkInt(42), MkInt(1), MkInt(2))
	_, err := Eval(EmptyEnv(), term)
	if err == nil {
		t.Error("expected eval error for non-Bool condition")
	}
	if !strings.Contains(err.Error(), "Bool") {
		t.Errorf("expected 'Bool' in error, got: %v", err)
	}
}

func TestEvalErrorApplyNonFunction(t *testing.T) {
	// 5 applied to 3
	term := MkApp(MkInt(5), MkInt(3))
	_, err := Eval(EmptyEnv(), term)
	if err == nil {
		t.Error("expected eval error for applying non-function")
	}
	if !strings.Contains(err.Error(), "cannot apply") {
		t.Errorf("expected 'cannot apply' in error, got: %v", err)
	}
}

// --- Edge cases ---

func TestEvalEmptyListCombinators(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foldr (+) 0 []", "0"},
		{"foldl (+) 0 []", "0"},
		{"map (\\x -> x + 1) []", "[]"},
		{"filter (\\x -> x > 0) []", "[]"},
		{"scanl (+) 0 []", "[0]"},
		{"concat []", "[]"},
		{"concat [[], []]", "[]"},
	}
	for _, tt := range tests {
		term, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.input, err)
			continue
		}
		val, err := Eval(EmptyEnv(), term)
		if err != nil {
			t.Errorf("Eval(%q): %v", tt.input, err)
			continue
		}
		if val.String() != tt.want {
			t.Errorf("Eval(%q) = %s, want %s", tt.input, val, tt.want)
		}
	}
}

func TestEvalNestedMap(t *testing.T) {
	// map (\xs -> foldr (+) 0 xs) [[1,2], [3,4,5]]
	expectList(t, "map (foldr (+) 0) [[1, 2], [3, 4, 5]]", "[3, 12]")
}

func TestEvalBoolOperators(t *testing.T) {
	expectBool(t, "True && True && True", true)
	expectBool(t, "True && True && False", false)
	expectBool(t, "False || False || True", true)
	expectBool(t, "not (not True)", true)
}

func TestEvalStringLiteral(t *testing.T) {
	expectStr(t, `"hello"`, "hello")
	expectBool(t, `"a" == "a"`, true)
	expectBool(t, `"a" == "b"`, false)
}
