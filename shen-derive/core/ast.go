// Package core implements the derivation core for shen-derive:
// a typed lambda calculus with list/tuple combinators, suitable for
// expressing fold-shaped pure computations and applying algebraic rewrites.
package core

import "fmt"

// Position tracks source location for error messages.
type Position struct {
	Line int
	Col  int
}

func (p Position) String() string {
	if p.Line == 0 && p.Col == 0 {
		return "<unknown>"
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Col)
}

// Term is the interface for all AST nodes in the derivation core.
type Term interface {
	termNode()
	Pos() Position
}

// Var represents a variable reference.
type Var struct {
	Name string
	P    Position
}

// Lam represents a lambda abstraction. ParamType may be nil if the
// annotation is omitted (the type checker will attempt to infer it).
type Lam struct {
	Param     string
	ParamType Type
	Body      Term
	P         Position
}

// App represents function application.
type App struct {
	Func Term
	Arg  Term
	P    Position
}

// Let represents a let-binding: let Name = Bound in Body.
type Let struct {
	Name  string
	Bound Term
	Body  Term
	P     Position
}

// LitKind distinguishes literal value types.
type LitKind int

const (
	LitInt LitKind = iota
	LitBool
	LitString
)

// Lit represents a literal value (int, bool, or string).
type Lit struct {
	Kind   LitKind
	IntVal int64
	BoolVal bool
	StrVal string
	P      Position
}

// ListLit represents a list literal: [e1, e2, ...].
type ListLit struct {
	Elems []Term
	P     Position
}

// TupleLit represents a pair literal: (e1, e2).
type TupleLit struct {
	Fst Term
	Snd Term
	P   Position
}

// IfExpr represents if-then-else: if Cond then Then else Else.
type IfExpr struct {
	Cond Term
	Then Term
	Else Term
	P    Position
}

// PrimOp names a primitive operation. String values match the surface syntax.
type PrimOp string

const (
	// List combinators
	PrimMap     PrimOp = "map"
	PrimFoldr   PrimOp = "foldr"
	PrimFoldl   PrimOp = "foldl"
	PrimScanl   PrimOp = "scanl"
	PrimUnfoldr PrimOp = "unfoldr"
	PrimFilter  PrimOp = "filter"
	PrimConcat  PrimOp = "concat"
	PrimCons    PrimOp = "cons"
	PrimNil     PrimOp = "nil"

	// Function composition
	PrimCompose PrimOp = "compose"

	// Tuple projections
	PrimFst PrimOp = "fst"
	PrimSnd PrimOp = "snd"

	// Arithmetic
	PrimAdd PrimOp = "+"
	PrimSub PrimOp = "-"
	PrimMul PrimOp = "*"
	PrimDiv PrimOp = "/"
	PrimMod PrimOp = "%"

	// Comparison
	PrimEq  PrimOp = "=="
	PrimNeq PrimOp = "/="
	PrimLt  PrimOp = "<"
	PrimLe  PrimOp = "<="
	PrimGt  PrimOp = ">"
	PrimGe  PrimOp = ">="

	// Boolean
	PrimAnd PrimOp = "&&"
	PrimOr  PrimOp = "||"
	PrimNot PrimOp = "not"

	// Numeric
	PrimNeg PrimOp = "negate"
)

// Prim represents a reference to a built-in primitive operation.
type Prim struct {
	Op PrimOp
	P  Position
}

// --- Term interface implementations ---

func (*Var) termNode()      {}
func (*Lam) termNode()      {}
func (*App) termNode()      {}
func (*Let) termNode()      {}
func (*Lit) termNode()      {}
func (*ListLit) termNode()  {}
func (*TupleLit) termNode() {}
func (*IfExpr) termNode()   {}
func (*Prim) termNode()     {}

func (n *Var) Pos() Position      { return n.P }
func (n *Lam) Pos() Position      { return n.P }
func (n *App) Pos() Position      { return n.P }
func (n *Let) Pos() Position      { return n.P }
func (n *Lit) Pos() Position      { return n.P }
func (n *ListLit) Pos() Position  { return n.P }
func (n *TupleLit) Pos() Position { return n.P }
func (n *IfExpr) Pos() Position   { return n.P }
func (n *Prim) Pos() Position     { return n.P }

// PrimArity returns the number of arguments a fully-applied primitive expects.
func PrimArity(op PrimOp) int {
	switch op {
	case PrimNil:
		return 0
	case PrimNot, PrimNeg, PrimFst, PrimSnd, PrimConcat:
		return 1
	case PrimAdd, PrimSub, PrimMul, PrimDiv, PrimMod,
		PrimEq, PrimNeq, PrimLt, PrimLe, PrimGt, PrimGe,
		PrimAnd, PrimOr,
		PrimCons, PrimMap, PrimFilter, PrimUnfoldr:
		return 2
	case PrimFoldr, PrimFoldl, PrimScanl, PrimCompose:
		return 3
	default:
		return 0
	}
}

// IsPrimName returns the PrimOp for a built-in name, or false if not a primitive.
func IsPrimName(name string) (PrimOp, bool) {
	switch name {
	case "map":
		return PrimMap, true
	case "foldr":
		return PrimFoldr, true
	case "foldl":
		return PrimFoldl, true
	case "scanl":
		return PrimScanl, true
	case "unfoldr":
		return PrimUnfoldr, true
	case "filter":
		return PrimFilter, true
	case "concat":
		return PrimConcat, true
	case "cons":
		return PrimCons, true
	case "nil":
		return PrimNil, true
	case "compose":
		return PrimCompose, true
	case "fst":
		return PrimFst, true
	case "snd":
		return PrimSnd, true
	case "not":
		return PrimNot, true
	case "negate":
		return PrimNeg, true
	default:
		return "", false
	}
}

// Helper constructors for building AST nodes programmatically.

func MkVar(name string) *Var          { return &Var{Name: name} }
func MkInt(n int64) *Lit              { return &Lit{Kind: LitInt, IntVal: n} }
func MkBool(b bool) *Lit              { return &Lit{Kind: LitBool, BoolVal: b} }
func MkStr(s string) *Lit             { return &Lit{Kind: LitString, StrVal: s} }
func MkPrim(op PrimOp) *Prim          { return &Prim{Op: op} }
func MkList(elems ...Term) *ListLit   { return &ListLit{Elems: elems} }
func MkTuple(a, b Term) *TupleLit     { return &TupleLit{Fst: a, Snd: b} }
func MkIf(c, t, e Term) *IfExpr       { return &IfExpr{Cond: c, Then: t, Else: e} }
func MkLam(p string, ty Type, b Term) *Lam { return &Lam{Param: p, ParamType: ty, Body: b} }
func MkApp(f, a Term) *App            { return &App{Func: f, Arg: a} }
func MkLet(n string, v, b Term) *Let  { return &Let{Name: n, Bound: v, Body: b} }

// MkApps applies f to multiple arguments: MkApps(f, a, b) = App(App(f, a), b).
func MkApps(f Term, args ...Term) Term {
	t := f
	for _, a := range args {
		t = MkApp(t, a)
	}
	return t
}
