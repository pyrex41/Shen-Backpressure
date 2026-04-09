package shen

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
)

type symValue interface{}

type symPoly struct {
	poly polynomial
}

type symBool struct {
	value bool
}

type symClosure struct {
	env   map[string]symValue
	param string
	body  core.Term
}

type symPrimPartial struct {
	op   core.PrimOp
	args []symValue
}

type proofSupportError struct {
	msg string
}

const (
	symbolicPolynomialMethod = "symbolic-polynomial"
	symbolicFragmentMethod   = "symbolic-fragment"
)

func (e *proofSupportError) Error() string {
	return e.msg
}

type polynomial struct {
	terms map[string]*big.Int
}

func dischargeSymbolic(cond laws.InstantiatedCondition) (DischargeResult, bool) {
	freeVars := conditionFreeVars(cond)
	sort.Strings(freeVars)

	freeVarTypes, err := inferConditionFreeVarTypes(cond, freeVars)
	if err != nil {
		return unsupportedProofResult(symbolicFragmentMethod, err), false
	}
	for _, name := range freeVars {
		if _, ok := freeVarTypes[name].(*core.TInt); !ok {
			return unsupportedProofResult(symbolicFragmentMethod,
				fmt.Errorf("free variable %s has non-integer type %s", name, freeVarTypes[name])), false
		}
	}

	env := make(map[string]symValue, len(freeVars))
	for _, name := range freeVars {
		env[name] = symPoly{poly: varPolynomial(name)}
	}

	lhs, err := symbolicEval(cond.LHS, env)
	if err != nil {
		return unsupportedProofResult(symbolicFragmentMethod, err), false
	}
	rhs, err := symbolicEval(cond.RHS, env)
	if err != nil {
		return unsupportedProofResult(symbolicFragmentMethod, err), false
	}

	// Extended symbolic fragment: polynomial equalities OR boolean combinations
	// (conj/disj/not) of comparisons (=,!=,<,<=,>,>=) over normalized integer
	// polynomial terms. Only soundly proves when normalization yields a
	// constant boolean truth value independent of free integer variables.
	if p1, ok := lhs.(symPoly); ok {
		if p2, ok := rhs.(symPoly); ok {
			if p1.poly.equal(p2.poly) {
				return DischargeResult{
					Discharged: true,
					Method:     symbolicPolynomialMethod,
					Output: fmt.Sprintf("proved normalized polynomial equality:\n  lhs = %s\n  rhs = %s",
						p1.poly.String(), p2.poly.String()),
				}, true
			}

			diff := p1.poly.sub(p2.poly)
			return DischargeResult{
				Discharged: false,
				Method:     symbolicPolynomialMethod,
				Output: fmt.Sprintf("lhs = %s\nrhs = %s\ndiff = %s",
					p1.poly.String(), p2.poly.String(), diff.String()),
				Error: fmt.Errorf("symbolic polynomial proof failed: normalized forms differ"),
			}, true
		}
	}

	if b1, ok := lhs.(symBool); ok {
		if b2, ok := rhs.(symBool); ok {
			if b1.value == b2.value {
				return DischargeResult{
					Discharged: true,
					Method:     symbolicFragmentMethod,
					Output: fmt.Sprintf("proved normalized boolean/comparison formula evaluates identically to %t\n(supported: and/or/not of arithmetic comparisons on integer polynomials)",
						b1.value),
				}, true
			}
			return DischargeResult{
				Discharged: false,
				Method:     symbolicFragmentMethod,
				Output:     fmt.Sprintf("lhs=%t rhs=%t", b1.value, b2.value),
				Error:      fmt.Errorf("boolean expressions normalize to different constants"),
			}, true
		}
	}

	return unsupportedProofResult(symbolicFragmentMethod,
		fmt.Errorf("lhs/rhs normalized to unsupported types in fragment: %T vs %T (only polys or constant-bool from supported arith comparisons)", lhs, rhs)), false
}

