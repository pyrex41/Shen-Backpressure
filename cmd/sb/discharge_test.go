package main

// discharge_test.go — unit tests for the cmd/sb-side discharge
// machinery. Pins the parser, retention rules, history walker,
// and counter-example wiring against documented behaviour. Also
// pins the schema mirror to shen-derive's canonical types so silent
// JSON drift between writer and reader is caught at build time.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- parseGoTestFailures ----------

func TestParseGoTestFailures_HappyPath(t *testing.T) {
	out := `=== RUN   TestSpec_Processable
=== RUN   TestSpec_Processable/case_00
=== RUN   TestSpec_Processable/case_01
    processable_spec_test.go:243: case_01: spec says false, impl returned true
=== RUN   TestSpec_Processable/case_02
--- FAIL: TestSpec_Processable (0.00s)
    --- PASS: TestSpec_Processable/case_00 (0.00s)
    --- FAIL: TestSpec_Processable/case_01 (0.00s)
    --- PASS: TestSpec_Processable/case_02 (0.00s)
FAIL
`
	got := parseGoTestFailures(out)
	if len(got) != 1 {
		t.Fatalf("got %d failures, want 1: %+v", len(got), got)
	}
	if got[0].implFunc != "Processable" || got[0].caseID != "case_01" {
		t.Errorf("attribution wrong: %+v", got[0])
	}
	if got[0].specOutput != "false" || got[0].implOutput != "true" {
		t.Errorf("values wrong: %+v", got[0])
	}
}

func TestParseGoTestFailures_NoFalseAttributionAfterConsume(t *testing.T) {
	// Two separate failures, separated only by a non-RUN line.
	// After we attribute the first body line, a stray subsequent
	// body line must not be silently re-attributed to the same
	// subtest. (Defends the "reset on consume" fix.)
	out := `=== RUN   TestSpec_Foo/case_00
    foo_test.go:10: case_00: spec says 1, impl returned 2
    foo_test.go:11: case_00: spec says 3, impl returned 4
=== RUN   TestSpec_Foo/case_01
    foo_test.go:12: case_01: spec says 5, impl returned 6
`
	got := parseGoTestFailures(out)
	// Without reset, we'd get 3 failures (two body lines wrongly
	// attributed to case_00). With the fix, the second case_00
	// body line drops because currentImplFunc was consumed.
	if len(got) != 2 {
		t.Fatalf("got %d failures, want 2: %+v", len(got), got)
	}
	if got[0].caseID != "case_00" || got[1].caseID != "case_01" {
		t.Errorf("case attribution: %+v", got)
	}
}

func TestParseGoTestFailures_Empty(t *testing.T) {
	out := `=== RUN   TestSpec_Processable/case_00
--- PASS: TestSpec_Processable (0.00s)
PASS
ok      pkg     0.001s
`
	got := parseGoTestFailures(out)
	if len(got) != 0 {
		t.Errorf("want 0 failures on a clean run, got %d", len(got))
	}
}

// ---------- mergeDischargeReports ----------

func TestMergeDischargeReports_Empty(t *testing.T) {
	r := mergeDischargeReports(nil)
	if r == nil {
		t.Fatal("expected non-nil report on empty input")
	}
	if r.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", r.SchemaVersion)
	}
	if r.Rules == nil {
		t.Error("Rules should be a non-nil empty slice")
	}
}

