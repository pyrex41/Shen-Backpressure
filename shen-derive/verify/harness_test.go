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

func TestBuildHarnessMultiClauseWithRecursion(t *testing.T) {
	// A multi-clause define with `where` guards that calls itself
	// recursively via the base env's define bindings. This is the
	// integration test for Direction B: pattern matching + guards +
	// recursive lookups all have to line up.
	src := `
(datatype drug-id
  X : string;
  ============
  X : drug-id;)

(datatype contraindication
  DrugA : drug-id;
  DrugB : drug-id;
  ========================
  [DrugA DrugB] : contraindication;)

(define pair-in-list?
  {drug-id --> drug-id --> (list contraindication) --> boolean}
  _ _ [] -> false
  A B [[X Y] | Rest] -> true  where (and (= A X) (= B Y))
  A B [[X Y] | Rest] -> true  where (and (= A Y) (= B X))
  A B [_ | Rest] -> (pair-in-list? A B Rest))
`
	tmp := t_tempFile(src)
	defer t_removeFile(tmp)

	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	def := sf.FindDefine("pair-in-list?")
	if def == nil {
		t.Fatal("pair-in-list? not found")
	}
	if got := len(def.Clauses); got != 4 {
		t.Fatalf("clauses: got %d, want 4", got)
	}

	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/dose/internal/shenguard", "shenguard")
	allDefines := make([]*specfile.Define, len(sf.Defines))
	for i := range sf.Defines {
		allDefines[i] = &sf.Defines[i]
	}

	base := buildBaseEnv(tt, allDefines)

	// Direct evaluation probes — bypass BuildHarness so we can assert on
	// specific values and exercise the recursive case.
	drugA := core.StringVal("alice")
	drugB := core.StringVal("bob")
	pairAB := core.ListVal{drugA, drugB}
	pairBA := core.ListVal{drugB, drugA}
	pairOther := core.ListVal{core.StringVal("carol"), core.StringVal("dave")}

	cases := []struct {
		name string
		args []core.Value
		want bool
	}{
		{"empty list → clause 0", []core.Value{drugA, drugB, core.ListVal(nil)}, false},
		{"direct match → clause 1", []core.Value{drugA, drugB, core.ListVal{pairAB}}, true},
		{"swapped match → clause 2", []core.Value{drugA, drugB, core.ListVal{pairBA}}, true},
		{"skip then match → clauses 3→1", []core.Value{drugA, drugB, core.ListVal{pairOther, pairAB}}, true},
		{"never matches → clause 3 recurse → 0", []core.Value{drugA, drugB, core.ListVal{pairOther, pairOther}}, false},
	}
	for _, tc := range cases {
		got, err := evalDefine(def, tc.args, base)
		if err != nil {
			t.Errorf("%s: eval error: %v", tc.name, err)
			continue
		}
		gb, ok := got.(core.BoolVal)
		if !ok {
			t.Errorf("%s: got %T, want BoolVal", tc.name, got)
			continue
		}
		if bool(gb) != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, bool(gb), tc.want)
		}
	}

	// End-to-end: BuildHarness should succeed on this spec, generate cases,
	// and emit valid-looking Go source.
	h, err := BuildHarness(&HarnessConfig{
		Spec:        def,
		TypeTable:   tt,
		AllDefines:  allDefines,
		ImplPkgPath: "example.com/dose/internal/derived",
		ImplPkgName: "derived",
		ImplFunc:    "PairInList",
		TestPkgName: "derived_test",
		MaxCases:    20,
	})
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if len(h.Cases) == 0 {
		t.Fatal("no cases generated")
	}
	for _, c := range h.Cases {
		if _, ok := c.Expected.(core.BoolVal); !ok {
			t.Errorf("case %s: expected bool, got %T", c.Name, c.Expected)
		}
	}

	source, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	for _, want := range []string{
		"func TestSpec_PairInList(t *testing.T)",
		"mustDrugId",
		"mustContraindication",
		"derived.PairInList(",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("emitted source missing %q", want)
		}
	}
}

func TestSeededSamplingDeterministicAndReproducible(t *testing.T) {
	// A spec with a pure-number signature. With seed=0, only the boundary
	// pool is used; with seed=42, 8 random draws are appended per primitive.
	src := `
(define add-one
  {number --> number}
  X -> (+ X 1))
`
	tmp := t_tempFile(src)
	defer t_removeFile(tmp)
	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	def := sf.FindDefine("add-one")
	if def == nil {
		t.Fatal("missing add-one")
	}
	tt := specfile.BuildTypeTable(sf.Datatypes, "", "")

	buildWithSeed := func(seed int64) *Harness {
		t.Helper()
		h, err := BuildHarness(&HarnessConfig{
			Spec:        def,
			TypeTable:   tt,
			ImplFunc:    "AddOne",
			TestPkgName: "pkg_test",
			MaxCases:    50,
			Seed:        seed,
		})
		if err != nil {
			t.Fatalf("BuildHarness seed=%d: %v", seed, err)
		}
		return h
	}

	hDet := buildWithSeed(0)
	hSeed1 := buildWithSeed(42)
	hSeed2 := buildWithSeed(42)

	// Deterministic (seed=0): 6 boundary values → 6 cases.
	if len(hDet.Cases) != 6 {
		t.Errorf("seed=0 cases: got %d, want 6", len(hDet.Cases))
	}

	// Seeded (seed=42): boundary (6) + 8 random draws = 14 cases.
	if len(hSeed1.Cases) != 14 {
		t.Errorf("seed=42 cases: got %d, want 14", len(hSeed1.Cases))
	}

	// Reproducibility: two runs with the same seed produce identical
	// input expressions.
	if len(hSeed1.Cases) != len(hSeed2.Cases) {
		t.Fatal("reproducibility: case counts differ")
	}
	for i := range hSeed1.Cases {
		a := hSeed1.Cases[i].Args[0].GoExpr
		b := hSeed2.Cases[i].Args[0].GoExpr
		if a != b {
			t.Errorf("case %d: seed-42 runs diverged: %q vs %q", i, a, b)
		}
	}

	// The seeded run should actually contain a value outside the boundary
	// pool (i.e. not one of 0, 1, -1, 5, 2.5, 100).
	boundary := map[string]bool{"0": true, "1": true, "-1": true, "5": true, "2.5": true, "100": true}
	foundNovel := false
	for _, c := range hSeed1.Cases {
		if !boundary[c.Args[0].GoExpr] {
			foundNovel = true
			break
		}
	}
	if !foundNovel {
		t.Error("seeded run produced no samples outside the boundary pool")
	}

	// Header comment should stamp the seed.
	src2, err := hSeed1.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if !strings.Contains(src2, "Sampling seed: 42") {
		t.Error("emitted source missing seed stamp")
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
