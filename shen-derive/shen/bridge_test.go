package shen

import (
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
)

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

func TestDischargeShen(t *testing.T) {
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
	if !result.Discharged {
		t.Fatalf("Shen discharge failed: %v\n  output: %s", result.Error, result.Output)
	}
	t.Logf("Shen tc+ result: method=%s, output=%s", result.Method, result.Output)
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

func TestEndToEndFoldrFusionStrict(t *testing.T) {
	// negate . foldr (+) 0 => foldr (\x z -> z - x) 0
	// Side condition: negate(x+y) = (\x z -> z-x) x (negate y)
	// i.e. -(x+y) = (-y) - x = -y - x  ✓

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
