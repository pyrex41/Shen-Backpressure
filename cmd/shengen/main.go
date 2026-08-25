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
	"flag"
	"fmt"
	"os"
	"strconv"
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
	// RuntimeVia, when non-empty, names a runtime checker function that
	// will be called to discharge this premise instead of inlining a
	// translated predicate. Set when the spec writes
	// `(predicate) : verified via <name>;`. See docs/RUNTIME-VIA.md for
	// the semantics and the compile-time witness shape.
	RuntimeVia string
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
// Shen Define AST (for helper function definitions)
// ============================================================================

type DefineClause struct {
	Patterns []string // raw pattern per parameter: "_", "[]", "[X | Rest]", "[[X Y] | Rest]", "Drug"
	Result   string   // "true", "false", or "(func-call args...)"
	Guard    string   // raw s-expr after 'where', or ""
}

type Define struct {
	Name    string
	Clauses []DefineClause
}

// DefineParam holds resolved Go type info for a define parameter.
type DefineParam struct {
	GoName       string // camelCase name
	GoType       string // Go type string
	ShenType     string // original Shen type
	IsList       bool
	ElemShenType string // for list params, the element's Shen type
}

// DefineResolved holds type-resolved info for generating Go code.
type DefineResolved struct {
	GoName     string
	ParamTypes []DefineParam
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
	Types          map[string]*TypeInfo
	ConcCount      map[string]int      // how many blocks produce each conclusion type
	SumTypes       map[string][]string // conclusion type → list of concrete block names
	Defines        map[string]*Define
	DefineResolved map[string]*DefineResolved
}

func newSymbolTable() *SymbolTable {
	return &SymbolTable{
		Types:          make(map[string]*TypeInfo),
		ConcCount:      make(map[string]int),
		SumTypes:       make(map[string][]string),
		Defines:        make(map[string]*Define),
		DefineResolved: make(map[string]*DefineResolved),
	}
}

// IsSumType returns true if the given Shen type name is a sum type
// (i.e., multiple datatype blocks produce this conclusion type).
func (st *SymbolTable) IsSumType(name string) bool {
	_, ok := st.SumTypes[name]
	return ok
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
				// Track sum type: conclusion type → concrete block names
				st.SumTypes[r.Conc.TypeName] = append(st.SumTypes[r.Conc.TypeName], typeName)
			}

			info := &TypeInfo{ShenName: typeName, GoName: toPascalCase(typeName)}

			isSumVariant := dt.Name != r.Conc.TypeName && concCount[r.Conc.TypeName] > 1

			if r.Conc.IsWrapped && len(r.Verified) == 0 && len(r.Premises) == 1 && isPrimitive(r.Premises[0].TypeName) {
				info.Category = "wrapper"
				info.WrappedPrim = r.Premises[0].TypeName
			} else if r.Conc.IsWrapped && len(r.Verified) > 0 && len(r.Premises) >= 1 && isPrimitive(r.Premises[0].TypeName) {
				info.Category = "constrained"
				info.WrappedPrim = r.Premises[0].TypeName
			} else if r.Conc.IsWrapped && len(r.Premises) == 1 && !isPrimitive(r.Premises[0].TypeName) && !isSumVariant {
				// Only use alias if NOT a sum type variant — sum variants need distinct types
				info.Category = "alias"
				info.WrappedType = r.Premises[0].TypeName
			} else if !r.Conc.IsWrapped && len(r.Verified) > 0 {
				info.Category = "guarded"
			} else if !r.Conc.IsWrapped || isSumVariant {
				info.Category = "composite"
			} else {
				info.Category = "composite"
			}

			// Build field info for composites/guarded AND sum type variants with wrapped conclusions
			if !r.Conc.IsWrapped || isSumVariant {
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

	// Pass 3: Register synthetic entries for sum types.
	// When downstream types reference a sum type conclusion (e.g. "authenticated-principal"),
	// the symbol table needs an entry so field lookups and accessor resolution work.
	for concType, variants := range st.SumTypes {
		if _, exists := st.Types[concType]; !exists {
			st.Types[concType] = &TypeInfo{
				ShenName: concType,
				GoName:   toPascalCase(concType),
				Category: "sumtype",
			}
		}
		_ = variants
	}
}

func (st *SymbolTable) Lookup(name string) *TypeInfo { return st.Types[name] }
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
	IsLeaf   bool // true when this node is an intentional atom (set during parsing)
}

func (s *SExpr) IsAtom() bool { return s.IsLeaf }
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
		code := baseGo + "." + toCamelCase(f.ShenName)
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
		code := baseGo + "." + toCamelCase(f.ShenName)
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
	goLhs := st.unwrap(lhs)
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
	goInner := st.unwrap(inner)
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

func (st *SymbolTable) unwrap(r *ResolvedExpr) string {
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

	// Check if op matches a defined function (from (define ...) blocks)
	if _, ok := st.Defines[expr.Op()]; ok {
		return st.translateDefineCall(expr, varMap)
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
	goL := st.unwrap(lhs)
	goR := st.unwrap(rhs)
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
		goL = st.unwrap(lhs)
	}
	if st.IsWrapper(rhs.ShenType) && isPrimitive(lhs.ShenType) {
		goR = st.unwrap(rhs)
	}
	return fmt.Sprintf("%s == %s", goL, goR),
		fmt.Sprintf("%s must equal %s", lhs.GoCode, rhs.GoCode)
}

