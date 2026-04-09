package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/shen"
)

const updateCorpusArtifactsEnv = "SHEN_DERIVE_UPDATE_CORPUS_ARTIFACTS"

type corpusRewrite struct {
	rule  *laws.Rule
	path  laws.Path
	extra laws.Bindings
}

type corpusCase struct {
	name string
	args []core.Term
	want core.Term
}

type corpusTarget struct {
	name     string
	funcName string
	artifact string
	spec     core.Term
	rewrites []corpusRewrite
	cases    []corpusCase
}

type compiledCorpusCase struct {
	name        string
	args        []string
	wantLiteral string
}

func TestV1Corpus(t *testing.T) {
	targets := v1CorpusTargets()
	if got := len(targets); got != 20 {
		t.Fatalf("v1 corpus drift: got %d targets, want 20", got)
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			runCorpusTarget(t, target)
		})
	}
}

func TestPhase9ExpansionTargets(t *testing.T) {
	targets := phase9ExpansionTargets()
	if got := len(targets); got != 4 {
		t.Fatalf("phase 9 expansion drift: got %d targets, want 4", got)
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			runCorpusTarget(t, target)
		})
	}
}

func runCorpusTarget(t *testing.T, target corpusTarget) {
	t.Helper()

	if _, err := core.CheckTerm(target.spec); err != nil {
		t.Fatalf("CheckTerm spec: %v", err)
	}

	derived := target.spec
	for _, step := range target.rewrites {
		if step.rule == nil || step.rule.Name == "" {
			t.Fatal("rewrite step must use a named law")
		}
		result, err := shen.RewriteStrict(derived, step.rule, step.path, step.extra)
		if err != nil {
			t.Fatalf("RewriteStrict(%s): %v", step.rule.Name, err)
		}
		if len(result.Obligations) != 0 {
			t.Fatalf("core corpus rewrites must be unconditional; %s left %d obligations", step.rule.Name, len(result.Obligations))
		}
		derived = result.Rewritten
	}

	derivedTy, err := core.CheckTerm(derived)
	if err != nil {
		t.Fatalf("CheckTerm derived: %v", err)
	}

	paramTypes, resultTy := unwrapFuncType(derivedTy)
	compiledCases := make([]compiledCorpusCase, 0, len(target.cases))
	for _, tc := range target.cases {
		specVal := evalApplied(t, target.spec, tc.args)
		derivedVal := evalApplied(t, derived, tc.args)
		wantVal := evalClosed(t, tc.want)

		if specVal.String() != wantVal.String() {
			t.Fatalf("%s: spec value = %s, want %s", tc.name, specVal, wantVal)
		}
		if derivedVal.String() != wantVal.String() {
			t.Fatalf("%s: derived value = %s, want %s", tc.name, derivedVal, wantVal)
		}

		if len(tc.args) != len(paramTypes) {
			t.Fatalf("%s: got %d args for %d parameters", tc.name, len(tc.args), len(paramTypes))
		}

		argLiterals := make([]string, len(tc.args))
		for i, arg := range tc.args {
			argLiterals[i] = goLiteralForValue(t, evalClosed(t, arg), paramTypes[i])
		}
		compiledCases = append(compiledCases, compiledCorpusCase{
			name:        tc.name,
			args:        argLiterals,
			wantLiteral: goLiteralForValue(t, wantVal, resultTy),
		})
	}

	goCode, err := LowerToGo(derived, derivedTy, target.funcName, "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	assertArtifact(t, target.artifact, target.name, goCode)
	compileAndTest(t, goCode, renderCompiledCorpusTest(target.funcName, resultTy, compiledCases))
}

func unwrapFuncType(ty core.Type) ([]core.Type, core.Type) {
	var params []core.Type
	cur := ty
	for {
		fn, ok := cur.(*core.TFun)
		if !ok {
			return params, cur
		}
		params = append(params, fn.Param)
		cur = fn.Result
	}
}

func evalApplied(t *testing.T, fn core.Term, args []core.Term) core.Value {
	t.Helper()
	return evalClosed(t, core.MkApps(fn, args...))
}

func evalClosed(t *testing.T, term core.Term) core.Value {
	t.Helper()
	v, err := core.Eval(core.EmptyEnv(), term)
	if err != nil {
		t.Fatalf("Eval(%s): %v", core.PrettyPrint(term), err)
	}
	return v
}

func goLiteralForValue(t *testing.T, v core.Value, ty core.Type) string {
	t.Helper()

	switch ty := ty.(type) {
	case *core.TInt:
		n, ok := v.(core.IntVal)
		if !ok {
			t.Fatalf("want int literal, got %T", v)
		}
		return fmt.Sprintf("%d", int64(n))
	case *core.TBool:
		b, ok := v.(core.BoolVal)
		if !ok {
			t.Fatalf("want bool literal, got %T", v)
		}
		if bool(b) {
			return "true"
		}
		return "false"
	case *core.TString:
		s, ok := v.(core.StringVal)
		if !ok {
			t.Fatalf("want string literal, got %T", v)
		}
		return fmt.Sprintf("%q", string(s))
	case *core.TList:
		xs, ok := v.(core.ListVal)
		if !ok {
			t.Fatalf("want list literal, got %T", v)
		}
		parts := make([]string, len(xs))
		for i, elem := range xs {
			parts[i] = goLiteralForValue(t, elem, ty.Elem)
		}
		return fmt.Sprintf("%s{%s}", GoType(ty), strings.Join(parts, ", "))
	case *core.TTuple:
		pair, ok := v.(*core.TupleVal)
		if !ok {
			t.Fatalf("want tuple literal, got %T", v)
		}
		return fmt.Sprintf("%s{%s, %s}",
			GoType(ty),
			goLiteralForValue(t, pair.Fst, ty.Fst),
			goLiteralForValue(t, pair.Snd, ty.Snd),
		)
	default:
		t.Fatalf("unsupported literal type %T", ty)
		return ""
	}
}

func renderCompiledCorpusTest(funcName string, resultTy core.Type, cases []compiledCorpusCase) string {
	var b strings.Builder
	b.WriteString("package derived\n\n")
	if needsSliceCompare(resultTy) {
		b.WriteString("import (\n\t\"slices\"\n\t\"testing\"\n)\n\n")
	} else if needsReflect(resultTy) {
		b.WriteString("import (\n\t\"reflect\"\n\t\"testing\"\n)\n\n")
	} else {
		b.WriteString("import \"testing\"\n\n")
	}
	fmt.Fprintf(&b, "func Test%sCorpus(t *testing.T) {\n", funcName)
	for _, tc := range cases {
		fmt.Fprintf(&b, "\tt.Run(%q, func(t *testing.T) {\n", tc.name)
		fmt.Fprintf(&b, "\t\tgot := %s(%s)\n", funcName, strings.Join(tc.args, ", "))
		fmt.Fprintf(&b, "\t\twant := %s\n", tc.wantLiteral)
		if needsSliceCompare(resultTy) {
			b.WriteString("\t\tif !slices.Equal(got, want) {\n")
		} else if needsReflect(resultTy) {
			b.WriteString("\t\tif !reflect.DeepEqual(got, want) {\n")
		} else {
			b.WriteString("\t\tif got != want {\n")
		}
		fmt.Fprintf(&b, "\t\t\tt.Fatalf(\"got = %%v, want %%v\", got, want)\n")
		b.WriteString("\t\t}\n")
		b.WriteString("\t})\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func needsSliceCompare(ty core.Type) bool {
	_, ok := ty.(*core.TList)
	return ok
}

func needsReflect(ty core.Type) bool {
	switch ty.(type) {
	case *core.TTuple:
		return true
	default:
		return false
	}
}

func assertArtifact(t *testing.T, artifactGroup, name, actual string) {
	t.Helper()

	if artifactGroup == "" {
		artifactGroup = "corpus"
	}
	path := filepath.Join("testdata", artifactGroup, name+".go")
	actual = normalizeCorpusArtifact(actual)

	if os.Getenv(updateCorpusArtifactsEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir artifact dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(actual+"\n"), 0644); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
	}

	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact %s: %v", path, err)
	}
	if want := normalizeCorpusArtifact(string(wantBytes)); want != actual {
		t.Fatalf("artifact %s is stale\n--- want ---\n%s\n--- actual ---\n%s", path, want, actual)
	}
}

func normalizeCorpusArtifact(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}

func mkIntList(xs ...int64) core.Term {
	elems := make([]core.Term, len(xs))
	for i, x := range xs {
		elems[i] = core.MkInt(x)
	}
	return core.MkList(elems...)
}

func mkBoolList(xs ...bool) core.Term {
	elems := make([]core.Term, len(xs))
	for i, x := range xs {
		elems[i] = core.MkBool(x)
	}
	return core.MkList(elems...)
}

func mkStringList(xs ...string) core.Term {
	elems := make([]core.Term, len(xs))
	for i, x := range xs {
		elems[i] = core.MkStr(x)
	}
	return core.MkList(elems...)
}

func mkBalanceStep() core.Term {
	return core.MkLam("state", nil,
		core.MkLam("tx", &core.TInt{},
			core.MkLet("next",
				core.MkApps(core.MkPrim(core.PrimSub),
					core.MkApp(core.MkPrim(core.PrimFst), core.MkVar("state")),
					core.MkVar("tx"),
				),
				core.MkTuple(
					core.MkVar("next"),
					core.MkApps(core.MkPrim(core.PrimAnd),
						core.MkApp(core.MkPrim(core.PrimSnd), core.MkVar("state")),
						core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("next"), core.MkInt(0)),
					),
				),
			),
		),
	)
}

