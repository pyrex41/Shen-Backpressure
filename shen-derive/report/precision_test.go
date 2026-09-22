package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrecisionForEveryBasis(t *testing.T) {
	cases := []struct {
		discharge string
		basis     string
		want      string
	}{
		{DischargeStatic, BasisGuardBrandBound, PrecisionStatic},
		{DischargeStatic, BasisGuardTypeAtBoundary, PrecisionStatic},
		{DischargeStatic, BasisGuardConstructorValidates, PrecisionStatic},
		{DischargeRuntimeSampled, BasisProverZ3PathCover, PrecisionPathCover},
		{DischargeRuntimeSampled, BasisShenDeriveSampled, PrecisionSampled},
		{DischargeRuntimeAttested, BasisRuntimeViaWitness, PrecisionRuntime},
		{DischargeRuntimeEvaluator, BasisRuntimeViaEvaluator, PrecisionRuntime},
		{DischargeRuntimeAttestedSampled, BasisRuntimeViaSampledEquivalence, PrecisionRuntime},
		{DischargeRuntimeAttestedDB, BasisRuntimeViaDBAttested, PrecisionRuntime},
		{DischargeUnproven, BasisVacuousDatatype, PrecisionUnproven},
		{DischargeUnproven, BasisNotDischarged, PrecisionUnproven},
		// Unknown basis falls back to the discharge column…
		{DischargeStatic, "flow-analysis", PrecisionStatic},
		// …and an unknown discharge falls all the way to unproven,
		// never to something stronger.
		{"who-knows", "also-unknown", PrecisionUnproven},
	}
	for _, c := range cases {
		if got := PrecisionFor(c.discharge, c.basis); got != c.want {
			t.Errorf("PrecisionFor(%q, %q) = %q, want %q", c.discharge, c.basis, got, c.want)
		}
	}
}

// TestPrecisionRankIsTotal pins the order the roadmap specifies:
// static > path-cover > sampled > runtime > unproven.
func TestPrecisionRankIsTotal(t *testing.T) {
	want := []string{
		PrecisionStatic, PrecisionPathCover, PrecisionSampled,
		PrecisionRuntime, PrecisionUnproven,
	}
	for i := 1; i < len(want); i++ {
		if PrecisionRank(want[i-1]) >= PrecisionRank(want[i]) {
			t.Errorf("%s should outrank %s", want[i-1], want[i])
		}
	}
	// An unrecognised value must be weaker than everything known, so a
	// report from a newer tool can never look stronger than it is.
	if PrecisionRank("quantum") <= PrecisionRank(PrecisionUnproven) {
		t.Error("unknown precision must rank below unproven")
	}
}

func TestWeakestPrecision(t *testing.T) {
	rules := []Rule{
		{Premises: []Premise{{Precision: PrecisionStatic}, {Precision: PrecisionSampled}}},
		{Premises: []Premise{{Precision: PrecisionStatic}}},
	}
	if got := WeakestPrecision(rules); got != PrecisionSampled {
		t.Errorf("WeakestPrecision = %q, want sampled", got)
	}
	if got := WeakestPrecision(nil); got != "" {
		t.Errorf("WeakestPrecision(nil) = %q, want empty", got)
	}
}

func TestAssignBlame(t *testing.T) {
	cases := []struct {
		name      string
		in        BlameInputs
		wantBlame string
		wantBasis string
	}{
		{
			"vacuous rule blames the spec",
			BlameInputs{Vacuous: true},
			BlameSpec, BlameBasisVacuous,
		},
		{
			"runtime-via failure blames the wrapper",
			BlameInputs{RuntimeVia: true},
			BlameWrapper, BlameBasisRuntimeVia,
		},
		{
			"evaluator and host disagree blames the lowering",
			BlameInputs{ShenHostAvailable: true, HostDisagreed: true},
			BlameLowering, BlameBasisEvaluatorHostDisagree,
		},
		{
			"evaluator and host agree blames the impl",
			BlameInputs{ShenHostAvailable: true},
			BlameImpl, BlameBasisEvaluatorAndHost,
		},
		{
			"no host still blames the impl, but says only the evaluator spoke",
			BlameInputs{},
			BlameImpl, BlameBasisEvaluatorOnly,
		},
		{
			"vacuity outranks everything else",
			BlameInputs{Vacuous: true, RuntimeVia: true, ShenHostAvailable: true, HostDisagreed: true},
			BlameSpec, BlameBasisVacuous,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blame, basis := AssignBlame(c.in)
			if blame != c.wantBlame || basis != c.wantBasis {
				t.Errorf("AssignBlame(%+v) = (%q, %q), want (%q, %q)",
					c.in, blame, basis, c.wantBlame, c.wantBasis)
			}
		})
	}
}

