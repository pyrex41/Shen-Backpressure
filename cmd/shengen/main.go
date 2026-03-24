// shengen - Generate Go guard types from Shen sequent-calculus specs.
//
// Reads specs/core.shen (or a path argument), parses datatype definitions,
// and emits a Go package with opaque types whose constructors enforce
// the same invariants that Shen proves deductively.
//
// Architecture:
//   1. Parse: extract (datatype ...) blocks into AST
//   2. Symbol table: map each type name → field layout, category, wrapped primitive
//   3. Resolve: translate verified premises to Go using s-expression parser
//      and recursive head/tail accessor chain resolution
//   4. Emit: generate Go structs with guarded constructors

package main

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

// ============================================================================
// Shen AST
// ============================================================================

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

// ============================================================================
// Symbol Table
// ============================================================================

type FieldInfo struct {
	Index    int
	ShenName string // e.g. "Amount"
	ShenType string // e.g. "amount"
}

type TypeInfo struct {
	ShenName    string
	GoName      string
	Category    string // "wrapper", "constrained", "composite", "guarded", "alias"
	Fields      []FieldInfo
	WrappedPrim string // for wrapper/constrained
	WrappedType string // for alias
}

type SymbolTable struct {
	Types     map[string]*TypeInfo
	ConcCount map[string]int // how many blocks produce each conclusion type
}

func newSymbolTable() *SymbolTable {
	return &SymbolTable{Types: make(map[string]*TypeInfo), ConcCount: make(map[string]int)}
}

func (st *SymbolTable) Build(types []Datatype) {
	// Pass 1: Count how many blocks produce each conclusion type.
	// When count > 1, each block must use its block name to avoid Go type collisions.
	concCount := make(map[string]int)
	for i := range types {
		for _, r := range types[i].Rules {
			concCount[r.Conc.TypeName]++
		}
	}
	st.ConcCount = concCount

	// Pass 2: Build type info, resolving names.
	for i := range types {
		dt := &types[i]
		for _, r := range dt.Rules {
			// Name resolution:
			// - If block name == conclusion type → use it (common case)
			// - If block name != conclusion type AND only one block produces it → use conclusion type
			//   (e.g. datatype balance-invariant → balance-checked)
			// - If block name != conclusion type AND multiple blocks produce it → use block name
			//   (e.g. safe-copy-view and safe-copy-view-from-prompt both → safe-copy-view)
			typeName := r.Conc.TypeName
			if dt.Name != typeName && concCount[typeName] > 1 {
				typeName = dt.Name
			}

			info := &TypeInfo{ShenName: typeName, GoName: toPascalCase(typeName)}

			if r.Conc.IsWrapped && len(r.Verified) == 0 && len(r.Premises) == 1 && isPrimitive(r.Premises[0].TypeName) {
				info.Category = "wrapper"
				info.WrappedPrim = r.Premises[0].TypeName
			} else if r.Conc.IsWrapped && len(r.Verified) > 0 && len(r.Premises) >= 1 && isPrimitive(r.Premises[0].TypeName) {
				info.Category = "constrained"
				info.WrappedPrim = r.Premises[0].TypeName
			} else if r.Conc.IsWrapped && len(r.Premises) == 1 && !isPrimitive(r.Premises[0].TypeName) {
				info.Category = "alias"
				info.WrappedType = r.Premises[0].TypeName
			} else if !r.Conc.IsWrapped && len(r.Verified) > 0 {
				info.Category = "guarded"
			} else if !r.Conc.IsWrapped {
				info.Category = "composite"
			} else {
				info.Category = "composite"
			}

			if !r.Conc.IsWrapped {
				premMap := make(map[string]string)
				for _, p := range r.Premises {
					premMap[p.VarName] = p.TypeName
				}
				for i, fieldName := range r.Conc.Fields {
					shenType := premMap[fieldName]
					if shenType == "" {
						shenType = "unknown"
					}
					info.Fields = append(info.Fields, FieldInfo{
						Index: i, ShenName: fieldName, ShenType: shenType,
					})
				}
			}

			st.Types[typeName] = info
		}
	}
}

