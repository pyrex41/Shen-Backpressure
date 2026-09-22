package flow

import (
	"fmt"
	"sort"
	"strings"
)

// EngineName identifies which evaluator produced a result. The Shen
// Prolog rules in sb/flow/stdlib.shen are the documented primary
// engine; the Go evaluator here is the fallback for environments with
// no Shen host on PATH, and is what runs in this repository's tests.
const (
	EngineShenProlog = "shen-prolog"
	EngineGo         = "go-datalog"
)

// Violation is one counterexample to a flow premise.
type Violation struct {
	// PremiseID is the premise the violation belongs to.
	PremiseID string
	// Location is the one-based file:line:col of the offending
	// reference — the single most useful bit for the next prompt.
	Location string
	// Symbol is the canonical symbol at Location.
	Symbol string
	// Enclosing is the canonical symbol of the definition the
	// offending reference sits in.
	Enclosing string
	// Path is the shortest violating call path, canonical symbols,
	// source first. Empty for constructor-only violations, which are
	// a single reference rather than a path.
	Path []string
	// Rationale is the English sentence the report carries.
	Rationale string
}

// Result is the evaluation of one flow premise.
type Result struct {
	PremiseID  string
	Expression string
	// Discharged is true when the premise held over the whole fact
	// base and the fact base was non-vacuous for it.
	Discharged bool
	// Vacuous is true when nothing in the fact base matched the
	// premise's subject — no reference to the constructor, or no
	// definition matching the source pattern. A vacuous premise is
	// not discharged: it means the pattern is stale or the indexer
	// did not see the code, both of which are backpressure on the
	// spec author rather than evidence.
	Vacuous bool
	// Considered counts the facts the premise actually ranged over
	// (references for constructor-only, source definitions for
	// must-pass-through).
	Considered int
	Violations []Violation
	Rationale  string
}

// Evaluate runs every premise of every declaration over the facts.
func Evaluate(fs *FactSet, decls []Decl) []Result {
	var out []Result
	for _, d := range decls {
		for _, p := range d.Premises {
			switch prem := p.(type) {
			case ConstructorOnly:
				out = append(out, evalConstructorOnly(fs, prem))
			case MustPassThrough:
				out = append(out, evalMustPassThrough(fs, prem))
			}
		}
	}
	return out
}

// evalConstructorOnly is the `unsanctioned-caller/2` search: a
// reference to the constructor whose enclosing definition is not one
// of the allowed callers.
//
// Two references are exempt by construction rather than by
// allowlist: the constructor's own definition site, and a reference
// from inside the file that defines it (the generated guard package
// names its own constructor in its own doc comments and helpers). Any
// other reference is a violation, including one made through an
// aliased import — the indexer resolved the alias to the same symbol
// before we ever saw it.
func evalConstructorOnly(fs *FactSet, p ConstructorOnly) Result {
	res := Result{PremiseID: p.ID(), Expression: p.Expression()}
	defFiles := map[string]bool{}
	for _, d := range fs.Defs {
		if p.Ctor.Matches(d.Symbol) {
			defFiles[d.File] = true
		}
	}
	for _, r := range fs.Refs {
		if !p.Ctor.Matches(r.Symbol) {
			continue
		}
		if defFiles[r.File] {
			continue
		}
		res.Considered++
		if MatchesAny(p.Allowed, r.Enclosing) {
			continue
		}
		res.Violations = append(res.Violations, Violation{
			PremiseID: p.ID(),
			Location:  r.Location(),
			Symbol:    Canonical(r.Symbol),
			Enclosing: Canonical(r.Enclosing),
			Rationale: fmt.Sprintf(
				"%s references %s, which only %s may construct. The reference resolves to the constructor symbol even through an aliased import, so renaming the import does not evade this premise.",
				describeEnclosing(r.Enclosing), Canonical(r.Symbol), joinPatterns(p.Allowed)),
		})
	}
	sortViolations(res.Violations)
	switch {
	case res.Considered == 0:
		res.Vacuous = true
		res.Rationale = fmt.Sprintf(
			"No reference to %s outside its defining file appears in the index. The premise ranged over nothing, so it is not evidence: either the pattern is stale or the indexer did not see the calling code.",
			p.Ctor.String())
	case len(res.Violations) == 0:
		res.Discharged = true
		res.Rationale = fmt.Sprintf(
			"All %d resolved references to %s lie inside %s.",
			res.Considered, p.Ctor.String(), joinPatterns(p.Allowed))
	default:
		res.Rationale = fmt.Sprintf(
			"%d of %d resolved references to %s lie outside %s.",
			len(res.Violations), res.Considered, p.Ctor.String(), joinPatterns(p.Allowed))
	}
	return res
}

