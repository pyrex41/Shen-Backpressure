package core

import "fmt"

// Value represents a runtime value in the evaluator.
type Value interface {
	valNode()
	String() string
}

type IntVal int64
type BoolVal bool
type StringVal string
type ListVal []Value
type TupleVal struct{ Fst, Snd Value }
type ClosureVal struct {
	Env   *Env
	Param string
	Body  Term
}
type PrimPartial struct {
	Op   PrimOp
	Args []Value
}

func (IntVal) valNode()       {}
func (BoolVal) valNode()      {}
func (StringVal) valNode()    {}
func (ListVal) valNode()      {}
func (*TupleVal) valNode()    {}
func (*ClosureVal) valNode()  {}
func (*PrimPartial) valNode() {}

func (v IntVal) String() string    { return fmt.Sprintf("%d", int64(v)) }
func (v BoolVal) String() string   { return fmt.Sprintf("%v", bool(v)) }
func (v StringVal) String() string { return fmt.Sprintf("%q", string(v)) }

func (v ListVal) String() string {
	s := "["
	for i, e := range v {
		if i > 0 {
			s += ", "
		}
		s += e.String()
	}
	return s + "]"
}

func (v *TupleVal) String() string {
	return "(" + v.Fst.String() + ", " + v.Snd.String() + ")"
}

func (v *ClosureVal) String() string  { return "<closure>" }
func (v *PrimPartial) String() string { return fmt.Sprintf("<prim:%s/%d>", v.Op, len(v.Args)) }

// Env is a linked-list environment mapping variable names to values.
type Env struct {
	name   string
	val    Value
	parent *Env
}

func EmptyEnv() *Env { return nil }

func (e *Env) Extend(name string, val Value) *Env {
	return &Env{name: name, val: val, parent: e}
}

func (e *Env) Lookup(name string) (Value, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.name == name {
			return cur.val, true
		}
	}
	return nil, false
}

// Eval evaluates a term in the given environment using big-step semantics.
func Eval(env *Env, term Term) (Value, error) {
	switch t := term.(type) {
	case *Var:
		v, ok := env.Lookup(t.Name)
		if !ok {
			return nil, fmt.Errorf("%s: unbound variable %q", t.P, t.Name)
		}
		return v, nil

	case *Lam:
		return &ClosureVal{Env: env, Param: t.Param, Body: t.Body}, nil

	case *App:
		fv, err := Eval(env, t.Func)
		if err != nil {
			return nil, err
		}
		av, err := Eval(env, t.Arg)
		if err != nil {
			return nil, err
		}
		return Apply(fv, av)

	case *Let:
		vv, err := Eval(env, t.Bound)
		if err != nil {
			return nil, err
		}
		return Eval(env.Extend(t.Name, vv), t.Body)

	case *Lit:
		switch t.Kind {
		case LitInt:
			return IntVal(t.IntVal), nil
		case LitBool:
			return BoolVal(t.BoolVal), nil
		case LitString:
			return StringVal(t.StrVal), nil
		}

	case *ListLit:
		vs := make(ListVal, len(t.Elems))
		for i, e := range t.Elems {
			v, err := Eval(env, e)
			if err != nil {
				return nil, err
			}
			vs[i] = v
		}
		return vs, nil

	case *TupleLit:
		fv, err := Eval(env, t.Fst)
		if err != nil {
			return nil, err
		}
		sv, err := Eval(env, t.Snd)
		if err != nil {
			return nil, err
		}
		return &TupleVal{Fst: fv, Snd: sv}, nil

	case *IfExpr:
		cv, err := Eval(env, t.Cond)
		if err != nil {
			return nil, err
		}
		b, ok := cv.(BoolVal)
		if !ok {
			return nil, fmt.Errorf("%s: if condition must be Bool, got %s", t.P, cv)
		}
		if bool(b) {
			return Eval(env, t.Then)
		}
		return Eval(env, t.Else)

	case *Prim:
		if t.Op == PrimNil {
			return ListVal(nil), nil
		}
		return &PrimPartial{Op: t.Op, Args: nil}, nil
	}

	return nil, fmt.Errorf("cannot evaluate: %T", term)
}

// Apply applies a function value to an argument value.
func Apply(f Value, arg Value) (Value, error) {
	switch fv := f.(type) {
	case *ClosureVal:
		return Eval(fv.Env.Extend(fv.Param, arg), fv.Body)

	case *PrimPartial:
		newArgs := make([]Value, len(fv.Args)+1)
		copy(newArgs, fv.Args)
		newArgs[len(fv.Args)] = arg
		arity := PrimArity(fv.Op)
		if len(newArgs) < arity {
			return &PrimPartial{Op: fv.Op, Args: newArgs}, nil
		}
		return ExecPrim(fv.Op, newArgs)
	}

	return nil, fmt.Errorf("cannot apply non-function value: %s (%T)", f, f)
}

