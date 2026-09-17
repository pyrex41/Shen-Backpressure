package main

// assess.go implements an advisory judgment boundary. JEV may prioritize a
// bounded investigation, but its output is deliberately stored as an
// Assessment and never merged into the discharge report or treated as proof.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultJEVBaseURL = "https://api.typesafe.ai"
	defaultJEVModel   = "jev-latest"
	assessmentDir     = ".sb/assessments"
)

// InvestigationOption is one unresolved obligation or failed deterministic
// gate that JEV may select for further investigation.
type InvestigationOption struct {
	ID              string   `json:"id"`
	Claim           string   `json:"claim"`
	Status          string   `json:"status"`
	Evidence        []string `json:"evidence,omitempty"`
	CandidateChecks []string `json:"candidate_checks,omitempty"`
}

type jevChoiceQuestion struct {
	Type         string         `json:"type"`
	Instructions any            `json:"instructions"`
	Criteria     map[string]any `json:"criteria"`
}

type jevNoulQuestion struct {
	Type         string         `json:"type"`
	Instructions any            `json:"instructions"`
	Criteria     map[string]any `json:"criteria,omitempty"`
}

type jevRequest struct {
	State     any            `json:"state"`
	Questions map[string]any `json:"questions"`
	Model     string         `json:"model"`
}

type jevChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type jevNoulAnswer struct {
	Type string  `json:"type"`
	Noul float64 `json:"noul"`
}

type jevResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// AssessmentRecord is replayable provenance for a model judgment. Authority
// is a fixed wire-level reminder that this record cannot satisfy a premise.
type AssessmentRecord struct {
	SchemaVersion int                   `json:"schema_version"`
	Kind          string                `json:"kind"`
	Authority     string                `json:"authority"`
	CreatedAt     string                `json:"created_at"`
	RequestHash   string                `json:"request_hash"`
	ArtifactHash  string                `json:"artifact_hash"`
	SpecHash      string                `json:"spec_hash,omitempty"`
	LatencyMS     int64                 `json:"latency_ms"`
	Request       jevRequest            `json:"request"`
	Response      jevResponse           `json:"response"`
	Priority      jevChoiceAnswer       `json:"priority"`
	Category      jevChoiceAnswer       `json:"category"`
	Investigate   jevNoulAnswer         `json:"investigate"`
	Options       []InvestigationOption `json:"options"`
}