func unsupportedProofResult(method string, err error) DischargeResult {
	return DischargeResult{
		Discharged: false,
		Method:     method,
		Error:      err,
	}
}

func inferConditionFreeVarTypes(cond laws.InstantiatedCondition, freeVars []string) (map[string]core.Type, error) {
	var wrapped core.Term = core.MkTuple(cond.LHS, cond.RHS)
	for i := len(freeVars) - 1; i >= 0; i-- {
		wrapped = core.MkLam(freeVars[i], nil, wrapped)
	}

	ty, err := core.CheckTerm(wrapped)
	if err != nil {
		return nil, fmt.Errorf("cannot infer obligation types: %w", err)
	}

	types := make(map[string]core.Type, len(freeVars))
	cur := ty
	for _, name := range freeVars {
		fn, ok := cur.(*core.TFun)
		if !ok {
			return nil, fmt.Errorf("expected function type while inferring %s, got %s", name, cur)
		}
		if _, ok := fn.Param.(*core.TVar); ok {
			return nil, fmt.Errorf("free variable %s remains polymorphic", name)
		}
		types[name] = fn.Param
		cur = fn.Result
	}

	return types, nil
}

func symbolicEval(term core.Term, env map[string]symValue) (symValue, error) {
	switch t := term.(type) {
	case *core.Var:
		if v, ok := env[t.Name]; ok {
			return v, nil
		}
		return nil, &proofSupportError{msg: fmt.Sprintf("unbound variable %q in symbolic prover", t.Name)}
	case *core.Lit:
		switch t.Kind {
		case core.LitInt:
			return symPoly{poly: constPolynomial(t.IntVal)}, nil
		case core.LitBool:
			return symBool{value: t.BoolVal}, nil
		default:
			return nil, &proofSupportError{msg: fmt.Sprintf("unsupported literal kind %v", t.Kind)}
		}
	case *core.Lam:
		return &symClosure{env: copySymEnv(env), param: t.Param, body: t.Body}, nil
	case *core.App:
		fn, err := symbolicEval(t.Func, env)
		if err != nil {
			return nil, err
		}
		arg, err := symbolicEval(t.Arg, env)
		if err != nil {
			return nil, err
		}
		return symbolicApply(fn, arg)
	case *core.Let:
		bound, err := symbolicEval(t.Bound, env)
		if err != nil {
			return nil, err
		}
		next := copySymEnv(env)
		next[t.Name] = bound
		return symbolicEval(t.Body, next)
	case *core.IfExpr:
		cond, err := symbolicEval(t.Cond, env)
		if err != nil {
			return nil, err
		}
		b, ok := cond.(symBool)
		if !ok {
			return nil, &proofSupportError{msg: "symbolic prover only supports ground boolean conditions"}
		}
		if b.value {
			return symbolicEval(t.Then, env)
		}
		return symbolicEval(t.Else, env)
	case *core.Prim:
		return &symPrimPartial{op: t.Op}, nil
	default:
		return nil, &proofSupportError{msg: fmt.Sprintf("unsupported term %T in symbolic prover", t)}
	}
}

func symbolicApply(fn symValue, arg symValue) (symValue, error) {
	switch f := fn.(type) {
	case *symClosure:
		next := copySymEnv(f.env)
		next[f.param] = arg
		return symbolicEval(f.body, next)
	case *symPrimPartial:
		args := append(append([]symValue(nil), f.args...), arg)
		if len(args) < core.PrimArity(f.op) {
			return &symPrimPartial{op: f.op, args: args}, nil
		}
		return symbolicExecPrim(f.op, args)
	default:
		return nil, &proofSupportError{msg: fmt.Sprintf("cannot apply symbolic value %T", fn)}
	}
}