func mkGeq0() core.Term {
	return core.MkLam("x", &core.TInt{},
		core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
	)
}

func mkAll(p core.Term) core.Term {
	return core.MkApps(core.MkPrim(core.PrimFoldr),
		core.MkLam("x", nil,
			core.MkLam("acc", nil,
				core.MkApps(core.MkPrim(core.PrimAnd),
					core.MkApp(p, core.MkVar("x")),
					core.MkVar("acc"),
				),
			),
		),
		core.MkBool(true),
	)
}

func mkProcessableSpec() core.Term {
	apply := core.MkLam("b", &core.TInt{},
		core.MkLam("tx", &core.TInt{},
			core.MkApps(core.MkPrim(core.PrimSub), core.MkVar("b"), core.MkVar("tx")),
		),
	)
	return core.MkLam("b0", &core.TInt{},
		core.MkLam("txs", &core.TList{Elem: &core.TInt{}},
			core.MkApp(mkAll(mkGeq0()),
				core.MkApps(core.MkPrim(core.PrimScanl),
					apply,
					core.MkVar("b0"),
					core.MkVar("txs"),
				),
			),
		),
	)
}

func mkSafeUnderLimitSpec() core.Term {
	return core.MkLam("limit", &core.TInt{},
		core.MkLam("xs", &core.TList{Elem: &core.TInt{}},
			core.MkApps(core.MkPrim(core.PrimFoldr),
				core.MkLam("x", &core.TInt{},
					core.MkLam("acc", &core.TBool{},
						core.MkIf(
							core.MkApps(core.MkPrim(core.PrimLe), core.MkVar("x"), core.MkVar("limit")),
							core.MkVar("acc"),
							core.MkBool(false),
						),
					),
				),
				core.MkBool(true),
				core.MkVar("xs"),
			),
		),
	)
}