// TestApplyBrandBinding upgrades exactly the paired premises and
// leaves everything else alone.
func TestApplyBrandBinding(t *testing.T) {
	rules := []Rule{
		{
			Name: "safe-transfer",
			Premises: []Premise{
				{ID: "safe-transfer.field-tx", Expression: "Tx : transaction",
					Discharge: DischargeStatic, DischargeBasis: BasisGuardTypeAtBoundary},
				{ID: "safe-transfer.field-check", Expression: "Check : balance-checked",
					Discharge: DischargeStatic, DischargeBasis: BasisGuardTypeAtBoundary},
			},
		},
		{
			Name: "balance-checked",
			Premises: []Premise{
				{ID: "balance-checked.field-bal", Expression: "Bal : number",
					Discharge: DischargeStatic, DischargeBasis: BasisGuardTypeAtBoundary},
				{ID: "balance-checked.field-tx", Expression: "Tx : transaction",
					Discharge: DischargeStatic, DischargeBasis: BasisGuardTypeAtBoundary},
				{ID: "balance-checked.verified-bal-head-tx", Expression: "(>= Bal (head Tx)) : verified",
					Discharge: DischargeStatic, DischargeBasis: BasisGuardConstructorValidates},
			},
		},
	}
	bt := &BrandTable{Types: []BrandTableType{
		{ShenName: "safe-transfer", GoName: "SafeTransfer", Signature: "SafeTransfer[B]",
			BoundPremises: []int{0, 1}, Bound: true},
		{ShenName: "balance-checked", GoName: "BalanceChecked", Signature: "BalanceChecked[B]",
			BoundPremises: []int{1}, Bound: true},
	}}

	n := ApplyBrandBinding(rules, bt, "internal/shenguard/guards_gen.go")
	if n != 3 {
		t.Errorf("upgraded %d premises, want 3", n)
	}

	for _, p := range rules[0].Premises {
		if p.DischargeBasis != BasisGuardBrandBound {
			t.Errorf("%s: basis %q, want guard-brand-bound", p.ID, p.DischargeBasis)
		}
		if p.BrandSignature != "SafeTransfer[B]" {
			t.Errorf("%s: brand signature %q", p.ID, p.BrandSignature)
		}
		if p.Precision != PrecisionStatic {
			t.Errorf("%s: precision %q, want static", p.ID, p.Precision)
		}
		found := false
		for _, ref := range p.CodeReferences {
			if strings.HasSuffix(ref, ":NewSafeTransfer") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no code reference at the generic constructor: %v", p.ID, p.CodeReferences)
		}
	}

	// balance-checked: the bare number is not evidence, so premise 0
	// keeps the plain boundary basis; the verified premise is never
	// touched, because brands do not change what the constructor
	// validates.
	if rules[1].Premises[0].DischargeBasis != BasisGuardTypeAtBoundary {
		t.Errorf("Bal should keep guard-type-at-boundary, got %q", rules[1].Premises[0].DischargeBasis)
	}
	if rules[1].Premises[1].DischargeBasis != BasisGuardBrandBound {
		t.Errorf("Tx should be brand-bound, got %q", rules[1].Premises[1].DischargeBasis)
	}
	if rules[1].Premises[2].DischargeBasis != BasisGuardConstructorValidates {
		t.Errorf("verified premise should be untouched, got %q", rules[1].Premises[2].DischargeBasis)
	}
}

// TestApplyBrandBindingNoTable is the --no-brands project: nothing
// changes, and no error is raised.
func TestApplyBrandBindingNoTable(t *testing.T) {
	rules := []Rule{{Name: "x", Premises: []Premise{
		{ID: "x.field-a", Discharge: DischargeStatic, DischargeBasis: BasisGuardTypeAtBoundary},
	}}}
	if n := ApplyBrandBinding(rules, nil, ""); n != 0 {
		t.Errorf("upgraded %d premises with no table", n)
	}
	if rules[0].Premises[0].DischargeBasis != BasisGuardTypeAtBoundary {
		t.Error("basis changed with no brand table")
	}
}

func TestLoadBrandTable(t *testing.T) {
	if bt, err := LoadBrandTable(""); bt != nil || err != nil {
		t.Errorf("empty path: got (%v, %v), want (nil, nil)", bt, err)
	}
	missing := filepath.Join(t.TempDir(), "absent.json")
	if bt, err := LoadBrandTable(missing); bt != nil || err != nil {
		t.Errorf("missing file: got (%v, %v), want (nil, nil)", bt, err)
	}

	path := filepath.Join(t.TempDir(), "brands.json")
	body := `{"schema_version":1,"spec":"specs/core.shen","types":[
	  {"shen_name":"safe-transfer","go_name":"SafeTransfer","signature":"SafeTransfer[B]",
	   "bound_premises":[0,1],"bound":true,"future_field":"ignored"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bt, err := LoadBrandTable(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := bt.Lookup("safe-transfer")
	if entry == nil || !entry.Bound || len(entry.BoundPremises) != 2 {
		t.Fatalf("bad entry: %+v", entry)
	}
	if bt.Lookup("nope") != nil {
		t.Error("Lookup of an absent type should be nil")
	}
}

// TestW5FieldsAreOmittedWhenAbsent is the additivity guarantee: a
// report that carries none of the W5 material marshals without any of
// the new keys, so pre-W5 consumers see byte-identical documents.
func TestW5FieldsAreOmittedWhenAbsent(t *testing.T) {
	r := &Report{
		SchemaVersion: SchemaVersion,
		Rules: []Rule{{
			Name:            "x",
			Premises:        []Premise{{ID: "x.field-a", Discharge: DischargeStatic}},
			CounterExamples: []CounterExample{{CaseID: "case_01"}},
		}},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"toolchain", "precision", "brand_signature", "blame", "blame_basis"} {
		if strings.Contains(string(data), `"`+key+`"`) {
			t.Errorf("unset %s key leaked into the JSON: %s", key, data)
		}
	}
	// signature stays a required, explicitly-null field.
	if !strings.Contains(string(data), `"signature":null`) {
		t.Errorf("signature should still render as null: %s", data)
	}
}
