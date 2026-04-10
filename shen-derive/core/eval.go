package core

import (
	"fmt"
	"strconv"
)

// Value represents a runtime value in the evaluator.
type Value interface {
	valNode()
	String() string
}

type IntVal int64
type FloatVal float64
type BoolVal bool
type StringVal string
type ListVal []Value
type TupleVal struct{ Fst, Snd Value }
type ClosureVal struct {
	Env   *Env
	Param string
	Body  Sexpr
}
type PrimPartial struct {
	Op   string
	Args []Value
}

// BuiltinFn is a host-language (Go) function exposed to the evaluator.
// Used to inject domain-specific operations (e.g., field accessors) without
// extending the built-in primitive table. The function is called once per
// argument via Apply; chain them for multi-argument operations by returning
// another BuiltinFn.
type BuiltinFn struct {
	Name string
	Fn   func(Value) (Value, error)
}

func (IntVal) valNode()       {}
func (FloatVal) valNode()     {}
func (BoolVal) valNode()      {}
func (StringVal) valNode()    {}
func (ListVal) valNode()      {}
func (*TupleVal) valNode()    {}
func (*ClosureVal) valNode()  {}
func (*PrimPartial) valNode() {}
func (*BuiltinFn) valNode()   {}

func (v IntVal) String() string { return fmt.Sprintf("%d", int64(v)) }
func (v FloatVal) String() string {
	return strconv.FormatFloat(float64(v), 'g', -1, 64)
}
func (v BoolVal) String() string { return fmt.Sprintf("%v", bool(v)) }
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
func (v *BuiltinFn) String() string   { return fmt.Sprintf("<builtin:%s>", v.Name) }

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

// primArity returns the arity of a built-in operation.
func primArity(op string) int {
	switch op {
	case "not", "fst", "snd", "concat":
		return 1
	case "+", "-", "*", "/", "%", "=", "!=", "<", "<=", ">", ">=", "and", "or", "cons", "map", "filter", "unfoldr":
		return 2
	case "foldr", "foldl", "scanl", "compose":
		return 3
	default:
		return 0
	}
}

// isBuiltin returns true if the symbol names a built-in operation.
func isBuiltin(name string) bool {
	return primArity(name) > 0
}

