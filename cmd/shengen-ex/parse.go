package main

// parse.go — Shen spec parsing for shengen-ex.
//
// The datatype parser (extractBlocks / parseDatatype / buildRule) is a
// faithful port of cmd/shengen's line-based parser so the two emitters
// classify every spec identically; parity_test.go pins that by diffing
// the symbol-table dump of both tools over every spec in the repo.
// cmd/shengen is `package main` and cannot be imported, hence the copy.
//
// On top of that, shengen-ex carries a small but real Shen reader
// (strings, [..|..] cons lists, {..} signatures) because the Elixir
// lowering of (define ...) blocks and verified premises works on proper
// syntax trees instead of re-splitting text.

import (
	"fmt"
	"os"
	"strings"
)

// ----------------------------------------------------------------------------
// Datatype model (mirrors cmd/shengen)
// ----------------------------------------------------------------------------

type Premise struct {
	VarName  string
	TypeName string
}

type VerifiedPremise struct {
	Raw        string
	RuntimeVia string
}

type Conclusion struct {
	Fields    []string // conclusion names introduced by premises (host fields)
	Items     []string // every raw item of a [..] conclusion, incl. literal tags
	TypeName  string
	IsWrapped bool
}

type Rule struct {
	Premises []Premise
	Verified []VerifiedPremise
	Conc     Conclusion
}

type Datatype struct {
	Name  string
	Rules []Rule
}

// ----------------------------------------------------------------------------
// Define model
// ----------------------------------------------------------------------------

type DefineClause struct {
	Patterns []*Node
	Result   *Node
	Guard    *Node // nil when there is no `where`
}

type Define struct {
	Name      string
	ParamType []string // Shen types from the {A --> B --> C} signature (all but last)
	RetType   string   // last type in the signature ("" when untyped)
	Clauses   []DefineClause
	Raw       string
}

// ----------------------------------------------------------------------------
// File parsing
// ----------------------------------------------------------------------------

