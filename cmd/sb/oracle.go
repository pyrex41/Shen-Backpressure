package main

// oracle.go — W6.D. The second oracle, and the `lowering` blame value
// it makes reachable.
//
// W5 defined four blame values — spec, impl, wrapper, lowering — and
// could only ever produce three. The rules say:
//
//	evaluator and Shen host agree, impl differs  → impl
//	evaluator and Shen host disagree             → lowering
//
// so `lowering` needs a Shen host, and there was none. Every W5 report
// therefore carries `blame_basis: evaluator-only`, which is the honest
// way to say "one oracle spoke, and a lowering bug and an
// implementation bug look identical from here".
//
// With a host, the second oracle is the spec itself. For a failing
// case, sb asks the host to evaluate the spec's `(define …)` on that
// case's inputs and compares the answer with the one shen-derive's Go
// evaluator computed:
//
//	they agree     the spec means what the evaluator said, so the
//	               implementation is what differs from it → impl, with
//	               blame_basis evaluator-and-host
//	they disagree  the two things that are supposed to be the same
//	               reading of one spec are not, so the finding is
//	               about the lowering, not about the impl → lowering
//
// A disagreement is the more interesting outcome and the less likely
// one, which is exactly why it has to be looked for rather than
// assumed away.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ShenSamplesDir is where `sb derive` puts the Shen-rendered sample
// tables. Under .sb/, regenerated on every derive run, so a table can
// never describe an older spec than the one just verified.
const ShenSamplesDir = ".sb"

// shenSamplesPath is the sidecar path for one define.
func shenSamplesPath(funcName string) string {
	return filepath.Join(ShenSamplesDir, "shen_samples."+sanitizeFileToken(funcName)+".json")
}

// sanitizeFileToken makes a Shen define name safe as a filename
// component. Define names carry `?`, `-` and `!`.
func sanitizeFileToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// shenSampleCase mirrors verify.ShenSampleCase.
type shenSampleCase struct {
	Name       string   `json:"name"`
	Args       []string `json:"args"`
	Expected   string   `json:"expected"`
	Provenance string   `json:"provenance,omitempty"`
}

// shenSampleFile mirrors verify.ShenSampleFile.
type shenSampleFile struct {
	Func    string           `json:"func"`
	Spec    string           `json:"spec"`
	Cases   []shenSampleCase `json:"cases"`
	Skipped []string         `json:"skipped,omitempty"`
}

