package flow

// shen.go — W6.C. Running the Shen Prolog rules for real.
//
// sb/flow/stdlib.shen has been described as the *primary* flow engine
// since W3, with the Go evaluator in engine.go as its fallback. It had
// never executed: the file defined `(define call …)`, and `call` is a
// Shen system function, so loading it failed on the first fact
// predicate. The rename to `calls` (facts.go) fixed the load; this file
// is what actually asks the rules a question.
//
// What the Shen engine returns is deliberately coarser than what the Go
// engine returns: a verdict per premise, not a violation list. Shen's
// `prolog?` answers "is this goal satisfiable", which is exactly a
// verdict and is not a set of solutions. Two engines agreeing on
// verdicts is the check worth having — it says the rules and their Go
// transcription decide the same thing — and the Go engine remains the
// one that produces the file:line and the shortest violating path for
// the report, because it can.
//
// Two verdicts per premise, not one:
//
//	considered   did the premise range over anything at all? A premise
//	             whose subject is absent from the index is vacuous, and
//	             a vacuous premise passing for a discharge is the
//	             failure mode both engines have to agree about.
//	violation    is there a counterexample?

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ShenVerdict is the Shen engine's answer for one premise.
type ShenVerdict struct {
	PremiseID string
	// Considered is false when nothing in the fact base matched the
	// premise's subject — the vacuity case.
	Considered bool
	// Violation is true when the rules found a counterexample.
	Violation bool
}

// Discharged mirrors Result.Discharged: ranged over something, found
// nothing wrong.
func (v ShenVerdict) Discharged() bool { return v.Considered && !v.Violation }

// Vacuous mirrors Result.Vacuous.
func (v ShenVerdict) Vacuous() bool { return !v.Considered }

// ShenHostRunner is the little the Shen engine needs to know about a
// host: where the binary is, and how long to wait. cmd/sb resolves it
// (ResolveShenHost) and hands it here, so this package has no opinion
// about which port is installed.
type ShenHostRunner struct {
	// Path is the host binary.
	Path string
	// StdlibPath is sb/flow/stdlib.shen on disk. sb writes the copy it
	// embeds to a temp file, so this works from an installed binary
	// with no checkout.
	StdlibPath string
	// Timeout kills the host. Shen's Prolog is a backtracking search
	// over a list-shaped fact base; on a monorepo it will not finish,
	// and the documented escape hatch is Soufflé. A timeout is how
	// that shows up rather than a hung gate.
	Timeout time.Duration
}

// queryMarker prefixes every line the generated query file prints, so
// the host's own chatter (load banners, run times, the value of each
// top-level form) can be ignored.
const queryMarker = "W6FLOW"

