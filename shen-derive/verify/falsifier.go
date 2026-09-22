package verify

// falsifier.go — a fourth sample source: inputs the falsifier found.
//
// The boundary pool is what an author thought to try. The seeded draws
// are noise around it. Path cover is one witness per feasible branch of
// the spec. All three are produced by *this* program reasoning about
// the spec, so all three inherit its blind spots: nothing here can
// propose an input because the *implementation* looks suspicious at
// that point.
//
// The falsifier can. `sb loop --falsify` runs after a passing
// iteration, hands a model the spec, the guards, the forgery corpus
// and the mutation survivors, and asks for one input where the spec and
// the implementation disagree. What comes back is a claim, not
// evidence — so it is written to a plain JSON file and re-derived here
// against the spec's own evaluator. If the spec agrees with the
// implementation on that input, the case is committed as an ordinary
// passing sample and the claim was wrong. If it disagrees, the
// generated test fails and the loop has a counterexample. Either way
// the sample is permanent, which is the point: a survivor killed once
// stays killed.
//
// The file format is deliberately dull, because a model writes it:
//
//	{
//	  "schema_version": 1,
//	  "samples": [
//	    {
//	      "spec": "processable",
//	      "note": "kills off-by-one:internal/derived/processable.go:33:16",
//	      "args": [0, [{"amount": 0, "from": "a", "to": "b"}]]
//	    }
//	  ]
//	}
//
// Each entry in `args` is decoded against the corresponding parameter's
// Shen type, so a number is a number, a list is a JSON array, and a
// composite datatype is either a JSON array of its field values in
// declaration order or an object keyed by field name.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// FalsifierSchemaVersion is the version this reader understands. A
// file with a higher version is an error rather than a partial read:
// silently ignoring samples would make the gate quietly weaker.
const FalsifierSchemaVersion = 1

// DefaultFalsifierSamplesPath is where `sb loop --falsify` appends what
// the falsifier finds.
const DefaultFalsifierSamplesPath = ".sb/falsifier-samples.json"

// FalsifierFile is the on-disk format.
type FalsifierFile struct {
	SchemaVersion int               `json:"schema_version"`
	Samples       []FalsifierSample `json:"samples"`
}

// FalsifierSample is one proposed input.
type FalsifierSample struct {
	// Spec names the (define …) this input is for, so one file can
	// carry samples for every spec in a project.
	Spec string `json:"spec"`

	// Note is the falsifier's own account of why this input is
	// interesting — usually the mutant id it claims to kill. It is
	// carried into the generated test as a comment, because a sample
	// with no rationale is one nobody dares delete later.
	Note string `json:"note,omitempty"`

	// Args holds one JSON value per parameter of the spec.
	Args []json.RawMessage `json:"args"`
}

// LoadFalsifierSamples reads path and decodes the samples for the
// named spec against its parameter types. A missing file is not an
// error — the falsifier is optional and most runs have found nothing.
//
// A sample that does not decode is reported as a warning rather than
// an error: one malformed entry from a model must not take the whole
// gate down, and the operator needs to be told which entry it was.
func LoadFalsifierSamples(path, specName string, paramTypes []string, tt *specfile.TypeTable) ([][]Sample, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var f FalsifierFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.SchemaVersion > FalsifierSchemaVersion {
		return nil, nil, fmt.Errorf("%s: schema_version %d is newer than this shen-derive understands (%d); "+
			"reading it partially would make the gate quietly weaker", path, f.SchemaVersion, FalsifierSchemaVersion)
	}

	var rows [][]Sample
	var warnings []string
	n := 0
	for i, s := range f.Samples {
		if s.Spec != specName {
			continue
		}
		if len(s.Args) != len(paramTypes) {
			warnings = append(warnings, fmt.Sprintf(
				"%s sample %d: %d argument(s) for a spec with %d parameter(s); skipped",
				path, i, len(s.Args), len(paramTypes)))
			continue
		}
		row := make([]Sample, len(paramTypes))
		ok := true
		for j, pt := range paramTypes {
			sample, err := decodeFalsifierArg(s.Args[j], pt, tt)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf(
					"%s sample %d, parameter %d (%s): %v; skipped", path, i, j, pt, err))
				ok = false
				break
			}
			sample.Provenance = fmt.Sprintf("falsify:%d", n)
			sample.Note = s.Note
			row[j] = sample
		}
		if ok {
			rows = append(rows, row)
			n++
		}
	}
	return rows, warnings, nil
}

