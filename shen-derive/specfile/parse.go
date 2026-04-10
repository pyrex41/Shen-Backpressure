// Package specfile parses .shen specification files for shen-derive.
//
// A .shen file contains (datatype ...) blocks (which shengen consumes to
// generate guard types) and (define ...) blocks (which shen-derive consumes
// to verify implementations match the spec).
//
// The parser is intentionally minimal: balanced-paren extraction for blocks,
// line-oriented parsing for datatypes, and delegation to core.ParseSexpr for
// define bodies. It mirrors shengen's parser (cmd/shengen/main.go) closely
// enough that both tools read the same files.
package specfile

import (
	"fmt"
	"os"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// SpecFile holds everything extracted from a .shen file.
type SpecFile struct {
	Path      string
	Datatypes []Datatype
	Defines   []Define
}

// Datatype is a parsed (datatype ...) block.
type Datatype struct {
	Name  string
	Rules []Rule
}

// Rule is a single inference rule inside a datatype block.
type Rule struct {
	Premises   []Premise         // "X : number;"
	Verified   []VerifiedPremise // "(>= X 0) : verified;"
	Conclusion Conclusion        // "X : amount;" or "[A B C] : transaction;"
}

type Premise struct {
	VarName  string
	TypeName string
}

type VerifiedPremise struct {
	Raw string // raw s-expression text, e.g. "(>= X 0)"
}

type Conclusion struct {
	Fields    []string // ["X"] or ["Amount", "From", "To"]
	TypeName  string
	IsWrapped bool // true when a single un-bracketed field
}

// Define is a parsed (define ...) block.
//
// shen-derive supports two shapes of define, both taken from Shen itself:
//
//  1. Single clause with a simple variable-symbol head:
//
//     (define processable
//       {amount --> (list transaction) --> boolean}
//       B0 Txs -> (foldr ...))
//
//  2. Multiple clauses with pattern matching and optional `where` guards:
//
//     (define pair-in-list?
//       _ _ [] -> false
//       A B [[X Y] | Rest] -> true  where (and (= A X) (= B Y))
//       A B [_ | Rest] -> (pair-in-list? A B Rest))
//
// The type signature is optional — shen-derive can parse an un-typed
// define, but the `verify` command will refuse to generate tests for one
// (since it has no way to sample inputs without the parameter types).
type Define struct {
	Name       string
	TypeSig    TypeSig  // may be zero-value if the define is untyped
	ParamNames []string // derived from the first clause's patterns
	Clauses    []Clause
}

// Clause is one pattern-match clause inside a (define ...) block.
type Clause struct {
	Patterns []core.Sexpr // one per parameter, parsed by core.ParseSexpr
	Guard    core.Sexpr   // nil when the clause has no `where` expression
	Body     core.Sexpr   // the expression after `->`
}

// Arity returns the number of parameters this define takes, derived from
// the first clause's patterns. Returns 0 if there are no clauses.
func (d *Define) Arity() int {
	if len(d.Clauses) == 0 {
		return 0
	}
	return len(d.Clauses[0].Patterns)
}

// TypeSig is a parsed {a --> b --> c} function type signature.
type TypeSig struct {
	ParamTypes []string // ["amount", "(list transaction)"]
	ReturnType string   // "boolean"
}

// ParseFile reads a .shen file and extracts its datatypes and defines.
func ParseFile(path string) (*SpecFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := stripShenComments(string(data))

	sf := &SpecFile{Path: path}

	for _, block := range extractBlocks(content, "(datatype ") {
		if dt, err := parseDatatype(block); err != nil {
			return nil, fmt.Errorf("%s: datatype: %w", path, err)
		} else if dt != nil {
			sf.Datatypes = append(sf.Datatypes, *dt)
		}
	}

	for _, block := range extractBlocks(content, "(define ") {
		if def, err := parseDefine(block); err != nil {
			return nil, fmt.Errorf("%s: define: %w", path, err)
		} else if def != nil {
			sf.Defines = append(sf.Defines, *def)
		}
	}

	return sf, nil
}

// FindDefine returns the named define block or nil.
func (sf *SpecFile) FindDefine(name string) *Define {
	for i := range sf.Defines {
		if sf.Defines[i].Name == name {
			return &sf.Defines[i]
		}
	}
	return nil
}

// FindDatatype returns the named datatype block or nil.
func (sf *SpecFile) FindDatatype(name string) *Datatype {
	for i := range sf.Datatypes {
		if sf.Datatypes[i].Name == name {
			return &sf.Datatypes[i]
		}
	}
	return nil
}

// --- Comment stripping ---

// stripShenComments removes \* ... *\ block comments from Shen source.
// It does NOT touch string literals since Shen strings use double quotes
// and don't contain the comment delimiters.
func stripShenComments(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\\' && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*\\")
			if end == -1 {
				break // unterminated comment — drop rest
			}
			i += end + 4
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// --- Balanced-paren block extraction ---

// extractBlocks finds balanced-paren blocks beginning with prefix.
// Matches shengen's extractBlocks (cmd/shengen/main.go:1129) exactly.
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

// --- Datatype parser ---

func parseDatatype(block string) (*Datatype, error) {
	block = strings.TrimPrefix(block, "(datatype ")
	nlIdx := strings.Index(block, "\n")
	if nlIdx == -1 {
		return nil, nil
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
		return nil, nil
	}
	return dt, nil
}

func allChar(s string, ch rune) bool {
	for _, c := range s {
		if c != ch {
			return false
		}
	}
	return true
}

func buildRule(premLines, concLines []string) *Rule {
	r := &Rule{}
	for _, line := range premLines {
		line = strings.TrimSuffix(strings.TrimSpace(line), ";")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ": verified") {
			raw := strings.TrimSpace(strings.TrimSuffix(line, ": verified"))
			r.Verified = append(r.Verified, VerifiedPremise{Raw: raw})
			continue
		}
		if strings.HasPrefix(line, "if ") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "if "))
			r.Verified = append(r.Verified, VerifiedPremise{Raw: raw})
			continue
		}
		if parts := strings.SplitN(line, " : ", 2); len(parts) == 2 {
			r.Premises = append(r.Premises, Premise{
				VarName:  strings.TrimSpace(parts[0]),
				TypeName: strings.TrimSpace(parts[1]),
			})
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
	r.Conclusion.TypeName = rhs
	if strings.HasPrefix(lhs, "[") && strings.HasSuffix(lhs, "]") {
		r.Conclusion.Fields = strings.Fields(lhs[1 : len(lhs)-1])
	} else {
		r.Conclusion.Fields = []string{lhs}
		r.Conclusion.IsWrapped = true
	}
	return r
}