func cmdAssess(args []string) {
	fs := flag.NewFlagSet("assess", flag.ExitOnError)
	diagnostic := fs.String("diagnostic", "", "diagnostic or failure text to route")
	diagnosticFile := fs.String("diagnostic-file", "", "read diagnostic text from a file ('-' for stdin)")
	model := fs.String("model", defaultJEVModel, "JEV model")
	baseURL := fs.String("base-url", envOr("JEV_BASE_URL", defaultJEVBaseURL), "TypeSafe API base URL")
	replay := fs.String("replay", "", "replay a recorded assessment without calling JEV")
	noCache := fs.Bool("no-cache", false, "ignore an assessment with the same complete input hash")
	jsonOut := fs.Bool("json", false, "emit the full assessment record as JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `sb assess — Prioritize a bounded investigation with JEV

Usage:
  sb assess --diagnostic "test failure text"
  sb assess --diagnostic-file failure.log
  sb assess --replay .sb/assessments/<hash>.json

The result is an advisory Assessment, not CheckedEvidence. It can reorder or
add investigation, but cannot skip gates or discharge an obligation. API keys
are read from JEV_API_KEY (preferred) or TYPESAFE_API_KEY.`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if *replay != "" {
		record, err := readAssessment(*replay)
		if err != nil {
			assessFatal(err)
		}
		emitAssessment(record, *jsonOut, true)
		return
	}

	text, err := loadDiagnostic(*diagnostic, *diagnosticFile)
	if err != nil {
		assessFatal(err)
	}
	if strings.TrimSpace(text) == "" {
		assessFatal(errors.New("provide --diagnostic or --diagnostic-file"))
	}

	cfg, err := LoadConfig()
	if err != nil {
		assessFatal(err)
	}
	options := buildInvestigationFrontier(cfg)
	artifactHash := currentArtifactHash()
	specHash := fileSHA256(cfg.Spec)
	request := buildJEVRequest(text, cfg, options, *model, artifactHash, specHash)
	requestHash, err := hashJSON(request)
	if err != nil {
		assessFatal(err)
	}
	cachePath := filepath.Join(assessmentDir, requestHash+".json")
	if !*noCache {
		if record, err := readAssessment(cachePath); err == nil {
			emitAssessment(record, *jsonOut, true)
			return
		} else if !os.IsNotExist(err) {
			assessFatal(err)
		}
	}

	apiKey := strings.TrimSpace(os.Getenv("JEV_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
	}
	if apiKey == "" {
		assessFatal(errors.New("JEV_API_KEY is not set"))
	}
	started := time.Now()
	response, err := callJEV(context.Background(), http.DefaultClient, strings.TrimRight(*baseURL, "/"), apiKey, request)
	latency := time.Since(started)
	if err != nil {
		assessFatal(err)
	}
	record, err := newAssessmentRecord(requestHash, artifactHash, specHash, latency, request, response, options)
	if err != nil {
		assessFatal(err)
	}
	if err := writeAssessment(cachePath, record); err != nil {
		assessFatal(err)
	}
	emitAssessment(record, *jsonOut, false)
}

func loadDiagnostic(inline, path string) (string, error) {
	if inline != "" && path != "" {
		return "", errors.New("use only one of --diagnostic and --diagnostic-file")
	}
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	if path != "" {
		b, err := os.ReadFile(path)
		return string(b), err
	}
	return inline, nil
}

func buildInvestigationFrontier(cfg *Config) []InvestigationOption {
	var out []InvestigationOption
	if report, _ := loadDischarge(DischargeReportPath); report != nil {
		for _, rule := range report.Rules {
			if rule.Status == DischargeStatusDischarged {
				continue
			}
			for _, premise := range rule.Premises {
				if premise.Discharge != DischargeUnproven && premise.SamplesFailed == 0 {
					continue
				}
				evidence := append([]string(nil), premise.CodeReferences...)
				if premise.Rationale != "" {
					evidence = append(evidence, premise.Rationale)
				}
				out = append(out, InvestigationOption{
					ID: "obligation:" + premise.ID, Claim: premise.Expression,
					Status: rule.Status, Evidence: evidence,
					CandidateChecks: []string{"inspect referenced implementation", "run or add an obligation-specific test"},
				})
			}
		}
	}
	// All declared checks remain candidates. A previously passing check can be
	// exactly the right discriminator for a new diagnostic; filtering to only
	// failed gates would hide shen-derive whenever an unrelated gate failed.
	for _, gate := range buildGateInfos(cfg) {
		status := gate.LastResult
		if status == "" {
			status = "not-run"
		}
		evidence := []string(nil)
		if gate.LastResult != "" {
			evidence = append(evidence, fmt.Sprintf("last result: %s", gate.LastResult))
		}
		if gate.LastResult == "fail" {
			evidence = append(evidence, fmt.Sprintf("last exit code: %d", gate.LastExitCode))
		}
		claim, checks := describeGateInvestigation(gate, cfg)
		out = append(out, InvestigationOption{
			ID: "gate:" + gate.Name, Claim: claim,
			Status: status, Evidence: evidence, CandidateChecks: checks,
		})
	}
	// A diagnostic can expose a verification gap that is absent from the
	// current report (for example, tenant scoping hidden behind a trusted DB
	// adapter). Keep that possibility bounded and explicit instead of forcing
	// the judge to mislabel it as one of the existing gates.
	out = append(out, InvestigationOption{
		ID:     "targeted:new-check",
		Claim:  "none of the existing declared checks can produce relevant evidence; design a new obligation-specific check or counterexample",
		Status: "candidate",
		CandidateChecks: []string{
			"construct the smallest deterministic reproduction from the diagnostic",
			"add a focused test or analyzer check without weakening existing gates",
		},
	})
	if len(out) == 1 {
		out = append(out, InvestigationOption{ID: "general", Claim: "investigate outside the known obligation map", Status: "fallback"})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func describeGateInvestigation(gate GateInfo, cfg *Config) (string, []string) {
	claim := "run or inspect deterministic gate: " + gate.Name
	checks := []string{}
	if gate.Run != "" {
		checks = append(checks, gate.Run)
	}
	name := strings.ToLower(gate.Name)
	switch {
	case strings.Contains(name, "derive"):
		claim = "compare hand-written implementation behavior against the Shen oracle on deterministic boundary samples"
		if len(cfg.DeriveSpecs) > 0 {
			covered := make([]string, 0, len(cfg.DeriveSpecs))
			for _, spec := range cfg.DeriveSpecs {
				covered = append(covered, spec.Func+" -> "+spec.ImplFunc)
			}
			claim += "; configured coverage is limited to: " + strings.Join(covered, ", ")
		}
		checks = append(checks, "run generated spec-equivalence tests and inspect concrete counterexamples")
	case strings.Contains(name, "parity"):
		claim = "compare equivalent implementations across target languages using shared fixtures"
		checks = append(checks, "run pairwise behavioral output comparison")
	case strings.Contains(name, "tcb-audit"):
		claim = "check generated guard artifacts for drift or trusted-boundary tampering"
	case strings.Contains(name, "shen-check"):
		claim = "type-check the Shen specification for internal consistency using the configured Shen runtime"
	case strings.Contains(name, "shengen"):
		claim = "regenerate structural guard types from the Shen specification"
	case strings.Contains(name, "test"):
		claim = "run the project's deterministic test suite"
	case strings.Contains(name, "build"):
		claim = "compile or type-check the target-language implementation"
	case strings.Contains(name, "cedar") || strings.Contains(name, "rego") || strings.Contains(name, "decidable"):
		claim = "regenerate and validate runtime policy artifacts against the Shen specification"
	}
	return claim, checks
}

func buildJEVRequest(diagnostic string, cfg *Config, options []InvestigationOption, model, artifactHash, specHash string) jevRequest {
	criteria := make(map[string]any, len(options))
	for _, option := range options {
		criteria[option.ID] = option
	}
	routingGuidance := map[string]any{
		"decision_rules": []string{
			"Match the diagnostic to the evidence a check actually produces, including its configured scope.",
			"A previously passing gate remains useful when it is the best discriminator for a new diagnostic.",
			"A previously failed gate is not automatically relevant; distinguish the observed failure from an unrelated known failure.",
			"Choose targeted:new-check only when every existing check is out of scope for the claim that needs evidence.",
			"Route investigation only. Never interpret a model choice or probability as proof that a requirement holds.",
		},
		"examples": []map[string]any{
			{
				"diagnostic": "A covered function disagrees with its Shen specification on a fractional or boundary input, while compilation succeeds.",
				"choose":     "gate:shen-derive, but only when its configured coverage names that function",
				"because":    "spec-equivalence sampling can produce a concrete behavioral counterexample",
			},
			{
				"diagnostic": "An authorization data adapter drops a required scope key, while the configured behavioral oracle covers only an unrelated identity predicate.",
				"choose":     "targeted:new-check",
				"because":    "none of the declared checks establishes scope isolation in that adapter",
			},
			{
				"diagnostic": "One language produces different output from the other implementations on a shared fixture.",
				"choose":     "the declared parity gate",
				"because":    "pairwise behavioral comparison directly reproduces the discrepancy",
			},
			{
				"diagnostic": "A configured command fails before meaningful execution while independent application tests and compilation succeed.",
				"choose":     "the affected command gate; category environment_or_tooling",
				"because":    "failure before execution indicates launch or environment trouble rather than application behavior",
			},
		},
	}
	return jevRequest{
		Model: model,
		State: map[string]any{
			"diagnostic": diagnostic, "project_language": cfg.Lang, "spec_path": cfg.Spec,
			"artifact_hash": artifactHash, "spec_hash": specHash, "obligation_frontier": options,
			"policy":           map[string]any{"assessment_is_advisory": true, "mandatory_checks_may_not_be_suppressed": true},
			"routing_guidance": routingGuidance,
		},
		Questions: map[string]any{
			"priority": jevChoiceQuestion{Type: "choice", Instructions: "Which single option should be investigated first to gain useful evidence for this diagnostic? Prefer an existing deterministic gate whenever its described evidence semantics cover the diagnostic. Choose targeted:new-check only when no existing declared check can produce the relevant evidence.", Criteria: criteria},
			"category": jevChoiceQuestion{Type: "choice", Instructions: "Which known failure category best fits the immediate cause described by this diagnostic? Classify command launch, missing executable, runtime incompatibility, or near-instant exit as environment_or_tooling; classify incorrect application behavior as implementation_defect; use spec_or_codegen_drift only when specification or generated artifacts are actually out of sync.", Criteria: map[string]any{
				"implementation_defect": "code behavior conflicts with the intended rule", "spec_or_codegen_drift": "generated artifacts or specs are out of sync",
				"verification_gap": "the relevant obligation lacks an applicable check", "environment_or_tooling": "the failure is primarily setup or tooling", "ambiguous_requirement": "multiple plausible interpretations require clarification",
			}},
			"investigate": jevNoulQuestion{Type: "noul", Instructions: "Is targeted investigation beyond rerunning already-mandatory deterministic checks likely to produce useful evidence?", Criteria: map[string]any{"true": "additional bounded investigation is useful", "false": "mandatory deterministic checks are the best next step"}},
		},
	}
}

func callJEV(ctx context.Context, client *http.Client, baseURL, apiKey string, request jevRequest) (jevResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return jevResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return jevResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return jevResponse{}, fmt.Errorf("JEV request: %w", err)
	}
	defer res.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return jevResponse{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return jevResponse{}, fmt.Errorf("JEV HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var response jevResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return jevResponse{}, fmt.Errorf("decode JEV response: %w", err)
	}
	return response, nil
}

func newAssessmentRecord(requestHash, artifactHash, specHash string, latency time.Duration, request jevRequest, response jevResponse, options []InvestigationOption) (*AssessmentRecord, error) {
	r := &AssessmentRecord{SchemaVersion: 1, Kind: "assessment", Authority: "advisory-only; does-not-discharge-evidence", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), RequestHash: requestHash, ArtifactHash: artifactHash, SpecHash: specHash, LatencyMS: latency.Milliseconds(), Request: request, Response: response, Options: options}
	if err := json.Unmarshal(response.Answers["priority"], &r.Priority); err != nil {
		return nil, fmt.Errorf("decode priority answer: %w", err)
	}
	if err := json.Unmarshal(response.Answers["category"], &r.Category); err != nil {
		return nil, fmt.Errorf("decode category answer: %w", err)
	}
	if err := json.Unmarshal(response.Answers["investigate"], &r.Investigate); err != nil {
		return nil, fmt.Errorf("decode investigate answer: %w", err)
	}
	return r, nil
}

func emitAssessment(record *AssessmentRecord, asJSON, replayed bool) {
	if asJSON {
		b, _ := json.MarshalIndent(record, "", "  ")
		fmt.Println(string(b))
		return
	}
	source := "live"
	if replayed {
		source = "replay/cache"
	}
	fmt.Printf("Assessment (%s, advisory only)\n", source)
	fmt.Printf("Priority: %s (confidence %.3f)\n", record.Priority.Choice, record.Priority.Confidence)
	fmt.Printf("Category: %s (confidence %.3f)\n", record.Category.Choice, record.Category.Confidence)
	fmt.Printf("Additional investigation useful: %.3f\n", record.Investigate.Noul)
	fmt.Printf("Record: %s/%s.json\n", assessmentDir, record.RequestHash)
	fmt.Println("Acceptance unchanged: run all mandatory gates; this assessment is not evidence.")
}

func readAssessment(path string) (*AssessmentRecord, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r AssessmentRecord
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
func writeAssessment(path string, r *AssessmentRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
func hashJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func fileSHA256(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func currentArtifactHash() string {
	cmd := exec.Command("git", "diff", "--no-ext-diff", "HEAD", "--")
	b, err := cmd.Output()
	if err != nil {
		b = nil
	}
	head, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	state := append(head, b...)
	sum := sha256.Sum256(state)
	return hex.EncodeToString(sum[:])
}
func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
func assessFatal(err error) { fmt.Fprintf(os.Stderr, "sb assess: %v\n", err); os.Exit(1) }
