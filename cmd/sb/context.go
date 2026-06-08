package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// ProjectContext is the top-level structured view of a Shen-Backpressure
// project, derived from the manifest and the Shen spec file. It is used by
// `sb context` for both JSON and Markdown output, and by the Ralph loop to
// hydrate LLM harness prompts.
type ProjectContext struct {
	Project       ProjectInfo              `json:"project"`
	Types         []TypeInfo               `json:"types"`
	Derive        *DeriveInfo              `json:"derive,omitempty"`
	Cedar         *CedarPolicyInfo         `json:"cedar,omitempty"`          // Cedar (SMT-strong) runtime policy emitter
	Rego          *RegoPolicyInfo          `json:"rego,omitempty"`           // Rego (OPA, middle terminating runtime) emitter; .rego text primary
	DecidableShen *DecidableShenPolicyInfo `json:"decidable_shen,omitempty"` // Decidable-Shen-fragment (native terminating) runtime policy tier
	Gates         []GateInfo               `json:"gates"`
	Discharge     *DischargeContextInfo    `json:"discharge,omitempty"`
	Backpressure  *BackpressureInfo        `json:"backpressure,omitempty"`
}

// DischargeContextInfo is the context-rendering view of the latest
// discharge report. It carries only the summary fields the agent
// needs in-prompt — full detail lives in .sb/discharge_report.json.
type DischargeContextInfo struct {
	GeneratedAt            string                  `json:"generated_at"`
	ReportPath             string                  `json:"report_path"`
	GitCommit              *string                 `json:"git_commit"`
	RuleCount              int                     `json:"rule_count"`
	RulesDischarged        int                     `json:"rules_discharged"`
	RulesViolated          int                     `json:"rules_violated"`
	RulesUnproven          int                     `json:"rules_unproven"`
	PremisesStatic         int                     `json:"premises_static"`
	PremisesRuntimeSampled int                     `json:"premises_runtime_sampled"`
	PremisesUnproven       int                     `json:"premises_unproven"`
	Violations             []DischargeViolationCtx `json:"violations,omitempty"`
}

// DischargeViolationCtx summarises one violated rule for the context
// renderer. At most one counter-example per violation is surfaced
// (the first); auditors who need more detail open the JSON or the
// audit-report Markdown.
type DischargeViolationCtx struct {
	Rule         string            `json:"rule"`
	PremiseID    string            `json:"premise_id"`
	CaseID       string            `json:"case_id"`
	Input        map[string]string `json:"input,omitempty"`
	SpecOutput   string            `json:"spec_output"`
	ImplOutput   string            `json:"impl_output"`
	ImplFunction string            `json:"impl_function,omitempty"`
	ImplFile     string            `json:"impl_file,omitempty"`
	Rationale    string            `json:"rationale,omitempty"`
}

// ProjectInfo holds the project-level manifest fields.
type ProjectInfo struct {
	Lang        string `json:"lang"`
	Pkg         string `json:"pkg"`
	Spec        string `json:"spec"`
	GuardOutput string `json:"guard_output"`
	DBWrappers  string `json:"db_wrappers,omitempty"`
}

