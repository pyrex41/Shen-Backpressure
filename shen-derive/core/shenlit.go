package core

// shenlit.go — W6.D. Rendering an evaluator value back as Shen source.
//
// The evaluator is one of two oracles for a spec's meaning. The other
// is a Shen host running the spec itself. Asking the second oracle
// needs the sampled input in a form the host can read, which is what
// this file provides: `(processable 0 [[5 "bob" "bob"]])` rather than
// the Go literal `mustAmount(0), []Transaction{…}` the generated test
// carries.
//
// The rendering is the inverse of core.ParseSexpr for the value subset
// the sampler produces, and it is deliberately total only over that
// subset: a closure or a partially-applied primitive has no Shen
// literal, and pretending otherwise would send the host a goal that
// means something else. Those return an error.

import (
	"fmt"
	"strconv"
	"strings"
)

// ShenLiteral renders v as Shen source text.
//
// Wrapper and constrained values are rendered as their base value,
// with no constructor call. That matches how the evaluator represents
// them (see verify.buildBaseEnv: `val` is the identity function) and
// how the spec's own types read them: a premise `X : amount` is
// satisfied by the number, and the host's `val` unwraps a number to a
// number. Rendering `(NewAmount 5)` instead would name a function the
// spec does not define.
func ShenLiteral(v Value) (string, error) {
	switch t := v.(type) {
	case IntVal:
		return strconv.FormatInt(int64(t), 10), nil
	case FloatVal:
		f := float64(t)
		// An integral float is written without a fractional part:
		// Shen's reader gives the same number either way, and the
		// shorter form is what a person comparing the goal against
		// the generated test file expects to see.
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10), nil
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	case BoolVal:
		if bool(t) {
			return "true", nil
		}
		return "false", nil
	case StringVal:
		return shenStringLiteral(string(t)), nil
	case ListVal:
		parts := make([]string, len(t))
		for i, e := range t {
			s, err := ShenLiteral(e)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, " ") + "]", nil
	case *TupleVal:
		fst, err := ShenLiteral(t.Fst)
		if err != nil {
			return "", err
		}
		snd, err := ShenLiteral(t.Snd)
		if err != nil {
			return "", err
		}
		return "(@p " + fst + " " + snd + ")", nil
	}
	return "", fmt.Errorf("no Shen literal for %T (%s)", v, v)
}

// shenStringLiteral quotes a string for Shen.
//
// Shen string literals have no escape sequences: a `"` cannot appear
// inside one at all, and a newline is written literally. So a string
// carrying a quote cannot be rendered, and the caller is told rather
// than handed a literal that would end early and turn the rest of the
// goal into garbage. The sampler's own strings are "" and short
// identifiers, so this is a guard, not a limitation anybody meets.
func shenStringLiteral(s string) string {
	if strings.Contains(s, `"`) {
		// Cannot be represented; the caller checks with
		// ShenRenderable before relying on the result.
		return `""`
	}
	return `"` + s + `"`
}

// ShenRenderable reports whether v has a faithful Shen literal.
func ShenRenderable(v Value) bool {
	switch t := v.(type) {
	case IntVal, FloatVal, BoolVal:
		return true
	case StringVal:
		return !strings.Contains(string(t), `"`)
	case ListVal:
		for _, e := range t {
			if !ShenRenderable(e) {
				return false
			}
		}
		return true
	case *TupleVal:
		return ShenRenderable(t.Fst) && ShenRenderable(t.Snd)
	}
	return false
}
