// brand_inference.go — infer phantom brand parameters (GDP brands) from
// the sequent-calculus sharing structure of a spec.
//
// Workstream W1 of thoughts/shared/plans/2026-09-22-verifier-throughput-roadmap.md.
//
// The problem this solves: a generated guard type like
//
//	type BalanceChecked struct { bal float64; tx Transaction }
//	func NewSafeTransfer(tx Transaction, check BalanceChecked) SafeTransfer
//
// says nothing about `check` being the check *for that* `tx`. Any
// BalanceChecked pairs with any Transaction. In the Ghosts of Departed
// Proofs (GDP) encoding, each value carries a phantom type parameter —
// its brand — and a proof about a value is parameterized by the same
// brand, so pairing a proof with the wrong value is a type error:
//
//	func NewSafeTransfer[B Brand](tx Transaction[B], check BalanceChecked[B]) SafeTransfer[B]
//
// Nothing new is written in the spec. The brand structure is already
// there, in which premise variables the rules share. The inference
// rules (verbatim from the roadmap):
//
//  1. Each premise variable of wrapper type introduces a brand variable.
//  2. If two premises are related by a verified premise or by a nested
//     field path, their brand variables unify.
//  3. The conclusion type is parameterized by the brands of its
//     constituents that survive unification.
//
// Concretely, as implemented here:
//
//   - "Brandable" types are the generated aggregate types — composite,
//     guarded, and sum types. Primitive wrappers (`amount`, `user-id`)
//     are values, not evidence, and stay unparameterized.
//   - A brandable type that is *consumed* as a premise somewhere in the
//     spec and inherits no brand from its own constituents MINTS a brand:
//     it is the head of a proof chain (`transaction`, `parsed-claims`).
//   - Otherwise a type INHERITS one brand parameter per surviving
//     equivalence class of its branded premises (`balance-checked` from
//     `Tx`, `safe-transfer` from the unified `Tx`/`Check` class).
//   - A brandable type that is neither consumed nor inheriting stays
//     unparameterized (`account-state` in the payment spec).
//
// The result is a BrandTable: for every Shen type name, how many brand
// parameters its Go/TypeScript type takes, what they are called, and
// which of them each premise of its rule carries. The emitters read only
// this table; they do no inference of their own.

package main

import (
	"fmt"
	"sort"
	"strings"
)

// BrandInfo is the inferred brand signature of one generated type.
type BrandInfo struct {
	ShenName string
	GoName   string

	// Params are the brand type-parameter names in declaration order,
	// e.g. ["B"] or ["B1", "B2"]. Empty means the type is not branded.
	Params []string

	// Minted is true when this type introduces its own brand rather
	// than inheriting one from a premise (rule 1: it is the head of a
	// proof chain). Its constructor cannot infer the brand from its
	// arguments, so callers instantiate it explicitly.
	Minted bool

	// PremiseArgs[i] holds the brand arguments that premise i of the
	// producing rule carries, in that premise type's parameter order.
	// Empty for unbranded premises.
	PremiseArgs [][]string

	// IsSum marks the synthetic entry for a sum-type conclusion (a Go
	// interface). Its Params are the widest variant's.
	IsSum bool
}

// Branded reports whether this type takes brand parameters.
func (b *BrandInfo) Branded() bool { return b != nil && len(b.Params) > 0 }

// BrandTable is the inferred brand signature of every type in a spec.
type BrandTable struct {
	Types map[string]*BrandInfo
}

// Lookup returns the brand info for a Shen type name, or nil.
func (bt *BrandTable) Lookup(shenType string) *BrandInfo {
	if bt == nil {
		return nil
	}
	return bt.Types[shenType]
}

// Params returns the brand parameter names of a Shen type ("" safe).
func (bt *BrandTable) Params(shenType string) []string {
	if info := bt.Lookup(shenType); info != nil {
		return info.Params
	}
	return nil
}

