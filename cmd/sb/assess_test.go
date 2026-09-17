package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildJEVRequestPreservesAdvisoryBoundary(t *testing.T) {
	options := []InvestigationOption{
		{ID: "obligation:tenant", Claim: "membership is tenant-scoped", Status: "unproven"},
		{ID: "gate:test", Claim: "tests pass", Status: "failed"},
	}
	req := buildJEVRequest("cross-tenant test failed", &Config{Lang: "go", Spec: "specs/core.shen"}, options, "jev-latest", "artifact", "spec")
	state := req.State.(map[string]any)
	policy := state["policy"].(map[string]any)
	if policy["assessment_is_advisory"] != true || policy["mandatory_checks_may_not_be_suppressed"] != true {
		t.Fatalf("missing safety policy in JEV state: %#v", policy)
	}
	guidance := state["routing_guidance"].(map[string]any)
	if len(guidance["decision_rules"].([]string)) < 4 || len(guidance["examples"].([]map[string]any)) < 4 {
		t.Fatalf("routing guidance is incomplete: %#v", guidance)
	}
	priority := req.Questions["priority"].(jevChoiceQuestion)
	if len(priority.Criteria) != 2 {
		t.Fatalf("priority criteria = %d, want 2", len(priority.Criteria))
	}
}

func TestBuildInvestigationFrontierAlwaysAllowsNewTargetedCheck(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	cfg := &Config{Gates: []GateDef{{Name: "test", Kind: GateKindCommand, Run: "go test ./..."}}}
	frontier := buildInvestigationFrontier(cfg)
	found := false
	for _, option := range frontier {
		if option.ID == "targeted:new-check" {
			found = true
		}
	}
	if !found {
		t.Fatalf("targeted:new-check missing from frontier: %#v", frontier)
	}
}

func TestBuildInvestigationFrontierKeepsPassingChecksEligible(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	writeJSONFixture(t, GatesLastRunPath, &GatesLastRun{
		SchemaVersion: 1,
		Gates: []GateLastResult{
			{Name: "shen-check", Passed: false, ExitCode: 1},
			{Name: "shen-derive", Passed: true},
		},
	})
	cfg := &Config{Gates: []GateDef{
		{Name: "shen-check", Kind: GateKindCommand, Run: "check"},
		{Name: "shen-derive", Kind: GateKindCommand, Run: "derive"},
	}}
	frontier := buildInvestigationFrontier(cfg)
	statuses := map[string]string{}
	for _, option := range frontier {
		statuses[option.ID] = option.Status
	}
	if statuses["gate:shen-check"] != "fail" || statuses["gate:shen-derive"] != "pass" {
		t.Fatalf("all checks should remain eligible with last status preserved: %#v", statuses)
	}
}

func TestDescribeGateInvestigationExplainsDeriveSemantics(t *testing.T) {
	cfg := &Config{DeriveSpecs: []DeriveSpec{{Func: "processable", ImplFunc: "Processable"}}}
	claim, checks := describeGateInvestigation(GateInfo{Name: "shen-derive", Kind: "derive"}, cfg)
	if !strings.Contains(claim, "Shen oracle") || !strings.Contains(claim, "processable -> Processable") || len(checks) == 0 {
		t.Fatalf("derive candidate lacks useful semantics: claim=%q checks=%#v", claim, checks)
	}
}

func TestCallJEVAndConstructAssessment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		var req jevRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
          "model":"jev-latest",
          "answers":{
            "priority":{"type":"choice","choice":"gate:test","confidence":0.8,"probabilities":{"gate:test":0.8,"general":0.2}},
            "category":{"type":"choice","choice":"implementation_defect","confidence":0.7,"probabilities":{"implementation_defect":0.7,"verification_gap":0.3}},
            "investigate":{"type":"noul","noul":0.9}
          },
          "usage":{"input_tokens":10,"output_tokens":3}
        }`))
	}))
	defer server.Close()

	req := jevRequest{Model: "jev-latest", State: "failure", Questions: map[string]any{"x": "y"}}
	response, err := callJEV(context.Background(), server.Client(), server.URL, "secret", req)
	if err != nil {
		t.Fatal(err)
	}
	record, err := newAssessmentRecord("request", "artifact", "spec", 125*time.Millisecond, req, response, nil)
	if err != nil {
		t.Fatal(err)
	}
	if record.Authority != "advisory-only; does-not-discharge-evidence" {
		t.Fatalf("authority = %q", record.Authority)
	}
	if record.Priority.Choice != "gate:test" || record.Investigate.Noul != 0.9 {
		t.Fatalf("unexpected decoded answers: %#v", record)
	}
	if record.LatencyMS != 125 {
		t.Fatalf("latency_ms = %d, want 125", record.LatencyMS)
	}
}

func TestAssessmentRecordRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "assessment.json")
	want := &AssessmentRecord{SchemaVersion: 1, Kind: "assessment", Authority: "advisory-only; does-not-discharge-evidence", RequestHash: "abc"}
	if err := writeAssessment(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	got, err := readAssessment(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestHash != want.RequestHash || got.Authority != want.Authority {
		t.Fatalf("round trip = %#v", got)
	}
}
