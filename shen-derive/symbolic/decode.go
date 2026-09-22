package symbolic

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// DecodeModel turns a solver's raw model text into shen-derive values,
// one per declared variable. Variables the solver left unconstrained
// get a canonical default (0, "", false) so a decoded model is always
// total over the query's variables.
func DecodeModel(vars []*Var, raw map[string]string) (map[string]core.Value, error) {
	out := make(map[string]core.Value, len(vars))
	for _, v := range vars {
		text, ok := raw[v.Name]
		if !ok {
			out[v.Name] = defaultFor(v.S)
			continue
		}
		val, err := decodeSMTValue(v.S, text)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", v.Name, err)
		}
		out[v.Name] = val
	}
	return out, nil
}

func defaultFor(s Sort) core.Value {
	switch s {
	case SortBool:
		return core.BoolVal(false)
	case SortString:
		return core.StringVal("")
	default:
		return core.IntVal(0)
	}
}

// decodeSMTValue parses one SMT-LIB2 value of the given sort.
func decodeSMTValue(s Sort, text string) (core.Value, error) {
	text = strings.TrimSpace(text)
	switch s {
	case SortBool:
		switch text {
		case "true":
			return core.BoolVal(true), nil
		case "false":
			return core.BoolVal(false), nil
		}
		return nil, fmt.Errorf("not a boolean: %q", text)

	case SortString:
		if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
			return core.StringVal(unescapeSMTString(text[1 : len(text)-1])), nil
		}
		return nil, fmt.Errorf("not a string literal: %q", text)

	default:
		f, err := decodeReal(text)
		if err != nil {
			return nil, err
		}
		return numberValue(f), nil
	}
}

// numberValue narrows a whole-valued real to core.IntVal so generated
// Go literals read as `5` rather than `5.0` wherever possible — the
// same convention the boundary pool uses.
func numberValue(f float64) core.Value {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return core.IntVal(int64(f))
	}
	return core.FloatVal(f)
}

// decodeReal parses the Real forms z3 emits: a numeral, `(- x)`, and
// `(/ a b)` with either operand possibly negated.
func decodeReal(text string) (float64, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "(") {
		toks, err := tokenizeSMT(text)
		if err != nil {
			return 0, err
		}
		return decodeRealToks(toks)
	}
	return strconv.ParseFloat(text, 64)
}

func decodeRealToks(toks []string) (float64, error) {
	f, next, err := realFromToks(toks, 0)
	if err != nil {
		return 0, err
	}
	if next != len(toks) {
		return 0, fmt.Errorf("trailing tokens in real value")
	}
	return f, nil
}

func realFromToks(toks []string, i int) (float64, int, error) {
	if i >= len(toks) {
		return 0, i, fmt.Errorf("truncated real value")
	}
	if toks[i] != "(" {
		f, err := strconv.ParseFloat(toks[i], 64)
		return f, i + 1, err
	}
	i++
	if i >= len(toks) {
		return 0, i, fmt.Errorf("truncated real expression")
	}
	op := toks[i]
	i++
	var args []float64
	for i < len(toks) && toks[i] != ")" {
		f, next, err := realFromToks(toks, i)
		if err != nil {
			return 0, i, err
		}
		args = append(args, f)
		i = next
	}
	if i >= len(toks) {
		return 0, i, fmt.Errorf("unclosed real expression")
	}
	i++ // consume ")"
	switch op {
	case "-":
		if len(args) == 1 {
			return -args[0], i, nil
		}
		if len(args) == 2 {
			return args[0] - args[1], i, nil
		}
	case "+":
		if len(args) == 2 {
			return args[0] + args[1], i, nil
		}
	case "*":
		if len(args) == 2 {
			return args[0] * args[1], i, nil
		}
	case "/":
		if len(args) == 2 {
			if args[1] == 0 {
				return 0, i, fmt.Errorf("model divides by zero")
			}
			return args[0] / args[1], i, nil
		}
	case "to_real", "to_int":
		if len(args) == 1 {
			return args[0], i, nil
		}
	}
	return 0, i, fmt.Errorf("unsupported real expression operator %q with %d args", op, len(args))
}

func unescapeSMTString(s string) string {
	return strings.ReplaceAll(s, `""`, `"`)
}

// DecodeSkeleton rebuilds a concrete core.Value from a symbolic
// skeleton and a decoded model. A skeleton's scalars are plain
// variables (see buildSkeleton), so this is a structural walk.
func DecodeSkeleton(sv SymVal, model map[string]core.Value) (core.Value, error) {
	switch v := sv.(type) {
	case *SNum:
		return scalarFromModel(v.T, model, SortReal)
	case *SStrV:
		return scalarFromModel(v.T, model, SortString)
	case *SBoolV:
		return scalarFromModel(v.T, model, SortBool)
	case *SListV:
		out := make([]core.Value, len(v.Elems))
		for i, e := range v.Elems {
			cv, err := DecodeSkeleton(e, model)
			if err != nil {
				return nil, err
			}
			out[i] = cv
		}
		return core.ListVal(out), nil
	case *STupleV:
		f, err := DecodeSkeleton(v.Fst, model)
		if err != nil {
			return nil, err
		}
		s, err := DecodeSkeleton(v.Snd, model)
		if err != nil {
			return nil, err
		}
		return &core.TupleVal{Fst: f, Snd: s}, nil
	}
	return nil, fmt.Errorf("cannot decode symbolic value %s", sv.String())
}

func scalarFromModel(t Term, model map[string]core.Value, s Sort) (core.Value, error) {
	if v, ok := t.(*Var); ok {
		if cv, ok := model[v.Name]; ok {
			return cv, nil
		}
		return defaultFor(s), nil
	}
	// A skeleton leaf can be a literal when the type table pins it.
	switch s {
	case SortBool:
		if b, ok := BoolLit(t); ok {
			return core.BoolVal(b), nil
		}
	case SortString:
		if str, ok := StrLit(t); ok {
			return core.StringVal(str), nil
		}
	default:
		if f, ok := RealLit(t); ok {
			return numberValue(f), nil
		}
	}
	return nil, fmt.Errorf("skeleton leaf %s is not a variable or literal", t.String())
}
