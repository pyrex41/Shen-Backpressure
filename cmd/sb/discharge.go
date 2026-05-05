package main

// discharge.go — discharge-report aggregation, history, and Markdown
// rendering for sb. The JSON schema here mirrors shen-derive's
// shen-derive/report/schema.go exactly so both modules read and write
// the same wire format. Cross-module Go imports are avoided because
// cmd/sb and shen-derive are separate Go modules.
//
// Wave 4 design: thoughts/shared/research/2026-05-05-discharge-report-schema.md.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DischargeReportPath is the project-relative path that sb writes the
// current discharge report to. Local-only: gitignored so each clone
// has its own per-iteration history.
const DischargeReportPath = ".sb/discharge_report.json"

// DischargeHistoryDir holds time-stamped copies of each successful
// run's report. Used to compute `discharged_since_commit` and the
// audit "verified continuously since X" claim.
const DischargeHistoryDir = ".sb/history"

// DischargeHistoryRetention is how many recent history entries to
// keep. Reports older than this are deleted on each run. The default
// is intentionally generous: 50 entries plus the first per month
// (roughly two months of intensive iteration).
const DischargeHistoryRetention = 50

// DischargeReport mirrors shen-derive/report/schema.go. Fields
// unspecified in v0 (signature) are kept null. Field order matches
// the schema doc; encoding/json preserves struct field order on
// marshal.
type DischargeReport struct {
	SchemaVersion int                  `json:"schema_version"`
	GeneratedAt   string               `json:"generated_at"`
	Spec          DischargeSpec        `json:"spec"`
	Impl          DischargeImpl        `json:"impl"`
	Tools         DischargeTools       `json:"tools"`
	Rules         []DischargeRule      `json:"rules"`
	Summary       DischargeSummary     `json:"summary"`
	Signature     *DischargeSignature  `json:"signature"`
}

type DischargeSpec struct {
	Files     []DischargeSpecFile `json:"files"`
	RuleCount int                 `json:"rule_count"`
}

type DischargeSpecFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type DischargeImpl struct {
	GitCommit       *string  `json:"git_commit"`
	GitDirty        *bool    `json:"git_dirty"`
	TargetLanguages []string `json:"target_languages"`
}

type DischargeTools struct {
	SBVersion            string  `json:"sb_version"`
	ShenDeriveVersion    string  `json:"shen_derive_version"`
	ShengenVersion       string  `json:"shengen_version"`
	ShenRuntime          *string `json:"shen_runtime"`
	ShenRuntimeAvailable bool    `json:"shen_runtime_available"`
}

type DischargeRule struct {
	Name                   string                  `json:"name"`
	Kind                   string                  `json:"kind"`
	SpecFile               string                  `json:"spec_file"`
	SpecExcerpt            string                  `json:"spec_excerpt"`
	HumanDescription       string                  `json:"human_description"`
	HumanDescriptionSource string                  `json:"human_description_source"`
	Premises               []DischargePremise      `json:"premises"`
	Status                 string                  `json:"status"`
	DischargedSinceCommit  *string                 `json:"discharged_since_commit"`
	CounterExamples        []DischargeCounter      `json:"counter_examples"`
}

type DischargePremise struct {
	ID             string   `json:"id"`
	Expression     string   `json:"expression"`
	Discharge      string   `json:"discharge"`
	DischargeBasis string   `json:"discharge_basis"`
	Rationale      string   `json:"rationale"`
	CodeReferences []string `json:"code_references,omitempty"`
	SamplesPassed  int      `json:"samples_passed"`
	SamplesFailed  int      `json:"samples_failed"`
	SampleSeed     *string  `json:"sample_seed"`
}

type DischargeCounter struct {
	CaseID          string            `json:"case_id"`
	Input           map[string]string `json:"input"`
	SpecOutput      string            `json:"spec_output"`
	ImplOutput      string            `json:"impl_output"`
	ImplFunction    string            `json:"impl_function"`
	ImplFile        string            `json:"impl_file,omitempty"`
	ImplLineHint    *int              `json:"impl_line_hint"`
	FirstSeenCommit *string           `json:"first_seen_commit"`
	Rationale       string            `json:"rationale"`
}

type DischargeSummary struct {
	RuleCount              int `json:"rule_count"`
	RulesDischarged        int `json:"rules_discharged"`
	RulesViolated          int `json:"rules_violated"`
	RulesUnproven          int `json:"rules_unproven"`
	PremisesTotal          int `json:"premises_total"`
	PremisesStatic         int `json:"premises_static"`
	PremisesRuntimeSampled int `json:"premises_runtime_sampled"`
	PremisesUnproven       int `json:"premises_unproven"`
}

type DischargeSignature struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Value     string `json:"value"`
}