// stripComments blanks Shen block comments (\* ... *\) and line comments
// (\\ ...) while preserving byte offsets and newlines. Comments that carry
// a `runtime-via` marker are kept verbatim because buildRule reads them.
func stripComments(s string) string {
	b := []byte(s)
	out := make([]byte, len(b))
	copy(out, b)
	inStr := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '\\' && i+1 < len(b) && b[i+1] == '*' {
			end := strings.Index(s[i+2:], `*\`)
			stop := len(b)
			if end >= 0 {
				stop = i + 2 + end + 2
			}
			body := s[i:stop]
			if !strings.Contains(body, "runtime-via") {
				for j := i; j < stop; j++ {
					if out[j] != '\n' {
						out[j] = ' '
					}
				}
			}
			i = stop - 1
			continue
		}
		if c == '\\' && i+1 < len(b) && b[i+1] == '\\' {
			j := i
			for j < len(b) && b[j] != '\n' {
				out[j] = ' '
				j++
			}
			i = j - 1
		}
	}
	return string(out)
}

func parseSpec(path string) ([]Datatype, []Define, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return parseSpecContent(string(data))
}

func parseSpecContent(raw string) ([]Datatype, []Define, error) {
	content := stripComments(raw)
	var types []Datatype
	for _, block := range extractBlocks(content, "(datatype ") {
		if dt := parseDatatype(block); dt != nil {
			types = append(types, *dt)
		}
	}
	var defines []Define
	for _, block := range extractBlocks(content, "(define ") {
		def, err := parseDefine(block)
		if err != nil {
			return nil, nil, err
		}
		if def != nil {
			defines = append(defines, *def)
		}
	}
	return types, defines, nil
}

func extractBlocks(content, prefix string) []string {
	var blocks []string
	remaining := content
	for {
		idx := strings.Index(remaining, prefix)
		if idx == -1 {
			break
		}
		remaining = remaining[idx:]
		depth, end := 0, -1
		for i, ch := range remaining {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
		if end == -1 {
			break
		}
		blocks = append(blocks, remaining[:end])
		remaining = remaining[end:]
	}
	return blocks
}

func parseDatatype(block string) *Datatype {
	block = strings.TrimPrefix(block, "(datatype ")
	nlIdx := strings.Index(block, "\n")
	if nlIdx == -1 {
		return nil
	}
	name := strings.TrimSpace(block[:nlIdx])
	body := strings.TrimRight(block[nlIdx:], " \t\n)")

	lines := strings.Split(body, "\n")
	dt := &Datatype{Name: name}
	var premLines, concLines []string
	seenInf := false

	flush := func() {
		if len(concLines) == 0 {
			return
		}
		if r := buildRule(premLines, concLines); r != nil {
			dt.Rules = append(dt.Rules, *r)
		}
	}

	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if len(t) >= 3 && (allChar(t, '=') || allChar(t, '_')) {
			if seenInf {
				flush()
				premLines, concLines = nil, nil
				seenInf = false
			}
			seenInf = true
			continue
		}
		if !seenInf {
			premLines = append(premLines, t)
		} else {
			concLines = append(concLines, t)
		}
	}
	flush()
	if len(dt.Rules) == 0 {
		return nil
	}
	return dt
}

func allChar(s string, ch rune) bool {
	for _, c := range s {
		if c != ch {
			return false
		}
	}
	return true
}

func premLineStartsNewPremise(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "if ") {
		return true
	}
	if len(t) >= 3 && (allChar(t, '=') || allChar(t, '_')) {
		return true
	}
	return strings.Contains(t, " : ") && !strings.Contains(t, ": verified")
}

func extractRuntimeViaComment(line string) (name string, stripped string, ok bool) {
	const open = `\*`
	const close = `*\`
	openIdx := strings.LastIndex(line, open)
	if openIdx < 0 {
		return "", line, false
	}
	rest := line[openIdx+len(open):]
	closeIdx := strings.Index(rest, close)
	if closeIdx < 0 {
		return "", line, false
	}
	body := strings.TrimSpace(rest[:closeIdx])
	for _, prefix := range []string{":runtime-via ", "runtime-via "} {
		if strings.HasPrefix(body, prefix) {
			n := strings.TrimSpace(strings.TrimPrefix(body, prefix))
			if n == "" {
				return "", line, false
			}
			return n, strings.TrimSpace(line[:openIdx]), true
		}
	}
	return "", line, false
}

func buildRule(premLines, concLines []string) *Rule {
	r := &Rule{}
	var logicalPremLines []string
	for i := 0; i < len(premLines); i++ {
		line := strings.TrimSpace(premLines[i])
		if strings.HasPrefix(line, "(") && !strings.Contains(line, ": verified") {
			for i+1 < len(premLines) && !strings.Contains(line, ": verified") {
				next := strings.TrimSpace(premLines[i+1])
				if premLineStartsNewPremise(next) {
					break
				}
				i++
				line += " " + next
			}
		}
		logicalPremLines = append(logicalPremLines, line)
	}
	for _, line := range logicalPremLines {
		line = strings.TrimSuffix(strings.TrimSpace(line), ";")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runtimeVia := ""
		if via, stripped, ok := extractRuntimeViaComment(line); ok {
			runtimeVia = via
			line = strings.TrimSuffix(strings.TrimSpace(stripped), ";")
			line = strings.TrimSpace(line)
		}
		if strings.HasSuffix(line, ": verified") {
			raw := strings.TrimSpace(strings.TrimSuffix(line, ": verified"))
			r.Verified = append(r.Verified, VerifiedPremise{Raw: raw, RuntimeVia: runtimeVia})
			continue
		}
		if strings.HasPrefix(line, "if ") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "if "))
			r.Verified = append(r.Verified, VerifiedPremise{Raw: raw, RuntimeVia: runtimeVia})
			continue
		}
		if parts := strings.SplitN(line, " : ", 2); len(parts) == 2 {
			r.Premises = append(r.Premises, Premise{VarName: strings.TrimSpace(parts[0]), TypeName: strings.TrimSpace(parts[1])})
		}
	}

	concStr := strings.TrimSpace(strings.TrimSuffix(strings.Join(concLines, " "), ";"))
	if strings.Contains(concStr, ">>") {
		return nil
	}
	parts := strings.SplitN(concStr, " : ", 2)
	if len(parts) != 2 {
		return nil
	}
	lhs, rhs := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	r.Conc.TypeName = rhs
	if strings.HasPrefix(lhs, "[") && strings.HasSuffix(lhs, "]") {
		premises := make(map[string]bool, len(r.Premises))
		for _, p := range r.Premises {
			premises[p.VarName] = true
		}
		for _, field := range strings.Fields(lhs[1 : len(lhs)-1]) {
			r.Conc.Items = append(r.Conc.Items, field)
			if premises[field] {
				r.Conc.Fields = append(r.Conc.Fields, field)
			}
		}
	} else {
		r.Conc.Fields = []string{lhs}
		r.Conc.Items = []string{lhs}
		r.Conc.IsWrapped = true
	}
	return r
}

// ----------------------------------------------------------------------------
// Shen reader
// ----------------------------------------------------------------------------

type NodeKind int

const (
	NAtom  NodeKind = iota // symbol, variable, number, boolean
	NStr                   // "string literal" (Atom holds the unquoted text)
	NList                  // ( ... )
	NCons                  // [ a b | tail ]  (Tail may be nil)
	NBrace                 // { ... } (type signatures)
)

type Node struct {
	Kind  NodeKind
	Atom  string
	Items []*Node
	Tail  *Node
}

func (n *Node) String() string {
	switch n.Kind {
	case NAtom:
		return n.Atom
	case NStr:
		return `"` + n.Atom + `"`
	case NList, NBrace, NCons:
		parts := make([]string, len(n.Items))
		for i, c := range n.Items {
			parts[i] = c.String()
		}
		s := strings.Join(parts, " ")
		switch n.Kind {
		case NList:
			return "(" + s + ")"
		case NBrace:
			return "{" + s + "}"
		default:
			if n.Tail != nil {
				s += " | " + n.Tail.String()
			}
			return "[" + s + "]"
		}
	}
	return "?"
}

// Op returns the head symbol of a (call ...) node, or "".
func (n *Node) Op() string {
	if n.Kind == NList && len(n.Items) > 0 && n.Items[0].Kind == NAtom {
		return n.Items[0].Atom
	}
	return ""
}

func (n *Node) Args() []*Node {
	if n.Kind == NList && len(n.Items) > 0 {
		return n.Items[1:]
	}
	return nil
}

func tokenizeShen(s string) ([]string, error) {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '"':
			flush()
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unterminated string literal")
			}
			toks = append(toks, string(rs[i:j+1]))
			i = j
		case c == '(' || c == ')' || c == '[' || c == ']' || c == '{' || c == '}' || c == '|':
			flush()
			toks = append(toks, string(c))
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		default:
			cur.WriteRune(c)
		}
	}
	flush()
	return toks, nil
}

type reader struct {
	toks []string
	pos  int
}

func (r *reader) eof() bool { return r.pos >= len(r.toks) }

func (r *reader) read() (*Node, error) {
	if r.eof() {
		return nil, fmt.Errorf("unexpected end of input")
	}
	t := r.toks[r.pos]
	r.pos++
	switch t {
	case "(":
		n := &Node{Kind: NList}
		for {
			if r.eof() {
				return nil, fmt.Errorf("unbalanced (")
			}
			if r.toks[r.pos] == ")" {
				r.pos++
				return n, nil
			}
			c, err := r.read()
			if err != nil {
				return nil, err
			}
			n.Items = append(n.Items, c)
		}
	case "{":
		n := &Node{Kind: NBrace}
		for {
			if r.eof() {
				return nil, fmt.Errorf("unbalanced {")
			}
			if r.toks[r.pos] == "}" {
				r.pos++
				return n, nil
			}
			c, err := r.read()
			if err != nil {
				return nil, err
			}
			n.Items = append(n.Items, c)
		}
	case "[":
		n := &Node{Kind: NCons}
		for {
			if r.eof() {
				return nil, fmt.Errorf("unbalanced [")
			}
			switch r.toks[r.pos] {
			case "]":
				r.pos++
				return n, nil
			case "|":
				r.pos++
				tail, err := r.read()
				if err != nil {
					return nil, err
				}
				n.Tail = tail
				if r.eof() || r.toks[r.pos] != "]" {
					return nil, fmt.Errorf("expected ] after | tail")
				}
				r.pos++
				return n, nil
			}
			c, err := r.read()
			if err != nil {
				return nil, err
			}
			n.Items = append(n.Items, c)
		}
	case ")", "]", "}", "|":
		return nil, fmt.Errorf("unexpected %q", t)
	}
	if strings.HasPrefix(t, `"`) {
		return &Node{Kind: NStr, Atom: strings.TrimSuffix(strings.TrimPrefix(t, `"`), `"`)}, nil
	}
	return &Node{Kind: NAtom, Atom: t}, nil
}

// readShen parses one Shen expression.
func readShen(s string) (*Node, error) {
	toks, err := tokenizeShen(s)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("empty expression")
	}
	r := &reader{toks: toks}
	n, err := r.read()
	if err != nil {
		return nil, err
	}
	if !r.eof() {
		return nil, fmt.Errorf("trailing tokens after expression %q", s)
	}
	return n, nil
}

