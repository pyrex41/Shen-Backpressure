// Package symbolic is a symbolic evaluator over core.Sexpr, sibling to
// core.Eval. Where core.Eval maps concrete arguments to a concrete value,
// symbolic evaluation maps *symbolic* arguments to a set of execution
// paths, each carrying a path condition — a conjunction of constraints
// that an input must satisfy to take that path.
//
// The constraint language is deliberately small: linear real arithmetic,
// booleans, and strings with length. That is exactly what the (define …)
// subset shen-derive accepts can produce, and all of it is in Z3's core.
// Path conditions are printed to SMT-LIB2 (see smtlib.go) and handed to
// a solver over a subprocess (see z3.go). Feasible paths yield a model,
// which is decoded back into shen-derive sample values (see decode.go).
//
// Lists are handled by bounded unrolling: list-typed parameters are
// enumerated at every concrete length from 0 to a configurable depth
// (default 4), so list recursion and the list primitives (foldr, scanl,
// map, filter, head, tail, cons, concat) all terminate with a concrete
// spine and symbolic scalars at the leaves.
package symbolic

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Sort is the SMT sort of a constraint term.
type Sort int

const (
	SortBool Sort = iota
	SortReal
	SortString
)

func (s Sort) String() string {
	switch s {
	case SortBool:
		return "Bool"
	case SortReal:
		return "Real"
	case SortString:
		return "String"
	}
	return "?"
}

// Term is a node of the constraint AST.
type Term interface {
	termNode()
	// Sort reports the term's SMT sort.
	Sort() Sort
	// String renders the term in SMT-LIB2 prefix syntax.
	String() string
}

// Var is a free variable standing for one scalar of one input.
type Var struct {
	Name string
	S    Sort
}

// Lit is a literal of one of the three sorts.
type Lit struct {
	S   Sort
	B   bool
	R   float64
	Str string
}

// App is an operator application. Op is already an SMT-LIB2 operator
// name (or "ite"), so printing is a straight prefix walk.
type App struct {
	Op   string
	S    Sort
	Args []Term
}

func (*Var) termNode() {}
func (*Lit) termNode() {}
func (*App) termNode() {}

func (v *Var) Sort() Sort { return v.S }
func (l *Lit) Sort() Sort { return l.S }
func (a *App) Sort() Sort { return a.S }

func (v *Var) String() string { return v.Name }

func (l *Lit) String() string {
	switch l.S {
	case SortBool:
		if l.B {
			return "true"
		}
		return "false"
	case SortString:
		return quoteSMTString(l.Str)
	default:
		return formatReal(l.R)
	}
}

func (a *App) String() string {
	parts := make([]string, len(a.Args))
	for i, t := range a.Args {
		parts[i] = t.String()
	}
	return "(" + a.Op + " " + strings.Join(parts, " ") + ")"
}