// decodeFalsifierArg turns one JSON value into a Sample of the given
// Shen type, producing both the evaluator value and the Go source
// expression. It reuses the same mustXxx helper convention as the
// boundary pool and the path witnesses, so all four sample sources
// share one set of helpers in the generated file.
func decodeFalsifierArg(raw json.RawMessage, shenType string, tt *specfile.TypeTable) (Sample, error) {
	shenType = strings.TrimSpace(shenType)

	// (list T)
	if elem := specfile.ElemType(shenType); elem != "" {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return Sample{}, fmt.Errorf("expected a JSON array for %s: %w", shenType, err)
		}
		vals := make(core.ListVal, len(items))
		exprs := make([]string, len(items))
		for i, it := range items {
			s, err := decodeFalsifierArg(it, elem, tt)
			if err != nil {
				return Sample{}, fmt.Errorf("element %d: %w", i, err)
			}
			vals[i] = s.Value
			exprs[i] = s.GoExpr
		}
		return Sample{
			Value:  vals,
			GoExpr: fmt.Sprintf("[]%s{%s}", tt.GoType(elem), strings.Join(exprs, ", ")),
		}, nil
	}

	// Composite and constrained declared types.
	if entry, ok := tt.Entries[shenType]; ok {
		switch entry.Category {
		case specfile.CatComposite, specfile.CatGuarded:
			fields, err := compositeFieldValues(raw, entry)
			if err != nil {
				return Sample{}, err
			}
			vals := make(core.ListVal, len(fields))
			exprs := make([]string, len(fields))
			for i, f := range entry.Fields {
				s, err := decodeFalsifierArg(fields[i], f.ShenType, tt)
				if err != nil {
					return Sample{}, fmt.Errorf("field %s (%s): %w", f.ShenName, f.ShenType, err)
				}
				vals[i] = s.Value
				exprs[i] = s.GoExpr
			}
			return Sample{
				Value:  vals,
				GoExpr: fmt.Sprintf("must%s(%s)", entry.GoName, strings.Join(exprs, ", ")),
			}, nil
		case specfile.CatWrapper, specfile.CatConstrained:
			inner, lit, err := decodePrimitive(raw)
			if err != nil {
				return Sample{}, err
			}
			if err := checkPrimKind(inner, entry.ShenPrim); err != nil {
				return Sample{}, fmt.Errorf("%s wraps a %s: %w", shenType, entry.ShenPrim, err)
			}
			return Sample{Value: inner, GoExpr: fmt.Sprintf("must%s(%s)", entry.GoName, lit)}, nil
		}
	}

	switch shenType {
	case "number", "string", "symbol", "boolean":
		v, lit, err := decodePrimitive(raw)
		if err != nil {
			return Sample{}, err
		}
		if err := checkPrimKind(v, shenType); err != nil {
			return Sample{}, err
		}
		return Sample{Value: v, GoExpr: lit}, nil
	}
	return Sample{}, fmt.Errorf("unknown Shen type %q", shenType)
}

// checkPrimKind rejects a JSON scalar of the wrong shape at decode
// time. Without it a string where a number belongs reaches the spec's
// evaluator and fails there, which reports the type error against the
// spec body rather than against the entry that caused it — a much
// worse message for whoever has to fix the file.
func checkPrimKind(v core.Value, shenPrim string) error {
	switch shenPrim {
	case "number":
		switch v.(type) {
		case core.IntVal, core.FloatVal:
			return nil
		}
		return fmt.Errorf("expected a number, got %s", jsonKindOf(v))
	case "string", "symbol":
		if _, ok := v.(core.StringVal); ok {
			return nil
		}
		return fmt.Errorf("expected a string, got %s", jsonKindOf(v))
	case "boolean":
		if _, ok := v.(core.BoolVal); ok {
			return nil
		}
		return fmt.Errorf("expected a boolean, got %s", jsonKindOf(v))
	}
	// An unknown primitive is not something to guess at; the evaluator
	// is a better judge than this function would be.
	return nil
}

func jsonKindOf(v core.Value) string {
	switch v.(type) {
	case core.IntVal, core.FloatVal:
		return "a number"
	case core.StringVal:
		return "a string"
	case core.BoolVal:
		return "a boolean"
	}
	return fmt.Sprintf("%T", v)
}

// compositeFieldValues accepts a composite either as a JSON array of
// its fields in declaration order, or as an object keyed by field
// name. Both are offered because a model writes this file and the
// array form is easy to get wrong silently while the object form is
// easy to get wrong loudly.
func compositeFieldValues(raw json.RawMessage, entry *specfile.TypeEntry) ([]json.RawMessage, error) {
	names := make([]string, len(entry.Fields))
	for i, f := range entry.Fields {
		names[i] = f.ShenName
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		if len(arr) != len(entry.Fields) {
			return nil, fmt.Errorf("%s takes %d field(s) (%s), got %d",
				entry.GoName, len(entry.Fields), strings.Join(names, ", "), len(arr))
		}
		return arr, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("expected an array of %d field(s) or an object keyed by %s for %s: %w",
			len(entry.Fields), strings.Join(names, "/"), entry.GoName, err)
	}
	// Field names are matched case-insensitively: the spec writes
	// `Amount` and JSON conventionally writes `amount`, and refusing
	// the second would be pedantry the author pays for.
	lookup := map[string]json.RawMessage{}
	for k, v := range obj {
		lookup[strings.ToLower(k)] = v
	}
	out := make([]json.RawMessage, len(entry.Fields))
	for i, name := range names {
		v, ok := lookup[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("%s is missing field %q; it takes %s",
				entry.GoName, name, strings.Join(names, ", "))
		}
		out[i] = v
	}
	if len(obj) > len(entry.Fields) {
		known := map[string]bool{}
		for _, n := range names {
			known[strings.ToLower(n)] = true
		}
		var extra []string
		for k := range obj {
			if !known[strings.ToLower(k)] {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			return nil, fmt.Errorf("%s has no field(s) %s; it takes %s",
				entry.GoName, strings.Join(extra, ", "), strings.Join(names, ", "))
		}
	}
	return out, nil
}

// decodePrimitive turns a JSON scalar into an evaluator value and the
// Go literal that denotes it.
func decodePrimitive(raw json.RawMessage) (core.Value, string, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, "", err
	}
	switch x := v.(type) {
	case bool:
		if x {
			return core.BoolVal(true), "true", nil
		}
		return core.BoolVal(false), "false", nil
	case string:
		return core.StringVal(x), fmt.Sprintf("%q", x), nil
	case json.Number:
		if i, err := x.Int64(); err == nil && !strings.ContainsAny(x.String(), ".eE") {
			return core.IntVal(i), x.String(), nil
		}
		f, err := x.Float64()
		if err != nil {
			return nil, "", fmt.Errorf("not a number: %s", x.String())
		}
		return core.FloatVal(f), formatFloatLiteral(f), nil
	}
	return nil, "", fmt.Errorf("expected a scalar, got %T", v)
}