// ExecPrim executes a fully-applied primitive operation.
func ExecPrim(op PrimOp, args []Value) (Value, error) {
	switch op {
	// Arithmetic
	case PrimAdd:
		return IntVal(asInt(args[0]) + asInt(args[1])), nil
	case PrimSub:
		return IntVal(asInt(args[0]) - asInt(args[1])), nil
	case PrimMul:
		return IntVal(asInt(args[0]) * asInt(args[1])), nil
	case PrimDiv:
		b := asInt(args[1])
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return IntVal(asInt(args[0]) / b), nil
	case PrimMod:
		b := asInt(args[1])
		if b == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return IntVal(asInt(args[0]) % b), nil
	case PrimNeg:
		return IntVal(-asInt(args[0])), nil

	// Comparison
	case PrimEq:
		return BoolVal(valEqual(args[0], args[1])), nil
	case PrimNeq:
		return BoolVal(!valEqual(args[0], args[1])), nil
	case PrimLt:
		return BoolVal(asInt(args[0]) < asInt(args[1])), nil
	case PrimLe:
		return BoolVal(asInt(args[0]) <= asInt(args[1])), nil
	case PrimGt:
		return BoolVal(asInt(args[0]) > asInt(args[1])), nil
	case PrimGe:
		return BoolVal(asInt(args[0]) >= asInt(args[1])), nil

	// Boolean
	case PrimAnd:
		return BoolVal(asBool(args[0]) && asBool(args[1])), nil
	case PrimOr:
		return BoolVal(asBool(args[0]) || asBool(args[1])), nil
	case PrimNot:
		return BoolVal(!asBool(args[0])), nil

	// List operations
	case PrimCons:
		xs := asList(args[1])
		result := make(ListVal, len(xs)+1)
		result[0] = args[0]
		copy(result[1:], xs)
		return result, nil

	case PrimConcat:
		xss := asList(args[0])
		var result ListVal
		for _, xs := range xss {
			result = append(result, asList(xs)...)
		}
		return result, nil

	case PrimFst:
		return asTuple(args[0]).Fst, nil
	case PrimSnd:
		return asTuple(args[0]).Snd, nil

	// Map: map f xs
	case PrimMap:
		f, xs := args[0], asList(args[1])
		result := make(ListVal, len(xs))
		for i, x := range xs {
			v, err := Apply(f, x)
			if err != nil {
				return nil, fmt.Errorf("map: %w", err)
			}
			result[i] = v
		}
		return result, nil

	// Foldr: foldr f e xs = f x1 (f x2 (... (f xn e)))
	case PrimFoldr:
		f, e, xs := args[0], args[1], asList(args[2])
		acc := e
		for i := len(xs) - 1; i >= 0; i-- {
			partial, err := Apply(f, xs[i])
			if err != nil {
				return nil, fmt.Errorf("foldr: %w", err)
			}
			acc, err = Apply(partial, acc)
			if err != nil {
				return nil, fmt.Errorf("foldr: %w", err)
			}
		}
		return acc, nil

	// Foldl: foldl f e xs = foldl f (f e x1) xs'
	case PrimFoldl:
		f, e, xs := args[0], args[1], asList(args[2])
		acc := e
		for _, x := range xs {
			partial, err := Apply(f, acc)
			if err != nil {
				return nil, fmt.Errorf("foldl: %w", err)
			}
			acc, err = Apply(partial, x)
			if err != nil {
				return nil, fmt.Errorf("foldl: %w", err)
			}
		}
		return acc, nil

	// Scanl: scanl f e xs returns [e, f e x1, f (f e x1) x2, ...]
	case PrimScanl:
		f, e, xs := args[0], args[1], asList(args[2])
		result := make(ListVal, 0, len(xs)+1)
		result = append(result, e)
		acc := e
		for _, x := range xs {
			partial, err := Apply(f, acc)
			if err != nil {
				return nil, fmt.Errorf("scanl: %w", err)
			}
			acc, err = Apply(partial, x)
			if err != nil {
				return nil, fmt.Errorf("scanl: %w", err)
			}
			result = append(result, acc)
		}
		return result, nil

	// Filter: filter p xs
	case PrimFilter:
		p, xs := args[0], asList(args[1])
		var result ListVal
		for _, x := range xs {
			pv, err := Apply(p, x)
			if err != nil {
				return nil, fmt.Errorf("filter: %w", err)
			}
			if asBool(pv) {
				result = append(result, x)
			}
		}
		return ListVal(result), nil

	// Unfoldr: unfoldr f seed where f : b -> (Bool, (a, b))
	// Returns [] when the Bool is False; otherwise cons the a and recur on b.
	case PrimUnfoldr:
		f, seed := args[0], args[1]
		var result ListVal
		for i := 0; i < 10000; i++ { // safety limit
			pair, err := Apply(f, seed)
			if err != nil {
				return nil, fmt.Errorf("unfoldr: %w", err)
			}
			tp := asTuple(pair)
			if !asBool(tp.Fst) {
				break
			}
			inner := asTuple(tp.Snd)
			result = append(result, inner.Fst)
			seed = inner.Snd
		}
		return result, nil

	// Compose: compose f g x = f (g x)
	case PrimCompose:
		f, g, x := args[0], args[1], args[2]
		gx, err := Apply(g, x)
		if err != nil {
			return nil, fmt.Errorf("compose: %w", err)
		}
		return Apply(f, gx)
	}

	return nil, fmt.Errorf("unknown primitive: %s", op)
}

// --- Value coercion helpers ---

func asInt(v Value) int64 {
	return int64(v.(IntVal))
}

func asBool(v Value) bool {
	return bool(v.(BoolVal))
}

func asList(v Value) ListVal {
	return v.(ListVal)
}

func asTuple(v Value) *TupleVal {
	return v.(*TupleVal)
}

// valEqual compares two values for structural equality.
func valEqual(a, b Value) bool {
	switch av := a.(type) {
	case IntVal:
		bv, ok := b.(IntVal)
		return ok && av == bv
	case BoolVal:
		bv, ok := b.(BoolVal)
		return ok && av == bv
	case StringVal:
		bv, ok := b.(StringVal)
		return ok && av == bv
	case ListVal:
		bv, ok := b.(ListVal)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case *TupleVal:
		bv, ok := b.(*TupleVal)
		return ok && valEqual(av.Fst, bv.Fst) && valEqual(av.Snd, bv.Snd)
	default:
		return false
	}
}