func loadShenSamples(path string) (*shenSampleFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f shenSampleFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// OracleVerdict is what the host said about one case.
type OracleVerdict struct {
	CaseID string
	// HostOutput is the host's answer, normalised.
	HostOutput string
	// EvaluatorOutput is what shen-derive's evaluator computed for the
	// same case, from the sample table.
	EvaluatorOutput string
	// Agree is true when the two oracles read the spec the same way.
	Agree bool
	// Err is non-nil when the host could not be asked — a missing
	// sample, a host failure. An unanswered case is left with the
	// single-oracle blame rather than guessed at.
	Err error
}

// ShenOracle asks a Shen host what a spec's define evaluates to.
//
// One host process per batch of cases, not per case: the goal
// expressions are passed as successive -e arguments after the prelude
// and the spec, which the host evaluates in order. Starting a Shen
// image costs more than every goal in a report put together.
type ShenOracle struct {
	Host    *ShenHost
	Timeout time.Duration
}

// oracleMarker prefixes the host's answers so its own chatter — load
// banners, run times, the printed value of each form — can be ignored.
const oracleMarker = "W6ORACLE"

// AskCases evaluates `(<func> <arg>…)` on the host for each named
// case and returns one verdict per case, in the order asked.
//
// The prelude is loaded before `(tc +)` is *not* issued at all here:
// this is evaluation, not typechecking, and gate 4 already made the
// typing claim. The prelude's declares are still loaded, because
// without them the host has no `val` and no field accessors to
// evaluate the body with.
func (o ShenOracle) AskCases(cfg *Config, samples *shenSampleFile, caseIDs []string) ([]OracleVerdict, error) {
	if !o.Host.Found() {
		return nil, fmt.Errorf("no Shen host")
	}
	byName := map[string]shenSampleCase{}
	for _, c := range samples.Cases {
		byName[c.Name] = c
	}

	var asked []shenSampleCase
	verdicts := make([]OracleVerdict, 0, len(caseIDs))
	for _, id := range caseIDs {
		c, ok := byName[id]
		if !ok {
			verdicts = append(verdicts, OracleVerdict{
				CaseID: id,
				Err:    fmt.Errorf("case %s is not in the Shen sample table (it may have no Shen literal: %s)", id, strings.Join(samples.Skipped, "; ")),
			})
			continue
		}
		asked = append(asked, c)
		verdicts = append(verdicts, OracleVerdict{CaseID: id, EvaluatorOutput: normalizeOracleValue(c.Expected)})
	}
	if len(asked) == 0 {
		return verdicts, nil
	}

	// The host needs the intrinsics the define's body calls — with
	// bodies, not just types, so this loads the prelude's *eval* half
	// and not its declares. Nothing turns the typechecker on: gate 4
	// already made the typing claim, and this run is about values.
	_, defines, err := ensurePrelude(cfg, samples.Spec)
	if err != nil {
		return verdicts, err
	}
	var files []string
	if defines != "" {
		files = append(files, preludeEvalPath())
	}
	files = append(files, samples.Spec)

	var exprs []string
	for _, c := range asked {
		exprs = append(exprs, fmt.Sprintf(`(output "%s ~A ~A~%%" "%s" (%s %s))`,
			oracleMarker, c.Name, samples.Func, strings.Join(c.Args, " ")))
	}
	// Evaluation order: the prelude's declares, then the prelude's
	// defines and the spec, then the goals. ShenEvalArgs puts -e
	// expressions before -l files, so the goals go in a file of their
	// own rather than as -e arguments.
	goalDir, err := os.MkdirTemp("", "sb-oracle-")
	if err != nil {
		return verdicts, err
	}
	defer os.RemoveAll(goalDir)
	goalPath := filepath.Join(goalDir, "goals.shen")
	if err := os.WriteFile(goalPath, []byte(strings.Join(exprs, "\n")+"\n"), 0o644); err != nil {
		return verdicts, err
	}
	files = append(files, goalPath)

	args := ShenEvalArgs(nil, nil, files)
	cmd := exec.Command(o.Host.Path, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return verdicts, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		runErr = fmt.Errorf("the Shen host timed out after %s", timeout)
	}
	out := buf.String()

	answers := parseOracleAnswers(out)
	for i := range verdicts {
		v := &verdicts[i]
		if v.Err != nil {
			continue
		}
		got, ok := answers[v.CaseID]
		if !ok {
			why := runErr
			if why == nil {
				why = fmt.Errorf("the host printed no answer")
			}
			v.Err = fmt.Errorf("case %s: %v", v.CaseID, why)
			continue
		}
		v.HostOutput = got
		v.Agree = got == v.EvaluatorOutput
	}
	return verdicts, nil
}

// parseOracleAnswers reads the marker lines.
func parseOracleAnswers(out string) map[string]string {
	answers := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(l, oracleMarker+" ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(l, oracleMarker+" "))
		sp := strings.IndexByte(rest, ' ')
		if sp < 0 {
			continue
		}
		answers[rest[:sp]] = normalizeOracleValue(rest[sp+1:])
	}
	return answers
}

// normalizeOracleValue makes the two oracles' answers comparable.
//
// The host prints a Shen value with its own writer and the sample
// table carries a Shen literal, so the two differ in whitespace and in
// how a list is punctuated. Comparing the *normalised* forms is the
// difference between finding a real disagreement and finding a
// formatting difference — and a formatting difference reported as
// `lowering` would be worse than no second oracle at all.
func normalizeOracleValue(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	s = strings.TrimSpace(s)
	repl := strings.NewReplacer("[", " [ ", "]", " ] ", ",", " ")
	s = repl.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// applySecondOracle consults the host about every behavioral
// counter-example in the report and rewrites blame accordingly.
//
// Counter-examples the flow gate produced already name a party and are
// left alone (their blame_basis is flow-analysis, and a symbol graph is
// not an oracle about a define's value). Cases the host could not be
// asked about keep the single-oracle blame, and the reason is printed:
// an unanswered case must not silently become evidence of agreement.
func applySecondOracle(cfg *Config, r *DischargeReport, host *ShenHost) {
	if !host.Found() || r == nil {
		return
	}
	oracle := ShenOracle{Host: host}
	for i := range r.Rules {
		rule := &r.Rules[i]
		if rule.Kind == FlowRuleKind || len(rule.CounterExamples) == 0 {
			continue
		}
		samples, err := loadShenSamples(shenSamplesPath(rule.Name))
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"sb derive: second oracle: no Shen sample table for %s (%v); blame stays single-oracle\n",
				rule.Name, err)
			continue
		}
		ids := make([]string, 0, len(rule.CounterExamples))
		for _, ce := range rule.CounterExamples {
			ids = append(ids, ce.CaseID)
		}
		verdicts, err := oracle.AskCases(cfg, samples, ids)
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"sb derive: second oracle: %s: %v; blame stays single-oracle\n", rule.Name, err)
			continue
		}
		byCase := map[string]OracleVerdict{}
		for _, v := range verdicts {
			byCase[v.CaseID] = v
		}
		for j := range rule.CounterExamples {
			ce := &rule.CounterExamples[j]
			v, ok := byCase[ce.CaseID]
			if !ok {
				continue
			}
			if v.Err != nil {
				fmt.Fprintf(os.Stderr,
					"sb derive: second oracle: %s/%s not consulted: %v\n", rule.Name, ce.CaseID, v.Err)
				continue
			}
			// Record what the host was actually asked, so an auditor
			// can re-run it by hand.
			if ce.Input != nil {
				if c := findSampleCase(samples, ce.CaseID); c != nil {
					ce.Input["shen_goal"] = "(" + samples.Func + " " + strings.Join(c.Args, " ") + ")"
				}
			}
			if v.Agree {
				ce.Blame = BlameImpl
				ce.BlameBasis = BlameBasisEvaluatorAndHost
				ce.Rationale = strings.TrimSpace(ce.Rationale) +
					fmt.Sprintf(" The Shen host (%s) evaluates the spec on this input to %s, agreeing with shen-derive's evaluator, so the implementation is what differs from the spec.",
						host.Name, v.HostOutput)
				continue
			}
			ce.Blame = BlameLowering
			ce.BlameBasis = BlameBasisEvaluatorAndHost
			ce.Rationale = strings.TrimSpace(ce.Rationale) +
				fmt.Sprintf(" The two oracles for this spec DISAGREE: shen-derive's evaluator says %s, the Shen host (%s) says %s. Until they agree, this case says nothing about the implementation — the fault is in the lowering from the spec to one of the two readings of it.",
					v.EvaluatorOutput, host.Name, v.HostOutput)
			fmt.Fprintf(os.Stderr,
				"sb derive: LOWERING: %s/%s — evaluator says %s, host says %s\n",
				rule.Name, ce.CaseID, v.EvaluatorOutput, v.HostOutput)
		}
	}
}

func findSampleCase(f *shenSampleFile, name string) *shenSampleCase {
	for i := range f.Cases {
		if f.Cases[i].Name == name {
			return &f.Cases[i]
		}
	}
	return nil
}
