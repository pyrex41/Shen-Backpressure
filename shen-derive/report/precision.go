package report

// precision.go — W5.4. Precision per premise and blame per
// counter-example.
//
// Two fields, one idea: a report that says "discharged" without saying
// *how well* and "violated" without saying *whose fault* makes every
// reader redo the same derivation by hand. Precision is a total order
// over evidence strength; blame names a responsible party. Both are
// derived from material the report already carries, so neither can
// contradict the rest of the document.
//
// Kept in its own file, and referenced from classify.go only through
// ApplyPrecision, so the W4 work landing in classify.go merges
// cleanly.

import (
	"encoding/json"
	"os"
	"strings"
)

// PrecisionFor maps a premise's discharge and basis onto the total
// order. The basis is consulted first because it is the more specific
// statement: `prover-z3-path-cover` and `shen-derive-sampled` are both
// `runtime-sample` discharges but are not the same evidence.
//
// Anything unrecognised falls to "unproven", which is the safe
// direction: a report may understate its evidence, never overstate it.
func PrecisionFor(discharge, basis string) string {
	switch basis {
	case BasisGuardBrandBound,
		BasisGuardTypeAtBoundary,
		BasisGuardConstructorValidates:
		return PrecisionStatic
	case BasisProverZ3PathCover:
		return PrecisionPathCover
	case BasisShenDeriveSampled:
		return PrecisionSampled
	case BasisRuntimeViaWitness,
		BasisRuntimeViaEvaluator,
		BasisRuntimeViaDBAttested:
		return PrecisionRuntime
	case BasisRuntimeViaSampledEquivalence:
		// Profile C has a runtime check *and* a sampled-equivalence
		// test pinning it to the spec. The check is what runs in
		// production, so runtime is the honest label; the sampled test
		// is what keeps it from drifting, and shows up in the
		// rationale.
		return PrecisionRuntime
	case BasisVacuousDatatype, BasisNotDischarged:
		return PrecisionUnproven
	}

	// No basis we recognise. Fall back to the discharge column, which
	// is a coarser but still meaningful statement.
	switch discharge {
	case DischargeStatic:
		return PrecisionStatic
	case DischargeRuntimeSampled:
		return PrecisionSampled
	case DischargeRuntimeAttested,
		DischargeRuntimeEvaluator,
		DischargeRuntimeAttestedSampled,
		DischargeRuntimeAttestedDB:
		return PrecisionRuntime
	}
	return PrecisionUnproven
}

// ApplyPrecision fills Precision on every premise of every rule. Safe
// to call repeatedly: the field is derived, never accumulated.
func ApplyPrecision(rules []Rule) {
	for i := range rules {
		for j := range rules[i].Premises {
			p := &rules[i].Premises[j]
			p.Precision = PrecisionFor(p.Discharge, p.DischargeBasis)
		}
	}
}

// WeakestPrecision returns the weakest precision across every premise
// of every rule — the report's headline claim, since a chain of
// reasoning is only as strong as its weakest link. Empty for no rules.
func WeakestPrecision(rules []Rule) string {
	weakest := ""
	for _, r := range rules {
		for _, p := range r.Premises {
			if p.Precision == "" {
				continue
			}
			if weakest == "" || PrecisionRank(p.Precision) > PrecisionRank(weakest) {
				weakest = p.Precision
			}
		}
	}
	return weakest
}

// ============================================================================
// Brand binding (W1 → W5)
// ============================================================================

// BrandTable is the subset of shengen's `--brand-table` JSON that the
// classifier needs. Decoded loosely: unknown fields are ignored so a
// newer shengen can add to the file without breaking older reports.
type BrandTable struct {
	Types []BrandTableType `json:"types"`
}

// BrandTableType is one Shen type's inferred brand signature.
type BrandTableType struct {
	ShenName      string `json:"shen_name"`
	GoName        string `json:"go_name"`
	Signature     string `json:"signature"`
	BoundPremises []int  `json:"bound_premises"`
	Bound         bool   `json:"bound"`
}

// LoadBrandTable reads a shengen --brand-table file. A missing path or
// a missing file yields (nil, nil): brands are opt-in, and a project
// that has not enabled them is not in error.
func LoadBrandTable(path string) (*BrandTable, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var bt BrandTable
	if err := json.Unmarshal(data, &bt); err != nil {
		return nil, err
	}
	return &bt, nil
}

// Lookup returns the entry for a Shen type name, or nil.
func (bt *BrandTable) Lookup(shenName string) *BrandTableType {
	if bt == nil {
		return nil
	}
	for i := range bt.Types {
		if bt.Types[i].ShenName == shenName {
			return &bt.Types[i]
		}
	}
	return nil
}

