package specfile

import (
	"path/filepath"
	"testing"
)

const paymentSpecPath = "../../examples/payment/specs/core.shen"

func TestParsePaymentSpec(t *testing.T) {
	sf, err := ParseFile(filepath.Clean(paymentSpecPath))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if got, want := len(sf.Datatypes), 6; got != want {
		t.Fatalf("datatypes: got %d, want %d", got, want)
	}

	wantNames := []string{"account-id", "amount", "transaction", "balance-invariant", "account-state", "safe-transfer"}
	for i, name := range wantNames {
		if sf.Datatypes[i].Name != name {
			t.Errorf("datatype[%d]: got %q, want %q", i, sf.Datatypes[i].Name, name)
		}
	}

	// Spot-check amount: single wrapped premise + one verified condition.
	amount := sf.FindDatatype("amount")
	if amount == nil || len(amount.Rules) != 1 {
		t.Fatalf("amount: expected 1 rule")
	}
	r := amount.Rules[0]
	if len(r.Premises) != 1 || r.Premises[0].TypeName != "number" {
		t.Errorf("amount premises: %+v", r.Premises)
	}
	if len(r.Verified) != 1 || r.Verified[0].Raw != "(>= X 0)" {
		t.Errorf("amount verified: %+v", r.Verified)
	}
	if !r.Conclusion.IsWrapped || r.Conclusion.TypeName != "amount" {
		t.Errorf("amount conclusion: %+v", r.Conclusion)
	}

	// Spot-check transaction: composite with three fields.
	tx := sf.FindDatatype("transaction")
	if tx == nil || len(tx.Rules) != 1 {
		t.Fatalf("transaction: expected 1 rule")
	}
	txr := tx.Rules[0]
	if txr.Conclusion.IsWrapped {
		t.Errorf("transaction should be composite, not wrapped")
	}
	if got := txr.Conclusion.Fields; len(got) != 3 || got[0] != "Amount" || got[1] != "From" || got[2] != "To" {
		t.Errorf("transaction fields: %v", got)
	}

	// The payment spec now includes a (define processable ...) block.
	if len(sf.Defines) != 1 {
		t.Fatalf("defines: got %d, want 1", len(sf.Defines))
	}
	proc := sf.FindDefine("processable")
	if proc == nil {
		t.Fatal("processable not found")
	}
	if got := proc.TypeSig.ParamTypes; len(got) != 2 || got[0] != "amount" || got[1] != "(list transaction)" {
		t.Errorf("processable param types: %v", got)
	}
	if proc.TypeSig.ReturnType != "boolean" {
		t.Errorf("processable return type: %q", proc.TypeSig.ReturnType)
	}
	if got := proc.ParamNames; len(got) != 2 || got[0] != "B0" || got[1] != "Txs" {
		t.Errorf("processable param names: %v", got)
	}
}

func TestParseTypeSig(t *testing.T) {
	cases := []struct {
		in         string
		wantParams []string
		wantRet    string
	}{
		{"{number --> number}", []string{"number"}, "number"},
		{"{amount --> (list transaction) --> boolean}", []string{"amount", "(list transaction)"}, "boolean"},
		{"{string --> string --> number --> boolean}", []string{"string", "string", "number"}, "boolean"},
	}
	for _, tc := range cases {
		sig, err := parseTypeSig(tc.in)
		if err != nil {
			t.Errorf("parseTypeSig(%q): %v", tc.in, err)
			continue
		}
		if len(sig.ParamTypes) != len(tc.wantParams) {
			t.Errorf("parseTypeSig(%q): params=%v, want %v", tc.in, sig.ParamTypes, tc.wantParams)
			continue
		}
		for i, p := range sig.ParamTypes {
			if p != tc.wantParams[i] {
				t.Errorf("parseTypeSig(%q): params[%d]=%q, want %q", tc.in, i, p, tc.wantParams[i])
			}
		}
		if sig.ReturnType != tc.wantRet {
			t.Errorf("parseTypeSig(%q): ret=%q, want %q", tc.in, sig.ReturnType, tc.wantRet)
		}
	}
}

func TestParseDefine(t *testing.T) {
	block := `(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx))))) (val B0) Txs)))`

	def, err := parseDefine(block)
	if err != nil {
		t.Fatalf("parseDefine: %v", err)
	}
	if def.Name != "processable" {
		t.Errorf("name: %q", def.Name)
	}
	if got := def.TypeSig.ParamTypes; len(got) != 2 || got[0] != "amount" || got[1] != "(list transaction)" {
		t.Errorf("param types: %v", got)
	}
	if def.TypeSig.ReturnType != "boolean" {
		t.Errorf("return type: %q", def.TypeSig.ReturnType)
	}
	if got := def.ParamNames; len(got) != 2 || got[0] != "B0" || got[1] != "Txs" {
		t.Errorf("param names: %v", got)
	}
	if def.Body == nil {
		t.Fatal("body is nil")
	}
}

func TestStripShenComments(t *testing.T) {
	in := `\* line comment *\
(datatype foo
  X : number;
  ============
  X : foo;)
\* another *\`
	out := stripShenComments(in)
	if containsAny(out, `\*`, `*\`) {
		t.Errorf("comments not stripped: %q", out)
	}
	if !containsAll(out, "datatype foo", "X : number") {
		t.Errorf("body lost: %q", out)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) < 0 {
			return false
		}
	}
	return true
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
