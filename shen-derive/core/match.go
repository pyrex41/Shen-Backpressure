package core

import "fmt"

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
	bindings := map[string]Value{}
	if err := matchInto(pat, v, bindings); err != nil {
		if err == errNoMatch {
			return nil, false, nil
		}
		return nil, false, err
	}
	return bindings, true, nil
}

// sentinel error used to signal a structural non-match (as distinct from a
// malformed pattern, which returns a real error).
var errNoMatch = fmt.Errorf("no match")

func matchInto(pat Sexpr, v Value, out map[string]Value) error {
	switch p := pat.(type) {
	case *Atom:
		return matchAtom(p, v, out)
	case *List:
		return matchList(p, v, out)
	}
	return fmt.Errorf("unsupported pattern node: %T", pat)
}

func matchAtom(p *Atom, v Value, out map[string]Value) error {
	switch p.Kind {
	case AtomSymbol:
		switch {
		case p.Val == "_":
			return nil
		case p.Val == "nil":
			if lv, ok := v.(ListVal); ok && len(lv) == 0 {
				return nil
			}
			return errNoMatch
		case isUpper(p.Val[0]):
			// Variable binding. Repeated uppercase names in the same clause
			// would require consistency; for now, last-one-wins. The callers
			// of Match don't use repeated names, so we don't enforce it.
			out[p.Val] = v
			return nil
		default:
			return fmt.Errorf("unsupported symbol pattern %q "+
				"(only _, nil, and uppercase vars are allowed)", p.Val)
		}
	case AtomInt, AtomFloat:
		lit, err := Eval(nil, p)
		if err != nil {
			return err
		}
		if valEqual(lit, v) {
			return nil
		}
		return errNoMatch
	case AtomBool:
		want := p.Val == "true"
		bv, ok := v.(BoolVal)
		if !ok || bool(bv) != want {
			return errNoMatch
		}
		return nil
	case AtomString:
		sv, ok := v.(StringVal)
		if !ok || string(sv) != p.Val {
			return errNoMatch
		}
		return nil
	}
	return fmt.Errorf("unsupported atom kind in pattern: %v", p.Kind)
}

func matchList(p *List, v Value, out map[string]Value) error {
	if HeadSym(p) != "cons" || len(p.Elems) != 3 {
		return fmt.Errorf("unsupported list pattern %q "+
			"(only cons patterns are supported)", p.String())
	}
	lv, ok := v.(ListVal)
	if !ok || len(lv) == 0 {
		return errNoMatch
	}
	// head pattern → first element
	if err := matchInto(p.Elems[1], lv[0], out); err != nil {
		return err
	}
	// tail pattern → rest of list as a ListVal
	tail := ListVal(lv[1:])
	return matchInto(p.Elems[2], tail, out)
}

func isUpper(b byte) bool { return b >= 'A' && b <= 'Z' }
