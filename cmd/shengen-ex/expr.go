package main

// expr.go — lowering Shen expressions to Elixir.
//
// Two translators live here:
//
//  1. defineGen lowers (define ...) blocks with *Shen data semantics*:
//     arguments are first converted to their Shen representation
//     (<NS>.Term.to_shen/1: wrappers become their primitive, composites
//     become lists in conclusion order), after which the Shen body maps
//     onto ordinary Elixir terms (lists, binaries, numbers, atoms).
//     Clause order, repeated pattern variables (non-linear patterns) and
//     `where` guards keep Shen's first-match semantics.
//
//  2. premiseGen lowers `(...) : verified` premises inside a guard
//     constructor. It resolves head/tail chains over guard types
//     statically to accessor calls (like the Go emitter does), unwraps
//     wrappers when they meet primitives, and falls back to the Shen
//     representation for anything it cannot type.
//
// Anything outside the supported fragment is an explicit error — never a
// silent `true`.

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func isVarAtom(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)[0]
	return unicode.IsUpper(r)
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func listElemType(t string) string {
	t = strings.TrimSpace(t)
	if strings.HasPrefix(t, "(list ") && strings.HasSuffix(t, ")") {
		return strings.TrimSpace(t[len("(list ") : len(t)-1])
	}
	return ""
}

// literal renders an atom node that is not a variable.
func literal(n *Node) (string, bool) {
	switch n.Kind {
	case NStr:
		return elixirLiteralString(n.Atom), true
	case NAtom:
		switch {
		case n.Atom == "true" || n.Atom == "false":
			return n.Atom, true
		case isNumber(n.Atom):
			return normalizeNumber(n.Atom), true
		case isVarAtom(n.Atom) || n.Atom == "_":
			return "", false
		default:
			return elixirAtom(n.Atom), true
		}
	case NCons:
		if len(n.Items) == 0 && n.Tail == nil {
			return "[]", true
		}
	}
	return "", false
}

func normalizeNumber(s string) string {
	s = strings.TrimPrefix(s, "+")
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	}
	if strings.HasPrefix(s, "-.") {
		s = "-0" + s[1:]
	}
	if strings.HasSuffix(s, ".") {
		s += "0"
	}
	return s
}

// ============================================================================
// 1. Define lowering (Shen data semantics)
// ============================================================================

type defineGen struct {
	st   *SymbolTable
	defs map[string]*Define
}

func implName(shen string) string {
	base := strings.TrimRight(fnName(shen), "?!")
	return "shen__" + base
}

// dexpr lowers a Shen expression evaluated over Shen-shaped data.
func (g *defineGen) dexpr(n *Node, scope map[string]string) (string, error) {
	switch n.Kind {
	case NStr:
		return elixirLiteralString(n.Atom), nil
	case NAtom:
		if v, ok := scope[n.Atom]; ok {
			return v, nil
		}
		if isVarAtom(n.Atom) {
			return "", fmt.Errorf("unbound variable %s", n.Atom)
		}
		if lit, ok := literal(n); ok {
			return lit, nil
		}
		return "", fmt.Errorf("unsupported atom %q", n.Atom)
	case NCons:
		var parts []string
		for _, it := range n.Items {
			c, err := g.dexpr(it, scope)
			if err != nil {
				return "", err
			}
			parts = append(parts, c)
		}
		s := strings.Join(parts, ", ")
		if n.Tail != nil {
			t, err := g.dexpr(n.Tail, scope)
			if err != nil {
				return "", err
			}
			if len(parts) == 0 {
				return t, nil
			}
			return "[" + s + " | " + t + "]", nil
		}
		return "[" + s + "]", nil
	case NList:
		return g.dcall(n, scope)
	}
	return "", fmt.Errorf("unsupported expression %s", n.String())
}

