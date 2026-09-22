// brand_emit.go — Go-emitter support for GDP brands (`--brands`).
//
// Two mechanisms are emitted, and they defeat two different forgeries:
//
//   - The phantom brand type parameter defeats the UNPAIRED proof. A
//     `BalanceChecked[B]` is evidence about the `Transaction[B]` it was
//     built from, so `NewSafeTransfer[B](tx Transaction[B], check
//     BalanceChecked[B])` will not accept a check minted for another
//     transaction. That is a compile error, with no runtime cost.
//
//   - The unexported `valid witness` field defeats the ZERO VALUE. Go's
//     visibility rules stop `BalanceChecked{principal: …}` from outside
//     the package, but they never stopped `BalanceChecked{}`. The
//     witness makes the empty literal's zero value detectable: every
//     accessor and every consuming constructor calls `mustBeMinted`,
//     which panics. A dropped error from a failing constructor lands in
//     the same place, because a failed constructor returns the
//     unminted zero value.
//
// The brand parameters themselves come from brand_inference.go; nothing
// here re-derives them.

package main

import (
	"fmt"
	"strings"
)

// brandPreamble is the fixed prologue emitted once per generated file
// when --brands is in effect.
const brandPreamble = `// --- GDP brands ---
//
// Brand is the constraint for the phantom brand parameter that every
// proof type in this package carries. Any type may serve as a brand,
// including a type declared inside the function that mints the value:
//
//	type reqBrand struct{}
//	tx := NewTransaction[reqBrand](amt, from, to)
//
// A brand declared that way cannot be named by any other scope, so a
// proof about this transaction cannot be handed a different one.
type Brand interface{}

// witness is the mint mark every generated value carries. Its zero
// value is NOT minted, which is what makes the empty literal
// ` + "`T{}`" + ` — legal Go from any package, since it names no field —
// detectable. Every accessor and every consuming constructor calls
// mustBeMinted, so a forged or dropped-error value panics the first
// time anything reads it. The panic is a runtime member of the TCB;
// see docs/TRUST-MODEL.md.
type witness struct{ minted bool }

// mint marks a value as having come out of its own constructor.
func mint() witness { return witness{minted: true} }

// mustBeMinted panics unless this value came from a constructor.
func (w witness) mustBeMinted(typeName string) {
	if !w.minted {
		panic("shenguard: forged " + typeName + " value: not produced by New" + typeName + " (zero value, empty literal, or a dropped constructor error)")
	}
}

`

// brandDeclSuffix renders the type-parameter list for a declaration:
// ["B"] → "[B Brand]", ["B1","B2"] → "[B1, B2 Brand]", nil → "".
func brandDeclSuffix(params []string) string {
	if len(params) == 0 {
		return ""
	}
	return "[" + strings.Join(params, ", ") + " Brand]"
}

// brandArgSuffix renders a type-argument list: ["B"] → "[B]".
func brandArgSuffix(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// brandedGoName is the Go type name of a generated type at its own
// brand parameters, e.g. "BalanceChecked[B]". Used for return types,
// zero values and receivers inside the type's own declarations.
func brandedGoName(bt *BrandTable, shenName, goName string) string {
	if bt == nil {
		return goName
	}
	return goName + brandArgSuffix(bt.Params(shenName))
}

// goTypeWithBrands renders a Shen type as a Go type, applying the given
// brand arguments when that type is branded. `args` comes from the
// producing rule's BrandInfo.PremiseArgs, so a premise is rendered at
// the brand it was unified into rather than at its own parameter names.
func goTypeWithBrands(st *SymbolTable, bt *BrandTable, shenType string, args []string) string {
	if bt == nil {
		return shenTypeToGo(shenType)
	}
	if elem := listElemType(shenType); elem != "" {
		return "[]" + goTypeWithBrands(st, bt, elem, args)
	}
	base := shenTypeToGo(shenType)
	target := resolveAliases(st, shenType)
	params := bt.Params(target)
	if len(params) == 0 {
		return base
	}
	if len(args) == 0 {
		// No unification context (e.g. a generated (define …) helper):
		// fall back to the type's own parameter names. The caller makes
		// itself generic over them.
		args = params
	}
	if len(args) > len(params) {
		args = args[:len(params)]
	}
	return base + brandArgSuffix(args)
}

// hasWitness reports whether a Shen type lowers to a generated struct
// that carries a witness field, and so can be checked by a consumer.
// Sum types lower to interfaces (no fields to reach), and primitives
// are not generated at all.
func hasWitness(st *SymbolTable, shenType string) bool {
	if listElemType(shenType) != "" {
		return false
	}
	info := st.Lookup(resolveAliases(st, shenType))
	if info == nil {
		return false
	}
	switch info.Category {
	case "wrapper", "constrained", "composite", "guarded":
		return true
	}
	return false
}

// emitWitnessChecks writes a mustBeMinted call for every constructor
// parameter that carries a witness. This is the "consuming constructor"
// half of the zero-value defence: a forged value cannot be laundered
// into a legitimate one by passing it up the chain.
func emitWitnessChecks(b *strings.Builder, st *SymbolTable, bt *BrandTable, premises []Premise) {
	if bt == nil {
		return
	}
	for _, p := range premises {
		if !hasWitness(st, p.TypeName) {
			continue
		}
		target := resolveAliases(st, p.TypeName)
		b.WriteString(fmt.Sprintf("\t%s.valid.mustBeMinted(%q)\n",
			toCamelCase(p.VarName), toPascalCase(target)))
	}
}

// emitAccessorWitnessCheck writes the accessor's own guard.
func emitAccessorWitnessCheck(bt *BrandTable, goName string) string {
	if bt == nil {
		return ""
	}
	return fmt.Sprintf("t.valid.mustBeMinted(%q); ", goName)
}

// emitWitnessField writes the leading witness field of a generated
// struct. Kept first so the generated layout is stable and the audit
// gate can find it with a cheap textual check.
func emitWitnessField(b *strings.Builder, bt *BrandTable) {
	if bt == nil {
		return
	}
	b.WriteString("\tvalid witness\n")
}

// mintField is the struct-literal element that marks a value minted.
func mintField(bt *BrandTable) string {
	if bt == nil {
		return ""
	}
	return "\t\tvalid: mint(),\n"
}

// mintInline is the same for single-line literals.
func mintInline(bt *BrandTable) string {
	if bt == nil {
		return ""
	}
	return "valid: mint(), "
}

// defineBrandParams collects the brand parameters a generated (define
// …) helper needs, in first-appearance order, so the helper can be
// declared generic over them.
func defineBrandParams(st *SymbolTable, bt *BrandTable, params []DefineParam) []string {
	if bt == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range params {
		shenType := p.ShenType
		if p.IsList && p.ElemShenType != "" {
			shenType = p.ElemShenType
		}
		for _, name := range bt.Params(resolveAliases(st, shenType)) {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}