// Status constants mirror the schema doc.
const (
	DischargeStatusDischarged = "discharged"
	DischargeStatusViolated   = "violated"
	DischargeStatusUnproven   = "unproven"

	DischargeStatic         = "static"
	DischargeRuntimeSampled = "runtime-sample"
	DischargeUnproven       = "unproven"
)

// loadDischarge reads a discharge report from path. Returns nil and
// no error when the file does not exist (consumers should treat that
// as "no report yet").
func loadDischarge(path string) (*DischargeReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var r DischargeReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}

// writeDischarge atomically writes r as pretty-printed JSON to path.
func writeDischarge(path string, r *DischargeReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".discharge-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// mergeDischargeReports combines per-spec partial reports into one
// project-level report. The first report's project metadata (spec
// files, target languages, tools) is retained; subsequent reports'
// rules and spec files are appended. Rules with the same name are
// kept in order of first appearance — sb derive emits one spec at a
// time so collisions across reports are not expected today.
func mergeDischargeReports(parts []*DischargeReport) *DischargeReport {
	if len(parts) == 0 {
		return &DischargeReport{
			SchemaVersion: 1,
			Spec:          DischargeSpec{Files: []DischargeSpecFile{}},
			Impl:          DischargeImpl{TargetLanguages: []string{}},
			Rules:         []DischargeRule{},
		}
	}
	out := *parts[0]
	out.Spec.Files = append([]DischargeSpecFile(nil), parts[0].Spec.Files...)
	out.Rules = append([]DischargeRule(nil), parts[0].Rules...)
	seenSpec := map[string]bool{}
	for _, sf := range out.Spec.Files {
		seenSpec[sf.Path] = true
	}
	seenRule := map[string]bool{}
	for _, r := range out.Rules {
		seenRule[r.Name] = true
	}
	for _, p := range parts[1:] {
		for _, sf := range p.Spec.Files {
			if !seenSpec[sf.Path] {
				out.Spec.Files = append(out.Spec.Files, sf)
				seenSpec[sf.Path] = true
			}
		}
		for _, r := range p.Rules {
			if !seenRule[r.Name] {
				out.Rules = append(out.Rules, r)
				seenRule[r.Name] = true
			}
		}
	}
	out.Spec.RuleCount = len(out.Rules)
	sort.SliceStable(out.Rules, func(i, j int) bool {
		return out.Rules[i].Name < out.Rules[j].Name
	})
	out.Summary = computeDischargeSummary(out.Rules)
	return &out
}

func computeDischargeSummary(rules []DischargeRule) DischargeSummary {
	s := DischargeSummary{RuleCount: len(rules)}
	for _, r := range rules {
		switch r.Status {
		case DischargeStatusDischarged:
			s.RulesDischarged++
		case DischargeStatusViolated:
			s.RulesViolated++
		case DischargeStatusUnproven:
			s.RulesUnproven++
		}
		for _, p := range r.Premises {
			s.PremisesTotal++
			switch p.Discharge {
			case DischargeStatic:
				s.PremisesStatic++
			case DischargeRuntimeSampled:
				s.PremisesRuntimeSampled++
			case DischargeUnproven:
				s.PremisesUnproven++
			}
		}
	}
	return s
}

// fillImplGit fetches `git rev-parse HEAD` and `git status --porcelain`
// in the current working directory and writes them into the report's
// impl section. Errors are non-fatal: a project not in git is
// reported with git_commit=null. The function is idempotent.
func fillImplGit(r *DischargeReport) {
	if commit, ok := gitHeadCommit(); ok {
		r.Impl.GitCommit = &commit
		dirty := gitWorkingDirDirty()
		r.Impl.GitDirty = &dirty
	}
}

func gitHeadCommit() (string, bool) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "", false
	}
	return commit, true
}

