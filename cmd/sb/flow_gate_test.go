package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/cmd/sb/flow"
)

func testDecl(t *testing.T, src string) flow.Decl {
	t.Helper()
	decls, err := flow.ParseSpec("specs/core.shen", src)
	if err != nil {
		t.Fatalf("parsing flow form: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	return decls[0]
}

const sampleFlowForm = `(flow tenant-access-discipline
  (constructor-only internal/shenguard/NewTenantAccess internal/verified/CheckTenantAccess))`

// TestFlowRulesDischarged checks the mapping from a discharged flow
// premise to the report row: discharge "static" with the reserved
// basis "flow-analysis", and a rule status of discharged.
func TestFlowRulesDischarged(t *testing.T) {
	decl := testDecl(t, sampleFlowForm)
	results := []flow.Result{{
		PremiseID:  decl.Premises[0].ID(),
		Expression: decl.Premises[0].Expression(),
		Discharged: true,
		Considered: 3,
		Rationale:  "All 3 resolved references lie inside the allowed caller.",
	}}
	out := &IndexOutcome{Indexer: "scip-go", Available: true}

	rules := flowRules([]flow.Decl{decl}, results, out, flow.EngineGo)
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	r := rules[0]
	if r.Kind != FlowRuleKind {
		t.Errorf("kind = %q, want %q", r.Kind, FlowRuleKind)
	}
	if r.Status != DischargeStatusDischarged {
		t.Errorf("status = %q, want discharged", r.Status)
	}
	if r.SpecFile != "specs/core.shen" || !strings.Contains(r.SpecExcerpt, "constructor-only") {
		t.Errorf("spec provenance lost: %q / %q", r.SpecFile, r.SpecExcerpt)
	}
	if len(r.Premises) != 1 {
		t.Fatalf("got %d premises, want 1", len(r.Premises))
	}
	p := r.Premises[0]
	if p.Discharge != DischargeStatic {
		t.Errorf("discharge = %q, want %q", p.Discharge, DischargeStatic)
	}
	if p.DischargeBasis != DischargeBasisFlow {
		t.Errorf("basis = %q, want %q", p.DischargeBasis, DischargeBasisFlow)
	}
	if !strings.Contains(p.Rationale, flow.EngineGo) || !strings.Contains(p.Rationale, "scip-go") {
		t.Errorf("rationale does not name the engine and index: %q", p.Rationale)
	}
	if len(r.CounterExamples) != 0 {
		t.Errorf("discharged rule carries %d counter-examples", len(r.CounterExamples))
	}
}

// TestFlowRulesViolated checks that a violation becomes a
// counterexample with the file:line of the offending reference and
// the shortest violating path, which is the whole point of the gate:
// more bits back per run than "grep found something".
func TestFlowRulesViolated(t *testing.T) {
	decl := testDecl(t, sampleFlowForm)
	results := []flow.Result{{
		PremiseID:  decl.Premises[0].ID(),
		Expression: decl.Premises[0].Expression(),
		Considered: 4,
		Rationale:  "1 of 4 resolved references lie outside the allowed caller.",
		Violations: []flow.Violation{{
			PremiseID: decl.Premises[0].ID(),
			Location:  "internal/bypass_harness/a08.go:68:12",
			Symbol:    "multi-tenant-api/internal/shenguard/NewTenantAccess",
			Enclosing: "multi-tenant-api/internal/bypass_harness/ForgeTenantAccessViaAlias",
			Path:      []string{"a/B", "c/D"},
			Rationale: "aliased import reaches the constructor",
		}},
	}}
	out := &IndexOutcome{Indexer: "scip-go", Available: true}

	r := flowRules([]flow.Decl{decl}, results, out, flow.EngineGo)[0]
	if r.Status != DischargeStatusViolated {
		t.Errorf("status = %q, want violated", r.Status)
	}
	if r.Premises[0].Discharge != DischargeUnproven {
		t.Errorf("discharge = %q, want unproven", r.Premises[0].Discharge)
	}
	if got := r.Premises[0].CodeReferences; len(got) != 1 || got[0] != "internal/bypass_harness/a08.go:68:12" {
		t.Errorf("code references = %v", got)
	}
	if len(r.CounterExamples) != 1 {
		t.Fatalf("got %d counter-examples, want 1", len(r.CounterExamples))
	}
	ce := r.CounterExamples[0]
	if ce.ImplFile != "internal/bypass_harness/a08.go" {
		t.Errorf("impl file = %q", ce.ImplFile)
	}
	if ce.ImplLineHint == nil || *ce.ImplLineHint != 68 {
		t.Errorf("line hint = %v, want 68", ce.ImplLineHint)
	}
	if ce.Input["shortest_violating_path"] != "a/B -> c/D" {
		t.Errorf("path not carried: %q", ce.Input["shortest_violating_path"])
	}
	if ce.Input["reference"] != "internal/bypass_harness/a08.go:68:12" {
		t.Errorf("reference not carried: %q", ce.Input["reference"])
	}
}

// TestFlowRulesVacuousIsNotDischarged pins the honesty rule at the
// report boundary.
func TestFlowRulesVacuousIsNotDischarged(t *testing.T) {
	decl := testDecl(t, sampleFlowForm)
	results := []flow.Result{{
		PremiseID:  decl.Premises[0].ID(),
		Expression: decl.Premises[0].Expression(),
		Vacuous:    true,
		Rationale:  "No reference appears in the index.",
	}}
	r := flowRules([]flow.Decl{decl}, results, &IndexOutcome{Indexer: "scip-go", Available: true}, flow.EngineGo)[0]
	if r.Status != DischargeStatusUnproven {
		t.Errorf("status = %q, want unproven", r.Status)
	}
	if r.Premises[0].Discharge != DischargeUnproven {
		t.Errorf("discharge = %q, want unproven", r.Premises[0].Discharge)
	}
	if r.Premises[0].DischargeBasis != DischargeBasisFlow {
		t.Errorf("basis = %q, want %q (the engine did run)", r.Premises[0].DischargeBasis, DischargeBasisFlow)
	}
}

// TestFlowRulesGrepFallback is the no-indexer path: the premise must
// be recorded unproven with basis "grep-fallback" and a rationale
// that says plainly why a regex is not evidence.
func TestFlowRulesGrepFallback(t *testing.T) {
	decl := testDecl(t, sampleFlowForm)
	out := &IndexOutcome{Indexer: "scip-go", Available: false, InstallHint: "go install …"}

	r := flowRules([]flow.Decl{decl}, nil, out, "")[0]
	if r.Status != DischargeStatusUnproven {
		t.Errorf("status = %q, want unproven", r.Status)
	}
	p := r.Premises[0]
	if p.Discharge != DischargeUnproven {
		t.Errorf("discharge = %q, want unproven", p.Discharge)
	}
	if p.DischargeBasis != DischargeBasisGrepFallback {
		t.Errorf("basis = %q, want %q", p.DischargeBasis, DischargeBasisGrepFallback)
	}
	if !strings.Contains(p.Rationale, "aliased import") {
		t.Errorf("rationale does not explain the weakness: %q", p.Rationale)
	}
}

// TestMergeFlowRulesPreservesDeriveRules checks that the flow gate
// contributes to the discharge report without clobbering what
// `sb derive` put there, and that re-running replaces its own rules
// rather than duplicating them.
func TestMergeFlowRulesPreservesDeriveRules(t *testing.T) {
	// Another test in this package may have left cwd inside a deleted
	// temp directory, so restore it before asking for a new one.
	testRestoreCwd(t)
	dir := t.TempDir()
	t.Cleanup(func() { testRestoreCwd(t) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	existing := &DischargeReport{
		SchemaVersion: 1,
		Rules: []DischargeRule{{
			Name: "same-user?", Kind: "define",
			Status:          DischargeStatusDischarged,
			CounterExamples: []DischargeCounter{},
			Premises: []DischargePremise{{
				ID: "p0", Discharge: DischargeStatic, DischargeBasis: "constructor-inline-check",
			}},
		}},
	}
	if err := os.MkdirAll(filepath.Dir(DischargeReportPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeDischarge(DischargeReportPath, existing); err != nil {
		t.Fatal(err)
	}

	decl := testDecl(t, sampleFlowForm)
	rules := flowRules([]flow.Decl{decl}, []flow.Result{{
		PremiseID: decl.Premises[0].ID(), Discharged: true, Considered: 1, Rationale: "ok",
	}}, &IndexOutcome{Indexer: "scip-go", Available: true}, flow.EngineGo)

	for i := 0; i < 2; i++ { // idempotence
		if err := mergeFlowRules(rules); err != nil {
			t.Fatalf("merge %d: %v", i, err)
		}
	}

	got, err := loadDischarge(DischargeReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("got %d rules, want 2 (the derive rule plus one flow rule): %+v", len(got.Rules), got.Rules)
	}
	var sawDerive, sawFlow bool
	for _, r := range got.Rules {
		switch r.Kind {
		case "define":
			sawDerive = true
		case FlowRuleKind:
			sawFlow = true
		}
	}
	if !sawDerive || !sawFlow {
		t.Errorf("derive rule kept = %v, flow rule added = %v", sawDerive, sawFlow)
	}
	if got.Summary.PremisesStatic != 2 {
		t.Errorf("summary static premises = %d, want 2", got.Summary.PremisesStatic)
	}
	if got.Spec.RuleCount != 2 {
		t.Errorf("spec.rule_count = %d, want 2", got.Spec.RuleCount)
	}
}

// TestFlowGateIsWiredFromManifest checks that a [[gates]] entry of
// kind "flow" is served by `sb flow` with the entry's `run` field
// passed through as the grep fallback.
func TestFlowGateIsWiredFromManifest(t *testing.T) {
	cfg := &Config{
		Gates: []GateDef{
			{Name: "tcb-audit", Kind: GateKindCommand, Run: "./bin/shenguard-audit.sh"},
			{Name: "flow", Kind: GateKindFlow, Run: "./bin/flow-grep-fallback.sh --strict"},
		},
	}
	gates := buildGateList(cfg)
	if len(gates) != 2 {
		t.Fatalf("got %d gates, want 2", len(gates))
	}
	g := gates[1]
	if g.name != "flow" || g.kind != GateKindFlow {
		t.Fatalf("gate = %+v", g)
	}
	self, err := os.Executable()
	if err != nil {
		t.Skip("no executable path in this environment")
	}
	if g.cmd != self {
		t.Errorf("flow gate runs %q, want the sb binary %q", g.cmd, self)
	}
	want := []string{"flow", "-fallback", "./bin/flow-grep-fallback.sh --strict"}
	if len(g.args) != len(want) {
		t.Fatalf("args = %v, want %v", g.args, want)
	}
	for i := range want {
		if g.args[i] != want[i] {
			t.Fatalf("args = %v, want %v", g.args, want)
		}
	}
}