// --- Define parser ---

func parseDefine(block string) (*Define, error) {
	// Strip the outer "(define " and matching ")".
	inner := strings.TrimPrefix(block, "(define ")
	inner = strings.TrimSuffix(inner, ")")
	inner = strings.TrimSpace(inner)

	// First token is the name.
	nameEnd := strings.IndexAny(inner, " \t\n")
	if nameEnd == -1 {
		return nil, fmt.Errorf("define: missing body")
	}
	name := strings.TrimSpace(inner[:nameEnd])
	rest := strings.TrimSpace(inner[nameEnd:])

	// Optionally parse a {...} type signature. Shen allows un-typed defines
	// (see dosage-calculator/specs/core.shen for examples); the verify
	// command will later refuse to run on them.
	var sig TypeSig
	if strings.HasPrefix(rest, "{") {
		sigEnd := strings.Index(rest, "}")
		if sigEnd == -1 {
			return nil, fmt.Errorf("define %s: unterminated type signature", name)
		}
		var err error
		sig, err = parseTypeSig(rest[:sigEnd+1])
		if err != nil {
			return nil, fmt.Errorf("define %s: %w", name, err)
		}
		rest = strings.TrimSpace(rest[sigEnd+1:])
	}

	clauses, err := parseDefineClauses(rest)
	if err != nil {
		return nil, fmt.Errorf("define %s: %w", name, err)
	}
	if len(clauses) == 0 {
		return nil, fmt.Errorf("define %s: no clauses", name)
	}

	// Derive ParamNames from the first clause's patterns. If all patterns
	// are simple uppercase symbols we use them directly (so existing
	// single-clause specs like `processable` keep referring to `B0`/`Txs`).
	// Otherwise we synthesize positional names.
	paramNames := deriveParamNames(clauses[0].Patterns)

	if len(sig.ParamTypes) > 0 && len(paramNames) != len(sig.ParamTypes) {
		return nil, fmt.Errorf("define %s: %d params in first clause, %d in type sig",
			name, len(paramNames), len(sig.ParamTypes))
	}

	return &Define{
		Name:       name,
		TypeSig:    sig,
		ParamNames: paramNames,
		Clauses:    clauses,
	}, nil
}

