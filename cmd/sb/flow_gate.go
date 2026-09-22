package main

// flow_gate.go — gate kind "flow": evaluate every (flow …) premise in
// the project's spec over the resolved symbol graph, and record the
// outcome in the discharge report.
//
// This is the gate that replaces a grep. A grep over source text
// cannot see that `sg "…/shenguard"` and `shenguard "…/shenguard"`
// name the same package, so it answers a question about spelling. The
// flow gate asks the indexer, which ran the language's own type
// checker, so it answers a question about the program.
//
// Honesty about what ran is the point of the fallback path: when no
// indexer is on PATH the legacy grep command from the gate's `run`
// field executes instead, and the premises are recorded as *unproven*
// with basis "grep-fallback". A report must never claim
// "flow-analysis" for evidence a regex produced.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pyrex41/Shen-Backpressure/cmd/sb/flow"
)

// DischargeBasisFlow is the reserved discharge basis for a premise
// proved by the flow engine over a resolved symbol graph.
const DischargeBasisFlow = "flow-analysis"

// DischargeBasisGrepFallback marks a premise that only the legacy
// regex gate looked at. It is never a discharge.
const DischargeBasisGrepFallback = "grep-fallback"

// FlowRuleKind is the `kind` field flow rules carry in the report.
const FlowRuleKind = "flow"