func gitWorkingDirDirty() bool {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// goTestFailureRE matches the format that shen-derive's emitted tests
// use:  case_NN: spec says X, impl returned Y
// The full lines from `go test` look like:
//   --- FAIL: TestSpec_Processable/case_07 (0.00s)
//       processable_spec_test.go:120: case_07: spec says true, impl returned false
var goTestFailureRE = regexp.MustCompile(`(case_\d+):\s+spec says\s+(.+?),\s+impl returned\s+(.+)$`)

// goTestSubtestRunRE matches the test runner's "=== RUN
// TestSpec_<Func>/<case>" header so we can pin a counter-example to
// the right impl function name. Using === RUN (not --- FAIL) is
// necessary because go test prints the failure body line *between*
// the RUN and FAIL summaries.
var goTestSubtestRunRE = regexp.MustCompile(`===\s+RUN\s+TestSpec_(\w+)/(case_\d+)`)

// parseGoTestFailures scans go-test combined stdout/stderr for
// counter-example lines and returns a map keyed by
// "ImplFunc/case_NN" -> (specOutput, implOutput).
type parsedFailure struct {
	implFunc   string
	caseID     string
	specOutput string
	implOutput string
}

func parseGoTestFailures(output string) []parsedFailure {
	var out []parsedFailure
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	currentImplFunc := ""
	currentCase := ""
	for scanner.Scan() {
		line := scanner.Text()
		if m := goTestSubtestRunRE.FindStringSubmatch(line); m != nil {
			currentImplFunc = m[1]
			currentCase = m[2]
			continue
		}
		if m := goTestFailureRE.FindStringSubmatch(line); m != nil && currentImplFunc != "" {
			caseID := m[1]
			if currentCase != "" {
				caseID = currentCase
			}
			out = append(out, parsedFailure{
				implFunc:   currentImplFunc,
				caseID:     caseID,
				specOutput: strings.TrimSpace(m[2]),
				implOutput: strings.TrimSpace(m[3]),
			})
			// Don't reset currentImplFunc/currentCase — go test
			// prints subsequent === RUN lines for siblings, and the
			// failure body of the current case can only appear once.
		}
	}
	return out
}

// applyCounterExamples mutates the report by attaching counter-example
// witnesses for each parsedFailure to the matching rule. Rules with at
// least one counter-example are flipped to status "violated".
func applyCounterExamples(r *DischargeReport, failures []parsedFailure, specs []DeriveSpec) {
	if len(failures) == 0 {
		return
	}
	implFuncToRule := map[string]string{}
	implFuncToFile := map[string]string{}
	for _, s := range specs {
		implFuncToRule[s.ImplFunc] = s.Func
		implFuncToFile[s.ImplFunc] = guessImplFile(s)
	}
	for _, f := range failures {
		ruleName, ok := implFuncToRule[f.implFunc]
		if !ok {
			continue
		}
		ce := DischargeCounter{
			CaseID:       f.caseID,
			Input:        map[string]string{"_": "see " + f.implFunc + "/" + f.caseID + " in generated test file"},
			SpecOutput:   f.specOutput,
			ImplOutput:   f.implOutput,
			ImplFunction: f.implFunc,
			ImplFile:     implFuncToFile[f.implFunc],
			Rationale:    fmt.Sprintf("Spec evaluates %s on case %s to %s; impl returned %s.", ruleName, f.caseID, f.specOutput, f.implOutput),
		}
		for i := range r.Rules {
			if r.Rules[i].Name != ruleName {
				continue
			}
			r.Rules[i].Status = DischargeStatusViolated
			r.Rules[i].CounterExamples = append(r.Rules[i].CounterExamples, ce)
			// Bump the runtime-sampled premise's failed count.
			for j := range r.Rules[i].Premises {
				if r.Rules[i].Premises[j].Discharge == DischargeRuntimeSampled {
					r.Rules[i].Premises[j].SamplesFailed++
					if r.Rules[i].Premises[j].SamplesPassed > 0 {
						r.Rules[i].Premises[j].SamplesPassed--
					}
				}
			}
		}
	}
	r.Summary = computeDischargeSummary(r.Rules)
}

// guessImplFile produces the most likely repo-relative impl source
// path for a derive spec. Heuristic: `<impl-pkg-rel>/<lowerImplFunc>.go`.
// Used only as a hint in counter_examples; not relied on for
// correctness.
func guessImplFile(s DeriveSpec) string {
	idx := strings.LastIndex(s.ImplPkg, "/")
	dir := s.ImplPkg
	if idx >= 0 {
		dir = s.ImplPkg[idx+1:]
	}
	// Walk the OutFile up one directory and look for a likely match.
	parent := filepath.Dir(s.OutFile)
	cand := filepath.Join(parent, lowerFirstASCII(s.ImplFunc)+".go")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return filepath.Join(parent, dir+".go")
}

func lowerFirstASCII(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'A' && s[0] <= 'Z' {
		return string(s[0]+32) + s[1:]
	}
	return s
}

// rotateDischargeHistory copies the current report into
// .sb/history/<timestamp>-<commit>.json and prunes older entries
// beyond the retention budget. Errors are reported but not fatal —
// history is a convenience, not a correctness boundary.
func rotateDischargeHistory(r *DischargeReport, currentPath string) error {
	if err := os.MkdirAll(DischargeHistoryDir, 0o755); err != nil {
		return err
	}
	commit := "no-commit"
	if r.Impl.GitCommit != nil && *r.Impl.GitCommit != "" {
		commit = (*r.Impl.GitCommit)[:min(7, len(*r.Impl.GitCommit))]
	}
	ts := time.Now().UTC().Format("2006-01-02T150405Z")
	target := filepath.Join(DischargeHistoryDir, ts+"-"+commit+".json")

	data, err := os.ReadFile(currentPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return err
	}
	pruneDischargeHistory(DischargeHistoryRetention)
	return nil
}

func pruneDischargeHistory(retain int) {
	entries, err := os.ReadDir(DischargeHistoryDir)
	if err != nil {
		return
	}
	type fileInfo struct {
		name string
		mod  time.Time
	}
	var items []fileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, fileInfo{name: e.Name(), mod: info.ModTime()})
	}
	if len(items) <= retain {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	for _, item := range items[retain:] {
		os.Remove(filepath.Join(DischargeHistoryDir, item.name))
	}
}

