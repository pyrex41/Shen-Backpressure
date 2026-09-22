package specfile

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseIgnoresFlowForms checks that a spec carrying (flow ...)
// premises loads unchanged.
//
// Flow premises are consumed by `sb flow` alone: it evaluates them
// over a resolved symbol graph (see docs/FLOW.md). shen-derive has
// nothing to do with them and must neither fail on the form nor
// mistake it for a datatype or a define. Both spellings are covered:
// bare at the top level, and inside a Shen `\* … *\` comment — which
// is how the examples write them, so that `shen tc+` does not meet an
// undefined `flow` symbol.
func TestParseIgnoresFlowForms(t *testing.T) {
	const flowForms = `(flow tenant-access-discipline
  (constructor-only internal/shenguard/NewTenantAccess
                    internal/verified/CheckTenantAccess)
  (must-pass-through *ListResources* internal/verified/CheckTenantAccess DB#Query*))

(flow resource-access-discipline
  (constructor-only internal/shenguard/NewResourceAccess
                    internal/verified/CheckResourceAccess))`

	const baseline = `(datatype user-id
  X : string;
  ==============
  X : user-id;)

(define same-user?
  {user-id --> user-id --> boolean}
  A B -> (= A B))`

	want := parseSpecString(t, baseline)

	variants := map[string]string{
		"bare":      baseline + "\n\n" + flowForms + "\n",
		"commented": baseline + "\n\n\\* flow premises\n" + flowForms + "\n*\\\n",
	}
	for name, src := range variants {
		got := parseSpecString(t, src)
		if len(got.Datatypes) != len(want.Datatypes) {
			t.Errorf("%s: got %d datatypes, want %d", name, len(got.Datatypes), len(want.Datatypes))
		}
		if len(got.Defines) != len(want.Defines) {
			t.Errorf("%s: got %d defines, want %d", name, len(got.Defines), len(want.Defines))
		}
		if d := got.FindDatatype("user-id"); d == nil {
			t.Errorf("%s: datatype user-id lost", name)
		}
		def := got.FindDefine("same-user?")
		if def == nil {
			t.Fatalf("%s: define same-user? lost", name)
		}
		if def.Arity() != 2 {
			t.Errorf("%s: same-user? arity = %d, want 2", name, def.Arity())
		}
		// A (flow ...) form must not leak in as a define or datatype
		// under its own name.
		if got.FindDefine("tenant-access-discipline") != nil || got.FindDatatype("tenant-access-discipline") != nil {
			t.Errorf("%s: a flow form was parsed as a spec rule", name)
		}
	}
}

// TestFlowFormsDoNotDisturbDocAnnotations guards the one real
// interaction risk: the `:doc` annotation scanner attaches a comment
// to the *next* datatype or define, and a flow form sitting between
// the two must not capture it.
func TestFlowFormsDoNotDisturbDocAnnotations(t *testing.T) {
	const datatype = `(datatype user-id
  X : string;
  ==============
  X : user-id;)`
	const doc = "\\* :doc \"A user identifier.\" *\\\n"

	variants := map[string]string{
		"bare flow form between":      doc + "\n(flow d (constructor-only a/B a/C))\n\n" + datatype,
		"commented flow form between": doc + "\n\\* (flow d (constructor-only a/B a/C)) *\\\n\n" + datatype,
		"no flow form":                doc + "\n" + datatype,
	}
	for name, src := range variants {
		sf := parseSpecString(t, src)
		d := sf.FindDatatype("user-id")
		if d == nil {
			t.Fatalf("%s: datatype user-id lost", name)
		}
		if d.Doc != "A user identifier." {
			t.Errorf("%s: doc = %q, want the annotation to reach the datatype past the flow form", name, d.Doc)
		}
	}
}

func parseSpecString(t *testing.T, src string) *SpecFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.shen")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	sf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return sf
}
