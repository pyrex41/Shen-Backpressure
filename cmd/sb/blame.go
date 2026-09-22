package main

// blame.go — W5.4, the sb half. shen-derive assigns precision and
// blame for what it classifies; sb does the same for everything it
// adds afterwards: counter-examples parsed out of `go test`, flow
// premises, and any premise whose discharge sb downgraded.
//
// The rules are the roadmap's, and they are applied in the order it
// gives them:
//
//	vacuous rule                            → spec
//	:runtime-via failure                    → wrapper
//	evaluator and Shen host disagree        → lowering
//	evaluator and host agree, impl differs  → impl
//	no host available, impl differs         → impl, blame_basis
//	                                          "evaluator-only"
//
// The last row is this repository's situation: there is no Shen host
// installed, so "the spec says X" means "the Go evaluator says X". A
// lowering bug and an implementation bug produce identical evidence
// under those conditions. Recording `evaluator-only` rather than
// quietly claiming `impl` is the difference between a report that
// knows what it knows and one that does not.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BlameBasisFlowAnalysis is recorded on a counter-example the flow
// gate produced: the violating reference is in the implementation, and
// a resolved symbol graph — not an oracle — is what found it.
const BlameBasisFlowAnalysis = "flow-analysis"

// BrandTablePath is where `sb gen` writes shengen's brand table.
// Under .sb/ with the other per-run artifacts, so it is regenerated
// alongside the guards file and cannot go stale relative to it.
const BrandTablePath = ".sb/brand_table.json"

// ensureBrandTable makes sure .sb/brand_table.json describes the
// project's current spec, and returns its path (empty when the
// project does not use brands, or when shengen is unavailable).
//
// `sb gen` writes the table as a side effect, but a project whose
// codegen gate is a shell script calling shengen directly — which is
// both examples — never goes through `sb gen`. Rather than require
// every such script to learn a new flag, sb derives the table itself
// when it needs one. The call is read-only with respect to the guards
// file: shengen writes the table and nothing else.
func ensureBrandTable(cfg *Config) string {
	if cfg == nil || !cfg.Brands || cfg.Lang != "go" || cfg.Spec == "" {
		return ""
	}
	shengen, err := FindShengen()
	if err != nil {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(BrandTablePath), 0o755); err != nil {
		return ""
	}
	cmd := exec.Command(shengen, "--spec", cfg.Spec, "--pkg", cfg.Pkg,
		"--brands", "--brand-table", BrandTablePath, "--dry-run")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return ""
	}
	abs, err := filepath.Abs(BrandTablePath)
	if err != nil {
		return BrandTablePath
	}
	return abs
}

// precisionForBasis maps a premise's discharge and basis onto the
// total order. Mirrors report.PrecisionFor; the two modules cannot
// import each other.
func precisionForBasis(discharge, basis string) string {
	// An unproven premise has no evidence whatever its basis field
	// says. The flow gate, for instance, records basis
	// "flow-analysis" on a premise the analysis *refuted*.
	if discharge == DischargeUnproven {
		return PrecisionUnproven
	}
	switch basis {
	case BasisGuardBrandBound, BasisGuardTypeAtBoundary, BasisGuardConstructorValidates:
		return PrecisionStatic
	case BasisProverZ3PathCover:
		return PrecisionPathCover
	case BasisShenDeriveSampled:
		return PrecisionSampled
	case DischargeBasisFlow:
		// A flow premise is discharged by an analysis of the resolved
		// symbol graph, before anything runs. That is a static claim
		// in the sense that matters here: no execution was needed.
		return PrecisionStatic
	case BasisVacuousDatatype, BasisNotDischarged, DischargeBasisGrepFallback:
		return PrecisionUnproven
	}
	switch discharge {
	case DischargeStatic:
		return PrecisionStatic
	case DischargeRuntimeSampled:
		return PrecisionSampled
	case DischargeRuntimeAttested, DischargeRuntimeEvaluator,
		DischargeRuntimeAttestedSampled, DischargeRuntimeAttestedDB:
		return PrecisionRuntime
	}
	return PrecisionUnproven
}

// applyPrecision fills Precision on every premise in the report.
// Derived, so it is safe to call repeatedly and must be called after
// any mutation of discharge or basis.
func applyPrecision(r *DischargeReport) {
	for i := range r.Rules {
		for j := range r.Rules[i].Premises {
			p := &r.Rules[i].Premises[j]
			p.Precision = precisionForBasis(p.Discharge, p.DischargeBasis)
		}
	}
}