func symbolicExecPrim(op core.PrimOp, args []symValue) (symValue, error) {
	switch op {
	case core.PrimAdd:
		a, err := asSymPoly(args[0])
		if err != nil {
			return nil, err
		}
		b, err := asSymPoly(args[1])
		if err != nil {
			return nil, err
		}
		return symPoly{poly: a.add(b)}, nil
	case core.PrimSub:
		a, err := asSymPoly(args[0])
		if err != nil {
			return nil, err
		}
		b, err := asSymPoly(args[1])
		if err != nil {
			return nil, err
		}
		return symPoly{poly: a.sub(b)}, nil
	case core.PrimMul:
		a, err := asSymPoly(args[0])
		if err != nil {
			return nil, err
		}
		b, err := asSymPoly(args[1])
		if err != nil {
			return nil, err
		}
		return symPoly{poly: a.mul(b)}, nil
	case core.PrimNeg:
		a, err := asSymPoly(args[0])
		if err != nil {
			return nil, err
		}
		return symPoly{poly: a.neg()}, nil
	case core.PrimEq:
		return compareSymPolys(args, func(c int) bool { return c == 0 }, "=")
	case core.PrimNeq:
		return compareSymPolys(args, func(c int) bool { return c != 0 }, "!=")
	case core.PrimLt:
		return compareSymPolys(args, func(c int) bool { return c < 0 }, "<")
	case core.PrimLe:
		return compareSymPolys(args, func(c int) bool { return c <= 0 }, "<=")
	case core.PrimGt:
		return compareSymPolys(args, func(c int) bool { return c > 0 }, ">")
	case core.PrimGe:
		return compareSymPolys(args, func(c int) bool { return c >= 0 }, ">=")
	case core.PrimAnd:
		a, err := asSymBool(args[0])
		if err != nil {
			return nil, err
		}
		b, err := asSymBool(args[1])
		if err != nil {
			return nil, err
		}
		return symBool{value: a && b}, nil
	case core.PrimOr:
		a, err := asSymBool(args[0])
		if err != nil {
			return nil, err
		}
		b, err := asSymBool(args[1])
		if err != nil {
			return nil, err
		}
		return symBool{value: a || b}, nil
	case core.PrimNot:
		a, err := asSymBool(args[0])
		if err != nil {
			return nil, err
		}
		return symBool{value: !a}, nil
	default:
		return nil, &proofSupportError{msg: fmt.Sprintf("unsupported primitive %s in symbolic prover", op)}
	}
}

func asSymPoly(v symValue) (polynomial, error) {
	p, ok := v.(symPoly)
	if !ok {
		return polynomial{}, &proofSupportError{msg: fmt.Sprintf("expected polynomial value, got %T", v)}
	}
	return p.poly, nil
}

func asSymBool(v symValue) (bool, error) {
	b, ok := v.(symBool)
	if !ok {
		return false, &proofSupportError{msg: fmt.Sprintf("expected boolean value, got %T", v)}
	}
	return b.value, nil
}

// asSymPolys extracts two polynomials; does NOT require them to be constant
// (constant check is done only for ordering comparisons).
func asSymPolys(args []symValue) (polynomial, polynomial, error) {
	a, err := asSymPoly(args[0])
	if err != nil {
		return polynomial{}, polynomial{}, err
	}
	b, err := asSymPoly(args[1])
	if err != nil {
		return polynomial{}, polynomial{}, err
	}
	return a, b, nil
}

// compareSymPolys decides arithmetic comparisons soundly only when the
// normalized difference polynomial is a constant (i.e. the comparison truth
// does not depend on the values of free integer variables).
func compareSymPolys(args []symValue, pred func(int) bool, opName string) (symValue, error) {
	a, b, err := asSymPolys(args)
	if err != nil {
		return nil, err
	}
	diff := a.sub(b)
	if !diff.isConstant() {
		return nil, &proofSupportError{msg: fmt.Sprintf("symbolic prover only supports %s comparisons of normalized integer polynomials whose difference is constant (truth value independent of free variables)", opName)}
	}
	return symBool{value: pred(diff.constant().Cmp(big.NewInt(0)))}, nil
}

