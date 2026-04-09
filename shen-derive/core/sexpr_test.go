package core

import (
	"testing"
)

func TestParseAtoms(t *testing.T) {
	tests := []struct {
		input string
		want  Sexpr
	}{
		{"42", Num(42)},
		{"-7", Num(-7)},
		{"3.14", Float(3.14)},
		{"true", Bool(true)},
		{"false", Bool(false)},
		{"foo", Sym("foo")},
		{"+", Sym("+")},
		{">=", Sym(">=")},
		{"?f", Sym("?f")},
		{`"hello"`, Str("hello")},
		{`"with\nnewline"`, Str("with\nnewline")},
	}
	for _, tt := range tests {
		got, err := ParseSexpr(tt.input)
		if err != nil {
			t.Errorf("ParseSexpr(%q) error: %v", tt.input, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("ParseSexpr(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseLists(t *testing.T) {
	tests := []struct {
		input string
		want  Sexpr
	}{
		{"(+ 1 2)", SList(Sym("+"), Num(1), Num(2))},
		{"(lambda X (+ X 1))", SList(Sym("lambda"), Sym("X"), SList(Sym("+"), Sym("X"), Num(1)))},
		{"()", SList()},
		{"(f)", SList(Sym("f"))},
		{"(f (g x))", SList(Sym("f"), SList(Sym("g"), Sym("x")))},
	}
	for _, tt := range tests {
		got, err := ParseSexpr(tt.input)
		if err != nil {
			t.Errorf("ParseSexpr(%q) error: %v", tt.input, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("ParseSexpr(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseSquareLists(t *testing.T) {
	// [1 2 3] desugars to (cons 1 (cons 2 (cons 3 nil)))
	got, err := ParseSexpr("[1 2 3]")
	if err != nil {
		t.Fatalf("ParseSexpr([1 2 3]) error: %v", err)
	}
	want := SList(Sym("cons"), Num(1),
		SList(Sym("cons"), Num(2),
			SList(Sym("cons"), Num(3), Sym("nil"))))
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// [a | b] desugars to (cons a b)
	got2, err := ParseSexpr("[a | b]")
	if err != nil {
		t.Fatalf("ParseSexpr([a | b]) error: %v", err)
	}
	want2 := SList(Sym("cons"), Sym("a"), Sym("b"))
	if !got2.Equal(want2) {
		t.Errorf("got %v, want %v", got2, want2)
	}

	// [] desugars to nil
	got3, err := ParseSexpr("[]")
	if err != nil {
		t.Fatalf("ParseSexpr([]) error: %v", err)
	}
	if !got3.Equal(Sym("nil")) {
		t.Errorf("[] should desugar to nil, got %v", got3)
	}
}

func TestParseComments(t *testing.T) {
	// Shen-style \\ comments
	got, err := ParseSexpr(`
		\\ this is a comment
		(+ 1 2)
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := SList(Sym("+"), Num(1), Num(2))
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Also handle -- comments
	got2, err := ParseSexpr(`
		-- this is a comment
		42
	`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !got2.Equal(Num(42)) {
		t.Errorf("got %v, want 42", got2)
	}
}

func TestParseAll(t *testing.T) {
	input := `
		(define sum {(list number) --> number}
		  Xs -> (foldl + 0 Xs))

		(derive sum
		  (rewrite foldl-map-fusion))
	`
	results, err := ParseAllSexprs(input)
	if err != nil {
		t.Fatalf("ParseAllSexprs error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 top-level forms, got %d", len(results))
	}
	if HeadSym(results[0]) != "define" {
		t.Errorf("first form should be (define ...), got head %q", HeadSym(results[0]))
	}
	if HeadSym(results[1]) != "derive" {
		t.Errorf("second form should be (derive ...), got head %q", HeadSym(results[1]))
	}
}

func TestPrettyPrint(t *testing.T) {
	tests := []struct {
		input Sexpr
		want  string
	}{
		{Num(42), "42"},
		{Sym("foo"), "foo"},
		{Str("hello"), `"hello"`},
		{SList(Sym("+"), Num(1), Num(2)), "(+ 1 2)"},
		{SList(Sym("lambda"), Sym("X"), SList(Sym("+"), Sym("X"), Num(1))),
			"(lambda X (+ X 1))"},
	}
	for _, tt := range tests {
		got := PrettyPrintSexpr(tt.input)
		if got != tt.want {
			t.Errorf("PrettyPrintSexpr(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPrettyPrintConsList(t *testing.T) {
	// (cons 1 (cons 2 (cons 3 nil))) should print as [1 2 3]
	expr := SList(Sym("cons"), Num(1),
		SList(Sym("cons"), Num(2),
			SList(Sym("cons"), Num(3), Sym("nil"))))
	got := PrettyPrintSexpr(expr)
	if got != "[1 2 3]" {
		t.Errorf("got %q, want [1 2 3]", got)
	}

	// (cons a b) should print as [a | b]
	expr2 := SList(Sym("cons"), Sym("a"), Sym("b"))
	got2 := PrettyPrintSexpr(expr2)
	if got2 != "[a | b]" {
		t.Errorf("got %q, want [a | b]", got2)
	}
}

func TestRoundTrip(t *testing.T) {
	inputs := []string{
		"42",
		"(+ 1 2)",
		"(lambda X (+ X 1))",
		"(foldl (lambda X (lambda Acc (+ X Acc))) 0 Xs)",
		"(compose (map ?f) (map ?g))",
		`"hello world"`,
		"true",
		"false",
	}
	for _, input := range inputs {
		parsed, err := ParseSexpr(input)
		if err != nil {
			t.Errorf("ParseSexpr(%q) error: %v", input, err)
			continue
		}
		printed := PrettyPrintSexpr(parsed)
		reparsed, err := ParseSexpr(printed)
		if err != nil {
			t.Errorf("re-parse of %q failed: %v", printed, err)
			continue
		}
		if !parsed.Equal(reparsed) {
			t.Errorf("round-trip failed: %q -> %q -> %v", input, printed, reparsed)
		}
	}
}

func TestSquareListRoundTrip(t *testing.T) {
	// Parse [1 2 3], print it, re-parse — should be equal
	parsed, err := ParseSexpr("[1 2 3]")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	printed := PrettyPrintSexpr(parsed)
	if printed != "[1 2 3]" {
		t.Fatalf("expected [1 2 3], got %q", printed)
	}
	reparsed, err := ParseSexpr(printed)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	if !parsed.Equal(reparsed) {
		t.Errorf("round-trip failed")
	}
}

func TestDeepCopy(t *testing.T) {
	orig := SList(Sym("lambda"), Sym("X"), SList(Sym("+"), Sym("X"), Num(1)))
	cp := DeepCopy(orig)
	if !orig.Equal(cp) {
		t.Errorf("copy should equal original")
	}
	// Mutate copy, verify original unchanged
	cpList := cp.(*List)
	cpList.Elems[1] = Sym("Y")
	if orig.Elems[1].(*Atom).Val != "X" {
		t.Errorf("mutation of copy affected original")
	}
}

func TestIsMetaVar(t *testing.T) {
	name, ok := IsMetaVar(Sym("?f"))
	if !ok || name != "?f" {
		t.Errorf("expected ?f to be a metavar")
	}

	_, ok = IsMetaVar(Sym("f"))
	if ok {
		t.Errorf("f should not be a metavar")
	}

	_, ok = IsMetaVar(Num(42))
	if ok {
		t.Errorf("42 should not be a metavar")
	}
}

func TestHeadSym(t *testing.T) {
	if HeadSym(SList(Sym("foldl"), Num(1))) != "foldl" {
		t.Error("expected foldl")
	}
	if HeadSym(Num(42)) != "" {
		t.Error("expected empty for non-list")
	}
	if HeadSym(SList()) != "" {
		t.Error("expected empty for empty list")
	}
}
