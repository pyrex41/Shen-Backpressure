package symbolic

import (
	"fmt"
	"strings"
)

// Status is a solver verdict.
type Status int

const (
	// StatusUnknown means the solver could not decide, timed out, or
	// was not available at all. Callers must treat it as "feasibility
	// not established" and never as either sat or unsat.
	StatusUnknown Status = iota
	StatusSat
	StatusUnsat
)

func (s Status) String() string {
	switch s {
	case StatusSat:
		return "sat"
	case StatusUnsat:
		return "unsat"
	}
	return "unknown"
}

// Query is one satisfiability question: does an assignment to Vars
// exist that satisfies every assertion?
type Query struct {
	Vars       []*Var
	Assertions []Term
}

// SolverResult is a solver answer. Model maps variable names to the raw
// SMT-LIB2 value text the solver printed; decode.go turns that into
// shen-derive values.
type SolverResult struct {
	Status Status
	Model  map[string]string
	// Raw is the solver's stdout, kept for diagnostics.
	Raw string
}

// Solver answers satisfiability queries. The production implementation
// shells out to Z3 (see z3.go); tests inject a fake so the whole path
// enumerator is testable without a solver binary on PATH.
type Solver interface {
	// Name identifies the solver in report output, e.g. "z3".
	Name() string
	// Check answers one query.
	Check(q *Query) (*SolverResult, error)
}

// RenderScript prints a query as an SMT-LIB2 script. Logic ALL covers
// the whole fragment this package emits: linear real arithmetic,
// booleans, and strings with length.
func RenderScript(q *Query) string {
	var b strings.Builder
	b.WriteString("(set-logic ALL)\n")
	b.WriteString("(set-option :produce-models true)\n")
	for _, v := range q.Vars {
		fmt.Fprintf(&b, "(declare-const %s %s)\n", v.Name, v.S)
	}
	for _, a := range q.Assertions {
		if a == nil {
			continue
		}
		if lit, ok := BoolLit(a); ok && lit {
			// `true` adds nothing; keep the script minimal so a
			// golden comparison stays readable.
			continue
		}
		fmt.Fprintf(&b, "(assert %s)\n", a.String())
	}
	b.WriteString("(check-sat)\n")
	if len(q.Vars) > 0 {
		names := make([]string, len(q.Vars))
		for i, v := range q.Vars {
			names[i] = v.Name
		}
		fmt.Fprintf(&b, "(get-value (%s))\n", strings.Join(names, " "))
	}
	b.WriteString("(exit)\n")
	return b.String()
}

// parseSolverOutput reads a solver's stdout: a status line followed by
// an optional `((x 1.0) (y "a"))` get-value block.
func parseSolverOutput(out string) (*SolverResult, error) {
	res := &SolverResult{Model: map[string]string{}, Raw: out}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return res, fmt.Errorf("solver produced no output")
	}
	// The status is the first non-empty, non-comment line.
	rest := trimmed
	for {
		nl := strings.IndexByte(rest, '\n')
		line := rest
		if nl >= 0 {
			line = rest[:nl]
		}
		l := strings.TrimSpace(line)
		if nl >= 0 {
			rest = rest[nl+1:]
		} else {
			rest = ""
		}
		if l == "" || strings.HasPrefix(l, ";") {
			if rest == "" {
				return res, fmt.Errorf("solver produced no status line")
			}
			continue
		}
		switch l {
		case "sat":
			res.Status = StatusSat
		case "unsat":
			res.Status = StatusUnsat
		case "unknown", "timeout":
			res.Status = StatusUnknown
		default:
			if strings.HasPrefix(l, "(error") {
				return res, fmt.Errorf("solver error: %s", l)
			}
			return res, fmt.Errorf("unexpected solver status %q", l)
		}
		break
	}
	if res.Status != StatusSat {
		return res, nil
	}
	pairs, err := parseGetValue(rest)
	if err != nil {
		return res, err
	}
	res.Model = pairs
	return res, nil
}

// parseGetValue parses `((x 1.0) (y (- 2.0)) (s "abc"))` into a map of
// variable name to raw value text.
func parseGetValue(s string) (map[string]string, error) {
	out := map[string]string{}
	toks, err := tokenizeSMT(s)
	if err != nil {
		return nil, err
	}
	// Expect: ( ( name value ) ( name value ) … )
	i := 0
	if i >= len(toks) || toks[i] != "(" {
		// No model block at all (e.g. a query with no variables).
		return out, nil
	}
	i++
	for i < len(toks) && toks[i] == "(" {
		i++
		if i >= len(toks) {
			return nil, fmt.Errorf("truncated get-value block")
		}
		name := toks[i]
		i++
		val, next, err := readSMTValue(toks, i)
		if err != nil {
			return nil, err
		}
		i = next
		if i >= len(toks) || toks[i] != ")" {
			return nil, fmt.Errorf("get-value pair for %s not closed", name)
		}
		i++
		out[name] = val
	}
	return out, nil
}

// readSMTValue reads one value starting at toks[i] — either a single
// token or a balanced parenthesised form — and returns its text plus
// the index just past it.
func readSMTValue(toks []string, i int) (string, int, error) {
	if i >= len(toks) {
		return "", i, fmt.Errorf("truncated value")
	}
	if toks[i] != "(" {
		return toks[i], i + 1, nil
	}
	depth := 0
	var parts []string
	for i < len(toks) {
		t := toks[i]
		if t == "(" {
			depth++
		} else if t == ")" {
			depth--
		}
		parts = append(parts, t)
		i++
		if depth == 0 {
			break
		}
	}
	if depth != 0 {
		return "", i, fmt.Errorf("unbalanced value expression")
	}
	return joinSMT(parts), i, nil
}

func joinSMT(parts []string) string {
	var b strings.Builder
	for idx, p := range parts {
		if p == ")" {
			b.WriteString(p)
			continue
		}
		if idx > 0 && parts[idx-1] != "(" {
			b.WriteByte(' ')
		}
		b.WriteString(p)
	}
	return b.String()
}

// tokenizeSMT splits SMT-LIB2 text into parens, string literals, and
// bare atoms.
func tokenizeSMT(s string) ([]string, error) {
	var toks []string
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(' || c == ')':
			toks = append(toks, string(c))
			i++
		case c == '"':
			j := i + 1
			var b strings.Builder
			b.WriteByte('"')
			for j < len(s) {
				if s[j] == '"' {
					// A doubled quote is an escaped quote.
					if j+1 < len(s) && s[j+1] == '"' {
						b.WriteByte('"')
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated string literal")
			}
			b.WriteByte('"')
			toks = append(toks, b.String())
			i = j + 1
		case c == ';':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t\n\r()\";", rune(s[j])) {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		}
	}
	return toks, nil
}
