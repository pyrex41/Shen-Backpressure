// Package core provides the s-expression representation for shen-derive.
//
// All terms in shen-derive are represented as s-expressions — the same
// representation used by Shen itself. This means shen-derive can handle
// any type Shen can express, and .shen files are the single source of truth
// for both shengen (guard types) and shen-derive (derived computations).
//
// Type checking delegates to Shen tc+ via subprocess. shen-derive does not
// have its own type system.
package core

import (
	"fmt"
	"strconv"
	"strings"
)

// Sexpr is the universal term representation — atoms, lists, and nothing else.
type Sexpr interface {
	sexprNode()
	String() string
	// Equal returns true if two s-expressions are structurally identical.
	Equal(other Sexpr) bool
}

// AtomKind distinguishes atom types.
type AtomKind int

const (
	AtomSymbol AtomKind = iota
	AtomInt
	AtomFloat
	AtomString
	AtomBool
)

// Atom represents a leaf value: symbol, number, string, or boolean.
type Atom struct {
	Val  string
	Kind AtomKind
}

// List represents a compound form: (head elem1 elem2 ...).
type List struct {
	Elems []Sexpr
}

func (*Atom) sexprNode() {}
func (*List) sexprNode() {}

func (a *Atom) String() string {
	switch a.Kind {
	case AtomString:
		return fmt.Sprintf("%q", a.Val)
	default:
		return a.Val
	}
}

func (l *List) String() string {
	parts := make([]string, len(l.Elems))
	for i, e := range l.Elems {
		parts[i] = e.String()
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func (a *Atom) Equal(other Sexpr) bool {
	b, ok := other.(*Atom)
	return ok && a.Val == b.Val && a.Kind == b.Kind
}

func (l *List) Equal(other Sexpr) bool {
	m, ok := other.(*List)
	if !ok || len(l.Elems) != len(m.Elems) {
		return false
	}
	for i := range l.Elems {
		if !l.Elems[i].Equal(m.Elems[i]) {
			return false
		}
	}
	return true
}

// --- Helper constructors ---

// Sym creates a symbol atom.
func Sym(s string) *Atom { return &Atom{Val: s, Kind: AtomSymbol} }

// Num creates an integer atom.
func Num(n int64) *Atom { return &Atom{Val: strconv.FormatInt(n, 10), Kind: AtomInt} }

// Float creates a float atom.
func Float(f float64) *Atom { return &Atom{Val: strconv.FormatFloat(f, 'f', -1, 64), Kind: AtomFloat} }

// Str creates a string atom.
func Str(s string) *Atom { return &Atom{Val: s, Kind: AtomString} }

// Bool creates a boolean atom.
func Bool(b bool) *Atom {
	if b {
		return &Atom{Val: "true", Kind: AtomBool}
	}
	return &Atom{Val: "false", Kind: AtomBool}
}

// SList creates a list from elements.
func SList(elems ...Sexpr) *List { return &List{Elems: elems} }

// --- Convenience constructors for common Shen forms ---

// Lambda creates (lambda Param Body).
func Lambda(param string, body Sexpr) *List {
	return SList(Sym("lambda"), Sym(param), body)
}

// SApply creates (f arg1 arg2 ...) — function application.
func SApply(f Sexpr, args ...Sexpr) *List {
	elems := make([]Sexpr, 0, 1+len(args))
	elems = append(elems, f)
	elems = append(elems, args...)
	return &List{Elems: elems}
}

// --- Inspection helpers ---

// IsSym checks if s is a symbol atom with the given name.
func IsSym(s Sexpr, name string) bool {
	a, ok := s.(*Atom)
	return ok && a.Kind == AtomSymbol && a.Val == name
}

// IsMetaVar checks if s is a metavariable (symbol starting with ?).
func IsMetaVar(s Sexpr) (string, bool) {
	a, ok := s.(*Atom)
	if ok && a.Kind == AtomSymbol && len(a.Val) > 1 && a.Val[0] == '?' {
		return a.Val, true
	}
	return "", false
}

// HeadSym returns the head symbol of a list, or empty string if not a symbol-headed list.
func HeadSym(s Sexpr) string {
	l, ok := s.(*List)
	if !ok || len(l.Elems) == 0 {
		return ""
	}
	a, ok := l.Elems[0].(*Atom)
	if !ok || a.Kind != AtomSymbol {
		return ""
	}
	return a.Val
}

// ListElems returns the elements if s is a List, or nil.
func ListElems(s Sexpr) []Sexpr {
	l, ok := s.(*List)
	if !ok {
		return nil
	}
	return l.Elems
}

// AtomVal returns the atom value and kind if s is an Atom, or false.
func AtomVal(s Sexpr) (string, AtomKind, bool) {
	a, ok := s.(*Atom)
	if !ok {
		return "", 0, false
	}
	return a.Val, a.Kind, true
}

// SexprIntVal returns the integer value if s is an integer atom.
func SexprIntVal(s Sexpr) (int64, bool) {
	a, ok := s.(*Atom)
	if !ok || a.Kind != AtomInt {
		return 0, false
	}
	n, err := strconv.ParseInt(a.Val, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// SexprFloatVal returns the float value if s is a float atom.
func SexprFloatVal(s Sexpr) (float64, bool) {
	a, ok := s.(*Atom)
	if !ok || (a.Kind != AtomFloat && a.Kind != AtomInt) {
		return 0, false
	}
	f, err := strconv.ParseFloat(a.Val, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// SexprBoolVal returns the boolean value if s is a boolean atom.
func SexprBoolVal(s Sexpr) (bool, bool) {
	a, ok := s.(*Atom)
	if !ok || a.Kind != AtomBool {
		return false, false
	}
	return a.Val == "true", true
}

// SymName returns the symbol name if s is a symbol atom.
func SymName(s Sexpr) (string, bool) {
	a, ok := s.(*Atom)
	if !ok || a.Kind != AtomSymbol {
		return "", false
	}
	return a.Val, true
}

// DeepCopy creates a deep copy of an s-expression.
func DeepCopy(s Sexpr) Sexpr {
	switch s := s.(type) {
	case *Atom:
		cp := *s
		return &cp
	case *List:
		elems := make([]Sexpr, len(s.Elems))
		for i, e := range s.Elems {
			elems[i] = DeepCopy(e)
		}
		return &List{Elems: elems}
	}
	return s
}
