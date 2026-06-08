// Package policyspec holds the Shen-spec parsing and target-selection primitives
// shared by the runtime-policy emitters (shen-cedar, shen-rego). It was extracted
// from the two emitters, which previously carried byte-for-byte copies of the
// S-expression parser, datatype/rule parser, rule map, transitive-premise walk,
// and target inference. Keeping one copy here removes ~250 lines of duplication
// per emitter and a class of drift bugs (e.g. the camel-case helpers silently
// diverging).
//
// There is a single, robust datatype parser. The emitters used to also carry a
// crude "v0" parser (parseDatatypeForCedar / parseDatatypeForRego) that mistook
// premise type annotations for conclusions; CollectConclusions now uses the
// robust ParseDatatypes so only real conclusions are surfaced.
package policyspec

import (
	"strconv"
	"strings"
	"unicode"
)

// ----------------------------------------------------------------------------
// S-expression parser (used to lower verified premises like (= IsMember true)).
// ----------------------------------------------------------------------------

type SExpr struct {
	Atom     string
	Children []*SExpr
	IsLeaf   bool
}

func (s *SExpr) IsAtom() bool { return s.IsLeaf }
func (s *SExpr) IsCall() bool { return len(s.Children) > 0 }
func (s *SExpr) Op() string {
	if s.IsCall() && len(s.Children) > 0 && s.Children[0].Atom != "" {
		return s.Children[0].Atom
	}
	return ""
}

func ParseSExpr(input string) *SExpr {
	tokens := tokenize(strings.TrimSpace(input))
	if len(tokens) == 0 {
		return &SExpr{Atom: "", IsLeaf: true}
	}
	expr, _ := parseTokens(tokens, 0)
	return expr
}

func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, ch := range s {
		switch ch {
		case '(', ')':
			flush()
			tokens = append(tokens, string(ch))
		case ' ', '\t', '\n':
			flush()
		default:
			cur.WriteRune(ch)
		}
	}
	flush()
	return tokens
}

func parseTokens(tokens []string, pos int) (*SExpr, int) {
	if pos >= len(tokens) {
		return &SExpr{Atom: "", IsLeaf: true}, pos
	}
	if tokens[pos] == "(" {
		pos++
		var children []*SExpr
		for pos < len(tokens) && tokens[pos] != ")" {
			child, np := parseTokens(tokens, pos)
			children = append(children, child)
			pos = np
		}
		if pos < len(tokens) {
			pos++
		}
		return &SExpr{Children: children}, pos
	}
	return &SExpr{Atom: tokens[pos], IsLeaf: true}, pos + 1
}

// ----------------------------------------------------------------------------
// Shen datatype / sequent-rule model (subset of shengen).
// ----------------------------------------------------------------------------

type Premise struct {
	VarName  string
	TypeName string
}

type VerifiedPremise struct {
	Raw string
}

