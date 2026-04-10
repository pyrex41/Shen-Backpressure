package specfile

import (
	"path/filepath"
	"testing"
)

func TestBuildTypeTablePayment(t *testing.T) {
	sf, err := ParseFile(filepath.Clean(paymentSpecPath))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	tt := BuildTypeTable(sf.Datatypes, "example.com/payment/internal/shenguard", "shenguard")

	// account-id: wrapper around string
	if e := tt.Entries["account-id"]; e == nil {
		t.Fatal("missing account-id")
	} else {
		if e.Category != CatWrapper {
			t.Errorf("account-id category: %s", e.Category)
		}
		if e.GoQualified != "shenguard.AccountId" {
			t.Errorf("account-id GoQualified: %s", e.GoQualified)
		}
		if e.GoPrimType != "string" {
			t.Errorf("account-id GoPrimType: %s", e.GoPrimType)
		}
	}

	// amount: constrained (has >= 0 invariant)
	if e := tt.Entries["amount"]; e == nil {
		t.Fatal("missing amount")
	} else {
		if e.Category != CatConstrained {
			t.Errorf("amount category: %s", e.Category)
		}
		if e.GoQualified != "shenguard.Amount" {
			t.Errorf("amount GoQualified: %s", e.GoQualified)
		}
		if e.GoPrimType != "float64" {
			t.Errorf("amount GoPrimType: %s", e.GoPrimType)
		}
	}

	// transaction: composite with 3 fields
	if e := tt.Entries["transaction"]; e == nil {
		t.Fatal("missing transaction")
	} else {
		if e.Category != CatComposite {
			t.Errorf("transaction category: %s", e.Category)
		}
		if len(e.Fields) != 3 {
			t.Fatalf("transaction fields: %d", len(e.Fields))
		}
		wantFields := []struct {
			shen     string
			shenType string
			goMethod string
		}{
			{"Amount", "amount", "Amount"},
			{"From", "account-id", "From"},
			{"To", "account-id", "To"},
		}
		for i, wf := range wantFields {
			f := e.Fields[i]
			if f.ShenName != wf.shen || f.ShenType != wf.shenType || f.GoMethod != wf.goMethod {
				t.Errorf("field[%d]: got {%s,%s,%s}, want %v", i, f.ShenName, f.ShenType, f.GoMethod, wf)
			}
		}
	}

	// balance-invariant: guarded (composite with verified condition). The
	// conclusion type is balance-checked, not balance-invariant.
	if e := tt.Entries["balance-checked"]; e == nil {
		t.Fatal("missing balance-checked")
	} else if e.Category != CatGuarded {
		t.Errorf("balance-checked category: %s", e.Category)
	}
}

func TestGoType(t *testing.T) {
	sf, err := ParseFile(filepath.Clean(paymentSpecPath))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	tt := BuildTypeTable(sf.Datatypes, "", "shenguard")

	cases := []struct {
		in, want string
	}{
		{"number", "float64"},
		{"string", "string"},
		{"boolean", "bool"},
		{"amount", "shenguard.Amount"},
		{"(list transaction)", "[]shenguard.Transaction"},
		{"(list (list number))", "[][]float64"},
	}
	for _, tc := range cases {
		if got := tt.GoType(tc.in); got != tc.want {
			t.Errorf("GoType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
