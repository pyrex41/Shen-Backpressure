package laws

import (
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// --- map-fusion tests ---

func TestMapFusionMatch(t *testing.T) {
	// Construct: map f . map g  (where f = \x -> x + 1, g = \x -> x * 2)
	f := core.MkLam("x", nil, core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)))
	g := core.MkLam("x", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkInt(2)))

	// map f . map g = compose (map f) (map g)
	term := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkApp(core.MkPrim(core.PrimMap), f),
		core.MkApp(core.MkPrim(core.PrimMap), g),
	)

	rule := MapFusion()
	result, err := Rewrite(term, rule, RootPath)
	if err != nil {
		t.Fatalf("Rewrite failed: %v", err)
	}

	// Should produce: map (compose f g)  i.e., map (f . g)
	got := core.PrettyPrint(result.Rewritten)
	t.Logf("map-fusion result: %s", got)

	// No side conditions
	if len(result.Obligations) != 0 {
		t.Errorf("expected 0 obligations, got %d", len(result.Obligations))
	}

	// Verify semantic equivalence: apply both to [1, 2, 3]
	// Original: map f (map g [1,2,3]) = map (+1) (map (*2) [1,2,3]) = map (+1) [2,4,6] = [3,5,7]
	// Rewritten: map (f.g) [1,2,3] = map (\x -> (x*2)+1) [1,2,3] = [3,5,7]
	origApplied := core.MkApps(term, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	rewrittenApplied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))

	origVal, err := core.Eval(core.EmptyEnv(), origApplied)
	if err != nil {
		t.Fatalf("eval original: %v", err)
	}
	rewrittenVal, err := core.Eval(core.EmptyEnv(), rewrittenApplied)
	if err != nil {
		t.Fatalf("eval rewritten: %v", err)
	}

	if origVal.String() != rewrittenVal.String() {
		t.Errorf("semantic mismatch: original=%s, rewritten=%s", origVal, rewrittenVal)
	}
	if origVal.String() != "[3, 5, 7]" {
		t.Errorf("unexpected result: %s", origVal)
	}
}

func TestMapFusionNoMatch(t *testing.T) {
	// This term doesn't match: map f . filter p (not map . map)
	term := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkApp(core.MkPrim(core.PrimMap), core.MkVar("f")),
		core.MkApp(core.MkPrim(core.PrimFilter), core.MkVar("p")),
	)

	rule := MapFusion()
	_, err := Rewrite(term, rule, RootPath)
	if err == nil {
		t.Error("expected match failure, got success")
	}
}

// --- map-foldr-fusion tests ---

func TestMapFoldrFusionMatch(t *testing.T) {
	// Construct: map f . foldr cons nil  (where f = \x -> x + 1)
	f := core.MkLam("x", nil, core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkApp(core.MkPrim(core.PrimMap), f),
		core.MkApps(core.MkPrim(core.PrimFoldr),
			core.MkPrim(core.PrimCons),
			core.MkPrim(core.PrimNil),
		),
	)

	rule := MapFoldrFusion()
	result, err := Rewrite(term, rule, RootPath)
	if err != nil {
		t.Fatalf("Rewrite failed: %v", err)
	}

	got := core.PrettyPrint(result.Rewritten)
	t.Logf("map-foldr-fusion result: %s", got)

	if len(result.Obligations) != 0 {
		t.Errorf("expected 0 obligations, got %d", len(result.Obligations))
	}

	// Verify semantic equivalence on [1, 2, 3]
	// Original: (map (+1) . foldr cons nil) [1,2,3] = map (+1) [1,2,3] = [2,3,4]
	// Rewritten: foldr (\x xs -> cons ((+1) x) xs) nil [1,2,3] = [2,3,4]
	origApplied := core.MkApps(term, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	rewrittenApplied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))

	origVal, err := core.Eval(core.EmptyEnv(), origApplied)
	if err != nil {
		t.Fatalf("eval original: %v", err)
	}
	rewrittenVal, err := core.Eval(core.EmptyEnv(), rewrittenApplied)
	if err != nil {
		t.Fatalf("eval rewritten: %v", err)
	}

	if origVal.String() != rewrittenVal.String() {
		t.Errorf("semantic mismatch: original=%s, rewritten=%s", origVal, rewrittenVal)
	}
	if origVal.String() != "[2, 3, 4]" {
		t.Errorf("unexpected result: %s", origVal)
	}
}

// --- foldr-fusion tests ---