// structuralMatchFallback handles equality expressions where head/tail resolution
// fails by finding fields with matching types between two composite variables.
//
// For (= (tail (tail (head Profile))) (tail Copy)):
//
//	Profile : known-profile → fields [Id:user-id, Email:email-addr, Demo:demographics]
//	Copy    : copy-content  → fields [Body:string, Demo:demographics]
//	Shared field type: demographics → profile.Demo == copy.Demo
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
				goL := toCamelCase(lhsVar) + "." + toCamelCase(lf.ShenName)
				goR := toCamelCase(rhsVar) + "." + toCamelCase(rf.ShenName)
				return fmt.Sprintf("%s == %s", goL, goR),
					fmt.Sprintf("%s.%s must equal %s.%s", toCamelCase(lhsVar), toCamelCase(lf.ShenName), toCamelCase(rhsVar), toCamelCase(rf.ShenName)),
					true
			}
		}
	}

	// Last resort: find ANY shared non-primitive field type
	for _, lf := range lhsInfo.Fields {
		for _, rf := range rhsInfo.Fields {
			if lf.ShenType == rf.ShenType && !isPrimitive(lf.ShenType) {
				goL := toCamelCase(lhsVar) + "." + toCamelCase(lf.ShenName)
				goR := toCamelCase(rhsVar) + "." + toCamelCase(rf.ShenName)
				return fmt.Sprintf("%s == %s", goL, goR),
					fmt.Sprintf("%s.%s must equal %s.%s", toCamelCase(lhsVar), toCamelCase(lf.ShenName), toCamelCase(rhsVar), toCamelCase(rf.ShenName)),
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
		goExpr, _ := st.translateEq(inner, varMap)
		// Prefer human-readable messages over the mechanical "not: X must equal Y"
		// fallback. Pattern-match the inner (= LHS RHS):
		//   (not (= X ""))  → "X must not be empty"
		//   (not (= X Y))   → "X must not equal Y"
		errMsg := st.notEqualsErrMsg(inner, varMap)
		return "!(" + goExpr + ")", errMsg
	}
	resolved, ok := st.resolveExpr(inner, varMap)
	if !ok {
		return fmt.Sprintf("/* TODO: %s */ true", expr.String()), "could not resolve not"
	}
	return "!(" + resolved.GoCode + ")", "negation of " + resolved.GoCode
}

// notEqualsErrMsg builds a friendlier error message for `(not (= LHS RHS))`
// premises. Falls back to "not: <eq message>" for anything that doesn't fit
// the recognized shapes (empty-string and same-side inequality).
func (st *SymbolTable) notEqualsErrMsg(eqExpr *SExpr, varMap map[string]string) string {
	_, fallback := st.translateEq(eqExpr, varMap)
	if len(eqExpr.Children) != 3 {
		return "not: " + fallback
	}
	lhs, lok := st.resolveExpr(eqExpr.Children[1], varMap)
	rhs, rok := st.resolveExpr(eqExpr.Children[2], varMap)
	if !lok || !rok {
		return "not: " + fallback
	}
	if isEmptyStringLiteral(rhs.GoCode) {
		return lhs.GoCode + " must not be empty"
	}
	if isEmptyStringLiteral(lhs.GoCode) {
		return rhs.GoCode + " must not be empty"
	}
	return lhs.GoCode + " must not equal " + rhs.GoCode
}

// isEmptyStringLiteral returns true if a generated expression is the literal "".
func isEmptyStringLiteral(goCode string) bool {
	return goCode == `""`
}

// negateGoExpr returns the boolean negation of a Go expression, applying a
// peephole that strips a redundant outer `!(...)`. This keeps the generated
// gate-site readable: a predicate like `!(x == "")` becomes `(x == "")`
// directly in the violation branch, instead of the visually awful
// `!(!(x == ""))`.
func negateGoExpr(goExpr string) string {
	if inner, ok := stripOuterNotParen(goExpr); ok {
		return "(" + inner + ")"
	}
	return "!(" + goExpr + ")"
}

// stripOuterNotParen returns (inner, true) when goExpr has the exact shape
// `!(<inner>)` with balanced parens — i.e. the leading `!(` matches the
// trailing `)`. Otherwise returns ("", false).
func stripOuterNotParen(goExpr string) (string, bool) {
	if !strings.HasPrefix(goExpr, "!(") || !strings.HasSuffix(goExpr, ")") {
		return "", false
	}
	// Walk the parens after the leading `!(` to ensure the matching `)` is the
	// trailing one. If a depth-zero `)` appears earlier the leading `!(` does
	// not bracket the whole expression (e.g. `!(a) && b`).
	depth := 1
	for i := 2; i < len(goExpr)-1; i++ {
		switch goExpr[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return "", false
			}
		}
	}
	if depth != 1 {
		return "", false
	}
	return goExpr[2 : len(goExpr)-1], true
}

func (st *SymbolTable) translateElementPremise(expr *SExpr, varMap map[string]string) (string, string) {
	if len(expr.Children) < 3 {
		return "/* TODO: element? */ true", "element? needs args"
	}
	resolved, ok := st.resolveExpr(expr.Children[1], varMap)
	if !ok {
		return "/* TODO: element? */ true", "could not resolve element?"
	}
	// Collect set elements from remaining children.
	// Shen list [a b c] tokenizes as atoms with brackets attached,
	// e.g. "[a", "b", "c]" — strip brackets to get clean element names.
	var elements []string
	for i := 2; i < len(expr.Children); i++ {
		atom := expr.Children[i].Atom
		if atom == "" {
			continue
		}
		atom = strings.TrimLeft(atom, "[")
		atom = strings.TrimRight(atom, "]")
		if atom != "" {
			elements = append(elements, atom)
		}
	}
	if len(elements) > 0 {
		varCode := st.unwrap(resolved)
		// Generate a map literal membership check
		var pairs []string
		for _, e := range elements {
			// Shen string atoms retain their surrounding quotes in the simple
			// tokenizer. Unquote them before applying %q so the generated Go key
			// is "none", not "\"none\"". Bare Shen symbols remain unchanged.
			if unquoted, err := strconv.Unquote(e); err == nil {
				e = unquoted
			}
			pairs = append(pairs, fmt.Sprintf("%q: true", e))
		}
		mapLiteral := "map[string]bool{" + strings.Join(pairs, ", ") + "}"
		return fmt.Sprintf("%s[%s]", mapLiteral, varCode),
			resolved.GoCode + " must be in the valid set"
	}
	return fmt.Sprintf("/* TODO: element? %s */ true", resolved.GoCode),
		resolved.GoCode + " must be in the valid set"
}

// ============================================================================
// Define Call Translation
// ============================================================================

// defineGoName converts a Shen function name like "pair-in-list?" to Go "pairInList".
func defineGoName(shenName string) string {
	name := strings.TrimSuffix(shenName, "?")
	return toCamelCase(name)
}

func (st *SymbolTable) translateDefineCall(expr *SExpr, varMap map[string]string) (string, string) {
	goName := defineGoName(expr.Op())
	var args []string
	for i := 1; i < len(expr.Children); i++ {
		resolved, ok := st.resolveExpr(expr.Children[i], varMap)
		if !ok {
			return fmt.Sprintf("/* TODO: %s */ true", expr.String()), "could not resolve define call args"
		}
		args = append(args, resolved.GoCode)
	}

	// Resolve parameter types on first encounter for code generation
	if _, ok := st.DefineResolved[expr.Op()]; !ok {
		st.resolveDefineTypes(expr, varMap)
	}

	return fmt.Sprintf("%s(%s)", goName, strings.Join(args, ", ")),
		fmt.Sprintf("%s check failed", goName)
}

func (st *SymbolTable) resolveDefineTypes(expr *SExpr, varMap map[string]string) {
	defName := expr.Op()
	def := st.Defines[defName]
	resolved := &DefineResolved{GoName: defineGoName(defName)}
	for i := 1; i < len(expr.Children); i++ {
		r, ok := st.resolveExpr(expr.Children[i], varMap)

		// Prefer pattern variable names from the define's clauses over call-site names.
		// This ensures generated function params match the guard variable references.
		paramName := "arg" + strconv.Itoa(i-1)
		if def != nil {
			for _, clause := range def.Clauses {
				idx := i - 1
				if idx < len(clause.Patterns) {
					pat := clause.Patterns[idx]
					if pat != "_" && pat != "[]" && !strings.HasPrefix(pat, "[") {
						paramName = toCamelCase(pat)
						break
					}
				}
			}
		}
		// Fallback to call-site atom name
		if paramName == "arg"+strconv.Itoa(i-1) {
			if expr.Children[i].IsAtom() && expr.Children[i].Atom != "" {
				paramName = toCamelCase(expr.Children[i].Atom)
			}
		}

		if !ok {
			resolved.ParamTypes = append(resolved.ParamTypes, DefineParam{GoName: paramName, GoType: "any", ShenType: "unknown"})
			continue
		}
		param := DefineParam{
			GoName:   paramName,
			GoType:   r.GoType,
			ShenType: r.ShenType,
		}
		if elem := listElemType(r.ShenType); elem != "" {
			param.IsList = true
			param.ElemShenType = elem
			param.GoType = "[]" + shenTypeToGo(elem)
		}
		resolved.ParamTypes = append(resolved.ParamTypes, param)
	}
	st.DefineResolved[defName] = resolved
}

