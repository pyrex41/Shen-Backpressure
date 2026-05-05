package report

import (
	"testing"
	"time"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

func TestClassifyDatatypes_Payment(t *testing.T) {
	sf, err := specfile.ParseFile("../../examples/payment/specs/core.shen")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	rules, err := ClassifyDatatypes(sf, "")
	if err != nil {
		t.Fatalf("ClassifyDatatypes: %v", err)
	}
	if len(rules) != len(sf.Datatypes) {
		t.Fatalf("rules: got %d, want %d", len(rules), len(sf.Datatypes))
	}

	// amount is constrained: one value premise + one verified premise.
	var amount *Rule
	for i := range rules {
		if rules[i].Name == "amount" {
			amount = &rules[i]
		}
	}
	if amount == nil {
		t.Fatal("amount rule missing")
	}
	if amount.Kind != "constrained" {
		t.Errorf("amount.Kind = %q, want constrained", amount.Kind)
	}
	if amount.Status != StatusDischarged {
		t.Errorf("amount.Status = %q, want %q", amount.Status, StatusDischarged)
	}
	if got := len(amount.Premises); got != 2 {
		t.Errorf("amount.Premises: got %d, want 2", got)
	}
	for _, p := range amount.Premises {
		if p.Discharge != DischargeStatic {
			t.Errorf("amount premise %s: discharge = %q, want static", p.ID, p.Discharge)
		}
	}
}

func TestClassifyDefine_RuntimeSampledShape(t *testing.T) {
	sf, err := specfile.ParseFile("../../examples/payment/specs/core.shen")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	def := sf.FindDefine("processable")
	if def == nil {
		t.Fatal("processable define missing")
	}
	rule := ClassifyDefine("specs/core.shen", def, 35, "deterministic-default", "Processable")

	if rule.Kind != "define" {
		t.Errorf("rule.Kind = %q, want define", rule.Kind)
	}
	if len(rule.Premises) != 1 {
		t.Fatalf("rule.Premises: got %d, want 1", len(rule.Premises))
	}
	p := rule.Premises[0]
	if p.Discharge != DischargeRuntimeSampled {
		t.Errorf("p.Discharge = %q, want runtime-sample", p.Discharge)
	}
	if p.SamplesPassed != 35 {
		t.Errorf("p.SamplesPassed = %d, want 35", p.SamplesPassed)
	}
	if p.SampleSeed == nil || *p.SampleSeed != "deterministic-default" {
		t.Errorf("p.SampleSeed = %v, want deterministic-default", p.SampleSeed)
	}
}

func TestBuildSummaryAndHash(t *testing.T) {
	sf, err := specfile.ParseFile("../../examples/payment/specs/core.shen")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	rules, _ := ClassifyDatatypes(sf, "")
	rules = append(rules, ClassifyDefine(sf.Path, sf.FindDefine("processable"), 35, "deterministic-default", "Processable"))

	r, err := Build(BuildOptions{
		SpecPath:       sf.Path,
		Now:            time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
		TargetLanguage: "go",
		Rules:          rules,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if r.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", r.SchemaVersion)
	}
	if r.Spec.Files[0].SHA256 == "" {
		t.Error("expected non-empty spec sha256")
	}
	if r.Summary.RuleCount != len(rules) {
		t.Errorf("Summary.RuleCount = %d, want %d", r.Summary.RuleCount, len(rules))
	}
	if r.Summary.PremisesRuntimeSampled != 1 {
		t.Errorf("Summary.PremisesRuntimeSampled = %d, want 1", r.Summary.PremisesRuntimeSampled)
	}
	if r.Summary.RulesDischarged != r.Summary.RuleCount {
		t.Errorf("rules_discharged = %d, want all %d", r.Summary.RulesDischarged, r.Summary.RuleCount)
	}
}