func mkPrefixNonzeroSpec() core.Term {
	return core.MkLam("xs", &core.TList{Elem: &core.TInt{}},
		core.MkApps(core.MkPrim(core.PrimScanl),
			core.MkLam("acc", &core.TBool{},
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimAnd),
						core.MkVar("acc"),
						core.MkApps(core.MkPrim(core.PrimNeq), core.MkVar("x"), core.MkInt(0)),
					),
				),
			),
			core.MkBool(true),
			core.MkVar("xs"),
		),
	)
}

func mkEqualZeroFlagsSpec() core.Term {
	return core.MkApp(core.MkPrim(core.PrimMap),
		core.MkLam("x", &core.TInt{},
			core.MkApps(core.MkPrim(core.PrimEq), core.MkVar("x"), core.MkInt(0)),
		),
	)
}

func mkStringEqFlagsSpec() core.Term {
	return core.MkLam("want", &core.TString{},
		core.MkLam("xs", &core.TList{Elem: &core.TString{}},
			core.MkApps(core.MkPrim(core.PrimMap),
				core.MkLam("s", &core.TString{},
					core.MkApps(core.MkPrim(core.PrimEq), core.MkVar("s"), core.MkVar("want")),
				),
				core.MkVar("xs"),
			),
		),
	)
}