// computeDischargedSinceCommit walks the history backward to find the
// last commit at which each rule's status differed from the current
// status. The successor commit becomes `discharged_since_commit`. If
// no history exists or all history shows the same status, the field
// is set to the current git commit (or left null when git is
// unavailable).
func computeDischargedSinceCommit(r *DischargeReport) {
	if r.Impl.GitCommit == nil {
		return
	}
	currentCommit := *r.Impl.GitCommit
	history := loadDischargeHistory()
	for i := range r.Rules {
		rule := &r.Rules[i]
		// Only stamp this for currently-discharged rules; for
		// violated/unproven rules the field stays null.
		if rule.Status != DischargeStatusDischarged {
			rule.DischargedSinceCommit = nil
			continue
		}
		since := currentCommit
		// Walk history newest -> oldest. Find the most recent entry
		// where this rule's status was NOT discharged. Its successor
		// is `discharged_since_commit`. If we never find one, the
		// rule has been discharged for as long as we have history,
		// in which case we use the oldest history entry's commit.
		var oldest *DischargeReport
		for _, h := range history {
			oldest = h
			matching := findRule(h, rule.Name)
			if matching == nil || matching.Status != DischargeStatusDischarged {
				if h.Impl.GitCommit != nil {
					since = currentCommit // we flipped recently; current commit is the boundary
				}
				break
			}
		}
		if oldest != nil && oldest.Impl.GitCommit != nil {
			// If the oldest history entry has this rule discharged,
			// we know it's been clean since at least that commit.
			matching := findRule(oldest, rule.Name)
			if matching != nil && matching.Status == DischargeStatusDischarged && oldest.Impl.GitCommit != nil {
				since = *oldest.Impl.GitCommit
			}
		}
		s := since
		rule.DischargedSinceCommit = &s
	}
}

// loadDischargeHistory returns all reports under .sb/history/ sorted
// newest-first by file modtime.
func loadDischargeHistory() []*DischargeReport {
	entries, err := os.ReadDir(DischargeHistoryDir)
	if err != nil {
		return nil
	}
	type entry struct {
		path string
		mod  time.Time
	}
	var es []entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		es = append(es, entry{
			path: filepath.Join(DischargeHistoryDir, e.Name()),
			mod:  info.ModTime(),
		})
	}
	sort.Slice(es, func(i, j int) bool { return es[i].mod.After(es[j].mod) })
	out := make([]*DischargeReport, 0, len(es))
	for _, e := range es {
		r, err := loadDischarge(e.path)
		if err != nil || r == nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

func findRule(r *DischargeReport, name string) *DischargeRule {
	for i := range r.Rules {
		if r.Rules[i].Name == name {
			return &r.Rules[i]
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// normaliseDischargePaths replaces absolute paths in a per-spec
// partial report with repo-relative ones derived from the manifest
// entry. shen-derive is invoked with absolute spec/guard paths
// (because it runs in its own module's directory), so the report it
// emits records absolutes; the project-level report should record
// the paths the user actually wrote in sb.toml.
func normaliseDischargePaths(r *DischargeReport, spec DeriveSpec) {
	relSpec := spec.Path
	for i := range r.Spec.Files {
		r.Spec.Files[i].Path = relSpec
	}
	for i := range r.Rules {
		r.Rules[i].SpecFile = relSpec
		for j := range r.Rules[i].Premises {
			refs := r.Rules[i].Premises[j].CodeReferences
			for k, ref := range refs {
				refs[k] = relativiseGuardRef(ref)
			}
			r.Rules[i].Premises[j].CodeReferences = refs
		}
	}
}

// relativiseGuardRef rewrites an absolute "/path/to/repo/.../guards_gen.go:42"
// path-with-line into a repo-relative form when the absolute path
// has the cwd as a prefix; otherwise returns it unchanged.
func relativiseGuardRef(ref string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ref
	}
	if !strings.HasPrefix(ref, cwd+"/") && !strings.HasPrefix(ref, cwd+string(filepath.Separator)) {
		return ref
	}
	return strings.TrimPrefix(strings.TrimPrefix(ref, cwd+"/"), cwd+string(filepath.Separator))
}