func TestFoldrFusionMatch(t *testing.T) {
	// Example: length . filter p
	// length = foldr (\_ n -> n + 1) 0
	// We want to fuse: length . filter p = ... a single foldr
	//
	// But let's use a simpler concrete example first:
	// f = \xs -> length xs, where length = foldr (\_ n -> n+1) 0
	// Actually, let's use the standard example:
	//
	// sum . map (+1)  — but that's map fusion, not foldr fusion.
	//
	// For foldr-fusion specifically:
	//   f . foldr g e = foldr h (f e)
	//   provided f (g x y) = h x (f y)
	//
	// Concrete example:
	//   sum = foldr (+) 0
	//   double = \x -> x * 2
	//   double . foldr (+) 0 = foldr h (double 0)
	//   where h x y = double (x + y) ... hmm that doesn't work because
	//   we need h x (double y) = double (x + y) = 2*(x+y) = 2x + 2y
	//   and h x (double y) = h x (2y), so h x z = 2x + z
	//   So h = \x y -> 2*x + y
	//
	// Let's verify: foldr h (double 0) [1,2,3]
	//   = h 1 (h 2 (h 3 (double 0)))
	//   = h 1 (h 2 (h 3 0))
	//   = h 1 (h 2 (2*3 + 0))
	//   = h 1 (h 2 6)
	//   = h 1 (2*2 + 6)
	//   = h 1 10
	//   = 2*1 + 10
	//   = 12
	// Original: double (foldr (+) 0 [1,2,3]) = double 6 = 12  ✓

	f := core.MkLam("n", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("n"), core.MkInt(2)))
	g := core.MkPrim(core.PrimAdd)
	e := core.MkInt(0)
	h := core.MkLam("x", nil, core.MkLam("y", nil,
		core.MkApps(core.MkPrim(core.PrimAdd),
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkInt(2)),
			core.MkVar("y"),
		),
	))

	// Term: compose f (foldr g e) = f . foldr (+) 0
	term := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	rule := FoldrFusion()

	// Supply ?h as supplemental binding
	extra := Bindings{"?h": h}

	result, err := RewriteWithSupplementalBindings(term, rule, RootPath, extra)
	if err != nil {
		t.Fatalf("Rewrite failed: %v", err)
	}

	got := core.PrettyPrint(result.Rewritten)
	t.Logf("foldr-fusion result: %s", got)

	// Check that we got exactly 1 side condition
	if len(result.Obligations) != 1 {
		t.Fatalf("expected 1 obligation, got %d", len(result.Obligations))
	}

	ob := result.Obligations[0]
	t.Logf("Side condition: %s = %s", core.PrettyPrint(ob.LHS), core.PrettyPrint(ob.RHS))
	t.Logf("Description: %s", ob.Description)

	// The side condition should be:
	//   f (g x y) = h x (f y)
	// Instantiated:
	//   (\n -> n*2) ((+) x y) = (\x y -> 2*x + y) x ((\n -> n*2) y)
	// Which simplifies to: 2*(x+y) = 2*x + 2*y (distributivity!)

	// Verify the side condition holds by evaluation with concrete values
	env := core.EmptyEnv().Extend("x", core.IntVal(3)).Extend("y", core.IntVal(7))
	lhsVal, err := core.Eval(env, ob.LHS)
	if err != nil {
		t.Fatalf("eval side condition LHS: %v", err)
	}
	rhsVal, err := core.Eval(env, ob.RHS)
	if err != nil {
		t.Fatalf("eval side condition RHS: %v", err)
	}
	if lhsVal.String() != rhsVal.String() {
		t.Errorf("side condition failed for x=3, y=7: LHS=%s, RHS=%s", lhsVal, rhsVal)
	}
	t.Logf("Side condition verified for x=3, y=7: %s = %s", lhsVal, rhsVal)

	// Verify semantic equivalence: apply both to [1, 2, 3]
	origApplied := core.MkApps(term, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	rewrittenApplied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))

	origVal, err := core.Eval(core.EmptyEnv(), origApplied)
	if err != nil {
		t.Fatalf("eval original: %v", err)
	}
	rewrittenVal, err := core.Eval(core.EmptyEnv(), rewrittenApplied)
	if err != nil {
		t.Fatalf("eval rewritten: %v", err)
	}

	if origVal.String() != rewrittenVal.String() {
		t.Errorf("semantic mismatch: original=%s, rewritten=%s", origVal, rewrittenVal)
	}
	if origVal.String() != "12" {
		t.Errorf("unexpected result: %s (want 12)", origVal)
	}
}