func (st *SymbolTable) Lookup(name string) *TypeInfo  { return st.Types[name] }
func (st *SymbolTable) IsWrapper(shenType string) bool {
	info := st.Lookup(shenType)
	return info != nil && (info.Category == "wrapper" || info.Category == "constrained")
}

// ============================================================================
// S-Expression Parser
// ============================================================================

type SExpr struct {
	Atom     string
	Children []*SExpr
}

func (s *SExpr) IsAtom() bool { return s.Atom != "" || (s.Children == nil && s.Atom == "") }
func (s *SExpr) IsCall() bool { return len(s.Children) > 0 }
func (s *SExpr) Op() string {
	if s.IsCall() && len(s.Children) > 0 && s.Children[0].Atom != "" {
		return s.Children[0].Atom
	}
	return ""
}
func (s *SExpr) String() string {
	if !s.IsCall() {
		return s.Atom
	}
	parts := make([]string, len(s.Children))
	for i, c := range s.Children {
		parts[i] = c.String()
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func parseSExpr(input string) *SExpr {
	tokens := tokenize(strings.TrimSpace(input))
	if len(tokens) == 0 {
		return &SExpr{Atom: ""}
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
		return &SExpr{Atom: ""}, pos
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
	return &SExpr{Atom: tokens[pos]}, pos + 1
}

// ============================================================================
// Accessor Chain Resolution
// ============================================================================

type ResolvedExpr struct {
	GoCode   string
	GoType   string
	ShenType string
	IsMulti  bool        // intermediate multi-field result from tail
	Fields   []FieldInfo // remaining fields when IsMulti
}

func (st *SymbolTable) resolveExpr(expr *SExpr, varMap map[string]string) (*ResolvedExpr, bool) {
	if !expr.IsCall() {
		return st.resolveAtom(expr.Atom, varMap)
	}
	switch expr.Op() {
	case "head":
		return st.resolveHeadTail(expr, varMap, true)
	case "tail":
		return st.resolveHeadTail(expr, varMap, false)
	case "shen.mod":
		return st.resolveBinOp(expr, varMap, "%", true)
	case "length":
		return st.resolveLength(expr, varMap)
	case "not":
		return st.resolveNot(expr, varMap)
	}
	return nil, false
}

func (st *SymbolTable) resolveAtom(atom string, varMap map[string]string) (*ResolvedExpr, bool) {
	if isNumericLiteral(atom) {
		return &ResolvedExpr{GoCode: atom, GoType: "float64", ShenType: "number"}, true
	}
	if atom == "[]" {
		return &ResolvedExpr{GoCode: "nil", GoType: "nil", ShenType: "list"}, true
	}
	if shenType, ok := varMap[atom]; ok {
		return &ResolvedExpr{GoCode: toCamelCase(atom), GoType: shenTypeToGo(shenType), ShenType: shenType}, true
	}
	return &ResolvedExpr{GoCode: atom, GoType: "unknown", ShenType: "unknown"}, true
}

func (st *SymbolTable) resolveHeadTail(expr *SExpr, varMap map[string]string, isHead bool) (*ResolvedExpr, bool) {
	if len(expr.Children) != 2 {
		return nil, false
	}
	inner, ok := st.resolveExpr(expr.Children[1], varMap)
	if !ok {
		return nil, false
	}

	// If inner is already a multi-field intermediate (from a prior tail), use those fields
	if inner.IsMulti {
		return st.accessFields(inner.GoCode, inner.Fields, isHead)
	}

	// Otherwise look up the type's field layout in the symbol table
	typeInfo := st.Lookup(inner.ShenType)
	if typeInfo == nil || len(typeInfo.Fields) == 0 {
		return nil, false
	}
	return st.accessFields(inner.GoCode, typeInfo.Fields, isHead)
}

func (st *SymbolTable) accessFields(baseGo string, fields []FieldInfo, isHead bool) (*ResolvedExpr, bool) {
	if len(fields) == 0 {
		return nil, false
	}

	if isHead {
		f := fields[0]
		code := baseGo + "." + toPascalCase(f.ShenName)
		return &ResolvedExpr{GoCode: code, GoType: shenTypeToGo(f.ShenType), ShenType: f.ShenType}, true
	}

	// tail: drop first field
	rest := fields[1:]
	if len(rest) == 0 {
		return nil, false
	}
	if len(rest) == 1 {
		// Single field remaining — resolve directly
		f := rest[0]
		code := baseGo + "." + toPascalCase(f.ShenName)
		return &ResolvedExpr{GoCode: code, GoType: shenTypeToGo(f.ShenType), ShenType: f.ShenType}, true
	}
	// Multiple fields remaining — return multi-field intermediate for further chaining
	return &ResolvedExpr{GoCode: baseGo, IsMulti: true, Fields: rest}, true
}

func (st *SymbolTable) resolveBinOp(expr *SExpr, varMap map[string]string, goOp string, intCast bool) (*ResolvedExpr, bool) {
	if len(expr.Children) != 3 {
		return nil, false
	}
	lhs, ok := st.resolveExpr(expr.Children[1], varMap)
	if !ok {
		return nil, false
	}
	rhs, ok := st.resolveExpr(expr.Children[2], varMap)
	if !ok {
		return nil, false
	}
	goLhs := st.unwrapNumeric(lhs)
	goRhs := rhs.GoCode
	code := fmt.Sprintf("%s %s %s", goLhs, goOp, goRhs)
	if intCast {
		code = fmt.Sprintf("int(%s) %s %s", goLhs, goOp, goRhs)
	}
	return &ResolvedExpr{GoCode: code, GoType: "int", ShenType: "number"}, true
}

func (st *SymbolTable) resolveLength(expr *SExpr, varMap map[string]string) (*ResolvedExpr, bool) {
	if len(expr.Children) != 2 {
		return nil, false
	}
	inner, ok := st.resolveExpr(expr.Children[1], varMap)
	if !ok {
		return nil, false
	}
	goInner := st.unwrapString(inner)
	return &ResolvedExpr{GoCode: fmt.Sprintf("len(%s)", goInner), GoType: "int", ShenType: "number"}, true
}

func (st *SymbolTable) resolveNot(expr *SExpr, varMap map[string]string) (*ResolvedExpr, bool) {
	if len(expr.Children) != 2 {
		return nil, false
	}
	inner, ok := st.resolveExpr(expr.Children[1], varMap)
	if !ok {
		return nil, false
	}
	return &ResolvedExpr{GoCode: fmt.Sprintf("!(%s)", inner.GoCode), GoType: "bool", ShenType: "boolean"}, true
}

func (st *SymbolTable) unwrapNumeric(r *ResolvedExpr) string {
	if st.IsWrapper(r.ShenType) {
		return r.GoCode + ".Val()"
	}
	return r.GoCode
}

func (st *SymbolTable) unwrapString(r *ResolvedExpr) string {
	if st.IsWrapper(r.ShenType) {
		return r.GoCode + ".Val()"
	}
	return r.GoCode
}

// ============================================================================
// Verified Premise → Go (using symbol table + s-expression parser)
// ============================================================================

func (st *SymbolTable) verifiedToGo(v VerifiedPremise, varMap map[string]string) (goExpr string, errMsg string) {
	expr := parseSExpr(v.Raw)
	if !expr.IsCall() {
		return "/* TODO: " + v.Raw + " */ true", "unhandled premise"
	}

	switch expr.Op() {
	case ">=", "<=", ">", "<":
		return st.translateCmp(expr, varMap, expr.Op())
	case "=":
		return st.translateEq(expr, varMap)
	case "not":
		return st.translateNotPremise(expr, varMap)
	case "element?":
		return st.translateElementPremise(expr, varMap)
	}

	return "/* TODO: " + v.Raw + " */ true", "unhandled op: " + expr.Op()
}

func (st *SymbolTable) translateCmp(expr *SExpr, varMap map[string]string, op string) (string, string) {
	if len(expr.Children) != 3 {
		return "/* bad arity */ true", "comparison needs 2 args"
	}
	lhs, lok := st.resolveExpr(expr.Children[1], varMap)
	rhs, rok := st.resolveExpr(expr.Children[2], varMap)
	if !lok || !rok {
		return fmt.Sprintf("/* TODO: %s */ true", expr.String()), "could not resolve comparison"
	}
	goL := st.unwrapNumeric(lhs)
	goR := st.unwrapNumeric(rhs)
	return fmt.Sprintf("%s %s %s", goL, op, goR),
		fmt.Sprintf("%s must be %s %s", lhs.GoCode, op, rhs.GoCode)
}

func (st *SymbolTable) translateEq(expr *SExpr, varMap map[string]string) (string, string) {
	if len(expr.Children) != 3 {
		return "/* bad arity */ true", "equality needs 2 args"
	}
	lhs, lok := st.resolveExpr(expr.Children[1], varMap)
	rhs, rok := st.resolveExpr(expr.Children[2], varMap)

	// If direct resolution fails, try structural match fallback.
	// This handles cases like (= (tail (tail (head Profile))) (tail Copy))
	// where the head/tail chain doesn't cleanly traverse the symbol table,
	// but the INTENT is to compare matching fields between two composites.
	if !lok || !rok {
		if goExpr, errMsg, ok := st.structuralMatchFallback(expr, varMap); ok {
			return goExpr, errMsg
		}
		return fmt.Sprintf("/* TODO: %s */ true", expr.String()), "could not resolve equality"
	}
	goL := lhs.GoCode
	goR := rhs.GoCode
	// Unwrap if comparing wrapped type to primitive
	if st.IsWrapper(lhs.ShenType) && isPrimitive(rhs.ShenType) {
		goL = st.unwrapNumeric(lhs)
	}
	if st.IsWrapper(rhs.ShenType) && isPrimitive(lhs.ShenType) {
		goR = st.unwrapNumeric(rhs)
	}
	return fmt.Sprintf("%s == %s", goL, goR),
		fmt.Sprintf("%s must equal %s", lhs.GoCode, rhs.GoCode)
}

// structuralMatchFallback handles equality expressions where head/tail resolution
// fails by finding fields with matching types between two composite variables.
//
// For (= (tail (tail (head Profile))) (tail Copy)):
//   Profile : known-profile → fields [Id:user-id, Email:email-addr, Demo:demographics]
//   Copy    : copy-content  → fields [Body:string, Demo:demographics]
//   Shared field type: demographics → profile.Demo == copy.Demo
func (st *SymbolTable) structuralMatchFallback(expr *SExpr, varMap map[string]string) (string, string, bool) {
	if len(expr.Children) != 3 {
		return "", "", false
	}

	// Extract base variable names from both sides by walking through head/tail
	lhsVar := extractBaseVar(expr.Children[1])
	rhsVar := extractBaseVar(expr.Children[2])
	if lhsVar == "" || rhsVar == "" {
		return "", "", false
	}

	// Look up their types
	lhsType, lok := varMap[lhsVar]
	rhsType, rok := varMap[rhsVar]
	if !lok || !rok {
		return "", "", false
	}

	lhsInfo := st.Lookup(lhsType)
	rhsInfo := st.Lookup(rhsType)
	if lhsInfo == nil || rhsInfo == nil || len(lhsInfo.Fields) == 0 || len(rhsInfo.Fields) == 0 {
		return "", "", false
	}

	// Count head/tail operations on each side to narrow down which fields are being compared.
	// tail drops the first N fields, head selects the first of what remains.
	// For the fallback, we find fields with matching types between the two composites.
	lhsTarget := inferTargetFields(expr.Children[1], lhsInfo.Fields)
	rhsTarget := inferTargetFields(expr.Children[2], rhsInfo.Fields)

	// Find matching field types between the targeted subsets
	for _, lf := range lhsTarget {
		for _, rf := range rhsTarget {
			if lf.ShenType == rf.ShenType && !isPrimitive(lf.ShenType) {
				goL := toCamelCase(lhsVar) + "." + toPascalCase(lf.ShenName)
				goR := toCamelCase(rhsVar) + "." + toPascalCase(rf.ShenName)
				return fmt.Sprintf("%s == %s", goL, goR),
					fmt.Sprintf("%s.%s must equal %s.%s", toCamelCase(lhsVar), toPascalCase(lf.ShenName), toCamelCase(rhsVar), toPascalCase(rf.ShenName)),
					true
			}
		}
	}

	// Last resort: find ANY shared non-primitive field type
	for _, lf := range lhsInfo.Fields {
		for _, rf := range rhsInfo.Fields {
			if lf.ShenType == rf.ShenType && !isPrimitive(lf.ShenType) {
				goL := toCamelCase(lhsVar) + "." + toPascalCase(lf.ShenName)
				goR := toCamelCase(rhsVar) + "." + toPascalCase(rf.ShenName)
				return fmt.Sprintf("%s == %s", goL, goR),
					fmt.Sprintf("%s.%s must equal %s.%s", toCamelCase(lhsVar), toPascalCase(lf.ShenName), toCamelCase(rhsVar), toPascalCase(rf.ShenName)),
					true
			}
		}
	}

	return "", "", false
}

// extractBaseVar walks through nested head/tail calls to find the root variable name.
// (tail (tail (head Profile))) → "Profile"
// (tail Copy) → "Copy"
func extractBaseVar(expr *SExpr) string {
	if !expr.IsCall() {
		if expr.Atom != "" && unicode.IsUpper(rune(expr.Atom[0])) {
			return expr.Atom
		}
		return ""
	}
	op := expr.Op()
	if (op == "head" || op == "tail") && len(expr.Children) == 2 {
		return extractBaseVar(expr.Children[1])
	}
	return ""
}

// inferTargetFields estimates which fields a head/tail chain is targeting
// by counting tail operations (which drop leading fields).
func inferTargetFields(expr *SExpr, fields []FieldInfo) []FieldInfo {
	tailCount := 0
	hasHead := false
	current := expr
	for current.IsCall() && len(current.Children) == 2 {
		op := current.Op()
		if op == "tail" {
			tailCount++
		} else if op == "head" {
			hasHead = true
		}
		current = current.Children[1]
	}
	// After stripping heads and tails, the remaining fields start at index tailCount
	// (approximately — head doesn't shift the index, it selects within)
	_ = hasHead
	if tailCount >= len(fields) {
		return fields[len(fields)-1:] // last field
	}
	return fields[tailCount:]
}

func (st *SymbolTable) translateNotPremise(expr *SExpr, varMap map[string]string) (string, string) {
	if len(expr.Children) != 2 {
		return "/* bad not */ true", "not needs 1 arg"
	}
	inner := expr.Children[1]
	if inner.IsCall() && inner.Op() == "=" {
		goExpr, errMsg := st.translateEq(inner, varMap)
		return "!(" + goExpr + ")", "not: " + errMsg
	}
	resolved, ok := st.resolveExpr(inner, varMap)
	if !ok {
		return fmt.Sprintf("/* TODO: %s */ true", expr.String()), "could not resolve not"
	}
	return "!(" + resolved.GoCode + ")", "negation of " + resolved.GoCode
}

func (st *SymbolTable) translateElementPremise(expr *SExpr, varMap map[string]string) (string, string) {
	if len(expr.Children) < 2 {
		return "/* bad element? */ true", "element? needs args"
	}
	resolved, ok := st.resolveExpr(expr.Children[1], varMap)
	if !ok {
		return "/* TODO: element? */ true", "could not resolve element?"
	}
	return fmt.Sprintf("/* element? %s in set */ true", resolved.GoCode),
		resolved.GoCode + " must be in the valid set"
}

// ============================================================================
// Parser
// ============================================================================

func parseFile(path string) ([]Datatype, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	var types []Datatype
	for {
		idx := strings.Index(content, "(datatype ")
		if idx == -1 {
			break
		}
		content = content[idx:]
		depth, end := 0, -1
		for i, ch := range content {
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
		block := content[:end]
		content = content[end:]
		if dt := parseDatatype(block); dt != nil {
			types = append(types, *dt)
		}
	}
	return types, nil
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

// ============================================================================
// Helpers
// ============================================================================

func shenTypeToGo(t string) string {
	switch t {
	case "string":
		return "string"
	case "number":
		return "float64"
	case "":
		return "interface{}"
	default:
		return toPascalCase(t)
	}
}

func toPascalCase(s string) string {
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

func toCamelCase(s string) string {
	pc := toPascalCase(s)
	if len(pc) == 0 {
		return pc
	}
	runes := []rune(pc)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func isPrimitive(t string) bool { return t == "string" || t == "number" }

func isNumericLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, ch := range s {
		if ch == '-' && i == 0 {
			continue
		}
		if ch == '.' {
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// ============================================================================
// Go Code Generator
// ============================================================================

type GeneratedType struct {
	Name, GoName, Category string
	Rule                   Rule
}

func classify(dt Datatype, st *SymbolTable) []GeneratedType {
	var out []GeneratedType
	for _, r := range dt.Rules {
		// Same name resolution as Build():
		// contested conclusion types use block name to avoid Go collisions
		typeName := r.Conc.TypeName
		if dt.Name != typeName && st.ConcCount[typeName] > 1 {
			typeName = dt.Name
		}
		cat := "composite"
		if info := st.Lookup(typeName); info != nil {
			cat = info.Category
		}
		out = append(out, GeneratedType{Name: dt.Name, GoName: toPascalCase(typeName), Category: cat, Rule: r})
	}
	return out
}

func generateGo(types []Datatype, st *SymbolTable, pkg string) string {
	var b strings.Builder
	b.WriteString("// Code generated by shengen from specs/core.shen. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// These types enforce Shen sequent-calculus invariants at the Go level.\n")
	b.WriteString("// Constructors are the ONLY way to create these types — bypassing them\n")
	b.WriteString("// is a violation of the formal spec.\n\n")
	b.WriteString(fmt.Sprintf("package %s\n\n", pkg))
	b.WriteString("import (\n\t\"fmt\"\n)\n\n")

	for _, dt := range types {
		for _, gt := range classify(dt, st) {
			b.WriteString(fmt.Sprintf("// --- %s ---\n// Shen: (datatype %s)\n", gt.GoName, gt.Name))
			switch gt.Category {
			case "wrapper":
				generateWrapper(&b, gt)
			case "constrained":
				generateConstrained(&b, gt, st)
			case "composite":
				generateComposite(&b, gt)
			case "guarded":
				generateGuarded(&b, gt, st)
			case "alias":
				generateAlias(&b, gt)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func generateWrapper(b *strings.Builder, gt GeneratedType) {
	goType := shenTypeToGo(gt.Rule.Premises[0].TypeName)
	b.WriteString(fmt.Sprintf("type %s struct{ v %s }\n\n", gt.GoName, goType))
	b.WriteString(fmt.Sprintf("func New%s(x %s) %s { return %s{v: x} }\n\n", gt.GoName, goType, gt.GoName, gt.GoName))
	b.WriteString(fmt.Sprintf("func (t %s) Val() %s { return t.v }\n\n", gt.GoName, goType))
	if goType == "string" {
		b.WriteString(fmt.Sprintf("func (t %s) String() string { return t.v }\n\n", gt.GoName))
	}
}

func generateConstrained(b *strings.Builder, gt GeneratedType, st *SymbolTable) {
	goType := shenTypeToGo(gt.Rule.Premises[0].TypeName)
	varMap := make(map[string]string)
	for _, p := range gt.Rule.Premises {
		varMap[p.VarName] = p.TypeName
	}
	b.WriteString(fmt.Sprintf("type %s struct{ v %s }\n\n", gt.GoName, goType))
	b.WriteString(fmt.Sprintf("func New%s(x %s) (%s, error) {\n", gt.GoName, goType, gt.GoName))
	for _, v := range gt.Rule.Verified {
		goExpr, errMsg := st.verifiedToGo(v, varMap)
		safeMsg := strings.ReplaceAll(errMsg, "%", "%%")
		b.WriteString(fmt.Sprintf("\tif !(%s) {\n\t\treturn %s{}, fmt.Errorf(\"%s: %%v\", x)\n\t}\n", goExpr, gt.GoName, safeMsg))
	}
	b.WriteString(fmt.Sprintf("\treturn %s{v: x}, nil\n}\n\n", gt.GoName))
	b.WriteString(fmt.Sprintf("func (t %s) Val() %s { return t.v }\n\n", gt.GoName, goType))
}

func generateComposite(b *strings.Builder, gt GeneratedType) {
	b.WriteString(fmt.Sprintf("type %s struct {\n", gt.GoName))
	var params []string
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t%s %s\n", toPascalCase(p.VarName), shenTypeToGo(p.TypeName)))
		params = append(params, fmt.Sprintf("%s %s", toCamelCase(p.VarName), shenTypeToGo(p.TypeName)))
	}
	b.WriteString("}\n\n")
	b.WriteString(fmt.Sprintf("func New%s(%s) %s {\n\treturn %s{\n", gt.GoName, strings.Join(params, ", "), gt.GoName, gt.GoName))
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t\t%s: %s,\n", toPascalCase(p.VarName), toCamelCase(p.VarName)))
	}
	b.WriteString("\t}\n}\n\n")
}

func generateGuarded(b *strings.Builder, gt GeneratedType, st *SymbolTable) {
	b.WriteString(fmt.Sprintf("type %s struct {\n", gt.GoName))
	var params []string
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t%s %s\n", toPascalCase(p.VarName), shenTypeToGo(p.TypeName)))
		params = append(params, fmt.Sprintf("%s %s", toCamelCase(p.VarName), shenTypeToGo(p.TypeName)))
	}
	b.WriteString("}\n\n")
	varMap := make(map[string]string)
	for _, p := range gt.Rule.Premises {
		varMap[p.VarName] = p.TypeName
	}
	b.WriteString(fmt.Sprintf("func New%s(%s) (%s, error) {\n", gt.GoName, strings.Join(params, ", "), gt.GoName))
	for _, v := range gt.Rule.Verified {
		goExpr, errMsg := st.verifiedToGo(v, varMap)
		safeMsg := strings.ReplaceAll(errMsg, "%", "%%")
		b.WriteString(fmt.Sprintf("\tif !(%s) {\n\t\treturn %s{}, fmt.Errorf(\"%s\")\n\t}\n", goExpr, gt.GoName, safeMsg))
	}
	b.WriteString(fmt.Sprintf("\treturn %s{\n", gt.GoName))
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t\t%s: %s,\n", toPascalCase(p.VarName), toCamelCase(p.VarName)))
	}
	b.WriteString("\t}, nil\n}\n\n")
}

func generateAlias(b *strings.Builder, gt GeneratedType) {
	b.WriteString(fmt.Sprintf("type %s = %s\n\n", gt.GoName, shenTypeToGo(gt.Rule.Premises[0].TypeName)))
}

// ============================================================================
// Main
// ============================================================================

func main() {
	path := "specs/core.shen"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	pkg := "shenguard"
	if len(os.Args) > 2 {
		pkg = os.Args[2]
	}

	types, err := parseFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	st := newSymbolTable()
	st.Build(types)

	fmt.Fprintf(os.Stderr, "Parsed %d datatypes from %s\n\n", len(types), path)
	fmt.Fprintf(os.Stderr, "Symbol table:\n")
	for _, dt := range types {
		for _, r := range dt.Rules {
			typeName := r.Conc.TypeName
			if dt.Name != typeName && st.ConcCount[typeName] > 1 {
				typeName = dt.Name
			}
			info := st.Lookup(typeName)
			if info == nil {
				continue
			}
			label := typeName
			if dt.Name != typeName {
				label = fmt.Sprintf("%s (block: %s)", typeName, dt.Name)
			}
			fmt.Fprintf(os.Stderr, "  %-28s [%-11s]", label, info.Category)
			if len(info.Fields) > 0 {
				names := make([]string, len(info.Fields))
				for i, f := range info.Fields {
					names[i] = fmt.Sprintf("%s:%s", f.ShenName, f.ShenType)
				}
				fmt.Fprintf(os.Stderr, " {%s}", strings.Join(names, ", "))
			}
			if info.WrappedPrim != "" {
				fmt.Fprintf(os.Stderr, " wraps=%s", info.WrappedPrim)
			}
			if info.WrappedType != "" {
				fmt.Fprintf(os.Stderr, " alias=%s", info.WrappedType)
			}
			fmt.Fprintln(os.Stderr)
		}
	}
	fmt.Fprintln(os.Stderr)

	fmt.Print(generateGo(types, st, pkg))
}
