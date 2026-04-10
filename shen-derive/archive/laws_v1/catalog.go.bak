package laws

import (
	"fmt"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// Catalog returns all available rewrite laws.
func Catalog() []*Rule {
	return []*Rule{
		MapFusion(),
		MapFoldrFusion(),
		FoldrFusion(),
		AllScanlFusion(),
	}
}

// LookupRule finds a rule by name in the catalog.
func LookupRule(name string) *Rule {
	for _, r := range Catalog() {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// --- map-fusion ---
//
// Law: map f . map g = map (f . g)
//
// No side conditions.
//
// Source: Bird, "Pearls of Functional Algorithm Design", Chapter 1;
//         Bird & de Moor, "Algebra of Programming", Section 3.1.
//
// In AST form:
//   LHS: compose (map ?f) (map ?g)       -- i.e., (map ?f) . (map ?g)
//   RHS: map (compose ?f ?g)              -- i.e., map (?f . ?g)
//
// Note: The LHS is represented as App(App(compose, App(map, ?f)), App(map, ?g))
// because "map f . map g" desugars to "compose (map f) (map g)" which at
// the partially-applied level is App(App(Prim(compose), App(Prim(map), ?f)), App(Prim(map), ?g)).

func MapFusion() *Rule {
	f := MetaVar("?f")
	g := MetaVar("?g")

	// LHS: compose (map ?f) (map ?g)
	// This is the partially-applied compose: two args supplied, waiting for a list.
	// At AST level: App(App(Prim(compose), App(Prim(map), ?f)), App(Prim(map), ?g))
	lhs := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkApp(core.MkPrim(core.PrimMap), f),
		core.MkApp(core.MkPrim(core.PrimMap), g),
	)

	// RHS: map (compose ?f ?g)
	// At AST level: App(Prim(map), App(App(Prim(compose), ?f), ?g))
	rhs := core.MkApp(core.MkPrim(core.PrimMap),
		core.MkApps(core.MkPrim(core.PrimCompose), f, g),
	)

	return &Rule{
		Name:     "map-fusion",
		LHS:      lhs,
		RHS:      rhs,
		Citation: `Bird, "Pearls of Functional Algorithm Design", Ch. 1; Bird & de Moor, "Algebra of Programming", §3.1`,
	}
}

// --- map-foldr-fusion ---
//
// Law: map f . foldr cons nil = foldr (\x xs -> cons (f x) xs) nil
//
// This is a specialization of foldr-fusion where g = cons, e = nil.
// It rewrites a map-after-identity-fold into a single fold that applies f
// during construction.
//
// Equivalently: map f = foldr (\x xs -> f x : xs) []
// So map f . foldr cons nil is just map f applied to the identity fold,
// which simplifies to foldr (cons . f) nil — but more precisely,
// the step function becomes \x xs -> cons (f x) xs.
//
// No side conditions (cons and nil form a free algebra on lists).
//
// Source: Bird, "Pearls of Functional Algorithm Design", Chapter 1;
//         derivable from foldr-fusion with f = map f', g = cons, e = nil.

func MapFoldrFusion() *Rule {
	f := MetaVar("?f")

	// LHS: compose (map ?f) (foldr cons nil)
	// = (map ?f) . (foldr cons nil)
	lhs := core.MkApps(core.MkPrim(core.PrimCompose),
		core.MkApp(core.MkPrim(core.PrimMap), f),
		core.MkApps(core.MkPrim(core.PrimFoldr),
			core.MkPrim(core.PrimCons),
			core.MkPrim(core.PrimNil),
		),
	)

	// RHS: foldr (\x xs -> cons (?f x) xs) nil
	// = foldr (\x xs -> (?f x) : xs) []
	rhs := core.MkApps(core.MkPrim(core.PrimFoldr),
		core.MkLam("x", nil,
			core.MkLam("xs", nil,
				core.MkApps(core.MkPrim(core.PrimCons),
					core.MkApp(f, core.MkVar("x")),
					core.MkVar("xs"),
				),
			),
		),
		core.MkPrim(core.PrimNil),
	)

	return &Rule{
		Name:     "map-foldr-fusion",
		LHS:      lhs,
		RHS:      rhs,
		Citation: `Bird, "Pearls of Functional Algorithm Design", Ch. 1; instance of foldr-fusion`,
	}
}

// --- foldr-fusion (the fusion law) ---
//
// Law: f . foldr g e = foldr h (f e)
//       provided  f (g x y) = h x (f y)  for all x, y
//
// The side condition is the "provided" clause. It must be discharged
// for the specific f, g, h in the user's derivation.
//
// Source: Bird, "Pearls of Functional Algorithm Design", Chapter 3;
//         Bird & de Moor, "Algebra of Programming", Theorem 3.1
//         (the "fusion law" / "fold-fusion" / "banana-split").
//
// In pattern form:
//   LHS: compose ?f (foldr ?g ?e)     -- ?f . foldr ?g ?e
//   RHS: foldr ?h (?f ?e)             -- foldr ?h (?f ?e)
//
// The RHS introduces a new metavariable ?h that is NOT bound by matching
// the LHS. Instead, ?h is determined by the side condition:
//   f (g x y) = h x (f y)
// The user must supply ?h (or it must be inferred from context).
//
// For the rewriter, we handle this by requiring the caller to supply ?h
// as part of the rewrite invocation (via supplemental bindings).

func FoldrFusion() *Rule {
	f := MetaVar("?f")
	g := MetaVar("?g")
	e := MetaVar("?e")
	h := MetaVar("?h")

	// LHS: compose ?f (foldr ?g ?e)
	lhs := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	// RHS: foldr ?h (?f ?e)
	rhs := core.MkApps(core.MkPrim(core.PrimFoldr),
		h,
		core.MkApp(f, e),
	)

	// Side condition: ?f (?g x y) = ?h x (?f y) for all x, y
	// The universally quantified variables x, y are concrete Var nodes.
	sc := SideCondition{
		Description: "f (g x y) = h x (f y) for all x, y",
		LHS: core.MkApp(f,
			core.MkApps(g, core.MkVar("x"), core.MkVar("y")),
		),
		RHS: core.MkApps(h,
			core.MkVar("x"),
			core.MkApp(f, core.MkVar("y")),
		),
	}

	return &Rule{
		Name:           "foldr-fusion",
		LHS:            lhs,
		RHS:            rhs,
		SideConditions: []SideCondition{sc},
		Citation:        `Bird, "Pearls of Functional Algorithm Design", Ch. 3; Bird & de Moor, "Algebra of Programming", Theorem 3.1 (fusion law)`,
	}
}

// --- all-scanl-fusion ---
//
// Law:
//   foldr (\x acc -> p x && acc) True (scanl f e xs)
//   =
//   snd (foldl (\state x ->
//         let next = f (fst state) x
//         in (next, snd state && p next))
//       (e, p e) xs)
//
// This is the running-invariant version of fusing `all p` with `scanl f e`.
// It preserves the initial-element check from `scanl`, so the accumulator
// starts at `(e, p e)` rather than `(e, True)`.
//
// Source: derived from the definitions of `all`, `scanl`, and `foldl`, in the
// spirit of Bird-Meertens fusion.
func AllScanlFusion() *Rule {
	p := MetaVar("?p")
	f := MetaVar("?f")
	e := MetaVar("?e")
	xs := MetaVar("?xs")

	allStep := core.MkLam("x", nil,
		core.MkLam("acc", nil,
			core.MkApps(core.MkPrim(core.PrimAnd),
				core.MkApp(p, core.MkVar("x")),
				core.MkVar("acc"),
			),
		),
	)

	lhs := core.MkApp(
		core.MkApps(core.MkPrim(core.PrimFoldr), allStep, core.MkBool(true)),
		core.MkApps(core.MkPrim(core.PrimScanl), f, e, xs),
	)

	step := core.MkLam("state", nil,
		core.MkLam("x", nil,
			core.MkLet("next",
				core.MkApps(f,
					core.MkApp(core.MkPrim(core.PrimFst), core.MkVar("state")),
					core.MkVar("x"),
				),
				core.MkTuple(
					core.MkVar("next"),
					core.MkApps(core.MkPrim(core.PrimAnd),
						core.MkApp(core.MkPrim(core.PrimSnd), core.MkVar("state")),
						core.MkApp(p, core.MkVar("next")),
					),
				),
			),
		),
	)

	rhs := core.MkApp(core.MkPrim(core.PrimSnd),
		core.MkApps(core.MkPrim(core.PrimFoldl),
			step,
			core.MkTuple(e, core.MkApp(p, e)),
			xs,
		),
	)

	return &Rule{
		Name:     "all-scanl-fusion",
		LHS:      lhs,
		RHS:      rhs,
		Citation: `Derived from the definitions of all, scanl, and foldl; Bird-Meertens style fusion`,
	}
}

// RewriteWithSupplementalBindings applies a rule at a path, with additional
// bindings for metavariables not bound by matching (e.g., ?h in foldr-fusion).
func RewriteWithSupplementalBindings(term core.Term, rule *Rule, path Path, extra Bindings) (*RewriteResult, error) {
	target, err := AtPath(term, path)
	if err != nil {
		return nil, err
	}

	bindings := Match(rule.LHS, target)
	if bindings == nil {
		return nil, fmt.Errorf("rewrite %s: LHS pattern does not match at path %v\n  pattern: %s\n  term:    %s",
			rule.Name, path, core.PrettyPrint(rule.LHS), core.PrettyPrint(target))
	}

	// Merge supplemental bindings
	for k, v := range extra {
		if !IsMetaVar(k) {
			return nil, fmt.Errorf("rewrite %s: supplemental binding %q is not a metavariable", rule.Name, k)
		}
		if _, ok := bindings[k]; ok {
			return nil, fmt.Errorf("rewrite %s: supplemental binding for %s would override an LHS match", rule.Name, k)
		}
		if !ruleMentionsMetaVar(rule, k) {
			return nil, fmt.Errorf("rewrite %s: supplemental binding %s is not used by the rule", rule.Name, k)
		}
		bindings[k] = v
	}

	rewritten := Substitute(rule.RHS, bindings)
	newTerm, err := ReplacePath(term, path, rewritten)
	if err != nil {
		return nil, err
	}

	var obligations []InstantiatedCondition
	for _, sc := range rule.SideConditions {
		obligations = append(obligations, InstantiatedCondition{
			Description: sc.Description,
			LHS:         Substitute(sc.LHS, bindings),
			RHS:         Substitute(sc.RHS, bindings),
		})
	}

	result := &RewriteResult{
		Original:    term,
		Rewritten:   newTerm,
		RuleName:    rule.Name,
		Obligations: obligations,
	}
	if err := validateRewriteResult(rule.Name, result); err != nil {
		return nil, err
	}
	return result, nil
}