// parseDefineClauses splits a define body into its pattern-match clauses.
// Mirrors the algorithm in cmd/shengen/main.go:parseDefine.
func parseDefineClauses(body string) ([]Clause, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("empty define body")
	}

	// Collapse whitespace and split on the " -> " separator. The resulting
	// alternating layout is:
	//   segments[0]  = first clause patterns
	//   segments[i]  = previous clause result (+ optional `where GUARD`)
	//                  followed by next clause patterns (i = 1 .. n-2)
	//   segments[n-1] = last clause result
	bodyOneLine := strings.Join(strings.Fields(body), " ")
	segments := strings.Split(bodyOneLine, " -> ")
	if len(segments) < 2 {
		return nil, fmt.Errorf("missing '->' in define body")
	}

	var clauses []Clause
	currentPatterns := segments[0]

	for i := 1; i < len(segments); i++ {
		seg := segments[i]
		var resultStr, guardStr, nextPatterns string

		if whereIdx := strings.Index(seg, " where "); whereIdx != -1 {
			resultStr = strings.TrimSpace(seg[:whereIdx])
			afterWhere := strings.TrimSpace(seg[whereIdx+len(" where "):])
			if strings.HasPrefix(afterWhere, "(") {
				expr, endIdx := extractBalancedParen(afterWhere)
				guardStr = expr
				nextPatterns = strings.TrimSpace(afterWhere[endIdx:])
			} else {
				// Rare: non-paren guard (single atom). Take the first token.
				toks := strings.Fields(afterWhere)
				if len(toks) == 0 {
					return nil, fmt.Errorf("empty `where` guard in clause %d", i)
				}
				guardStr = toks[0]
				nextPatterns = strings.Join(toks[1:], " ")
			}
		} else {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "(") {
				expr, endIdx := extractBalancedParen(seg)
				resultStr = expr
				nextPatterns = strings.TrimSpace(seg[endIdx:])
			} else {
				toks := strings.Fields(seg)
				if len(toks) == 0 {
					return nil, fmt.Errorf("empty clause result at segment %d", i)
				}
				resultStr = toks[0]
				if len(toks) > 1 {
					nextPatterns = strings.Join(toks[1:], " ")
				}
			}
		}

		patternStrs := splitPatterns(currentPatterns)
		if len(patternStrs) == 0 {
			return nil, fmt.Errorf("clause %d: no patterns", len(clauses))
		}

		patterns := make([]core.Sexpr, len(patternStrs))
		for j, ps := range patternStrs {
			s, err := core.ParseSexpr(ps)
			if err != nil {
				return nil, fmt.Errorf("clause %d: pattern %q: %w", len(clauses), ps, err)
			}
			patterns[j] = s
		}

		body, err := core.ParseSexpr(resultStr)
		if err != nil {
			return nil, fmt.Errorf("clause %d: body %q: %w", len(clauses), resultStr, err)
		}

		var guard core.Sexpr
		if guardStr != "" {
			guard, err = core.ParseSexpr(guardStr)
			if err != nil {
				return nil, fmt.Errorf("clause %d: where guard %q: %w",
					len(clauses), guardStr, err)
			}
		}

		clauses = append(clauses, Clause{
			Patterns: patterns,
			Guard:    guard,
			Body:     body,
		})
		currentPatterns = nextPatterns
	}

	// Sanity: every clause should have the same arity as the first.
	want := len(clauses[0].Patterns)
	for i, cl := range clauses {
		if len(cl.Patterns) != want {
			return nil, fmt.Errorf("clause %d has %d patterns, expected %d",
				i, len(cl.Patterns), want)
		}
	}

	return clauses, nil
}

