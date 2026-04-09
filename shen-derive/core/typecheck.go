package core

import "fmt"

// TypeChecker implements type checking with simple unification for the
// derivation core. Lambda binders should have explicit type annotations
// for reliable checking; unannotated lambdas get fresh type variables.
type TypeChecker struct {
	nextVar int
	subst   map[string]Type
}

// NewTypeChecker creates a fresh type checker.
func NewTypeChecker() *TypeChecker {
	return &TypeChecker{subst: make(map[string]Type)}
}

// TypeEnv maps variable names to their types.
type TypeEnv struct {
	name   string
	ty     Type
	parent *TypeEnv
}

func EmptyTypeEnv() *TypeEnv { return nil }

func (e *TypeEnv) Extend(name string, ty Type) *TypeEnv {
	return &TypeEnv{name: name, ty: ty, parent: e}
}

func (e *TypeEnv) Lookup(name string) (Type, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.name == name {
			return cur.ty, true
		}
	}
	return nil, false
}

func (tc *TypeChecker) freshVar() *TVar {
	tc.nextVar++
	return &TVar{Name: fmt.Sprintf("$t%d", tc.nextVar)}
}

// resolve follows the substitution chain for a type variable.
func (tc *TypeChecker) resolve(t Type) Type {
	if tv, ok := t.(*TVar); ok {
		if bound, ok := tc.subst[tv.Name]; ok {
			resolved := tc.resolve(bound)
			tc.subst[tv.Name] = resolved
			return resolved
		}
	}
	return t
}

// apply applies the current substitution throughout a type.
func (tc *TypeChecker) apply(t Type) Type {
	if t == nil {
		return nil
	}
	t = tc.resolve(t)
	switch t := t.(type) {
	case *TInt, *TBool, *TString:
		return t
	case *TFun:
		return &TFun{Param: tc.apply(t.Param), Result: tc.apply(t.Result)}
	case *TList:
		return &TList{Elem: tc.apply(t.Elem)}
	case *TTuple:
		return &TTuple{Fst: tc.apply(t.Fst), Snd: tc.apply(t.Snd)}
	case *TVar:
		return t // unresolved variable
	}
	return t
}

// occurs checks whether type variable tv occurs in type t (for the occurs check).
func (tc *TypeChecker) occurs(tv *TVar, t Type) bool {
	t = tc.resolve(t)
	switch t := t.(type) {
	case *TVar:
		return tv.Name == t.Name
	case *TFun:
		return tc.occurs(tv, t.Param) || tc.occurs(tv, t.Result)
	case *TList:
		return tc.occurs(tv, t.Elem)
	case *TTuple:
		return tc.occurs(tv, t.Fst) || tc.occurs(tv, t.Snd)
	}
	return false
}

// unify unifies two types, updating the substitution.
func (tc *TypeChecker) unify(t1, t2 Type) error {
	t1 = tc.resolve(t1)
	t2 = tc.resolve(t2)

	// Type variable cases
	if tv1, ok := t1.(*TVar); ok {
		if tv2, ok := t2.(*TVar); ok && tv1.Name == tv2.Name {
			return nil
		}
		if tc.occurs(tv1, t2) {
			return fmt.Errorf("infinite type: %s occurs in %s", tv1, tc.apply(t2))
		}
		tc.subst[tv1.Name] = t2
		return nil
	}
	if tv2, ok := t2.(*TVar); ok {
		if tc.occurs(tv2, t1) {
			return fmt.Errorf("infinite type: %s occurs in %s", tv2, tc.apply(t1))
		}
		tc.subst[tv2.Name] = t1
		return nil
	}

	// Structural cases
	switch t1 := t1.(type) {
	case *TInt:
		if _, ok := t2.(*TInt); ok {
			return nil
		}
	case *TBool:
		if _, ok := t2.(*TBool); ok {
			return nil
		}
	case *TString:
		if _, ok := t2.(*TString); ok {
			return nil
		}
	case *TFun:
		if t2f, ok := t2.(*TFun); ok {
			if err := tc.unify(t1.Param, t2f.Param); err != nil {
				return err
			}
			return tc.unify(t1.Result, t2f.Result)
		}
	case *TList:
		if t2l, ok := t2.(*TList); ok {
			return tc.unify(t1.Elem, t2l.Elem)
		}
	case *TTuple:
		if t2t, ok := t2.(*TTuple); ok {
			if err := tc.unify(t1.Fst, t2t.Fst); err != nil {
				return err
			}
			return tc.unify(t1.Snd, t2t.Snd)
		}
	}

	return fmt.Errorf("type mismatch: expected %s, got %s", tc.apply(t1), tc.apply(t2))
}

