package verify

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// HarnessConfig holds everything needed to generate a spec-equivalence test.
type HarnessConfig struct {
	Spec      *specfile.Define
	TypeTable *specfile.TypeTable

	// Implementation package info — these are used to generate the import
	// and the qualified function call in the test body.
	ImplPkgPath string // e.g. "example.com/payment/internal/derived"
	ImplPkgName string // e.g. "derived"
	ImplFunc    string // e.g. "Processable"

	// Package the test file itself lives in. Usually matches ImplPkgName + "_test".
	TestPkgName string // e.g. "derived_test"

	// Optional: override the number of list sizes tried (default 3).
	MaxCases int
}

// Harness is the prepared-but-not-yet-emitted verification bundle.
type Harness struct {
	Config *HarnessConfig
	Cases  []Case
}

// Case is one (input, expected output) pair.
type Case struct {
	Name       string
	Args       []Sample   // one per parameter of the spec
	Expected   core.Value // spec-evaluated output
	ExpectedGo string     // Go literal for the expected value
}

// BuildHarness evaluates the spec on sampled inputs and records the
// expected outputs. It does not emit any source code yet.
func BuildHarness(cfg *HarnessConfig) (*Harness, error) {
	if cfg.Spec == nil {
		return nil, fmt.Errorf("nil spec")
	}
	if cfg.TypeTable == nil {
		return nil, fmt.Errorf("nil type table")
	}
	if len(cfg.Spec.ParamNames) != len(cfg.Spec.TypeSig.ParamTypes) {
		return nil, fmt.Errorf("spec %s: param count mismatch", cfg.Spec.Name)
	}

	// Generate samples per parameter.
	paramSamples := make([][]Sample, len(cfg.Spec.ParamNames))
	for i, pt := range cfg.Spec.TypeSig.ParamTypes {
		ss, err := GenSamples(pt, cfg.TypeTable)
		if err != nil {
			return nil, fmt.Errorf("samples for param %s: %w", cfg.Spec.ParamNames[i], err)
		}
		paramSamples[i] = ss
	}

	// Cartesian product, bounded. Default is high enough that a reasonable
	// set of param samples × list shapes is exhaustively covered.
	maxCases := cfg.MaxCases
	if maxCases <= 0 {
		maxCases = 50
	}
	combos := cartesian(paramSamples, maxCases)

	h := &Harness{Config: cfg}
	for idx, combo := range combos {
		val, err := evalSpec(cfg.Spec, combo, cfg.TypeTable)
		if err != nil {
			return nil, fmt.Errorf("case %d: eval spec: %w", idx, err)
		}
		goLit, err := goLiteralFor(val, cfg.Spec.TypeSig.ReturnType, cfg.TypeTable)
		if err != nil {
			return nil, fmt.Errorf("case %d: literal: %w", idx, err)
		}
		h.Cases = append(h.Cases, Case{
			Name:       fmt.Sprintf("case_%02d", idx),
			Args:       combo,
			Expected:   val,
			ExpectedGo: goLit,
		})
	}
	return h, nil
}

// cartesian computes the cartesian product of per-parameter samples, capped
// at maxCases total.
func cartesian(paramSamples [][]Sample, maxCases int) [][]Sample {
	if len(paramSamples) == 0 {
		return nil
	}
	var out [][]Sample
	indices := make([]int, len(paramSamples))
	for {
		row := make([]Sample, len(paramSamples))
		for i, idx := range indices {
			row[i] = paramSamples[i][idx]
		}
		out = append(out, row)
		if len(out) >= maxCases {
			return out
		}
		// increment indices like an odometer
		i := len(indices) - 1
		for i >= 0 {
			indices[i]++
			if indices[i] < len(paramSamples[i]) {
				break
			}
			indices[i] = 0
			i--
		}
		if i < 0 {
			return out
		}
	}
}

// evalSpec evaluates the spec body with the given argument values. It
// constructs an evaluation environment that contains:
//   - the spec's parameters bound to their sampled values
//   - `val` as the identity function (wrapper types are primitives at eval time)
//   - one closure per field accessor in the type table
func evalSpec(def *specfile.Define, args []Sample, tt *specfile.TypeTable) (core.Value, error) {
	env := buildSpecEnv(def, args, tt)
	return core.Eval(env, def.Body)
}

