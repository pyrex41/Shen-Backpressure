package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// BuildOptions holds the inputs needed to produce a per-spec partial
// discharge report. The aggregating layer (sb derive) fills in
// project-level fields like git commit, sb version, history-derived
// `discharged_since_commit`, and any counter-examples discovered by
// running `go test` on the impl.
type BuildOptions struct {
	// SpecPath is the spec file's repo-relative path. Used both to
	// hash the file and to populate Rule.SpecFile.
	SpecPath string

	// Now is the timestamp to record in the report. Callers pass a
	// concrete value to keep tests reproducible; production code uses
	// time.Now().UTC().
	Now time.Time

	// TargetLanguage is "go" or "ts" (currently only "go" is wired).
	TargetLanguage string

	// SBVersion / ShenDeriveVersion / ShengenVersion are tool
	// versions captured at run time. Callers may leave them empty;
	// the field is rendered as the empty string then.
	SBVersion         string
	ShenDeriveVersion string
	ShengenVersion    string

	// ShenRuntime, when non-empty, names the live Shen runtime that is
	// composed with the build via at least one `:runtime-via` annotation
	// in the spec. Empty means the build is pure-static — no runtime
	// composition. Populated by callers that know they're building a
	// project with active runtime-via specs (see
	// docs/RUNTIME-VIA.md). Surfaces as `tools.shen_runtime` and flips
	// `tools.shen_runtime_available` to true. v1 schema reserved both
	// fields; this commit puts them to use without changing the schema.
	ShenRuntime string

	// Rules is the complete set of rules for this spec, fully
	// classified. Callers obtain this by combining ClassifyDatatypes
	// and one ClassifyDefine per (define …) block they care about.
	Rules []Rule
}

// Build assembles a Report from the provided rules and metadata. It
// computes the spec hash, populates the summary, and sorts rules
// deterministically by name.
func Build(opts BuildOptions) (*Report, error) {
	hash, err := hashFile(opts.SpecPath)
	if err != nil {
		return nil, fmt.Errorf("hash %s: %w", opts.SpecPath, err)
	}

	rules := append([]Rule(nil), opts.Rules...)
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Name < rules[j].Name
	})

	var shenRuntime *string
	shenRuntimeAvailable := false
	if opts.ShenRuntime != "" {
		name := opts.ShenRuntime
		shenRuntime = &name
		shenRuntimeAvailable = true
	}

	r := &Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   opts.Now.UTC().Format(time.RFC3339),
		Spec: SpecInfo{
			Files: []SpecFile{
				{Path: opts.SpecPath, SHA256: hash},
			},
			RuleCount: len(rules),
		},
		Impl: ImplInfo{
			TargetLanguages: []string{opts.TargetLanguage},
		},
		Tools: ToolsInfo{
			SBVersion:            opts.SBVersion,
			ShenDeriveVersion:    opts.ShenDeriveVersion,
			ShengenVersion:       opts.ShengenVersion,
			ShenRuntime:          shenRuntime,
			ShenRuntimeAvailable: shenRuntimeAvailable,
		},
		Rules:     rules,
		Signature: nil,
	}
	r.Summary = summarise(rules)
	return r, nil
}

// summarise recomputes the rule and premise counts from a rule slice.
// Always called when finalising a report so the summary cannot drift
// from the rules array.
func summarise(rules []Rule) Summary {
	s := Summary{RuleCount: len(rules)}
	for _, r := range rules {
		switch r.Status {
		case StatusDischarged:
			s.RulesDischarged++
		case StatusViolated:
			s.RulesViolated++
		case StatusUnproven:
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

// Recompute re-runs summarisation in place. Use after mutating rules
// (e.g. when sb derive fills in counter-examples).
func (r *Report) Recompute() {
	r.Summary = summarise(r.Rules)
}

// MarshalJSON renders the report with stable formatting: two-space
// indent and field order matching the schema.
func (r *Report) MarshalJSON() ([]byte, error) {
	type alias Report
	return json.Marshal((*alias)(r))
}

// MarshalIndent returns the JSON bytes for r with the canonical
// formatting used by all written reports.
func (r *Report) MarshalIndent() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// WriteFile writes the report to path with 0644 permissions, parent
// directory created on demand. The file is replaced atomically by
// writing to a sibling tempfile and renaming, so concurrent readers
// never observe a partially-written report.
func (r *Report) WriteFile(path string) error {
	if err := os.MkdirAll(parentDir(path), 0o755); err != nil {
		return err
	}
	data, err := r.MarshalIndent()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parentDir(path), ".discharge_report-*.tmp")
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

// LoadReport reads a previously-written discharge report.
func LoadReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
