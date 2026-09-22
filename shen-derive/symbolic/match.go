package symbolic

import (
	"fmt"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// matchSym is the symbolic counterpart of core.Match. Instead of a
// yes/no answer it returns the *condition* under which the pattern
// matches: structure (list length, value shape) is concrete so it
// folds to a literal, while a literal atom pattern against a symbolic
// scalar produces an equality the enumerator can branch on.
//
// Supported patterns mirror core.Match exactly:
//
//   - `_`                  wildcard, binds nothing
//   - `Uppercase`          variable binding
//   - `nil`                matches an empty list
//   - number/string/bool   literal, compared by equality
//   - (cons H T)           non-empty list, head and tail matched
//
// Fixed-length list patterns desugar to nested cons in the parser, so
// cons is the only list form needed here.
func matchSym(pat core.Sexpr, v SymVal, env *SEnv) (*SEnv, Term, error) {
	switch p := pat.(type) {
	case *core.Atom:
		return matchSymAtom(p, v, env)
	case *core.List:
		return matchSymList(p, v, env)
	}
	return env, nil, fmt.Errorf("unsupported pattern node: %T", pat)
}

func matchSymAtom(p *core.Atom, v SymVal, env *SEnv) (*SEnv, Term, error) {
	switch p.Kind {
	case core.AtomSymbol:
		switch {
		case p.Val == "_":
			return env, B(true), nil
		case p.Val == "nil":
			l, ok := v.(*SListV)
			if !ok {
				return env, B(false), nil
			}
			return env, B(len(l.Elems) == 0), nil
		case isUpper(p.Val[0]):
			return env.Extend(p.Val, v), B(true), nil
		default:
			return env, nil, fmt.Errorf("unsupported symbol pattern %q "+
				"(only _, nil, and uppercase vars are allowed)", p.Val)
		}

	case core.AtomInt, core.AtomFloat:
		n, ok := v.(*SNum)
		if !ok {
			return env, B(false), nil
		}
		f, ok := core.SexprFloatVal(p)
		if !ok {
			return env, nil, fmt.Errorf("bad numeric literal in pattern: %s", p.Val)
		}
		t, err := EqTerm(n.T, R(f))
		return env, t, err

	case core.AtomBool:
		b, ok := v.(*SBoolV)
		if !ok {
			return env, B(false), nil
		}
		t, err := EqTerm(b.T, B(p.Val == "true"))
		return env, t, err

	case core.AtomString:
		s, ok := v.(*SStrV)
		if !ok {
			return env, B(false), nil
		}
		t, err := EqTerm(s.T, S(p.Val))
		return env, t, err
	}
	return env, nil, fmt.Errorf("unsupported atom kind in pattern: %v", p.Kind)
}

func matchSymList(p *core.List, v SymVal, env *SEnv) (*SEnv, Term, error) {
	if core.HeadSym(p) != "cons" || len(p.Elems) != 3 {
		return env, nil, fmt.Errorf("unsupported list pattern %q "+
			"(only cons patterns are supported)", p.String())
	}
	l, ok := v.(*SListV)
	if !ok || len(l.Elems) == 0 {
		return env, B(false), nil
	}
	env, headCond, err := matchSym(p.Elems[1], l.Elems[0], env)
	if err != nil {
		return env, nil, err
	}
	tail := &SListV{Elems: l.Elems[1:], ShenType: l.ShenType, ElemType: l.ElemType}
	env, tailCond, err := matchSym(p.Elems[2], tail, env)
	if err != nil {
		return env, nil, err
	}
	return env, And(headCond, tailCond), nil
}

func isUpper(b byte) bool { return b >= 'A' && b <= 'Z' }
