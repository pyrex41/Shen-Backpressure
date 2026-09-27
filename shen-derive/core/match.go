package core

import (
	"fmt"
	"strconv"
)

// Match tries to match a Shen-style pattern (an Sexpr) against a runtime
// Value. On success it returns the variable bindings produced by the match.
// On structural mismatch it returns (nil, false, nil). On an unrecognized
// pattern shape it returns a non-nil error.
//
// Supported patterns:
//   - Atom{Sym "_"}                     — wildcard, binds nothing
//   - Atom{Sym "Uppercase"}             — variable binding
//   - Atom{Sym "nil"}                   — matches an empty list
//   - Atom{Int/Float/Bool/String}       — literal; matches by valEqual
//   - List{cons, head, tail}            — cons pattern; matches a non-empty
//                                         list, binds head and tail recursively
//
// Fixed-length list patterns like `[A B]` desugar to
// `(cons A (cons B nil))`, so the cons case is the only list form needed.
func Match(pat Sexpr, v Value) (map[string]Value, bool, error) {
	env, ok, err := MatchEnv(pat, v, nil)
	if !ok {
		return nil, false, err
	}
	bindings := map[string]Value{}
	for e := env; e != nil; e = e.parent {
		if _, seen := bindings[e.name]; !seen {
			bindings[e.name] = e.val
		}
	}
	return bindings, true, nil
}

// MatchEnv is Match that binds the pattern's variables by extending env
// instead of building a map, which is what an evaluator needs and
// allocates far less. On a structural mismatch it returns (nil, false,
// nil).
func MatchEnv(pat Sexpr, v Value, env *Env) (*Env, bool, error) {
	env, err := matchInto(pat, v, env)
	if err != nil {
		if err == errNoMatch {
			return nil, false, nil
		}
		return nil, false, err
	}
	return env, true, nil
}

// sentinel error used to signal a structural non-match (as distinct from a
// malformed pattern, which returns a real error).
var errNoMatch = fmt.Errorf("no match")

func matchInto(pat Sexpr, v Value, env *Env) (*Env, error) {
	switch p := pat.(type) {
	case *Atom:
		return matchAtom(p, v, env)
	case *List:
		return matchList(p, v, env)
	}
	return nil, fmt.Errorf("unsupported pattern node: %T", pat)
}

func matchAtom(p *Atom, v Value, env *Env) (*Env, error) {
	switch p.Kind {
	case AtomSymbol:
		switch {
		case p.Val == "_":
			return env, nil
		case p.Val == "nil":
			if lv, ok := v.(ListVal); ok && len(lv) == 0 {
				return env, nil
			}
			return nil, errNoMatch
		case isUpper(p.Val[0]):
			// Variable binding. Repeated uppercase names in the same clause
			// would require consistency; for now, last-one-wins. The callers
			// of Match don't use repeated names, so we don't enforce it.
			return env.Extend(p.Val, v), nil
		default:
			return nil, fmt.Errorf("unsupported symbol pattern %q "+
				"(only _, nil, and uppercase vars are allowed)", p.Val)
		}
	case AtomInt, AtomFloat:
		// Compare numerically without boxing the literal, as valEqual
		// would: IntVal(1) matches the pattern 1.0 and vice versa.
		want, err := strconv.ParseFloat(p.Val, 64)
		if err != nil {
			return nil, fmt.Errorf("bad numeric literal in pattern: %s", p.Val)
		}
		if got, ok := asNum(v); ok && got == want {
			return env, nil
		}
		return nil, errNoMatch
	case AtomBool:
		want := p.Val == "true"
		bv, ok := v.(BoolVal)
		if !ok || bool(bv) != want {
			return nil, errNoMatch
		}
		return env, nil
	case AtomString:
		sv, ok := v.(StringVal)
		if !ok || string(sv) != p.Val {
			return nil, errNoMatch
		}
		return env, nil
	}
	return nil, fmt.Errorf("unsupported atom kind in pattern: %v", p.Kind)
}

func matchList(p *List, v Value, env *Env) (*Env, error) {
	if HeadSym(p) != "cons" || len(p.Elems) != 3 {
		return nil, fmt.Errorf("unsupported list pattern %q "+
			"(only cons patterns are supported)", p.String())
	}
	lv, ok := v.(ListVal)
	if !ok || len(lv) == 0 {
		return nil, errNoMatch
	}
	// head pattern → first element
	env, err := matchInto(p.Elems[1], lv[0], env)
	if err != nil {
		return nil, err
	}
	// tail pattern → rest of list as a ListVal
	return matchInto(p.Elems[2], lv[1:], env)
}

func isUpper(b byte) bool { return b >= 'A' && b <= 'Z' }
