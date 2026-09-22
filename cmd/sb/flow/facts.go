// Package flow turns a resolved symbol graph into flow facts and
// evaluates flow premises over them.
//
// The fact vocabulary is deliberately tiny and language-independent —
// it is what every SCIP indexer already knows:
//
//	(def  sym file start-line start-col end-line end-col)
//	(ref  sym file line col enclosing-def-sym)
//	(call caller-sym callee-sym)
//
// Facts are emitted as Shen s-expressions (.sb/facts.shen) so the
// primary engine — the Prolog rules in sb/flow/stdlib.shen — needs no
// parser at all. This package additionally implements an equivalent
// evaluator in Go for environments with no Shen host on PATH; see
// docs/FLOW.md for which engine runs when.
//
// Positions are zero-based, exactly as SCIP reports them. Rendering
// for humans (file:line) adds one; nothing else does.
package flow

import (
	"fmt"
	"sort"
	"strings"
)

// Def is a definition fact: a symbol and the extent it occupies.
//
// The extent is the definition's *enclosing* range (signature plus
// body) when the indexer provides one, which is what makes reference
// containment meaningful. When it does not, the extent degenerates to
// the name's own range and only references on the definition line can
// be attributed to it.
type Def struct {
	Symbol    string
	File      string
	StartLine int32
	StartCol  int32
	EndLine   int32
	EndCol    int32
}

// Contains reports whether the definition's extent covers a position.
func (d Def) Contains(line, col int32) bool {
	if line < d.StartLine || line > d.EndLine {
		return false
	}
	if line == d.StartLine && col < d.StartCol {
		return false
	}
	if line == d.EndLine && col > d.EndCol {
		return false
	}
	return true
}

// lineSpan is the definition's height, used to pick the innermost of
// two containing definitions.
func (d Def) lineSpan() int32 { return d.EndLine - d.StartLine }

// Ref is a reference fact: a use of Symbol at a position, attributed
// to the definition that encloses it (empty when the position lies
// outside every known definition, e.g. a package-level var).
type Ref struct {
	Symbol    string
	File      string
	Line      int32
	Col       int32
	Enclosing string
}

// Location renders the reference as a one-based file:line:col, the
// form a counterexample carries.
func (r Ref) Location() string {
	return fmt.Sprintf("%s:%d:%d", r.File, r.Line+1, r.Col+1)
}

// Call is a call-graph edge: Caller's body references the callable
// Callee. It is derived from Ref, not observed separately — an
// indexer records references, and a reference to a callable from
// inside a definition is a call edge for flow purposes.
type Call struct {
	Caller string
	Callee string
}

// FactSet is the whole fact base for one indexed tree.
type FactSet struct {
	Defs  []Def
	Refs  []Ref
	Calls []Call

	// Indexer names the tool that produced the underlying index
	// ("scip-go", "scip-typescript"). Recorded so a discharge report
	// can name what is in its TCB.
	Indexer string
	// Language is the index's language tag, as the indexer reported it.
	Language string

	callsFrom map[string][]string
	defsBySym map[string][]Def
}

// Index builds the lookup tables the engine walks. It is idempotent
// and must be called after mutating the slices directly (the
// constructors in this package call it for you).
func (fs *FactSet) Index() {
	fs.callsFrom = map[string][]string{}
	seen := map[Call]bool{}
	for _, c := range fs.Calls {
		if seen[c] {
			continue
		}
		seen[c] = true
		fs.callsFrom[c.Caller] = append(fs.callsFrom[c.Caller], c.Callee)
	}
	fs.defsBySym = map[string][]Def{}
	for _, d := range fs.Defs {
		fs.defsBySym[d.Symbol] = append(fs.defsBySym[d.Symbol], d)
	}
}

// CalleesOf returns the callees of sym in deterministic order.
func (fs *FactSet) CalleesOf(sym string) []string {
	if fs.callsFrom == nil {
		fs.Index()
	}
	out := append([]string(nil), fs.callsFrom[sym]...)
	sort.Strings(out)
	return out
}

// DefsOf returns the definition facts for sym.
func (fs *FactSet) DefsOf(sym string) []Def {
	if fs.defsBySym == nil {
		fs.Index()
	}
	return fs.defsBySym[sym]
}

