package laws

import (
	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// mustParse parses an s-expression or panics. Used for law definitions
// which are known-valid at compile time.
func mustParse(s string) core.Sexpr {
	expr, err := core.ParseSexpr(s)
	if err != nil {
		panic("laws: invalid s-expression in law definition: " + err.Error() + ": " + s)
	}
	return expr
}

// Catalog returns all available rewrite laws.
func Catalog() []*Rule {
	return []*Rule{
		MapFusion(),
		MapFoldrFusion(),
		FoldrFusion(),
		AllScanlFusion(),
		FilterFusion(),
		FoldrMap(),
		FoldlFusion(),
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
// No side conditions.
//
// Source: Bird, Ch.1; Bird & de Moor, §3.1.

func MapFusion() *Rule {
	return &Rule{
		Name:     "map-fusion",
		LHS:      mustParse("(compose (map ?f) (map ?g))"),
		RHS:      mustParse("(map (compose ?f ?g))"),
		Citation: `Bird, "Pearls of Functional Algorithm Design", Ch. 1; Bird & de Moor, "Algebra of Programming", §3.1`,
	}
}

// --- map-foldr-fusion ---
//
// Law: map f . foldr cons nil = foldr (\x xs -> cons (f x) xs) nil
// No side conditions.
//
// Source: Bird, Ch.1; instance of foldr-fusion.

func MapFoldrFusion() *Rule {
	return &Rule{
		Name:     "map-foldr-fusion",
		LHS:      mustParse("(compose (map ?f) (foldr cons nil))"),
		RHS:      mustParse("(foldr (lambda X (lambda Xs (cons (?f X) Xs))) nil)"),
		Citation: `Bird, "Pearls of Functional Algorithm Design", Ch. 1; instance of foldr-fusion`,
	}
}

// --- foldr-fusion ---
//
// Law: f . foldr g e = foldr h (f e)
//       provided  f (g x y) = h x (f y)  for all x, y
//
// ?h must be supplied via supplemental bindings.
//
// Source: Bird, Ch.3; Bird & de Moor, Theorem 3.1.

func FoldrFusion() *Rule {
	return &Rule{
		Name: "foldr-fusion",
		LHS:  mustParse("(compose ?f (foldr ?g ?e))"),
		RHS:  mustParse("(foldr ?h (?f ?e))"),
		SideConditions: []SideCondition{
			{
				Description: "f (g x y) = h x (f y) for all x, y",
				LHS:         mustParse("(?f (?g x y))"),
				RHS:         mustParse("(?h x (?f y))"),
			},
		},
		Citation: `Bird, "Pearls of Functional Algorithm Design", Ch. 3; Bird & de Moor, "Algebra of Programming", Theorem 3.1 (fusion law)`,
	}
}

// --- all-scanl-fusion ---
//
// Law:
//   foldr (\x acc -> p x && acc) True (scanl f e xs)
//   =
//   snd (foldl (\state x ->
//         let next = f (fst state) x
//         in (@p next (and (snd state) (p next))))
//       (@p e (p e)) xs)
//
// Source: Bird-Meertens style fusion.

func AllScanlFusion() *Rule {
	lhs := mustParse(`(
		(foldr (lambda X (lambda Acc (and (?p X) Acc))) true (scanl ?f ?e ?xs))
	)`)
	// unwrap the outer parens from the parse (we wrapped for readability)
	if l, ok := lhs.(*core.List); ok && len(l.Elems) == 1 {
		lhs = l.Elems[0]
	}

	rhs := mustParse(`(
		(snd (foldl
			(lambda State (lambda X
				(let Next (?f (fst State) X)
					(@p Next (and (snd State) (?p Next))))))
			(@p ?e (?p ?e))
			?xs))
	)`)
	if l, ok := rhs.(*core.List); ok && len(l.Elems) == 1 {
		rhs = l.Elems[0]
	}

	return &Rule{
		Name:     "all-scanl-fusion",
		LHS:      lhs,
		RHS:      rhs,
		Citation: `Derived from the definitions of all, scanl, and foldl; Bird-Meertens style fusion`,
	}
}

// --- filter-fusion ---
//
// Law: filter p . filter q = filter (\x -> p x && q x)
// No side conditions.

func FilterFusion() *Rule {
	return &Rule{
		Name:     "filter-fusion",
		LHS:      mustParse("(compose (filter ?p) (filter ?q))"),
		RHS:      mustParse("(filter (lambda X (and (?p X) (?q X))))"),
		Citation: `Standard; dual of map-fusion for filter`,
	}
}

// --- foldr-map ---
//
// Law: foldr g e . map f = foldr (\x acc -> g (f x) acc) e
// No side conditions.
//
// Fuses a fold after a map into a single pass.

func FoldrMap() *Rule {
	return &Rule{
		Name:     "foldr-map",
		LHS:      mustParse("(compose (foldr ?g ?e) (map ?f))"),
		RHS:      mustParse("(foldr (lambda X (lambda Acc (?g (?f X) Acc))) ?e)"),
		Citation: `Bird & de Moor, "Algebra of Programming"; standard fold-map fusion`,
	}
}

// --- foldl-fusion ---
//
// Law: g . foldl f e = foldl h (g e)
//       provided  g (f x y) = h x (g y)  for all x, y
//
// Dual of foldr-fusion. ?h must be supplied via supplemental bindings.

func FoldlFusion() *Rule {
	return &Rule{
		Name: "foldl-fusion",
		LHS:  mustParse("(compose ?g (foldl ?f ?e))"),
		RHS:  mustParse("(foldl ?h (?g ?e))"),
		SideConditions: []SideCondition{
			{
				Description: "g (f x y) = h x (g y) for all x, y",
				LHS:         mustParse("(?g (?f x y))"),
				RHS:         mustParse("(?h x (?g y))"),
			},
		},
		Citation: `Dual of foldr-fusion; Bird & de Moor, "Algebra of Programming"`,
	}
}
