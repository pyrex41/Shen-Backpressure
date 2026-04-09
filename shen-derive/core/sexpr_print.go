package core

import (
	"fmt"
	"strings"
)

// PrettyPrintSexpr renders an s-expression in Shen surface syntax.
func PrettyPrintSexpr(s Sexpr) string {
	if s == nil {
		return "()"
	}
	return ppSexpr(s)
}

func ppSexpr(s Sexpr) string {
	switch s := s.(type) {
	case *Atom:
		if s.Kind == AtomString {
			return fmt.Sprintf("%q", s.Val)
		}
		return s.Val
	case *List:
		if len(s.Elems) == 0 {
			return "()"
		}

		// Try to render as [a b c] sugar for cons chains
		if elems, tail, ok := desugarConsList(s); ok {
			parts := make([]string, len(elems))
			for i, e := range elems {
				parts[i] = ppSexpr(e)
			}
			if IsSym(tail, "nil") {
				return "[" + strings.Join(parts, " ") + "]"
			}
			return "[" + strings.Join(parts, " ") + " | " + ppSexpr(tail) + "]"
		}

		parts := make([]string, len(s.Elems))
		for i, e := range s.Elems {
			parts[i] = ppSexpr(e)
		}
		return "(" + strings.Join(parts, " ") + ")"
	}
	return fmt.Sprintf("%v", s)
}

// desugarConsList checks if a list is a cons chain:
// (cons a (cons b (cons c nil))) → [a, b, c], nil, true
// (cons a (cons b tail)) → [a, b], tail, true
func desugarConsList(l *List) (elems []Sexpr, tail Sexpr, ok bool) {
	if len(l.Elems) != 3 || !IsSym(l.Elems[0], "cons") {
		return nil, nil, false
	}

	elems = append(elems, l.Elems[1])
	rest := l.Elems[2]

	for {
		rl, isList := rest.(*List)
		if !isList || len(rl.Elems) != 3 || !IsSym(rl.Elems[0], "cons") {
			return elems, rest, true
		}
		elems = append(elems, rl.Elems[1])
		rest = rl.Elems[2]
	}
}