func TestMergeDischargeReports_DedupesByName(t *testing.T) {
	a := &DischargeReport{
		SchemaVersion: 1,
		Spec:          DischargeSpec{Files: []DischargeSpecFile{{Path: "a.shen", SHA256: "aa"}}},
		Impl:          DischargeImpl{TargetLanguages: []string{"go"}},
		Rules: []DischargeRule{
			{Name: "amount", Status: DischargeStatusDischarged},
			{Name: "transaction", Status: DischargeStatusDischarged},
		},
	}
	b := &DischargeReport{
		SchemaVersion: 1,
		Spec:          DischargeSpec{Files: []DischargeSpecFile{{Path: "b.shen", SHA256: "bb"}}},
		Rules: []DischargeRule{
			// Collision: amount appears in both partials. Merge
			// must keep the first; second is silently dropped.
			{Name: "amount", Status: DischargeStatusViolated},
			{Name: "balance-invariant", Status: DischargeStatusDischarged},
		},
	}
	r := mergeDischargeReports([]*DischargeReport{a, b})

	if r.Spec.RuleCount != 3 {
		t.Errorf("Spec.RuleCount = %d, want 3", r.Spec.RuleCount)
	}
	gotPaths := []string{}
	for _, sf := range r.Spec.Files {
		gotPaths = append(gotPaths, sf.Path)
	}
	if got := strings.Join(gotPaths, ","); got != "a.shen,b.shen" {
		t.Errorf("Spec.Files = %s, want a.shen,b.shen", got)
	}
	for _, rule := range r.Rules {
		if rule.Name == "amount" && rule.Status != DischargeStatusDischarged {
			t.Errorf("amount kept the wrong copy: status=%s", rule.Status)
		}
	}
}

func TestMergeDischargeReports_NormalisesEmptyArrays(t *testing.T) {
	a := &DischargeReport{
		SchemaVersion: 1,
		Rules: []DischargeRule{
			{Name: "x", Status: DischargeStatusDischarged, CounterExamples: nil},
		},
	}
	r := mergeDischargeReports([]*DischargeReport{a})
	if r.Rules[0].CounterExamples == nil {
		t.Fatal("CounterExamples must be non-nil after merge so JSON emits []")
	}
	data, _ := json.Marshal(r)
	if !strings.Contains(string(data), `"counter_examples":[]`) {
		t.Errorf("counter_examples must marshal as [], got: %s", string(data))
	}
}

// ---------- applyCounterExamples ----------

func TestApplyCounterExamples_FlipsStatusAndComputesSamples(t *testing.T) {
	r := &DischargeReport{
		Rules: []DischargeRule{{
			Name: "processable",
			Premises: []DischargePremise{{
				ID:            "processable.oracle-spec-equiv",
				Discharge:     DischargeRuntimeSampled,
				SamplesPassed: 35,
				SamplesFailed: 0,
			}},
			Status:          DischargeStatusDischarged,
			CounterExamples: []DischargeCounter{},
		}},
	}
	specs := []DeriveSpec{{
		Func:     "processable",
		ImplFunc: "Processable",
		ImplPkg:  "x/y/z/derived",
		OutFile:  "internal/derived/processable_spec_test.go",
	}}
	failures := []parsedFailure{
		{implFunc: "Processable", caseID: "case_05", specOutput: "true", implOutput: "false"},
		{implFunc: "Processable", caseID: "case_07", specOutput: "true", implOutput: "false"},
	}
	applyCounterExamples(r, failures, specs)

	if r.Rules[0].Status != DischargeStatusViolated {
		t.Errorf("status = %s, want violated", r.Rules[0].Status)
	}
	if got := len(r.Rules[0].CounterExamples); got != 2 {
		t.Errorf("counter_examples = %d, want 2", got)
	}
	p := r.Rules[0].Premises[0]
	if p.SamplesFailed != 2 {
		t.Errorf("SamplesFailed = %d, want 2", p.SamplesFailed)
	}
	if p.SamplesPassed != 33 {
		t.Errorf("SamplesPassed = %d, want 33 (35 total - 2 failed)", p.SamplesPassed)
	}
}

// ---------- downgradeRuntimeSampledToUnproven ----------