// evalMustPassThrough is the violation search for
// `must-pass-through Src Proof Sink`.
//
// For each definition matching Src, walk the call graph breadth-first
// and prune any definition that references Proof: past a declassifier
// the flow is sanctioned. If the walk reaches a call whose callee
// matches Sink, the BFS path to that call is a shortest violating
// path and the call site is the counterexample location.
func evalMustPassThrough(fs *FactSet, p MustPassThrough) Result {
	res := Result{PremiseID: p.ID(), Expression: p.Expression()}
	proofRefs := refsMatching(fs, p.Proof)
	sources := sourceDefs(fs, p.Source)
	refIndex := refsByEnclosing(fs)

	for _, src := range sources {
		res.Considered++
		if v, ok := searchViolation(fs, p, src, proofRefs, refIndex); ok {
			v.PremiseID = p.ID()
			res.Violations = append(res.Violations, v)
		}
	}
	sortViolations(res.Violations)
	switch {
	case res.Considered == 0:
		res.Vacuous = true
		res.Rationale = fmt.Sprintf(
			"No definition matching %s appears in the index. The premise ranged over nothing, so it is not evidence.",
			p.Source.String())
	case len(res.Violations) == 0:
		res.Discharged = true
		res.Rationale = fmt.Sprintf(
			"Every call path from the %d definitions matching %s to a call of %s passes through a reference to %s.",
			res.Considered, p.Source.String(), p.Sink.String(), p.Proof.String())
	default:
		res.Rationale = fmt.Sprintf(
			"%d of %d definitions matching %s reach %s without a reference to %s.",
			len(res.Violations), res.Considered, p.Source.String(), p.Sink.String(), p.Proof.String())
	}
	return res
}

// searchViolation runs the pruned BFS for one source definition.
func searchViolation(fs *FactSet, p MustPassThrough, src string, proofRefs map[string]bool, refIndex map[string][]Ref) (Violation, bool) {
	if proofRefs[src] {
		return Violation{}, false
	}
	type node struct {
		sym  string
		path []string
	}
	queue := []node{{sym: src, path: []string{src}}}
	seen := map[string]bool{src: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		// A call of the sink from cur is the violation.
		for _, ref := range refIndex[cur.sym] {
			if !p.Sink.Matches(ref.Symbol) {
				continue
			}
			return Violation{
				Location:  ref.Location(),
				Symbol:    Canonical(ref.Symbol),
				Enclosing: Canonical(cur.sym),
				Path:      canonicalPath(cur.path),
				Rationale: fmt.Sprintf(
					"%s reaches the sink %s at %s without any definition on the path %s referencing the proof %s.",
					Canonical(src), Canonical(ref.Symbol), ref.Location(),
					strings.Join(canonicalPath(cur.path), " → "), p.Proof.String()),
			}, true
		}
		for _, callee := range fs.CalleesOf(cur.sym) {
			if seen[callee] || proofRefs[callee] {
				// Pruned: either already visited, or a
				// declassifier — past it the flow is sanctioned.
				continue
			}
			seen[callee] = true
			queue = append(queue, node{sym: callee, path: append(append([]string(nil), cur.path...), callee)})
		}
	}
	return Violation{}, false
}

// refsMatching returns the set of definitions that reference a symbol
// matching pat — the declassifier set for a must-pass-through search.
func refsMatching(fs *FactSet, pat Pattern) map[string]bool {
	out := map[string]bool{}
	for _, r := range fs.Refs {
		if r.Enclosing != "" && pat.Matches(r.Symbol) {
			out[r.Enclosing] = true
		}
	}
	return out
}

// sourceDefs returns the callable definitions matching pat, ordered.
func sourceDefs(fs *FactSet, pat Pattern) []string {
	set := map[string]bool{}
	for _, d := range fs.Defs {
		if pat.Matches(d.Symbol) && IsCallable(d.Symbol) {
			set[d.Symbol] = true
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func refsByEnclosing(fs *FactSet) map[string][]Ref {
	out := map[string][]Ref{}
	for _, r := range fs.Refs {
		if r.Enclosing == "" {
			continue
		}
		out[r.Enclosing] = append(out[r.Enclosing], r)
	}
	return out
}

func canonicalPath(path []string) []string {
	out := make([]string, len(path))
	for i, s := range path {
		out[i] = Canonical(s)
	}
	return out
}

func describeEnclosing(sym string) string {
	if sym == "" {
		return "a package-level declaration"
	}
	return Canonical(sym)
}

func joinPatterns(pats []Pattern) string {
	parts := make([]string, len(pats))
	for i, p := range pats {
		parts[i] = p.String()
	}
	return strings.Join(parts, ", ")
}

func sortViolations(vs []Violation) {
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].Location < vs[j].Location })
}