// buildSpecEnv populates an Env for spec evaluation. See evalSpec.
func buildSpecEnv(def *specfile.Define, args []Sample, tt *specfile.TypeTable) *core.Env {
	env := core.EmptyEnv()

	// `val` is identity — wrapper/constrained values are already primitives
	// at evaluation time.
	env = env.Extend("val", &core.BuiltinFn{
		Name: "val",
		Fn:   func(v core.Value) (core.Value, error) { return v, nil },
	})

	// Register field accessors. Each composite field becomes a closure that
	// projects the corresponding index out of a ListVal. We use the Shen field
	// name lowered to match common spec-writing style (e.g., field "Amount" →
	// accessor `amount`). We also register the exact field name for safety.
	registered := map[string]bool{}
	for _, entry := range tt.Entries {
		for _, f := range entry.Fields {
			register := func(name string, idx int) {
				if registered[name] {
					return
				}
				registered[name] = true
				i := idx
				env = env.Extend(name, &core.BuiltinFn{
					Name: name,
					Fn: func(v core.Value) (core.Value, error) {
						lv, ok := v.(core.ListVal)
						if !ok {
							return nil, fmt.Errorf("field accessor %q: not a composite value", name)
						}
						if i < 0 || i >= len(lv) {
							return nil, fmt.Errorf("field accessor %q: index %d out of range", name, i)
						}
						return lv[i], nil
					},
				})
			}
			register(strings.ToLower(f.ShenName), f.Index)
			register(f.ShenName, f.Index)
		}
	}

	// Bind parameters.
	for i, pname := range def.ParamNames {
		env = env.Extend(pname, args[i].Value)
	}
	return env
}