// Check infers the type of a term in the given type environment.
func (tc *TypeChecker) Check(env *TypeEnv, term Term) (Type, error) {
	switch t := term.(type) {
	case *Var:
		ty, ok := env.Lookup(t.Name)
		if !ok {
			return nil, fmt.Errorf("%s: unbound variable %q", t.P, t.Name)
		}
		return ty, nil

	case *Lam:
		paramTy := t.ParamType
		if paramTy == nil {
			paramTy = tc.freshVar()
		}
		bodyTy, err := tc.Check(env.Extend(t.Param, paramTy), t.Body)
		if err != nil {
			return nil, err
		}
		return &TFun{Param: paramTy, Result: bodyTy}, nil

	case *App:
		funcTy, err := tc.Check(env, t.Func)
		if err != nil {
			return nil, err
		}
		argTy, err := tc.Check(env, t.Arg)
		if err != nil {
			return nil, err
		}
		resultTy := tc.freshVar()
		expectedFuncTy := &TFun{Param: argTy, Result: resultTy}
		if err := tc.unify(funcTy, expectedFuncTy); err != nil {
			return nil, fmt.Errorf("%s: in application: %w", t.P, err)
		}
		return tc.apply(resultTy), nil

	case *Let:
		boundTy, err := tc.Check(env, t.Bound)
		if err != nil {
			return nil, err
		}
		return tc.Check(env.Extend(t.Name, boundTy), t.Body)

	case *Lit:
		switch t.Kind {
		case LitInt:
			return &TInt{}, nil
		case LitBool:
			return &TBool{}, nil
		case LitString:
			return &TString{}, nil
		}

	case *ListLit:
		if len(t.Elems) == 0 {
			return &TList{Elem: tc.freshVar()}, nil
		}
		elemTy, err := tc.Check(env, t.Elems[0])
		if err != nil {
			return nil, err
		}
		for i := 1; i < len(t.Elems); i++ {
			eTy, err := tc.Check(env, t.Elems[i])
			if err != nil {
				return nil, err
			}
			if err := tc.unify(elemTy, eTy); err != nil {
				return nil, fmt.Errorf("%s: list element %d: %w", t.Elems[i].Pos(), i, err)
			}
		}
		return &TList{Elem: tc.apply(elemTy)}, nil

	case *TupleLit:
		fstTy, err := tc.Check(env, t.Fst)
		if err != nil {
			return nil, err
		}
		sndTy, err := tc.Check(env, t.Snd)
		if err != nil {
			return nil, err
		}
		return &TTuple{Fst: fstTy, Snd: sndTy}, nil

	case *IfExpr:
		condTy, err := tc.Check(env, t.Cond)
		if err != nil {
			return nil, err
		}
		if err := tc.unify(condTy, &TBool{}); err != nil {
			return nil, fmt.Errorf("%s: if condition must be Bool: %w", t.P, err)
		}
		thenTy, err := tc.Check(env, t.Then)
		if err != nil {
			return nil, err
		}
		elseTy, err := tc.Check(env, t.Else)
		if err != nil {
			return nil, err
		}
		if err := tc.unify(thenTy, elseTy); err != nil {
			return nil, fmt.Errorf("%s: if branches have different types: %w", t.P, err)
		}
		return tc.apply(thenTy), nil

	case *Prim:
		return tc.primType(t.Op), nil
	}

	return nil, fmt.Errorf("cannot type-check: %T", term)
}