func TestFoldrFusionSideConditionInstantiation(t *testing.T) {
	// Verify that the side condition metavariables are correctly filled.
	// Use: length . filter p
	// length = foldr (\_ n -> n + 1) 0, so:
	//   f = length = \xs -> foldr (\_ n -> n+1) 0 xs ... but that's not right
	//   for foldr-fusion, f is the outer function, not a fold.
	//
	// Actually, length IS a fold: foldr (\_ n -> n+1) 0
	// So "f . foldr g e" with f = length doesn't make sense directly
	// because length itself is a fold.
	//
	// Better example: negate . sum where sum = foldr (+) 0
	//   f = negate, g = (+), e = 0
	//   Side condition: negate (x + y) = h x (negate y)
	//   So h x z = -(x + (-z))... hmm, negate(x+y) = -x-y
	//   h x (negate y) = h x (-y) should equal -(x+y) = -x-y
	//   So h x z = -x + z  (i.e., h = \x z -> z - x)
	//
	// Verify: negate(foldr (+) 0 [1,2,3]) = negate(6) = -6
	//         foldr h (negate 0) [1,2,3] = foldr (\x z -> z-x) 0 [1,2,3]
	//         = (\x z -> z-x) 1 ((\x z -> z-x) 2 ((\x z -> z-x) 3 0))
	//         = (z-x=0-3=-3) then (z-x=-3-2=-5) then (z-x=-5-1=-6)
	//         = -6  ✓

	f := core.MkPrim(core.PrimNeg) // negate
	g := core.MkPrim(core.PrimAdd)
	e := core.MkInt(0)
	h := core.MkLam("x", nil, core.MkLam("z", nil,
		core.MkApps(core.MkPrim(core.PrimSub), core.MkVar("z"), core.MkVar("x")),
	))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	rule := FoldrFusion()
	result, err := RewriteWithSupplementalBindings(term, rule, RootPath, Bindings{"?h": h})
	if err != nil {
		t.Fatalf("Rewrite failed: %v", err)
	}

	// Verify the side condition: negate(x + y) = (\x z -> z - x) x (negate y)
	// For x=5, y=3: negate(5+3) = negate(8) = -8
	//               h 5 (negate 3) = h 5 (-3) = -3 - 5 = -8  ✓
	ob := result.Obligations[0]
	env := core.EmptyEnv().Extend("x", core.IntVal(5)).Extend("y", core.IntVal(3))
	lv, _ := core.Eval(env, ob.LHS)
	rv, _ := core.Eval(env, ob.RHS)
	if lv.String() != rv.String() {
		t.Errorf("side condition: LHS=%s, RHS=%s (should be equal)", lv, rv)
	}

	// Verify end-to-end
	origApplied := core.MkApps(term, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	rewrittenApplied := core.MkApps(result.Rewritten, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))
	ov, _ := core.Eval(core.EmptyEnv(), origApplied)
	rv2, _ := core.Eval(core.EmptyEnv(), rewrittenApplied)
	if ov.String() != rv2.String() {
		t.Errorf("semantic mismatch: %s vs %s", ov, rv2)
	}
	if ov.String() != "-6" {
		t.Errorf("expected -6, got %s", ov)
	}
}

// --- Pattern matching tests ---

func TestMatchMetaVar(t *testing.T) {
	pattern := MetaVar("?x")
	term := core.MkInt(42)

	b := Match(pattern, term)
	if b == nil {
		t.Fatal("expected match")
	}
	if !TermsEqual(b["?x"], core.MkInt(42)) {
		t.Errorf("?x bound to %v, want 42", b["?x"])
	}
}

func TestMatchConsistency(t *testing.T) {
	// Pattern: App(?f, ?f) — requires both occurrences to match the same thing
	pattern := core.MkApp(MetaVar("?f"), MetaVar("?f"))
	good := core.MkApp(core.MkInt(1), core.MkInt(1))
	bad := core.MkApp(core.MkInt(1), core.MkInt(2))

	if Match(pattern, good) == nil {
		t.Error("expected match for (1, 1)")
	}
	if Match(pattern, bad) != nil {
		t.Error("expected no match for (1, 2)")
	}
}

func TestSubstitute(t *testing.T) {
	template := core.MkApp(MetaVar("?f"), MetaVar("?x"))
	bindings := Bindings{
		"?f": core.MkPrim(core.PrimNeg),
		"?x": core.MkInt(5),
	}

	result := Substitute(template, bindings)
	got := core.PrettyPrint(result)
	want := "-5"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRewriteAtPath(t *testing.T) {
	// Apply map-fusion inside a larger term:
	// let xs = [1,2,3] in (map f . map g) xs
	f := core.MkVar("f")
	g := core.MkVar("g")

	inner := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkApp(core.MkPrim(core.PrimMap), f),
		core.MkApp(core.MkPrim(core.PrimMap), g),
	)
	term := core.MkApp(inner, core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3)))

	// The compose is at path [0] (the Func of the outer App)
	rule := MapFusion()
	result, err := Rewrite(term, rule, Path{0})
	if err != nil {
		t.Fatalf("Rewrite at path failed: %v", err)
	}

	got := core.PrettyPrint(result.Rewritten)
	t.Logf("rewrite at path result: %s", got)

	// Should now be: map (f . g) [1, 2, 3]
	// instead of: (map f . map g) [1, 2, 3]
	if len(result.Obligations) != 0 {
		t.Errorf("expected 0 obligations, got %d", len(result.Obligations))
	}
}

func TestCatalogLookup(t *testing.T) {
	for _, name := range []string{"map-fusion", "map-foldr-fusion", "foldr-fusion"} {
		r := LookupRule(name)
		if r == nil {
			t.Errorf("LookupRule(%q) returned nil", name)
		} else if r.Citation == "" {
			t.Errorf("rule %q has no citation", name)
		}
	}

	if LookupRule("nonexistent") != nil {
		t.Error("expected nil for nonexistent rule")
	}
}