// ============================================================================
// Define → Go Code Generation
// ============================================================================

// analyzeDefine determines the loop structure of a define.
func analyzeDefine(def *Define) (loopParamIdx int, baseResult string) {
	loopParamIdx = -1
	baseResult = "false"

	// Find which parameter has list destructuring [... | Rest]
	for _, clause := range def.Clauses {
		for j, pat := range clause.Patterns {
			if strings.Contains(pat, "|") {
				loopParamIdx = j
				break
			}
		}
		if loopParamIdx >= 0 {
			break
		}
	}

	// Find base case (empty list pattern [])
	for _, clause := range def.Clauses {
		if loopParamIdx >= 0 && loopParamIdx < len(clause.Patterns) {
			if clause.Patterns[loopParamIdx] == "[]" {
				baseResult = clause.Result
				break
			}
		}
	}

	return
}

// extractDestructureBindings parses a pattern like "[[X Y] | Rest]" and returns
// the variable names for the element's fields (e.g., ["X", "Y"]).
func extractDestructureBindings(pattern string) []string {
	// Strip outer brackets and | Rest]
	inner := pattern
	inner = strings.TrimPrefix(inner, "[")
	pipeIdx := strings.Index(inner, "|")
	if pipeIdx == -1 {
		return nil
	}
	inner = strings.TrimSpace(inner[:pipeIdx])

	// Check for nested destructuring [[X Y]]
	if strings.HasPrefix(inner, "[") {
		inner = strings.TrimPrefix(inner, "[")
		inner = strings.TrimSuffix(inner, "]")
		return strings.Fields(inner)
	}

	// Simple element: [Med | Meds] → the element variable is "Med"
	fields := strings.Fields(inner)
	if len(fields) == 1 {
		return fields
	}
	return nil
}

// translateDefineGuard translates a where-clause s-expression to Go code.
func (st *SymbolTable) translateDefineGuard(guard string, localVarMap map[string]string) string {
	expr := parseSExpr(guard)
	if !expr.IsCall() {
		return "true"
	}
	return st.translateGuardExpr(expr, localVarMap)
}

func (st *SymbolTable) translateGuardExpr(expr *SExpr, varMap map[string]string) string {
	if !expr.IsCall() {
		if r, ok := st.resolveAtom(expr.Atom, varMap); ok {
			return r.GoCode
		}
		return expr.Atom
	}

	switch expr.Op() {
	case "and":
		if len(expr.Children) == 3 {
			l := st.translateGuardExpr(expr.Children[1], varMap)
			r := st.translateGuardExpr(expr.Children[2], varMap)
			return l + " && " + r
		}
	case "or":
		if len(expr.Children) == 3 {
			l := st.translateGuardExpr(expr.Children[1], varMap)
			r := st.translateGuardExpr(expr.Children[2], varMap)
			return l + " || " + r
		}
	case "not":
		if len(expr.Children) == 2 {
			inner := st.translateGuardExpr(expr.Children[1], varMap)
			return "!(" + inner + ")"
		}
	case "=":
		if len(expr.Children) == 3 {
			lhs, lok := st.resolveExpr(expr.Children[1], varMap)
			rhs, rok := st.resolveExpr(expr.Children[2], varMap)
			if lok && rok {
				goL := st.unwrap(lhs)
				goR := st.unwrap(rhs)
				return goL + " == " + goR
			}
		}
	default:
		// Check if it's a call to another defined function
		if _, ok := st.Defines[expr.Op()]; ok {
			goName := defineGoName(expr.Op())
			var args []string
			for i := 1; i < len(expr.Children); i++ {
				r, ok := st.resolveExpr(expr.Children[i], varMap)
				if ok {
					args = append(args, r.GoCode)
				} else {
					args = append(args, toCamelCase(expr.Children[i].Atom))
				}
			}
			// Resolve types for the called define if not yet done
			if _, resolved := st.DefineResolved[expr.Op()]; !resolved {
				st.resolveDefineTypes(expr, varMap)
			}
			return fmt.Sprintf("%s(%s)", goName, strings.Join(args, ", "))
		}
	}
	return "/* TODO: " + expr.String() + " */ true"
}

// resolveTransitiveDefines walks resolved defines and resolves any defines
// called from their guards, repeating until fixpoint.
func (st *SymbolTable) resolveTransitiveDefines() {
	changed := true
	for changed {
		changed = false
		for name, def := range st.Defines {
			resolved, ok := st.DefineResolved[name]
			if !ok {
				continue
			}
			loopParamIdx, _ := analyzeDefine(def)
			for _, clause := range def.Clauses {
				if clause.Guard == "" {
					continue
				}
				expr := parseSExpr(clause.Guard)
				calledName := findDefineCalls(expr)
				if calledName == "" {
					continue
				}
				if _, alreadyResolved := st.DefineResolved[calledName]; alreadyResolved {
					continue
				}
				// Build varMap for this clause using the resolved param types
				localVarMap := make(map[string]string)
				for i, p := range resolved.ParamTypes {
					if i == loopParamIdx {
						continue
					}
					if i < len(clause.Patterns) && clause.Patterns[i] != "_" && !strings.HasPrefix(clause.Patterns[i], "[") {
						localVarMap[clause.Patterns[i]] = p.ShenType
					}
				}
				// Add loop element binding
				if loopParamIdx >= 0 && loopParamIdx < len(resolved.ParamTypes) && loopParamIdx < len(clause.Patterns) {
					listParam := resolved.ParamTypes[loopParamIdx]
					bindings := extractDestructureBindings(clause.Patterns[loopParamIdx])
					if len(bindings) == 1 && bindings[0] != "_" {
						localVarMap[bindings[0]] = listParam.ElemShenType
					}
				}
				// Resolve the called define's types from this call context
				st.resolveDefineTypes(expr, localVarMap)
				changed = true
			}
		}
	}
}