type Conclusion struct {
	Fields    []string
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

// extractBlocks pulls top-level (prefix ...) blocks out of content, balancing
// parens. Robust version from shengen.
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

func ParseDatatypes(content string) []Datatype {
	var out []Datatype
	for _, block := range extractBlocks(content, "(datatype ") {
		if dt := parseDatatype(block); dt != nil {
			out = append(out, *dt)
		}
	}
	return out
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

func buildRule(premLines, concLines []string) *Rule {
	r := &Rule{}
	for _, line := range premLines {
		line = strings.TrimSuffix(strings.TrimSpace(line), ";")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ": verified") {
			r.Verified = append(r.Verified, VerifiedPremise{Raw: strings.TrimSpace(strings.TrimSuffix(line, ": verified"))})
			continue
		}
		if strings.HasPrefix(line, "if ") {
			r.Verified = append(r.Verified, VerifiedPremise{Raw: strings.TrimSpace(strings.TrimPrefix(line, "if "))})
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
		r.Conc.Fields = strings.Fields(lhs[1 : len(lhs)-1])
	} else {
		r.Conc.Fields = []string{lhs}
		r.Conc.IsWrapped = true
	}
	return r
}

// ----------------------------------------------------------------------------
// Rule lookup + transitive precondition collection.
// ----------------------------------------------------------------------------

// RuleInfo pairs a parsed rule with the datatype it came from.
type RuleInfo struct {
	Rule   Rule
	DtName string
}

// BuildRuleMap indexes rules by their conclusion type name (first wins).
func BuildRuleMap(dts []Datatype) map[string]RuleInfo {
	m := make(map[string]RuleInfo)
	for _, dt := range dts {
		for _, r := range dt.Rules {
			key := r.Conc.TypeName
			if _, exists := m[key]; !exists {
				m[key] = RuleInfo{Rule: r, DtName: dt.Name}
			}
		}
	}
	return m
}

// BuildRuleVariants indexes ALL rules concluding each type name (not just the
// first). A conclusion with more than one rule is a sum type — e.g.
// authenticated-principal = human-principal ∨ service-principal — and every
// variant must be considered when lowering, or one branch is silently dropped.
func BuildRuleVariants(dts []Datatype) map[string][]RuleInfo {
	m := make(map[string][]RuleInfo)
	for _, dt := range dts {
		for _, r := range dt.Rules {
			key := r.Conc.TypeName
			m[key] = append(m[key], RuleInfo{Rule: r, DtName: dt.Name})
		}
	}
	return m
}

// CollectClauses returns the disjunctive normal form — a slice of conjunctions —
// of the verified premises constraining target. Typed premises that reference
// other rules are expanded transitively; a sum-typed conclusion (multiple
// variant rules) expands to a disjunction, so the lowering encodes "any variant"
// rather than dropping all but the first. Cycles are broken via seen (a cyclic
// edge contributes no further premises down that path). The result always has at
// least one clause (possibly empty = no constraint).
func CollectClauses(target string, variants map[string][]RuleInfo, seen map[string]bool) [][]VerifiedPremise {
	if seen[target] {
		return [][]VerifiedPremise{nil}
	}
	rules, ok := variants[target]
	if !ok {
		return [][]VerifiedPremise{nil}
	}
	seen[target] = true
	defer delete(seen, target)

	var all [][]VerifiedPremise
	for _, ri := range rules {
		// Each variant starts as one clause of its own verified premises, then
		// ANDs in every typed premise that references a rule (cartesian product
		// across their disjunctions keeps the result in DNF).
		clauses := [][]VerifiedPremise{append([]VerifiedPremise(nil), ri.Rule.Verified...)}
		for _, p := range ri.Rule.Premises {
			if _, isRule := variants[p.TypeName]; !isRule {
				continue
			}
			clauses = crossProduct(clauses, CollectClauses(p.TypeName, variants, seen))
		}
		all = append(all, clauses...)
	}
	if len(all) == 0 {
		return [][]VerifiedPremise{nil}
	}
	return all
}

// crossProduct distributes AND over two DNF expressions: every clause of a
// combined with every clause of b (concatenating their premises).
func crossProduct(a, b [][]VerifiedPremise) [][]VerifiedPremise {
	out := make([][]VerifiedPremise, 0, len(a)*len(b))
	for _, ca := range a {
		for _, cb := range b {
			merged := make([]VerifiedPremise, 0, len(ca)+len(cb))
			merged = append(merged, ca...)
			merged = append(merged, cb...)
			out = append(out, merged)
		}
	}
	return out
}

// EvalClauses evaluates DNF clauses against env: a disjunction over clauses,
// each clause a conjunction (EvalVerified). ok is false only if every clause is
// indeterminate (so callers can treat indeterminate-everywhere as deny).
func EvalClauses(clauses [][]VerifiedPremise, env map[string]any) (result bool, ok bool) {
	anyOK := false
	for _, c := range clauses {
		v, vok := EvalVerified(c, env)
		if vok {
			anyOK = true
			if v {
				return true, true
			}
		}
	}
	return false, anyOK
}

// CollectTransitiveVerified gathers the verified premises constraining a target,
// following typed premises that reference other rules. A resource-access rule
// with `Access : tenant-access` inherits tenant-access's membership check, so a
// lowered policy enforces the full precondition chain rather than only the
// rule's own `: verified` lines. Dependency premises come first; cycles are
// guarded via seen.
func CollectTransitiveVerified(target string, ruleMap map[string]RuleInfo, seen map[string]bool) []VerifiedPremise {
	if seen[target] {
		return nil
	}
	seen[target] = true
	ri, ok := ruleMap[target]
	if !ok {
		return nil
	}
	var out []VerifiedPremise
	for _, p := range ri.Rule.Premises {
		if _, isRule := ruleMap[p.TypeName]; isRule {
			out = append(out, CollectTransitiveVerified(p.TypeName, ruleMap, seen)...)
		}
	}
	out = append(out, ri.Rule.Verified...)
	return out
}

// ----------------------------------------------------------------------------
// Target selection.
// ----------------------------------------------------------------------------

// ParseTargets splits and trims a comma-separated --targets flag value.
func ParseTargets(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// CollectConclusions returns every rule conclusion type name in the spec, using
// the robust parser (only real conclusions, not premise type annotations).
func CollectConclusions(specContent string) []string {
	var conclusions []string
	for _, dt := range ParseDatatypes(specContent) {
		for _, r := range dt.Rules {
			conclusions = append(conclusions, r.Conc.TypeName)
		}
	}
	return conclusions
}

// InferAccessTargets selects access-shaped conclusions by name suffix
// (-access / -permit / -allow, case-insensitive). Order-preserving.
func InferAccessTargets(allConclusions []string) []string {
	var requested []string
	for _, t := range allConclusions {
		lt := strings.ToLower(t)
		if strings.HasSuffix(lt, "-access") || strings.HasSuffix(lt, "-permit") || strings.HasSuffix(lt, "-allow") {
			requested = append(requested, t)
		}
	}
	return requested
}

// SelectTargets returns explicit targets (when --targets is non-empty) or falls
// back to suffix inference over the spec's conclusions.
func SelectTargets(targetsFlag string, allConclusions []string) []string {
	requested := ParseTargets(targetsFlag)
	if len(requested) == 0 {
		requested = InferAccessTargets(allConclusions)
	}
	return requested
}

// ----------------------------------------------------------------------------
// Small shared helpers.
// ----------------------------------------------------------------------------

func DirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i != -1 {
		return p[:i]
	}
	return "."
}

func IsNumericLiteral(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// ToPascalCase splits on - / _ and upper-cases each segment's first rune.
func ToPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	var b strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

// ToCamelCase is ToPascalCase with a lower-cased leading rune (attr names).
func ToCamelCase(s string) string {
	pc := ToPascalCase(s)
	if len(pc) == 0 {
		return pc
	}
	runes := []rune(pc)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// ToSnakeIdent makes a Rego-friendly identifier: - and . become _, leading rune
// lower-cased (e.g. tenant-access -> tenant_access).
func ToSnakeIdent(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	runes := []rune(s)
	if len(runes) > 0 {
		runes[0] = unicode.ToLower(runes[0])
	}
	return string(runes)
}

// ----------------------------------------------------------------------------
// Total evaluator for the decidable fragment.
//
// EvalVerified is the single source of truth for "does this conjunction of
// verified premises hold for a given assignment". Both the generated total-eval
// stub and the differential harness use it, so the pure-shen tier can never
// drift from a hand-maintained copy. It is total by construction: the fragment
// admits only =, not(=), and numeric comparisons over literals/variables.
// ----------------------------------------------------------------------------

// EvalVerified evaluates a conjunction of verified premises against env, which
// maps camelCase attribute names to values (bool, float64, or string). Returns
// (result, ok). ok is false when a premise uses an unsupported form or
// references a variable absent from env, so callers can treat "indeterminate"
// as deny rather than silently passing.
func EvalVerified(premises []VerifiedPremise, env map[string]any) (result bool, ok bool) {
	for _, p := range premises {
		e := ParseSExpr(p.Raw)
		// Premises the emitters cannot lower (e.g. structural cross-field bindings
		// like (= User (head (head Jwt))) that verify the JWT sub-claim) are not
		// enforced at the policy layer — they are authentication concerns
		// discharged upstream by the guard constructor. Skip them here so the
		// evaluator stays consistent with the emitted Cedar/Rego, which also omit
		// them. This is the "scope to authorization" boundary.
		if !PremiseLowerable(e) {
			continue
		}
		v, vok := evalPremise(e, env)
		if !vok {
			return false, false
		}
		if !v {
			return false, true
		}
	}
	return true, true
}

// PremiseLowerable reports whether a verified premise is in the flat fragment
// the Cedar/Rego emitters can lower: =, not(=), element?, or a numeric
// comparison, whose operands are all atoms (no nested calls like head/tail).
// Premises outside this fragment are treated as upstream-verified preconditions.
func PremiseLowerable(e *SExpr) bool {
	if e == nil || !e.IsCall() {
		return false
	}
	op := e.Op()
	if op == "not" {
		if len(e.Children) != 2 || !e.Children[1].IsCall() {
			return false
		}
		e = e.Children[1]
		op = e.Op()
	}
	switch op {
	case "=", "element?", ">=", "<=", ">", "<":
		for _, c := range e.Children[1:] {
			if c.IsCall() {
				return false
			}
		}
		return true
	}
	return false
}

func evalPremise(e *SExpr, env map[string]any) (bool, bool) {
	if e == nil || !e.IsCall() {
		return false, false
	}
	switch e.Op() {
	case "=":
		if len(e.Children) != 3 {
			return false, false
		}
		l, lok := evalOperand(e.Children[1], env)
		r, rok := evalOperand(e.Children[2], env)
		if !lok || !rok {
			return false, false
		}
		return valuesEqual(l, r), true
	case "not":
		if len(e.Children) != 2 {
			return false, false
		}
		inner, ok := evalPremise(e.Children[1], env)
		if !ok {
			return false, false
		}
		return !inner, true
	case "element?":
		// (element? Elem Coll) -> Elem ∈ Coll, where Coll resolves to a slice.
		if len(e.Children) != 3 {
			return false, false
		}
		el, eok := evalOperand(e.Children[1], env)
		coll, cok := evalOperand(e.Children[2], env)
		if !eok || !cok {
			return false, false
		}
		items, ok := coll.([]any)
		if !ok {
			return false, false
		}
		for _, it := range items {
			if valuesEqual(el, it) {
				return true, true
			}
		}
		return false, true
	case ">=", "<=", ">", "<":
		if len(e.Children) != 3 {
			return false, false
		}
		l, lok := evalOperand(e.Children[1], env)
		r, rok := evalOperand(e.Children[2], env)
		lf, lfok := toFloat(l)
		rf, rfok := toFloat(r)
		if !lok || !rok || !lfok || !rfok {
			return false, false
		}
		switch e.Op() {
		case ">=":
			return lf >= rf, true
		case "<=":
			return lf <= rf, true
		case ">":
			return lf > rf, true
		default:
			return lf < rf, true
		}
	}
	return false, false
}

// evalOperand resolves a literal (true/false/number/"string") or a variable
// (camelCase lookup in env). Returns ok=false for an unbound variable.
func evalOperand(e *SExpr, env map[string]any) (any, bool) {
	if e == nil || e.IsCall() {
		return nil, false
	}
	atom := e.Atom
	switch atom {
	case "":
		return nil, false
	case "true":
		return true, true
	case "false":
		return false, true
	}
	if len(atom) >= 2 && strings.HasPrefix(atom, `"`) && strings.HasSuffix(atom, `"`) {
		return atom[1 : len(atom)-1], true
	}
	if IsNumericLiteral(atom) {
		f, _ := strconv.ParseFloat(atom, 64)
		return f, true
	}
	if v, present := env[ToCamelCase(atom)]; present {
		return v, true
	}
	return nil, false
}

func valuesEqual(a, b any) bool {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			return af == bf
		}
		return false
	}
	return a == b
}

func toFloat(x any) (float64, bool) {
	switch v := x.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}
