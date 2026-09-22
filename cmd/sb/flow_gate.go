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
		os.Exit(runFlowFallback(cfg, decls, out, *fallback))
	}

	results := flow.Evaluate(out.Facts, decls)
	rules := flowRules(decls, results, out, flow.EngineGo)
	if err := mergeFlowRules(rules); err != nil {
		fmt.Fprintf(os.Stderr, "sb flow: warning: recording premises in %s: %v\n", DischargeReportPath, err)
	}

	violations := 0
	for _, r := range results {
		violations += len(r.Violations)
	}
	printFlowResults(results, out)
	if violations > 0 {
		os.Exit(1)
	}
}

// runFlowFallback executes the legacy grep gate, records every
// premise as unproven with basis "grep-fallback", and returns the exit
// code the gate should carry. A failing grep is still a failing gate:
// degrading the *evidence* must not degrade the *enforcement*.
func runFlowFallback(cfg *Config, decls []flow.Decl, out *IndexOutcome, fallback string) int {
	fmt.Fprintf(os.Stderr,
		"sb flow: WARNING: no SCIP indexer on PATH (%s). Flow premises cannot be discharged;\n"+
			"         install it with: %s\n", out.Indexer, out.InstallHint)

	rules := flowRules(decls, nil, out, "")
	if err := mergeFlowRules(rules); err != nil {
		fmt.Fprintf(os.Stderr, "sb flow: warning: recording premises in %s: %v\n", DischargeReportPath, err)
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

// printFlowResults renders the human-facing gate output.
func printFlowResults(results []flow.Result, out *IndexOutcome) {
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
	fmt.Fprintf(os.Stderr, "sb flow: engine=%s index=%s facts=%s\n", flow.EngineGo, out.Indexer, out.FactsPath)
}
