package main

import (
	"strings"
	"testing"
)

// ============================================================================
// Target Selection / Inference Tests (parallel to shen-cedar; same shapes)
// ============================================================================

func TestRegoTargetInference(t *testing.T) {
	tests := []struct {
		name string
		flag string
		all  []string
		want []string
	}{
		{
			name: "explicit single",
			flag: "tenant-access",
			all:  []string{"user-id", "tenant-access", "amount"},
			want: []string{"tenant-access"},
		},
		{
			name: "explicit multiple comma",
			flag: "tenant-access,resource-access",
			all:  nil,
			want: []string{"tenant-access", "resource-access"},
		},
		{
			name: "inference on access suffixes",
			flag: "",
			all:  []string{"user-id", "tenant-access", "resource-access", "balance-checked"},
			want: []string{"tenant-access", "resource-access"},
		},
		{
			name: "inference on permit/allow suffixes",
			flag: "",
			all:  []string{"read-permit", "write-allow", "other"},
			want: []string{"read-permit", "write-allow"},
		},
		{
			name: "no matching suffixes (payment-like)",
			flag: "",
			all:  []string{"account-id", "amount", "transaction", "balance-checked"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectRegoTargets(tt.flag, tt.all)
			if len(got) != len(tt.want) {
				t.Fatalf("selectRegoTargets(%q, %v) = %v (len=%d), want %v (len=%d)",
					tt.flag, tt.all, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ============================================================================
// Emission structure (text Rego) smoke
// ============================================================================

func TestBuildRegoModule_ProducesDefaultDenyAndIfBodies(t *testing.T) {
	targets := []string{"tenant-access", "resource-access"}
	// Empty ruleMap → will use fallback comments but still emit rules + defaults.
	mod := buildRegoModule("multi_tenant_authz", targets, map[string][]ruleInfo{})

	if !strings.Contains(mod, "default tenant_access := false") {
		t.Error("expected default tenant_access := false")
	}
	if !strings.Contains(mod, "default resource_access := false") {
		t.Error("expected default resource_access := false")
	}
	if !strings.Contains(mod, "tenant_access if {") {
		t.Error("expected tenant_access if body")
	}
	if !strings.Contains(mod, "package multi_tenant_authz") {
		t.Error("expected package declaration")
	}
	if !strings.Contains(mod, "import rego.v1") {
		t.Error("expected rego.v1 import (modern OPA style)")
	}
}

func TestBuildRegoModule_SumTypeEmitsMultipleBodies(t *testing.T) {
	spec := `(datatype jwt-token
  X : string;
  (not (= X "")) : verified;
  ============================
  X : jwt-token;)

(datatype service-credential
  Secret : string;
  (not (= Secret "")) : verified;
  ================================
  Secret : service-credential;)

(datatype human-principal
  Auth : jwt-token;
  ===========================
  Auth : authenticated-principal;)

(datatype service-principal
  Cred : service-credential;
  ============================
  Cred : authenticated-principal;)

(datatype tenant-access
  Principal : authenticated-principal;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Principal IsMember] : tenant-access;)`
	variants := buildRuleVariants(parseDatatypes(spec))
	mod := buildRegoModule("authz", []string{"tenant-access"}, variants)
	if n := strings.Count(mod, "tenant_access if {"); n != 2 {
		t.Fatalf("expected 2 tenant_access bodies for the sum type, got %d:\n%s", n, mod)
	}
	if !strings.Contains(mod, `input.x != ""`) || !strings.Contains(mod, `input.secret != ""`) {
		t.Errorf("expected human (x) and service (secret) branches; got:\n%s", mod)
	}
}

func TestBuildRegoModule_InheritsTransitivePreconditions(t *testing.T) {
	spec := `(datatype tenant-access
  Principal : string;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Principal IsMember] : tenant-access;)

(datatype resource-access
  Access : tenant-access;
  IsOwned : boolean;
  (= IsOwned true) : verified;
  ================================
  [Access IsOwned] : resource-access;)`
	variants := buildRuleVariants(parseDatatypes(spec))
	mod := buildRegoModule("multi_tenant_authz", []string{"tenant-access", "resource-access"}, variants)

	// Isolate the resource_access rule body.
	idx := strings.Index(mod, "resource_access if {")
	if idx == -1 {
		t.Fatalf("no resource_access rule emitted:\n%s", mod)
	}
	body := mod[idx:]
	if end := strings.Index(body, "}"); end != -1 {
		body = body[:end]
	}
	if !strings.Contains(body, "input.isMember == true") {
		t.Errorf("resource_access body must inherit the tenant-access membership check; got:\n%s", body)
	}
	if !strings.Contains(body, "input.isOwned == true") {
		t.Errorf("resource_access body must include its own ownership check; got:\n%s", body)
	}
}

func TestRegoConditionFromVerified_NotEquality(t *testing.T) {
	// (not (= Token "")) must lower to a real inequality, not be dropped.
	cond := regoConditionFromVerified(VerifiedPremise{Raw: `(not (= Token ""))`})
	if cond != `input.token != ""` {
		t.Errorf("expected `input.token != \"\"`, got %q", cond)
	}
}

func TestRegoConditionFromVerified_ElementMembership(t *testing.T) {
	// (element? Elem Coll) must lower to Rego membership (consistent with Cedar contains).
	cond := regoConditionFromVerified(VerifiedPremise{Raw: "(element? Role Roles)"})
	if cond != "input.role in input.roles" {
		t.Errorf("expected `input.role in input.roles`, got %q", cond)
	}
}

func TestRegoConditionFromVerified_SimpleEquality(t *testing.T) {
	v := VerifiedPremise{Raw: "(= IsMember true)"}
	cond := regoConditionFromVerified(v)
	if cond == "" || !strings.Contains(cond, "input.isMember == true") {
		t.Errorf("expected input.isMember == true lowering, got %q", cond)
	}
}

func containsString(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