// Eval evaluates an s-expression in the given environment.
func Eval(env *Env, sexpr Sexpr) (Value, error) {
	switch s := sexpr.(type) {
	case *Atom:
		switch s.Kind {
		case AtomInt:
			n, err := strconv.ParseInt(s.Val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("bad int literal: %s", s.Val)
			}
			return IntVal(n), nil
		case AtomFloat:
			f, err := strconv.ParseFloat(s.Val, 64)
			if err != nil {
				return nil, fmt.Errorf("bad float literal: %s", s.Val)
			}
			return FloatVal(f), nil
		case AtomBool:
			return BoolVal(s.Val == "true"), nil
		case AtomString:
			return StringVal(s.Val), nil
		case AtomSymbol:
			if s.Val == "nil" {
				return ListVal(nil), nil
			}
			if isBuiltin(s.Val) {
				return &PrimPartial{Op: s.Val, Args: nil}, nil
			}
			v, ok := env.Lookup(s.Val)
			if !ok {
				return nil, fmt.Errorf("unbound variable %q", s.Val)
			}
			return v, nil
		}

	case *List:
		if len(s.Elems) == 0 {
			return ListVal(nil), nil
		}

		head := HeadSym(s)

		// Special forms
		switch head {
		case "lambda":
			// (lambda Param Body)
			if len(s.Elems) != 3 {
				return nil, fmt.Errorf("lambda: expected 3 elements, got %d", len(s.Elems))
			}
			param, ok := SymName(s.Elems[1])
			if !ok {
				return nil, fmt.Errorf("lambda: param must be a symbol")
			}
			return &ClosureVal{Env: env, Param: param, Body: s.Elems[2]}, nil

		case "let":
			// (let Name Val Body)
			if len(s.Elems) != 4 {
				return nil, fmt.Errorf("let: expected 4 elements, got %d", len(s.Elems))
			}
			name, ok := SymName(s.Elems[1])
			if !ok {
				return nil, fmt.Errorf("let: name must be a symbol")
			}
			val, err := Eval(env, s.Elems[2])
			if err != nil {
				return nil, err
			}
			return Eval(env.Extend(name, val), s.Elems[3])

		case "if":
			// (if Cond Then Else)
			if len(s.Elems) != 4 {
				return nil, fmt.Errorf("if: expected 4 elements, got %d", len(s.Elems))
			}
			cv, err := Eval(env, s.Elems[1])
			if err != nil {
				return nil, err
			}
			b, ok := cv.(BoolVal)
			if !ok {
				return nil, fmt.Errorf("if: condition must be Bool, got %T (%s)", cv, cv)
			}
			if bool(b) {
				return Eval(env, s.Elems[2])
			}
			return Eval(env, s.Elems[3])

		case "@p":
			// (@p a b) — tuple
			if len(s.Elems) != 3 {
				return nil, fmt.Errorf("@p: expected 3 elements, got %d", len(s.Elems))
			}
			fst, err := Eval(env, s.Elems[1])
			if err != nil {
				return nil, err
			}
			snd, err := Eval(env, s.Elems[2])
			if err != nil {
				return nil, err
			}
			return &TupleVal{Fst: fst, Snd: snd}, nil
		}

		// Evaluate head and all args, then apply
		fv, err := Eval(env, s.Elems[0])
		if err != nil {
			return nil, err
		}
		// Apply args one at a time (curried)
		result := fv
		for _, arg := range s.Elems[1:] {
			av, err := Eval(env, arg)
			if err != nil {
				return nil, err
			}
			result, err = Apply(result, av)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	}

	return nil, fmt.Errorf("cannot evaluate: %T", sexpr)
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
		arity := primArity(fv.Op)
		if len(newArgs) < arity {
			return &PrimPartial{Op: fv.Op, Args: newArgs}, nil
		}
		return execPrim(fv.Op, newArgs)

	case *BuiltinFn:
		return fv.Fn(arg)
	}

	return nil, fmt.Errorf("cannot apply non-function value: %s (%T)", f, f)
}

