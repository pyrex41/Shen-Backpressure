// Package verify generates Go tests that check an implementation matches
// a Shen spec. The spec is the oracle: we evaluate it on sampled inputs and
// emit a test that asserts the implementation produces the same outputs.
package verify

import (
	"fmt"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// Sample is one concrete input value for a single parameter of the spec.
// It carries both the evaluator value (for spec evaluation) and the Go
// source expression (for emitting the test file).
type Sample struct {
	Value  core.Value // used by the spec evaluator
	GoExpr string     // used by the generated Go test, e.g. "mustAmount(100)"
}

// GenSamples returns a small, deterministic set of samples for the given
// Shen type. The sample count is intentionally tiny — the goal is a handful
// of meaningful cases, not exhaustive coverage.
func GenSamples(shenType string, tt *specfile.TypeTable) ([]Sample, error) {
	shenType = strings.TrimSpace(shenType)

	// list types: (list T)
	if elemType := specfile.ElemType(shenType); elemType != "" {
		elemSamples, err := GenSamples(elemType, tt)
		if err != nil {
			return nil, err
		}
		return listSamples(elemType, elemSamples, tt), nil
	}

	// primitives
	switch shenType {
	case "number":
		return numberSamples(), nil
	case "string", "symbol":
		return []Sample{
			{Value: core.StringVal(""), GoExpr: `""`},
			{Value: core.StringVal("alice"), GoExpr: `"alice"`},
			{Value: core.StringVal("bob"), GoExpr: `"bob"`},
		}, nil
	case "boolean":
		return []Sample{
			{Value: core.BoolVal(true), GoExpr: "true"},
			{Value: core.BoolVal(false), GoExpr: "false"},
		}, nil
	}

	// declared types: look up in TypeTable
	entry, ok := tt.Entries[shenType]
	if !ok {
		return nil, fmt.Errorf("unknown Shen type %q", shenType)
	}

	switch entry.Category {
	case specfile.CatWrapper, specfile.CatConstrained:
		return wrapperSamples(entry, tt)

	case specfile.CatComposite, specfile.CatGuarded:
		return compositeSamples(entry, tt)

	case specfile.CatAlias:
		// Follow the alias: shengen records WrappedType on the first premise.
		// For v1 we don't track alias targets here; treat like the underlying
		// entry if present, otherwise error.
		return nil, fmt.Errorf("alias type %q not supported in samples yet", shenType)

	case specfile.CatSumType:
		return nil, fmt.Errorf("sum type %q sampling not supported yet", shenType)

	default:
		return nil, fmt.Errorf("unhandled category %q for %q", entry.Category, shenType)
	}
}

// wrapperSamples samples a wrapper/constrained type by sampling its
// underlying primitive and filtering out any samples that would violate
// the constrained type's verified predicates. The evaluator value is just
// the primitive (the wrapping is invisible at eval time). The Go
// expression uses the shengen-generated constructor via a "mustXxx" helper.
func wrapperSamples(entry *specfile.TypeEntry, tt *specfile.TypeTable) ([]Sample, error) {
	primSamples, err := GenSamples(entry.ShenPrim, tt)
	if err != nil {
		return nil, err
	}

	// For constrained types, drop any sample whose raw primitive value
	// fails the verified predicates. This lets us include "unsafe" values
	// (negatives, zero) in the primitive pool without them causing panics
	// when wrapped.
	if entry.Category == specfile.CatConstrained {
		primSamples = filterByConstraints(entry, primSamples)
	}

	var out []Sample
	helper := "must" + entry.GoName
	for _, ps := range primSamples {
		out = append(out, Sample{
			Value:  ps.Value, // primitive values pass through unchanged
			GoExpr: fmt.Sprintf("%s(%s)", helper, ps.GoExpr),
		})
	}
	return out, nil
}

// numberSamples returns the canonical primitive-number sample pool.
// Includes zero, positive, negative, and a fractional value so specs that
// branch on any of these are exercised. Constrained wrappers filter this
// pool via their verified predicates.
func numberSamples() []Sample {
	return []Sample{
		{Value: core.IntVal(0), GoExpr: "0"},
		{Value: core.IntVal(1), GoExpr: "1"},
		{Value: core.IntVal(-1), GoExpr: "-1"},
		{Value: core.IntVal(5), GoExpr: "5"},
		{Value: core.FloatVal(2.5), GoExpr: "2.5"},
		{Value: core.IntVal(100), GoExpr: "100"},
	}
}

// filterByConstraints removes samples whose value fails any verified
// predicate on the entry. A sample passes iff every predicate evaluates
// to true when the entry's variable is bound to the sample value.
// Parse/eval errors are treated as failures (sample dropped).
func filterByConstraints(entry *specfile.TypeEntry, candidates []Sample) []Sample {
	if len(entry.Verified) == 0 || entry.VarName == "" {
		return candidates
	}
	preds := make([]core.Sexpr, 0, len(entry.Verified))
	for _, raw := range entry.Verified {
		p, err := core.ParseSexpr(raw)
		if err != nil {
			// Unparseable predicate — be safe: drop everything.
			return nil
		}
		preds = append(preds, p)
	}

	var out []Sample
	for _, s := range candidates {
		ok := true
		env := core.EmptyEnv().Extend(entry.VarName, s.Value)
		for _, p := range preds {
			result, err := core.Eval(env, p)
			if err != nil {
				ok = false
				break
			}
			b, isBool := result.(core.BoolVal)
			if !isBool || !bool(b) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

// compositeSamples builds a small set of samples by picking the first sample
// of each field and producing one combined sample, then adding a couple of
// variations. This keeps cases bounded.
func compositeSamples(entry *specfile.TypeEntry, tt *specfile.TypeTable) ([]Sample, error) {
	if len(entry.Fields) == 0 {
		return nil, fmt.Errorf("composite %q has no fields", entry.ShenName)
	}

	// Sample each field.
	fieldSamples := make([][]Sample, len(entry.Fields))
	for i, f := range entry.Fields {
		fs, err := GenSamples(f.ShenType, tt)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", f.ShenName, err)
		}
		if len(fs) == 0 {
			return nil, fmt.Errorf("field %s: no samples", f.ShenName)
		}
		fieldSamples[i] = fs
	}

	// Produce one variation per unique index up to the longest field
	// sample list, so that if any field has a "tricky" sample
	// (e.g., a fractional number) it appears in at least one composite.
	maxVariations := 0
	for _, fs := range fieldSamples {
		if len(fs) > maxVariations {
			maxVariations = len(fs)
		}
	}

	helper := "must" + entry.GoName
	var out []Sample
	for v := 0; v < maxVariations; v++ {
		values := make([]core.Value, len(entry.Fields))
		goExprs := make([]string, len(entry.Fields))
		for i, fs := range fieldSamples {
			idx := v % len(fs)
			values[i] = fs[idx].Value
			goExprs[i] = fs[idx].GoExpr
		}
		out = append(out, Sample{
			Value:  core.ListVal(values),
			GoExpr: fmt.Sprintf("%s(%s)", helper, strings.Join(goExprs, ", ")),
		})
	}
	return out, nil
}

// listSamples returns list samples built from elem samples. It produces:
//   - the empty list
//   - a singleton for each distinct elem sample (capped)
//   - a multi-element list mixing the first few elem samples
//
// Producing one singleton per elem sample is important: otherwise
// "tricky" elem samples that only appear at higher indices (e.g. a
// fractional composite at index 3) never end up inside any list,
// leaving a whole class of bugs undetected.
func listSamples(elemType string, elemSamples []Sample, tt *specfile.TypeTable) []Sample {
	goElemType := tt.GoType(elemType)
	empty := Sample{
		Value:  core.ListVal(nil),
		GoExpr: fmt.Sprintf("[]%s{}", goElemType),
	}
	out := []Sample{empty}

	if len(elemSamples) == 0 {
		return out
	}

	const maxSingletons = 6
	n := len(elemSamples)
	if n > maxSingletons {
		n = maxSingletons
	}
	for i := 0; i < n; i++ {
		out = append(out, Sample{
			Value:  core.ListVal([]core.Value{elemSamples[i].Value}),
			GoExpr: fmt.Sprintf("[]%s{%s}", goElemType, elemSamples[i].GoExpr),
		})
	}

	// Multi-element list: first three elem samples if available.
	if len(elemSamples) >= 2 {
		m := 3
		if len(elemSamples) < m {
			m = len(elemSamples)
		}
		vals := make([]core.Value, m)
		exprs := make([]string, m)
		for i := 0; i < m; i++ {
			vals[i] = elemSamples[i].Value
			exprs[i] = elemSamples[i].GoExpr
		}
		out = append(out, Sample{
			Value:  core.ListVal(vals),
			GoExpr: fmt.Sprintf("[]%s{%s}", goElemType, strings.Join(exprs, ", ")),
		})
	}

	return out
}
