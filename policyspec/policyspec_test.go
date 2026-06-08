package policyspec

import "testing"

const chainSpec = `(datatype jwt-token
  X : string;
  (not (= X "")) : verified;
  ============================
  X : jwt-token;)

(datatype tenant-access
  Token : jwt-token;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Token IsMember] : tenant-access;)

(datatype resource-access
  Access : tenant-access;
  IsOwned : boolean;
  (= IsOwned true) : verified;
  ================================
  [Access IsOwned] : resource-access;)`

func TestCollectConclusions_OnlyRealConclusions(t *testing.T) {
	got := CollectConclusions(chainSpec)
	want := map[string]bool{"jwt-token": true, "tenant-access": true, "resource-access": true}
	if len(got) != 3 {
		t.Fatalf("expected 3 conclusions, got %v", got)
	}
	for _, c := range got {
		if !want[c] {
			t.Errorf("unexpected conclusion %q (premise types must not surface); got %v", c, got)
		}
	}
}

func TestCollectTransitiveVerified_FollowsChain(t *testing.T) {
	rm := BuildRuleMap(ParseDatatypes(chainSpec))
	vs := CollectTransitiveVerified("resource-access", rm, map[string]bool{})
	var member, owned, token bool
	for _, v := range vs {
		switch {
		case contains(v.Raw, "IsMember"):
			member = true
		case contains(v.Raw, "IsOwned"):
			owned = true
		case contains(v.Raw, "not"):
			token = true
		}
	}
	if !member || !owned || !token {
		t.Errorf("resource-access must inherit token non-emptiness + membership + ownership; got %+v", vs)
	}
}

func TestEvalVerified_DecidableFragment(t *testing.T) {
	rm := BuildRuleMap(ParseDatatypes(chainSpec))
	prem := CollectTransitiveVerified("resource-access", rm, map[string]bool{})

	cases := []struct {
		name string
		env  map[string]any
		want bool
	}{
		{"all hold", map[string]any{"x": "tok", "isMember": true, "isOwned": true}, true},
		{"empty token", map[string]any{"x": "", "isMember": true, "isOwned": true}, false},
		{"not member", map[string]any{"x": "tok", "isMember": false, "isOwned": true}, false},
		{"not owned", map[string]any{"x": "tok", "isMember": true, "isOwned": false}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := EvalVerified(prem, c.env)
			if !ok {
				t.Fatalf("EvalVerified ok=false for env %v (unbound var or bad form)", c.env)
			}
			if got != c.want {
				t.Errorf("EvalVerified = %v, want %v (env %v)", got, c.want, c.env)
			}
		})
	}
}

func TestEvalVerified_ElementMembership(t *testing.T) {
	prem := []VerifiedPremise{{Raw: "(element? Role Roles)"}}
	in := map[string]any{"role": "admin", "roles": []any{"user", "admin"}}
	if got, ok := EvalVerified(prem, in); !ok || !got {
		t.Errorf("expected member; got %v ok=%v", got, ok)
	}
	out := map[string]any{"role": "root", "roles": []any{"user", "admin"}}
	if got, ok := EvalVerified(prem, out); !ok || got {
		t.Errorf("expected non-member; got %v ok=%v", got, ok)
	}
}

func TestEvalVerified_UnboundVarIsIndeterminate(t *testing.T) {
	prem := []VerifiedPremise{{Raw: "(= IsMember true)"}}
	if _, ok := EvalVerified(prem, map[string]any{}); ok {
		t.Error("EvalVerified must report ok=false when a variable is unbound")
	}
}

const sumTypeSpec = `(datatype jwt-token
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

func TestCollectClauses_SumTypeExpandsToDisjunction(t *testing.T) {
	variants := BuildRuleVariants(ParseDatatypes(sumTypeSpec))
	clauses := CollectClauses("tenant-access", variants, map[string]bool{})
	if len(clauses) != 2 {
		t.Fatalf("authenticated-principal sum type must yield 2 clauses (human ∨ service); got %d: %+v", len(clauses), clauses)
	}
	// Each clause carries isMember; one carries the jwt branch, the other the secret branch.
	var human, service bool
	for _, c := range clauses {
		var hasMember, hasX, hasSecret bool
		for _, p := range c {
			hasMember = hasMember || contains(p.Raw, "IsMember")
			hasX = hasX || contains(p.Raw, "X")
			hasSecret = hasSecret || contains(p.Raw, "Secret")
		}
		if !hasMember {
			t.Errorf("every clause must carry isMember; clause %+v", c)
		}
		human = human || hasX
		service = service || hasSecret
	}
	if !human || !service {
		t.Errorf("expected one human (jwt) and one service (secret) clause; got %+v", clauses)
	}
}

func TestEvalClauses_EitherVariantSatisfies(t *testing.T) {
	variants := BuildRuleVariants(ParseDatatypes(sumTypeSpec))
	clauses := CollectClauses("tenant-access", variants, map[string]bool{})

	cases := []struct {
		name string
		env  map[string]any
		want bool
	}{
		{"human ok", map[string]any{"isMember": true, "x": "tok", "secret": ""}, true},
		{"service ok", map[string]any{"isMember": true, "x": "", "secret": "s"}, true},
		{"neither principal", map[string]any{"isMember": true, "x": "", "secret": ""}, false},
		{"not member", map[string]any{"isMember": false, "x": "tok", "secret": "s"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := EvalClauses(clauses, c.env)
			if !ok {
				t.Fatalf("EvalClauses ok=false for %v", c.env)
			}
			if got != c.want {
				t.Errorf("EvalClauses = %v, want %v (env %v)", got, c.want, c.env)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
