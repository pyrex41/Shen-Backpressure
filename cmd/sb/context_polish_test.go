package main

// context_polish_test.go — tests for the context/audit-report polish
// landed alongside this file:
//   - GateInfo.LastResult populated from .sb/gates_last_run.json when
//     the sidecar exists, and left empty when it doesn't.
//   - sb context --evidence output shape and absent-report behaviour.
//   - the richer DischargeCounter rendering in audit-report.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- GateInfo.LastResult overlay ----------

func TestBuildGateInfos_LastResultPopulatedWhenHistoryExists(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	// Write a gates_last_run.json with two gates: one PASS one FAIL.
	lr := &GatesLastRun{
		SchemaVersion: 1,
		GeneratedAt:   "2026-05-22T12:00:00Z",
		Gates: []GateLastResult{
			{Name: "shengen", Passed: true, ExitCode: 0, DurationMs: 120},
			{Name: "test", Passed: false, ExitCode: 2, DurationMs: 340},
		},
	}
	writeJSONFixture(t, GatesLastRunPath, lr)

	cfg := &Config{
		Gates: []GateDef{
			{Name: "shengen", Kind: GateKindCommand, Run: "true"},
			{Name: "test", Kind: GateKindCommand, Run: "false"},
		},
	}
	gates := buildGateInfos(cfg)
	if len(gates) != 2 {
		t.Fatalf("expected 2 gates, got %d", len(gates))
	}
	if gates[0].LastResult != "pass" {
		t.Errorf("shengen: LastResult = %q, want pass", gates[0].LastResult)
	}
	if gates[0].LastDurationMs != 120 {
		t.Errorf("shengen: LastDurationMs = %d, want 120", gates[0].LastDurationMs)
	}
	if gates[1].LastResult != "fail" {
		t.Errorf("test: LastResult = %q, want fail", gates[1].LastResult)
	}
	if gates[1].LastExitCode != 2 {
		t.Errorf("test: LastExitCode = %d, want 2", gates[1].LastExitCode)
	}
	if gates[1].LastDurationMs != 340 {
		t.Errorf("test: LastDurationMs = %d, want 340", gates[1].LastDurationMs)
	}
}

func TestBuildGateInfos_LastResultAbsentWhenHistoryMissing(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	// No .sb/gates_last_run.json written. buildGateInfos must be
	// silent and leave LastResult empty.
	cfg := &Config{
		Gates: []GateDef{{Name: "shengen", Kind: GateKindCommand, Run: "true"}},
	}
	gates := buildGateInfos(cfg)
	if len(gates) != 1 {
		t.Fatalf("expected 1 gate, got %d", len(gates))
	}
	if gates[0].LastResult != "" {
		t.Errorf("LastResult = %q, want empty (no history)", gates[0].LastResult)
	}
	if gates[0].LastDurationMs != 0 {
		t.Errorf("LastDurationMs = %d, want 0", gates[0].LastDurationMs)
	}
}