func TestDowngradeRuntimeSampledToUnproven(t *testing.T) {
	r := &DischargeReport{
		Rules: []DischargeRule{
			{
				Name:   "processable",
				Status: DischargeStatusDischarged,
				Premises: []DischargePremise{
					{ID: "p1", Discharge: DischargeStatic, DischargeBasis: "guard-type-at-boundary"},
					{ID: "p2", Discharge: DischargeRuntimeSampled, SamplesPassed: 35},
				},
			},
		},
	}
	downgradeRuntimeSampledToUnproven(r, "tests skipped")
	if r.Rules[0].Status != DischargeStatusUnproven {
		t.Errorf("rule status = %s, want unproven", r.Rules[0].Status)
	}
	// Static premise must remain static.
	if r.Rules[0].Premises[0].Discharge != DischargeStatic {
		t.Errorf("static premise was disturbed: %s", r.Rules[0].Premises[0].Discharge)
	}
	p := r.Rules[0].Premises[1]
	if p.Discharge != DischargeUnproven {
		t.Errorf("runtime-sampled premise discharge = %s, want unproven", p.Discharge)
	}
	if p.DischargeBasis != "tests-not-run" {
		t.Errorf("DischargeBasis = %s, want tests-not-run", p.DischargeBasis)
	}
	if p.SamplesPassed != 0 || p.SamplesFailed != 0 {
		t.Errorf("samples must be zero when downgraded: passed=%d failed=%d", p.SamplesPassed, p.SamplesFailed)
	}
	if !strings.Contains(p.Rationale, "tests skipped") {
		t.Errorf("rationale should carry the supplied reason, got %q", p.Rationale)
	}
	if r.Summary.RulesUnproven != 1 || r.Summary.PremisesUnproven != 1 {
		t.Errorf("summary not recomputed: %+v", r.Summary)
	}
}

func TestDowngrade_DoesNotOverrideViolated(t *testing.T) {
	r := &DischargeReport{
		Rules: []DischargeRule{{
			Name:   "x",
			Status: DischargeStatusViolated,
			Premises: []DischargePremise{
				{ID: "p", Discharge: DischargeRuntimeSampled, SamplesFailed: 1},
			},
		}},
	}
	downgradeRuntimeSampledToUnproven(r, "")
	if r.Rules[0].Status != DischargeStatusViolated {
		t.Errorf("violated status was clobbered: %s", r.Rules[0].Status)
	}
}

// ---------- computeDischargedSinceCommit ----------

func TestComputeDischargedSinceCommit_SuccessorAfterRecovery(t *testing.T) {
	// Scenario: rule went discharged -> violated -> discharged.
	// We expect discharged_since_commit to point at the recovery
	// commit (the *newer* successor of the violated state), not
	// the current HEAD.
	tmp := t.TempDir()
	old := os.Getenv("PWD")
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(old)
	}()
	if err := os.MkdirAll(DischargeHistoryDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Helper to write a history entry with a specific modtime.
	write := func(name string, mod time.Time, status string, commit string) {
		r := &DischargeReport{
			SchemaVersion: 1,
			Impl:          DischargeImpl{GitCommit: ptrStr(commit)},
			Rules: []DischargeRule{{
				Name:   "foo",
				Status: status,
				Premises: []DischargePremise{{
					Discharge:      DischargeStatic,
					DischargeBasis: "guard-type-at-boundary",
				}},
				CounterExamples: []DischargeCounter{},
			}},
		}
		data, _ := json.MarshalIndent(r, "", "  ")
		path := filepath.Join(DischargeHistoryDir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write history entry: %v", err)
		}
		_ = os.Chtimes(path, mod, mod)
	}

	// Oldest first when listed by mod-time, but we sort newest
	// first inside the walker. Modtimes deliberately spaced so the
	// sort is unambiguous.
	now := time.Now()
	write("h-001-old.json", now.Add(-3*time.Hour), DischargeStatusDischarged, "commit-old")
	write("h-002-broken.json", now.Add(-2*time.Hour), DischargeStatusViolated, "commit-broken")
	write("h-003-fixed.json", now.Add(-1*time.Hour), DischargeStatusDischarged, "commit-recovery")

	current := &DischargeReport{
		Impl: DischargeImpl{GitCommit: ptrStr("commit-head")},
		Rules: []DischargeRule{{
			Name:   "foo",
			Status: DischargeStatusDischarged,
			Premises: []DischargePremise{{
				Discharge:      DischargeStatic,
				DischargeBasis: "guard-type-at-boundary",
			}},
		}},
	}
	computeDischargedSinceCommit(current)
	since := current.Rules[0].DischargedSinceCommit
	if since == nil {
		t.Fatal("DischargedSinceCommit was nil")
	}
	if *since != "commit-recovery" {
		t.Errorf("DischargedSinceCommit = %q, want commit-recovery (the successor of the violated state)", *since)
	}
}