// CheckTerm is a convenience wrapper that type-checks a term in an empty environment.
func CheckTerm(term Term) (Type, error) {
	tc := NewTypeChecker()
	ty, err := tc.Check(EmptyTypeEnv(), term)
	if err != nil {
		return nil, err
	}
	return tc.apply(ty), nil
}

// primType returns the (freshly instantiated) type of a primitive operation.
func (tc *TypeChecker) primType(op PrimOp) Type {
	switch op {
	case PrimAdd, PrimSub, PrimMul, PrimDiv, PrimMod:
		return MkTFun(&TInt{}, &TInt{}, &TInt{})
	case PrimNeg:
		return MkTFun(&TInt{}, &TInt{})
	case PrimEq, PrimNeq:
		a := tc.freshVar()
		return MkTFun(a, a, &TBool{})
	case PrimLt, PrimLe, PrimGt, PrimGe:
		return MkTFun(&TInt{}, &TInt{}, &TBool{})
	case PrimAnd, PrimOr:
		return MkTFun(&TBool{}, &TBool{}, &TBool{})
	case PrimNot:
		return MkTFun(&TBool{}, &TBool{})
	case PrimCons:
		a := tc.freshVar()
		return MkTFun(a, &TList{Elem: a}, &TList{Elem: a})
	case PrimNil:
		return &TList{Elem: tc.freshVar()}
	case PrimFst:
		a, b := tc.freshVar(), tc.freshVar()
		return &TFun{Param: &TTuple{Fst: a, Snd: b}, Result: a}
	case PrimSnd:
		a, b := tc.freshVar(), tc.freshVar()
		return &TFun{Param: &TTuple{Fst: a, Snd: b}, Result: b}
	case PrimMap:
		a, b := tc.freshVar(), tc.freshVar()
		return MkTFun(&TFun{Param: a, Result: b}, &TList{Elem: a}, &TList{Elem: b})
	case PrimFoldr:
		a, b := tc.freshVar(), tc.freshVar()
		return MkTFun(&TFun{Param: a, Result: &TFun{Param: b, Result: b}}, b, &TList{Elem: a}, b)
	case PrimFoldl:
		a, b := tc.freshVar(), tc.freshVar()
		return MkTFun(&TFun{Param: b, Result: &TFun{Param: a, Result: b}}, b, &TList{Elem: a}, b)
	case PrimScanl:
		a, b := tc.freshVar(), tc.freshVar()
		return MkTFun(&TFun{Param: b, Result: &TFun{Param: a, Result: b}}, b, &TList{Elem: a}, &TList{Elem: b})
	case PrimFilter:
		a := tc.freshVar()
		return MkTFun(&TFun{Param: a, Result: &TBool{}}, &TList{Elem: a}, &TList{Elem: a})
	case PrimConcat:
		a := tc.freshVar()
		return &TFun{Param: &TList{Elem: &TList{Elem: a}}, Result: &TList{Elem: a}}
	case PrimCompose:
		a, b, c := tc.freshVar(), tc.freshVar(), tc.freshVar()
		return MkTFun(&TFun{Param: b, Result: c}, &TFun{Param: a, Result: b}, a, c)
	case PrimUnfoldr:
		a, b := tc.freshVar(), tc.freshVar()
		// unfoldr : (b -> (Bool, (a, b))) -> b -> [a]
		pairTy := &TTuple{Fst: &TBool{}, Snd: &TTuple{Fst: a, Snd: b}}
		return MkTFun(&TFun{Param: b, Result: pairTy}, b, &TList{Elem: a})
	}
	return tc.freshVar()
}
