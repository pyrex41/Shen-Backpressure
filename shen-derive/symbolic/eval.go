package symbolic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// defaultMaxCallDepth bounds recursion through (define …) references.
// List recursion terminates on its own because list spines are concrete
// (see the package doc), but a define that recurses on a number would
// not, so the evaluator refuses rather than looping.
const defaultMaxCallDepth = 128

// SDefineV is a partially applied reference to a sibling (define …).
// Resolving define calls through a value (rather than a host closure)
// keeps the evaluator's forking machinery in one place.
type SDefineV struct {
	Def  *specfile.Define
	Args []SymVal
}

func (*SDefineV) symValNode() {}
func (v *SDefineV) String() string {
	return fmt.Sprintf("<define:%s/%d>", v.Def.Name, len(v.Args))
}

// symExec is one symbolic execution of a spec body: a replay of the
// decision sequence in `decisions`, extended with fresh decisions
// (defaulting to the true branch) once the prefix runs out.
//
// The enumerator drives this in the classic generational style: run
// with a prefix, record which decisions were actually taken, then
// enqueue one new prefix per decision beyond the replayed part with
// that decision flipped. That covers the whole decision tree without
// needing continuations inside a recursive Go evaluator.
type symExec struct {
	decisions []bool
	k         int    // index of the next decision point
	taken     []bool // the full decision sequence this run produced
	pc        []Term // accumulated path condition
	callDepth int
	maxCallDepth int
	// base is the environment clause patterns bind on top of: `val`,
	// the field accessors, and the sibling define references.
	base *SEnv
}

// branch resolves a symbolic condition to a concrete direction,
// recording the choice and the corresponding constraint. A condition
// that folded to a literal is not a decision point: it contributes
// nothing to the path condition and does not consume a decision index,
// so indices stay aligned across replays.
func (e *symExec) branch(cond Term) bool {
	if b, ok := BoolLit(cond); ok {
		return b
	}
	take := true
	if e.k < len(e.decisions) {
		take = e.decisions[e.k]
	}
	e.k++
	e.taken = append(e.taken, take)
	if take {
		e.pc = append(e.pc, cond)
	} else {
		e.pc = append(e.pc, Not(cond))
	}
	return take
}

// --- primitives ---

func primArity(op string) int {
	switch op {
	case "not", "fst", "snd", "concat", "head", "tail":
		return 1
	case "+", "-", "*", "/", "%", "=", "!=", "<", "<=", ">", ">=", "and", "or", "cons", "map", "filter", "unfoldr":
		return 2
	case "foldr", "foldl", "scanl", "compose":
		return 3
	}
	return 0
}

func isBuiltin(name string) bool { return primArity(name) > 0 }

// baseSymEnv mirrors verify.buildBaseEnv for the symbolic evaluator:
// `val` as identity, one accessor per composite field, and one
// reference per sibling define so bodies can call each other by name.
func baseSymEnv(tt *specfile.TypeTable, defines []*specfile.Define) *SEnv {
	env := EmptySEnv()
	env = env.Extend("val", &SBuiltinV{
		Name: "val",
		Fn:   func(v SymVal) (SymVal, error) { return v, nil },
	})

	if tt != nil {
		registered := map[string]bool{}
		register := func(name string, idx int) {
			if registered[name] {
				return
			}
			registered[name] = true
			i := idx
			env = env.Extend(name, &SBuiltinV{
				Name: name,
				Fn: func(v SymVal) (SymVal, error) {
					lv, ok := v.(*SListV)
					if !ok {
						return nil, fmt.Errorf("field accessor %q: not a composite value", name)
					}
					if i < 0 || i >= len(lv.Elems) {
						return nil, fmt.Errorf("field accessor %q: index %d out of range", name, i)
					}
					return lv.Elems[i], nil
				},
			})
		}
		for _, entry := range tt.Entries {
			for _, f := range entry.Fields {
				register(strings.ToLower(f.ShenName), f.Index)
				register(f.ShenName, f.Index)
			}
		}
	}

	for _, d := range defines {
		if d == nil {
			continue
		}
		env = env.Extend(d.Name, &SDefineV{Def: d})
	}
	return env
}

// eval symbolically evaluates an s-expression.
func (e *symExec) eval(env *SEnv, sexpr core.Sexpr) (SymVal, error) {
	switch s := sexpr.(type) {
	case *core.Atom:
		return e.evalAtom(env, s)
	case *core.List:
		return e.evalList(env, s)
	}
	return nil, fmt.Errorf("cannot symbolically evaluate: %T", sexpr)
}