// TypeInfo describes a single guard type parsed from the Shen spec.
type TypeInfo struct {
	ShenName     string   `json:"shen_name"`
	TargetName   string   `json:"target_name"`
	Category     string   `json:"category"` // wrapper, constrained, composite, guarded
	Constructor  string   `json:"constructor"`
	Fields       []string `json:"fields,omitempty"`
	Verified     []string `json:"verified,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// DeriveInfo summarises configured shen-derive specs.
type DeriveInfo struct {
	Enabled bool             `json:"enabled"`
	Specs   []DeriveSpecInfo `json:"specs"`
}

// DeriveSpecInfo is a per-spec summary for the context output.
type DeriveSpecInfo struct {
	Lang     string `json:"lang"`
	Func     string `json:"func"`
	ImplFunc string `json:"impl_func"`
	OutFile  string `json:"out_file"`
}

// CedarPolicyInfo summarises [cedar] (and future sibling policy emitter) config
// for sb context output and prompt hydration. This makes the runtime policy
// targets visible to agents and the discharge/evidence story (Cedar JSON as a
// stronger provenance tier than pure sampling for the access slice).
type CedarPolicyInfo struct {
	Enabled     bool     `json:"enabled"`
	SchemaOut   string   `json:"schema_out,omitempty"`
	PoliciesOut string   `json:"policies_out,omitempty"`
	Targets     []string `json:"targets,omitempty"`
}

// RegoPolicyInfo summarises [rego] config for context + prompts.
// Primary artifact is the .rego text module (opa eval friendly).
// Documents the middle tier position: more expressive than Cedar for
// aggregation/document/infra gating rules; still terminating.
type RegoPolicyInfo struct {
	Enabled   bool     `json:"enabled"`
	ModuleOut string   `json:"module_out,omitempty"`
	Targets   []string `json:"targets,omitempty"`
}

// DecidableShenPolicyInfo summarises the decidable-Shen-fragment tier
// (the "runtime shen that is also decidable", middle lattice tier).
// Annotation-driven ( @decidable-fragment ), certified by fragment judgment
// (sequent calculus / Prolog gatekeeper). Can run directly in Shen ports or
// via tiny total-eval stub. Extends differential n-way comparison.
type DecidableShenPolicyInfo struct {
	Enabled bool     `json:"enabled"`
	Targets []string `json:"targets,omitempty"`
	// Future: CertPath, EvalStub etc for the emitter artifacts.
}

// GateInfo mirrors a single gate from the manifest (or the synthesised legacy
// five-gate list) for context output.
//
// LastResult / LastDurationMs / LastExitCode are sourced from
// .sb/gates_last_run.json (written by `sb gates`). They are absent
// when no run has been recorded — the JSON omitempty keeps the
// context payload small in the cold-start case. Renderers treat an
// empty LastResult as "no last run" (`[—]`).
type GateInfo struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Run            string `json:"run,omitempty"`
	ParallelGroup  string `json:"parallel_group,omitempty"`
	LastResult     string `json:"last_result,omitempty"`      // "pass" | "fail" | ""
	LastDurationMs int64  `json:"last_duration_ms,omitempty"` // 0 when LastResult==""
	LastExitCode   int    `json:"last_exit_code,omitempty"`   // omitempty hides zero on pass
}

// BackpressureInfo summarises the latest entry in plans/backpressure.log if
// one exists. Only populated when the log is non-empty.
type BackpressureInfo struct {
	HasFailures   bool   `json:"has_failures"`
	LatestFailure string `json:"latest_failure,omitempty"`
	Summary       string `json:"summary,omitempty"`
}

// BuildContext assembles a ProjectContext from a loaded Config. It parses the
// Shen spec to extract guard types, normalises gates from the manifest (or
// synthesises them from the legacy command fields), and checks for recent
// backpressure log entries.
func BuildContext(cfg *Config) (*ProjectContext, error) {
	ctx := &ProjectContext{
		Project: ProjectInfo{
			Lang:        cfg.Lang,
			Pkg:         cfg.Pkg,
			Spec:        cfg.Spec,
			GuardOutput: cfg.Output,
			DBWrappers:  cfg.DBWrap,
		},
	}

	types, err := parseSpecTypes(cfg.Spec, cfg.Lang)
	if err != nil {
		// Spec parse failures are non-fatal for context output — we still
		// want to emit the rest of the manifest so users can diagnose.
		fmt.Fprintf(os.Stderr, "sb context: warning: parsing %s: %v\n", cfg.Spec, err)
	}
	ctx.Types = types

	ctx.Gates = buildGateInfos(cfg)

	if len(cfg.DeriveSpecs) > 0 {
		di := &DeriveInfo{Enabled: true}
		for _, s := range cfg.DeriveSpecs {
			di.Specs = append(di.Specs, DeriveSpecInfo{
				Lang:     s.Lang,
				Func:     s.Func,
				ImplFunc: s.ImplFunc,
				OutFile:  s.OutFile,
			})
		}
		ctx.Derive = di
	}

	if cfg.Cedar.SchemaOut != "" || cfg.Cedar.PoliciesOut != "" || len(cfg.Cedar.Targets) > 0 {
		ctx.Cedar = &CedarPolicyInfo{
			Enabled:     true,
			SchemaOut:   cfg.Cedar.SchemaOut,
			PoliciesOut: cfg.Cedar.PoliciesOut,
			Targets:     append([]string(nil), cfg.Cedar.Targets...),
		}
	}

	if cfg.Rego.ModuleOut != "" || len(cfg.Rego.Targets) > 0 {
		ctx.Rego = &RegoPolicyInfo{
			Enabled:   true,
			ModuleOut: cfg.Rego.ModuleOut,
			Targets:   append([]string(nil), cfg.Rego.Targets...),
		}
	}

	if cfg.DecidableShen.Enabled || len(cfg.DecidableShen.Targets) > 0 {
		ctx.DecidableShen = &DecidableShenPolicyInfo{
			Enabled: true,
			Targets: append([]string(nil), cfg.DecidableShen.Targets...),
		}
	}

	if di := readDischargeContext(DischargeReportPath); di != nil {
		ctx.Discharge = di
	}

	if bp := readBackpressure("plans/backpressure.log"); bp != nil {
		ctx.Backpressure = bp
	}

	return ctx, nil
}

// readDischargeContext reads .sb/discharge_report.json and projects
// it into the small DischargeContextInfo shape used by both the JSON
// and Markdown context renderers. Returns nil when no report exists,
// matching Wave 4's "omit the section entirely" requirement.
func readDischargeContext(path string) *DischargeContextInfo {
	r, err := loadDischarge(path)
	if err != nil || r == nil {
		return nil
	}
	out := &DischargeContextInfo{
		GeneratedAt:            r.GeneratedAt,
		ReportPath:             path,
		GitCommit:              r.Impl.GitCommit,
		RuleCount:              r.Summary.RuleCount,
		RulesDischarged:        r.Summary.RulesDischarged,
		RulesViolated:          r.Summary.RulesViolated,
		RulesUnproven:          r.Summary.RulesUnproven,
		PremisesStatic:         r.Summary.PremisesStatic,
		PremisesRuntimeSampled: r.Summary.PremisesRuntimeSampled,
		PremisesUnproven:       r.Summary.PremisesUnproven,
	}
	for _, rule := range r.Rules {
		if rule.Status != DischargeStatusViolated {
			continue
		}
		premiseID := ""
		for _, p := range rule.Premises {
			if p.SamplesFailed > 0 {
				premiseID = p.ID
				break
			}
		}
		var ce DischargeCounter
		if len(rule.CounterExamples) > 0 {
			ce = rule.CounterExamples[0]
		}
		out.Violations = append(out.Violations, DischargeViolationCtx{
			Rule:         rule.Name,
			PremiseID:    premiseID,
			CaseID:       ce.CaseID,
			Input:        ce.Input,
			SpecOutput:   ce.SpecOutput,
			ImplOutput:   ce.ImplOutput,
			ImplFunction: ce.ImplFunction,
			ImplFile:     ce.ImplFile,
			Rationale:    ce.Rationale,
		})
	}
	return out
}

// buildGateInfos produces the public GateInfo list for the context. When the
// manifest uses [[gates]], each entry is copied directly; otherwise we
// synthesise the legacy five-gate pipeline from the command fields.
//
// Each GateInfo is decorated with the latest known PASS/FAIL outcome
// from .sb/gates_last_run.json (written by `sb gates`). When the
// sidecar is missing — first checkout, fresh worktree, or pipeline
// has never run — the LastResult field stays empty so renderers can
// show `[—]`. Schema-locked discharge_report.json is untouched.
func buildGateInfos(cfg *Config) []GateInfo {
	var out []GateInfo
	if cfg.HasManifestGates() {
		for _, g := range cfg.Gates {
			out = append(out, GateInfo{
				Name:          g.Name,
				Kind:          string(g.Kind),
				Run:           g.Run,
				ParallelGroup: g.ParallelGroup,
			})
		}
	} else {
		testGroup, buildGroup := "", ""
		if cfg.Relaxed {
			testGroup, buildGroup = "build-test", "build-test"
		}
		out = []GateInfo{
			{Name: "shengen", Kind: "command", Run: cfg.Gen},
			{Name: "test", Kind: "command", Run: cfg.Test, ParallelGroup: testGroup},
			{Name: "build", Kind: "command", Run: cfg.Build, ParallelGroup: buildGroup},
			{Name: "shen-check", Kind: "command", Run: cfg.Check},
			{Name: "tcb-audit", Kind: "command", Run: cfg.Audit},
		}
	}
	if len(cfg.DeriveSpecs) > 0 {
		out = append(out, GateInfo{Name: "shen-derive", Kind: "derive"})
	}
	// Append shen-cedar policy gate entry (for context listing) when CedarConfig present,
	// matching the auto-append logic in gates.go and derive handling.
	if cfg.Cedar.SchemaOut != "" || cfg.Cedar.PoliciesOut != "" || len(cfg.Cedar.Targets) > 0 {
		out = append(out, GateInfo{Name: "shen-cedar", Kind: "policy"})
	}
	// Append shen-rego policy gate entry (parallel to cedar).
	if cfg.Rego.ModuleOut != "" || len(cfg.Rego.Targets) > 0 {
		out = append(out, GateInfo{Name: "shen-rego", Kind: "policy"})
	}
	// Append shen-decidable (decidable fragment) gate when DecidableShen config present.
	// (Sketch: certification + pure-shen-fragment-eval for differential.)
	if cfg.DecidableShen.Enabled || len(cfg.DecidableShen.Targets) > 0 {
		out = append(out, GateInfo{Name: "shen-decidable", Kind: "policy"})
	}
	// Defensive overlay of last-run results. readGatesLastRun returns
	// nil on any I/O or parse error; gateLastResultByName returns nil
	// for gates that didn't appear in the last run (e.g. a newly
	// added manifest gate).
	if lr := readGatesLastRun(GatesLastRunPath); lr != nil {
		for i := range out {
			res := gateLastResultByName(lr, out[i].Name)
			if res == nil {
				continue
			}
			if res.Passed {
				out[i].LastResult = "pass"
			} else {
				out[i].LastResult = "fail"
			}
			out[i].LastDurationMs = res.DurationMs
			out[i].LastExitCode = res.ExitCode
		}
	}
	return out
}

// readBackpressure reads the backpressure log and extracts a summary of the
// latest failure, if any. Returns nil when the file is missing or empty.
func readBackpressure(path string) *BackpressureInfo {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	latest := lines[len(lines)-1]
	summary := latest
	if len(lines) > 5 {
		summary = strings.Join(lines[len(lines)-5:], "\n")
	}
	return &BackpressureInfo{
		HasFailures:   true,
		LatestFailure: latest,
		Summary:       summary,
	}
}

// RenderJSON returns the context as pretty-printed JSON.
func (ctx *ProjectContext) RenderJSON() ([]byte, error) {
	return json.MarshalIndent(ctx, "", "  ")
}

// RenderMarkdown returns a compact Markdown view intended for LLM prompt
// hydration. Sections are omitted when empty to keep the prompt small.
func (ctx *ProjectContext) RenderMarkdown() string {
	var b strings.Builder
	b.WriteString("## Project Context\n\n")
	fmt.Fprintf(&b, "**Language**: %s | **Package**: %s | **Spec**: %s\n",
		ctx.Project.Lang, ctx.Project.Pkg, ctx.Project.Spec)
	if ctx.Project.GuardOutput != "" {
		fmt.Fprintf(&b, "**Guard output**: %s\n", ctx.Project.GuardOutput)
	}
	if ctx.Project.DBWrappers != "" {
		fmt.Fprintf(&b, "**DB wrappers**: %s\n", ctx.Project.DBWrappers)
	}

	if len(ctx.Types) > 0 {
		b.WriteString("\n### Guard Types\n\n")
		b.WriteString("| Type | Category | Constructor |\n")
		b.WriteString("|------|----------|-------------|\n")
		for _, t := range ctx.Types {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", t.TargetName, t.Category, t.Constructor)
		}

		// Proof chain: topological chain of types via Dependencies.
		if chain := proofChain(ctx.Types); len(chain) > 1 {
			b.WriteString("\n**Proof Chain**: ")
			b.WriteString(strings.Join(chain, " -> "))
			b.WriteString("\n")
		}
	}

	if len(ctx.Gates) > 0 {
		b.WriteString("\n### Gates\n\n")
		for i, g := range ctx.Gates {
			line := fmt.Sprintf("%d. %s (%s)", i+1, g.Name, g.Kind)
			line += " " + formatGateLastResult(g)
			if g.ParallelGroup != "" {
				line += fmt.Sprintf(" [parallel: %s]", g.ParallelGroup)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	if ctx.Derive != nil && len(ctx.Derive.Specs) > 0 {
		b.WriteString("\n### Derive Coverage\n\n")
		for _, s := range ctx.Derive.Specs {
			fmt.Fprintf(&b, "- %s -> %s (%s)\n", s.Func, s.ImplFunc, s.OutFile)
		}
	}

	if ctx.Discharge != nil {
		renderDischargeSection(&b, ctx.Discharge)
	}

	if ctx.Cedar != nil && ctx.Cedar.Enabled {
		b.WriteString("\n### Cedar Policies (runtime emitter)\n\n")
		if ctx.Cedar.SchemaOut != "" {
			fmt.Fprintf(&b, "- schema: %s\n", ctx.Cedar.SchemaOut)
		}
		if ctx.Cedar.PoliciesOut != "" {
			fmt.Fprintf(&b, "- policies: %s\n", ctx.Cedar.PoliciesOut)
		}
		if len(ctx.Cedar.Targets) > 0 {
			fmt.Fprintf(&b, "- targets: %s\n", strings.Join(ctx.Cedar.Targets, ", "))
		} else {
			b.WriteString("- targets: (inferred from shape / @policy-target annotations)\n")
		}
		b.WriteString("(Cedar is the SMT-strongest tier for snapshot access predicates.)\n")
	}

	if ctx.Rego != nil && ctx.Rego.Enabled {
		b.WriteString("\n### Rego Policies (OPA middle-tier terminating runtime emitter)\n\n")
		if ctx.Rego.ModuleOut != "" {
			fmt.Fprintf(&b, "- module (primary text .rego): %s\n", ctx.Rego.ModuleOut)
		}
		if len(ctx.Rego.Targets) > 0 {
			fmt.Fprintf(&b, "- targets: %s\n", strings.Join(ctx.Rego.Targets, ", "))
		} else {
			b.WriteString("- targets: (inferred from shape / @policy-target annotations)\n")
		}
		b.WriteString("(Rego: Datalog-derived, supports aggregation/joins/walk/graph for what Cedar cannot express. Primary form is text .rego (opa eval / conftest). Reserve for non-hot-path + infra gating. Lattice: Cedar (SMT) ⊂ Rego (terminating foreign) ⊂ Decidable-Shen-fragment ⊂ full-TC pure-Shen.)\n")
	}

	if ctx.DecidableShen != nil && ctx.DecidableShen.Enabled {
		b.WriteString("\n### Decidable-Shen-Fragment (native runtime policy tier)\n\n")
		b.WriteString("Shen-native but decidable fragment (sequent calculus + Prolog gatekeeper).\n")
		b.WriteString("Restricted: no general recursion, stratified/Horn bodies, total forms only.\n")
		if len(ctx.DecidableShen.Targets) > 0 {
			fmt.Fprintf(&b, "- targets: %s\n", strings.Join(ctx.DecidableShen.Targets, ", "))
		} else {
			b.WriteString("- targets: (inferred or @decidable-fragment annotated; see specs)\n")
		}
		b.WriteString("- emitter/mode: tiny recognizer + certifier (or total-eval stub); can run directly in shen-* ports with termination guarantee.\n")
		b.WriteString("- lattice: Cedar ⊂ Rego ⊂ Decidable-Shen-fragment ⊂ full-TC pure-Shen\n")
		b.WriteString("- differential: extends n-way (guard vs Cedar vs pure-shen-fragment-eval on same samples)\n")
	}

	if ctx.Backpressure != nil && ctx.Backpressure.HasFailures {
		b.WriteString("\n### Backpressure\n\n")
		fmt.Fprintf(&b, "Latest failure: %s\n", ctx.Backpressure.LatestFailure)
	}

	return b.String()
}

// formatGateLastResult renders a gate's last-run outcome as a short,
// agent-skim-friendly tag — `[PASS 0.12s]`, `[FAIL exit=2 0.34s]`, or
// `[—]` when no run is recorded. The duration is rounded to a single
// fractional digit (no millisecond noise) and the FAIL form surfaces
// the exit code so an agent reading context cold can tell whether the
// previous run actually executed or fell off a cliff. The em-dash
// fallback intentionally distinguishes "not yet run" from "passed
// silently with zero duration".
func formatGateLastResult(g GateInfo) string {
	if g.LastResult == "" {
		return "[—]"
	}
	dur := time.Duration(g.LastDurationMs) * time.Millisecond
	if g.LastResult == "pass" {
		return fmt.Sprintf("[PASS %s]", roundDuration(dur))
	}
	if g.LastExitCode != 0 {
		return fmt.Sprintf("[FAIL exit=%d %s]", g.LastExitCode, roundDuration(dur))
	}
	return fmt.Sprintf("[FAIL %s]", roundDuration(dur))
}

// roundDuration produces a short human-readable duration string. We
// intentionally round to centiseconds for short runs and seconds for
// longer ones so a 12-gate pipeline summary stays under a screen
// width. time.Duration.String() prints noisy fractions for short
// values (e.g. "1.234567ms"); this trims that.
func roundDuration(d time.Duration) string {
	if d < time.Millisecond {
		// Below 1ms: just say "<1ms" — agents don't need finer.
		return "<1ms"
	}
	if d < time.Second {
		return d.Round(10 * time.Millisecond).String()
	}
	if d < 10*time.Second {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

// renderDischargeSection writes the terse Wave 4 discharge-report
// summary into b. The block is intentionally short — enough for an
// agent to skim in five seconds and find the failing case. Full
// detail lives in .sb/discharge_report.json (machine) or
// `sb audit-report` (human).
func renderDischargeSection(b *strings.Builder, di *DischargeContextInfo) {
	b.WriteString("\n### Discharge Report\n\n")
	fmt.Fprintf(b, "%d premises proven statically (via guard types)\n", di.PremisesStatic)
	fmt.Fprintf(b, "%d premises sampled clean (deterministic seed)\n", di.PremisesRuntimeSampled-violatedSampledCount(di))
	if di.PremisesUnproven > 0 {
		fmt.Fprintf(b, "%d premises unproven\n", di.PremisesUnproven)
	}
	if di.RulesViolated == 0 {
		fmt.Fprintf(b, "%d/%d rules discharged.\n", di.RulesDischarged, di.RuleCount)
	} else {
		fmt.Fprintf(b, "%d/%d rules discharged; %d violated.\n",
			di.RulesDischarged, di.RuleCount, di.RulesViolated)
	}
	for _, v := range di.Violations {
		b.WriteString("\nCounter-example for ")
		b.WriteString(v.Rule)
		if v.PremiseID != "" {
			fmt.Fprintf(b, ".%s", v.PremiseID)
		}
		b.WriteString(":\n")
		fmt.Fprintf(b, "  Case:        %s\n", v.CaseID)
		if v.SpecOutput != "" {
			fmt.Fprintf(b, "  Spec says:   %s\n", v.SpecOutput)
		}
		if v.ImplOutput != "" {
			fmt.Fprintf(b, "  Impl says:   %s\n", v.ImplOutput)
		}
		if v.ImplFile != "" {
			fmt.Fprintf(b, "  Impl file:   %s\n", v.ImplFile)
		}
		if v.ImplFunction != "" {
			fmt.Fprintf(b, "  Reproduce:   go test -run TestSpec_%s/%s\n", v.ImplFunction, v.CaseID)
		}
	}
	fmt.Fprintf(b, "\nLatest report: %s (full JSON for tooling)\n", di.ReportPath)
}

// violatedSampledCount returns the count of runtime-sampled premises
// that have been flipped to violated (i.e. `samples_failed > 0`).
// Used to subtract from the "sampled clean" line so the rendering
// stays accurate when failures land.
func violatedSampledCount(di *DischargeContextInfo) int {
	// The discharge context summary doesn't carry per-premise detail,
	// so we approximate: every violation contributes one failing
	// sampled premise.
	return len(di.Violations)
}

// proofChain returns a best-effort ordering of types following the
// dependency edges. The ordering is stable: it walks types in source order
// and emits each exactly once, skipping ones that don't form a chain with
// their predecessor. For the common payment example this produces
// amount -> transaction -> balance-checked -> safe-transfer.
func proofChain(types []TypeInfo) []string {
	if len(types) == 0 {
		return nil
	}
	// Build a name->index for quick lookup.
	byName := make(map[string]int, len(types))
	for i, t := range types {
		byName[t.ShenName] = i
	}
	// Linear chain: include types that depend on the previous one, plus
	// roots at the start.
	var out []string
	for _, t := range types {
		if len(t.Dependencies) == 0 {
			out = append(out, t.TargetName)
			continue
		}
		// Only include if at least one dependency is already in the chain.
		for _, dep := range t.Dependencies {
			if _, ok := byName[dep]; ok {
				out = append(out, t.TargetName)
				break
			}
		}
	}
	return out
}

// cmdContext is the CLI entry point for `sb context`.
func cmdContext(args []string) {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	format := fs.String("format", "markdown", "output format: json or markdown")
	evidence := fs.Bool("evidence", false, "emit the mixed-evidence summary (static/runtime-sampled/unproven per rule) instead of the standard context")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb context — Emit project context from the manifest

Usage: sb context [flags]

Parses sb.toml and the Shen spec to produce a structured view of the
project: guard types, gate pipeline, derive coverage, and any recent
backpressure failures. Output is consumed by humans (markdown) or by
the Ralph loop for LLM prompt hydration (json).

When --evidence is set, emits the mixed-evidence summary instead: a
table showing how many premises per rule are discharged statically
vs runtime-sampled vs unproven, sourced from .sb/discharge_report.json.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb context: %v\n", err)
		os.Exit(1)
	}

	ctx, err := BuildContext(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb context: %v\n", err)
		os.Exit(1)
	}

	if *evidence {
		switch *format {
		case "json":
			ev := BuildEvidenceSummary(DischargeReportPath)
			data, err := json.MarshalIndent(ev, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb context: rendering evidence json: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(string(data))
		case "markdown", "md":
			fmt.Print(RenderEvidenceMarkdown(DischargeReportPath))
		default:
			fmt.Fprintf(os.Stderr, "sb context: unknown format %q (want json or markdown)\n", *format)
			os.Exit(1)
		}
		return
	}

	switch *format {
	case "json":
		data, err := ctx.RenderJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb context: rendering json: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	case "markdown", "md":
		fmt.Print(ctx.RenderMarkdown())
	default:
		fmt.Fprintf(os.Stderr, "sb context: unknown format %q (want json or markdown)\n", *format)
		os.Exit(1)
	}
}

// -----------------------------------------------------------------------------
// Minimal Shen spec parser
//
// Mirrors the approach in shen-derive/specfile/parse.go but is self-contained
// so the cmd/sb module doesn't pick up a dependency on shen-derive. It only
// extracts (datatype ...) blocks and classifies them into the four guard
// categories — wrapper, constrained, composite, guarded. (define ...) blocks
// are ignored here; derive coverage comes from the manifest instead.
// -----------------------------------------------------------------------------

// parseSpecTypes reads a .shen spec file and returns the guard types it
// declares, classified and with target-language constructor signatures.
// A missing spec file is treated as an empty type list, not an error.
func parseSpecTypes(path, lang string) ([]TypeInfo, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	content := stripShenComments(string(data))
	blocks := extractDatatypeBlocks(content)

	// First pass: classify each datatype and collect raw type info.
	var types []TypeInfo
	knownTypes := make(map[string]bool)
	for _, block := range blocks {
		ti := parseDatatypeBlock(block, lang)
		if ti == nil {
			continue
		}
		knownTypes[ti.ShenName] = true
		types = append(types, *ti)
	}

	// Filter each type's Dependencies down to the known-type set so we
	// don't surface spurious entries for primitive-like aliases.
	for i := range types {
		if len(types[i].Dependencies) == 0 {
			continue
		}
		filtered := types[i].Dependencies[:0]
		for _, d := range types[i].Dependencies {
			if knownTypes[d] {
				filtered = append(filtered, d)
			}
		}
		types[i].Dependencies = filtered
	}

	return types, nil
}

// stripShenComments removes \* ... *\ block comments and \\ ... line comments
// from a Shen source string, preserving string-literal contents.
func stripShenComments(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '"' {
			b.WriteByte(s[i])
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					b.WriteByte(s[i])
					b.WriteByte(s[i+1])
					i += 2
					continue
				}
				b.WriteByte(s[i])
				if s[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}
		if i+1 < len(s) && s[i] == '\\' && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*\\")
			if end == -1 {
				break
			}
			i += end + 4
			continue
		}
		if i+1 < len(s) && s[i] == '\\' && s[i+1] == '\\' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// extractDatatypeBlocks finds balanced-paren "(datatype ...)" blocks.
func extractDatatypeBlocks(content string) []string {
	const prefix = "(datatype "
	var out []string
	remaining := content
	for {
		idx := strings.Index(remaining, prefix)
		if idx == -1 {
			break
		}
		remaining = remaining[idx:]
		depth, end := 0, -1
		i := 0
		for i < len(remaining) {
			ch := remaining[i]
			if ch == '"' {
				i++
				for i < len(remaining) {
					if remaining[i] == '\\' && i+1 < len(remaining) {
						i += 2
						continue
					}
					if remaining[i] == '"' {
						i++
						break
					}
					i++
				}
				continue
			}
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
			i++
		}
		if end == -1 {
			break
		}
		out = append(out, remaining[:end])
		remaining = remaining[end:]
	}
	return out
}

// parseDatatypeBlock parses a single (datatype ...) block, classifies it, and
// builds a TypeInfo with the target-language constructor signature.
func parseDatatypeBlock(block, lang string) *TypeInfo {
	inner := strings.TrimPrefix(block, "(datatype ")
	nlIdx := strings.Index(inner, "\n")
	if nlIdx == -1 {
		return nil
	}
	shenName := strings.TrimSpace(inner[:nlIdx])
	body := strings.TrimRight(inner[nlIdx:], " \t\n)")

	// Find the first ==== separator and split premises from conclusion.
	lines := strings.Split(body, "\n")
	var premLines, concLines []string
	seenInf := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if len(t) >= 3 && (allRune(t, '=') || allRune(t, '_')) {
			seenInf = true
			continue
		}
		if !seenInf {
			premLines = append(premLines, t)
		} else {
			concLines = append(concLines, t)
		}
	}
	if len(concLines) == 0 {
		return nil
	}

	// Parse premises: value premises "X : type;" and verified premises
	// "(...) : verified;".
	type fieldPremise struct {
		name, typ string
	}
	var fields []fieldPremise
	var verified []string
	for _, raw := range premLines {
		line := strings.TrimSuffix(strings.TrimSpace(raw), ";")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ": verified") {
			v := strings.TrimSpace(strings.TrimSuffix(line, ": verified"))
			verified = append(verified, v)
			continue
		}
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) != 2 {
			continue
		}
		fields = append(fields, fieldPremise{
			name: strings.TrimSpace(parts[0]),
			typ:  strings.TrimSpace(parts[1]),
		})
	}
	if len(fields) == 0 {
		return nil
	}

	// Parse conclusion "[...] : typename" or "X : typename".
	concStr := strings.TrimSpace(strings.TrimSuffix(strings.Join(concLines, " "), ";"))
	if strings.Contains(concStr, ">>") {
		return nil // subtype/refinement rules (not a plain datatype)
	}
	cp := strings.SplitN(concStr, " : ", 2)
	if len(cp) != 2 {
		return nil
	}
	// Classify.
	category := classifyType(len(fields), len(verified))

	// Target language name.
	targetName := toTargetName(shenName, lang)

	// Constructor signature.
	var fieldNames []string
	var fieldTypes []string
	for _, f := range fields {
		fieldNames = append(fieldNames, f.name)
		fieldTypes = append(fieldTypes, f.typ)
	}
	ctor := buildConstructor(targetName, fieldTypes, len(verified) > 0, lang)

	// Dependencies: field types that look like other datatypes (i.e. not
	// primitive shen types).
	deps := collectDependencies(fieldTypes)

	return &TypeInfo{
		ShenName:     shenName,
		TargetName:   targetName,
		Category:     category,
		Constructor:  ctor,
		Fields:       fieldNames,
		Verified:     verified,
		Dependencies: deps,
	}
}

// classifyType maps (value-field count, verified-premise count) to one of the
// four guard categories.
func classifyType(nFields, nVerified int) string {
	switch {
	case nFields == 1 && nVerified == 0:
		return "wrapper"
	case nFields == 1 && nVerified > 0:
		return "constrained"
	case nFields > 1 && nVerified == 0:
		return "composite"
	default:
		return "guarded"
	}
}

// collectDependencies returns the subset of field types that look like other
// user-defined datatypes (kebab-case identifiers, not Shen primitives).
func collectDependencies(fieldTypes []string) []string {
	primitives := map[string]bool{
		"number": true, "string": true, "boolean": true, "symbol": true,
		"unit": true,
	}
	var out []string
	seen := make(map[string]bool)
	for _, ft := range fieldTypes {
		ft = strings.TrimSpace(ft)
		// Handle "(list T)" — extract T.
		if strings.HasPrefix(ft, "(list ") && strings.HasSuffix(ft, ")") {
			ft = strings.TrimSpace(ft[6 : len(ft)-1])
		}
		if ft == "" || primitives[ft] || strings.ContainsAny(ft, "() ") {
			continue
		}
		if seen[ft] {
			continue
		}
		seen[ft] = true
		out = append(out, ft)
	}
	return out
}

// toTargetName converts a kebab-case Shen name to the target language's
// conventional type name. Go uses PascalCase; TypeScript uses PascalCase too
// for type declarations.
func toTargetName(shenName, lang string) string {
	parts := strings.Split(shenName, "-")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}

// buildConstructor returns a human-readable constructor signature string.
// For constrained/guarded types (those with verified premises), the
// constructor returns an error alongside the value.
func buildConstructor(targetName string, fieldTypes []string, withError bool, lang string) string {
	var params []string
	for _, ft := range fieldTypes {
		params = append(params, targetType(ft, lang))
	}
	paramList := strings.Join(params, ", ")

	switch lang {
	case "ts":
		ret := targetName
		if withError {
			return fmt.Sprintf("new%s(%s): %s | Error", targetName, paramList, ret)
		}
		return fmt.Sprintf("new%s(%s): %s", targetName, paramList, ret)
	default: // go
		if withError {
			return fmt.Sprintf("New%s(%s) (%s, error)", targetName, paramList, targetName)
		}
		return fmt.Sprintf("New%s(%s) %s", targetName, paramList, targetName)
	}
}

// targetType maps a Shen type name to the target language's type name.
// Known user types are converted via toTargetName; primitives map to the
// language's native type; unknown forms fall back to the raw Shen text.
func targetType(shenType, lang string) string {
	shenType = strings.TrimSpace(shenType)
	// Handle "(list T)".
	if strings.HasPrefix(shenType, "(list ") && strings.HasSuffix(shenType, ")") {
		inner := strings.TrimSpace(shenType[6 : len(shenType)-1])
		innerT := targetType(inner, lang)
		switch lang {
		case "ts":
			return innerT + "[]"
		default:
			return "[]" + innerT
		}
	}
	switch shenType {
	case "number":
		if lang == "ts" {
			return "number"
		}
		return "float64"
	case "string":
		return "string"
	case "boolean":
		if lang == "ts" {
			return "boolean"
		}
		return "bool"
	}
	// User-defined type — convert kebab-case to target name.
	if strings.ContainsAny(shenType, "() ") {
		return shenType
	}
	return toTargetName(shenType, lang)
}

// allRune reports whether every rune in s equals r. Used to detect ===== and
// _____ separator lines.
func allRune(s string, r rune) bool {
	for _, c := range s {
		if c != r {
			return false
		}
	}
	return true
}