func v1CorpusTargets() []corpusTarget {
	sum := corpusTarget{
		name:     "sum",
		funcName: "Sum",
		spec: core.MkApps(core.MkPrim(core.PrimFoldl),
			core.MkPrim(core.PrimAdd),
			core.MkInt(0),
		),
		cases: []corpusCase{
			{name: "mixed-signs", args: []core.Term{mkIntList(-1, 2, -3)}, want: core.MkInt(-2)},
			{name: "empty", args: []core.Term{mkIntList()}, want: core.MkInt(0)},
			{name: "ascending", args: []core.Term{mkIntList(1, 2, 3, 4, 5)}, want: core.MkInt(15)},
		},
	}

	runningSum := corpusTarget{
		name:     "running-sum",
		funcName: "RunningSum",
		spec: core.MkApps(core.MkPrim(core.PrimScanl),
			core.MkPrim(core.PrimAdd),
			core.MkInt(0),
		),
		cases: []corpusCase{
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList(0)},
			{name: "positive", args: []core.Term{mkIntList(1, 2, 3)}, want: mkIntList(0, 1, 3, 6)},
			{name: "mixed", args: []core.Term{mkIntList(2, -1, 5)}, want: mkIntList(0, 2, 1, 6)},
		},
	}

	squares := corpusTarget{
		name:     "squares",
		funcName: "Squares",
		spec: core.MkApp(core.MkPrim(core.PrimMap),
			core.MkLam("x", &core.TInt{},
				core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkVar("x")),
			),
		),
		cases: []corpusCase{
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList()},
			{name: "positive", args: []core.Term{mkIntList(1, 2, 3, 4)}, want: mkIntList(1, 4, 9, 16)},
			{name: "mixed", args: []core.Term{mkIntList(-2, 0, 3)}, want: mkIntList(4, 0, 9)},
		},
	}

	filterPositive := corpusTarget{
		name:     "filter-positive",
		funcName: "FilterPositive",
		spec: core.MkApp(core.MkPrim(core.PrimFilter),
			core.MkLam("x", &core.TInt{},
				core.MkApps(core.MkPrim(core.PrimGt), core.MkVar("x"), core.MkInt(0)),
			),
		),
		cases: []corpusCase{
			{name: "mixed", args: []core.Term{mkIntList(-2, -1, 0, 1, 2, 3)}, want: mkIntList(1, 2, 3)},
			{name: "all-negative", args: []core.Term{mkIntList(-3, -2, -1)}, want: mkIntList()},
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList()},
		},
	}

	allNonNegative := corpusTarget{
		name:     "all-non-negative",
		funcName: "AllNonNegative",
		spec: core.MkApps(core.MkPrim(core.PrimFoldr),
			core.MkLam("x", &core.TInt{},
				core.MkLam("acc", &core.TBool{},
					core.MkIf(
						core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
						core.MkVar("acc"),
						core.MkBool(false),
					),
				),
			),
			core.MkBool(true),
		),
		cases: []corpusCase{
			{name: "all-good", args: []core.Term{mkIntList(1, 2, 3, 4)}, want: core.MkBool(true)},
			{name: "contains-negative", args: []core.Term{mkIntList(1, -2, 3)}, want: core.MkBool(false)},
			{name: "empty", args: []core.Term{mkIntList()}, want: core.MkBool(true)},
		},
	}

	sumPositives := corpusTarget{
		name:     "sum-positives",
		funcName: "SumPositives",
		spec: core.MkApps(core.MkPrim(core.PrimFoldr),
			core.MkLam("x", &core.TInt{},
				core.MkLam("acc", &core.TInt{},
					core.MkIf(
						core.MkApps(core.MkPrim(core.PrimGt), core.MkVar("x"), core.MkInt(0)),
						core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkVar("acc")),
						core.MkVar("acc"),
					),
				),
			),
			core.MkInt(0),
		),
		cases: []corpusCase{
			{name: "mixed", args: []core.Term{mkIntList(1, -2, 3, -4, 5)}, want: core.MkInt(9)},
			{name: "all-negative", args: []core.Term{mkIntList(-1, -2)}, want: core.MkInt(0)},
			{name: "empty", args: []core.Term{mkIntList()}, want: core.MkInt(0)},
		},
	}

	singletonBool := corpusTarget{
		name:     "singleton-bool",
		funcName: "SingletonBool",
		spec: core.MkLam("x", &core.TBool{},
			core.MkList(core.MkVar("x")),
		),
		cases: []corpusCase{
			{name: "true", args: []core.Term{core.MkBool(true)}, want: mkBoolList(true)},
			{name: "false", args: []core.Term{core.MkBool(false)}, want: mkBoolList(false)},
			{name: "true-again", args: []core.Term{core.MkBool(true)}, want: mkBoolList(true)},
		},
	}

	incAllLet := corpusTarget{
		name:     "inc-all-let",
		funcName: "IncAllLet",
		spec: core.MkApp(core.MkPrim(core.PrimMap),
			core.MkLam("x", &core.TInt{},
				core.MkLet("y",
					core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)),
					core.MkVar("y"),
				),
			),
		),
		cases: []corpusCase{
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList()},
			{name: "positive", args: []core.Term{mkIntList(1, 2, 3)}, want: mkIntList(2, 3, 4)},
			{name: "mixed", args: []core.Term{mkIntList(-1, 0, 5)}, want: mkIntList(0, 1, 6)},
		},
	}

	nonNegFlags := corpusTarget{
		name:     "non-neg-flags",
		funcName: "NonNegFlags",
		spec: core.MkApps(core.MkPrim(core.PrimFoldr),
			core.MkLam("x", &core.TInt{},
				core.MkLam("xs", &core.TList{Elem: &core.TBool{}},
					core.MkApps(core.MkPrim(core.PrimCons),
						core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
						core.MkVar("xs"),
					),
				),
			),
			core.MkPrim(core.PrimNil),
		),
		cases: []corpusCase{
			{name: "mixed", args: []core.Term{mkIntList(-1, 0, 2)}, want: mkBoolList(false, true, true)},
			{name: "all-negative", args: []core.Term{mkIntList(-3, -2)}, want: mkBoolList(false, false)},
			{name: "empty", args: []core.Term{mkIntList()}, want: mkBoolList()},
		},
	}

	identityBools := corpusTarget{
		name:     "identity-bools",
		funcName: "IdentityBools",
		spec: core.MkApp(core.MkPrim(core.PrimMap),
			core.MkLam("x", &core.TBool{},
				core.MkIf(core.MkVar("x"), core.MkVar("x"), core.MkBool(false)),
			),
		),
		cases: []corpusCase{
			{name: "mixed", args: []core.Term{mkBoolList(true, false, true)}, want: mkBoolList(true, false, true)},
			{name: "all-false", args: []core.Term{mkBoolList(false, false)}, want: mkBoolList(false, false)},
			{name: "empty", args: []core.Term{mkBoolList()}, want: mkBoolList()},
		},
	}

	paymentProcessable := corpusTarget{
		name:     "payment-processable",
		funcName: "PaymentProcessable",
		spec:     mkProcessableSpec(),
		rewrites: []corpusRewrite{
			{rule: laws.AllScanlFusion(), path: laws.Path{0, 0}},
		},
		cases: []corpusCase{
			{name: "overspend", args: []core.Term{core.MkInt(100), mkIntList(30, 50, 40)}, want: core.MkBool(false)},
			{name: "safe", args: []core.Term{core.MkInt(100), mkIntList(30, 20, 10)}, want: core.MkBool(true)},
			{name: "negative-initial", args: []core.Term{core.MkInt(-1), mkIntList()}, want: core.MkBool(false)},
			{name: "exact-depletion", args: []core.Term{core.MkInt(5), mkIntList(3, 2)}, want: core.MkBool(true)},
		},
	}

	product := corpusTarget{
		name:     "product",
		funcName: "Product",
		spec: core.MkApps(core.MkPrim(core.PrimFoldl),
			core.MkPrim(core.PrimMul),
			core.MkInt(1),
		),
		cases: []corpusCase{
			{name: "empty", args: []core.Term{mkIntList()}, want: core.MkInt(1)},
			{name: "positive", args: []core.Term{mkIntList(2, 3, 4)}, want: core.MkInt(24)},
			{name: "mixed-signs", args: []core.Term{mkIntList(-1, 2, -3)}, want: core.MkInt(6)},
		},
	}

	runningProduct := corpusTarget{
		name:     "running-product",
		funcName: "RunningProduct",
		spec: core.MkApps(core.MkPrim(core.PrimScanl),
			core.MkPrim(core.PrimMul),
			core.MkInt(1),
		),
		cases: []corpusCase{
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList(1)},
			{name: "positive", args: []core.Term{mkIntList(2, 3, 4)}, want: mkIntList(1, 2, 6, 24)},
			{name: "contains-zero", args: []core.Term{mkIntList(5, 0, 2)}, want: mkIntList(1, 5, 0, 0)},
		},
	}

	anyNegative := corpusTarget{
		name:     "any-negative",
		funcName: "AnyNegative",
		spec: core.MkApps(core.MkPrim(core.PrimFoldr),
			core.MkLam("x", &core.TInt{},
				core.MkLam("acc", &core.TBool{},
					core.MkIf(
						core.MkApps(core.MkPrim(core.PrimLt), core.MkVar("x"), core.MkInt(0)),
						core.MkBool(true),
						core.MkVar("acc"),
					),
				),
			),
			core.MkBool(false),
		),
		cases: []corpusCase{
			{name: "contains-negative", args: []core.Term{mkIntList(1, -2, 3)}, want: core.MkBool(true)},
			{name: "all-non-negative", args: []core.Term{mkIntList(0, 2, 4)}, want: core.MkBool(false)},
			{name: "empty", args: []core.Term{mkIntList()}, want: core.MkBool(false)},
		},
	}

	countNonNegative := corpusTarget{
		name:     "count-non-negative",
		funcName: "CountNonNegative",
		spec: core.MkApps(core.MkPrim(core.PrimFoldl),
			core.MkLam("n", &core.TInt{},
				core.MkLam("x", &core.TInt{},
					core.MkIf(
						core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
						core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("n"), core.MkInt(1)),
						core.MkVar("n"),
					),
				),
			),
			core.MkInt(0),
		),
		cases: []corpusCase{
			{name: "mixed", args: []core.Term{mkIntList(-1, 0, 2)}, want: core.MkInt(2)},
			{name: "all-negative", args: []core.Term{mkIntList(-3, -2)}, want: core.MkInt(0)},
			{name: "empty", args: []core.Term{mkIntList()}, want: core.MkInt(0)},
		},
	}

	balanceFinal := corpusTarget{
		name:     "balance-final",
		funcName: "BalanceFinal",
		spec: core.MkLam("b0", &core.TInt{},
			core.MkLam("txs", &core.TList{Elem: &core.TInt{}},
				core.MkApp(core.MkPrim(core.PrimFst),
					core.MkApps(core.MkPrim(core.PrimFoldl),
						mkBalanceStep(),
						core.MkTuple(
							core.MkVar("b0"),
							core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("b0"), core.MkInt(0)),
						),
						core.MkVar("txs"),
					),
				),
			),
		),
		cases: []corpusCase{
			{name: "overspend", args: []core.Term{core.MkInt(100), mkIntList(30, 50, 40)}, want: core.MkInt(-20)},
			{name: "safe", args: []core.Term{core.MkInt(100), mkIntList(30, 20, 10)}, want: core.MkInt(40)},
			{name: "empty-negative", args: []core.Term{core.MkInt(-1), mkIntList()}, want: core.MkInt(-1)},
		},
	}

	balanceStatePair := corpusTarget{
		name:     "balance-state-pair",
		funcName: "BalanceStatePair",
		spec: core.MkLam("b0", &core.TInt{},
			core.MkLam("txs", &core.TList{Elem: &core.TInt{}},
				core.MkApps(core.MkPrim(core.PrimFoldl),
					mkBalanceStep(),
					core.MkTuple(
						core.MkVar("b0"),
						core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("b0"), core.MkInt(0)),
					),
					core.MkVar("txs"),
				),
			),
		),
		cases: []corpusCase{
			{name: "overspend", args: []core.Term{core.MkInt(100), mkIntList(30, 50, 40)}, want: core.MkTuple(core.MkInt(-20), core.MkBool(false))},
			{name: "safe", args: []core.Term{core.MkInt(100), mkIntList(30, 20, 10)}, want: core.MkTuple(core.MkInt(40), core.MkBool(true))},
			{name: "exact-depletion", args: []core.Term{core.MkInt(5), mkIntList(3, 2)}, want: core.MkTuple(core.MkInt(0), core.MkBool(true))},
		},
	}

	mapFusionBasic := corpusTarget{
		name:     "map-fusion-basic",
		funcName: "MapFusionBasic",
		spec: core.MkApps(core.MkPrim(core.PrimCompose),
			core.MkApp(core.MkPrim(core.PrimMap),
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)),
				),
			),
			core.MkApp(core.MkPrim(core.PrimMap),
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkInt(2)),
				),
			),
		),
		rewrites: []corpusRewrite{
			{rule: laws.MapFusion(), path: laws.RootPath},
		},
		cases: []corpusCase{
			{name: "ascending", args: []core.Term{mkIntList(1, 2, 3)}, want: mkIntList(3, 5, 7)},
			{name: "mixed", args: []core.Term{mkIntList(-2, 0, 5)}, want: mkIntList(-3, 1, 11)},
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList()},
		},
	}

	mapIncThenSquare := corpusTarget{
		name:     "map-inc-then-square",
		funcName: "MapIncThenSquare",
		spec: core.MkApps(core.MkPrim(core.PrimCompose),
			core.MkApp(core.MkPrim(core.PrimMap),
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkVar("x")),
				),
			),
			core.MkApp(core.MkPrim(core.PrimMap),
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)),
				),
			),
		),
		rewrites: []corpusRewrite{
			{rule: laws.MapFusion(), path: laws.RootPath},
		},
		cases: []corpusCase{
			{name: "ascending", args: []core.Term{mkIntList(1, 2, 3)}, want: mkIntList(4, 9, 16)},
			{name: "mixed", args: []core.Term{mkIntList(-2, 0, 5)}, want: mkIntList(1, 1, 36)},
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList()},
		},
	}

	mapFoldrIdentity := corpusTarget{
		name:     "map-foldr-identity",
		funcName: "MapFoldrIdentity",
		spec: core.MkApps(core.MkPrim(core.PrimCompose),
			core.MkApp(core.MkPrim(core.PrimMap),
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)),
				),
			),
			core.MkApps(core.MkPrim(core.PrimFoldr),
				core.MkPrim(core.PrimCons),
				core.MkPrim(core.PrimNil),
			),
		),
		rewrites: []corpusRewrite{
			{rule: laws.MapFoldrFusion(), path: laws.RootPath},
		},
		cases: []corpusCase{
			{name: "ascending", args: []core.Term{mkIntList(1, 2, 3)}, want: mkIntList(2, 3, 4)},
			{name: "mixed", args: []core.Term{mkIntList(-2, 0, 5)}, want: mkIntList(-1, 1, 6)},
			{name: "empty", args: []core.Term{mkIntList()}, want: mkIntList()},
		},
	}

	return []corpusTarget{
		sum,
		runningSum,
		squares,
		filterPositive,
		allNonNegative,
		sumPositives,
		singletonBool,
		incAllLet,
		nonNegFlags,
		identityBools,
		paymentProcessable,
		product,
		runningProduct,
		anyNegative,
		countNonNegative,
		balanceFinal,
		balanceStatePair,
		mapFusionBasic,
		mapIncThenSquare,
		mapFoldrIdentity,
	}
}