// execPrim executes a fully-applied primitive operation.
func execPrim(op string, args []Value) (Value, error) {
	switch op {
	// Arithmetic: promote int+int→int, anything-with-float→float.
	case "+":
		return numBinOp(args, "+",
			func(a, b int64) (int64, error) { return a + b, nil },
			func(a, b float64) (float64, error) { return a + b, nil })
	case "-":
		return numBinOp(args, "-",
			func(a, b int64) (int64, error) { return a - b, nil },
			func(a, b float64) (float64, error) { return a - b, nil })
	case "*":
		return numBinOp(args, "*",
			func(a, b int64) (int64, error) { return a * b, nil },
			func(a, b float64) (float64, error) { return a * b, nil })
	case "/":
		return numBinOp(args, "/",
			func(a, b int64) (int64, error) {
				if b == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				return a / b, nil
			},
			func(a, b float64) (float64, error) {
				if b == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				return a / b, nil
			})
	case "%":
		// Modulo is integer-only.
		a, err := asInt(args[0])
		if err != nil {
			return nil, fmt.Errorf("%%: %w", err)
		}
		b, err := asInt(args[1])
		if err != nil {
			return nil, fmt.Errorf("%%: %w", err)
		}
		if b == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return IntVal(a % b), nil

	// Comparison
	case "=":
		return BoolVal(valEqual(args[0], args[1])), nil
	case "!=":
		return BoolVal(!valEqual(args[0], args[1])), nil
	case "<":
		return numCmp(args, "<", func(a, b float64) bool { return a < b })
	case "<=":
		return numCmp(args, "<=", func(a, b float64) bool { return a <= b })
	case ">":
		return numCmp(args, ">", func(a, b float64) bool { return a > b })
	case ">=":
		return numCmp(args, ">=", func(a, b float64) bool { return a >= b })

	// Boolean
	case "and":
		a, err := asBool(args[0])
		if err != nil {
			return nil, fmt.Errorf("and: %w", err)
		}
		b, err := asBool(args[1])
		if err != nil {
			return nil, fmt.Errorf("and: %w", err)
		}
		return BoolVal(a && b), nil
	case "or":
		a, err := asBool(args[0])
		if err != nil {
			return nil, fmt.Errorf("or: %w", err)
		}
		b, err := asBool(args[1])
		if err != nil {
			return nil, fmt.Errorf("or: %w", err)
		}
		return BoolVal(a || b), nil
	case "not":
		a, err := asBool(args[0])
		if err != nil {
			return nil, fmt.Errorf("not: %w", err)
		}
		return BoolVal(!a), nil

	// List operations
	case "cons":
		xs, err := asList(args[1])
		if err != nil {
			return nil, fmt.Errorf("cons: %w", err)
		}
		result := make(ListVal, len(xs)+1)
		result[0] = args[0]
		copy(result[1:], xs)
		return result, nil

	case "concat":
		xss, err := asList(args[0])
		if err != nil {
			return nil, fmt.Errorf("concat: %w", err)
		}
		var result ListVal
		for _, x := range xss {
			inner, err := asList(x)
			if err != nil {
				return nil, fmt.Errorf("concat: %w", err)
			}
			result = append(result, inner...)
		}
		return result, nil

	case "fst":
		t, err := asTuple(args[0])
		if err != nil {
			return nil, fmt.Errorf("fst: %w", err)
		}
		return t.Fst, nil
	case "snd":
		t, err := asTuple(args[0])
		if err != nil {
			return nil, fmt.Errorf("snd: %w", err)
		}
		return t.Snd, nil

	// Map: map f xs
	case "map":
		xs, err := asList(args[1])
		if err != nil {
			return nil, fmt.Errorf("map: %w", err)
		}
		f := args[0]
		result := make(ListVal, len(xs))
		for i, x := range xs {
			v, err := Apply(f, x)
			if err != nil {
				return nil, fmt.Errorf("map: %w", err)
			}
			result[i] = v
		}
		return result, nil

	// Foldr: foldr f e xs
	case "foldr":
		xs, err := asList(args[2])
		if err != nil {
			return nil, fmt.Errorf("foldr: %w", err)
		}
		f, e := args[0], args[1]
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

	// Foldl: foldl f e xs
	case "foldl":
		xs, err := asList(args[2])
		if err != nil {
			return nil, fmt.Errorf("foldl: %w", err)
		}
		f, e := args[0], args[1]
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

	// Scanl: scanl f e xs
	case "scanl":
		xs, err := asList(args[2])
		if err != nil {
			return nil, fmt.Errorf("scanl: %w", err)
		}
		f, e := args[0], args[1]
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
	case "filter":
		xs, err := asList(args[1])
		if err != nil {
			return nil, fmt.Errorf("filter: %w", err)
		}
		p := args[0]
		var result ListVal
		for _, x := range xs {
			pv, err := Apply(p, x)
			if err != nil {
				return nil, fmt.Errorf("filter: %w", err)
			}
			b, err := asBool(pv)
			if err != nil {
				return nil, fmt.Errorf("filter: %w", err)
			}
			if b {
				result = append(result, x)
			}
		}
		return ListVal(result), nil

	// Unfoldr: unfoldr f seed
	case "unfoldr":
		f, seed := args[0], args[1]
		var result ListVal
		for i := 0; i < 10000; i++ {
			pair, err := Apply(f, seed)
			if err != nil {
				return nil, fmt.Errorf("unfoldr: %w", err)
			}
			tp, err := asTuple(pair)
			if err != nil {
				return nil, fmt.Errorf("unfoldr: %w", err)
			}
			cont, err := asBool(tp.Fst)
			if err != nil {
				return nil, fmt.Errorf("unfoldr: %w", err)
			}
			if !cont {
				break
			}
			inner, err := asTuple(tp.Snd)
			if err != nil {
				return nil, fmt.Errorf("unfoldr: %w", err)
			}
			result = append(result, inner.Fst)
			seed = inner.Snd
		}
		return result, nil

	// Compose: compose f g x
	case "compose":
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

func asInt(v Value) (int64, error) {
	iv, ok := v.(IntVal)
	if !ok {
		return 0, fmt.Errorf("expected Int, got %T (%s)", v, v)
	}
	return int64(iv), nil
}

// asNum accepts IntVal or FloatVal and returns a float64 view.
func asNum(v Value) (float64, bool) {
	return AsNum(v)
}

// AsNum is the exported form of asNum, used by packages that need to
// interpret a Value as a number (e.g. the verify harness).
func AsNum(v Value) (float64, bool) {
	switch x := v.(type) {
	case IntVal:
		return float64(x), true
	case FloatVal:
		return float64(x), true
	}
	return 0, false
}

// numBinOp implements a numeric binary operator that promotes int+int to int
// but promotes anything-involving-float to float.
func numBinOp(
	args []Value,
	name string,
	intOp func(int64, int64) (int64, error),
	floatOp func(float64, float64) (float64, error),
) (Value, error) {
	ai, aIsInt := args[0].(IntVal)
	bi, bIsInt := args[1].(IntVal)
	if aIsInt && bIsInt {
		r, err := intOp(int64(ai), int64(bi))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return IntVal(r), nil
	}
	a, aok := asNum(args[0])
	if !aok {
		return nil, fmt.Errorf("%s: expected number, got %T", name, args[0])
	}
	b, bok := asNum(args[1])
	if !bok {
		return nil, fmt.Errorf("%s: expected number, got %T", name, args[1])
	}
	r, err := floatOp(a, b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return FloatVal(r), nil
}

// numCmp implements a numeric comparison that promotes both sides to float64.
func numCmp(args []Value, name string, pred func(float64, float64) bool) (Value, error) {
	a, aok := asNum(args[0])
	if !aok {
		return nil, fmt.Errorf("%s: expected number, got %T", name, args[0])
	}
	b, bok := asNum(args[1])
	if !bok {
		return nil, fmt.Errorf("%s: expected number, got %T", name, args[1])
	}
	return BoolVal(pred(a, b)), nil
}

func asBool(v Value) (bool, error) {
	bv, ok := v.(BoolVal)
	if !ok {
		return false, fmt.Errorf("expected Bool, got %T (%s)", v, v)
	}
	return bool(bv), nil
}

func asList(v Value) (ListVal, error) {
	lv, ok := v.(ListVal)
	if !ok {
		return nil, fmt.Errorf("expected List, got %T (%s)", v, v)
	}
	return lv, nil
}

func asTuple(v Value) (*TupleVal, error) {
	tv, ok := v.(*TupleVal)
	if !ok {
		return nil, fmt.Errorf("expected Tuple, got %T (%s)", v, v)
	}
	return tv, nil
}

// valEqual compares two values for structural equality.
// Numbers compare by float value, so IntVal(1) == FloatVal(1.0).
func valEqual(a, b Value) bool {
	if an, aok := asNum(a); aok {
		if bn, bok := asNum(b); bok {
			return an == bn
		}
	}
	switch av := a.(type) {
	case IntVal:
		bv, ok := b.(IntVal)
		return ok && av == bv
	case FloatVal:
		bv, ok := b.(FloatVal)
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