// formatReal renders a float64 as an SMT-LIB2 Real literal. Negative
// numbers need the unary-minus form because SMT-LIB2 has no negative
// numerals.
func formatReal(f float64) string {
	if f < 0 {
		return "(- " + formatReal(-f) + ")"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// quoteSMTString renders a Go string as an SMT-LIB2 string literal.
// SMT-LIB2 escapes a double quote by doubling it.
func quoteSMTString(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// --- constructors (with constant folding) ---

// NewVar makes a fresh variable term.
func NewVar(name string, s Sort) *Var { return &Var{Name: name, S: s} }

// R makes a Real literal.
func R(f float64) *Lit { return &Lit{S: SortReal, R: f} }

// B makes a Bool literal.
func B(b bool) *Lit { return &Lit{S: SortBool, B: b} }

// S makes a String literal.
func S(s string) *Lit { return &Lit{S: SortString, Str: s} }

// BoolLit returns the literal value of t when t is a boolean literal.
func BoolLit(t Term) (bool, bool) {
	l, ok := t.(*Lit)
	if !ok || l.S != SortBool {
		return false, false
	}
	return l.B, true
}

// RealLit returns the literal value of t when t is a real literal.
func RealLit(t Term) (float64, bool) {
	l, ok := t.(*Lit)
	if !ok || l.S != SortReal {
		return 0, false
	}
	return l.R, true
}

// StrLit returns the literal value of t when t is a string literal.
func StrLit(t Term) (string, bool) {
	l, ok := t.(*Lit)
	if !ok || l.S != SortString {
		return "", false
	}
	return l.Str, true
}

// Not negates a boolean term, folding literals and double negation.
func Not(t Term) Term {
	if b, ok := BoolLit(t); ok {
		return B(!b)
	}
	if a, ok := t.(*App); ok && a.Op == "not" {
		return a.Args[0]
	}
	return &App{Op: "not", S: SortBool, Args: []Term{t}}
}

// And conjoins terms, dropping `true` and collapsing on `false`.
func And(ts ...Term) Term {
	var keep []Term
	for _, t := range ts {
		if b, ok := BoolLit(t); ok {
			if b {
				continue
			}
			return B(false)
		}
		keep = append(keep, t)
	}
	switch len(keep) {
	case 0:
		return B(true)
	case 1:
		return keep[0]
	}
	return &App{Op: "and", S: SortBool, Args: keep}
}

// Or disjoins terms, dropping `false` and collapsing on `true`.
func Or(ts ...Term) Term {
	var keep []Term
	for _, t := range ts {
		if b, ok := BoolLit(t); ok {
			if b {
				return B(true)
			}
			continue
		}
		keep = append(keep, t)
	}
	switch len(keep) {
	case 0:
		return B(false)
	case 1:
		return keep[0]
	}
	return &App{Op: "or", S: SortBool, Args: keep}
}

// Arith builds a linear-arithmetic application over Real terms.
// Supported ops: + - * /. Literal operands are folded.
func Arith(op string, a, b Term) (Term, error) {
	x, xok := RealLit(a)
	y, yok := RealLit(b)
	if xok && yok {
		switch op {
		case "+":
			return R(x + y), nil
		case "-":
			return R(x - y), nil
		case "*":
			return R(x * y), nil
		case "/":
			if y == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return R(x / y), nil
		}
	}
	// Keep the fragment linear: a product with two non-literal operands
	// is outside LRA, so refuse it rather than emit something the solver
	// will answer "unknown" to.
	if op == "*" && !xok && !yok {
		return nil, fmt.Errorf("non-linear multiplication is outside the supported fragment")
	}
	if op == "/" && !yok {
		return nil, fmt.Errorf("division by a symbolic denominator is outside the supported fragment")
	}
	switch op {
	case "+", "-", "*", "/":
		return &App{Op: op, S: SortReal, Args: []Term{a, b}}, nil
	}
	return nil, fmt.Errorf("unsupported arithmetic operator %q", op)
}

// Cmp builds a numeric comparison, folding literal operands.
func Cmp(op string, a, b Term) (Term, error) {
	x, xok := RealLit(a)
	y, yok := RealLit(b)
	if xok && yok {
		switch op {
		case "<":
			return B(x < y), nil
		case "<=":
			return B(x <= y), nil
		case ">":
			return B(x > y), nil
		case ">=":
			return B(x >= y), nil
		}
	}
	switch op {
	case "<", "<=", ">", ">=":
		return &App{Op: op, S: SortBool, Args: []Term{a, b}}, nil
	}
	return nil, fmt.Errorf("unsupported comparison %q", op)
}

// EqTerm builds an equality between two same-sorted terms.
func EqTerm(a, b Term) (Term, error) {
	if a.Sort() != b.Sort() {
		// Mixed sorts can never be equal in this fragment.
		return B(false), nil
	}
	switch a.Sort() {
	case SortReal:
		if x, ok := RealLit(a); ok {
			if y, ok2 := RealLit(b); ok2 {
				return B(x == y), nil
			}
		}
	case SortString:
		if x, ok := StrLit(a); ok {
			if y, ok2 := StrLit(b); ok2 {
				return B(x == y), nil
			}
		}
	case SortBool:
		if x, ok := BoolLit(a); ok {
			if y, ok2 := BoolLit(b); ok2 {
				return B(x == y), nil
			}
		}
	}
	return &App{Op: "=", S: SortBool, Args: []Term{a, b}}, nil
}

// StrLen builds (str.len s).
func StrLen(a Term) (Term, error) {
	if a.Sort() != SortString {
		return nil, fmt.Errorf("str.len: expected a string term")
	}
	if s, ok := StrLit(a); ok {
		return R(float64(len(s))), nil
	}
	return &App{Op: "str.len", S: SortReal, Args: []Term{a}}, nil
}

// Ite builds an if-then-else term, folding a literal condition.
func Ite(cond, then, els Term) (Term, error) {
	if b, ok := BoolLit(cond); ok {
		if b {
			return then, nil
		}
		return els, nil
	}
	if then.Sort() != els.Sort() {
		return nil, fmt.Errorf("ite: branches have different sorts (%s vs %s)", then.Sort(), els.Sort())
	}
	return &App{Op: "ite", S: then.Sort(), Args: []Term{cond, then, els}}, nil
}

// FreeVars returns every variable appearing in the given terms, sorted
// by name so declarations are emitted deterministically.
func FreeVars(ts ...Term) []*Var {
	seen := map[string]*Var{}
	var walk func(Term)
	walk = func(t Term) {
		switch x := t.(type) {
		case *Var:
			seen[x.Name] = x
		case *App:
			for _, a := range x.Args {
				walk(a)
			}
		}
	}
	for _, t := range ts {
		if t != nil {
			walk(t)
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Var, 0, len(names))
	for _, n := range names {
		out = append(out, seen[n])
	}
	return out
}