func generateDefineHelpers(b *strings.Builder, st *SymbolTable) {
	// Resolve transitive dependencies before ordering
	st.resolveTransitiveDefines()

	// Fixpoint loop: generating one define's code may resolve types for
	// another define (via guard translation). Keep going until no new
	// defines are generated.
	generated := make(map[string]bool)

	for {
		// Order ungenerated, resolved defines by dependency
		var ordered []*Define
		emitted := make(map[string]bool)
		for k := range generated {
			emitted[k] = true
		}

		for pass := 0; pass < 3; pass++ {
			for name, def := range st.Defines {
				if emitted[name] {
					continue
				}
				if _, ok := st.DefineResolved[name]; !ok {
					continue
				}
				allDepsEmitted := true
				for _, clause := range def.Clauses {
					if clause.Guard != "" {
						expr := parseSExpr(clause.Guard)
						if calledName := findDefineCalls(expr); calledName != "" {
							if _, isDef := st.Defines[calledName]; isDef && !emitted[calledName] {
								allDepsEmitted = false
							}
						}
					}
				}
				if allDepsEmitted {
					ordered = append(ordered, def)
					emitted[name] = true
				}
			}
		}

		if len(ordered) == 0 {
			break
		}

		for _, def := range ordered {
			resolved := st.DefineResolved[def.Name]
			generateOneDefine(b, def, resolved, st)
			generated[def.Name] = true
		}
	}
}

func findDefineCalls(expr *SExpr) string {
	if !expr.IsCall() {
		return ""
	}
	if strings.HasSuffix(expr.Op(), "?") {
		return expr.Op()
	}
	for _, child := range expr.Children {
		if name := findDefineCalls(child); name != "" {
			return name
		}
	}
	return ""
}

func generateOneDefine(b *strings.Builder, def *Define, resolved *DefineResolved, st *SymbolTable) {
	loopParamIdx, baseResult := analyzeDefine(def)
	if loopParamIdx < 0 || loopParamIdx >= len(resolved.ParamTypes) {
		b.WriteString(fmt.Sprintf("// TODO: could not analyze define %s (no list iteration found)\n\n", def.Name))
		return
	}

	// Function signature
	var params []string
	for _, p := range resolved.ParamTypes {
		params = append(params, fmt.Sprintf("%s %s", p.GoName, p.GoType))
	}
	b.WriteString(fmt.Sprintf("// %s is generated from Shen define %s\n", resolved.GoName, def.Name))
	b.WriteString(fmt.Sprintf("func %s(%s) bool {\n", resolved.GoName, strings.Join(params, ", ")))

	// Loop over the list parameter
	listParam := resolved.ParamTypes[loopParamIdx]
	elemVar := "elem"
	b.WriteString(fmt.Sprintf("\tfor _, %s := range %s {\n", elemVar, listParam.GoName))

	// Generate if-blocks for guarded clauses
	for _, clause := range def.Clauses {
		if clause.Guard == "" {
			continue
		}
		if loopParamIdx >= len(clause.Patterns) {
			continue
		}

		// Build local variable map for this clause
		localVarMap := make(map[string]string)

		// Add non-list parameters
		for i, p := range resolved.ParamTypes {
			if i == loopParamIdx {
				continue
			}
			// Find the variable name used in this clause's pattern
			if i < len(clause.Patterns) && clause.Patterns[i] != "_" {
				localVarMap[clause.Patterns[i]] = p.ShenType
			}
		}

		// Add element destructuring bindings
		bindings := extractDestructureBindings(clause.Patterns[loopParamIdx])
		if len(bindings) > 1 && listParam.ElemShenType != "" {
			// Nested destructure [[X Y] | Rest] — X and Y are fields of the element type
			elemInfo := st.Lookup(listParam.ElemShenType)
			if elemInfo != nil && len(elemInfo.Fields) >= len(bindings) {
				for j, varName := range bindings {
					if varName == "_" {
						continue
					}
					field := elemInfo.Fields[j]
					// Map the variable to an accessor: elem.FieldName()
					// We create a synthetic type entry so resolveAtom can find it
					localVarMap[varName] = field.ShenType
				}
				// Generate field access variables
				for j, varName := range bindings {
					if varName == "_" {
						continue
					}
					field := elemInfo.Fields[j]
					accessor := elemVar + "." + toPascalCase(field.ShenName) + "()"
					// Override the camelCase mapping — we need elem.Field() not just varName
					// We do this by directly substituting in the varMap
					localVarMap[varName] = field.ShenType
					// We need to patch resolveAtom to emit the right code.
					// Simplest: just do string replacement after translation.
					_ = accessor
				}
			}
		} else if len(bindings) == 1 && bindings[0] != "_" {
			// Simple element [Med | Meds]
			localVarMap[bindings[0]] = listParam.ElemShenType
		}

		// Translate guard expression
		guardGo := st.translateDefineGuard(clause.Guard, localVarMap)

		// Post-process: replace camelCase variable references with actual element accessors
		if len(bindings) > 1 && listParam.ElemShenType != "" {
			elemInfo := st.Lookup(listParam.ElemShenType)
			if elemInfo != nil {
				for j, varName := range bindings {
					if varName == "_" || j >= len(elemInfo.Fields) {
						continue
					}
					field := elemInfo.Fields[j]
					accessor := elemVar + "." + toPascalCase(field.ShenName) + "()"
					if st.IsWrapper(field.ShenType) {
						// Replace varName.Val() with elem.Field().Val()
						guardGo = strings.ReplaceAll(guardGo, toCamelCase(varName)+".Val()", accessor+".Val()")
						// Also replace bare varName references
						guardGo = strings.ReplaceAll(guardGo, toCamelCase(varName), accessor)
					} else {
						guardGo = strings.ReplaceAll(guardGo, toCamelCase(varName), accessor)
					}
				}
			}
		} else if len(bindings) == 1 && bindings[0] != "_" {
			// Replace element variable reference with loop var
			guardGo = strings.ReplaceAll(guardGo, toCamelCase(bindings[0]), elemVar)
		}

		b.WriteString(fmt.Sprintf("\t\tif %s {\n\t\t\treturn %s\n\t\t}\n", guardGo, clause.Result))
	}

	b.WriteString("\t}\n")
	b.WriteString(fmt.Sprintf("\treturn %s\n", baseResult))
	b.WriteString("}\n\n")
}

// ============================================================================
// Parser
// ============================================================================

// extractBlocks finds all balanced-paren blocks starting with the given prefix.
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

