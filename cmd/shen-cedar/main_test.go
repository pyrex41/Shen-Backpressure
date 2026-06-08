package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// ============================================================================
// Lowering semantics: transitive preconditions must not be dropped.
// ============================================================================

const chainedAccessSpec = `(datatype tenant-access
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

func TestCollectTransitiveVerified_InheritsDependencyPremises(t *testing.T) {
	rm := buildRuleMap(parseDatatypes(chainedAccessSpec))

	// tenant-access has only its own membership premise.
	tenant := collectTransitiveVerified("tenant-access", rm, map[string]bool{})
	if len(tenant) != 1 || !strings.Contains(tenant[0].Raw, "IsMember") {
		t.Fatalf("tenant-access verified = %+v, want one IsMember premise", tenant)
	}

	// resource-access must inherit tenant-access's membership check (via the
	// `Access : tenant-access` premise) in addition to its own ownership check.
	res := collectTransitiveVerified("resource-access", rm, map[string]bool{})
	var sawMember, sawOwned bool
	for _, v := range res {
		if strings.Contains(v.Raw, "IsMember") {
			sawMember = true
		}
		if strings.Contains(v.Raw, "IsOwned") {
			sawOwned = true
		}
	}
	if !sawMember {
		t.Error("resource-access lowering dropped the inherited tenant-access membership precondition")
	}
	if !sawOwned {
		t.Error("resource-access lowering missing its own ownership premise")
	}
}

func TestCedarBodyFromVerified_ElementMembership(t *testing.T) {
	// (element? Elem Coll) must lower to Cedar contains (consistent with Rego `in`).
	body := cedarBodyFromVerified(VerifiedPremise{Raw: "(element? Role Roles)"})
	c, ok := body["contains"].(map[string]any)
	if !ok {
		t.Fatalf("expected a contains body, got %+v", body)
	}
	left, _ := c["left"].(map[string]any)
	if dot, _ := left["."].(map[string]any); dot["attr"] != "roles" {
		t.Errorf("expected left context.roles (the collection), got %+v", left)
	}
	right, _ := c["right"].(map[string]any)
	if dot, _ := right["."].(map[string]any); dot["attr"] != "role" {
		t.Errorf("expected right context.role (the element), got %+v", right)
	}
}

func TestCedarBodyFromVerified_NotEquality(t *testing.T) {
	// (not (= Token "")) must lower to a Cedar != body, not be dropped.
	body := cedarBodyFromVerified(VerifiedPremise{Raw: `(not (= Token ""))`})
	ne, ok := body["!="].(map[string]any)
	if !ok {
		t.Fatalf("expected a != body, got %+v", body)
	}
	left, _ := ne["left"].(map[string]any)
	if dot, _ := left["."].(map[string]any); dot["attr"] != "token" {
		t.Errorf("expected left context.token, got %+v", left)
	}
	if right, _ := ne["right"].(map[string]any); right["Value"] != "" {
		t.Errorf("expected right Value \"\", got %+v", ne["right"])
	}
}

const sumTypeAccessSpec = `(datatype jwt-token
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

func TestBuildCedarPolicies_SumTypeEmitsDisjunction(t *testing.T) {
	variants := buildRuleVariants(parseDatatypes(sumTypeAccessSpec))
	set := buildCedarPolicies([]string{"tenant-access"}, variants)
	if len(set.Policies) != 2 {
		t.Fatalf("authenticated-principal sum type must emit 2 permit policies (human ∨ service); got %d", len(set.Policies))
	}
	for _, p := range set.Policies {
		if p.Annotations["shen_conclusion"] != "tenant-access" {
			t.Errorf("each clause policy must annotate shen_conclusion=tenant-access; got %v", p.Annotations)
		}
	}
	b, _ := json.Marshal(set)
	js := string(b)
	if !strings.Contains(js, "isMember") {
		t.Error("both clauses must carry the isMember precondition")
	}
}

func TestBuildCedarPolicies_ResourceAccessCarriesMembership(t *testing.T) {
	variants := buildRuleVariants(parseDatatypes(chainedAccessSpec))
	set := buildCedarPolicies([]string{"resource-access"}, variants)
	b, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(b)
	if !strings.Contains(js, "isMember") {
		t.Error("emitted resource-access policy must reference isMember (inherited precondition)")
	}
	if !strings.Contains(js, "isOwned") {
		t.Error("emitted resource-access policy must reference isOwned")
	}
	if !strings.Contains(js, `"attr":"level"`) || !strings.Contains(js, `"Value":"resource"`) {
		t.Error("emitted resource-access policy must be scoped to resource-level requests")
	}
}

// ============================================================================
// Target Selection / Inference Tests (table-driven, explicit + inference + edge)
// ============================================================================

func TestTargetInference(t *testing.T) {
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
			name: "explicit with spaces",
			flag: " foo-bar , baz-permit ",
			all:  []string{},
			want: []string{"foo-bar", "baz-permit"},
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
			name: "inference case insensitive suffix",
			flag: "",
			all:  []string{"Foo-Access", "Bar-PERMIT", "Baz-Allow"},
			want: []string{"Foo-Access", "Bar-PERMIT", "Baz-Allow"},
		},
		{
			name: "empty conclusions yields empty",
			flag: "",
			all:  []string{},
			want: nil, // infer returns nil slice when no matches
		},
		{
			name: "no matching suffixes (payment-like non-access spec)",
			flag: "",
			all:  []string{"account-id", "amount", "transaction", "balance-checked", "account-state", "safe-transfer"},
			want: nil, // no -access/-permit/-allow
		},
		{
			name: "explicit overrides even on access list",
			flag: "payment-check",
			all:  []string{"tenant-access", "resource-access"},
			want: []string{"payment-check"},
		},
		{
			name: "explicit empty string falls back to inference",
			flag: "",
			all:  []string{"x-access"},
			want: []string{"x-access"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectCedarTargets(tt.flag, tt.all)
			if len(got) != len(tt.want) {
				t.Fatalf("selectCedarTargets(%q, %v) = %v (len=%d), want %v (len=%d)",
					tt.flag, tt.all, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q want %q (full: got=%v want=%v)", i, got[i], tt.want[i], got, tt.want)
				}
			}
		})
	}
}