// weakestPrecision returns the weakest precision anywhere in the
// report — its headline claim, since a chain of reasoning is only as
// strong as its weakest link. Empty when nothing is classified.
func weakestPrecision(r *DischargeReport) string {
	weakest := ""
	for _, rule := range r.Rules {
		for _, p := range rule.Premises {
			if p.Precision == "" {
				continue
			}
			if weakest == "" || PrecisionRank(p.Precision) > PrecisionRank(weakest) {
				weakest = p.Precision
			}
		}
	}
	return weakest
}

// assignBlame fills blame and blame_basis on every counter-example.
//
// shenHostAvailable says whether a live Shen host was consulted
// alongside the spec's own Go evaluator. With one, an impl
// disagreement is unambiguously the impl's (both oracles agreed on the
// spec's meaning). Without one, the assignment is the same but the
// basis records that only one oracle spoke.
func assignBlame(r *DischargeReport, shenHostAvailable bool) {
	for i := range r.Rules {
		rule := &r.Rules[i]
		vacuous := rule.Status == DischargeStatusVacuous
		runtimeVia := ruleHasRuntimeVia(rule)
		for j := range rule.CounterExamples {
			ce := &rule.CounterExamples[j]
			// A counter-example that already names a party keeps it:
			// the flow gate assigns its own blame at evaluation time,
			// where it knows things this pass does not.
			if ce.Blame != "" {
				continue
			}
			blame, basis := blameFor(vacuous, runtimeVia, shenHostAvailable)
			ce.Blame = blame
			ce.BlameBasis = basis
		}
	}
}

// blameFor is the rule table, extracted so it can be tested without
// building a report.
func blameFor(vacuous, runtimeVia, shenHostAvailable bool) (blame, basis string) {
	switch {
	case vacuous:
		return BlameSpec, BlameBasisVacuous
	case runtimeVia:
		return BlameWrapper, BlameBasisRuntimeVia
	case shenHostAvailable:
		// A live host was consulted and agreed with the spec's Go
		// evaluator — otherwise the harness would have reported the
		// disagreement itself and the rule would carry a lowering
		// finding instead of a counter-example.
		return BlameImpl, BlameBasisEvaluatorAndHost
	default:
		return BlameImpl, BlameBasisEvaluatorOnly
	}
}

// ruleHasRuntimeVia reports whether any premise of the rule is
// discharged through a :runtime-via checker, which makes a failure
// wrapper code's rather than the implementation's.
func ruleHasRuntimeVia(rule *DischargeRule) bool {
	for _, p := range rule.Premises {
		switch p.Discharge {
		case DischargeRuntimeAttested, DischargeRuntimeEvaluator,
			DischargeRuntimeAttestedSampled, DischargeRuntimeAttestedDB:
			return true
		}
	}
	return false
}

// detectShenRuntimeHost reports whether a *live Shen host* is
// available to act as a second oracle, which is the condition the
// blame rules care about.
//
// Before W6 this was `detectShenRuntime(cfg) == "shen-sbcl"`, which
// asks a different question entirely: detectShenRuntime scans the spec
// for `:runtime-via` markers and names the runtime the *report* should
// mention. A project could therefore be sitting next to a working Shen
// host and still be told it had none, because its spec used
// `:runtime-via :eval`; and a project that named a checker got
// `evaluator-and-host` without anything having asked a host anything.
//
// The condition that matters is simply "can sb run the spec somewhere
// other than its own evaluator", so that is what this asks now. What
// the host is then actually asked is in oracle.go.
func detectShenRuntimeHost(cfg *Config) bool {
	return ResolveShenHost(cfg).Found()
}

// blamedParties returns the distinct blame values in the report, in
// the order they first appear. Used by `sb context` to lead its
// summary with the responsible party.
func blamedParties(r *DischargeReport) []string {
	var out []string
	for _, rule := range r.Rules {
		for _, ce := range rule.CounterExamples {
			if ce.Blame == "" {
				continue
			}
			out = appendUniqueStr(out, ce.Blame)
		}
	}
	return out
}

// blameSummaryLine is the one sentence `sb context` leads with when
// anything is broken: who is responsible, and on what basis.
func blameSummaryLine(r *DischargeReport) string {
	parties := blamedParties(r)
	if len(parties) == 0 {
		return ""
	}
	labels := make([]string, 0, len(parties))
	for _, p := range parties {
		labels = append(labels, BlameLabel(p))
	}
	basis := firstBlameBasis(r)
	line := "BLAME: " + strings.Join(labels, "; ")
	if basis != "" {
		line += " [basis: " + basis + "]"
	}
	return line
}

func firstBlameBasis(r *DischargeReport) string {
	for _, rule := range r.Rules {
		for _, ce := range rule.CounterExamples {
			if ce.BlameBasis != "" {
				return ce.BlameBasis
			}
		}
	}
	return ""
}
