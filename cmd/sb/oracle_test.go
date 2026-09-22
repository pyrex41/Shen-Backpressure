package main

// oracle_test.go — W6.D. The `lowering` blame path, fired.
//
// W5 shipped `lowering` as a value the rules could name and nothing
// could produce, and recorded that as a gap: "the lowering blame value
// is implemented and unit-tested but cannot be produced in this
// environment". These tests produce it, on a real host.
//
// The disagreement is forced on the evaluator's side of the record.
// The Shen sample table is what `sb derive` reads as "this is what
// shen-derive's Go evaluator computed"; writing a table whose
// `expected` is wrong is exactly the situation a lowering bug creates,
// and it is the only way to stage one without shipping a broken
// evaluator. The host is the genuine article throughout: it loads the
// prelude and the spec and evaluates the define for real.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oracleTestSpec is a self-contained spec with one wrapper, one
// composite and one define over them, so the prelude has a `val` and
// an accessor to emit and the host has something to evaluate.
const oracleTestSpec = `(datatype item-id
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

// oracleTestProject stages a project directory the oracle can run in:
// an sb.toml naming the host and the spec, and a shen-derive dir so
// ensurePrelude finds the generator.
func oracleTestProject(t *testing.T, host string) (dir, specPath string) {
	t.Helper()
	dir = t.TempDir()
	specPath = filepath.Join(dir, "core.shen")
	if err := os.WriteFile(specPath, []byte(oracleTestSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	deriveDir, err := filepath.Abs(filepath.Join("..", "..", "shen-derive"))
	if err != nil {
		t.Fatal(err)
	}
	toml := "[project]\nlang = \"go\"\n\n[paths]\nspec = \"core.shen\"\n\n" +
		"[shen]\nbin = " + quoteTOML(host) + "\n\n" +
		"[derive]\ndir = " + quoteTOML(deriveDir) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "sb.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, specPath
}

func quoteTOML(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}

// writeOracleSamples writes a Shen sample table claiming `expected`
// for one case of `heavy?`.
func writeOracleSamples(t *testing.T, dir, specPath, caseID, expected string) {
	t.Helper()
	f := shenSampleFile{
		Func: "heavy?",
		Spec: specPath,
		Cases: []shenSampleCase{{
			Name:     caseID,
			Args:     []string{"5", `[10 "widget"]`},
			Expected: expected,
		}},
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ShenSamplesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, shenSamplesPath("heavy?")), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// reportWithCounterExample is a minimal report carrying one behavioral
// counter-example, as `sb derive` would have left it: blame already
// assigned by assignBlame, which with a host present says `impl` on
// the strength of a promise the oracle has yet to keep.
func reportWithCounterExample(caseID string) *DischargeReport {
	return &DischargeReport{
		SchemaVersion: 1,
		Rules: []DischargeRule{{
			Name:   "heavy?",
			Status: DischargeStatusViolated,
			CounterExamples: []DischargeCounter{{
				CaseID:     caseID,
				Input:      map[string]string{"_": "see the generated test file"},
				SpecOutput: "true",
				ImplOutput: "false",
				Blame:      BlameImpl,
				BlameBasis: BlameBasisEvaluatorAndHost,
				Rationale:  "Spec evaluates heavy? on this case to true; impl returned false.",
			}},
		}},
	}
}

// runOracleCase stages a project, writes a sample table with the given
// evaluator answer, and runs applySecondOracle from inside it.
func runOracleCase(t *testing.T, expected string) DischargeCounter {
	t.Helper()
	host := findTestShenHost(t)
	if host == "" {
		t.Skip("no Shen host: set $SHEN, put shen-sbcl/shen-scheme/shen on PATH, or run `make shen-go`")
	}
	dir, specPath := oracleTestProject(t, host)
	writeOracleSamples(t, dir, specPath, "case_00", expected)

	// applySecondOracle resolves paths against the working directory
	// (a gate runs in the project), so the test does too, and puts it
	// back afterwards.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	resolved := ResolveShenHost(cfg)
	if !resolved.Found() {
		t.Fatalf("the staged sb.toml did not resolve a host (bin = %q)", cfg.ShenBin)
	}
	r := reportWithCounterExample("case_00")
	applySecondOracle(cfg, r, resolved)
	return r.Rules[0].CounterExamples[0]
}

// TestSecondOracleAgreesAndBlamesImpl is the common case: the host
// reads the spec the way the evaluator did, so the disagreement is
// between the spec and the implementation.
//
// `(heavy? 5 [10 "widget"])` is `(> (val (w P)) (val Limit))` — 10 > 5
// — so both oracles say true.
func TestSecondOracleAgreesAndBlamesImpl(t *testing.T) {
	ce := runOracleCase(t, "true")
	if ce.Blame != BlameImpl {
		t.Errorf("blame: got %q, want %q", ce.Blame, BlameImpl)
	}
	if ce.BlameBasis != BlameBasisEvaluatorAndHost {
		t.Errorf("blame_basis: got %q, want %q", ce.BlameBasis, BlameBasisEvaluatorAndHost)
	}
	if !strings.Contains(ce.Rationale, "agreeing with shen-derive's evaluator") {
		t.Errorf("rationale does not record the host's agreement: %s", ce.Rationale)
	}
	// The goal is recorded so an auditor can re-run it by hand. That
	// is the whole difference between a report that asserts a second
	// oracle and one that can be checked.
	if got := ce.Input["shen_goal"]; got != `(heavy? 5 [10 "widget"])` {
		t.Errorf("shen_goal: got %q", got)
	}
}

// TestSecondOracleDisagreesAndBlamesLowering is the path W5 could
// describe and not reach. The sample table claims the evaluator
// computed false; the host computes true. Two readings of one spec
// differ, so the counter-example says nothing about the
// implementation and the blame moves to the lowering.
func TestSecondOracleDisagreesAndBlamesLowering(t *testing.T) {
	ce := runOracleCase(t, "false")
	if ce.Blame != BlameLowering {
		t.Fatalf("blame: got %q, want %q — the lowering path did not fire", ce.Blame, BlameLowering)
	}
	if ce.BlameBasis != BlameBasisEvaluatorAndHost {
		t.Errorf("blame_basis: got %q, want %q", ce.BlameBasis, BlameBasisEvaluatorAndHost)
	}
	for _, want := range []string{"DISAGREE", "evaluator says false", "says true"} {
		if !strings.Contains(ce.Rationale, want) {
			t.Errorf("rationale does not contain %q: %s", want, ce.Rationale)
		}
	}
}

// TestSecondOracleLeavesUnaskableCasesAlone: a counter-example with no
// entry in the sample table keeps whatever blame it had. Silently
// treating "the host was not asked" as "the host agreed" would make
// the evaluator-and-host basis worthless.
func TestSecondOracleLeavesUnaskableCasesAlone(t *testing.T) {
	host := findTestShenHost(t)
	if host == "" {
		t.Skip("no Shen host available")
	}
	dir, specPath := oracleTestProject(t, host)
	writeOracleSamples(t, dir, specPath, "case_00", "true")
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	r := reportWithCounterExample("case_99_not_in_table")
	applySecondOracle(cfg, r, ResolveShenHost(cfg))
	ce := r.Rules[0].CounterExamples[0]
	if ce.Blame != BlameImpl {
		t.Errorf("blame changed for an unasked case: %q", ce.Blame)
	}
	if strings.Contains(ce.Rationale, "agreeing with") {
		t.Errorf("rationale claims agreement for a case the host was never asked: %s", ce.Rationale)
	}
}

// TestNormalizeOracleValue pins the comparison that decides between
// `impl` and `lowering`. The host's writer and the sample table's
// literal punctuate lists differently, and a formatting difference
// reported as a lowering bug would be worse than no second oracle.
func TestNormalizeOracleValue(t *testing.T) {
	same := [][2]string{
		{"true", " true "},
		{`[1 2 3]`, `[1, 2, 3]`},
		{`[[1 "a"]]`, `[ [ 1 "a" ] ]`},
		{`"quoted"`, `quoted`},
	}
	for _, p := range same {
		if normalizeOracleValue(p[0]) != normalizeOracleValue(p[1]) {
			t.Errorf("%q and %q should normalise alike, got %q and %q",
				p[0], p[1], normalizeOracleValue(p[0]), normalizeOracleValue(p[1]))
		}
	}
	if normalizeOracleValue("true") == normalizeOracleValue("false") {
		t.Error("true and false must not normalise alike")
	}
}
