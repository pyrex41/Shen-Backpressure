package report

import (
	"fmt"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// ClassifyDatatypes turns the parsed datatype blocks into report
// rules. Each rule has its premises classified statically: value
// premises like `Bal : number` are discharged via guard-type at the
// function boundary; verified premises like `(>= X 0) : verified`
// are discharged at construction time by the generated guard
// constructor. The classification reflects what the existing
// shengen-emitted Go does — it is a faithful description of the
// current evidence, not a separate static analyser.
//
// The optional guardOutputPath, when non-empty and resolvable, is
// used to populate `code_references` with `guards_gen.go:N` pointers
// for each guarded type. When empty, code_references is omitted.
func ClassifyDatatypes(sf *specfile.SpecFile, guardOutputPath string) ([]Rule, error) {
	var rules []Rule
	guardLines, _ := readGuardLines(guardOutputPath)

	for _, dt := range sf.Datatypes {
		for _, r := range dt.Rules {
			rule := classifyDatatypeRule(sf.Path, dt, r, guardOutputPath, guardLines)
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func classifyDatatypeRule(specPath string, dt specfile.Datatype, r specfile.Rule, guardPath string, guardLines map[string]int) Rule {
	kind := classifyKind(r)
	rule := Rule{
		Name:            dt.Name,
		Kind:            kind,
		SpecFile:        specPath,
		SpecExcerpt:     renderDatatypeExcerpt(dt, r),
		Status:          StatusDischarged,
		CounterExamples: []CounterExample{},
	}
	rule.HumanDescription, rule.HumanDescriptionSource = describeDatatype(dt, r, kind)

	// One premise per value premise (X : type), one per verified
	// expression. Each is statically discharged.
	for _, p := range r.Premises {
		prem := Premise{
			ID:         premiseID(dt.Name, "field", p.VarName),
			Expression: fmt.Sprintf("%s : %s", p.VarName, p.TypeName),
		}
		if isShenPrimitiveType(p.TypeName) {
			// Primitive premises are discharged at the function
			// boundary by the target language's type system —
			// callers can't pass a non-`number` to a parameter
			// typed `number` (after lowering to float64).
			prem.Discharge = DischargeStatic
			prem.DischargeBasis = BasisGuardTypeAtBoundary
			prem.Rationale = fmt.Sprintf(
				"%s is typed %s; the target language's type system rejects non-%s values at construction.",
				p.VarName, p.TypeName, p.TypeName,
			)
		} else {
			// Composite/named-type premise: discharged because the
			// guard constructor for the named type already enforces
			// every premise of *that* type. The proof chain is
			// transitive.
			prem.Discharge = DischargeStatic
			prem.DischargeBasis = BasisGuardTypeAtBoundary
			prem.Rationale = fmt.Sprintf(
				"%s is typed %s; values of that type can only be constructed via shengen's guarded constructor, which enforces all of %s's premises transitively.",
				p.VarName, p.TypeName, p.TypeName,
			)
		}
		if guardPath != "" {
			if line, ok := guardLines[dt.Name]; ok {
				prem.CodeReferences = []string{fmt.Sprintf("%s:%d", guardPath, line)}
			}
		}
		rule.Premises = append(rule.Premises, prem)
	}
	for _, v := range r.Verified {
		prem := Premise{
			ID:             premiseID(dt.Name, "verified", slugifyExpr(v.Raw)),
			Expression:     fmt.Sprintf("%s : verified", v.Raw),
			Discharge:      DischargeStatic,
			DischargeBasis: BasisGuardConstructorValidates,
			Rationale: fmt.Sprintf(
				"shengen's generated constructor for %s rejects inputs that do not satisfy %s, so this premise holds for any value of type %s reachable in the impl.",
				dt.Name, v.Raw, dt.Name,
			),
		}
		if guardPath != "" {
			if line, ok := guardLines[dt.Name]; ok {
				prem.CodeReferences = []string{fmt.Sprintf("%s:%d", guardPath, line)}
			}
		}
		rule.Premises = append(rule.Premises, prem)
	}
	return rule
}

// ClassifyDefine produces a Rule for a (define …) block. The
// discharge here is exactly one premise: "spec output equals impl
// output on sampled inputs". That premise is runtime-sampled by
// shen-derive's generated test.
//
// sampleCount is the number of cases shen-derive's harness produced;
// seedLabel is "deterministic-default" for a zero seed or the integer
// seed as a string otherwise. samplesFailed and counterExamples are
// filled in by the consumer (sb derive) after `go test` runs.
func ClassifyDefine(specPath string, def *specfile.Define, sampleCount int, seedLabel string, implFunc string) Rule {
	rule := Rule{
		Name:            def.Name,
		Kind:            "define",
		SpecFile:        specPath,
		SpecExcerpt:     renderDefineExcerpt(def),
		Status:          StatusDischarged,
		CounterExamples: []CounterExample{},
	}
	rule.HumanDescription, rule.HumanDescriptionSource = describeDefine(def)

	prem := Premise{
		ID:             premiseID(def.Name, "oracle", "spec-equiv"),
		Expression:     fmt.Sprintf("spec(%s) ≡ impl(%s) on sampled inputs", def.Name, implFunc),
		Discharge:      DischargeRuntimeSampled,
		DischargeBasis: BasisShenDeriveSampled,
		Rationale: fmt.Sprintf(
			"shen-derive evaluated the spec on %d sampled cases (%s) and emitted a Go test asserting impl returns the same value on each.",
			sampleCount, seedLabel,
		),
		SamplesPassed: sampleCount,
		SamplesFailed: 0,
		SampleSeed:    strPtr(seedLabel),
	}
	rule.Premises = append(rule.Premises, prem)
	return rule
}

// classifyKind reports the rule's kind in the schema vocabulary
// (mirrors specfile.TypeCategory plus "define").
func classifyKind(r specfile.Rule) string {
	nFields := len(r.Premises)
	nVerified := len(r.Verified)
	switch {
	case nFields == 1 && nVerified == 0:
		return "wrapper"
	case nFields == 1 && nVerified > 0:
		return "constrained"
	case nFields > 1 && nVerified == 0:
		return "composite"
	default:
		return "guarded"
	}
}

// renderDatatypeExcerpt produces a stable, single-rule excerpt of a
// (datatype …) block. We render from parsed structure rather than
// re-extracting the original bytes so the output is canonical.
func renderDatatypeExcerpt(dt specfile.Datatype, r specfile.Rule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(datatype %s\n", dt.Name)
	for _, p := range r.Premises {
		fmt.Fprintf(&b, "  %s : %s;\n", p.VarName, p.TypeName)
	}
	for _, v := range r.Verified {
		fmt.Fprintf(&b, "  %s : verified;\n", v.Raw)
	}
	b.WriteString("  ====================\n")
	if r.Conclusion.IsWrapped {
		fmt.Fprintf(&b, "  %s : %s;)\n", r.Conclusion.Fields[0], r.Conclusion.TypeName)
	} else {
		fmt.Fprintf(&b, "  [%s] : %s;)\n", strings.Join(r.Conclusion.Fields, " "), r.Conclusion.TypeName)
	}
	return b.String()
}

func renderDefineExcerpt(def *specfile.Define) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(define %s", def.Name)
	if len(def.TypeSig.ParamTypes) > 0 {
		fmt.Fprintf(&b, "\n  {%s --> %s}",
			strings.Join(def.TypeSig.ParamTypes, " --> "),
			def.TypeSig.ReturnType,
		)
	}
	if len(def.Clauses) == 1 {
		fmt.Fprintf(&b, "\n  %s -> ...)", strings.Join(def.ParamNames, " "))
	} else {
		fmt.Fprintf(&b, "\n  ; %d clauses)", len(def.Clauses))
	}
	return b.String()
}

func describeDatatype(dt specfile.Datatype, r specfile.Rule, kind string) (string, string) {
	if dt.Doc != "" {
		return dt.Doc, HumanDescriptionFromDoc
	}
	switch kind {
	case "wrapper":
		return fmt.Sprintf(
			"A %s value is a %s with no further runtime constraints; the type exists to keep raw %ss from being mistaken for one.",
			dt.Name, r.Premises[0].TypeName, r.Premises[0].TypeName,
		), HumanDescriptionAutoGenerated
	case "constrained":
		return fmt.Sprintf(
			"A %s value is a %s that satisfies %d additional constraint(s) checked at construction.",
			dt.Name, r.Premises[0].TypeName, len(r.Verified),
		), HumanDescriptionAutoGenerated
	case "composite":
		fields := make([]string, len(r.Premises))
		for i, p := range r.Premises {
			fields[i] = p.VarName
		}
		return fmt.Sprintf(
			"A %s bundles %d fields (%s) into a single typed value.",
			dt.Name, len(fields), strings.Join(fields, ", "),
		), HumanDescriptionAutoGenerated
	case "guarded":
		return fmt.Sprintf(
			"A %s is a multi-field structure whose constructor enforces %d cross-field invariant(s).",
			dt.Name, len(r.Verified),
		), HumanDescriptionAutoGenerated
	}
	return fmt.Sprintf("Shen rule %s.", dt.Name), HumanDescriptionAutoGenerated
}

func describeDefine(def *specfile.Define) (string, string) {
	if def.Doc != "" {
		return def.Doc, HumanDescriptionFromDoc
	}
	if len(def.TypeSig.ParamTypes) > 0 {
		return fmt.Sprintf(
			"A pure function %s : %s --> %s. The Shen spec is the oracle; the impl is asserted to match it on every sampled input.",
			def.Name,
			strings.Join(def.TypeSig.ParamTypes, " --> "),
			def.TypeSig.ReturnType,
		), HumanDescriptionAutoGenerated
	}
	return fmt.Sprintf(
		"A pure function %s. The Shen spec is the oracle; the impl is asserted to match it on every sampled input.",
		def.Name,
	), HumanDescriptionAutoGenerated
}

// premiseID makes a stable ID slug. Format: rule.kind-key.
func premiseID(rule, kind, key string) string {
	clean := func(s string) string {
		var b strings.Builder
		for _, c := range s {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
				b.WriteRune(c)
			case c == '-' || c == '_':
				b.WriteRune(c)
			default:
				b.WriteByte('-')
			}
		}
		return strings.Trim(b.String(), "-")
	}
	return strings.ToLower(fmt.Sprintf("%s.%s-%s", clean(rule), clean(kind), clean(key)))
}

func slugifyExpr(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	last := byte('-')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
			last = c
		default:
			if last != '-' {
				b.WriteByte('-')
				last = '-'
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func isShenPrimitiveType(t string) bool {
	switch strings.TrimSpace(t) {
	case "number", "string", "boolean", "symbol":
		return true
	}
	return false
}

func strPtr(s string) *string { return &s }
