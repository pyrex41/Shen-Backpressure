package core

import "fmt"

// Type represents types in the derivation core's type system.
type Type interface {
	typeNode()
	String() string
}

// TInt is the integer type.
type TInt struct{}

// TBool is the boolean type.
type TBool struct{}

// TString is the string type.
type TString struct{}

// TFun is a function type: Param -> Result.
type TFun struct {
	Param  Type
	Result Type
}

// TList is a list type: [Elem].
type TList struct {
	Elem Type
}

// TTuple is a pair type: (Fst, Snd).
type TTuple struct {
	Fst Type
	Snd Type
}

// TVar is a type variable for parametric polymorphism.
type TVar struct {
	Name string
}

// --- Type interface implementations ---

func (*TInt) typeNode()    {}
func (*TBool) typeNode()   {}
func (*TString) typeNode() {}
func (*TFun) typeNode()    {}
func (*TList) typeNode()   {}
func (*TTuple) typeNode()  {}
func (*TVar) typeNode()    {}

func (*TInt) String() string    { return "Int" }
func (*TBool) String() string   { return "Bool" }
func (*TString) String() string { return "String" }

func (t *TFun) String() string {
	ps := t.Param.String()
	// Parenthesize function-type params: (a -> b) -> c
	if _, ok := t.Param.(*TFun); ok {
		ps = "(" + ps + ")"
	}
	return ps + " -> " + t.Result.String()
}

func (t *TList) String() string {
	return "[" + t.Elem.String() + "]"
}

func (t *TTuple) String() string {
	return "(" + t.Fst.String() + ", " + t.Snd.String() + ")"
}

func (t *TVar) String() string {
	return t.Name
}

// TypesEqual checks structural equality of two types, treating type variables
// with the same name as equal. Does not perform unification.
func TypesEqual(a, b Type) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch a := a.(type) {
	case *TInt:
		_, ok := b.(*TInt)
		return ok
	case *TBool:
		_, ok := b.(*TBool)
		return ok
	case *TString:
		_, ok := b.(*TString)
		return ok
	case *TFun:
		bf, ok := b.(*TFun)
		return ok && TypesEqual(a.Param, bf.Param) && TypesEqual(a.Result, bf.Result)
	case *TList:
		bl, ok := b.(*TList)
		return ok && TypesEqual(a.Elem, bl.Elem)
	case *TTuple:
		bt, ok := b.(*TTuple)
		return ok && TypesEqual(a.Fst, bt.Fst) && TypesEqual(a.Snd, bt.Snd)
	case *TVar:
		bv, ok := b.(*TVar)
		return ok && a.Name == bv.Name
	}
	return false
}

// FreeTypeVars returns the set of free type variable names in a type.
func FreeTypeVars(t Type) map[string]bool {
	fv := make(map[string]bool)
	freeTypeVarsAcc(t, fv)
	return fv
}

func freeTypeVarsAcc(t Type, acc map[string]bool) {
	if t == nil {
		return
	}
	switch t := t.(type) {
	case *TInt, *TBool, *TString:
		// no free vars
	case *TFun:
		freeTypeVarsAcc(t.Param, acc)
		freeTypeVarsAcc(t.Result, acc)
	case *TList:
		freeTypeVarsAcc(t.Elem, acc)
	case *TTuple:
		freeTypeVarsAcc(t.Fst, acc)
		freeTypeVarsAcc(t.Snd, acc)
	case *TVar:
		acc[t.Name] = true
	}
}

// SubstType replaces type variables according to the given substitution map.
func SubstType(t Type, subst map[string]Type) Type {
	if t == nil {
		return nil
	}
	switch t := t.(type) {
	case *TInt, *TBool, *TString:
		return t
	case *TFun:
		return &TFun{
			Param:  SubstType(t.Param, subst),
			Result: SubstType(t.Result, subst),
		}
	case *TList:
		return &TList{Elem: SubstType(t.Elem, subst)}
	case *TTuple:
		return &TTuple{
			Fst: SubstType(t.Fst, subst),
			Snd: SubstType(t.Snd, subst),
		}
	case *TVar:
		if s, ok := subst[t.Name]; ok {
			return s
		}
		return t
	}
	return t
}

// Helper constructors for types.
func MkTFun(params ...Type) Type {
	if len(params) < 2 {
		panic(fmt.Sprintf("MkTFun needs at least 2 types, got %d", len(params)))
	}
	// Build right-associated: a -> b -> c = a -> (b -> c)
	result := params[len(params)-1]
	for i := len(params) - 2; i >= 0; i-- {
		result = &TFun{Param: params[i], Result: result}
	}
	return result
}
