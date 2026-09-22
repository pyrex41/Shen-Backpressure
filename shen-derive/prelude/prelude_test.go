package prelude

// prelude_test.go — what the generator emits, and what it refuses to.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

func build(t *testing.T, src string) *Prelude {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.shen")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	sf, err := specfile.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return Build(sf)
}

const oneWrapperSpec = `(datatype item-id
  X : string;
  ==============
  X : item-id;)

(datatype weight
  X : number;
  (>= X 0) : verified;
  ====================
  X : weight;)

(datatype parcel
  W : weight;
  Id : item-id;
  =====================
  [W Id] : parcel;)

(define heavy?
  {weight --> parcel --> boolean}
  Limit P -> (> (val (w P)) (val Limit)))
`

// TestNothingIsEmittedSpeculatively: only the intrinsics the spec's
// defines actually mention get a declaration. A signature in the TCB
// that buys no evidence is a standing invitation for the spec to start
// relying on it silently.
func TestNothingIsEmittedSpeculatively(t *testing.T) {
	p := build(t, oneWrapperSpec)
	if !strings.Contains(p.Declares, "(declare val [weight --> number])") {
		t.Errorf("val not declared at the spec's one constrained wrapper:\n%s", p.Declares)
	}
	if !strings.Contains(p.Declares, "(declare w [parcel --> weight])") {
		t.Errorf("the `w` accessor was not declared:\n%s", p.Declares)
	}
	// `id` is a field of parcel but the define never uses it.
	if strings.Contains(p.Declares, "(declare id ") {
		t.Errorf("an unused accessor was declared:\n%s", p.Declares)
	}
	// No list combinator appears in this define.
	for _, fn := range []string{"scanl", "foldr", "foldl", "unfoldr", "compose"} {
		if strings.Contains(p.Defines, "(define "+fn+"\n") {
			t.Errorf("%s was defined although the spec does not use it:\n%s", fn, p.Defines)
		}
	}
}

// TestEvalHalfIsRunnable: the eval prelude carries bodies, not types,
// and the accessor projects the right field. Shen's nth is 1-based, so
// field 0 is `(nth 1 …)` — an off-by-one here would make the second
// oracle disagree with the evaluator on every case and blame the
// lowering for a bug in this file.
func TestEvalHalfIsRunnable(t *testing.T) {
	p := build(t, oneWrapperSpec)
	if !strings.Contains(p.Eval, "(define val\n  X -> X)") {
		t.Errorf("val has no identity body:\n%s", p.Eval)
	}
	if !strings.Contains(p.Eval, "(define w\n  V -> (nth 1 V))") {
		t.Errorf("the `w` accessor does not project field 0 (Shen nth is 1-based):\n%s", p.Eval)
	}
	if strings.Contains(p.Eval, "(declare ") {
		t.Errorf("the eval half must carry no declares — it is loaded with tc off:\n%s", p.Eval)
	}
}

// TestCombinatorsAreEmittedWhenUsed, with their signatures, so the
// host typechecks them rather than taking them on trust.
func TestCombinatorsAreEmittedWhenUsed(t *testing.T) {
	p := build(t, `(define sums
  {(list number) --> number}
  Xs -> (foldr (lambda X (lambda Acc (+ X Acc))) 0 Xs))
`)
	if !strings.Contains(p.Defines, "(define foldr\n") {
		t.Errorf("foldr not defined although the spec folds:\n%s", p.Defines)
	}
	if !strings.Contains(p.Defines, "{(B --> (A --> A)) --> A --> (list B) --> A}") {
		t.Errorf("foldr has no type signature, so the host would not check it:\n%s", p.Defines)
	}
	if strings.Contains(p.Defines, "(define scanl\n") {
		t.Errorf("scanl emitted although unused:\n%s", p.Defines)
	}
}

// TestTwoConstrainedWrappersDeclareNoVal. Shen has no overloading, so
// picking one of two would be arbitrary. Refusing and saying why is
// the honest outcome: gate 4 then fails on the first `val`, loudly,
// rather than passing under a signature nobody chose.
func TestTwoConstrainedWrappersDeclareNoVal(t *testing.T) {
	p := build(t, `(datatype weight
  X : number;
  (>= X 0) : verified;
  ====================
  X : weight;)

(datatype label
  X : string;
  (not (= X "")) : verified;
  ====================
  X : label;)

(define positive?
  {weight --> boolean}
  W -> (> (val W) 0))
`)
	if strings.Contains(p.Declares, "(declare val ") {
		t.Errorf("val was declared despite two candidate wrappers:\n%s", p.Declares)
	}
	if len(p.Notes) == 0 {
		t.Fatal("the generator recorded no gap for the ambiguity")
	}
	joined := strings.Join(p.Notes, " ")
	if !strings.Contains(joined, "overloading") {
		t.Errorf("the gap does not explain itself: %s", joined)
	}
	if !strings.Contains(p.Declares, "GAP:") {
		t.Errorf("the gap is not visible in the emitted file:\n%s", p.Declares)
	}
}

// TestHeaderNamesTheSpecByBaseName: the prelude lands in a project's
// .sb/, so an absolute path in the header would make the file differ
// between two checkouts of one commit for no reason.
func TestHeaderNamesTheSpecByBaseName(t *testing.T) {
	p := build(t, oneWrapperSpec)
	for _, half := range []string{p.Declares, p.Defines, p.Eval} {
		if !strings.Contains(half, "spec.shen") {
			t.Errorf("header does not name the spec:\n%s", half)
		}
		if strings.Contains(half, string(filepath.Separator)+"spec.shen") {
			t.Errorf("header carries a path rather than a base name:\n%s", half)
		}
	}
}
