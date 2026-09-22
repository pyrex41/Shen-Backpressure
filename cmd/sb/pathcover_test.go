package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyDerivePathCoverDefaults covers the precedence between the
// [derive] table's path_cover/path_depth defaults and the per-spec
// overrides in [[derive.specs]].
func TestApplyDerivePathCoverDefaults(t *testing.T) {
	yes, no := true, false
	specs := []tomlDeriveSpec{
		{Path: "a.shen", Func: "a"},                                // inherits the table default
		{Path: "b.shen", Func: "b", PathCover: &no},                // explicit opt-out
		{Path: "c.shen", Func: "c", PathCover: &yes, PathDepth: 6}, // explicit opt-in + depth
	}
	cfg := &Config{}
	applyDerive(cfg, "", true, 3, specs)
	if len(cfg.DeriveSpecs) != 3 {
		t.Fatalf("want 3 specs, got %d", len(cfg.DeriveSpecs))
	}
	if !cfg.DeriveSpecs[0].PathCover || cfg.DeriveSpecs[0].PathDepth != 3 {
		t.Fatalf("spec a should inherit the table default: %+v", cfg.DeriveSpecs[0])
	}
	if cfg.DeriveSpecs[1].PathCover {
		t.Fatalf("spec b opted out but PathCover is true: %+v", cfg.DeriveSpecs[1])
	}
	if !cfg.DeriveSpecs[2].PathCover || cfg.DeriveSpecs[2].PathDepth != 6 {
		t.Fatalf("spec c should override depth: %+v", cfg.DeriveSpecs[2])
	}

	// With no table-level default, an unset spec stays off — the
	// feature must be opt-in so existing projects are unaffected.
	cfg2 := &Config{}
	applyDerive(cfg2, "", false, 0, []tomlDeriveSpec{{Path: "a.shen", Func: "a"}})
	if cfg2.DeriveSpecs[0].PathCover || cfg2.DeriveSpecs[0].PathDepth != 0 {
		t.Fatalf("path cover must default off: %+v", cfg2.DeriveSpecs[0])
	}
}