// ============================================================================
// Emission Structure Tests
// ============================================================================

func TestEmitsValidCedarJSON(t *testing.T) {
	// Builders must produce structs that serialize to the expected minimal valid Cedar JSON shape.
	// (Do not change emitted artifacts in this refactor; only testability.)
	schema := buildMinimalCedarSchema(nil)
	if schema.Schema != "2021-06-01" {
		t.Errorf("schema version = %q, want %q", schema.Schema, "2021-06-01")
	}
	expectedEntities := []string{"User", "Tenant", "Resource"}
	for _, e := range expectedEntities {
		if _, ok := schema.Entities[e]; !ok {
			t.Errorf("schema.Entities missing expected %q; have keys: %v", e, keysOf(schema.Entities))
		}
	}
	if len(schema.Actions) == 0 {
		t.Error("schema.Actions should not be empty")
	}

	// Policies: must be a valid array (non-nil in struct, and key present after marshal)
	policies := buildMinimalCedarPolicies([]string{"tenant-access", "resource-access"})
	if policies.FormatVersion != 1 {
		t.Errorf("policies.FormatVersion = %d, want 1", policies.FormatVersion)
	}
	if policies.PolicyStoreID == "" {
		t.Error("expected non-empty PolicyStoreID")
	}
	if policies.Policies == nil {
		t.Fatal("policies.Policies must be a (possibly empty) slice, not nil for valid array emission")
	}
	// Even in v0 minimal, we emit at least the starter policy so the array is populated.
	if len(policies.Policies) == 0 {
		t.Error("expected at least one policy in the array for current minimal builder")
	}

	// Verify it round-trips as valid JSON with a "policies" array key (smoke for "valid Cedar JSON").
	b, err := json.Marshal(policies)
	if err != nil {
		t.Fatalf("failed to marshal policies: %v", err)
	}
	js := string(b)
	if !strings.Contains(js, `"policies"`) {
		t.Error("marshaled policies JSON must contain a \"policies\" key (array)")
	}
	if !strings.Contains(js, `"format_version"`) {
		t.Error("marshaled policies JSON must contain format_version")
	}
}