func parseFile(path string) ([]Datatype, []Define, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	content := string(data)

	var types []Datatype
	for _, block := range extractBlocks(content, "(datatype ") {
		if dt := parseDatatype(block); dt != nil {
			types = append(types, *dt)
		}
	}

	var defines []Define
	for _, block := range extractBlocks(content, "(define ") {
		if def := parseDefine(block); def != nil {
			defines = append(defines, *def)
		}
	}

	return types, defines, nil
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

func parseDefine(block string) *Define {
	// Strip outer parens: (define name\n body)
	block = strings.TrimPrefix(block, "(define ")
	nlIdx := strings.Index(block, "\n")
	if nlIdx == -1 {
		return nil
	}
	name := strings.TrimSpace(block[:nlIdx])
	// Strip trailing close-paren that ends the (define ...) block
	body := strings.TrimRight(block[nlIdx:], " \t\n)")

	def := &Define{Name: name}

	// Join body into single line, then split on " -> " to get alternating segments.
	// segments[0] = first clause patterns
	// segments[i] = previous clause result [where guard] + next clause patterns
	// segments[last] = last clause result
	bodyOneLine := strings.Join(strings.Fields(body), " ")
	segments := strings.Split(bodyOneLine, " -> ")
	if len(segments) < 2 {
		return nil
	}

	// Process pairs: (segments[i], segments[i+1]) form a clause.
	// But segments[i+1] may contain both the result AND the next clause's patterns.
	// We carry forward the "remaining patterns" from each segment.
	currentPatterns := segments[0]

	for i := 1; i < len(segments); i++ {
		seg := segments[i]
		var result, guard, nextPatterns string

		// Check for 'where' keyword first
		whereIdx := strings.Index(seg, " where ")
		if whereIdx != -1 {
			result = strings.TrimSpace(seg[:whereIdx])
			afterWhere := strings.TrimSpace(seg[whereIdx+7:])
			// Guard is a balanced s-expression
			if strings.HasPrefix(afterWhere, "(") {
				guardExpr, endIdx := extractBalancedParen(afterWhere)
				guard = guardExpr
				nextPatterns = strings.TrimSpace(afterWhere[endIdx:])
			} else {
				guard = afterWhere
			}
		} else {
			// No where clause — split result from next patterns.
			// Result is one token (true/false) or a balanced paren expression.
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "(") {
				expr, endIdx := extractBalancedParen(seg)
				result = expr
				nextPatterns = strings.TrimSpace(seg[endIdx:])
			} else {
				tokens := strings.Fields(seg)
				result = tokens[0]
				if len(tokens) > 1 {
					nextPatterns = strings.Join(tokens[1:], " ")
				}
			}
		}

		// Clean up result
		result = strings.TrimRight(result, ")")
		result = strings.TrimSpace(result)
		if strings.HasPrefix(result, "(") {
			r, _ := extractBalancedParen(result + ")")
			if r != "" {
				result = r
			}
		}

		patterns := splitPatterns(currentPatterns)
		if len(patterns) > 0 {
			def.Clauses = append(def.Clauses, DefineClause{
				Patterns: patterns,
				Result:   result,
				Guard:    guard,
			})
		}

		currentPatterns = nextPatterns
	}

	if len(def.Clauses) == 0 {
		return nil
	}
	return def
}

