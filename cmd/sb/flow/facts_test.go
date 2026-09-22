package flow

import (
	"strings"
	"testing"
)

func sampleFacts() *FactSet {
	fs := &FactSet{
		Indexer:  "scip-go",
		Language: "go",
		Defs: []Def{
			{Symbol: "pkg/F().", File: "a.go", StartLine: 10, StartCol: 0, EndLine: 20, EndCol: 1},
		},
		Refs: []Ref{
			{Symbol: "pkg/G().", File: "a.go", Line: 12, Col: 4, Enclosing: "pkg/F()."},
			{Symbol: "pkg/H().", File: "a.go", Line: 13, Col: 4, Enclosing: ""},
		},
		Calls: []Call{{Caller: "pkg/F().", Callee: "pkg/G()."}},
	}
	fs.Index()
	return fs
}

func TestWriteParseFactsRoundTrip(t *testing.T) {
	want := sampleFacts()
	got, err := ParseFacts(want.WriteFacts())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Defs) != 1 || got.Defs[0] != want.Defs[0] {
		t.Errorf("defs = %+v, want %+v", got.Defs, want.Defs)
	}
	if len(got.Refs) != 2 || got.Refs[0] != want.Refs[0] || got.Refs[1] != want.Refs[1] {
		t.Errorf("refs = %+v, want %+v", got.Refs, want.Refs)
	}
	if len(got.Calls) != 1 || got.Calls[0] != want.Calls[0] {
		t.Errorf("calls = %+v, want %+v", got.Calls, want.Calls)
	}
}

// TestWriteFactsHeaderIsAShenComment matters because the fact file is
// loaded straight into a Shen host: anything outside a form has to be
// inside `\* … *\`.
func TestWriteFactsHeaderIsAShenComment(t *testing.T) {
	src := sampleFacts().WriteFacts()
	if !strings.HasPrefix(src, `\*`) {
		t.Errorf("fact file does not open with a Shen comment: %q", src[:20])
	}
	body := src[strings.Index(src, `*\`)+2:]
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "(") || !strings.HasSuffix(line, ")") {
			t.Errorf("non-form line outside the header comment: %q", line)
		}
	}
}

func TestRefLocationIsOneBased(t *testing.T) {
	r := Ref{File: "a.go", Line: 12, Col: 4}
	if got := r.Location(); got != "a.go:13:5" {
		t.Errorf("Location() = %q, want a.go:13:5", got)
	}
}

func TestDefContains(t *testing.T) {
	d := Def{StartLine: 10, StartCol: 4, EndLine: 12, EndCol: 2}
	cases := []struct {
		line, col int32
		want      bool
	}{
		{10, 4, true}, {10, 3, false}, {11, 0, true},
		{12, 2, true}, {12, 3, false}, {9, 99, false}, {13, 0, false},
	}
	for _, c := range cases {
		if got := d.Contains(c.line, c.col); got != c.want {
			t.Errorf("Contains(%d,%d) = %v, want %v", c.line, c.col, got, c.want)
		}
	}
}

func TestParseFactsRejectsBadFacts(t *testing.T) {
	cases := map[string]string{
		"unknown head": `(nope "a" "b")`,
		"short def":    `(def "a" "b" 1 2)`,
		"short ref":    `(ref "a" "b" 1)`,
		"long call":    `(call "a" "b" "c")`,
		"bad position": `(def "a" "b" x 2 3 4)`,
	}
	for name, src := range cases {
		if _, err := ParseFacts(src); err == nil {
			t.Errorf("%s: expected an error for %s", name, src)
		}
	}
}

// TestParseFactsToleratesComments guards the cache path: the fact
// file sb writes carries a header comment and blank lines, and
// re-reading it must not choke on either.
func TestParseFactsToleratesComments(t *testing.T) {
	src := "\\* a comment with (parens) and \"quotes\" *\\\n\n" + `(call "a" "b")` + "\n\n"
	fs, err := ParseFacts(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.Calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(fs.Calls))
	}
}

func TestCalleesOfIsDeterministic(t *testing.T) {
	fs := &FactSet{Calls: []Call{
		{Caller: "a", Callee: "z"}, {Caller: "a", Callee: "b"}, {Caller: "a", Callee: "b"},
	}}
	fs.Index()
	got := fs.CalleesOf("a")
	if len(got) != 2 || got[0] != "b" || got[1] != "z" {
		t.Errorf("CalleesOf = %v, want [b z] with the duplicate collapsed", got)
	}
}
