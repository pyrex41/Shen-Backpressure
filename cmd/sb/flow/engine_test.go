package flow

import (
	"strings"
	"testing"
)

// parseDecl is a helper: parse one (flow …) form and fail loudly.
func parseDecl(t *testing.T, src string) Decl {
	t.Helper()
	decls, err := ParseSpec("test.shen", src)
	if err != nil {
		t.Fatalf("parsing %s: %v", src, err)
	}
	if len(decls) != 1 {
		t.Fatalf("parsed %d decls, want 1", len(decls))
	}
	return decls[0]
}

func resultByID(t *testing.T, results []Result, id string) Result {
	t.Helper()
	for _, r := range results {
		if r.PremiseID == id {
			return r
		}
	}
	t.Fatalf("no result for premise %q", id)
	return Result{}
}

// TestConstructorOnlyCatchesAliasedCaller is the headline property:
// the fixture's ForgeAccess reaches the raw constructor through an
// aliased import, which a regex over the unaliased package qualifier
// cannot see and the resolved symbol graph resolves to the same
// symbol.
func TestConstructorOnlyCatchesAliasedCaller(t *testing.T) {
	facts := loadFixture(t)
	decl := parseDecl(t, `(flow guard-discipline
  (constructor-only guard/NewAccess app/Check))`)

	results := Evaluate(facts, []Decl{decl})
	res := resultByID(t, results, "constructor-only:guard/NewAccess")

	if res.Discharged {
		t.Fatal("premise reported discharged although ForgeAccess calls the constructor")
	}
	if len(res.Violations) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(res.Violations), res.Violations)
	}
	v := res.Violations[0]
	if !strings.HasPrefix(v.Location, "app/app.go:27:") {
		t.Errorf("violation location = %q, want the ForgeAccess call site in app/app.go line 27", v.Location)
	}
	if v.Enclosing != "tiny/app/ForgeAccess" {
		t.Errorf("violation enclosing = %q, want tiny/app/ForgeAccess", v.Enclosing)
	}
	if res.Considered != 2 {
		t.Errorf("considered %d references, want 2 (Check and ForgeAccess)", res.Considered)
	}
}

// TestConstructorOnlyDischarges checks the positive case: allow both
// callers and the premise is discharged over a non-empty fact range.
func TestConstructorOnlyDischarges(t *testing.T) {
	facts := loadFixture(t)
	decl := parseDecl(t, `(flow guard-discipline
  (constructor-only guard/NewAccess app/Check app/ForgeAccess))`)

	res := resultByID(t, Evaluate(facts, []Decl{decl}), "constructor-only:guard/NewAccess")
	if !res.Discharged {
		t.Fatalf("premise not discharged: %s (%+v)", res.Rationale, res.Violations)
	}
	if res.Vacuous {
		t.Error("premise reported vacuous although two references were considered")
	}
}

// TestConstructorOnlyVacuous pins the honesty rule: a premise whose
// subject appears nowhere in the index is *not* discharged. A stale
// pattern must read as "no evidence", never as "proved".
func TestConstructorOnlyVacuous(t *testing.T) {
	facts := loadFixture(t)
	decl := parseDecl(t, `(flow guard-discipline
  (constructor-only guard/NoSuchConstructor app/Check))`)

	res := resultByID(t, Evaluate(facts, []Decl{decl}), "constructor-only:guard/NoSuchConstructor")
	if res.Discharged {
		t.Error("a premise that ranged over nothing was reported discharged")
	}
	if !res.Vacuous {
		t.Error("premise not marked vacuous")
	}
}

// TestMustPassThroughFindsShortestPath checks the noninterference
// search: HandleListBad reaches the sink via readRows without
// obtaining the proof, and HandleListGood is pruned because it
// references the declassifier.
func TestMustPassThroughFindsShortestPath(t *testing.T) {
	facts := loadFixture(t)
	decl := parseDecl(t, `(flow sink-discipline
  (must-pass-through app/HandleList* app/Check Store#Query))`)

	res := resultByID(t, Evaluate(facts, []Decl{decl}), "must-pass-through:app/HandleList*→Store#Query")
	if res.Considered != 2 {
		t.Fatalf("considered %d sources, want 2 (HandleListGood, HandleListBad)", res.Considered)
	}
	if len(res.Violations) != 1 {
		t.Fatalf("got %d violations, want 1 (HandleListBad only): %+v", len(res.Violations), res.Violations)
	}
	v := res.Violations[0]
	want := []string{"tiny/app/HandleListBad", "tiny/app/readRows"}
	if len(v.Path) != len(want) {
		t.Fatalf("violating path = %v, want %v", v.Path, want)
	}
	for i := range want {
		if v.Path[i] != want[i] {
			t.Fatalf("violating path = %v, want %v", v.Path, want)
		}
	}
	if !strings.HasPrefix(v.Location, "app/app.go:50:") {
		t.Errorf("violation location = %q, want the Query call site in app/app.go line 50", v.Location)
	}
}

// TestMustPassThroughDischargesWhenProofPresent restricts the source
// pattern to the good handler and expects a discharge.
func TestMustPassThroughDischargesWhenProofPresent(t *testing.T) {
	facts := loadFixture(t)
	decl := parseDecl(t, `(flow sink-discipline
  (must-pass-through app/HandleListGood app/Check Store#Query))`)

	res := resultByID(t, Evaluate(facts, []Decl{decl}), "must-pass-through:app/HandleListGood→Store#Query")
	if !res.Discharged {
		t.Fatalf("premise not discharged: %s (%+v)", res.Rationale, res.Violations)
	}
}

// TestMustPassThroughVacuous pins the same honesty rule for the
// path-search premise.
func TestMustPassThroughVacuous(t *testing.T) {
	facts := loadFixture(t)
	decl := parseDecl(t, `(flow sink-discipline
  (must-pass-through app/NoSuchHandler app/Check Store#Query))`)

	res := resultByID(t, Evaluate(facts, []Decl{decl}), "must-pass-through:app/NoSuchHandler→Store#Query")
	if res.Discharged || !res.Vacuous {
		t.Errorf("want vacuous and not discharged, got discharged=%v vacuous=%v", res.Discharged, res.Vacuous)
	}
}

// TestEvaluateFromFactFile runs the whole evaluation over facts that
// went through the fact-file round trip, which is the path the Shen
// engine takes. Both engines must see the same fact base.
func TestEvaluateFromFactFile(t *testing.T) {
	facts, err := ParseFacts(loadFixture(t).WriteFacts())
	if err != nil {
		t.Fatalf("parsing facts: %v", err)
	}
	decl := parseDecl(t, `(flow both
  (constructor-only guard/NewAccess app/Check)
  (must-pass-through app/HandleList* app/Check Store#Query))`)

	results := Evaluate(facts, []Decl{decl})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if len(r.Violations) != 1 {
			t.Errorf("%s: got %d violations, want 1", r.PremiseID, len(r.Violations))
		}
	}
}