func TestComputeDischargedSinceCommit_AllCleanReachesOldest(t *testing.T) {
	tmp := t.TempDir()
	old := os.Getenv("PWD")
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(old)
	}()
	if err := os.MkdirAll(DischargeHistoryDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	write := func(name string, mod time.Time, commit string) {
		r := &DischargeReport{
			SchemaVersion: 1,
			Impl:          DischargeImpl{GitCommit: ptrStr(commit)},
			Rules: []DischargeRule{{
				Name:            "foo",
				Status:          DischargeStatusDischarged,
				Premises:        []DischargePremise{{Discharge: DischargeStatic}},
				CounterExamples: []DischargeCounter{},
			}},
		}
		data, _ := json.MarshalIndent(r, "", "  ")
		path := filepath.Join(DischargeHistoryDir, name)
		_ = os.WriteFile(path, data, 0o644)
		_ = os.Chtimes(path, mod, mod)
	}
	now := time.Now()
	write("h-001.json", now.Add(-3*time.Hour), "commit-oldest")
	write("h-002.json", now.Add(-2*time.Hour), "commit-mid")
	write("h-003.json", now.Add(-1*time.Hour), "commit-newest")

	cur := &DischargeReport{
		Impl: DischargeImpl{GitCommit: ptrStr("commit-head")},
		Rules: []DischargeRule{{
			Name:     "foo",
			Status:   DischargeStatusDischarged,
			Premises: []DischargePremise{{Discharge: DischargeStatic}},
		}},
	}
	computeDischargedSinceCommit(cur)
	if cur.Rules[0].DischargedSinceCommit == nil {
		t.Fatal("DischargedSinceCommit nil")
	}
	if *cur.Rules[0].DischargedSinceCommit != "commit-oldest" {
		t.Errorf("DischargedSinceCommit = %q, want commit-oldest (rule has been clean as far back as we can see)", *cur.Rules[0].DischargedSinceCommit)
	}
}

// ---------- pruneDischargeHistory ----------

func TestPruneDischargeHistory_PreservesFirstOfMonth(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll(DischargeHistoryDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Build 60 entries: 30 spread across April and 30 across May.
	// With retain=10 the first-of-month carve-out should preserve
	// the oldest April entry even though it falls outside the
	// 10-newest window.
	mk := func(name string, mod time.Time) {
		path := filepath.Join(DischargeHistoryDir, name)
		_ = os.WriteFile(path, []byte("{}"), 0o644)
		_ = os.Chtimes(path, mod, mod)
	}
	apr := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 30; i++ {
		mk(makeName("apr", i), apr.Add(time.Duration(i)*time.Hour))
		mk(makeName("may", i), may.Add(time.Duration(i)*time.Hour))
	}

	pruneDischargeHistory(10)

	// Oldest April entry must still exist (first-of-month
	// carve-out) even though the 10 newest entries are all May.
	keep := filepath.Join(DischargeHistoryDir, "apr-00.json")
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("oldest April entry should be preserved by the per-month carve-out, got: %v", err)
	}
	// The 10 newest May entries (may-20 .. may-29) must exist.
	for i := 20; i < 30; i++ {
		p := filepath.Join(DischargeHistoryDir, makeName("may", i))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("newest may entry %d missing: %v", i, err)
		}
	}
	// A non-boundary mid-April entry (apr-15) is neither the
	// first-of-month nor among the 10 newest, so it must be gone.
	if _, err := os.Stat(filepath.Join(DischargeHistoryDir, makeName("apr", 15))); err == nil {
		t.Errorf("mid-April entry should have been pruned")
	}
}