// TestLoadConfigPathCover checks the TOML surface end to end.
func TestLoadConfigPathCover(t *testing.T) {
	dir := t.TempDir()
	toml := `
[project]
lang = "go"
pkg = "shenguard"

[paths]
spec = "specs/core.shen"
output = "internal/shenguard/guards_gen.go"

[engine]
relaxed = false

[derive]
dir = "../../shen-derive"
path_cover = true
path_depth = 3

[[derive.specs]]
path = "specs/core.shen"
func = "processable"
impl_pkg = "x/internal/derived"
impl_func = "Processable"
guard_pkg = "x/internal/shenguard"
out_file = "internal/derived/processable_spec_test.go"

[[derive.specs]]
path = "specs/core.shen"
func = "other"
impl_pkg = "x/internal/derived"
impl_func = "Other"
guard_pkg = "x/internal/shenguard"
out_file = "internal/derived/other_spec_test.go"
path_cover = false
`
	if err := os.WriteFile(filepath.Join(dir, "sb.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	prev, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(prev) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.DeriveSpecs) != 2 {
		t.Fatalf("want 2 derive specs, got %d", len(cfg.DeriveSpecs))
	}
	if !cfg.DeriveSpecs[0].PathCover || cfg.DeriveSpecs[0].PathDepth != 3 {
		t.Fatalf("first spec: %+v", cfg.DeriveSpecs[0])
	}
	if cfg.DeriveSpecs[1].PathCover {
		t.Fatalf("second spec opted out: %+v", cfg.DeriveSpecs[1])
	}
}

// TestVacuousRulesAndSummary covers the vacuity plumbing on the sb
// side: the summary counter, the rule scan sb derive fails on, and the
// interaction with the tests-did-not-run downgrade.
func TestVacuousRulesAndSummary(t *testing.T) {
	r := &DischargeReport{
		SchemaVersion: 1,
		Rules: []DischargeRule{
			{
				Name:           "bounded-fee",
				Status:         DischargeStatusVacuous,
				VacuityMessage: "datatype bounded-fee is uninhabited",
				Premises: []DischargePremise{
					{ID: "bounded-fee.verified-x", Discharge: DischargeUnproven, DischargeBasis: "vacuous-datatype"},
				},
			},
			{
				Name:   "total",
				Status: DischargeStatusDischarged,
				Premises: []DischargePremise{
					{ID: "total.oracle-spec-equiv", Discharge: DischargeRuntimeSampled, SamplesPassed: 12},
				},
			},
		},
	}
	r.Summary = computeDischargeSummary(r.Rules)
	if r.Summary.RulesVacuous != 1 {
		t.Fatalf("rules_vacuous = %d, want 1", r.Summary.RulesVacuous)
	}
	if r.Summary.RulesDischarged != 1 {
		t.Fatalf("rules_discharged = %d, want 1", r.Summary.RulesDischarged)
	}
	names := vacuousRules(r)
	if len(names) != 1 || names[0] != "bounded-fee" {
		t.Fatalf("vacuousRules = %v", names)
	}
	if vacuousRules(nil) != nil {
		t.Fatal("vacuousRules(nil) should be nil")
	}

	// A --regen/--skip-test downgrade must not soften "vacuous" to
	// "unproven": the uninhabited finding does not depend on tests.
	downgradeRuntimeSampledToUnproven(r, "tests did not run")
	if r.Rules[0].Status != DischargeStatusVacuous {
		t.Fatalf("vacuous status lost to the downgrade: %q", r.Rules[0].Status)
	}
	if r.Rules[1].Status != DischargeStatusUnproven {
		t.Fatalf("sampled rule should be downgraded, got %q", r.Rules[1].Status)
	}
	if r.Summary.RulesVacuous != 1 {
		t.Fatalf("rules_vacuous after downgrade = %d", r.Summary.RulesVacuous)
	}
}

// TestPathCounterPassthrough checks the three additive premise counters
// survive an unmarshal/merge/marshal round trip, and that a report
// without them stays free of the keys.
func TestPathCounterPassthrough(t *testing.T) {
	src := `{
      "schema_version": 1,
      "spec": {"files": [{"path": "specs/core.shen", "sha256": "abc"}], "rule_count": 1},
      "impl": {"git_commit": null, "git_dirty": null, "target_languages": ["go"]},
      "tools": {"sb_version": "", "shen_derive_version": "0.3.0", "shengen_version": "",
                "shen_runtime": null, "shen_runtime_available": false},
      "rules": [{
        "name": "processable", "kind": "define", "spec_file": "specs/core.shen",
        "spec_excerpt": "", "human_description": "", "human_description_source": "auto-generated",
        "premises": [{
          "id": "processable.oracle-spec-equiv", "expression": "spec ≡ impl",
          "discharge": "runtime-sample", "discharge_basis": "prover-z3-path-cover",
          "rationale": "", "samples_passed": 44, "samples_failed": 0,
          "sample_seed": "deterministic-default",
          "paths_total": 10, "paths_feasible": 9, "paths_dead": 1
        }],
        "status": "discharged", "discharged_since_commit": null, "counter_examples": []
      }],
      "summary": {"rule_count": 1, "rules_discharged": 1, "rules_violated": 0, "rules_unproven": 0,
                  "premises_total": 1, "premises_static": 0, "premises_runtime_sampled": 1,
                  "premises_unproven": 0},
      "signature": null
    }`
	var r DischargeReport
	if err := json.Unmarshal([]byte(src), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p := r.Rules[0].Premises[0]
	if p.PathsTotal == nil || *p.PathsTotal != 10 ||
		p.PathsFeasible == nil || *p.PathsFeasible != 9 ||
		p.PathsDead == nil || *p.PathsDead != 1 {
		t.Fatalf("counters not parsed: %+v", p)
	}

	merged := mergeDischargeReports([]*DischargeReport{&r})
	out, err := json.Marshal(merged)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"paths_total":10`, `"paths_feasible":9`, `"paths_dead":1`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("round trip lost %s:\n%s", want, out)
		}
	}

	// A premise with no path cover must not gain the keys.
	bare := &DischargeReport{Rules: []DischargeRule{{
		Name: "amount", Status: DischargeStatusDischarged,
		Premises: []DischargePremise{{ID: "amount.field-x", Discharge: DischargeStatic}},
	}}}
	bareOut, err := json.Marshal(mergeDischargeReports([]*DischargeReport{bare}))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"paths_total", "paths_feasible", "paths_dead", "rules_vacuous", "vacuity_message"} {
		if strings.Contains(string(bareOut), unwanted) {
			t.Fatalf("additive field %q leaked into a report that has none:\n%s", unwanted, bareOut)
		}
	}
}

// TestAuditRendersPathCoverAndVacuity checks the Markdown rendering.
func TestAuditRendersPathCoverAndVacuity(t *testing.T) {
	total, feasible, dead := 10, 9, 1
	r := &DischargeReport{
		SchemaVersion: 1,
		GeneratedAt:   "2026-09-22T00:00:00Z",
		Spec:          DischargeSpec{Files: []DischargeSpecFile{{Path: "specs/core.shen", SHA256: "abc"}}, RuleCount: 2},
		Impl:          DischargeImpl{TargetLanguages: []string{"go"}},
		Rules: []DischargeRule{
			{
				Name: "processable", Kind: "define", Status: DischargeStatusDischarged,
				Premises: []DischargePremise{{
					ID: "processable.oracle-spec-equiv", Expression: "spec ≡ impl",
					Discharge: DischargeRuntimeSampled, DischargeBasis: "prover-z3-path-cover",
					SamplesPassed: 44, PathsTotal: &total, PathsFeasible: &feasible, PathsDead: &dead,
				}},
			},
			{
				Name: "bounded-fee", Kind: "constrained", Status: DischargeStatusVacuous,
				VacuityMessage: "datatype bounded-fee is uninhabited: no value can satisfy (>= X 10) ∧ (< X 5)",
				Premises: []DischargePremise{{
					ID: "bounded-fee.verified-x", Expression: "(>= X 10) : verified",
					Discharge: DischargeUnproven, DischargeBasis: "vacuous-datatype",
				}},
			},
		},
	}
	r.Summary = computeDischargeSummary(r.Rules)
	md := renderAuditMarkdown(r, ".sb/discharge_report.json")

	for _, want := range []string{
		"1 vacuous",
		"At least one rule is vacuous",
		"⛔ Vacuous (uninhabited datatype)",
		"**Uninhabited.** datatype bounded-fee is uninhabited",
		"path cover — 10 path(s) enumerated, 9 feasible",
		"1 dead (unsatisfiable path condition), 0 undecided",
		"A dead path is a branch of the spec no input can reach",
		"**Path cover**",
		"**Vacuous**",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("rendering missing %q", want)
		}
	}
}
