package shen

import (
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
)

func mkAddVarConst(name string, n int64) core.Term {
	return core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar(name), core.MkInt(n))
}

func mkCmp(op core.PrimOp, lhs, rhs core.Term) core.Term {
	return core.MkApps(core.MkPrim(op), lhs, rhs)
}

func mkAnd(lhs, rhs core.Term) core.Term {
	return core.MkApps(core.MkPrim(core.PrimAnd), lhs, rhs)
}

func mkOr(lhs, rhs core.Term) core.Term {
	return core.MkApps(core.MkPrim(core.PrimOr), lhs, rhs)
}

func mkNot(term core.Term) core.Term {
	return core.MkApp(core.MkPrim(core.PrimNot), term)
}

func TestEmitObligation(t *testing.T) {
	// Test the side condition from foldr-fusion:
	// double (x + y) = h x (double y)
	// where double = \n -> n * 2, h = \x y -> x*2 + y

	f := core.MkLam("n", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("n"), core.MkInt(2)))
	g := core.MkPrim(core.PrimAdd)
	h := core.MkLam("a", nil, core.MkLam("b", nil,
		core.MkApps(core.MkPrim(core.PrimAdd),
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("a"), core.MkInt(2)),
			core.MkVar("b"),
		),
	))

	// Instantiated side condition: f (g x y) = h x (f y)
	cond := laws.InstantiatedCondition{
		Description: "f (g x y) = h x (f y) for all x, y",
		LHS: core.MkApp(f,
			core.MkApps(g, core.MkVar("x"), core.MkVar("y")),
		),
		RHS: core.MkApps(h,
			core.MkVar("x"),
			core.MkApp(f, core.MkVar("y")),
		),
	}

	shenSpec := EmitObligation(cond)
	t.Logf("Generated Shen spec:\n%s", shenSpec)

	// Check it contains key elements
	if !strings.Contains(shenSpec, "obligation-discharged") {
		t.Error("missing obligation-discharged conclusion")
	}
	if !strings.Contains(shenSpec, "X : number") {
		t.Error("missing X : number premise")
	}
	if !strings.Contains(shenSpec, "Y : number") {
		t.Error("missing Y : number premise")
	}
	if !strings.Contains(shenSpec, "verified") {
		t.Error("missing verified premise")
	}
}

func TestEmitObligationNoFreeVars(t *testing.T) {
	// Test with a ground equality (no free variables)
	cond := laws.InstantiatedCondition{
		Description: "2 + 3 = 5",
		LHS:         core.MkApps(core.MkPrim(core.PrimAdd), core.MkInt(2), core.MkInt(3)),
		RHS:         core.MkInt(5),
	}

	spec := EmitObligation(cond)
	t.Logf("Generated:\n%s", spec)

	if !strings.Contains(spec, "obligation-discharged") {
		t.Error("missing conclusion")
	}
}

func TestDischargeEmpiricalMapFusion(t *testing.T) {
	// map-fusion has no obligations, but let's test empirical discharge
	// with a trivially true condition: x + 0 = x
	cond := laws.InstantiatedCondition{
		Description: "x + 0 = x (identity)",
		LHS:         core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(0)),
		RHS:         core.MkVar("x"),
	}

	result := DischargeEmpirical(cond)
	if !result.Discharged {
		t.Fatalf("expected empirical discharge to succeed: %v", result.Error)
	}
	t.Logf("Empirical result: %s", result.Output)
}

func TestDischargeEmpiricalFoldrFusion(t *testing.T) {
	// foldr-fusion: double(x + y) = 2*x + 2*y (distributivity)
	// This is: (\n -> n*2)(x + y) = (\a b -> a*2 + b) x ((\n -> n*2) y)
	f := core.MkLam("n", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("n"), core.MkInt(2)))
	h := core.MkLam("a", nil, core.MkLam("b", nil,
		core.MkApps(core.MkPrim(core.PrimAdd),
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("a"), core.MkInt(2)),
			core.MkVar("b"),
		),
	))

	cond := laws.InstantiatedCondition{
		Description: "f (g x y) = h x (f y) for all x, y",
		LHS: core.MkApp(f,
			core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkVar("y")),
		),
		RHS: core.MkApps(h,
			core.MkVar("x"),
			core.MkApp(f, core.MkVar("y")),
		),
	}

	result := DischargeEmpirical(cond)
	if !result.Discharged {
		t.Fatalf("expected discharge to succeed: %v\n  output: %s", result.Error, result.Output)
	}
	t.Logf("Empirical result: %s", result.Output)
}