// IsBranded reports whether the named Shen type carries brands.
func (bt *BrandTable) IsBranded(shenType string) bool {
	return bt.Lookup(shenType).Branded()
}

// ============================================================================
// Brandability
// ============================================================================

// brandableCategory reports whether a symbol-table category names a
// generated aggregate that can carry evidence, and therefore a brand.
//
// Primitive wrappers ("wrapper", "constrained") are values: branding
// them would parameterize `Amount` and `UserId` with no binding to
// gain, at the cost of infecting every signature in the host program.
// Aliases are transparent and take their target's brands.
func brandableCategory(cat string) bool {
	switch cat {
	case "composite", "guarded", "sumtype":
		return true
	}
	return false
}

// resolveAliases follows alias chains to the underlying Shen type name.
func resolveAliases(st *SymbolTable, shenType string) string {
	seen := map[string]bool{}
	for {
		info := st.Lookup(shenType)
		if info == nil || info.Category != "alias" || info.WrappedType == "" {
			return shenType
		}
		if seen[shenType] {
			return shenType
		}
		seen[shenType] = true
		shenType = info.WrappedType
	}
}

// ============================================================================
// Inference
// ============================================================================

// InferBrands computes the brand table for a parsed spec.
//
// It runs to a fixpoint because a type's brand count depends on the
// brand counts of its premises, and a spec may mention a type before
// the block that defines it. Brand counts only ever grow, so the
// iteration terminates.
func InferBrands(types []Datatype, st *SymbolTable) *BrandTable {
	bt := &BrandTable{Types: make(map[string]*BrandInfo)}

	// Every generated type gets an entry, branded or not, so emitters
	// and golden tests can ask about any name.
	for _, name := range sortedTypeNames(st) {
		info := st.Lookup(name)
		bt.Types[name] = &BrandInfo{
			ShenName: name,
			GoName:   toPascalCase(name),
			IsSum:    info.Category == "sumtype",
		}
	}

	consumed := consumedTypes(types, st)

	for pass := 0; pass < 16; pass++ {
		changed := false

		for _, dt := range types {
			for _, gt := range classify(dt, st) {
				shenName := generatedShenName(dt, gt, st)
				info := bt.Types[shenName]
				if info == nil {
					continue
				}
				if !brandableCategory(categoryOf(st, shenName)) {
					continue
				}
				if inferRuleBrands(bt, st, shenName, gt.Rule, consumed[shenName], info) {
					changed = true
				}
			}
		}

		// A sum-type interface carries the widest variant's brands, so
		// every variant satisfies it at its own brand.
		for concType, variants := range st.SumTypes {
			info := bt.Types[concType]
			if info == nil {
				continue
			}
			widest := 0
			for _, v := range variants {
				if n := len(bt.Params(v)); n > widest {
					widest = n
				}
			}
			if widest > len(info.Params) {
				info.Params = brandParamNames(widest)
				changed = true
			}
			// Pad variants so each satisfies the interface at its own
			// brands even if it inherits fewer.
			for _, v := range variants {
				vi := bt.Types[v]
				if vi != nil && len(vi.Params) < widest {
					vi.Params = brandParamNames(widest)
					changed = true
				}
			}
		}

		// Aliases are transparent: they take their target's brands.
		for name, info := range bt.Types {
			ti := st.Lookup(name)
			if ti == nil || ti.Category != "alias" || ti.WrappedType == "" {
				continue
			}
			target := bt.Params(resolveAliases(st, ti.WrappedType))
			if len(target) > len(info.Params) {
				info.Params = append([]string(nil), target...)
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	return bt
}

// generatedShenName mirrors the name resolution in SymbolTable.Build /
// classify: a block whose name differs from its conclusion type keeps
// the conclusion name unless the conclusion is contested.
func generatedShenName(dt Datatype, gt GeneratedType, st *SymbolTable) string {
	typeName := gt.Rule.Conc.TypeName
	if dt.Name != typeName && st.ConcCount[typeName] > 1 {
		typeName = dt.Name
	}
	return typeName
}

func categoryOf(st *SymbolTable, shenName string) string {
	if info := st.Lookup(shenName); info != nil {
		return info.Category
	}
	return ""
}

// inferRuleBrands applies rules 1–3 to one rule. Returns true when the
// table changed.
func inferRuleBrands(bt *BrandTable, st *SymbolTable, shenName string, r Rule, isConsumed bool, info *BrandInfo) bool {
	// Rule 1: each branded premise contributes one brand variable per
	// brand parameter of its own type.
	type slot struct{ prem, idx int }
	var slots []slot
	premSlots := make([][]int, len(r.Premises))
	for i, p := range r.Premises {
		target := resolveAliases(st, premiseElemType(p.TypeName))
		n := len(bt.Params(target))
		for j := 0; j < n; j++ {
			premSlots[i] = append(premSlots[i], len(slots))
			slots = append(slots, slot{prem: i, idx: j})
		}
	}

	// Rule 2: unify slots of premises related by a nested field path or
	// by a shared verified premise. Slots unify positionally.
	uf := newUnionFind(len(slots))
	for i := range r.Premises {
		for j := i + 1; j < len(r.Premises); j++ {
			if len(premSlots[i]) == 0 || len(premSlots[j]) == 0 {
				continue
			}
			if !premisesRelated(st, r, r.Premises[i], r.Premises[j]) {
				continue
			}
			n := len(premSlots[i])
			if len(premSlots[j]) < n {
				n = len(premSlots[j])
			}
			for k := 0; k < n; k++ {
				uf.union(premSlots[i][k], premSlots[j][k])
			}
		}
	}

	// Rule 3: surviving classes, ordered by first appearance, become the
	// conclusion's brand parameters.
	classOrder := []int{}
	classIndex := map[int]int{}
	for s := range slots {
		root := uf.find(s)
		if _, seen := classIndex[root]; !seen {
			classIndex[root] = len(classOrder)
			classOrder = append(classOrder, root)
		}
	}

	params := brandParamNames(len(classOrder))
	minted := false
	if len(params) == 0 && isConsumed {
		// The head of a proof chain: nothing to inherit, but downstream
		// rules want to talk about *this* value, so mint a brand.
		params = brandParamNames(1)
		minted = true
	}

	premiseArgs := make([][]string, len(r.Premises))
	for i := range r.Premises {
		for _, s := range premSlots[i] {
			premiseArgs[i] = append(premiseArgs[i], params[classIndex[uf.find(s)]])
		}
	}

	changed := false
	if len(params) > len(info.Params) {
		info.Params = params
		info.Minted = minted
		changed = true
	} else if len(params) == len(info.Params) && info.Minted != minted && len(params) > 0 {
		info.Minted = minted
		changed = true
	}
	if !equalStringMatrix(info.PremiseArgs, premiseArgs) {
		info.PremiseArgs = premiseArgs
		changed = true
	}
	return changed
}

// premisesRelated implements rule 2's two relations.
func premisesRelated(st *SymbolTable, r Rule, a, b Premise) bool {
	at := resolveAliases(st, premiseElemType(a.TypeName))
	btName := resolveAliases(st, premiseElemType(b.TypeName))
	if typeContains(st, at, btName) || typeContains(st, btName, at) {
		return true
	}
	for _, v := range r.Verified {
		if verifiedMentions(v.Raw, a.VarName) && verifiedMentions(v.Raw, b.VarName) {
			return true
		}
	}
	return false
}

// typeContains reports whether `outer` reaches `inner` along a nested
// field path of length >= 1. Strict containment: a type does not
// contain itself, so two premises of the same type stay distinct
// (two transactions deserve two brands).
func typeContains(st *SymbolTable, outer, inner string) bool {
	if outer == "" || inner == "" {
		return false
	}
	visited := map[string]bool{}
	var walk func(string, int) bool
	walk = func(cur string, depth int) bool {
		if depth > 16 || visited[cur] {
			return false
		}
		visited[cur] = true
		info := st.Lookup(cur)
		if info == nil {
			return false
		}
		// A sum type reaches through its variants.
		if info.Category == "sumtype" {
			for _, v := range st.SumTypes[cur] {
				if v == inner || walk(v, depth+1) {
					return true
				}
			}
		}
		for _, f := range info.Fields {
			ft := resolveAliases(st, premiseElemType(f.ShenType))
			if ft == inner {
				return true
			}
			if walk(ft, depth+1) {
				return true
			}
		}
		return false
	}
	return walk(outer, 0)
}

// verifiedMentions reports whether a verified premise's raw
// s-expression mentions the given Shen variable as an atom. Catches
// `(= User (head (head Jwt)))` mentioning both `User` and `Jwt`.
func verifiedMentions(raw, varName string) bool {
	if raw == "" || varName == "" {
		return false
	}
	for _, tok := range tokenize(raw) {
		if strings.Trim(tok, "[]()") == varName {
			return true
		}
	}
	return false
}

// consumedTypes returns the set of brandable types used as a premise
// type somewhere in the spec — the types other rules reason about, and
// therefore the types that need a brand to be reasoned about *by name*.
func consumedTypes(types []Datatype, st *SymbolTable) map[string]bool {
	out := map[string]bool{}
	for _, dt := range types {
		for _, r := range dt.Rules {
			for _, p := range r.Premises {
				target := resolveAliases(st, premiseElemType(p.TypeName))
				if brandableCategory(categoryOf(st, target)) {
					out[target] = true
					// Consuming a sum type consumes its variants: they
					// must be expressible at the interface's brand.
					for _, v := range st.SumTypes[target] {
						out[v] = true
					}
				}
			}
		}
	}
	return out
}

// ============================================================================
// Rendering — the golden brand table
// ============================================================================

// String renders the brand table in a stable, diffable form. This is
// what the golden tests pin and what `--dry-run --brands` prints.
func (bt *BrandTable) String() string {
	if bt == nil {
		return ""
	}
	names := make([]string, 0, len(bt.Types))
	for n := range bt.Types {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, n := range names {
		info := bt.Types[n]
		kind := "inherited"
		switch {
		case len(info.Params) == 0:
			kind = "unbranded"
		case info.IsSum:
			kind = "sum"
		case info.Minted:
			kind = "minted"
		}
		sig := info.GoName
		if len(info.Params) > 0 {
			sig += "[" + strings.Join(info.Params, ", ") + "]"
		}
		fmt.Fprintf(&b, "%-24s %-10s %s", n, kind, sig)
		if len(info.PremiseArgs) > 0 {
			var parts []string
			for _, args := range info.PremiseArgs {
				if len(args) == 0 {
					parts = append(parts, "-")
					continue
				}
				parts = append(parts, strings.Join(args, "+"))
			}
			fmt.Fprintf(&b, "  premises(%s)", strings.Join(parts, " "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ============================================================================
// Small helpers
// ============================================================================

// premiseElemType unwraps `(list X)` to X. A premise of list type is a
// premise about X's, so the brand travels with the element: a
// `(list cart-item)` field makes `cart-item` a consumed type and its
// brand one of the conclusion's.
func premiseElemType(shenType string) string {
	if elem := listElemType(shenType); elem != "" {
		return elem
	}
	return shenType
}

func brandParamNames(n int) []string {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []string{"B"}
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("B%d", i+1)
	}
	return out
}

func sortedTypeNames(st *SymbolTable) []string {
	names := make([]string, 0, len(st.Types))
	for n := range st.Types {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func equalStringMatrix(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

type unionFind struct{ parent []int }

func newUnionFind(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if ra < rb {
		u.parent[rb] = ra
	} else {
		u.parent[ra] = rb
	}
}
