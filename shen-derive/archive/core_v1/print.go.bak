package core

import (
	"fmt"
	"strings"
)

// PrettyPrint renders a term in readable Haskell-like surface syntax.
func PrettyPrint(t Term) string {
	return ppTerm(t, 0)
}

// Precedence levels for parenthesization.
const (
	precTop    = 0 // lambda, let, if
	precOr     = 1
	precAnd    = 2
	precCmp    = 3
	precAdd    = 4
	precMul    = 5
	precComp   = 6
	precApp    = 7
	precAtom   = 8
)

func ppTerm(t Term, prec int) string {
	switch t := t.(type) {
	case *Var:
		return t.Name

	case *Lit:
		switch t.Kind {
		case LitInt:
			if t.IntVal < 0 {
				s := fmt.Sprintf("(%d)", t.IntVal)
				return s
			}
			return fmt.Sprintf("%d", t.IntVal)
		case LitBool:
			if t.BoolVal {
				return "True"
			}
			return "False"
		case LitString:
			return fmt.Sprintf("%q", t.StrVal)
		}

	case *Prim:
		return ppPrimOp(t.Op)

	case *ListLit:
		elems := make([]string, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = ppTerm(e, precTop)
		}
		return "[" + strings.Join(elems, ", ") + "]"

	case *TupleLit:
		return "(" + ppTerm(t.Fst, precTop) + ", " + ppTerm(t.Snd, precTop) + ")"

	case *IfExpr:
		s := "if " + ppTerm(t.Cond, precTop) +
			" then " + ppTerm(t.Then, precTop) +
			" else " + ppTerm(t.Else, precTop)
		if prec > precTop {
			return "(" + s + ")"
		}
		return s

	case *Lam:
		s := ppLam(t)
		if prec > precTop {
			return "(" + s + ")"
		}
		return s

	case *Let:
		s := "let " + t.Name + " = " + ppTerm(t.Bound, precTop) +
			" in " + ppTerm(t.Body, precTop)
		if prec > precTop {
			return "(" + s + ")"
		}
		return s

	case *App:
		// Detect and pretty-print infix operators
		if s, ok := ppInfix(t, prec); ok {
			return s
		}
		// Detect negate: negate x => -x
		if p, ok := t.Func.(*Prim); ok && p.Op == PrimNeg {
			s := "-" + ppTerm(t.Arg, precAtom)
			if prec > precMul {
				return "(" + s + ")"
			}
			return s
		}
		// Regular application
		s := ppTerm(t.Func, precApp) + " " + ppTerm(t.Arg, precAtom)
		if prec > precApp {
			return "(" + s + ")"
		}
		return s
	}

	return fmt.Sprintf("<?%T>", t)
}

// ppLam collects nested lambdas into a multi-parameter form.
func ppLam(l *Lam) string {
	var params []string
	body := Term(l)
	for {
		lam, ok := body.(*Lam)
		if !ok {
			break
		}
		if lam.ParamType != nil {
			params = append(params, "("+lam.Param+" : "+lam.ParamType.String()+")")
		} else {
			params = append(params, lam.Param)
		}
		body = lam.Body
	}
	return "\\" + strings.Join(params, " ") + " -> " + ppTerm(body, precTop)
}

// ppInfix detects App(App(Prim(op), lhs), rhs) and prints as infix.
func ppInfix(app *App, prec int) (string, bool) {
	inner, ok := app.Func.(*App)
	if !ok {
		return "", false
	}
	p, ok := inner.Func.(*Prim)
	if !ok {
		return "", false
	}

	op := p.Op
	var opStr string
	var opPrec int

	switch op {
	case PrimAdd:
		opStr, opPrec = "+", precAdd
	case PrimSub:
		opStr, opPrec = "-", precAdd
	case PrimMul:
		opStr, opPrec = "*", precMul
	case PrimDiv:
		opStr, opPrec = "/", precMul
	case PrimMod:
		opStr, opPrec = "%", precMul
	case PrimEq:
		opStr, opPrec = "==", precCmp
	case PrimNeq:
		opStr, opPrec = "/=", precCmp
	case PrimLt:
		opStr, opPrec = "<", precCmp
	case PrimLe:
		opStr, opPrec = "<=", precCmp
	case PrimGt:
		opStr, opPrec = ">", precCmp
	case PrimGe:
		opStr, opPrec = ">=", precCmp
	case PrimAnd:
		opStr, opPrec = "&&", precAnd
	case PrimOr:
		opStr, opPrec = "||", precOr
	case PrimCompose:
		opStr, opPrec = ".", precComp
	case PrimCons:
		opStr, opPrec = ":", precAdd
	default:
		return "", false
	}

	lhs := ppTerm(inner.Arg, opPrec+1)
	// Right-associative operators (compose) don't need parens on the right
	rhsPrec := opPrec + 1
	if op == PrimCompose {
		rhsPrec = opPrec
	}
	rhs := ppTerm(app.Arg, rhsPrec)
	s := lhs + " " + opStr + " " + rhs
	if prec > opPrec {
		return "(" + s + ")", true
	}
	return s, true
}

func ppPrimOp(op PrimOp) string {
	switch op {
	case PrimAdd, PrimSub, PrimMul, PrimDiv, PrimMod,
		PrimEq, PrimNeq, PrimLt, PrimLe, PrimGt, PrimGe,
		PrimAnd, PrimOr:
		return "(" + string(op) + ")"
	default:
		return string(op)
	}
}

// PrintValue renders a value in readable syntax.
func PrintValue(v Value) string {
	return v.String()
}