// Sort orders every fact slice so the emitted fact file is a pure
// function of the index, not of map iteration order. A golden test
// depends on this.
func (fs *FactSet) Sort() {
	sort.SliceStable(fs.Defs, func(i, j int) bool {
		a, b := fs.Defs[i], fs.Defs[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		if a.StartCol != b.StartCol {
			return a.StartCol < b.StartCol
		}
		return a.Symbol < b.Symbol
	})
	sort.SliceStable(fs.Refs, func(i, j int) bool {
		a, b := fs.Refs[i], fs.Refs[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.Symbol < b.Symbol
	})
	sort.SliceStable(fs.Calls, func(i, j int) bool {
		if fs.Calls[i].Caller != fs.Calls[j].Caller {
			return fs.Calls[i].Caller < fs.Calls[j].Caller
		}
		return fs.Calls[i].Callee < fs.Calls[j].Callee
	})
}

// --- fact file serialisation ----------------------------------------

// quoteShen renders s as a Shen string literal. Shen string literals
// have no escape sequences, so an embedded double quote cannot be
// represented; SCIP symbols do not contain one, and if a future
// indexer emits one we substitute a single quote rather than write a
// fact file the Shen reader would choke on.
func quoteShen(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `'`) + `"`
}

// WriteFacts renders the fact set as the Shen s-expression fact file
// that .sb/facts.shen holds and sb/flow/stdlib.shen consumes.
func (fs *FactSet) WriteFacts() string {
	var b strings.Builder
	b.WriteString("\\* Generated by `sb index` — do not edit.\n")
	b.WriteString("   Flow facts over a resolved symbol graph. Positions are zero-based,\n")
	b.WriteString("   exactly as the SCIP indexer reports them.\n\n")
	b.WriteString("     (def sym file start-line start-col end-line end-col)\n")
	b.WriteString("     (ref sym file line col enclosing-def-sym)\n")
	b.WriteString("     (call caller-sym callee-sym)\n\n")
	fmt.Fprintf(&b, "   indexer: %s\n   language: %s\n", fs.Indexer, fs.Language)
	fmt.Fprintf(&b, "   defs: %d  refs: %d  calls: %d\n*\\\n\n", len(fs.Defs), len(fs.Refs), len(fs.Calls))
	for _, d := range fs.Defs {
		fmt.Fprintf(&b, "(def %s %s %d %d %d %d)\n",
			quoteShen(d.Symbol), quoteShen(d.File), d.StartLine, d.StartCol, d.EndLine, d.EndCol)
	}
	for _, r := range fs.Refs {
		fmt.Fprintf(&b, "(ref %s %s %d %d %s)\n",
			quoteShen(r.Symbol), quoteShen(r.File), r.Line, r.Col, quoteShen(r.Enclosing))
	}
	for _, c := range fs.Calls {
		fmt.Fprintf(&b, "(call %s %s)\n", quoteShen(c.Caller), quoteShen(c.Callee))
	}
	return b.String()
}

// ParseFacts reads a fact file back. It accepts exactly what
// WriteFacts emits (plus Shen `\* … *\` comments and blank lines) and
// is used by the Go engine's tests and by the flow gate when a cached
// fact file is reused.
func ParseFacts(src string) (*FactSet, error) {
	fs := &FactSet{}
	for _, form := range splitForms(src) {
		toks, err := tokenizeForm(form)
		if err != nil {
			return nil, err
		}
		if len(toks) == 0 {
			continue
		}
		switch toks[0] {
		case "def":
			if len(toks) != 7 {
				return nil, fmt.Errorf("flow: (def …) wants 6 arguments, got %d", len(toks)-1)
			}
			nums, err := parseInts(toks[3:7])
			if err != nil {
				return nil, err
			}
			fs.Defs = append(fs.Defs, Def{
				Symbol: toks[1], File: toks[2],
				StartLine: nums[0], StartCol: nums[1], EndLine: nums[2], EndCol: nums[3],
			})
		case "ref":
			if len(toks) != 6 {
				return nil, fmt.Errorf("flow: (ref …) wants 5 arguments, got %d", len(toks)-1)
			}
			nums, err := parseInts(toks[3:5])
			if err != nil {
				return nil, err
			}
			fs.Refs = append(fs.Refs, Ref{
				Symbol: toks[1], File: toks[2],
				Line: nums[0], Col: nums[1], Enclosing: toks[5],
			})
		case "call":
			if len(toks) != 3 {
				return nil, fmt.Errorf("flow: (call …) wants 2 arguments, got %d", len(toks)-1)
			}
			fs.Calls = append(fs.Calls, Call{Caller: toks[1], Callee: toks[2]})
		default:
			return nil, fmt.Errorf("flow: unknown fact %q", toks[0])
		}
	}
	fs.Index()
	return fs, nil
}

func parseInts(toks []string) ([]int32, error) {
	out := make([]int32, len(toks))
	for i, t := range toks {
		var v int32
		if _, err := fmt.Sscanf(t, "%d", &v); err != nil {
			return nil, fmt.Errorf("flow: %q is not a position", t)
		}
		out[i] = v
	}
	return out, nil
}

// splitForms returns the top-level parenthesised forms in src,
// skipping Shen `\* … *\` comments and anything outside a form.
func splitForms(src string) []string {
	var forms []string
	depth := 0
	start := 0
	inString := false
	for i := 0; i < len(src); i++ {
		if !inString && depth == 0 && strings.HasPrefix(src[i:], `\*`) {
			if end := strings.Index(src[i:], `*\`); end >= 0 {
				i += end + 1
				continue
			}
			break
		}
		switch src[i] {
		case '"':
			inString = !inString
		case '(':
			if inString {
				continue
			}
			if depth == 0 {
				start = i
			}
			depth++
		case ')':
			if inString {
				continue
			}
			depth--
			if depth == 0 {
				forms = append(forms, src[start:i+1])
			}
		}
	}
	return forms
}

// tokenizeForm splits one form into its head symbol and arguments,
// unquoting string literals.
func tokenizeForm(form string) ([]string, error) {
	form = strings.TrimSpace(form)
	if !strings.HasPrefix(form, "(") || !strings.HasSuffix(form, ")") {
		return nil, fmt.Errorf("flow: %q is not a form", form)
	}
	inner := form[1 : len(form)-1]
	var toks []string
	i := 0
	for i < len(inner) {
		switch {
		case inner[i] == ' ' || inner[i] == '\n' || inner[i] == '\t' || inner[i] == '\r':
			i++
		case inner[i] == '"':
			j := strings.IndexByte(inner[i+1:], '"')
			if j < 0 {
				return nil, fmt.Errorf("flow: unterminated string in %q", form)
			}
			toks = append(toks, inner[i+1:i+1+j])
			i += j + 2
		default:
			j := i
			for j < len(inner) && !strings.ContainsRune(" \n\t\r", rune(inner[j])) {
				j++
			}
			toks = append(toks, inner[i:j])
			i = j
		}
	}
	return toks, nil
}