func (g *defineGen) dargs(n *Node, scope map[string]string) ([]string, error) {
	var out []string
	for _, a := range n.Args() {
		c, err := g.dexpr(a, scope)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (g *defineGen) dcall(n *Node, scope map[string]string) (string, error) {
	op := n.Op()
	if op == "" {
		return "", fmt.Errorf("unsupported application %s", n.String())
	}
	if op == "let" {
		// (let X V Body) / (let X V Y W Body)
		args := n.Args()
		if len(args) < 3 || len(args)%2 == 0 {
			return "", fmt.Errorf("malformed let %s", n.String())
		}
		inner := copyScope(scope)
		var stmts []string
		for i := 0; i+1 < len(args)-1; i += 2 {
			if args[i].Kind != NAtom || !isVarAtom(args[i].Atom) {
				return "", fmt.Errorf("let binds non-variable in %s", n.String())
			}
			v, err := g.dexpr(args[i+1], inner)
			if err != nil {
				return "", err
			}
			name := toSnake(args[i].Atom) + "_" + strconv.Itoa(len(inner))
			inner[args[i].Atom] = name
			stmts = append(stmts, name+" = "+v)
		}
		body, err := g.dexpr(args[len(args)-1], inner)
		if err != nil {
			return "", err
		}
		return "(" + strings.Join(stmts, "; ") + "; " + body + ")", nil
	}
	a, err := g.dargs(n, scope)
	if err != nil {
		return "", err
	}
	arity := func(k int) error {
		if len(a) != k {
			return fmt.Errorf("%s expects %d argument(s) in %s", op, k, n.String())
		}
		return nil
	}
	switch op {
	case "=":
		if err := arity(2); err != nil {
			return "", err
		}
		return "(" + a[0] + " == " + a[1] + ")", nil
	case ">", "<", ">=", "<=":
		if err := arity(2); err != nil {
			return "", err
		}
		return "(" + a[0] + " " + op + " " + a[1] + ")", nil
	case "+", "-", "*", "/":
		if err := arity(2); err != nil {
			return "", err
		}
		return "(" + a[0] + " " + op + " " + a[1] + ")", nil
	case "not":
		if err := arity(1); err != nil {
			return "", err
		}
		return "not " + a[0], nil
	case "and", "or":
		if len(a) < 2 {
			return "", fmt.Errorf("%s needs at least 2 arguments", op)
		}
		return "(" + strings.Join(a, " "+op+" ") + ")", nil
	case "if":
		if err := arity(3); err != nil {
			return "", err
		}
		return "if(" + a[0] + ", do: " + a[1] + ", else: " + a[2] + ")", nil
	case "head", "hd":
		if err := arity(1); err != nil {
			return "", err
		}
		return "hd(" + a[0] + ")", nil
	case "tail", "tl":
		if err := arity(1); err != nil {
			return "", err
		}
		return "tl(" + a[0] + ")", nil
	case "cons":
		if err := arity(2); err != nil {
			return "", err
		}
		return "[" + a[0] + " | " + a[1] + "]", nil
	case "element?":
		if err := arity(2); err != nil {
			return "", err
		}
		return "Enum.member?(" + a[1] + ", " + a[0] + ")", nil
	case "length":
		if err := arity(1); err != nil {
			return "", err
		}
		return "length(" + a[0] + ")", nil
	case "empty?":
		if err := arity(1); err != nil {
			return "", err
		}
		return "(" + a[0] + " == [])", nil
	case "cons?":
		if err := arity(1); err != nil {
			return "", err
		}
		return "match?([_ | _], " + a[0] + ")", nil
	case "append":
		if err := arity(2); err != nil {
			return "", err
		}
		return "(" + a[0] + " ++ " + a[1] + ")", nil
	case "reverse":
		if err := arity(1); err != nil {
			return "", err
		}
		return "Enum.reverse(" + a[0] + ")", nil
	case "cn":
		if err := arity(2); err != nil {
			return "", err
		}
		return "(" + a[0] + " <> " + a[1] + ")", nil
	case "nth":
		if err := arity(2); err != nil {
			return "", err
		}
		return "Enum.at(" + a[1] + ", " + a[0] + " - 1)", nil
	case "shen.mod":
		if err := arity(2); err != nil {
			return "", err
		}
		return "rem(" + a[0] + ", " + a[1] + ")", nil
	case "string?":
		return "is_binary(" + a[0] + ")", arity(1)
	case "number?":
		return "is_number(" + a[0] + ")", arity(1)
	case "integer?":
		return "is_integer(" + a[0] + ")", arity(1)
	case "boolean?":
		return "is_boolean(" + a[0] + ")", arity(1)
	case "symbol?":
		return "(is_atom(" + a[0] + ") and not is_boolean(" + a[0] + "))", arity(1)
	}
	if d, ok := g.defs[op]; ok {
		if len(d.Clauses) > 0 && len(d.Clauses[0].Patterns) != len(a) {
			return "", fmt.Errorf("%s called with %d argument(s), defined with %d (partial application is not supported)", op, len(a), len(d.Clauses[0].Patterns))
		}
		return implName(op) + "(" + strings.Join(a, ", ") + ")", nil
	}
	return "", fmt.Errorf("unsupported function %q in %s", op, n.String())
}

func copyScope(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// collectVars records every variable atom referenced in n.
func collectVars(n *Node, into map[string]int) {
	if n == nil {
		return
	}
	switch n.Kind {
	case NAtom:
		if isVarAtom(n.Atom) {
			into[n.Atom]++
		}
	case NList, NCons, NBrace:
		for _, it := range n.Items {
			collectVars(it, into)
		}
		collectVars(n.Tail, into)
	}
}

// dpattern lowers a Shen pattern to an Elixir match pattern. used says
// which variables the body/guard references; unused ones are prefixed
// with `_` unless repeated (a repeated variable is an equality test).
func (g *defineGen) dpattern(n *Node, scope map[string]string, used, patCount map[string]int) (string, error) {
	switch n.Kind {
	case NStr:
		return elixirLiteralString(n.Atom), nil
	case NAtom:
		if n.Atom == "_" {
			return "_", nil
		}
		if isVarAtom(n.Atom) {
			name := toSnake(n.Atom)
			if used[n.Atom] == 0 && patCount[n.Atom] <= 1 {
				return "_" + name, nil
			}
			scope[n.Atom] = name
			return name, nil
		}
		lit, ok := literal(n)
		if !ok {
			return "", fmt.Errorf("unsupported pattern atom %q", n.Atom)
		}
		return lit, nil
	case NCons:
		var parts []string
		for _, it := range n.Items {
			p, err := g.dpattern(it, scope, used, patCount)
			if err != nil {
				return "", err
			}
			parts = append(parts, p)
		}
		s := strings.Join(parts, ", ")
		if n.Tail != nil {
			t, err := g.dpattern(n.Tail, scope, used, patCount)
			if err != nil {
				return "", err
			}
			return "[" + s + " | " + t + "]", nil
		}
		return "[" + s + "]", nil
	}
	return "", fmt.Errorf("unsupported pattern %s (only variables, literals and [..|..] lists)", n.String())
}

// irrefutable: every pattern is `_` or a variable used exactly once.
func irrefutable(pats []*Node) bool {
	seen := map[string]bool{}
	for _, p := range pats {
		if p.Kind != NAtom {
			return false
		}
		if p.Atom == "_" {
			continue
		}
		if !isVarAtom(p.Atom) || seen[p.Atom] {
			return false
		}
		seen[p.Atom] = true
	}
	return true
}

// lowerDefine emits the Elixir implementation of one define.
func (g *defineGen) lowerDefine(d *Define) (string, error) {
	if len(d.Clauses) == 0 {
		return "", fmt.Errorf("define %s has no clauses", d.Name)
	}
	arity := len(d.Clauses[0].Patterns)
	for _, c := range d.Clauses {
		if len(c.Patterns) != arity {
			return "", fmt.Errorf("define %s: clauses disagree on arity", d.Name)
		}
	}
	impl := implName(d.Name)
	hasGuard := false
	for _, c := range d.Clauses {
		if c.Guard != nil {
			hasGuard = true
		}
	}
	var b strings.Builder
	argNames := make([]string, arity)
	for i := range argNames {
		argNames[i] = "a__" + strconv.Itoa(i+1)
	}
	if hasGuard {
		fmt.Fprintf(&b, "  defp %s(%s), do: %s__c1(%s)\n", impl, strings.Join(argNames, ", "), impl, strings.Join(argNames, ", "))
	}
	reachedIrrefutable := false
	for ci, c := range d.Clauses {
		if reachedIrrefutable {
			break
		}
		used := map[string]int{}
		collectVars(c.Result, used)
		collectVars(c.Guard, used)
		patCount := map[string]int{}
		for _, p := range c.Patterns {
			collectVars(p, patCount)
		}
		scope := map[string]string{}
		var pats []string
		for _, p := range c.Patterns {
			ps, err := g.dpattern(p, scope, used, patCount)
			if err != nil {
				return "", fmt.Errorf("define %s clause %d: %w", d.Name, ci+1, err)
			}
			pats = append(pats, ps)
		}
		body, err := g.dexpr(c.Result, scope)
		if err != nil {
			return "", fmt.Errorf("define %s clause %d: %w", d.Name, ci+1, err)
		}
		fname := impl
		next := impl
		if hasGuard {
			fname = impl + "__c" + strconv.Itoa(ci+1)
			next = impl + "__c" + strconv.Itoa(ci+2)
		}
		refutable := !irrefutable(c.Patterns)
		if c.Guard == nil {
			fmt.Fprintf(&b, "  defp %s(%s), do: %s\n", fname, strings.Join(pats, ", "), body)
			if !refutable {
				reachedIrrefutable = true
			} else if hasGuard {
				fmt.Fprintf(&b, "  defp %s(%s), do: %s(%s)\n", fname, strings.Join(argNames, ", "), next, strings.Join(argNames, ", "))
			}
			continue
		}
		guard, err := g.dexpr(c.Guard, scope)
		if err != nil {
			return "", fmt.Errorf("define %s clause %d guard: %w", d.Name, ci+1, err)
		}
		bound := make([]string, arity)
		for i := range pats {
			if pats[i] == "_" || strings.HasPrefix(pats[i], "_") && !strings.ContainsAny(pats[i], "[]\"") {
				bound[i] = argNames[i]
			} else {
				bound[i] = pats[i] + " = " + argNames[i]
			}
		}
		fmt.Fprintf(&b, "  defp %s(%s) do\n    if %s, do: %s, else: %s(%s)\n  end\n",
			fname, strings.Join(bound, ", "), guard, body, next, strings.Join(argNames, ", "))
		if refutable {
			fmt.Fprintf(&b, "  defp %s(%s), do: %s(%s)\n", fname, strings.Join(argNames, ", "), next, strings.Join(argNames, ", "))
		}
	}
	if !reachedIrrefutable {
		last := impl
		if hasGuard {
			last = impl + "__c" + strconv.Itoa(len(d.Clauses)+1)
		}
		us := make([]string, arity)
		for i := range us {
			us[i] = "_"
		}
		fmt.Fprintf(&b, "  defp %s(%s), do: raise(ArgumentError, %s)\n", last, strings.Join(us, ", "),
			elixirLiteralString("partial function "+d.Name))
	}
	return b.String(), nil
}

// definesUsed returns the defines reachable from roots, in spec order.
func definesUsed(roots map[string]bool, defs []Define) []string {
	byName := map[string]*Define{}
	for i := range defs {
		byName[defs[i].Name] = &defs[i]
	}
	seen := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		d, ok := byName[name]
		if !ok {
			return
		}
		seen[name] = true
		for _, c := range d.Clauses {
			for _, callee := range calledSymbols(c.Result) {
				walk(callee)
			}
			for _, callee := range calledSymbols(c.Guard) {
				walk(callee)
			}
		}
	}
	for r := range roots {
		walk(r)
	}
	var out []string
	for _, d := range defs {
		if seen[d.Name] {
			out = append(out, d.Name)
		}
	}
	return out
}

func calledSymbols(n *Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	switch n.Kind {
	case NList:
		if op := n.Op(); op != "" {
			out = append(out, op)
		}
		for _, it := range n.Items {
			out = append(out, calledSymbols(it)...)
		}
	case NCons:
		for _, it := range n.Items {
			out = append(out, calledSymbols(it)...)
		}
		out = append(out, calledSymbols(n.Tail)...)
	}
	return out
}

// ============================================================================
// 2. Premise lowering (typed, over guard structs)
// ============================================================================

// rexpr is a lowered premise sub-expression.
type rexpr struct {
	code     string
	shenType string // Shen type when known, "" otherwise
	shaped   bool   // code already evaluates to the Shen representation
	view     []FieldInfo
	viewBase string // struct code the view's fields are read from
	viewMod  string
}

type premiseGen struct {
	st  *SymbolTable
	env map[string]rexpr // Shen var -> lowered var
}

func (p *premiseGen) termMod() string { return p.st.Namespace + ".Term" }

// shapedCode renders r as its Shen representation.
func (p *premiseGen) shapedCode(r rexpr) string {
	if r.view != nil {
		parts := make([]string, len(r.view))
		for i, f := range r.view {
			parts[i] = p.shapedCode(p.fieldAccess(r.viewBase, r.viewMod, f))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	if r.shaped || isPrimitive(r.shenType) {
		return r.code
	}
	if p.st.IsWrapper(r.shenType) && isPrimitive(p.st.Lookup(r.shenType).WrappedPrim) {
		return p.st.moduleFor(r.shenType) + ".val(" + r.code + ")"
	}
	if elem := listElemType(r.shenType); elem != "" && isPrimitive(elem) {
		return r.code
	}
	return p.termMod() + ".to_shen(" + r.code + ")"
}

func (p *premiseGen) fieldAccess(base, mod string, f FieldInfo) rexpr {
	t := f.ShenType
	if t == "unknown" {
		t = ""
	}
	return rexpr{code: mod + "." + toSnake(f.ShenName) + "(" + base + ")", shenType: t}
}

func (p *premiseGen) resolve(n *Node) (rexpr, error) {
	switch n.Kind {
	case NStr:
		return rexpr{code: elixirLiteralString(n.Atom), shenType: "string", shaped: true}, nil
	case NAtom:
		if v, ok := p.env[n.Atom]; ok {
			return v, nil
		}
		if isVarAtom(n.Atom) {
			return rexpr{}, fmt.Errorf("variable %s is not bound by the rule's premises", n.Atom)
		}
		switch {
		case n.Atom == "true" || n.Atom == "false":
			return rexpr{code: n.Atom, shenType: "boolean", shaped: true}, nil
		case isNumber(n.Atom):
			return rexpr{code: normalizeNumber(n.Atom), shenType: "number", shaped: true}, nil
		}
		return rexpr{code: elixirAtom(n.Atom), shenType: "symbol", shaped: true}, nil
	case NCons:
		var parts []string
		for _, it := range n.Items {
			r, err := p.resolve(it)
			if err != nil {
				return rexpr{}, err
			}
			parts = append(parts, p.shapedCode(r))
		}
		code := "[" + strings.Join(parts, ", ") + "]"
		if n.Tail != nil {
			t, err := p.resolve(n.Tail)
			if err != nil {
				return rexpr{}, err
			}
			code = "[" + strings.Join(parts, ", ") + " | " + p.shapedCode(t) + "]"
		}
		return rexpr{code: code, shaped: true}, nil
	case NList:
		return p.call(n)
	}
	return rexpr{}, fmt.Errorf("unsupported premise expression %s", n.String())
}

func (p *premiseGen) headTail(n *Node, isHead bool) (rexpr, error) {
	args := n.Args()
	if len(args) != 1 {
		return rexpr{}, fmt.Errorf("%s expects 1 argument", n.Op())
	}
	inner, err := p.resolve(args[0])
	if err != nil {
		return rexpr{}, err
	}
	var fields []FieldInfo
	base, mod := inner.code, ""
	if inner.view != nil {
		fields, base, mod = inner.view, inner.viewBase, inner.viewMod
	} else if !inner.shaped {
		if info := p.st.Lookup(inner.shenType); info != nil && len(info.Fields) > 0 && !info.Rule.Conc.IsWrapped &&
			len(info.Rule.Conc.Items) == len(info.Fields) {
			fields, mod = info.Fields, p.st.moduleFor(inner.shenType)
		}
	}
	if fields != nil {
		if isHead {
			return p.fieldAccess(base, mod, fields[0]), nil
		}
		return rexpr{view: fields[1:], viewBase: base, viewMod: mod}, nil
	}
	if elem := listElemType(inner.shenType); elem != "" && !inner.shaped {
		if isHead {
			return rexpr{code: "hd(" + inner.code + ")", shenType: elem}, nil
		}
		return rexpr{code: "tl(" + inner.code + ")", shenType: inner.shenType}, nil
	}
	op := "tl"
	if isHead {
		op = "hd"
	}
	return rexpr{code: op + "(" + p.shapedCode(inner) + ")", shaped: true}, nil
}

// eqOperands renders both sides of an equality so the comparison has
// Shen semantics: identical guard types compare structurally, anything
// else compares in the Shen representation.
func (p *premiseGen) eqOperands(l, r rexpr) (string, string) {
	if l.view == nil && r.view == nil && !l.shaped && !r.shaped && l.shenType != "" && l.shenType == r.shenType {
		return l.code, r.code
	}
	return p.shapedCode(l), p.shapedCode(r)
}

func (p *premiseGen) call(n *Node) (rexpr, error) {
	op := n.Op()
	switch op {
	case "head":
		return p.headTail(n, true)
	case "tail":
		return p.headTail(n, false)
	}
	var rs []rexpr
	for _, a := range n.Args() {
		r, err := p.resolve(a)
		if err != nil {
			return rexpr{}, err
		}
		rs = append(rs, r)
	}
	need := func(k int) error {
		if len(rs) != k {
			return fmt.Errorf("%s expects %d argument(s) in %s", op, k, n.String())
		}
		return nil
	}
	boolean := func(code string) (rexpr, error) {
		return rexpr{code: code, shenType: "boolean", shaped: true}, nil
	}
	switch op {
	case "=":
		if err := need(2); err != nil {
			return rexpr{}, err
		}
		l, r := p.eqOperands(rs[0], rs[1])
		return boolean(l + " == " + r)
	case ">", "<", ">=", "<=":
		if err := need(2); err != nil {
			return rexpr{}, err
		}
		return boolean(p.shapedCode(rs[0]) + " " + op + " " + p.shapedCode(rs[1]))
	case "+", "-", "*", "/":
		if err := need(2); err != nil {
			return rexpr{}, err
		}
		return rexpr{code: "(" + p.shapedCode(rs[0]) + " " + op + " " + p.shapedCode(rs[1]) + ")", shenType: "number", shaped: true}, nil
	case "not":
		if err := need(1); err != nil {
			return rexpr{}, err
		}
		return boolean("not (" + p.shapedCode(rs[0]) + ")")
	case "and", "or":
		if len(rs) < 2 {
			return rexpr{}, fmt.Errorf("%s needs at least 2 arguments", op)
		}
		parts := make([]string, len(rs))
		for i, r := range rs {
			parts[i] = "(" + p.shapedCode(r) + ")"
		}
		return boolean(strings.Join(parts, " "+op+" "))
	case "element?":
		if err := need(2); err != nil {
			return rexpr{}, err
		}
		if n.Args()[1].Kind == NCons && n.Args()[1].Tail == nil {
			return boolean(p.shapedCode(rs[0]) + " in " + rs[1].code)
		}
		return boolean("Enum.member?(" + p.shapedCode(rs[1]) + ", " + p.shapedCode(rs[0]) + ")")
	case "length":
		if err := need(1); err != nil {
			return rexpr{}, err
		}
		return rexpr{code: "length(" + p.shapedCode(rs[0]) + ")", shenType: "number", shaped: true}, nil
	case "shen.mod":
		if err := need(2); err != nil {
			return rexpr{}, err
		}
		return rexpr{code: "rem(" + p.shapedCode(rs[0]) + ", " + p.shapedCode(rs[1]) + ")", shenType: "number", shaped: true}, nil
	case "empty?":
		if err := need(1); err != nil {
			return rexpr{}, err
		}
		return boolean(p.shapedCode(rs[0]) + " == []")
	}
	if d, ok := p.st.Defines[op]; ok {
		args := make([]string, len(rs))
		for i, r := range rs {
			if r.view != nil {
				args[i] = p.shapedCode(r)
			} else {
				args[i] = r.code
			}
		}
		return rexpr{code: p.st.Namespace + ".Defines." + fnName(op) + "(" + strings.Join(args, ", ") + ")",
			shenType: d.RetType, shaped: true}, nil
	}
	return rexpr{}, fmt.Errorf("unsupported function %q in premise %s", op, n.String())
}

// lowerPremise returns the Elixir boolean for a verified premise, plus the
// premise variables it references (for the error payload), plus the
// defines it calls.
func (p *premiseGen) lowerPremise(raw string) (code string, vars []string, calls []string, err error) {
	n, err := readShen(raw)
	if err != nil {
		return "", nil, nil, err
	}
	r, err := p.resolve(n)
	if err != nil {
		return "", nil, nil, err
	}
	seen := map[string]bool{}
	var walk func(*Node)
	walk = func(x *Node) {
		if x == nil {
			return
		}
		switch x.Kind {
		case NAtom:
			if _, ok := p.env[x.Atom]; ok && !seen[x.Atom] {
				seen[x.Atom] = true
				vars = append(vars, x.Atom)
			}
		case NList, NCons:
			for _, it := range x.Items {
				walk(it)
			}
			walk(x.Tail)
		}
	}
	walk(n)
	for _, c := range calledSymbols(n) {
		if _, ok := p.st.Defines[c]; ok {
			calls = append(calls, c)
		}
	}
	return p.shapedCode(r), vars, calls, nil
}

// negate renders the violation condition for a premise.
func negate(code string) string {
	if strings.HasPrefix(code, "not (") && strings.HasSuffix(code, ")") {
		inner := code[len("not (") : len(code)-1]
		depth := 0
		ok := true
		for _, c := range inner {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth < 0 {
					ok = false
					break
				}
			}
		}
		if ok && depth == 0 {
			return inner
		}
	}
	return "not (" + code + ")"
}
