package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/report"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/symbolic"
)

// TestVacuityFixtureIsUninhabited checks the library-level verdict on
// the committed fixture: bounded-fee is uninhabited, amount is not.
func TestVacuityFixtureIsUninhabited(t *testing.T) {
	solver, err := symbolic.FindSolver(0)
	if err != nil {
		t.Skip("z3 not on PATH")
	}
	sf, err := specfile.ParseFile(filepath.Join("testdata", "vacuous.shen"))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard")
	findings, err := symbolic.CheckVacuity(tt, solver)
	if err != nil {
		t.Fatalf("CheckVacuity: %v", err)
	}
	verdicts := map[string]symbolic.VacuityVerdict{}
	for _, f := range findings {
		verdicts[f.Type] = f.Verdict
	}
	if verdicts["bounded-fee"] != symbolic.VacuityVacuous {
		t.Fatalf("bounded-fee should be vacuous, got %v", verdicts)
	}
	if verdicts["amount"] != symbolic.VacuityInhabited {
		t.Fatalf("amount should be inhabited, got %v", verdicts)
	}
}

// TestDatatypeBlockFor covers the block-name lookup used to attach a
// vacuity finding to the right report rule. Payment's
// `balance-invariant` block concludes `balance-checked`, so the two
// names differ.
func TestDatatypeBlockFor(t *testing.T) {
	src := "(datatype balance-invariant\n" +
		"  Bal : number;\n" +
		"  (>= Bal 0) : verified;\n" +
		"  ==========\n" +
		"  [Bal] : balance-checked;)\n"
	f := filepath.Join(t.TempDir(), "spec.shen")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	sf, err := specfile.ParseFile(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := datatypeBlockFor(sf, "balance-checked"); got != "balance-invariant" {
		t.Fatalf("datatypeBlockFor(balance-checked) = %q", got)
	}
	if got := datatypeBlockFor(sf, "balance-invariant"); got != "balance-invariant" {
		t.Fatalf("datatypeBlockFor(balance-invariant) = %q", got)
	}
	if got := datatypeBlockFor(sf, "nope"); got != "nope" {
		t.Fatalf("unknown type should pass through, got %q", got)
	}
}

// TestVerifyFailsGateOnVacuousDatatype runs the real CLI against the
// fixture: it must exit non-zero, name the problem, and still write a
// discharge report carrying the `vacuous` rule status.
func TestVerifyFailsGateOnVacuousDatatype(t *testing.T) {
	if _, err := exec.LookPath("z3"); err != nil {
		t.Skip("z3 not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	outFile := filepath.Join(dir, "total_spec_test.go")
	reportFile := filepath.Join(dir, "discharge.json")

	cmd := exec.Command("go", "run", ".",
		"verify", filepath.Join("testdata", "vacuous.shen"),
		"--func", "total",
		"--impl-pkg", "example.com/derived",
		"--impl-func", "Total",
		"--import", "example.com/guards",
		"--out", outFile,
		"--report-out", reportFile,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("verify should fail on an uninhabited datatype; output:\n%s", out)
	}
	text := string(out)
	if !strings.Contains(text, "VACUOUS") || !strings.Contains(text, "bounded-fee") {
		t.Fatalf("failure should name the uninhabited type:\n%s", text)
	}
	if !strings.Contains(text, "uninhabited") {
		t.Fatalf("failure should be readable:\n%s", text)
	}

	// The artifacts are written before the gate fails, so the finding
	// is machine-readable and not only a log line.
	data, readErr := os.ReadFile(reportFile)
	if readErr != nil {
		t.Fatalf("discharge report should still be written: %v", readErr)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	if rep.Summary.RulesVacuous != 1 {
		t.Fatalf("rules_vacuous = %d, want 1", rep.Summary.RulesVacuous)
	}
	var found bool
	for _, r := range rep.Rules {
		if r.Name != "bounded-fee" {
			if r.Status == report.StatusVacuous {
				t.Fatalf("rule %s should not be vacuous", r.Name)
			}
			continue
		}
		found = true
		if r.Status != report.StatusVacuous {
			t.Fatalf("bounded-fee status = %q, want vacuous", r.Status)
		}
		if !strings.Contains(r.VacuityMessage, "uninhabited") {
			t.Fatalf("vacuity_message should explain: %q", r.VacuityMessage)
		}
		for _, p := range r.Premises {
			if p.DischargeBasis != report.BasisVacuousDatatype {
				t.Fatalf("premise %s basis = %q, want %q", p.ID, p.DischargeBasis, report.BasisVacuousDatatype)
			}
		}
	}
	if !found {
		t.Fatal("report has no bounded-fee rule")
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("the test file should still be written: %v", err)
	}
}

// TestVerifyPathCoverReportCounters checks the report side of path
// cover: the prover basis and the three additive counters.
func TestVerifyPathCoverReportCounters(t *testing.T) {
	if _, err := exec.LookPath("z3"); err != nil {
		t.Skip("z3 not on PATH")
	}
	dir := t.TempDir()
	reportFile := filepath.Join(dir, "discharge.json")
	cmd := exec.Command("go", "run", ".",
		"verify", filepath.Join("..", "examples", "payment", "specs", "core.shen"),
		"--func", "processable",
		"--impl-pkg", "ralph-shen-agent/internal/derived",
		"--impl-func", "Processable",
		"--import", "ralph-shen-agent/internal/shenguard",
		"--path-cover",
		"--out", filepath.Join(dir, "out.go"),
		"--report-out", reportFile,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify --path-cover failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "paths_total=10 paths_feasible=9 paths_dead=1") {
		t.Fatalf("unexpected path counters:\n%s", out)
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatal(err)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	var prem *report.Premise
	for i := range rep.Rules {
		if rep.Rules[i].Name != "processable" {
			continue
		}
		prem = &rep.Rules[i].Premises[0]
	}
	if prem == nil {
		t.Fatal("no processable rule in report")
	}
	if prem.DischargeBasis != report.BasisProverZ3PathCover {
		t.Fatalf("basis = %q, want %q", prem.DischargeBasis, report.BasisProverZ3PathCover)
	}
	if prem.PathsTotal == nil || *prem.PathsTotal != 10 {
		t.Fatalf("paths_total = %v, want 10", prem.PathsTotal)
	}
	if prem.PathsFeasible == nil || *prem.PathsFeasible != 9 {
		t.Fatalf("paths_feasible = %v, want 9", prem.PathsFeasible)
	}
	if prem.PathsDead == nil || *prem.PathsDead != 1 {
		t.Fatalf("paths_dead = %v, want 1", prem.PathsDead)
	}
	// The counters must be absent without path cover, so existing
	// reports stay byte-identical.
	if !strings.Contains(string(data), `"paths_total": 10`) {
		t.Fatalf("paths_total should be serialised:\n%s", firstN(string(data), 400))
	}
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