func copySymEnv(env map[string]symValue) map[string]symValue {
	out := make(map[string]symValue, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

func zeroPolynomial() polynomial {
	return polynomial{terms: make(map[string]*big.Int)}
}

func constPolynomial(n int64) polynomial {
	p := zeroPolynomial()
	if n != 0 {
		p.terms[""] = big.NewInt(n)
	}
	return p
}

func varPolynomial(name string) polynomial {
	p := zeroPolynomial()
	p.terms[name] = big.NewInt(1)
	return p
}

func (p polynomial) add(q polynomial) polynomial {
	out := p.clone()
	for mono, coeff := range q.terms {
		out.addCoeff(mono, coeff)
	}
	return out
}

func (p polynomial) sub(q polynomial) polynomial {
	out := p.clone()
	for mono, coeff := range q.terms {
		out.addCoeff(mono, new(big.Int).Neg(coeff))
	}
	return out
}

func (p polynomial) mul(q polynomial) polynomial {
	out := zeroPolynomial()
	for m1, c1 := range p.terms {
		for m2, c2 := range q.terms {
			coeff := new(big.Int).Mul(c1, c2)
			out.addCoeff(multiplyMonomials(m1, m2), coeff)
		}
	}
	return out
}

func (p polynomial) neg() polynomial {
	out := zeroPolynomial()
	for mono, coeff := range p.terms {
		out.terms[mono] = new(big.Int).Neg(coeff)
	}
	return out
}

func (p polynomial) equal(q polynomial) bool {
	p = p.clone()
	q = q.clone()
	p.normalize()
	q.normalize()
	if len(p.terms) != len(q.terms) {
		return false
	}
	for mono, coeff := range p.terms {
		other, ok := q.terms[mono]
		if !ok || coeff.Cmp(other) != 0 {
			return false
		}
	}
	return true
}

func (p polynomial) isConstant() bool {
	p = p.clone()
	p.normalize()
	return len(p.terms) == 0 || (len(p.terms) == 1 && p.terms[""] != nil)
}

func (p polynomial) constant() *big.Int {
	if coeff, ok := p.terms[""]; ok {
		return new(big.Int).Set(coeff)
	}
	return big.NewInt(0)
}

func (p polynomial) clone() polynomial {
	out := zeroPolynomial()
	for mono, coeff := range p.terms {
		out.terms[mono] = new(big.Int).Set(coeff)
	}
	return out
}

func (p *polynomial) addCoeff(mono string, coeff *big.Int) {
	if coeff.Sign() == 0 {
		return
	}
	if existing, ok := p.terms[mono]; ok {
		sum := new(big.Int).Add(existing, coeff)
		if sum.Sign() == 0 {
			delete(p.terms, mono)
			return
		}
		p.terms[mono] = sum
		return
	}
	p.terms[mono] = new(big.Int).Set(coeff)
}

func (p *polynomial) normalize() {
	for mono, coeff := range p.terms {
		if coeff.Sign() == 0 {
			delete(p.terms, mono)
		}
	}
}

func (p polynomial) String() string {
	p = p.clone()
	p.normalize()
	if len(p.terms) == 0 {
		return "0"
	}

	monos := make([]string, 0, len(p.terms))
	for mono := range p.terms {
		monos = append(monos, mono)
	}
	sort.Strings(monos)

	parts := make([]string, 0, len(monos))
	for _, mono := range monos {
		coeff := p.terms[mono]
		switch mono {
		case "":
			parts = append(parts, coeff.String())
		default:
			switch coeff.Cmp(big.NewInt(1)) {
			case 0:
				parts = append(parts, mono)
			default:
				if coeff.Cmp(big.NewInt(-1)) == 0 {
					parts = append(parts, "-"+mono)
				} else {
					parts = append(parts, fmt.Sprintf("%s*%s", coeff.String(), mono))
				}
			}
		}
	}
	return strings.Join(parts, " + ")
}

func multiplyMonomials(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	parts := append(splitMonomial(a), splitMonomial(b)...)
	sort.Strings(parts)
	return strings.Join(parts, "*")
}

func splitMonomial(mono string) []string {
	if mono == "" {
		return nil
	}
	return strings.Split(mono, "*")
}
