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

func TestSummary(t *testing.T) {
	sf, err := ParseFile(filepath.Clean(paymentSpecPath))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	tt := BuildTypeTable(sf.Datatypes, "example.com/payment/internal/shenguard", "shenguard")
	summaries := tt.Summary()

	if len(summaries) == 0 {
		t.Fatal("empty summary")
	}

	// Build a lookup map for convenience.
	byName := map[string]TypeSummary{}
	for _, s := range summaries {
		byName[s.ShenName] = s
	}

	// account-id: wrapper
	if s, ok := byName["account-id"]; !ok {
		t.Error("missing account-id in summary")
	} else {
		if s.Category != "wrapper" {
			t.Errorf("account-id category: %s", s.Category)
		}
		if s.TargetName != "AccountId" {
			t.Errorf("account-id TargetName: %s", s.TargetName)
		}
	}

	// amount: constrained with a constraint
	if s, ok := byName["amount"]; !ok {
		t.Error("missing amount in summary")
	} else {
		if s.Category != "constrained" {
			t.Errorf("amount category: %s", s.Category)
		}
		if len(s.Constraints) != 1 || s.Constraints[0] != "(>= X 0)" {
			t.Errorf("amount constraints: %v", s.Constraints)
		}
	}

	// transaction: composite with 3 fields
	if s, ok := byName["transaction"]; !ok {
		t.Error("missing transaction in summary")
	} else {
		if s.Category != "composite" {
			t.Errorf("transaction category: %s", s.Category)
		}
		if len(s.Fields) != 3 {
			t.Errorf("transaction fields: %v", s.Fields)
		}
		// Should have dependencies on amount and account-id
		if len(s.Dependencies) != 2 {
			t.Errorf("transaction dependencies: %v", s.Dependencies)
		}
	}

	// balance-checked: guarded with constraints
	if s, ok := byName["balance-checked"]; !ok {
		t.Error("missing balance-checked in summary")
	} else {
		if s.Category != "guarded" {
			t.Errorf("balance-checked category: %s", s.Category)
		}
		if len(s.Constraints) == 0 {
			t.Error("balance-checked: expected constraints")
		}
	}
}

func TestSummarySorted(t *testing.T) {
	sf, err := ParseFile(filepath.Clean(paymentSpecPath))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	tt := BuildTypeTable(sf.Datatypes, "", "shenguard")
	summaries := tt.Summary()

	for i := 1; i < len(summaries); i++ {
		if summaries[i].ShenName < summaries[i-1].ShenName {
			t.Errorf("not sorted: %q before %q", summaries[i-1].ShenName, summaries[i].ShenName)
		}
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