// splitPatterns tokenizes a pattern string respecting bracket nesting.
// "[Med | Meds]" stays as one token; "[[X Y] | Rest]" stays as one token.
func splitPatterns(s string) []string {
	var patterns []string
	var current strings.Builder
	depth := 0
	for _, ch := range s {
		switch ch {
		case '[':
			depth++
			current.WriteRune(ch)
		case ']':
			depth--
			current.WriteRune(ch)
			if depth == 0 && current.Len() > 0 {
				patterns = append(patterns, current.String())
				current.Reset()
			}
		case ' ', '\t':
			if depth > 0 {
				current.WriteRune(ch)
			} else if current.Len() > 0 {
				patterns = append(patterns, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		patterns = append(patterns, current.String())
	}
	return patterns
}

// extractBalancedParen extracts a balanced parenthesized expression from s.
// Returns the expression and the index past its end.
func extractBalancedParen(s string) (string, int) {
	if len(s) == 0 || s[0] != '(' {
		return "", 0
	}
	depth := 0
	for i, ch := range s {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return s[:i+1], i + 1
			}
		}
	}
	return s, len(s)
}

// extractRuntimeViaComment looks for a trailing `\* :runtime-via <name> *\`
// Shen block-comment on the given line. If present, returns (name,
// line-without-comment, true). Otherwise returns ("", line, false).
//
// The annotation MUST be at the end of the line (after the `;`); embedding
// it anywhere else is unsupported. Shen treats `\* ... *\` as a comment so
// its tc+ ignores the annotation; shengen sees the marker and uses it.
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
	// Accept either `:runtime-via <name>` or `runtime-via <name>`.
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
	// A verified s-expression may span lines. Coalesce it before classifying
	// premises so a readable enum list is not silently discarded.
	var logicalPremLines []string
	for i := 0; i < len(premLines); i++ {
		line := strings.TrimSpace(premLines[i])
		if strings.HasPrefix(line, "(") && !strings.Contains(line, ": verified") {
			for i+1 < len(premLines) && !strings.Contains(line, ": verified") {
				i++
				line += " " + strings.TrimSpace(premLines[i])
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
		// Extract any trailing `\* :runtime-via <name> *\` Shen-comment
		// annotation. Shen treats `\* ... *\` as a block comment so its
		// tc+ ignores this; shengen sees the marker and uses it to
		// discharge the matching verified premise at runtime instead of
		// inlining the predicate. See docs/RUNTIME-VIA.md.
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
		// A Shen semantic sum normally uses a literal constructor tag followed
		// by its typed payload, e.g. [step-wait Duration].  Literal atoms are
		// not host fields: the concrete generated variant type already carries
		// that information.  Keep only conclusion names introduced by premises.
		premises := make(map[string]bool, len(r.Premises))
		for _, p := range r.Premises {
			premises[p.VarName] = true
		}
		for _, field := range strings.Fields(lhs[1 : len(lhs)-1]) {
			if premises[field] {
				r.Conc.Fields = append(r.Conc.Fields, field)
			}
		}
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
	if strings.HasPrefix(t, "(list ") {
		inner := strings.TrimSuffix(strings.TrimPrefix(t, "(list "), ")")
		return "[]" + shenTypeToGo(inner)
	}
	switch t {
	case "string", "symbol":
		return "string"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "":
		return "any"
	default:
		return toPascalCase(t)
	}
}

// listElemType extracts the element type from a "(list X)" type string.
func listElemType(t string) string {
	if strings.HasPrefix(t, "(list ") {
		return strings.TrimSuffix(strings.TrimPrefix(t, "(list "), ")")
	}
	return ""
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

// goKeywords lists Go reserved words that can't be used as identifiers.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

func toCamelCase(s string) string {
	pc := toPascalCase(s)
	if len(pc) == 0 {
		return pc
	}
	runes := []rune(pc)
	runes[0] = unicode.ToLower(runes[0])
	name := string(runes)
	if goKeywords[name] {
		name += "_"
	}
	return name
}

func isPrimitive(t string) bool {
	return t == "string" || t == "number" || t == "boolean" || t == "symbol"
}

func isNumericLiteral(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
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

func generateGo(types []Datatype, st *SymbolTable, pkg string, specPath string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("// Code generated by shengen from %s. DO NOT EDIT.\n", specPath))
	b.WriteString("//\n")
	b.WriteString("// These types enforce Shen sequent-calculus invariants at the Go level.\n")
	b.WriteString("// Constructors are the ONLY way to create these types — bypassing them\n")
	b.WriteString("// is a violation of the formal spec.\n\n")
	b.WriteString(fmt.Sprintf("package %s\n\n", pkg))

	// Only import fmt if there are constrained or guarded types that use fmt.Errorf
	needsFmt := false
	for _, dt := range types {
		for _, gt := range classify(dt, st) {
			if gt.Category == "constrained" || gt.Category == "guarded" {
				needsFmt = true
				break
			}
		}
		if needsFmt {
			break
		}
	}

	// Detect `:runtime-via` annotations across the spec.
	//   - Named checkers (profiles A/C/D) need `"context"`/`"errors"`, a
	//     `runtimeChecker` witness type, and one `var _` per checker.
	//   - `:eval` (profile B) needs `"context"`/`"errors"` and the
	//     evalhost import, but no witness type — the evaluator is the
	//     checker.
	runtimeCheckerNames := collectRuntimeCheckerNames(types)
	hasNamedChecker := len(runtimeCheckerNames) > 0
	hasEval := hasEvalRuntimeVia(types)
	hasRuntimeVia := hasNamedChecker || hasEval

	imports := []string{}
	if needsFmt {
		imports = append(imports, "\"fmt\"")
	}
	if hasRuntimeVia {
		imports = append(imports, "\"context\"", "\"errors\"")
	}
	if hasEval {
		imports = append(imports, fmt.Sprintf("%q", evalhostImportPath))
	}
	if len(imports) > 0 {
		b.WriteString("import (\n")
		for _, imp := range imports {
			b.WriteString("\t" + imp + "\n")
		}
		b.WriteString(")\n\n")
	}

	if hasNamedChecker {
		b.WriteString("// runtimeChecker is the contract every `:runtime-via <name>`\n")
		b.WriteString("// function must satisfy. The generated constructor passes the\n")
		b.WriteString("// predicate's spec-side name plus positional arguments; the\n")
		b.WriteString("// checker returns (ok, err). See docs/RUNTIME-VIA.md.\n")
		b.WriteString("type runtimeChecker func(ctx context.Context, predicate string, args ...any) (bool, error)\n\n")
		b.WriteString("// Compile-time witnesses. The build fails if any named runtime\n")
		b.WriteString("// checker is missing or has the wrong signature — this is what\n")
		b.WriteString("// makes the runtime call non-skippable: there is no path through\n")
		b.WriteString("// the constructor that does not consult the checker.\n")
		for _, name := range runtimeCheckerNames {
			b.WriteString(fmt.Sprintf("var _ runtimeChecker = %s\n", name))
		}
		b.WriteString("\n")
	}
	if hasRuntimeVia {
		b.WriteString("// errRuntimeCheckRejected is returned when a runtime check\n")
		b.WriteString("// reports the predicate is false (ok == false, err == nil).\n")
		b.WriteString("var errRuntimeCheckRejected = errors.New(\"runtime check rejected\")\n\n")
	}

	// Generate sum type interfaces.
	// Each sum type gets an interface with a private marker method.
	// Concrete variants implement this interface.
	sumTypeVariants := make(map[string]bool) // tracks which concrete types need marker methods
	for concType, variants := range st.SumTypes {
		goIface := toPascalCase(concType)
		markerMethod := "is" + goIface
		b.WriteString(fmt.Sprintf("// --- %s (sum type) ---\n", goIface))
		b.WriteString(fmt.Sprintf("// Multiple Shen datatype blocks produce this type.\n"))
		b.WriteString(fmt.Sprintf("// Variants: %s\n", strings.Join(variants, ", ")))
		b.WriteString(fmt.Sprintf("type %s interface {\n\t%s()\n}\n\n", goIface, markerMethod))
		for _, v := range variants {
			sumTypeVariants[v] = true
		}
	}

	for _, dt := range types {
		for _, gt := range classify(dt, st) {
			b.WriteString(fmt.Sprintf("// --- %s ---\n// Shen: (datatype %s)\n", gt.GoName, gt.Name))
			switch gt.Category {
			case "wrapper":
				generateWrapper(&b, gt)
			case "constrained":
				generateConstrained(&b, gt, st)
			case "composite":
				generateComposite(&b, gt, st)
			case "guarded":
				generateGuarded(&b, gt, st)
			case "alias":
				generateAlias(&b, gt)
			}
			// If this type is a variant of a sum type, emit marker method
			if sumTypeVariants[gt.Name] {
				// Find which sum type this belongs to
				for concType := range st.SumTypes {
					for _, v := range st.SumTypes[concType] {
						if v == gt.Name {
							goIface := toPascalCase(concType)
							b.WriteString(fmt.Sprintf("func (t %s) is%s() {}\n\n", gt.GoName, goIface))
						}
					}
				}
			}
			b.WriteString("\n")
		}
	}
	// Generate helper functions from (define ...) blocks AFTER types,
	// so that verified premises have had a chance to resolve define types.
	// Use fixpoint loop for transitive resolution (define A calls define B).
	generateDefineHelpers(&b, st)

	return b.String()
}

// runtimeViaSpec is the parsed form of a `:runtime-via` annotation.
// Grammar: `:runtime-via {<fname> | :eval} [:sampled] [:requires-db]`.
// The raw marker text is stored on VerifiedPremise.RuntimeVia; this
// splits it into its components. See docs/RUNTIME-VIA.md (profiles A–D).
type runtimeViaSpec struct {
	checker    string // bound checker fn (profiles A, C, D); "" when eval
	eval       bool   // :eval — evaluator-hosted (profile B)
	sampled    bool   // :sampled — shen-derive emits a sampled-equivalence test (profile C)
	requiresDB bool   // :requires-db — checker takes a DB handle (profile D)
}

// parseRuntimeVia splits a raw marker (everything after `:runtime-via `)
// into its components.
func parseRuntimeVia(raw string) runtimeViaSpec {
	fields := strings.Fields(raw)
	var s runtimeViaSpec
	if len(fields) == 0 {
		return s
	}
	if fields[0] == ":eval" {
		s.eval = true
	} else {
		s.checker = fields[0]
	}
	for _, t := range fields[1:] {
		switch t {
		case ":sampled":
			s.sampled = true
		case ":requires-db":
			s.requiresDB = true
		}
	}
	return s
}

// collectRuntimeCheckerNames returns the distinct named checkers
// referenced by `:runtime-via <name>` annotations (profiles A/C/D),
// sorted to keep emission deterministic. `:eval` premises (profile B)
// use the embedded evaluator and contribute no checker witness.
func collectRuntimeCheckerNames(types []Datatype) []string {
	seen := make(map[string]bool)
	var names []string
	for _, dt := range types {
		for _, r := range dt.Rules {
			for _, v := range r.Verified {
				if v.RuntimeVia == "" {
					continue
				}
				spec := parseRuntimeVia(v.RuntimeVia)
				if spec.checker == "" {
					continue // :eval — no witness
				}
				if seen[spec.checker] {
					continue
				}
				seen[spec.checker] = true
				names = append(names, spec.checker)
			}
		}
	}
	sortStrings(names)
	return names
}

// hasEvalRuntimeVia reports whether any verified premise in the spec
// uses `:runtime-via :eval` (profile B), which requires the evalhost
// import in the generated module.
func hasEvalRuntimeVia(types []Datatype) bool {
	for _, dt := range types {
		for _, r := range dt.Rules {
			for _, v := range r.Verified {
				if v.RuntimeVia != "" && parseRuntimeVia(v.RuntimeVia).eval {
					return true
				}
			}
		}
	}
	return false
}

// evalhostImportPath is the canonical import path of the embedded
// evaluator host that profile-B (`:runtime-via :eval`) constructors
// call. Projects using profile B must have the shen-derive module on
// their module graph (a require + replace in go.mod).
const evalhostImportPath = "github.com/pyrex41/Shen-Backpressure/shen-derive/runtime/evalhost"

// sortStrings is a tiny wrapper to avoid importing sort just for one call.
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j-1] > xs[j]; j-- {
			xs[j-1], xs[j] = xs[j], xs[j-1]
		}
	}
}

// ruleHasRuntimeVia reports whether any verified premise on r uses a
// `:runtime-via <name>` annotation.
func ruleHasRuntimeVia(r Rule) bool {
	for _, v := range r.Verified {
		if v.RuntimeVia != "" {
			return true
		}
	}
	return false
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

	hasRuntime := ruleHasRuntimeVia(gt.Rule)
	if hasRuntime {
		b.WriteString(fmt.Sprintf("func New%s(ctx context.Context, x %s) (%s, error) {\n", gt.GoName, goType, gt.GoName))
	} else {
		b.WriteString(fmt.Sprintf("func New%s(x %s) (%s, error) {\n", gt.GoName, goType, gt.GoName))
	}
	for _, v := range gt.Rule.Verified {
		if v.RuntimeVia != "" {
			emitRuntimeViaCheckGo(b, v, gt.Name, gt.GoName, []string{gt.Rule.Premises[0].VarName}, []string{"x"})
			continue
		}
		goExpr, errMsg := st.verifiedToGo(v, varMap)
		safeMsg := strings.ReplaceAll(errMsg, "%", "%%")
		safeMsg = strings.ReplaceAll(safeMsg, `"`, `\"`)
		b.WriteString(fmt.Sprintf("\tif %s {\n\t\treturn %s{}, fmt.Errorf(\"%s: %%v\", x)\n\t}\n", negateGoExpr(goExpr), gt.GoName, safeMsg))
	}
	b.WriteString(fmt.Sprintf("\treturn %s{v: x}, nil\n}\n\n", gt.GoName))
	b.WriteString(fmt.Sprintf("func (t %s) Val() %s { return t.v }\n\n", gt.GoName, goType))
}

// emitRuntimeViaCheckGo emits the gate-site for a `:runtime-via`
// verified premise.
//
// For a named checker (profiles A/C/D) the constructor calls the
// checker with the spec-side predicate name (the datatype block name)
// and the positional goArgs (camelCased variable names from the premise
// list).
//
// For `:eval` (profile B) the constructor calls the embedded evaluator
// host with the premise's raw s-expression and the Shen variable names
// bound positionally to goArgs — the spec predicate IS the runtime
// check, no separate checker.
//
// In both cases any error propagates verbatim and an `ok == false`
// outcome surfaces as errRuntimeCheckRejected wrapped with the
// predicate name, so the failure narrates which gate rejected what.
func emitRuntimeViaCheckGo(b *strings.Builder, v VerifiedPremise, predicateName string, zeroType string, shenVarNames []string, goArgs []string) {
	spec := parseRuntimeVia(v.RuntimeVia)

	if spec.eval {
		nameLits := make([]string, len(shenVarNames))
		for i, n := range shenVarNames {
			nameLits[i] = strconv.Quote(n)
		}
		namesArr := "[]string{" + strings.Join(nameLits, ", ") + "}"
		argsExpr := ""
		for _, a := range goArgs {
			argsExpr += ", " + a
		}
		b.WriteString(fmt.Sprintf("\tok, err := evalhost.Check(ctx, %q, %s%s)\n", v.Raw, namesArr, argsExpr))
		b.WriteString(fmt.Sprintf("\tif err != nil {\n\t\treturn %s{}, fmt.Errorf(\"evalhost rejected %s: %%w\", err)\n\t}\n", zeroType, predicateName))
		b.WriteString(fmt.Sprintf("\tif !ok {\n\t\treturn %s{}, fmt.Errorf(\"evalhost rejected %s: %%w\", errRuntimeCheckRejected)\n\t}\n", zeroType, predicateName))
		return
	}

	checker := spec.checker
	argsExpr := ""
	for _, a := range goArgs {
		argsExpr += ", " + a
	}
	b.WriteString(fmt.Sprintf("\tok, err := %s(ctx, %q%s)\n", checker, predicateName, argsExpr))
	b.WriteString(fmt.Sprintf("\tif err != nil {\n\t\treturn %s{}, fmt.Errorf(\"%s rejected %s: %%w\", err)\n\t}\n", zeroType, checker, predicateName))
	b.WriteString(fmt.Sprintf("\tif !ok {\n\t\treturn %s{}, fmt.Errorf(\"%s rejected %s: %%w\", errRuntimeCheckRejected)\n\t}\n", zeroType, checker, predicateName))
}

func generateComposite(b *strings.Builder, gt GeneratedType, st *SymbolTable) {
	b.WriteString(fmt.Sprintf("type %s struct {\n", gt.GoName))
	var params []string
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t%s %s\n", toCamelCase(p.VarName), shenTypeToGo(p.TypeName)))
		params = append(params, fmt.Sprintf("%s %s", toCamelCase(p.VarName), shenTypeToGo(p.TypeName)))
	}
	b.WriteString("}\n\n")
	b.WriteString(fmt.Sprintf("func New%s(%s) %s {\n\treturn %s{\n", gt.GoName, strings.Join(params, ", "), gt.GoName, gt.GoName))
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t\t%s: %s,\n", toCamelCase(p.VarName), toCamelCase(p.VarName)))
	}
	b.WriteString("\t}\n}\n\n")
	// Accessor methods for each field
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("func (t %s) %s() %s { return t.%s }\n\n",
			gt.GoName, toPascalCase(p.VarName), shenTypeToGo(p.TypeName), toCamelCase(p.VarName)))
	}
}