func TestDischargeEmpiricalFails(t *testing.T) {
	// A false side condition: x + y = x * y (not generally true)
	cond := laws.InstantiatedCondition{
		Description: "x + y = x * y (should fail)",
		LHS:         core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkVar("y")),
		RHS:         core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkVar("y")),
	}

	result := DischargeEmpirical(cond)
	if result.Discharged {
		t.Error("expected empirical discharge to fail for x+y=x*y")
	}
	t.Logf("Correctly rejected: %v (%s)", result.Error, result.Output)
}

func TestDischargeShenValidationOnly(t *testing.T) {
	shenBin := FindShenBinary()
	if shenBin == "" {
		t.Skip("Shen runtime not available")
	}

	// Test: double(x+y) = 2*x + 2*y
	f := core.MkLam("n", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("n"), core.MkInt(2)))
	h := core.MkLam("a", nil, core.MkLam("b", nil,
		core.MkApps(core.MkPrim(core.PrimAdd),
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("a"), core.MkInt(2)),
			core.MkVar("b"),
		),
	))

	cond := laws.InstantiatedCondition{
		Description: "f (g x y) = h x (f y) for all x, y",
		LHS: core.MkApp(f,
			core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkVar("y")),
		),
		RHS: core.MkApps(h,
			core.MkVar("x"),
			core.MkApp(f, core.MkVar("y")),
		),
	}

	result := DischargeShenOnly(cond)
	if result.Discharged {
		t.Fatalf("Shen tc+ should not certify quantified obligations: %+v", result)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "does not constitute a proof") {
		t.Fatalf("expected validation-only error, got: %v", result.Error)
	}
	t.Logf("Shen tc+ validation result: method=%s, output=%s", result.Method, result.Output)
}

func TestDischargeRejectsFalseGroundEquality(t *testing.T) {
	cond := laws.InstantiatedCondition{
		Description: "2 + 2 = 5",
		LHS:         core.MkApps(core.MkPrim(core.PrimAdd), core.MkInt(2), core.MkInt(2)),
		RHS:         core.MkInt(5),
	}

	result := Discharge(cond)
	if result.Discharged {
		t.Fatalf("expected false ground equality to be rejected: %+v", result)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "ground equality failed") {
		t.Fatalf("expected exact ground-eval failure, got: %v", result.Error)
	}
}

func TestDischargeSymbolicallyProvesQuantifiedObligation(t *testing.T) {
	// Arithmetic case still supported via polynomial normalization in the
	// extended symbolic fragment.
	f := core.MkLam("n", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("n"), core.MkInt(2)))
	h := core.MkLam("a", nil, core.MkLam("b", nil,
		core.MkApps(core.MkPrim(core.PrimAdd),
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("a"), core.MkInt(2)),
			core.MkVar("b"),
		),
	))

	cond := laws.InstantiatedCondition{
		Description: "f (g x y) = h x (f y) for all x, y",
		LHS: core.MkApp(f,
			core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkVar("y")),
		),
		RHS: core.MkApps(h,
			core.MkVar("x"),
			core.MkApp(f, core.MkVar("y")),
		),
	}

	result := Discharge(cond)
	if !result.Discharged {
		t.Fatalf("expected quantified obligation to be proved: %+v", result)
	}
	if result.Method != "symbolic-polynomial" {
		t.Fatalf("expected symbolic-polynomial proof, got: %+v", result)
	}
	t.Logf("Proved with symbolic-polynomial: %s", result.Output)
}

func TestDischargeLeavesUnsupportedQuantifiedObligationsUndischarged(t *testing.T) {
	cond := laws.InstantiatedCondition{
		Description: "not (not b) = b",
		LHS:         core.MkApp(core.MkPrim(core.PrimNot), core.MkApp(core.MkPrim(core.PrimNot), core.MkVar("b"))),
		RHS:         core.MkVar("b"),
	}

	result := Discharge(cond)
	if result.Discharged {
		t.Fatalf("unsupported quantified obligation should remain undischarged: %+v", result)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "not soundly discharged") {
		t.Fatalf("expected undischarged error, got: %v", result.Error)
	}
	t.Logf("Correctly treated as diagnostic-only: %s", result.Method)
}

