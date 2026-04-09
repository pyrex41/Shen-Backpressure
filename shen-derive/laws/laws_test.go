package laws

import (
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

func p(s string) core.Sexpr {
	expr, err := core.ParseSexpr(s)
	if err != nil {
		panic("test parse error: " + err.Error() + ": " + s)
	}
	return expr
}

func TestCatalog(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 7 {
		t.Errorf("expected 7 laws in catalog, got %d", len(catalog))
	}
	names := map[string]bool{}
	for _, r := range catalog {
		if names[r.Name] {
			t.Errorf("duplicate law name: %s", r.Name)
		}
		names[r.Name] = true
	}
}

func TestLookupRule(t *testing.T) {
	r := LookupRule("map-fusion")
	if r == nil {
		t.Fatal("map-fusion not found")
	}
	if r.Name != "map-fusion" {
		t.Errorf("wrong name: %s", r.Name)
	}

	if LookupRule("nonexistent") != nil {
		t.Error("should return nil for unknown rule")
	}
}

func TestMapFusionMatch(t *testing.T) {
	// (compose (map inc) (map double)) should match map-fusion
	term := p("(compose (map inc) (map double))")
	rule := MapFusion()

	result, err := Rewrite(term, rule, RootPath)
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	if len(result.Obligations) != 0 {
		t.Errorf("map-fusion should have no obligations, got %d", len(result.Obligations))
	}

	// Result should be (map (compose inc double))
	want := p("(map (compose inc double))")
	if !result.Rewritten.Equal(want) {
		t.Errorf("got %s, want %s",
			core.PrettyPrintSexpr(result.Rewritten),
			core.PrettyPrintSexpr(want))
	}
}

func TestMapFusionNoMatch(t *testing.T) {
	// (compose (filter p) (map g)) should NOT match map-fusion
	term := p("(compose (filter p) (map g))")
	rule := MapFusion()

	_, err := Rewrite(term, rule, RootPath)
	if err == nil {
		t.Error("expected error for non-matching term")
	}
}

func TestMapFoldrFusionMatch(t *testing.T) {
	term := p("(compose (map f) (foldr cons nil))")
	rule := MapFoldrFusion()

	result, err := Rewrite(term, rule, RootPath)
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	if len(result.Obligations) != 0 {
		t.Errorf("should have no obligations, got %d", len(result.Obligations))
	}

	// Result should be (foldr (lambda X (lambda Xs (cons (f X) Xs))) nil)
	want := p("(foldr (lambda X (lambda Xs (cons (f X) Xs))) nil)")
	if !result.Rewritten.Equal(want) {
		t.Errorf("got %s, want %s",
			core.PrettyPrintSexpr(result.Rewritten),
			core.PrettyPrintSexpr(want))
	}
}

func TestFoldrFusionMatch(t *testing.T) {
	// (compose negate (foldr + 0)) with supplemental ?h = -
	term := p("(compose negate (foldr + 0))")
	rule := FoldrFusion()

	h := core.Sym("-")
	result, err := RewriteWithSupplementalBindings(term, rule, RootPath, Bindings{"?h": h})
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	if len(result.Obligations) != 1 {
		t.Fatalf("foldr-fusion should have 1 obligation, got %d", len(result.Obligations))
	}

	// Result should be (foldr - (negate 0))
	want := p("(foldr - (negate 0))")
	if !result.Rewritten.Equal(want) {
		t.Errorf("got %s, want %s",
			core.PrettyPrintSexpr(result.Rewritten),
			core.PrettyPrintSexpr(want))
	}

	// Check obligation: (negate (+ x y)) = (- x (negate y))
	ob := result.Obligations[0]
	if ob.Description != "f (g x y) = h x (f y) for all x, y" {
		t.Errorf("wrong obligation description: %s", ob.Description)
	}
}

func TestFoldrFusionMissingH(t *testing.T) {
	term := p("(compose negate (foldr + 0))")
	rule := FoldrFusion()

	// Without supplemental binding for ?h, should fail with unresolved metavar
	_, err := Rewrite(term, rule, RootPath)
	if err == nil {
		t.Error("expected error for missing ?h binding")
	}
}

func TestFilterFusionMatch(t *testing.T) {
	term := p("(compose (filter positive?) (filter even?))")
	rule := FilterFusion()

	result, err := Rewrite(term, rule, RootPath)
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	if len(result.Obligations) != 0 {
		t.Errorf("filter-fusion should have no obligations, got %d", len(result.Obligations))
	}

	want := p("(filter (lambda X (and (positive? X) (even? X))))")
	if !result.Rewritten.Equal(want) {
		t.Errorf("got %s, want %s",
			core.PrettyPrintSexpr(result.Rewritten),
			core.PrettyPrintSexpr(want))
	}
}

func TestFoldrMapMatch(t *testing.T) {
	term := p("(compose (foldr + 0) (map square))")
	rule := FoldrMap()

	result, err := Rewrite(term, rule, RootPath)
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	if len(result.Obligations) != 0 {
		t.Errorf("should have no obligations, got %d", len(result.Obligations))
	}

	want := p("(foldr (lambda X (lambda Acc (+ (square X) Acc))) 0)")
	if !result.Rewritten.Equal(want) {
		t.Errorf("got %s, want %s",
			core.PrettyPrintSexpr(result.Rewritten),
			core.PrettyPrintSexpr(want))
	}
}

func TestFoldlFusionMatch(t *testing.T) {
	term := p("(compose negate (foldl + 0))")
	rule := FoldlFusion()

	h := core.Sym("-")
	result, err := RewriteWithSupplementalBindings(term, rule, RootPath, Bindings{"?h": h})
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	if len(result.Obligations) != 1 {
		t.Fatalf("foldl-fusion should have 1 obligation, got %d", len(result.Obligations))
	}

	want := p("(foldl - (negate 0))")
	if !result.Rewritten.Equal(want) {
		t.Errorf("got %s, want %s",
			core.PrettyPrintSexpr(result.Rewritten),
			core.PrettyPrintSexpr(want))
	}
}

func TestRewriteAtPath(t *testing.T) {
	// Apply map-fusion inside a larger term
	// (foldl (compose (map f) (map g)) 0 xs) — rewrite at path {0} (the step function position)
	// Wait, that's not right. The path indexes into List.Elems.
	// (foldl step init xs) is a 4-elem list. step is at index 1.
	term := p("(foldl (compose (map f) (map g)) init xs)")
	rule := MapFusion()

	result, err := Rewrite(term, rule, Path{1})
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}

	want := p("(foldl (map (compose f g)) init xs)")
	if !result.Rewritten.Equal(want) {
		t.Errorf("got %s, want %s",
			core.PrettyPrintSexpr(result.Rewritten),
			core.PrettyPrintSexpr(want))
	}
}

