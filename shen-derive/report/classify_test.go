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

	// amount is constrained: one value premise (static) + one verified
	// premise carrying :runtime-via :eval (profile B, evaluator-hosted).
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
	profileB := 0
	for _, p := range amount.Premises {
		want := DischargeStatic
		if p.RuntimeProfile == "B" {
			profileB++
			want = DischargeRuntimeEvaluator
		}
		if p.Discharge != want {
			t.Errorf("amount premise %s: discharge = %q, want %q", p.ID, p.Discharge, want)
		}
	}
	if profileB != 1 {
		t.Errorf("amount profile-B premises: got %d, want 1", profileB)
	}
}

// TestClassifyDatatypes_RuntimeViaProfiles pins the four runtime-via
// discharge profiles (A–D). Each datatype carries one value premise
// (statically discharged) plus one verified premise with a
// :runtime-via marker that should classify by its profile.
func TestClassifyDatatypes_RuntimeViaProfiles(t *testing.T) {
	mk := func(name string, m *specfile.RuntimeViaMarker) specfile.Datatype {
		return specfile.Datatype{
			Name: name,
			Rules: []specfile.Rule{{
				Premises: []specfile.Premise{{VarName: "X", TypeName: "string"}},
				Verified: []specfile.VerifiedPremise{{
					Raw:        "(> (length X) 0)",
					RuntimeVia: m,
				}},
				Conclusion: specfile.Conclusion{Fields: []string{"X"}, TypeName: name, IsWrapped: true},
			}},
		}
	}
	sf := &specfile.SpecFile{
		Path: "specs/core.shen",
		Datatypes: []specfile.Datatype{
			mk("dt-a", &specfile.RuntimeViaMarker{Checker: "shenEval"}),
			mk("dt-b", &specfile.RuntimeViaMarker{Eval: true}),
			mk("dt-c", &specfile.RuntimeViaMarker{Checker: "checkThing", Sampled: true}),
			mk("dt-d", &specfile.RuntimeViaMarker{Checker: "checkTenantMembership", RequiresDB: true}),
		},
	}

	rules, err := ClassifyDatatypes(sf, "")
	if err != nil {
		t.Fatalf("ClassifyDatatypes: %v", err)
	}

	want := map[string]struct {
		discharge string
		basis     string
		profile   string
		checker   string // "" → RuntimeChecker must be nil
	}{
		"dt-a": {DischargeRuntimeAttested, BasisRuntimeViaWitness, "A", "shenEval"},
		"dt-b": {DischargeRuntimeEvaluator, BasisRuntimeViaEvaluator, "B", ""},
		"dt-c": {DischargeRuntimeAttestedSampled, BasisRuntimeViaSampledEquivalence, "C", "checkThing"},
		"dt-d": {DischargeRuntimeAttestedDB, BasisRuntimeViaDBAttested, "D", "checkTenantMembership"},
	}

	for _, r := range rules {
		w, ok := want[r.Name]
		if !ok {
			t.Errorf("unexpected rule %q", r.Name)
			continue
		}
		var via *Premise
		for i := range r.Premises {
			if r.Premises[i].RuntimeProfile != "" {
				via = &r.Premises[i]
			}
		}
		if via == nil {
			t.Errorf("%s: no runtime-via premise classified", r.Name)
			continue
		}
		if via.Discharge != w.discharge {
			t.Errorf("%s: discharge = %q, want %q", r.Name, via.Discharge, w.discharge)
		}
		if via.DischargeBasis != w.basis {
			t.Errorf("%s: basis = %q, want %q", r.Name, via.DischargeBasis, w.basis)
		}
		if via.RuntimeProfile != w.profile {
			t.Errorf("%s: profile = %q, want %q", r.Name, via.RuntimeProfile, w.profile)
		}
		if w.checker == "" {
			if via.RuntimeChecker != nil {
				t.Errorf("%s: RuntimeChecker = %q, want nil (profile B)", r.Name, *via.RuntimeChecker)
			}
		} else {
			if via.RuntimeChecker == nil || *via.RuntimeChecker != w.checker {
				t.Errorf("%s: RuntimeChecker = %v, want %q", r.Name, via.RuntimeChecker, w.checker)
			}
		}
	}

	// Summary counts each profile bucket exactly once.
	s := summarise(rules)
	if s.PremisesRuntimeAttested != 1 || s.PremisesRuntimeEvaluator != 1 ||
		s.PremisesRuntimeAttestedSampled != 1 || s.PremisesRuntimeAttestedDB != 1 {
		t.Errorf("summary runtime counters = A:%d B:%d C:%d D:%d, want 1 each",
			s.PremisesRuntimeAttested, s.PremisesRuntimeEvaluator,
			s.PremisesRuntimeAttestedSampled, s.PremisesRuntimeAttestedDB)
	}
	// The four value premises (X : string) are still static.
	if s.PremisesStatic != 4 {
		t.Errorf("PremisesStatic = %d, want 4", s.PremisesStatic)
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

// TestBuildPopulatesShenRuntime asserts that the ShenRuntime
// BuildOption surfaces in Tools.ShenRuntime + flips
// Tools.ShenRuntimeAvailable. Default (empty option) preserves the
// historical nil / false output, so existing callers stay unchanged.
// See thoughts/shared/research/2026-05-22-schema-v1-additions.md.
func TestBuildPopulatesShenRuntime(t *testing.T) {
	sf, err := specfile.ParseFile("../../examples/payment/specs/core.shen")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	rules, _ := ClassifyDatatypes(sf, "")

	// Default: ShenRuntime empty → fields stay nil / false.
	defaultR, err := Build(BuildOptions{
		SpecPath:       sf.Path,
		Now:            time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
		TargetLanguage: "go",
		Rules:          rules,
	})
	if err != nil {
		t.Fatalf("Build (default): %v", err)
	}
	if defaultR.Tools.ShenRuntime != nil {
		t.Errorf("default ShenRuntime: want nil, got %v", *defaultR.Tools.ShenRuntime)
	}
	if defaultR.Tools.ShenRuntimeAvailable {
		t.Errorf("default ShenRuntimeAvailable: want false, got true")
	}

	// Populated: ShenRuntime="shen-sbcl" surfaces in both fields.
	r, err := Build(BuildOptions{
		SpecPath:       sf.Path,
		Now:            time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
		TargetLanguage: "go",
		ShenRuntime:    "shen-sbcl",
		Rules:          rules,
	})
	if err != nil {
		t.Fatalf("Build (populated): %v", err)
	}
	if r.Tools.ShenRuntime == nil || *r.Tools.ShenRuntime != "shen-sbcl" {
		t.Errorf("ShenRuntime: want shen-sbcl, got %v", r.Tools.ShenRuntime)
	}
	if !r.Tools.ShenRuntimeAvailable {
		t.Errorf("ShenRuntimeAvailable: want true, got false")
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