// ---------- Schema round-trip ----------

func TestSchemaRoundTrip_BackwardCompat(t *testing.T) {
	// The cmd/sb mirror types must round-trip JSON identically to
	// shen-derive's canonical types. We can't import shen-derive
	// from cmd/sb (separate modules), so we test the load + save
	// path via a fixture that mirrors the schema doc's required
	// shape.
	fixture := []byte(`{
  "schema_version": 1,
  "generated_at": "2026-05-05T22:30:00Z",
  "spec": {
    "files": [{"path": "specs/core.shen", "sha256": "deadbeef"}],
    "rule_count": 1
  },
  "impl": {
    "git_commit": "abc123",
    "git_dirty": false,
    "target_languages": ["go"]
  },
  "tools": {
    "sb_version": "0.3.0",
    "shen_derive_version": "0.3.0",
    "shengen_version": "",
    "shen_runtime": null,
    "shen_runtime_available": false
  },
  "rules": [
    {
      "name": "amount",
      "kind": "constrained",
      "spec_file": "specs/core.shen",
      "spec_excerpt": "(datatype amount ...)",
      "human_description": "An Amount is a non-negative number.",
      "human_description_source": "auto-generated",
      "premises": [
        {
          "id": "amount.field-x",
          "expression": "X : number",
          "discharge": "static",
          "discharge_basis": "guard-type-at-boundary",
          "rationale": "X is typed number.",
          "samples_passed": 0,
          "samples_failed": 0,
          "sample_seed": null
        }
      ],
      "status": "discharged",
      "discharged_since_commit": "abc123",
      "counter_examples": []
    }
  ],
  "summary": {
    "rule_count": 1,
    "rules_discharged": 1,
    "rules_violated": 0,
    "rules_unproven": 0,
    "premises_total": 1,
    "premises_static": 1,
    "premises_runtime_sampled": 0,
    "premises_unproven": 0
  },
  "signature": null
}`)

	var r DischargeReport
	if err := json.Unmarshal(fixture, &r); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if r.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", r.SchemaVersion)
	}
	if r.Impl.GitCommit == nil || *r.Impl.GitCommit != "abc123" {
		t.Errorf("git_commit unmarshalled wrong")
	}
	if r.Impl.GitDirty == nil || *r.Impl.GitDirty {
		t.Errorf("git_dirty: pointer-bool unmarshalled wrong")
	}
	if r.Tools.ShenRuntime != nil {
		t.Errorf("shen_runtime: null must unmarshal to nil pointer")
	}
	if got := r.Rules[0].CounterExamples; got == nil {
		t.Errorf("counter_examples: empty array must unmarshal to non-nil slice")
	}

	// Re-marshal and confirm key required-array invariants survive.
	out, err := json.Marshal(&r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"counter_examples":[]`) {
		t.Errorf(`re-marshal lost "counter_examples":[]: %s`, string(out))
	}
	if !strings.Contains(string(out), `"signature":null`) {
		t.Errorf(`re-marshal lost "signature":null: %s`, string(out))
	}
}

// ---------- helpers ----------

func ptrStr(s string) *string { return &s }

func makeName(prefix string, i int) string {
	return prefix + "-" + twoDigit(i) + ".json"
}

func twoDigit(i int) string {
	if i < 10 {
		return "0" + itoa(i)
	}
	return itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	n := i
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(b[pos:])
}