func generateGuarded(b *strings.Builder, gt GeneratedType, st *SymbolTable) {
	b.WriteString(fmt.Sprintf("type %s struct {\n", gt.GoName))
	var params []string
	var argNames []string
	var shenVarNames []string
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t%s %s\n", toCamelCase(p.VarName), shenTypeToGo(p.TypeName)))
		params = append(params, fmt.Sprintf("%s %s", toCamelCase(p.VarName), shenTypeToGo(p.TypeName)))
		argNames = append(argNames, toCamelCase(p.VarName))
		shenVarNames = append(shenVarNames, p.VarName)
	}
	b.WriteString("}\n\n")
	varMap := make(map[string]string)
	for _, p := range gt.Rule.Premises {
		varMap[p.VarName] = p.TypeName
	}
	hasRuntime := ruleHasRuntimeVia(gt.Rule)
	if hasRuntime {
		b.WriteString(fmt.Sprintf("func New%s(ctx context.Context, %s) (%s, error) {\n", gt.GoName, strings.Join(params, ", "), gt.GoName))
	} else {
		b.WriteString(fmt.Sprintf("func New%s(%s) (%s, error) {\n", gt.GoName, strings.Join(params, ", "), gt.GoName))
	}
	for _, v := range gt.Rule.Verified {
		if v.RuntimeVia != "" {
			emitRuntimeViaCheckGo(b, v, gt.Name, gt.GoName, shenVarNames, argNames)
			continue
		}
		goExpr, errMsg := st.verifiedToGo(v, varMap)
		safeMsg := strings.ReplaceAll(errMsg, "%", "%%")
		safeMsg = strings.ReplaceAll(safeMsg, `"`, `\"`)
		b.WriteString(fmt.Sprintf("\tif %s {\n\t\treturn %s{}, fmt.Errorf(\"%s\")\n\t}\n", negateGoExpr(goExpr), gt.GoName, safeMsg))
	}
	b.WriteString(fmt.Sprintf("\treturn %s{\n", gt.GoName))
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("\t\t%s: %s,\n", toCamelCase(p.VarName), toCamelCase(p.VarName)))
	}
	b.WriteString("\t}, nil\n}\n\n")
	// Accessor methods for each field
	for _, p := range gt.Rule.Premises {
		b.WriteString(fmt.Sprintf("func (t %s) %s() %s { return t.%s }\n\n",
			gt.GoName, toPascalCase(p.VarName), shenTypeToGo(p.TypeName), toCamelCase(p.VarName)))
	}
}

