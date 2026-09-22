package flow

import (
	"strings"
	"testing"
)

// TestParseSpecBareAndCommented pins the rule that sb reads a
// (flow …) form whether it stands bare at the top level or sits
// inside a Shen `\* … *\` comment. The examples keep the forms inside
// a comment so `shen tc+` (gate 4) does not meet an undefined `flow`
// symbol; a project whose Shen host knows the form can write it bare.
func TestParseSpecBareAndCommented(t *testing.T) {
	bare := `
(datatype amount
  X : number;
  ==============
  X : amount;)

(flow tenant-access-discipline
  (constructor-only internal/shenguard/NewTenantAccess
                    internal/verified/CheckTenantAccess)
  (must-pass-through *ListResources* internal/verified/CheckTenantAccess DB#Query*))
`
	commented := "\\* preamble\n" + bare + "\n*\\\n"

	for name, src := range map[string]string{"bare": bare, "commented": commented} {
		decls, err := ParseSpec("core.shen", src)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(decls) != 1 {
			t.Fatalf("%s: got %d decls, want 1", name, len(decls))
		}
		d := decls[0]
		if d.Name != "tenant-access-discipline" {
			t.Errorf("%s: name = %q", name, d.Name)
		}
		if d.SpecFile != "core.shen" {
			t.Errorf("%s: spec file = %q", name, d.SpecFile)
		}
		if len(d.Premises) != 2 {
			t.Fatalf("%s: got %d premises, want 2", name, len(d.Premises))
		}
		co, ok := d.Premises[0].(ConstructorOnly)
		if !ok {
			t.Fatalf("%s: first premise is %T, want ConstructorOnly", name, d.Premises[0])
		}
		if len(co.Allowed) != 1 {
			t.Errorf("%s: got %d allowed callers, want 1", name, len(co.Allowed))
		}
		if co.ID() != "constructor-only:internal/shenguard/NewTenantAccess" {
			t.Errorf("%s: premise id = %q", name, co.ID())
		}
		mpt, ok := d.Premises[1].(MustPassThrough)
		if !ok {
			t.Fatalf("%s: second premise is %T, want MustPassThrough", name, d.Premises[1])
		}
		if mpt.Sink.String() != "DB#Query*" {
			t.Errorf("%s: sink = %q", name, mpt.Sink.String())
		}
		if !strings.Contains(d.Raw, "must-pass-through") {
			t.Errorf("%s: raw excerpt lost a clause: %q", name, d.Raw)
		}
	}
}

func TestParseSpecMultipleForms(t *testing.T) {
	src := `
(flow one (constructor-only a/B a/C))
(flow two (constructor-only d/E d/F))
`
	decls, err := ParseSpec("core.shen", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 2 || decls[0].Name != "one" || decls[1].Name != "two" {
		t.Fatalf("got %+v", decls)
	}
}

func TestParseSpecNoFlowForms(t *testing.T) {
	decls, err := ParseSpec("core.shen", "(datatype amount X : number; == X : amount;)")
	if err != nil {
		t.Fatal(err)
	}
	if len(decls) != 0 {
		t.Fatalf("got %d decls, want 0", len(decls))
	}
}

func TestParseSpecErrors(t *testing.T) {
	cases := map[string]string{
		"unknown clause":        `(flow f (no-such-clause a b))`,
		"empty form":            `(flow f)`,
		"short constructor":     `(flow f (constructor-only a))`,
		"wrong arity for paths": `(flow f (must-pass-through a b))`,
	}
	for name, src := range cases {
		if _, err := ParseSpec("core.shen", src); err == nil {
			t.Errorf("%s: expected an error for %s", name, src)
		}
	}
}
