package verify

import (
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

func TestBuildHarnessPayment(t *testing.T) {
	// Use ParseFile on a real temp file with both datatypes and a define.
	src := `
(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)

(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx))))) (val B0) Txs)))
`
	tmp := t_tempFile(src)
	defer t_removeFile(tmp)

	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(sf.Defines) != 1 {
		t.Fatalf("defines: got %d, want 1", len(sf.Defines))
	}
	def := sf.FindDefine("processable")
	if def == nil {
		t.Fatal("processable not found")
	}

	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/payment/internal/shenguard", "shenguard")

	cfg := &HarnessConfig{
		Spec:        def,
		TypeTable:   tt,
		ImplPkgPath: "example.com/payment/internal/derived",
		ImplPkgName: "derived",
		ImplFunc:    "Processable",
		TestPkgName: "derived_test",
		MaxCases:    6,
	}

	h, err := BuildHarness(cfg)
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if len(h.Cases) == 0 {
		t.Fatal("no cases generated")
	}
	t.Logf("generated %d cases", len(h.Cases))

	// Verify at least one case has a sensible expected value. With B0=0
	// and an empty list, processable should be true.
	//
	// We don't assert which case is which — we just check that the
	// expected values are all booleans.
	for _, c := range h.Cases {
		if _, ok := c.Expected.(core.BoolVal); !ok {
			t.Errorf("case %s: expected bool, got %T", c.Name, c.Expected)
		}
	}

	// Emit the Go source and sanity-check it.
	src2, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	t.Logf("emitted source:\n%s", src2)

	for _, want := range []string{
		"package derived_test",
		`shenguard "example.com/payment/internal/shenguard"`,
		`derived "example.com/payment/internal/derived"`,
		"func TestSpec_Processable(t *testing.T)",
		"mustAmount",
		"mustTransaction",
		"mustAccountId",
		"derived.Processable(tc.b0, tc.txs)",
	} {
		if !strings.Contains(src2, want) {
			t.Errorf("emitted source missing %q", want)
		}
	}
}

func TestConstraintFilteringRejectsNegativesForAmount(t *testing.T) {
	src := `
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)
`
	tmp := t_tempFile(src)
	defer t_removeFile(tmp)
	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	tt := specfile.BuildTypeTable(sf.Datatypes, "", "shenguard")

	samples, err := GenSamples("amount", tt)
	if err != nil {
		t.Fatalf("GenSamples: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("no samples")
	}
	for _, s := range samples {
		n, ok := core.AsNum(s.Value)
		if !ok {
			t.Errorf("sample %s: not numeric", s.GoExpr)
			continue
		}
		if n < 0 {
			t.Errorf("sample %s has negative value %v — constraint (>= X 0) not enforced", s.GoExpr, n)
		}
	}
	// And we should have more than the old "3 hardcoded positives" count.
	if len(samples) < 4 {
		t.Errorf("expected expanded sample set, got %d", len(samples))
	}
}

func TestDomainTypedReturn(t *testing.T) {
	// A spec whose return type is a constrained guard type.
	// double-amount takes an amount and returns double the underlying value,
	// wrapped back into an amount.
	src := `
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(define double-amount
  {amount --> amount}
  A -> (* 2 (val A)))
`
	tmp := t_tempFile(src)
	defer t_removeFile(tmp)
	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	def := sf.FindDefine("double-amount")
	if def == nil {
		t.Fatal("missing double-amount")
	}
	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/demo/internal/shenguard", "shenguard")

	h, err := BuildHarness(&HarnessConfig{
		Spec:        def,
		TypeTable:   tt,
		ImplPkgPath: "example.com/demo/internal/derived",
		ImplPkgName: "derived",
		ImplFunc:    "DoubleAmount",
		TestPkgName: "derived_test",
		MaxCases:    6,
	})
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}

	source, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	t.Logf("source:\n%s", source)

	// The want column should be float64, not shenguard.Amount.
	if !strings.Contains(source, "want float64") {
		t.Errorf("expected `want float64` for wrapper return")
	}
	// The comparison should unwrap via .Val().
	if !strings.Contains(source, "got.Val() != tc.want") {
		t.Errorf("expected `got.Val() != tc.want`")
	}

	// Spot-check one expected value: double-amount(5) should produce want=10.
	for _, c := range h.Cases {
		if c.Args[0].GoExpr == "mustAmount(5)" {
			if c.ExpectedGo != "10" {
				t.Errorf("double-amount(5): got want=%q, expected 10", c.ExpectedGo)
			}
		}
	}
}

func TestEvalSpecSimple(t *testing.T) {
	// A dead-simple "sum is non-negative" spec, no domain types.
	src := `
(define pos-sum
  {number --> (list number) --> boolean}
  Start Xs -> (>= (foldl + Start Xs) 0))
`
	tmp := t_tempFile(src)
	defer t_removeFile(tmp)
	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	def := sf.FindDefine("pos-sum")
	if def == nil {
		t.Fatal("missing pos-sum")
	}

	tt := specfile.BuildTypeTable(sf.Datatypes, "", "")

	h, err := BuildHarness(&HarnessConfig{
		Spec:        def,
		TypeTable:   tt,
		ImplFunc:    "PosSum",
		TestPkgName: "pos_sum_test",
		MaxCases:    6,
	})
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if len(h.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range h.Cases {
		if _, ok := c.Expected.(core.BoolVal); !ok {
			t.Errorf("case %s: expected bool, got %T (%v)", c.Name, c.Expected, c.Expected)
		}
	}
}