func generateAlias(b *strings.Builder, gt GeneratedType) {
	b.WriteString(fmt.Sprintf("type %s = %s\n\n", gt.GoName, shenTypeToGo(gt.Rule.Premises[0].TypeName)))
}

// ============================================================================
// Main
// ============================================================================

func printSymbolTable(types []Datatype, st *SymbolTable, path string) {
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
}

// ============================================================================
// Scoped DB Wrapper Generator
// ============================================================================

// generateDBWrappers emits proof-carrying DB wrapper types.
// For each GUARDED or COMPOSITE type that contains an ID field (a wrapper around string),
// it generates a scoped DB struct whose constructor requires the proof type and captures
// the verified ID at construction time.
func generateDBWrappers(types []Datatype, st *SymbolTable, pkg string, specPath string) string {
	var wrappers []struct {
		proofGoName string // e.g. "TenantAccess"
		idField     string // e.g. "tenant"
		idGoType    string // e.g. "TenantId"
		idAccessor  string // e.g. "Tenant"
	}

	for _, dt := range types {
		for _, gt := range classify(dt, st) {
			if gt.Category != "guarded" && gt.Category != "composite" {
				continue
			}
			// Look for fields whose type is a string wrapper (an ID type)
			for _, p := range gt.Rule.Premises {
				fieldInfo := st.Lookup(p.TypeName)
				if fieldInfo == nil || fieldInfo.Category != "wrapper" || fieldInfo.WrappedPrim != "string" {
					continue
				}
				wrappers = append(wrappers, struct {
					proofGoName string
					idField     string
					idGoType    string
					idAccessor  string
				}{
					proofGoName: gt.GoName,
					idField:     toCamelCase(p.VarName),
					idGoType:    toPascalCase(p.TypeName),
					idAccessor:  toPascalCase(p.VarName),
				})
			}
		}
	}

	if len(wrappers) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("// Code generated by shengen from %s. DO NOT EDIT.\n", specPath))
	b.WriteString("//\n")
	b.WriteString("// Scoped DB wrappers — proof-carrying database access.\n")
	b.WriteString("// Each wrapper captures a verified ID from a guard type proof,\n")
	b.WriteString("// ensuring all queries are automatically scoped.\n\n")
	b.WriteString(fmt.Sprintf("package %s\n\n", pkg))
	b.WriteString("import \"database/sql\"\n\n")

	for _, w := range wrappers {
		wrapperName := w.proofGoName + "DB"
		b.WriteString(fmt.Sprintf("// %s provides %s-scoped database access.\n", wrapperName, w.idGoType))
		b.WriteString(fmt.Sprintf("// Constructed from a %s proof — the %s is captured and cannot be changed.\n", w.proofGoName, w.idField))
		b.WriteString(fmt.Sprintf("type %s struct {\n", wrapperName))
		b.WriteString(fmt.Sprintf("\tDB      *sql.DB\n"))
		b.WriteString(fmt.Sprintf("\t%s string\n", w.idField))
		b.WriteString("}\n\n")
		b.WriteString(fmt.Sprintf("// New%s creates a scoped DB wrapper from a %s proof.\n", wrapperName, w.proofGoName))
		b.WriteString(fmt.Sprintf("func New%s(db *sql.DB, proof %s) %s {\n", wrapperName, w.proofGoName, wrapperName))
		b.WriteString(fmt.Sprintf("\treturn %s{DB: db, %s: proof.%s().Val()}\n",
			wrapperName, w.idField, w.idAccessor))
		b.WriteString("}\n\n")
		b.WriteString(fmt.Sprintf("// ScopedID returns the verified %s captured at construction time.\n", w.idField))
		b.WriteString(fmt.Sprintf("func (d %s) ScopedID() string { return d.%s }\n\n", wrapperName, w.idField))
	}

	return b.String()
}

func main() {
	outFile := flag.String("out", "", "Output file path (default: stdout)")
	specFile := flag.String("spec", "", "Spec file path (alternative to positional arg)")
	pkgName := flag.String("pkg", "", "Go package name (alternative to positional arg)")
	dryRun := flag.Bool("dry-run", false, "Parse and show symbol table only, don't generate code")
	dbWrappers := flag.String("db-wrappers", "", "Generate scoped DB wrappers to this file")
	flag.Parse()

	path := "specs/core.shen"
	if *specFile != "" {
		path = *specFile
	} else if flag.NArg() > 0 {
		path = flag.Arg(0)
	}
	pkg := "shenguard"
	if *pkgName != "" {
		pkg = *pkgName
	} else if flag.NArg() > 1 {
		pkg = flag.Arg(1)
	}

	types, defines, err := parseFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	st := newSymbolTable()
	st.Build(types)

	// Register defines in symbol table
	for i := range defines {
		st.Defines[defines[i].Name] = &defines[i]
	}

	printSymbolTable(types, st, path)

	if len(defines) > 0 {
		fmt.Fprintf(os.Stderr, "Defined functions:\n")
		for _, def := range defines {
			fmt.Fprintf(os.Stderr, "  %-28s [%d clauses]\n", def.Name, len(def.Clauses))
		}
		fmt.Fprintln(os.Stderr)
	}

	if *dryRun {
		return
	}

	output := generateGo(types, st, pkg, path)

	if *outFile != "" {
		if err := os.WriteFile(*outFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", *outFile, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Generated %s from %s (package %s)\n", *outFile, path, pkg)
	} else {
		fmt.Print(output)
	}

	// Generate scoped DB wrappers if requested
	if *dbWrappers != "" {
		dbOutput := generateDBWrappers(types, st, pkg, path)
		if dbOutput != "" {
			if err := os.WriteFile(*dbWrappers, []byte(dbOutput), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", *dbWrappers, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Generated %s (scoped DB wrappers)\n", *dbWrappers)
		}
	}
}