// splitPatterns tokenizes a pattern string respecting bracket nesting.
// "[Med | Meds]" stays as one token; "[[X Y] | Rest]" stays as one token.
// Ported from cmd/shengen/main.go:splitPatterns.
func splitPatterns(s string) []string {
	var patterns []string
	var current strings.Builder
	depth := 0
	flush := func() {
		if current.Len() > 0 {
			patterns = append(patterns, current.String())
			current.Reset()
		}
	}
	for _, ch := range s {
		switch ch {
		case '[':
			depth++
			current.WriteRune(ch)
		case ']':
			depth--
			current.WriteRune(ch)
			if depth == 0 {
				flush()
			}
		case ' ', '\t':
			if depth > 0 {
				current.WriteRune(ch)
			} else {
				flush()
			}
		default:
			current.WriteRune(ch)
		}
	}
	flush()
	return patterns
}

// extractBalancedParen returns the balanced parenthesized expression at the
// start of s (including the outer parens) and the index just past its end.
// Returns ("", 0) if s does not start with '('. Ported from shengen.
func extractBalancedParen(s string) (string, int) {
	if len(s) == 0 || s[0] != '(' {
		return "", 0
	}
	depth := 0
	for i, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i+1], i + 1
			}
		}
	}
	return s, len(s)
}

// deriveParamNames inspects the first clause's patterns and returns names
// suitable for Go test field generation. Simple uppercase-symbol patterns
// keep their names; everything else becomes "p0", "p1", ...
func deriveParamNames(patterns []core.Sexpr) []string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		out[i] = fmt.Sprintf("p%d", i)
		if atom, ok := p.(*core.Atom); ok && atom.Kind == core.AtomSymbol {
			if name := atom.Val; len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
				out[i] = name
			}
		}
	}
	return out
}

// parseTypeSig parses "{a --> b --> c}" into ParamTypes=[a, b], ReturnType=c.
func parseTypeSig(sig string) (TypeSig, error) {
	sig = strings.TrimSpace(sig)
	if !strings.HasPrefix(sig, "{") || !strings.HasSuffix(sig, "}") {
		return TypeSig{}, fmt.Errorf("type sig must be wrapped in {...}")
	}
	inner := strings.TrimSpace(sig[1 : len(sig)-1])

	// Split on " --> " respecting parenthesized list types.
	// Since "(list T)" doesn't contain "-->", a plain string split is safe.
	parts := splitArrow(inner)
	if len(parts) < 2 {
		return TypeSig{}, fmt.Errorf("type sig needs at least one arrow: %q", sig)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return TypeSig{
		ParamTypes: parts[:len(parts)-1],
		ReturnType: parts[len(parts)-1],
	}, nil
}

// splitArrow splits on " --> " outside of parentheses.
func splitArrow(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && i+4 < len(s) && s[i:i+5] == " --> " {
			parts = append(parts, s[start:i])
			start = i + 5
			i += 4
		}
	}
	parts = append(parts, s[start:])
	return parts
}

