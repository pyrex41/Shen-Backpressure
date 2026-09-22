package flow

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/cmd/sb/internal/scip"
)

var update = flag.Bool("update", false, "rewrite the golden fact file from the fixture index")

const (
	fixtureIndex = "testdata/tiny.scip"
	fixtureFacts = "testdata/tiny.facts.shen"
)

// loadFixture decodes the checked-in index and builds facts from it.
func loadFixture(t *testing.T) *FactSet {
	t.Helper()
	raw, err := os.ReadFile(fixtureIndex)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	idx, err := scip.Parse(raw)
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	return FromIndex(idx, "scip-go")
}

// TestGoldenFacts pins the fact file the index lowers to. The fixture
// index was produced by scip-go over testdata/tinysrc (see its
// go.mod for the regeneration recipe); the golden file is the
// contract every flow engine reads, so a change to it is a change to
// the interface between `sb index` and the rules.
func TestGoldenFacts(t *testing.T) {
	facts := loadFixture(t)
	got := facts.WriteFacts()

	if *update {
		if err := os.WriteFile(fixtureFacts, []byte(got), 0o644); err != nil {
			t.Fatalf("updating golden: %v", err)
		}
		t.Logf("wrote %s", filepath.Clean(fixtureFacts))
		return
	}

	want, err := os.ReadFile(fixtureFacts)
	if err != nil {
		t.Fatalf("reading golden (run: go test ./flow -run TestGoldenFacts -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("fact file drifted from %s.\nRe-run with -update after reviewing the diff.\n--- got ---\n%s", fixtureFacts, got)
	}
}

// TestGoldenFactsRoundTrip checks that the emitted fact file parses
// back to the same facts. The Shen engine reads the file, the Go
// engine reads the in-memory set, and the two must agree.
func TestGoldenFactsRoundTrip(t *testing.T) {
	facts := loadFixture(t)
	reparsed, err := ParseFacts(facts.WriteFacts())
	if err != nil {
		t.Fatalf("re-parsing emitted facts: %v", err)
	}
	if len(reparsed.Defs) != len(facts.Defs) {
		t.Errorf("defs: parsed %d, emitted %d", len(reparsed.Defs), len(facts.Defs))
	}
	if len(reparsed.Refs) != len(facts.Refs) {
		t.Errorf("refs: parsed %d, emitted %d", len(reparsed.Refs), len(facts.Refs))
	}
	if len(reparsed.Calls) != len(facts.Calls) {
		t.Errorf("calls: parsed %d, emitted %d", len(reparsed.Calls), len(facts.Calls))
	}
	if reparsed.WriteFacts() == "" {
		t.Error("re-emitted fact file is empty")
	}
}

// TestFixtureEnclosingAttribution is the property that makes the fact
// vocabulary useful: every reference inside a function body is
// attributed to that function by range containment, not by proximity.
func TestFixtureEnclosingAttribution(t *testing.T) {
	facts := loadFixture(t)
	var found bool
	for _, r := range facts.Refs {
		if Canonical(r.Symbol) != "tiny/guard/NewAccess" {
			continue
		}
		switch Canonical(r.Enclosing) {
		case "tiny/app/Check", "tiny/app/ForgeAccess":
			found = true
		case "":
			t.Errorf("reference to NewAccess at %s has no enclosing definition", r.Location())
		}
	}
	if !found {
		t.Fatal("no reference to tiny/guard/NewAccess attributed to a known caller")
	}
}