func phase9ExpansionTargets() []corpusTarget {
	return []corpusTarget{
		{
			name:     "safe-under-limit",
			funcName: "SafeUnderLimit",
			artifact: "expansion",
			spec:     mkSafeUnderLimitSpec(),
			cases: []corpusCase{
				{name: "all-safe", args: []core.Term{core.MkInt(5), mkIntList(1, 5, 3)}, want: core.MkBool(true)},
				{name: "contains-breach", args: []core.Term{core.MkInt(5), mkIntList(1, 7, 3)}, want: core.MkBool(false)},
				{name: "empty", args: []core.Term{core.MkInt(-1), mkIntList()}, want: core.MkBool(true)},
			},
		},
		{
			name:     "prefix-nonzero",
			funcName: "PrefixNonzero",
			artifact: "expansion",
			spec:     mkPrefixNonzeroSpec(),
			cases: []corpusCase{
				{name: "empty", args: []core.Term{mkIntList()}, want: mkBoolList(true)},
				{name: "all-nonzero", args: []core.Term{mkIntList(3, -2, 5)}, want: mkBoolList(true, true, true, true)},
				{name: "zero-breaks-prefix", args: []core.Term{mkIntList(3, 0, 2)}, want: mkBoolList(true, true, false, false)},
			},
		},
		{
			name:     "equal-zero-flags",
			funcName: "EqualZeroFlags",
			artifact: "expansion",
			spec:     mkEqualZeroFlagsSpec(),
			cases: []corpusCase{
				{name: "mixed", args: []core.Term{mkIntList(-1, 0, 2, 0)}, want: mkBoolList(false, true, false, true)},
				{name: "all-zero", args: []core.Term{mkIntList(0, 0)}, want: mkBoolList(true, true)},
				{name: "empty", args: []core.Term{mkIntList()}, want: mkBoolList()},
			},
		},
		{
			name:     "string-eq-flags",
			funcName: "StringEqFlags",
			artifact: "expansion",
			spec:     mkStringEqFlagsSpec(),
			cases: []corpusCase{
				{name: "mixed", args: []core.Term{core.MkStr("ok"), mkStringList("ok", "no", "ok")}, want: mkBoolList(true, false, true)},
				{name: "empty-target-and-list", args: []core.Term{core.MkStr(""), mkStringList()}, want: mkBoolList()},
				{name: "empty-string-matches", args: []core.Term{core.MkStr(""), mkStringList("", "a", "")}, want: mkBoolList(true, false, true)},
			},
		},
	}
}