// EvaluateShen loads the stdlib, the facts, and a generated query file
// into the Shen host and returns one verdict per premise.
//
// factsPath is the fact file `sb index` wrote. Loading it *is*
// asserting the facts: the stdlib defines def/ref/calls as functions
// that push onto three globals, which is why no parser ships with the
// rules.
func EvaluateShen(runner ShenHostRunner, factsPath string, decls []Decl) ([]ShenVerdict, string, error) {
	premises := orderedPremises(decls)
	if len(premises) == 0 {
		return nil, "", nil
	}

	dir, err := os.MkdirTemp("", "sb-flow-shen-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(dir)

	queryPath := filepath.Join(dir, "query.shen")
	if err := os.WriteFile(queryPath, []byte(renderQuery(premises)), 0o644); err != nil {
		return nil, "", err
	}

	// No `(tc +)` here. The rules are Prolog over untyped facts, and
	// Shen's typechecker has nothing to say about `defprolog`; running
	// it would only slow the load down.
	args := []string{"eval", "-q",
		"-l", runner.StdlibPath,
		"-l", factsPath,
		"-l", queryPath,
	}
	cmd := exec.Command(runner.Path, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timeout := runner.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	select {
	case err = <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return nil, buf.String(), fmt.Errorf("the Shen engine did not finish in %s; Shen's Prolog is a backtracking search over a list-shaped fact base and this one is too big for it (see docs/FLOW.md: the escape hatch is to emit Soufflé from the same rule text)", timeout)
	}
	out := buf.String()
	if err != nil {
		return nil, out, fmt.Errorf("the Shen host failed: %w", err)
	}

	verdicts, err := parseVerdicts(premises, out)
	return verdicts, out, err
}

// orderedPremises flattens the declarations into the evaluation order,
// which is also the order the query file prints in.
func orderedPremises(decls []Decl) []Premise {
	var out []Premise
	for _, d := range decls {
		out = append(out, d.Premises...)
	}
	return out
}

// renderQuery writes one Shen file that asks both questions about
// every premise and prints the answers under queryMarker.
//
// `allowed-caller?` is redefined immediately before each
// constructor-only query rather than once for the file: it is a
// per-premise allowlist, and the stdlib's comment says the project
// supplies it. Redefining a user function between two top-level forms
// is ordinary Shen.
func renderQuery(premises []Premise) string {
	var b strings.Builder
	b.WriteString("\\* Generated by `sb flow --engine shen` — one query per premise. *\\\n\n")
	for i, p := range premises {
		switch prem := p.(type) {
		case ConstructorOnly:
			fmt.Fprintf(&b, "(define allowed-caller?\n  Ctor Caller -> %s)\n\n",
				allowedCallerBody(prem.Allowed))
			fmt.Fprintf(&b, "(output \"%s ~A considered ~A~%%\" %d (prolog? (ctor-reference %s Caller File Line)))\n",
				queryMarker, i, shenString(prem.Ctor.String()))
			fmt.Fprintf(&b, "(output \"%s ~A violation ~A~%%\" %d (prolog? (unsanctioned-caller %s Caller File Line)))\n\n",
				queryMarker, i, shenString(prem.Ctor.String()))
		case MustPassThrough:
			fmt.Fprintf(&b, "(output \"%s ~A considered ~A~%%\" %d (prolog? (source-def %s Sym)))\n",
				queryMarker, i, shenString(prem.Source.String()))
			fmt.Fprintf(&b, "(output \"%s ~A violation ~A~%%\" %d (prolog? (must-pass-through-violation %s %s %s Path)))\n\n",
				queryMarker, i, shenString(prem.Source.String()),
				shenString(prem.Proof.String()), shenString(prem.Sink.String()))
		}
	}
	return b.String()
}

// allowedCallerBody renders the allowlist as a Shen boolean
// expression. An empty allowlist is `false`: `(constructor-only C)`
// with no permitted caller means no reference at all is sanctioned,
// which is what MatchesAny on an empty slice says on the Go side.
func allowedCallerBody(allowed []Pattern) string {
	if len(allowed) == 0 {
		return "false"
	}
	expr := fmt.Sprintf("(matches? %s Caller)", shenString(allowed[len(allowed)-1].String()))
	for i := len(allowed) - 2; i >= 0; i-- {
		expr = fmt.Sprintf("(or (matches? %s Caller) %s)", shenString(allowed[i].String()), expr)
	}
	return expr
}

// shenString quotes a pattern as a Shen string literal. Patterns come
// from the spec file and hold symbol path characters and `*`, never a
// quote or a backslash, so rejecting those is a guard against a
// malformed spec rather than an escaping scheme.
func shenString(s string) string {
	s = strings.NewReplacer("\"", "", "\\", "").Replace(s)
	return "\"" + s + "\""
}

// parseVerdicts reads the marker lines back.
//
// A premise with no marker line is an error, not a default: the whole
// point of running the Shen engine is that its answer is independent,
// and silently substituting "discharged" for "the host did not say"
// would make the comparison against the Go engine worthless.
func parseVerdicts(premises []Premise, out string) ([]ShenVerdict, error) {
	type pair struct{ considered, violation *bool }
	seen := make([]pair, len(premises))
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 4 || fields[0] != queryMarker {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(fields[1], "%d", &idx); err != nil || idx < 0 || idx >= len(premises) {
			continue
		}
		var val bool
		switch fields[3] {
		case "true":
			val = true
		case "false":
			val = false
		default:
			return nil, fmt.Errorf("the Shen engine answered %q for premise %s, which is neither true nor false",
				fields[3], premises[idx].ID())
		}
		v := val
		switch fields[2] {
		case "considered":
			seen[idx].considered = &v
		case "violation":
			seen[idx].violation = &v
		}
	}
	verdicts := make([]ShenVerdict, 0, len(premises))
	for i, p := range premises {
		if seen[i].considered == nil || seen[i].violation == nil {
			return nil, fmt.Errorf("the Shen engine did not answer for premise %s; its output was:\n%s", p.ID(), out)
		}
		verdicts = append(verdicts, ShenVerdict{
			PremiseID:  p.ID(),
			Considered: *seen[i].considered,
			Violation:  *seen[i].violation,
		})
	}
	return verdicts, nil
}

// EngineDisagreement is one premise the two engines decided
// differently about.
type EngineDisagreement struct {
	PremiseID string
	Go        string
	Shen      string
}

func (d EngineDisagreement) String() string {
	return fmt.Sprintf("%s: go says %s, shen says %s", d.PremiseID, d.Go, d.Shen)
}

// CompareEngines checks the Go engine's results against the Shen
// engine's verdicts and returns every premise they disagree about.
//
// This is the whole value of `--engine both`. docs/FLOW.md has always
// named "the two engines implement the same rules" as a trust
// assumption; until the Shen engine ran, that assumption was
// unfalsifiable. Now a disagreement is a red gate.
func CompareEngines(goResults []Result, shenVerdicts []ShenVerdict) []EngineDisagreement {
	byID := map[string]ShenVerdict{}
	for _, v := range shenVerdicts {
		byID[v.PremiseID] = v
	}
	var out []EngineDisagreement
	for _, r := range goResults {
		v, ok := byID[r.PremiseID]
		if !ok {
			out = append(out, EngineDisagreement{
				PremiseID: r.PremiseID,
				Go:        verdictWord(r.Discharged, r.Vacuous, len(r.Violations) > 0),
				Shen:      "nothing (the premise was not evaluated)",
			})
			continue
		}
		goWord := verdictWord(r.Discharged, r.Vacuous, len(r.Violations) > 0)
		shenWord := verdictWord(v.Discharged(), v.Vacuous(), v.Violation)
		if goWord != shenWord {
			out = append(out, EngineDisagreement{PremiseID: r.PremiseID, Go: goWord, Shen: shenWord})
		}
	}
	for _, v := range shenVerdicts {
		found := false
		for _, r := range goResults {
			if r.PremiseID == v.PremiseID {
				found = true
				break
			}
		}
		if !found {
			out = append(out, EngineDisagreement{
				PremiseID: v.PremiseID,
				Go:        "nothing (the premise was not evaluated)",
				Shen:      verdictWord(v.Discharged(), v.Vacuous(), v.Violation),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PremiseID < out[j].PremiseID })
	return out
}

func verdictWord(discharged, vacuous, violation bool) string {
	switch {
	case violation:
		return "violated"
	case vacuous:
		return "vacuous"
	case discharged:
		return "discharged"
	}
	return "unproven"
}