func TestSupplementalBindingOverride(t *testing.T) {
	term := p("(compose negate (foldr + 0))")
	rule := FoldrFusion()

	// ?f is already bound by LHS matching — supplemental should fail
	_, err := RewriteWithSupplementalBindings(term, rule, RootPath,
		Bindings{"?f": core.Sym("something")})
	if err == nil {
		t.Error("expected error for overriding LHS-matched binding")
	}
}

func TestSupplementalBindingUnused(t *testing.T) {
	term := p("(compose negate (foldr + 0))")
	rule := FoldrFusion()

	// ?z is not mentioned by the rule
	_, err := RewriteWithSupplementalBindings(term, rule, RootPath,
		Bindings{"?h": core.Sym("-"), "?z": core.Sym("unused")})
	if err == nil {
		t.Error("expected error for unused supplemental binding")
	}
}

func TestMatchConsistency(t *testing.T) {
	// Pattern: (?f ?f) — same metavar twice, must match same subtree
	pattern := p("(?f ?f)")
	term1 := p("(a a)")
	term2 := p("(a b)")

	b1 := Match(pattern, term1)
	if b1 == nil {
		t.Error("expected match for (a a)")
	}

	b2 := Match(pattern, term2)
	if b2 != nil {
		t.Error("expected no match for (a b) with pattern (?f ?f)")
	}
}