// ApplyBrandBinding upgrades the discharge basis of every field
// premise that W1's brand inference pairs, from
// `guard-type-at-boundary` to `guard-brand-bound`, and records the
// generic constructor signature that does the pairing.
//
// The upgrade is narrow on purpose. `guard-type-at-boundary` says the
// caller must hand over a value of the right *type*. `guard-brand-bound`
// says it must hand over a proof about the right *subject*: the brand
// is an unexported type minted inside the guards package, so a caller
// holding a BalanceChecked for some other transaction cannot name the
// type that would let it call NewSafeTransfer. That is the difference
// between "a balance was checked" and "this transaction's balance was
// checked", and it is the forgery the roadmap's ground-truth section
// opens with.
//
// Only field premises are touched: verified premises are discharged by
// the constructor's own validation, which brands do not change.
// Premise order in the report mirrors the rule's field order, which is
// the order the brand table indexes.
func ApplyBrandBinding(rules []Rule, bt *BrandTable, guardPath string) int {
	if bt == nil {
		return 0
	}
	upgraded := 0
	for i := range rules {
		entry := bt.Lookup(rules[i].Name)
		if entry == nil || len(entry.BoundPremises) == 0 {
			continue
		}
		bound := map[int]bool{}
		for _, idx := range entry.BoundPremises {
			bound[idx] = true
		}
		// Field premises come first and in declaration order (see
		// classifyDatatypeRule), so the nth field premise is premise n
		// in the brand table.
		field := 0
		for j := range rules[i].Premises {
			p := &rules[i].Premises[j]
			if !strings.Contains(p.ID, ".field-") {
				continue
			}
			idx := field
			field++
			if !bound[idx] || p.Discharge != DischargeStatic {
				continue
			}
			p.DischargeBasis = BasisGuardBrandBound
			p.BrandSignature = entry.Signature
			p.Rationale = brandBoundRationale(rules[i].Name, entry, p.Expression)
			if guardPath != "" {
				p.CodeReferences = appendUnique(p.CodeReferences,
					guardPath+":"+constructorRefSuffix(entry.GoName))
			}
			upgraded++
		}
	}
	ApplyPrecision(rules)
	return upgraded
}

// brandBoundRationale explains the upgrade in the report's own voice.
func brandBoundRationale(ruleName string, entry *BrandTableType, expression string) string {
	return "shengen's brand inference gives " + entry.Signature +
		" a phantom brand parameter shared with this premise, so `New" + entry.GoName +
		"` accepts only evidence minted for the same subject. " +
		"The premise `" + expression + "` is therefore discharged by proof binding, not merely by type: " +
		"a proof of the right type about the wrong value is a compile error, because the brand is an " +
		"unexported type the caller cannot name. (Rule " + ruleName + ".)"
}

// constructorRefSuffix names the generic constructor a brand-bound
// premise points at. The guards file is generated, so the constructor
// name is derivable rather than discovered — `readGuardLines` only
// indexes the datatype comment, which is the type, not the signature.
func constructorRefSuffix(goName string) string {
	return "New" + goName
}

func appendUnique(refs []string, ref string) []string {
	for _, r := range refs {
		if r == ref {
			return refs
		}
	}
	return append(refs, ref)
}

// ============================================================================
// Blame
// ============================================================================

// BlameInputs carries what the blame rules need beyond the
// counter-example itself.
type BlameInputs struct {
	// RuntimeVia is true when the failing premise carries a
	// `:runtime-via` annotation: the failure is in wrapper code.
	RuntimeVia bool
	// ShenHostAvailable is true when a live Shen host was consulted
	// alongside the spec's Go evaluator.
	ShenHostAvailable bool
	// HostDisagreed is true when the Go evaluator and the Shen host
	// produced different answers for this case. Meaningful only when
	// ShenHostAvailable.
	HostDisagreed bool
	// Vacuous is true when the enclosing rule is uninhabited.
	Vacuous bool
}

// AssignBlame applies the roadmap's blame rules, in order:
//
//	vacuous rule                          → spec
//	:runtime-via failure                  → wrapper
//	evaluator and host disagree           → lowering
//	evaluator and host agree, impl differs → impl
//	no host, impl differs                 → impl (basis: evaluator-only)
//
// The last row is the honest one for this repo today: with no Shen
// host installed, "the spec says X" means "the Go evaluator says X",
// and a lowering bug would look exactly like an implementation bug.
// Recording the basis rather than silently claiming `impl` is what
// keeps the report from overstating what it knows.
func AssignBlame(in BlameInputs) (blame, basis string) {
	switch {
	case in.Vacuous:
		return BlameSpec, BlameBasisVacuous
	case in.RuntimeVia:
		return BlameWrapper, BlameBasisRuntimeVia
	case in.ShenHostAvailable && in.HostDisagreed:
		return BlameLowering, BlameBasisEvaluatorHostDisagree
	case in.ShenHostAvailable:
		return BlameImpl, BlameBasisEvaluatorAndHost
	default:
		return BlameImpl, BlameBasisEvaluatorOnly
	}
}

// BlameLabel renders a blame value for a human reader: the party, and
// what to do about it.
func BlameLabel(blame string) string {
	switch blame {
	case BlameSpec:
		return "spec (the Shen rule itself is wrong or uninhabited)"
	case BlameImpl:
		return "impl (the implementation disagrees with the spec)"
	case BlameWrapper:
		return "wrapper (a :runtime-via checker or its generated wrapper)"
	case BlameLowering:
		return "lowering (the spec's two evaluators do not agree on its meaning)"
	}
	return blame
}