// readAll parses a sequence of Shen forms.
func readAll(s string) ([]*Node, error) {
	toks, err := tokenizeShen(s)
	if err != nil {
		return nil, err
	}
	r := &reader{toks: toks}
	var out []*Node
	for !r.eof() {
		n, err := r.read()
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// parseDefine reads `(define name {sig} clause...)`. Clauses are
// `pattern... -> result [where guard]`. Backtracking clauses (<-) are
// rejected explicitly rather than silently mis-lowered.
func parseDefine(block string) (*Define, error) {
	n, err := readShen(block)
	if err != nil {
		return nil, fmt.Errorf("parse define: %w", err)
	}
	if n.Op() != "define" || len(n.Items) < 2 || n.Items[1].Kind != NAtom {
		return nil, nil
	}
	def := &Define{Name: n.Items[1].Atom, Raw: block}
	rest := n.Items[2:]
	if len(rest) > 0 && rest[0].Kind == NBrace {
		var types []string
		var cur []string
		for _, it := range rest[0].Items {
			if it.Kind == NAtom && it.Atom == "-->" {
				types = append(types, strings.Join(cur, " "))
				cur = nil
				continue
			}
			cur = append(cur, it.String())
		}
		types = append(types, strings.Join(cur, " "))
		if len(types) >= 1 {
			def.RetType = types[len(types)-1]
			def.ParamType = types[:len(types)-1]
		}
		rest = rest[1:]
	}
	var pats []*Node
	for i := 0; i < len(rest); i++ {
		it := rest[i]
		if it.Kind == NAtom && it.Atom == "<-" {
			return nil, fmt.Errorf("define %s: backtracking clauses (<-) are not supported by shengen-ex", def.Name)
		}
		if it.Kind == NAtom && it.Atom == "->" {
			if i+1 >= len(rest) {
				return nil, fmt.Errorf("define %s: missing result after ->", def.Name)
			}
			cl := DefineClause{Patterns: pats, Result: rest[i+1]}
			i++
			if i+2 < len(rest) && rest[i+1].Kind == NAtom && rest[i+1].Atom == "where" {
				cl.Guard = rest[i+2]
				i += 2
			}
			def.Clauses = append(def.Clauses, cl)
			pats = nil
			continue
		}
		pats = append(pats, it)
	}
	if len(pats) > 0 {
		return nil, fmt.Errorf("define %s: dangling patterns without ->", def.Name)
	}
	if len(def.Clauses) == 0 {
		return nil, nil
	}
	return def, nil
}