func keysOf(m map[string]cedarEntity) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ============================================================================
// Parser Smoke Tests (multi-tenant access rules + basic extraction)
// ============================================================================

func TestParse_MultiTenantAccessRules(t *testing.T) {
	// Access datatype shapes from examples/multi-tenant-api/specs/core.shen, plus
	// prerequisite wrappers to exercise multi-block parsing.
	spec := `(datatype tenant-id
  X : string;
  ==============
  X : tenant-id;)

(datatype authenticated-principal
  X : string;
  ==============
  X : authenticated-principal;)

(datatype tenant-access
  Principal : authenticated-principal;
  Tenant : tenant-id;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Principal Tenant IsMember] : tenant-access;)

(datatype resource-id
  X : string;
  ==============
  X : resource-id;)

(datatype resource-access
  Access : tenant-access;
  Resource : resource-id;
  IsOwned : boolean;
  (= IsOwned true) : verified;
  ================================
  [Access Resource IsOwned] : resource-access;)`

	conclusions := collectConclusions(spec)
	for _, want := range []string{"tenant-id", "authenticated-principal", "tenant-access", "resource-id", "resource-access"} {
		if !containsString(conclusions, want) {
			t.Errorf("collectConclusions must surface %q conclusion; got %v", want, conclusions)
		}
	}
	// The robust parser returns only real conclusions — premise type annotations
	// like "string"/"boolean" must NOT be reported (the old crude parser did).
	for _, notWant := range []string{"string", "boolean"} {
		if containsString(conclusions, notWant) {
			t.Errorf("collectConclusions must not surface premise type %q as a conclusion; got %v", notWant, conclusions)
		}
	}
	// Exactly one conclusion per datatype (5 datatypes).
	if len(conclusions) != 5 {
		t.Errorf("expected 5 conclusions (one per datatype), got %d: %v", len(conclusions), conclusions)
	}

	// Datatype names and conclusion linkage.
	rm := buildRuleMap(parseDatatypes(spec))
	if ri, ok := rm["tenant-access"]; !ok || ri.DtName != "tenant-access" {
		t.Errorf("tenant-access rule must map to datatype tenant-access; got %+v ok=%v", rm["tenant-access"], ok)
	}
}

func TestParseDatatypes_Basic(t *testing.T) {
	spec := `(datatype foo-bar
  X : string;
  ==========
  X : foo-bar;)`
	dts := parseDatatypes(spec)
	if len(dts) != 1 {
		t.Fatalf("expected 1 datatype, got %d: %+v", len(dts), dts)
	}
	if dts[0].Name != "foo-bar" {
		t.Errorf("Name = %q, want foo-bar", dts[0].Name)
	}
	if len(dts[0].Rules) != 1 || dts[0].Rules[0].Conc.TypeName != "foo-bar" {
		t.Errorf("expected single rule concluding foo-bar; got %+v", dts[0].Rules)
	}
}

func TestParseDatatypes_CountsBlocks(t *testing.T) {
	spec := `(datatype a
  X : string;
  =====
  X : a;)
(datatype b
  Y : number;
  =====
  Y : b;)`
	dts := parseDatatypes(spec)
	if len(dts) != 2 {
		t.Fatalf("expected 2 datatypes, got %d: %+v", len(dts), dts)
	}
}

func TestParseTargets_Table(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil}, // after split/trim all dropped -> empty make but no appends? wait impl returns the made slice which has len0
		{"single", []string{"single"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" x-access , y-permit ", []string{"x-access", "y-permit"}},
	}
	for _, tt := range tests {
		got := parseTargets(tt.in)
		if len(got) != len(tt.want) {
			// special case: impl for all-whitespace uses make(0, n) so non-nil len0; our "nil" want
			if len(tt.want) == 0 && len(got) == 0 {
				continue
			}
			t.Errorf("parseTargets(%q) len=%d want len=%d (got=%v want=%v)", tt.in, len(got), len(tt.want), got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseTargets(%q)[%d] = %q want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

// ============================================================================
// Helpers (mirrors style of cmd/shengen/main_test.go test helpers)
// ============================================================================

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