func cmdFlow(args []string) {
	fs := flag.NewFlagSet("flow", flag.ExitOnError)
	fallback := fs.String("fallback", "", "legacy grep command to run when no SCIP indexer is available")
	force := fs.Bool("force", false, "re-run the indexer even when cached facts are current")
	noReport := fs.Bool("no-report", false, "evaluate and print, but do not record premises in the discharge report (used by `sb forgery`, whose staged tree is not the project)")
	engine := fs.String("engine", "", "which engine evaluates the premises: go, shen, or both (default: both when a Shen host is available, go otherwise)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb flow — Evaluate (flow ...) premises over the resolved symbol graph

Usage: sb flow [flags]

Reads every (flow <name> ...) form from the project spec, runs
`+"`sb index`"+` (cached by tree hash), evaluates each premise, and merges
the outcome into %s:

  (constructor-only <ctor> <allowed-caller>...)
      every resolved reference to <ctor> lies inside an allowed caller
  (must-pass-through <source> <proof> <sink>)
      every call path from <source> to <sink> references <proof>

Discharged premises carry basis %q. Violations carry the
file:line of the offending reference and the shortest violating path.
With no indexer on PATH the -fallback command runs instead and the
premises are recorded unproven with basis %q.

Two engines evaluate the same rules (W6). -engine shen runs the Shen
Prolog rules in sb/flow/stdlib.shen, which are the primary statement
of what a flow premise means; -engine go runs their transcription in
cmd/sb/flow, which is the one that produces file:line and the shortest
violating path. -engine both runs each and FAILS if they disagree
about any premise, which is what turns "the two engines implement the
same rules" from a documented assumption into a checked one. The
default is both when a Shen host is available and go otherwise; the
report records which in the flow_engine field.

Flags:
`, DischargeReportPath, DischargeBasisFlow, DischargeBasisGrepFallback)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb flow: %v\n", err)
		os.Exit(1)
	}

	decls, err := loadFlowDecls(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb flow: %v\n", err)
		os.Exit(1)
	}
	if len(decls) == 0 {
		fmt.Fprintf(os.Stderr, "sb flow: no (flow ...) premises in %s — nothing to check\n", cfg.Spec)
		return
	}

	out, err := ensureIndex(cfg, *force, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb flow: %v\n", err)
		os.Exit(1)
	}

	if !out.Available {
		os.Exit(runFlowFallback(cfg, decls, out, *fallback, *noReport))
	}

	mode, host, err := resolveFlowEngine(cfg, *engine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb flow: %v\n", err)
		os.Exit(1)
	}

	results := flow.Evaluate(out.Facts, decls)

	// The Shen engine is asked the same questions and its verdicts are
	// compared. A disagreement fails the gate before anything is
	// recorded: a report that named one engine while the other
	// disagreed would be worse than a report with no engine at all.
	var disagreements []flow.EngineDisagreement
	if mode == FlowEngineShen || mode == FlowEngineBoth {
		verdicts, hostOut, shenErr := runShenFlowEngine(host, out.FactsPath, decls)
		if shenErr != nil {
			fmt.Fprintf(os.Stderr, "sb flow: Shen engine: %v\n", shenErr)
			if s := strings.TrimSpace(hostOut); s != "" {
				fmt.Fprintln(os.Stderr, s)
			}
			os.Exit(1)
		}
		disagreements = flow.CompareEngines(results, verdicts)
		for _, v := range verdicts {
			fmt.Fprintf(os.Stderr, "      shen: %s → %s\n", v.PremiseID, shenVerdictWord(v))
		}
	}

	rules := flowRules(decls, results, out, mode)
	if !*noReport {
		if err := mergeFlowRules(rules); err != nil {
			fmt.Fprintf(os.Stderr, "sb flow: warning: recording premises in %s: %v\n", DischargeReportPath, err)
		}
		if err := recordFlowEngine(mode, host); err != nil {
			fmt.Fprintf(os.Stderr, "sb flow: warning: recording flow_engine in %s: %v\n", DischargeReportPath, err)
		}
	}

	violations := 0
	for _, r := range results {
		violations += len(r.Violations)
	}
	printFlowResults(results, out, mode, host)
	if len(disagreements) > 0 {
		fmt.Fprintf(os.Stderr,
			"sb flow: FAIL — the Shen Prolog rules and the Go engine disagree about %d premise(s):\n",
			len(disagreements))
		for _, d := range disagreements {
			fmt.Fprintf(os.Stderr, "      %s\n", d)
		}
		fmt.Fprintln(os.Stderr,
			"      The rules in sb/flow/stdlib.shen are the primary statement of what a\n"+
				"      flow premise means; cmd/sb/flow is a transcription of them. One of the\n"+
				"      two is wrong, and no premise here is evidence until they agree.")
		os.Exit(1)
	}
	if violations > 0 {
		os.Exit(1)
	}
}

// Flow engine modes. The strings are what the report's `flow_engine`
// field carries, so they are part of the schema.
const (
	FlowEngineGo   = "go"
	FlowEngineShen = "shen"
	FlowEngineBoth = "both"
)

// resolveFlowEngine turns the -engine flag into a mode plus, when one
// is needed, a resolved host.
//
// The default is `both` when a host is available and `go` when not.
// Defaulting to `both` is the point of W6: the Shen rules are the
// documented primary engine, and leaving them opt-in is how they went
// four workstreams without executing. Asking for `shen` or `both`
// explicitly with no host is an error rather than a silent downgrade,
// because a caller who named the engine wants that engine.
func resolveFlowEngine(cfg *Config, want string) (string, *ShenHost, error) {
	host := ResolveShenHost(cfg)
	switch want {
	case "", FlowEngineBoth:
		if host.Found() {
			return FlowEngineBoth, host, nil
		}
		if want == FlowEngineBoth {
			return "", nil, fmt.Errorf("-engine both needs a Shen host. %s", ShenInstallHint)
		}
		fmt.Fprintf(os.Stderr, "sb flow: no Shen host; running the Go engine alone. %s\n", ShenInstallHint)
		return FlowEngineGo, nil, nil
	case FlowEngineShen:
		if !host.Found() {
			return "", nil, fmt.Errorf("-engine shen needs a Shen host. %s", ShenInstallHint)
		}
		return FlowEngineShen, host, nil
	case FlowEngineGo:
		return FlowEngineGo, nil, nil
	default:
		return "", nil, fmt.Errorf("unknown -engine %q (want go, shen, or both)", want)
	}
}

// runShenFlowEngine materialises the embedded Prolog stdlib and runs
// it against the fact file.
//
// The stdlib is read from sb's embedded skilldata rather than from the
// checkout, so `sb flow --engine shen` works from an installed binary
// in a project that has no copy of sb/flow/. `make check-skilldata`
// keeps the embedded copy equal to the canonical one.
func runShenFlowEngine(host *ShenHost, factsPath string, decls []flow.Decl) ([]flow.ShenVerdict, string, error) {
	data, err := skilldata.ReadFile("skilldata/flow/stdlib.shen")
	if err != nil {
		return nil, "", fmt.Errorf("reading the embedded Prolog stdlib: %w", err)
	}
	dir, err := os.MkdirTemp("", "sb-flow-stdlib-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(dir)
	stdlib := filepath.Join(dir, "stdlib.shen")
	if err := os.WriteFile(stdlib, data, 0o644); err != nil {
		return nil, "", err
	}
	absFacts, err := filepath.Abs(factsPath)
	if err != nil {
		return nil, "", err
	}
	return flow.EvaluateShen(flow.ShenHostRunner{
		Path:       host.Path,
		StdlibPath: stdlib,
		Timeout:    5 * time.Minute,
	}, absFacts, decls)
}

func shenVerdictWord(v flow.ShenVerdict) string {
	switch {
	case v.Violation:
		return "violated"
	case v.Vacuous():
		return "vacuous"
	default:
		return "discharged"
	}
}

// recordFlowEngine stores which engine(s) ran, and which host, in the
// discharge report. Additive and omitempty: a report from a run that
// never had a flow gate marshals exactly as it did before W6.
func recordFlowEngine(mode string, host *ShenHost) error {
	r, err := loadDischarge(DischargeReportPath)
	if err != nil || r == nil {
		return err
	}
	r.FlowEngine = mode
	if host.Found() {
		if r.Toolchain == nil {
			r.Toolchain = &DischargeToolchain{}
		}
		r.Toolchain.ShenHost = host.Name
		r.Toolchain.ShenHostVersion = host.Version
	}
	return writeDischarge(DischargeReportPath, r)
}

// runFlowFallback executes the legacy grep gate, records every
// premise as unproven with basis "grep-fallback", and returns the exit
// code the gate should carry. A failing grep is still a failing gate:
// degrading the *evidence* must not degrade the *enforcement*.
func runFlowFallback(cfg *Config, decls []flow.Decl, out *IndexOutcome, fallback string, noReport bool) int {
	fmt.Fprintf(os.Stderr,
		"sb flow: WARNING: no SCIP indexer on PATH (%s). Flow premises cannot be discharged;\n"+
			"         install it with: %s\n", out.Indexer, out.InstallHint)

	rules := flowRules(decls, nil, out, "")
	if !noReport {
		if err := mergeFlowRules(rules); err != nil {
			fmt.Fprintf(os.Stderr, "sb flow: warning: recording premises in %s: %v\n", DischargeReportPath, err)
		}
	}

	if fallback == "" {
		fmt.Fprintf(os.Stderr, "sb flow: no -fallback command configured; premises recorded unproven (%s)\n",
			DischargeBasisGrepFallback)
		return 0
	}
	fmt.Fprintf(os.Stderr, "sb flow: falling back to the legacy grep gate: %s\n", fallback)
	bin, argv := SplitCommand(fallback)
	cmd := exec.Command(bin, argv...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if s := strings.TrimSpace(buf.String()); s != "" {
		fmt.Fprintln(os.Stderr, s)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb flow: legacy grep gate failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "sb flow: legacy grep gate passed; premises remain unproven (%s)\n",
		DischargeBasisGrepFallback)
	return 0
}

// loadFlowDecls reads the (flow …) forms from the project spec and
// from every spec named by [[derive.specs]], de-duplicated by name.
func loadFlowDecls(cfg *Config) ([]flow.Decl, error) {
	paths := []string{cfg.Spec}
	for _, s := range cfg.DeriveSpecs {
		paths = append(paths, s.Path)
	}
	seenPath := map[string]bool{}
	seenName := map[string]bool{}
	var out []flow.Decl
	for _, p := range paths {
		if p == "" || seenPath[p] {
			continue
		}
		seenPath[p] = true
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		decls, err := flow.ParseSpec(p, string(data))
		if err != nil {
			return nil, err
		}
		for _, d := range decls {
			if seenName[d.Name] {
				continue
			}
			seenName[d.Name] = true
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// flowRules turns evaluation results into discharge-report rules, one
// per (flow …) declaration. results may be nil, which is the
// no-indexer case: every premise is then unproven with basis
// "grep-fallback".
func flowRules(decls []flow.Decl, results []flow.Result, out *IndexOutcome, engine string) []DischargeRule {
	byID := map[string]flow.Result{}
	for _, r := range results {
		byID[r.PremiseID] = r
	}
	var rules []DischargeRule
	for _, d := range decls {
		rule := DischargeRule{
			Name:                   d.Name,
			Kind:                   FlowRuleKind,
			SpecFile:               d.SpecFile,
			SpecExcerpt:            d.Raw,
			HumanDescription:       flowRuleDescription(d),
			HumanDescriptionSource: "auto",
			Status:                 DischargeStatusDischarged,
			CounterExamples:        []DischargeCounter{},
		}
		for _, p := range d.Premises {
			res, ran := byID[p.ID()]
			prem := DischargePremise{
				ID:         p.ID(),
				Expression: p.Expression(),
			}
			switch {
			case !ran:
				prem.Discharge = DischargeUnproven
				prem.DischargeBasis = DischargeBasisGrepFallback
				prem.Rationale = fmt.Sprintf(
					"No SCIP indexer (%s) was available, so this premise was not evaluated over a resolved symbol graph. The legacy regex gate ran instead; a regex cannot see through an aliased import, so this is not evidence for the premise.",
					out.Indexer)
			case res.Discharged:
				prem.Discharge = DischargeStatic
				prem.DischargeBasis = DischargeBasisFlow
				prem.Rationale = res.Rationale + fmt.Sprintf(" Engine: %s; index: %s.", engine, out.Indexer)
			case res.Vacuous:
				prem.Discharge = DischargeUnproven
				prem.DischargeBasis = DischargeBasisFlow
				prem.Rationale = res.Rationale + fmt.Sprintf(" Engine: %s; index: %s.", engine, out.Indexer)
			default:
				prem.Discharge = DischargeUnproven
				prem.DischargeBasis = DischargeBasisFlow
				prem.Rationale = res.Rationale + fmt.Sprintf(" Engine: %s; index: %s.", engine, out.Indexer)
			}
			for _, v := range res.Violations {
				prem.CodeReferences = append(prem.CodeReferences, v.Location)
				rule.CounterExamples = append(rule.CounterExamples, flowCounterExample(p.ID(), v))
			}
			if prem.Discharge != DischargeStatic {
				if len(res.Violations) > 0 {
					rule.Status = DischargeStatusViolated
				} else if rule.Status != DischargeStatusViolated {
					rule.Status = DischargeStatusUnproven
				}
			}
			rule.Premises = append(rule.Premises, prem)
		}
		// W5.4 — precision and blame. A flow premise the analysis
		// discharged is static evidence: nothing had to run. A
		// violation is the implementation's — the handler that
		// reaches the sink without the proof is impl code — and the
		// basis names the analysis that found it rather than an
		// oracle that did not exist.
		for i := range rule.Premises {
			rule.Premises[i].Precision = precisionForBasis(
				rule.Premises[i].Discharge, rule.Premises[i].DischargeBasis)
		}
		for i := range rule.CounterExamples {
			rule.CounterExamples[i].Blame = BlameImpl
			rule.CounterExamples[i].BlameBasis = BlameBasisFlowAnalysis
		}
		rules = append(rules, rule)
	}
	return rules
}

func flowCounterExample(premiseID string, v flow.Violation) DischargeCounter {
	line := 0
	if parts := strings.Split(v.Location, ":"); len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &line)
	}
	file := v.Location
	if i := strings.Index(v.Location, ":"); i >= 0 {
		file = v.Location[:i]
	}
	input := map[string]string{
		"premise":   premiseID,
		"reference": v.Location,
		"symbol":    v.Symbol,
		"enclosing": v.Enclosing,
	}
	if len(v.Path) > 0 {
		input["shortest_violating_path"] = strings.Join(v.Path, " -> ")
	}
	lineHint := &line
	if line == 0 {
		lineHint = nil
	}
	return DischargeCounter{
		CaseID:       premiseID + "@" + v.Location,
		Input:        input,
		SpecOutput:   "no such reference exists",
		ImplOutput:   "reference exists at " + v.Location,
		ImplFunction: v.Enclosing,
		ImplFile:     file,
		ImplLineHint: lineHint,
		Rationale:    v.Rationale,
	}
}

func flowRuleDescription(d flow.Decl) string {
	var parts []string
	for _, p := range d.Premises {
		switch prem := p.(type) {
		case flow.ConstructorOnly:
			parts = append(parts, fmt.Sprintf(
				"only %s may reference the constructor %s", joinStrings(patternStrings(prem.Allowed)), prem.Ctor.String()))
		case flow.MustPassThrough:
			parts = append(parts, fmt.Sprintf(
				"every call path from %s to %s references %s first", prem.Source.String(), prem.Sink.String(), prem.Proof.String()))
		}
	}
	return "Flow discipline: " + joinStrings(parts) + "."
}

func patternStrings(pats []flow.Pattern) []string {
	out := make([]string, len(pats))
	for i, p := range pats {
		out[i] = p.String()
	}
	return out
}

func joinStrings(parts []string) string {
	switch len(parts) {
	case 0:
		return "nothing"
	case 1:
		return parts[0]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}

// mergeFlowRules writes the flow rules into the discharge report,
// replacing any flow rules a previous run left there and leaving
// every derive-produced rule untouched. A missing report is created:
// the flow gate can run without `sb derive` having run first.
func mergeFlowRules(rules []DischargeRule) error {
	r, err := loadDischarge(DischargeReportPath)
	if err != nil {
		return err
	}
	if r == nil {
		r = &DischargeReport{
			SchemaVersion: 1,
			Spec:          DischargeSpec{Files: []DischargeSpecFile{}},
			Impl:          DischargeImpl{TargetLanguages: []string{}},
			Rules:         []DischargeRule{},
			Tools:         DischargeTools{SBVersion: version},
		}
	}
	kept := make([]DischargeRule, 0, len(r.Rules)+len(rules))
	for _, existing := range r.Rules {
		if existing.Kind == FlowRuleKind {
			continue
		}
		kept = append(kept, existing)
	}
	kept = append(kept, rules...)
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	r.Rules = kept
	r.Spec.RuleCount = len(r.Rules)
	r.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if r.Tools.SBVersion == "" {
		r.Tools.SBVersion = version
	}
	fillImplGit(r)
	r.Summary = computeDischargeSummary(r.Rules)
	return writeDischarge(DischargeReportPath, r)
}

// carryFlowRules copies flow rules from the report already on disk
// into r, which `sb derive` is about to write. Rules r already has
// under the same name win, so a fresh flow run is never overwritten
// by a stale one. Missing or unreadable reports are simply nothing to
// carry.
func carryFlowRules(r *DischargeReport) {
	prev, err := loadDischarge(DischargeReportPath)
	if err != nil || prev == nil {
		return
	}
	// W6 — which engine(s) evaluated the premises travels with them.
	// `sb derive` writes the report from scratch and runs last, so
	// without this the flow_engine field a `sb flow` run recorded
	// minutes earlier is dropped and the report goes silent about
	// whether the Shen rules or only their Go transcription ran.
	if r.FlowEngine == "" && prev.FlowEngine != "" {
		r.FlowEngine = prev.FlowEngine
	}
	have := map[string]bool{}
	for _, rule := range r.Rules {
		have[rule.Name] = true
	}
	added := false
	for _, rule := range prev.Rules {
		if rule.Kind != FlowRuleKind || have[rule.Name] {
			continue
		}
		r.Rules = append(r.Rules, rule)
		added = true
	}
	if !added {
		return
	}
	sort.SliceStable(r.Rules, func(i, j int) bool { return r.Rules[i].Name < r.Rules[j].Name })
	r.Spec.RuleCount = len(r.Rules)
	r.Summary = computeDischargeSummary(r.Rules)
}

// printFlowResults renders the human-facing gate output.
func printFlowResults(results []flow.Result, out *IndexOutcome, mode string, host *ShenHost) {
	for _, res := range results {
		status := "PASS"
		switch {
		case len(res.Violations) > 0:
			status = "FAIL"
		case res.Vacuous:
			status = "VACUOUS"
		}
		fmt.Fprintf(os.Stderr, "%s  %s\n      %s\n", status, res.PremiseID, res.Rationale)
		for _, v := range res.Violations {
			fmt.Fprintf(os.Stderr, "      %s: %s\n", v.Location, v.Rationale)
			if len(v.Path) > 0 {
				fmt.Fprintf(os.Stderr, "        shortest violating path: %s\n", strings.Join(v.Path, " → "))
			}
		}
	}
	hostNote := ""
	if host.Found() {
		hostNote = " host=" + host.Name
	}
	fmt.Fprintf(os.Stderr, "sb flow: engine=%s%s index=%s facts=%s\n", mode, hostNote, out.Indexer, out.FactsPath)
}