func (e *symExec) evalAtom(env *SEnv, s *core.Atom) (SymVal, error) {
	switch s.Kind {
	case core.AtomInt:
		n, err := strconv.ParseInt(s.Val, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad int literal: %s", s.Val)
		}
		return &SNum{T: R(float64(n)), ShenType: "number"}, nil
	case core.AtomFloat:
		f, err := strconv.ParseFloat(s.Val, 64)
		if err != nil {
			return nil, fmt.Errorf("bad float literal: %s", s.Val)
		}
		return &SNum{T: R(f), ShenType: "number"}, nil
	case core.AtomBool:
		return &SBoolV{T: B(s.Val == "true"), ShenType: "boolean"}, nil
	case core.AtomString:
		return &SStrV{T: S(s.Val), ShenType: "string"}, nil
	case core.AtomSymbol:
		if s.Val == "nil" {
			return &SListV{}, nil
		}
		if v, ok := env.Lookup(s.Val); ok {
			return v, nil
		}
		if isBuiltin(s.Val) {
			return &SPrimV{Op: s.Val}, nil
		}
		return nil, fmt.Errorf("unbound variable %q", s.Val)
	}
	return nil, fmt.Errorf("unsupported atom kind %v", s.Kind)
}

func (e *symExec) evalList(env *SEnv, s *core.List) (SymVal, error) {
	if len(s.Elems) == 0 {
		return &SListV{}, nil
	}
	switch core.HeadSym(s) {
	case "lambda":
		if len(s.Elems) != 3 {
			return nil, fmt.Errorf("lambda: expected 3 elements, got %d", len(s.Elems))
		}
		param, ok := core.SymName(s.Elems[1])
		if !ok {
			return nil, fmt.Errorf("lambda: param must be a symbol")
		}
		return &SClosureV{Env: env, Param: param, Body: s.Elems[2]}, nil

	case "let":
		if len(s.Elems) != 4 {
			return nil, fmt.Errorf("let: expected 4 elements, got %d", len(s.Elems))
		}
		name, ok := core.SymName(s.Elems[1])
		if !ok {
			return nil, fmt.Errorf("let: name must be a symbol")
		}
		val, err := e.eval(env, s.Elems[2])
		if err != nil {
			return nil, err
		}
		return e.eval(env.Extend(name, val), s.Elems[3])

	case "if":
		if len(s.Elems) != 4 {
			return nil, fmt.Errorf("if: expected 4 elements, got %d", len(s.Elems))
		}
		cv, err := e.eval(env, s.Elems[1])
		if err != nil {
			return nil, err
		}
		cb, ok := cv.(*SBoolV)
		if !ok {
			return nil, fmt.Errorf("if: condition must be Bool, got %s", cv.String())
		}
		if e.branch(cb.T) {
			return e.eval(env, s.Elems[2])
		}
		return e.eval(env, s.Elems[3])

	case "cond":
		// (cond (Test1 Val1) (Test2 Val2) …). Each test is a decision
		// point; falling off the end is an error, exactly as Shen's
		// own `cond` raises.
		for _, arm := range s.Elems[1:] {
			pair := core.ListElems(arm)
			if len(pair) != 2 {
				return nil, fmt.Errorf("cond: each arm must be (Test Value)")
			}
			tv, err := e.eval(env, pair[0])
			if err != nil {
				return nil, err
			}
			tb, ok := tv.(*SBoolV)
			if !ok {
				return nil, fmt.Errorf("cond: test must be Bool, got %s", tv.String())
			}
			if e.branch(tb.T) {
				return e.eval(env, pair[1])
			}
		}
		return nil, fmt.Errorf("cond: no arm matched")

	case "@p":
		if len(s.Elems) != 3 {
			return nil, fmt.Errorf("@p: expected 3 elements, got %d", len(s.Elems))
		}
		fst, err := e.eval(env, s.Elems[1])
		if err != nil {
			return nil, err
		}
		snd, err := e.eval(env, s.Elems[2])
		if err != nil {
			return nil, err
		}
		return &STupleV{Fst: fst, Snd: snd}, nil
	}

	fv, err := e.eval(env, s.Elems[0])
	if err != nil {
		return nil, err
	}
	result := fv
	for _, arg := range s.Elems[1:] {
		av, err := e.eval(env, arg)
		if err != nil {
			return nil, err
		}
		result, err = e.apply(result, av)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// apply applies a symbolic function value to one argument.
func (e *symExec) apply(f SymVal, arg SymVal) (SymVal, error) {
	switch fv := f.(type) {
	case *SClosureV:
		return e.eval(fv.Env.Extend(fv.Param, arg), fv.Body)

	case *SPrimV:
		args := append(append([]SymVal{}, fv.Args...), arg)
		if len(args) < primArity(fv.Op) {
			return &SPrimV{Op: fv.Op, Args: args}, nil
		}
		return e.execPrim(fv.Op, args)

	case *SBuiltinV:
		return fv.Fn(arg)

	case *SDefineV:
		args := append(append([]SymVal{}, fv.Args...), arg)
		if len(args) < fv.Def.Arity() {
			return &SDefineV{Def: fv.Def, Args: args}, nil
		}
		return e.evalDefine(fv.Def, args)
	}
	return nil, fmt.Errorf("cannot apply non-function value: %s", f.String())
}

// evalDefine dispatches on a define's clauses, forking at every
// pattern-match test and `where` guard that is not already decided.
func (e *symExec) evalDefine(def *specfile.Define, args []SymVal) (SymVal, error) {
	max := e.maxCallDepth
	if max <= 0 {
		max = defaultMaxCallDepth
	}
	if e.callDepth >= max {
		return nil, fmt.Errorf("call depth %d exceeded evaluating %s "+
			"(recursion that does not shrink a list is outside the bounded fragment)", max, def.Name)
	}
	e.callDepth++
	defer func() { e.callDepth-- }()

	if len(def.Clauses) == 0 {
		return nil, fmt.Errorf("define %s: no clauses", def.Name)
	}
	for i, cl := range def.Clauses {
		if len(cl.Patterns) != len(args) {
			return nil, fmt.Errorf("define %s clause %d: %d patterns vs %d args",
				def.Name, i, len(cl.Patterns), len(args))
		}
		env, cond, err := e.matchClause(cl.Patterns, args)
		if err != nil {
			return nil, fmt.Errorf("define %s clause %d: %w", def.Name, i, err)
		}
		if !e.branch(cond) {
			continue
		}
		if cl.Guard != nil {
			g, err := e.eval(env, cl.Guard)
			if err != nil {
				return nil, fmt.Errorf("define %s clause %d: guard: %w", def.Name, i, err)
			}
			gb, ok := g.(*SBoolV)
			if !ok {
				return nil, fmt.Errorf("define %s clause %d: guard returned %s, expected Bool",
					def.Name, i, g.String())
			}
			if !e.branch(gb.T) {
				continue
			}
		}
		return e.eval(env, cl.Body)
	}
	return nil, fmt.Errorf("define %s: non-exhaustive clauses (no clause matched on this path)", def.Name)
}

// matchClause matches every (pattern, argument) pair, returning the
// extended environment and the condition under which the clause
// applies. A structurally impossible match yields B(false).
func (e *symExec) matchClause(patterns []core.Sexpr, args []SymVal) (*SEnv, Term, error) {
	env := e.base
	conds := make([]Term, 0, len(patterns))
	for i, p := range patterns {
		var err error
		var c Term
		env, c, err = matchSym(p, args[i], env)
		if err != nil {
			return nil, nil, err
		}
		if b, ok := BoolLit(c); ok && !b {
			return nil, B(false), nil
		}
		conds = append(conds, c)
	}
	return env, And(conds...), nil
}

// --- primitive execution ---

func (e *symExec) execPrim(op string, args []SymVal) (SymVal, error) {
	switch op {
	case "+", "-", "*", "/":
		a, err := asNum(op, args[0])
		if err != nil {
			return nil, err
		}
		b, err := asNum(op, args[1])
		if err != nil {
			return nil, err
		}
		t, err := Arith(op, a, b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		return &SNum{T: t, ShenType: "number"}, nil

	case "%":
		a, err := asNum(op, args[0])
		if err != nil {
			return nil, err
		}
		b, err := asNum(op, args[1])
		if err != nil {
			return nil, err
		}
		x, xok := RealLit(a)
		y, yok := RealLit(b)
		if !xok || !yok {
			return nil, fmt.Errorf("%%: symbolic modulo is outside the supported fragment")
		}
		if int64(y) == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return &SNum{T: R(float64(int64(x) % int64(y))), ShenType: "number"}, nil

	case "<", "<=", ">", ">=":
		a, err := asNum(op, args[0])
		if err != nil {
			return nil, err
		}
		b, err := asNum(op, args[1])
		if err != nil {
			return nil, err
		}
		t, err := Cmp(op, a, b)
		if err != nil {
			return nil, err
		}
		return &SBoolV{T: t, ShenType: "boolean"}, nil

	case "=", "!=":
		t, err := symEqual(args[0], args[1])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		if op == "!=" {
			t = Not(t)
		}
		return &SBoolV{T: t, ShenType: "boolean"}, nil

	case "and", "or":
		a, err := asBool(op, args[0])
		if err != nil {
			return nil, err
		}
		b, err := asBool(op, args[1])
		if err != nil {
			return nil, err
		}
		if op == "and" {
			return &SBoolV{T: And(a, b), ShenType: "boolean"}, nil
		}
		return &SBoolV{T: Or(a, b), ShenType: "boolean"}, nil

	case "not":
		a, err := asBool(op, args[0])
		if err != nil {
			return nil, err
		}
		return &SBoolV{T: Not(a), ShenType: "boolean"}, nil

	case "cons":
		xs, err := asList(op, args[1])
		if err != nil {
			return nil, err
		}
		elems := append([]SymVal{args[0]}, xs.Elems...)
		return &SListV{Elems: elems, ShenType: xs.ShenType, ElemType: xs.ElemType}, nil

	case "head":
		xs, err := asList(op, args[0])
		if err != nil {
			return nil, err
		}
		if len(xs.Elems) == 0 {
			return nil, fmt.Errorf("head: empty list")
		}
		return xs.Elems[0], nil

	case "tail":
		xs, err := asList(op, args[0])
		if err != nil {
			return nil, err
		}
		if len(xs.Elems) == 0 {
			return nil, fmt.Errorf("tail: empty list")
		}
		return &SListV{Elems: xs.Elems[1:], ShenType: xs.ShenType, ElemType: xs.ElemType}, nil

	case "concat":
		xss, err := asList(op, args[0])
		if err != nil {
			return nil, err
		}
		var out []SymVal
		inner := ""
		for _, x := range xss.Elems {
			l, err := asList(op, x)
			if err != nil {
				return nil, err
			}
			if inner == "" {
				inner = l.ShenType
			}
			out = append(out, l.Elems...)
		}
		return &SListV{Elems: out, ShenType: inner}, nil

	case "fst", "snd":
		t, ok := args[0].(*STupleV)
		if !ok {
			return nil, fmt.Errorf("%s: expected a tuple, got %s", op, args[0].String())
		}
		if op == "fst" {
			return t.Fst, nil
		}
		return t.Snd, nil

	case "map":
		xs, err := asList(op, args[1])
		if err != nil {
			return nil, err
		}
		out := make([]SymVal, len(xs.Elems))
		for i, x := range xs.Elems {
			v, err := e.apply(args[0], x)
			if err != nil {
				return nil, fmt.Errorf("map: %w", err)
			}
			out[i] = v
		}
		return &SListV{Elems: out}, nil

	case "filter":
		xs, err := asList(op, args[1])
		if err != nil {
			return nil, err
		}
		var out []SymVal
		for _, x := range xs.Elems {
			pv, err := e.apply(args[0], x)
			if err != nil {
				return nil, fmt.Errorf("filter: %w", err)
			}
			pb, err := asBool("filter", pv)
			if err != nil {
				return nil, err
			}
			// Keeping or dropping an element changes the shape of the
			// result, so this is a genuine decision point.
			if e.branch(pb) {
				out = append(out, x)
			}
		}
		return &SListV{Elems: out, ShenType: xs.ShenType, ElemType: xs.ElemType}, nil

	case "foldr":
		xs, err := asList(op, args[2])
		if err != nil {
			return nil, err
		}
		acc := args[1]
		for i := len(xs.Elems) - 1; i >= 0; i-- {
			partial, err := e.apply(args[0], xs.Elems[i])
			if err != nil {
				return nil, fmt.Errorf("foldr: %w", err)
			}
			acc, err = e.apply(partial, acc)
			if err != nil {
				return nil, fmt.Errorf("foldr: %w", err)
			}
		}
		return acc, nil

	case "foldl":
		xs, err := asList(op, args[2])
		if err != nil {
			return nil, err
		}
		acc := args[1]
		for _, x := range xs.Elems {
			partial, err := e.apply(args[0], acc)
			if err != nil {
				return nil, fmt.Errorf("foldl: %w", err)
			}
			acc, err = e.apply(partial, x)
			if err != nil {
				return nil, fmt.Errorf("foldl: %w", err)
			}
		}
		return acc, nil

	case "scanl":
		xs, err := asList(op, args[2])
		if err != nil {
			return nil, err
		}
		acc := args[1]
		out := []SymVal{acc}
		for _, x := range xs.Elems {
			partial, err := e.apply(args[0], acc)
			if err != nil {
				return nil, fmt.Errorf("scanl: %w", err)
			}
			acc, err = e.apply(partial, x)
			if err != nil {
				return nil, fmt.Errorf("scanl: %w", err)
			}
			out = append(out, acc)
		}
		return &SListV{Elems: out}, nil

	case "unfoldr":
		// unfoldr's list length depends on the seed, so it only has a
		// bounded symbolic reading when every step folds to a literal.
		f, seed := args[0], args[1]
		var out []SymVal
		for i := 0; i < 64; i++ {
			pair, err := e.apply(f, seed)
			if err != nil {
				return nil, fmt.Errorf("unfoldr: %w", err)
			}
			tp, ok := pair.(*STupleV)
			if !ok {
				return nil, fmt.Errorf("unfoldr: step must return a tuple")
			}
			cb, err := asBool("unfoldr", tp.Fst)
			if err != nil {
				return nil, err
			}
			cont, ok := BoolLit(cb)
			if !ok {
				return nil, fmt.Errorf("unfoldr: symbolic termination condition is outside the bounded fragment")
			}
			if !cont {
				break
			}
			inner, ok := tp.Snd.(*STupleV)
			if !ok {
				return nil, fmt.Errorf("unfoldr: step must return (@p Cont (@p Elem Seed))")
			}
			out = append(out, inner.Fst)
			seed = inner.Snd
		}
		return &SListV{Elems: out}, nil

	case "compose":
		gx, err := e.apply(args[1], args[2])
		if err != nil {
			return nil, fmt.Errorf("compose: %w", err)
		}
		return e.apply(args[0], gx)
	}
	return nil, fmt.Errorf("unknown primitive: %s", op)
}

// --- coercions ---

func asNum(op string, v SymVal) (Term, error) {
	n, ok := v.(*SNum)
	if !ok {
		return nil, fmt.Errorf("%s: expected a number, got %s", op, v.String())
	}
	return n.T, nil
}

func asBool(op string, v SymVal) (Term, error) {
	b, ok := v.(*SBoolV)
	if !ok {
		return nil, fmt.Errorf("%s: expected a boolean, got %s", op, v.String())
	}
	return b.T, nil
}

func asList(op string, v SymVal) (*SListV, error) {
	l, ok := v.(*SListV)
	if !ok {
		return nil, fmt.Errorf("%s: expected a list, got %s", op, v.String())
	}
	return l, nil
}

// symEqual builds the condition under which two symbolic values are
// equal. Structure (list length, tuple shape) is concrete, so a shape
// mismatch folds straight to false.
func symEqual(a, b SymVal) (Term, error) {
	switch x := a.(type) {
	case *SNum:
		y, ok := b.(*SNum)
		if !ok {
			return B(false), nil
		}
		return EqTerm(x.T, y.T)
	case *SStrV:
		y, ok := b.(*SStrV)
		if !ok {
			return B(false), nil
		}
		return EqTerm(x.T, y.T)
	case *SBoolV:
		y, ok := b.(*SBoolV)
		if !ok {
			return B(false), nil
		}
		return EqTerm(x.T, y.T)
	case *SListV:
		y, ok := b.(*SListV)
		if !ok || len(x.Elems) != len(y.Elems) {
			return B(false), nil
		}
		conds := make([]Term, 0, len(x.Elems))
		for i := range x.Elems {
			c, err := symEqual(x.Elems[i], y.Elems[i])
			if err != nil {
				return nil, err
			}
			conds = append(conds, c)
		}
		return And(conds...), nil
	case *STupleV:
		y, ok := b.(*STupleV)
		if !ok {
			return B(false), nil
		}
		f, err := symEqual(x.Fst, y.Fst)
		if err != nil {
			return nil, err
		}
		s, err := symEqual(x.Snd, y.Snd)
		if err != nil {
			return nil, err
		}
		return And(f, s), nil
	}
	return nil, fmt.Errorf("cannot compare %s for equality", a.String())
}