// goLiteralFor converts an evaluated spec value to a Go source expression
// matching the spec's return type.
func goLiteralFor(v core.Value, shenType string, tt *specfile.TypeTable) (string, error) {
	shenType = strings.TrimSpace(shenType)

	// list types
	if elem := specfile.ElemType(shenType); elem != "" {
		lv, ok := v.(core.ListVal)
		if !ok {
			return "", fmt.Errorf("expected list for %s, got %T", shenType, v)
		}
		goElem := tt.GoType(elem)
		parts := make([]string, len(lv))
		for i, e := range lv {
			s, err := goLiteralFor(e, elem, tt)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return fmt.Sprintf("[]%s{%s}", goElem, strings.Join(parts, ", ")), nil
	}

	// primitives
	switch shenType {
	case "number":
		switch x := v.(type) {
		case core.IntVal:
			return fmt.Sprintf("%d", int64(x)), nil
		case core.FloatVal:
			return formatFloatLiteral(float64(x)), nil
		}
	case "string", "symbol":
		if s, ok := v.(core.StringVal); ok {
			return fmt.Sprintf("%q", string(s)), nil
		}
	case "boolean":
		if b, ok := v.(core.BoolVal); ok {
			if bool(b) {
				return "true", nil
			}
			return "false", nil
		}
	}

	// wrapper/constrained: return Go primitive literal (the test compares
	// against the implementation's direct return if it's a primitive, or
	// uses mustXxx if it returns the guard type). For v1 we treat the
	// return type as primitive — most "observation" functions return bool
	// or number.
	if entry, ok := tt.Entries[shenType]; ok {
		switch entry.Category {
		case specfile.CatWrapper, specfile.CatConstrained:
			return goLiteralFor(v, entry.ShenPrim, tt)
		}
	}

	return "", fmt.Errorf("cannot produce Go literal for %s value %T", shenType, v)
}

// Emit writes the complete Go test source to buf.
func (h *Harness) Emit() (string, error) {
	var b bytes.Buffer
	cfg := h.Config

	b.WriteString("// Code generated by shen-derive. DO NOT EDIT.\n")
	b.WriteString("// Regenerate with: shen-derive verify <spec.shen> --func " + cfg.Spec.Name + "\n")
	b.WriteString("//\n")
	b.WriteString("// This file checks that " + cfg.ImplFunc + " matches the Shen spec\n")
	b.WriteString("// by evaluating the spec on sampled inputs and comparing outputs.\n\n")

	b.WriteString("package " + cfg.TestPkgName + "\n\n")

	// Imports.
	imports := []string{`"testing"`}
	if cfg.TypeTable.ImportPath != "" {
		imports = append(imports, fmt.Sprintf("%s %q", cfg.TypeTable.ImportAlias, cfg.TypeTable.ImportPath))
	}
	if cfg.ImplPkgPath != "" && cfg.ImplPkgName != cfg.TestPkgName {
		imports = append(imports, fmt.Sprintf("%s %q", cfg.ImplPkgName, cfg.ImplPkgPath))
	}
	b.WriteString("import (\n")
	for _, imp := range imports {
		b.WriteString("\t" + imp + "\n")
	}
	b.WriteString(")\n\n")

	// Helper constructors for every type referenced via "mustXxx".
	helpers := collectHelpers(h, cfg.TypeTable)
	for _, helper := range helpers {
		b.WriteString(helper)
		b.WriteString("\n")
	}

	// Determine how the test compares return values.
	// For wrapper/constrained return types, the implementation function
	// returns the guard type (e.g. shenguard.Amount) but the spec evaluates
	// to the underlying primitive. The test compares `got.Val()` against a
	// primitive `want`.
	retShenType := cfg.Spec.TypeSig.ReturnType
	wantGoType := cfg.TypeTable.GoType(retShenType)
	gotExpr := "got"
	if entry, ok := cfg.TypeTable.Entries[retShenType]; ok {
		switch entry.Category {
		case specfile.CatWrapper, specfile.CatConstrained:
			wantGoType = entry.GoPrimType
			gotExpr = "got.Val()"
		case specfile.CatComposite, specfile.CatGuarded, specfile.CatSumType, specfile.CatAlias:
			return "", fmt.Errorf("verify: return type %q (category %s) not yet supported",
				retShenType, entry.Category)
		}
	}

	// The test function.
	testName := "TestSpec_" + cfg.ImplFunc
	fmt.Fprintf(&b, "func %s(t *testing.T) {\n", testName)
	fmt.Fprintf(&b, "\tcases := []struct {\n")
	fmt.Fprintf(&b, "\t\tname string\n")
	for i, pname := range cfg.Spec.ParamNames {
		goTy := cfg.TypeTable.GoType(cfg.Spec.TypeSig.ParamTypes[i])
		fmt.Fprintf(&b, "\t\t%s %s\n", lowerFirst(pname), goTy)
	}
	fmt.Fprintf(&b, "\t\twant %s\n", wantGoType)
	fmt.Fprintf(&b, "\t}{\n")
	for _, c := range h.Cases {
		fmt.Fprintf(&b, "\t\t{\n")
		fmt.Fprintf(&b, "\t\t\tname: %q,\n", c.Name)
		for i, pname := range cfg.Spec.ParamNames {
			fmt.Fprintf(&b, "\t\t\t%s: %s,\n", lowerFirst(pname), c.Args[i].GoExpr)
		}
		fmt.Fprintf(&b, "\t\t\twant: %s,\n", c.ExpectedGo)
		fmt.Fprintf(&b, "\t\t},\n")
	}
	fmt.Fprintf(&b, "\t}\n")

	// The test loop.
	fmt.Fprintf(&b, "\tfor _, tc := range cases {\n")
	fmt.Fprintf(&b, "\t\tt.Run(tc.name, func(t *testing.T) {\n")

	implQualifier := ""
	if cfg.ImplPkgName != cfg.TestPkgName && cfg.ImplPkgName != "" {
		implQualifier = cfg.ImplPkgName + "."
	}
	argList := make([]string, len(cfg.Spec.ParamNames))
	for i, pname := range cfg.Spec.ParamNames {
		argList[i] = "tc." + lowerFirst(pname)
	}
	fmt.Fprintf(&b, "\t\t\tgot := %s%s(%s)\n", implQualifier, cfg.ImplFunc, strings.Join(argList, ", "))

	if isSliceReturn(retShenType) {
		fmt.Fprintf(&b, "\t\t\tif !sliceEq(%s, tc.want) {\n", gotExpr)
	} else {
		fmt.Fprintf(&b, "\t\t\tif %s != tc.want {\n", gotExpr)
	}
	fmt.Fprintf(&b, "\t\t\t\tt.Fatalf(\"%%s: spec says %%v, impl returned %%v\", tc.name, tc.want, got)\n")
	fmt.Fprintf(&b, "\t\t\t}\n")
	fmt.Fprintf(&b, "\t\t})\n")
	fmt.Fprintf(&b, "\t}\n")
	fmt.Fprintf(&b, "}\n")

	if isSliceReturn(retShenType) {
		fmt.Fprintf(&b, "\nfunc sliceEq[T comparable](a, b []T) bool {\n")
		fmt.Fprintf(&b, "\tif len(a) != len(b) {\n\t\treturn false\n\t}\n")
		fmt.Fprintf(&b, "\tfor i := range a {\n\t\tif a[i] != b[i] {\n\t\t\treturn false\n\t\t}\n\t}\n")
		fmt.Fprintf(&b, "\treturn true\n")
		fmt.Fprintf(&b, "}\n")
	}

	return b.String(), nil
}

// collectHelpers scans the harness's generated Go expressions for
// references to "mustXxx" helper constructors and emits a Go source body
// for each one.
func collectHelpers(h *Harness, tt *specfile.TypeTable) []string {
	needed := map[string]bool{}
	var walk func(s string)
	walk = func(s string) {
		for {
			idx := strings.Index(s, "must")
			if idx == -1 {
				return
			}
			end := idx + 4
			for end < len(s) && isIdentRune(s[end]) {
				end++
			}
			if end > idx+4 {
				needed[s[idx:end]] = true
			}
			s = s[end:]
		}
	}
	for _, c := range h.Cases {
		for _, arg := range c.Args {
			walk(arg.GoExpr)
		}
		walk(c.ExpectedGo)
	}

	// Also walk helpers that reference other helpers transitively.
	// Start simple: resolve in one pass for the types we find, which
	// is enough for payment (Transaction's helper calls mustAccountId).
	for {
		added := false
		for _, entry := range tt.Entries {
			name := "must" + entry.GoName
			if !needed[name] {
				continue
			}
			// Composite helpers reference other helpers.
			for _, f := range entry.Fields {
				if fEntry, ok := tt.Entries[f.ShenType]; ok {
					depName := "must" + fEntry.GoName
					if !needed[depName] {
						needed[depName] = true
						added = true
					}
				}
			}
		}
		if !added {
			break
		}
	}

	// Emit in deterministic order.
	names := make([]string, 0, len(needed))
	for n := range needed {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []string
	for _, name := range names {
		shenName := shenNameForHelper(name, tt)
		if shenName == "" {
			continue
		}
		entry := tt.Entries[shenName]
		if entry == nil {
			continue
		}
		out = append(out, emitHelper(entry, tt))
	}
	return out
}

// shenNameForHelper finds the entry whose GoName matches "Xxx" in "mustXxx".
func shenNameForHelper(helper string, tt *specfile.TypeTable) string {
	goName := strings.TrimPrefix(helper, "must")
	for shen, entry := range tt.Entries {
		if entry.GoName == goName {
			return shen
		}
	}
	return ""
}

// emitHelper produces a Go source snippet defining the mustXxx helper for
// one type entry. For wrappers/constrained it unwraps the error. For
// composites it delegates to the generated constructor.
func emitHelper(entry *specfile.TypeEntry, tt *specfile.TypeTable) string {
	helperName := "must" + entry.GoName
	qualified := entry.GoQualified
	constructor := "New" + entry.GoName
	if tt.ImportAlias != "" {
		constructor = tt.ImportAlias + "." + constructor
	}

	switch entry.Category {
	case specfile.CatWrapper:
		return fmt.Sprintf(
			"func %s(x %s) %s { return %s(x) }\n",
			helperName, entry.GoPrimType, qualified, constructor,
		)

	case specfile.CatConstrained:
		return fmt.Sprintf(
			"func %s(x %s) %s { v, err := %s(x); if err != nil { panic(err) }; return v }\n",
			helperName, entry.GoPrimType, qualified, constructor,
		)

	case specfile.CatComposite, specfile.CatGuarded:
		params := make([]string, len(entry.Fields))
		argNames := make([]string, len(entry.Fields))
		for i, f := range entry.Fields {
			argNames[i] = fmt.Sprintf("a%d", i)
			goTy := tt.GoType(f.ShenType)
			params[i] = fmt.Sprintf("%s %s", argNames[i], goTy)
		}
		if entry.Category == specfile.CatGuarded {
			return fmt.Sprintf(
				"func %s(%s) %s { v, err := %s(%s); if err != nil { panic(err) }; return v }\n",
				helperName, strings.Join(params, ", "), qualified, constructor, strings.Join(argNames, ", "),
			)
		}
		return fmt.Sprintf(
			"func %s(%s) %s { return %s(%s) }\n",
			helperName, strings.Join(params, ", "), qualified, constructor, strings.Join(argNames, ", "),
		)
	}
	return ""
}

// --- small helpers ---

func isIdentRune(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'A' && s[0] <= 'Z' {
		return string(s[0]+32) + s[1:]
	}
	return s
}

func isSliceReturn(shenType string) bool {
	return strings.HasPrefix(strings.TrimSpace(shenType), "(list ")
}

// formatFloatLiteral produces a Go numeric literal for a float64 value.
// Whole-integer floats get ".0" appended to disambiguate from int in
// Go's untyped constant rules (so the literal is always float64).
func formatFloatLiteral(f float64) string {
	s := fmt.Sprintf("%g", f)
	// If the result looks like an integer, add ".0" so Go sees it as float.
	hasDot := false
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == 'e' || s[i] == 'E' {
			hasDot = true
			break
		}
	}
	if !hasDot {
		s += ".0"
	}
	return s
}
