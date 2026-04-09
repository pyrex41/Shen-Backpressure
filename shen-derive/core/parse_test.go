package core

import (
	"testing"
)

func TestParseLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"42", "42"},
		{"True", "True"},
		{"False", "False"},
		{`"hello"`, `"hello"`},
	}
	for _, tt := range tests {
		term, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.input, err)
			continue
		}
		got := PrettyPrint(term)
		if got != tt.want {
			t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseList(t *testing.T) {
	term, err := Parse("[1, 2, 3]")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "[1, 2, 3]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseTuple(t *testing.T) {
	term, err := Parse("(1, True)")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "(1, True)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseLambda(t *testing.T) {
	term, err := Parse(`\x -> x + 1`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := `\x -> x + 1`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseMultiParamLambda(t *testing.T) {
	term, err := Parse(`\x y -> x + y`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := `\x y -> x + y`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseAnnotatedLambda(t *testing.T) {
	term, err := Parse(`\(x : Int) -> x + 1`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := `\(x : Int) -> x + 1`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseLet(t *testing.T) {
	term, err := Parse("let x = 5 in x + 1")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "let x = 5 in x + 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseIf(t *testing.T) {
	term, err := Parse("if True then 1 else 2")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "if True then 1 else 2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseOperatorSection(t *testing.T) {
	term, err := Parse("(+)")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	if got != "(+)" {
		t.Errorf("got %q, want %q", got, "(+)")
	}
}

func TestParseFoldr(t *testing.T) {
	term, err := Parse("foldr (+) 0 [1, 2, 3, 4, 5]")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "foldr (+) 0 [1, 2, 3, 4, 5]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseCompose(t *testing.T) {
	term, err := Parse("f . g")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "f . g"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseComposeRightAssoc(t *testing.T) {
	term, err := Parse("f . g . h")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "f . g . h"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParsePrecedence(t *testing.T) {
	// 2 + 3 * 4 should be 2 + (3 * 4), pretty-printed as "2 + 3 * 4"
	term, err := Parse("2 + 3 * 4")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "2 + 3 * 4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseNegation(t *testing.T) {
	term, err := Parse("-5")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "(-5)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseApplication(t *testing.T) {
	term, err := Parse("f x y")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := "f x y"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseComplexExpr(t *testing.T) {
	input := `let double = \x -> x * 2 in map double [1, 2, 3]`
	term, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := PrettyPrint(term)
	want := `let double = \x -> x * 2 in map double [1, 2, 3]`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseRoundtrip(t *testing.T) {
	inputs := []string{
		"42",
		"True",
		"[1, 2, 3]",
		"(1, 2)",
		`\x -> x + 1`,
		"let x = 5 in x",
		"if True then 1 else 2",
		"foldr (+) 0 [1, 2, 3]",
		"map (\\x -> x * 2) [1, 2, 3]",
		"f . g",
	}
	for _, input := range inputs {
		term, err := Parse(input)
		if err != nil {
			t.Errorf("Parse(%q): %v", input, err)
			continue
		}
		printed := PrettyPrint(term)
		term2, err := Parse(printed)
		if err != nil {
			t.Errorf("re-Parse(%q) [from %q]: %v", printed, input, err)
			continue
		}
		printed2 := PrettyPrint(term2)
		if printed != printed2 {
			t.Errorf("roundtrip mismatch: %q -> %q -> %q", input, printed, printed2)
		}
	}
}

func TestParseLineComment(t *testing.T) {
	term, err := Parse("-- this is a comment\n42")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	v, err := Eval(EmptyEnv(), term)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if int64(v.(IntVal)) != 42 {
		t.Errorf("got %v, want 42", v)
	}
}