func TestBuildGateInfos_LastResultIgnoresMalformedSidecar(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if err := os.MkdirAll(filepath.Dir(GatesLastRunPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a JSON blob that's NOT a GatesLastRun — defensive read
	// must treat it as "no information" rather than crash.
	if err := os.WriteFile(GatesLastRunPath, []byte(`{"hello":"world"}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := &Config{
		Gates: []GateDef{{Name: "shengen", Kind: GateKindCommand, Run: "true"}},
	}
	gates := buildGateInfos(cfg)
	if gates[0].LastResult != "" {
		t.Errorf("malformed sidecar should yield empty LastResult, got %q", gates[0].LastResult)
	}
}

// ---------- Markdown rendering of LastResult ----------

func TestRenderMarkdown_GateRowIncludesLastResultTag(t *testing.T) {
	ctx := &ProjectContext{
		Project: ProjectInfo{Lang: "go", Pkg: "x", Spec: "specs/core.shen"},
		Gates: []GateInfo{
			{Name: "shengen", Kind: "command", LastResult: "pass", LastDurationMs: 150},
			{Name: "test", Kind: "command", LastResult: "fail", LastExitCode: 2, LastDurationMs: 1234},
			{Name: "shen-derive", Kind: "derive"}, // no history yet
		},
	}
	md := ctx.RenderMarkdown()
	if !strings.Contains(md, "1. shengen (command) [PASS") {
		t.Errorf("PASS tag missing for shengen:\n%s", md)
	}
	if !strings.Contains(md, "2. test (command) [FAIL exit=2") {
		t.Errorf("FAIL exit tag missing for test:\n%s", md)
	}
	if !strings.Contains(md, "3. shen-derive (derive) [—]") {
		t.Errorf("em-dash tag missing for shen-derive:\n%s", md)
	}
}

// ---------- Evidence summary ----------

func TestBuildEvidenceSummary_AvailableFalseWhenNoReport(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	s := BuildEvidenceSummary(DischargeReportPath)
	if s.Available {
		t.Errorf("Available should be false when no report exists")
	}
	if s.PremisesTotal != 0 || len(s.Rules) != 0 {
		t.Errorf("expected zero totals and empty rules, got %+v", s)
	}
}

func TestBuildEvidenceSummary_AggregatesByDischargeKind(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	// Write a discharge report with a mix of static + runtime-sampled
	// + unproven premises across two rules.
	r := &DischargeReport{
		SchemaVersion: 1,
		Spec:          DischargeSpec{Files: []DischargeSpecFile{{Path: "x.shen"}}},
		Rules: []DischargeRule{
			{
				Name:   "amount",
				Kind:   "constrained",
				Status: DischargeStatusDischarged,
				Premises: []DischargePremise{
					{ID: "amount.field-x", Discharge: DischargeStatic},
					{ID: "amount.non-neg", Discharge: DischargeStatic},
				},
				CounterExamples: []DischargeCounter{},
			},
			{
				Name:   "processable",
				Kind:   "predicate",
				Status: DischargeStatusUnproven,
				Premises: []DischargePremise{
					{ID: "processable.balance", Discharge: DischargeStatic},
					{ID: "processable.oracle", Discharge: DischargeRuntimeSampled, SamplesPassed: 35},
					{ID: "processable.future", Discharge: DischargeUnproven},
				},
				CounterExamples: []DischargeCounter{},
			},
		},
		Summary: DischargeSummary{
			PremisesStatic:         3,
			PremisesRuntimeSampled: 1,
			PremisesUnproven:       1,
			PremisesTotal:          5,
		},
	}
	writeJSONFixture(t, DischargeReportPath, r)

	s := BuildEvidenceSummary(DischargeReportPath)
	if !s.Available {
		t.Fatal("Available should be true when a report exists")
	}
	if s.PremisesTotal != 5 || s.PremisesStatic != 3 || s.PremisesRuntimeSampled != 1 || s.PremisesUnproven != 1 {
		t.Errorf("totals wrong: %+v", s)
	}
	if len(s.Rules) != 2 {
		t.Fatalf("expected 2 rule rows, got %d", len(s.Rules))
	}
	// Sorted alphabetically.
	if s.Rules[0].Name != "amount" || s.Rules[1].Name != "processable" {
		t.Errorf("rules not sorted by name: %+v", s.Rules)
	}
	if got := s.Rules[1]; got.Static != 1 || got.RuntimeSampled != 1 || got.Unproven != 1 || got.Total != 3 {
		t.Errorf("processable row wrong: %+v", got)
	}
}

func TestRenderEvidenceMarkdown_HasTableHeaderAndTotals(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	r := &DischargeReport{
		SchemaVersion: 1,
		Rules: []DischargeRule{
			{
				Name:   "amount",
				Kind:   "wrapper",
				Status: DischargeStatusDischarged,
				Premises: []DischargePremise{
					{ID: "p1", Discharge: DischargeStatic},
				},
				CounterExamples: []DischargeCounter{},
			},
		},
		Summary: DischargeSummary{PremisesStatic: 1, PremisesTotal: 1},
	}
	writeJSONFixture(t, DischargeReportPath, r)

	md := RenderEvidenceMarkdown(DischargeReportPath)
	if !strings.Contains(md, "## Mixed-Evidence Summary") {
		t.Errorf("missing top heading: %s", md)
	}
	if !strings.Contains(md, "| Rule | Kind | Status | Static | Runtime-sampled | Unproven | Total |") {
		t.Errorf("missing table header: %s", md)
	}
	if !strings.Contains(md, "`amount`") || !strings.Contains(md, "wrapper") {
		t.Errorf("rule row missing: %s", md)
	}
	if !strings.Contains(md, "1 static") {
		t.Errorf("static count missing from totals line: %s", md)
	}
}

func TestRenderEvidenceMarkdown_NoReportEmitsExplanation(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	md := RenderEvidenceMarkdown(DischargeReportPath)
	if !strings.Contains(md, "No discharge report found") {
		t.Errorf("missing absent-report explanation: %s", md)
	}
	if strings.Contains(md, "| Rule |") {
		t.Errorf("should not emit the table header when report missing: %s", md)
	}
}

// ---------- Richer counter-example rendering ----------

func TestRenderCounterExample_RichForm(t *testing.T) {
	var b strings.Builder
	lineHint := 187
	ce := DischargeCounter{
		CaseID: "case_07",
		Input: map[string]string{
			"balance":      "0",
			"transactions": "[{amount: 2.5}]",
		},
		SpecOutput:   "false",
		ImplOutput:   "true",
		ImplFunction: "Processable",
		ImplFile:     "internal/derived/processable.go",
		ImplLineHint: &lineHint,
		Rationale:    "Spec evaluates processable on case_07 to false; impl returned true.",
	}
	renderCounterExample(&b, ce)
	out := b.String()

	// Heading.
	if !strings.Contains(out, "#### Case `case_07`") {
		t.Errorf("missing case heading: %s", out)
	}
	// Multi-line input code block, sorted keys.
	if !strings.Contains(out, "Input:\n\n```\nbalance = 0\ntransactions = [{amount: 2.5}]\n```") {
		t.Errorf("multi-line input block missing or unsorted: %s", out)
	}
	// Spec output fenced block.
	if !strings.Contains(out, "Spec output (Shen oracle):\n\n```shen\nfalse\n```") {
		t.Errorf("spec output block missing: %s", out)
	}
	// Impl output fenced block with function name.
	if !strings.Contains(out, "Impl output (`Processable`):\n\n```\ntrue\n```") {
		t.Errorf("impl output block missing function name: %s", out)
	}
	// File reference with line hint. The path lives inside a
	// backtick span, with the line number appearing immediately
	// after the closing backtick (`path`:NN) so a Markdown renderer
	// keeps the path itself in a code style.
	if !strings.Contains(out, "`internal/derived/processable.go`:187") {
		t.Errorf("file:line hint missing: %s", out)
	}
	// Reproducer in its own fenced block.
	if !strings.Contains(out, "Reproduce:\n\n```sh\ngo test -run 'TestSpec_Processable/case_07' -v\n```") {
		t.Errorf("reproducer block missing: %s", out)
	}
	// Rationale as blockquote.
	if !strings.Contains(out, "> Spec evaluates processable on case_07 to false; impl returned true.") {
		t.Errorf("rationale blockquote missing: %s", out)
	}
}

func TestRenderCounterExample_HandlesMinimalShape(t *testing.T) {
	// No input map, no impl function, no rationale — the renderer
	// must still produce a coherent (if terse) block without panicking
	// or emitting empty fenced regions for required pieces.
	var b strings.Builder
	ce := DischargeCounter{
		CaseID:     "case_00",
		SpecOutput: "1",
		ImplOutput: "2",
	}
	renderCounterExample(&b, ce)
	out := b.String()
	if !strings.Contains(out, "#### Case `case_00`") {
		t.Errorf("heading missing: %s", out)
	}
	if strings.Contains(out, "Input:") {
		t.Errorf("Input section should be suppressed when map empty: %s", out)
	}
	if strings.Contains(out, "Reproduce:") {
		t.Errorf("Reproduce section should be suppressed without impl function: %s", out)
	}
	if !strings.Contains(out, "target implementation") {
		t.Errorf("impl block should fall back to generic label: %s", out)
	}
}

// ---------- helpers ----------

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}

func writeJSONFixture(t *testing.T, relPath string, v interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(relPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(relPath), err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(relPath, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}
