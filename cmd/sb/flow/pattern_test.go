package flow

import "testing"

func TestCanonical(t *testing.T) {
	cases := []struct{ symbol, want string }{
		{
			"scip-go gomod multi-tenant-api 6b9dde09b3a9 `multi-tenant-api/internal/verified`/CheckTenantAccess().",
			"multi-tenant-api/internal/verified/CheckTenantAccess",
		},
		{
			"scip-go gomod multi-tenant-api 6b9dde09b3a9 `multi-tenant-api/internal/handlers`/Server#handleListResources().",
			"multi-tenant-api/internal/handlers/Server#handleListResources",
		},
		{
			"scip-go gomod github.com/golang/go/src go1.25.6 `database/sql`/DB#Query().",
			"database/sql/DB#Query",
		},
		{
			"scip-typescript npm shen-web-tools 0.0.0 `src/guards.ts`/newTenantAccess().",
			"src/guards.ts/newTenantAccess",
		},
		// A package name containing a space stays one field.
		{
			"scip-go gomod `some pkg` v1 `some pkg/thing`/F().",
			"some pkg/thing/F",
		},
		// Non-symbols pass through.
		{"local 12", "local 12"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Canonical(c.symbol); got != c.want {
			t.Errorf("Canonical(%q) = %q, want %q", c.symbol, got, c.want)
		}
	}
}

func TestPatternMatches(t *testing.T) {
	const ctor = "scip-go gomod multi-tenant-api abc `multi-tenant-api/internal/shenguard`/NewTenantAccess()."
	const query = "scip-go gomod github.com/golang/go/src go1.25.6 `database/sql`/DB#Query()."
	const queryRow = "scip-go gomod github.com/golang/go/src go1.25.6 `database/sql`/DB#QueryRow()."
	const handler = "scip-go gomod multi-tenant-api abc `multi-tenant-api/internal/handlers`/Server#handleListResources()."

	cases := []struct {
		pattern string
		symbol  string
		want    bool
	}{
		// Suffix match at a segment boundary: the module path and the
		// indexer's version hash never appear in a pattern.
		{"internal/shenguard/NewTenantAccess", ctor, true},
		{"NewTenantAccess", ctor, true},
		{"shenguard/NewTenantAccess", ctor, true},
		// Not a segment boundary, so no match.
		{"enantAccess", ctor, false},
		{"internal/shenguard/NewTenantAccessX", ctor, false},
		// `#` is a boundary too.
		{"DB#Query", query, true},
		{"Query", query, true},
		{"DB#Query", queryRow, false},
		{"DB#Query*", queryRow, true},
		// Globs.
		{"internal/handlers/Server#handle*", handler, true},
		{"*ListResources*", handler, true},
		{"*ListResources", handler, true}, // a trailing glob is not implied
		{"*ListResource", handler, false}, // …so this does not match

		{"*", handler, true},
	}
	for _, c := range cases {
		if got := MustPattern(c.pattern).Matches(c.symbol); got != c.want {
			t.Errorf("MustPattern(%q).Matches(%q) = %v, want %v", c.pattern, Canonical(c.symbol), got, c.want)
		}
	}
}

func TestIsCallable(t *testing.T) {
	if !IsCallable("scip-go gomod m v `m/p`/F().") {
		t.Error("function symbol not reported callable")
	}
	if IsCallable("scip-go gomod m v `m/p`/T#") {
		t.Error("type symbol reported callable")
	}
	if IsCallable("scip-go gomod m v `m/p`/T#field.") {
		t.Error("field symbol reported callable")
	}
}

func TestMatchesAny(t *testing.T) {
	pats := []Pattern{MustPattern("a/B"), MustPattern("c/D")}
	if !MatchesAny(pats, "s m p v `x/c`/D().") {
		t.Error("expected a match on the second pattern")
	}
	if MatchesAny(pats, "s m p v `x/y`/Z().") {
		t.Error("unexpected match")
	}
	if MatchesAny(nil, "s m p v `x/y`/Z().") {
		t.Error("empty pattern list must not match")
	}
}