func TestDischargeSymbolicallyProvesComparisonFormulas(t *testing.T) {
	tests := []struct {
		name string
		lhs  core.Term
		rhs  core.Term
	}{
		{
			name: "eq",
			lhs:  mkCmp(core.PrimEq, mkAddVarConst("x", 1), mkAddVarConst("x", 1)),
			rhs:  core.MkBool(true),
		},
		{
			name: "neq",
			lhs:  mkCmp(core.PrimNeq, mkAddVarConst("x", 1), mkAddVarConst("x", 2)),
			rhs:  core.MkBool(true),
		},
		{
			name: "lt",
			lhs:  mkCmp(core.PrimLt, mkAddVarConst("x", 1), mkAddVarConst("x", 2)),
			rhs:  core.MkBool(true),
		},
		{
			name: "le",
			lhs:  mkCmp(core.PrimLe, mkAddVarConst("x", 2), mkAddVarConst("x", 2)),
			rhs:  core.MkBool(true),
		},
		{
			name: "gt",
			lhs:  mkCmp(core.PrimGt, mkAddVarConst("x", 2), mkAddVarConst("x", 1)),
			rhs:  core.MkBool(true),
		},
		{
			name: "ge",
			lhs:  mkCmp(core.PrimGe, mkAddVarConst("x", 2), mkAddVarConst("x", 2)),
			rhs:  core.MkBool(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := laws.InstantiatedCondition{
				Description: tt.name + " comparison is assignment-independent",
				LHS:         tt.lhs,
				RHS:         tt.rhs,
			}

			result := Discharge(cond)
			if !result.Discharged {
				t.Fatalf("expected symbolic-fragment to prove supported comparison: %+v", result)
			}
			if result.Method != "symbolic-fragment" {
				t.Fatalf("expected symbolic-fragment, got %s: %+v", result.Method, result)
			}
			t.Logf("Proved supported %s comparison: %s", tt.name, result.Output)
		})
	}
}

func TestDischargeSymbolicallyProvesBooleanConnectives(t *testing.T) {
	tests := []struct {
		name string
		lhs  core.Term
		rhs  core.Term
	}{
		{
			name: "and",
			lhs: mkAnd(
				mkCmp(core.PrimNeq, mkAddVarConst("x", 1), mkAddVarConst("x", 2)),
				mkCmp(core.PrimGe, mkAddVarConst("x", 2), mkAddVarConst("x", 2)),
			),
			rhs: core.MkBool(true),
		},
		{
			name: "or",
			lhs: mkOr(
				mkCmp(core.PrimGt, mkAddVarConst("x", 1), mkAddVarConst("x", 2)),
				mkCmp(core.PrimEq, mkAddVarConst("x", 2), mkAddVarConst("x", 2)),
			),
			rhs: core.MkBool(true),
		},
		{
			name: "not",
			lhs:  mkNot(mkCmp(core.PrimLt, mkAddVarConst("x", 2), mkAddVarConst("x", 1))),
			rhs:  mkCmp(core.PrimLe, mkAddVarConst("x", 1), mkAddVarConst("x", 2)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := laws.InstantiatedCondition{
				Description: tt.name + " connective over supported comparisons",
				LHS:         tt.lhs,
				RHS:         tt.rhs,
			}

			result := Discharge(cond)
			if !result.Discharged {
				t.Fatalf("expected symbolic-fragment to prove boolean connective: %+v", result)
			}
			if result.Method != "symbolic-fragment" {
				t.Fatalf("expected symbolic-fragment, got %s: %+v", result.Method, result)
			}
			t.Logf("Proved supported %s connective: %s", tt.name, result.Output)
		})
	}
}

func TestDischargeRejectsVariableDependentComparisonFormulas(t *testing.T) {
	tests := []struct {
		name string
		lhs  core.Term
		rhs  core.Term
	}{
		{
			name: "eq",
			lhs:  mkCmp(core.PrimEq, core.MkVar("x"), core.MkVar("y")),
			rhs:  core.MkBool(false),
		},
		{
			name: "neq",
			lhs:  mkCmp(core.PrimNeq, core.MkVar("x"), core.MkVar("y")),
			rhs:  core.MkBool(true),
		},
		{
			name: "lt",
			lhs:  mkCmp(core.PrimLt, core.MkVar("x"), core.MkVar("y")),
			rhs:  core.MkBool(false),
		},
		{
			name: "and",
			lhs:  mkAnd(mkCmp(core.PrimLt, core.MkVar("x"), core.MkVar("y")), core.MkBool(true)),
			rhs:  core.MkBool(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := laws.InstantiatedCondition{
				Description: tt.name + " comparison depends on quantified vars",
				LHS:         tt.lhs,
				RHS:         tt.rhs,
			}

			result := Discharge(cond)
			if result.Discharged {
				t.Fatalf("variable-dependent comparison should remain undischarged: %+v", result)
			}
			if result.Error == nil || !strings.Contains(result.Error.Error(), "not soundly discharged") {
				t.Fatalf("expected diagnostic-only rejection, got: %+v", result)
			}
			t.Logf("Correctly rejected unsupported %s case: method=%s output=%s", tt.name, result.Method, result.Output)
		})
	}
}

// --- End-to-end tests ---

func TestEndToEndMapFusion(t *testing.T) {
	// Start with map f . map g, apply map-fusion, confirm no obligations.
	f := core.MkLam("x", nil, core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)))
	g := core.MkLam("x", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkInt(2)))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkApp(core.MkPrim(core.PrimMap), f),
		core.MkApp(core.MkPrim(core.PrimMap), g),
	)

	rule := laws.MapFusion()
	result, err := RewriteStrict(term, rule, laws.RootPath, nil)
	if err != nil {
		t.Fatalf("strict rewrite failed: %v", err)
	}

	// Verify result
	applied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	val, err := core.Eval(core.EmptyEnv(), applied)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	if val.String() != "[3, 5, 7]" {
		t.Errorf("expected [3, 5, 7], got %s", val)
	}
	t.Logf("map-fusion end-to-end: %s -> %s", core.PrettyPrint(term), core.PrettyPrint(result.Rewritten))
}

func TestEndToEndFoldrFusionStrictNegate(t *testing.T) {
	// negate . foldr (+) 0 => foldr (\x z -> z - x) 0
	f := core.MkPrim(core.PrimNeg)
	g := core.MkPrim(core.PrimAdd)
	e := core.MkInt(0)
	h := core.MkLam("x", nil, core.MkLam("z", nil,
		core.MkApps(core.MkPrim(core.PrimSub), core.MkVar("z"), core.MkVar("x")),
	))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	rule := laws.FoldrFusion()
	extra := laws.Bindings{"?h": h}

	result, err := RewriteStrict(term, rule, laws.RootPath, extra)
	if err != nil {
		t.Fatalf("strict rewrite failed: %v", err)
	}

	// Verify: negate(sum [1,2,3]) = negate(6) = -6
	applied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	val, err := core.Eval(core.EmptyEnv(), applied)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	if val.String() != "-6" {
		t.Errorf("expected -6, got %s", val)
	}
	t.Logf("foldr-fusion strict end-to-end: %s", core.PrettyPrint(result.Rewritten))
	t.Logf("  result on [1,2,3]: %s", val)
}

func TestEndToEndFoldrFusionStrictDouble(t *testing.T) {
	f := core.MkLam("n", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("n"), core.MkInt(2)))
	g := core.MkPrim(core.PrimAdd)
	e := core.MkInt(0)
	h := core.MkLam("x", nil, core.MkLam("y", nil,
		core.MkApps(core.MkPrim(core.PrimAdd),
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkInt(2)),
			core.MkVar("y"),
		),
	))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	result, err := RewriteStrict(term, laws.FoldrFusion(), laws.RootPath, laws.Bindings{"?h": h})
	if err != nil {
		t.Fatalf("strict rewrite failed: %v", err)
	}

	applied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	val, err := core.Eval(core.EmptyEnv(), applied)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	if val.String() != "12" {
		t.Errorf("expected 12, got %s", val)
	}
}

func TestRewriteStrictAcceptsProvedFoldrFusion(t *testing.T) {
	f := core.MkPrim(core.PrimNeg)
	g := core.MkPrim(core.PrimAdd)
	e := core.MkInt(0)
	h := core.MkLam("x", nil, core.MkLam("z", nil,
		core.MkApps(core.MkPrim(core.PrimSub), core.MkVar("z"), core.MkVar("x")),
	))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	result, err := RewriteStrict(term, laws.FoldrFusion(), laws.RootPath, laws.Bindings{"?h": h})
	if err != nil {
		t.Fatalf("expected strict rewrite to succeed, got: %v", err)
	}
	applied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	val, err := core.Eval(core.EmptyEnv(), applied)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	if val.String() != "-6" {
		t.Fatalf("expected -6, got %s", val)
	}
}

func TestRewriteStrictRejectsInvalidFoldrFusionWitness(t *testing.T) {
	term := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkPrim(core.PrimNeg),
		core.MkApps(core.MkPrim(core.PrimFoldr), core.MkPrim(core.PrimAdd), core.MkInt(0)),
	)
	badH := core.MkLam("x", nil, core.MkLam("z", nil,
		core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("z"), core.MkVar("x")),
	))

	_, err := RewriteStrict(term, laws.FoldrFusion(), laws.RootPath, laws.Bindings{"?h": badH})
	if err == nil {
		t.Fatal("expected strict rewrite to reject invalid witness")
	}
}
