package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mutateFixture = `\* comment with (datatype fake X : verified;) inside *\
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype transfer
  From : string;
  To : string;
  Amt : amount;
  (not (= From To)) : verified;
  (element? From
     ["a" "b"]) : verified;
  ==============================
  [From To Amt] : transfer;)
`

func TestFindVerifiedPremises(t *testing.T) {
	sites := findVerifiedPremises(mutateFixture)
	if len(sites) != 3 {
		t.Fatalf("want 3 premises (comment ignored), got %d: %+v", len(sites), sites)
	}
	want := []struct {
		dt, expr string
		line     int
	}{
		{"amount", "(>= X 0)", 4},
		{"transfer", "(not (= From To))", 12},
		{"transfer", `(element? From ["a" "b"])`, 13},
	}
	for i, w := range want {
		s := sites[i]
		if s.Datatype != w.dt || s.Expr != w.expr || s.Line != w.line {
			t.Errorf("site %d = %s %q line %d; want %s %q line %d", i, s.Datatype, s.Expr, s.Line, w.dt, w.expr, w.line)
		}
	}
	// Blanking one premise removes exactly that premise and keeps offsets.
	m := blankSpan(mutateFixture, sites[1].start, sites[1].end)
	if len(m) != len(mutateFixture) || strings.Contains(m, "(not (= From To))") || !strings.Contains(m, "(>= X 0) : verified;") {
		t.Fatalf("bad mutant:\n%s", m)
	}
	if got := findVerifiedPremises(m); len(got) != 2 {
		t.Fatalf("mutant should have 2 premises, got %d", len(got))
	}
}

func TestResolveShenDefaults(t *testing.T) {
	bin, argv, iso, err := resolveShen(MutateConfig{Shen: "/opt/shen-erl/bin/shen-erl"})
	if err != nil || bin != "/opt/shen-erl/bin/shen-erl" || strings.Join(argv, " ") != "script {file}" || iso != "mutant" {
		t.Fatalf("shen-erl defaults: %s %v %s %v", bin, argv, iso, err)
	}
	_, argv, iso, _ = resolveShen(MutateConfig{Shen: "shen-sbcl"})
	if strings.Join(argv, " ") != "-l {file}" || iso != "hostile" {
		t.Fatalf("shen-sbcl defaults: %v %s", argv, iso)
	}
	_, argv, iso, _ = resolveShen(MutateConfig{Shen: "node", Args: []string{"shen.js", "{file}"}, Isolate: "hostile"})
	if strings.Join(argv, " ") != "shen.js {file}" || iso != "hostile" {
		t.Fatalf("explicit args: %v %s", argv, iso)
	}
	if _, _, _, err := resolveShen(MutateConfig{Shen: "x", Isolate: "bogus"}); err == nil {
		t.Fatal("bogus isolate accepted")
	}
}

// TestMutateSpecWithFakeShen drives the full gate with a stub "Shen" that
// accepts a hostile file iff the premise it targets is missing from the
// mutant spec. It checks the baseline, killed and survived paths without
// needing a real Shen runtime.
func TestMutateSpecWithFakeShen(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "core.shen")
	os.WriteFile(spec, []byte(mutateFixture), 0o644)
	h1 := filepath.Join(dir, "h1.shen")
	h2 := filepath.Join(dir, "h2.shen")
	good := filepath.Join(dir, "good.shen")
	os.WriteFile(h1, []byte("needs: (>= X 0)"), 0o644)
	os.WriteFile(h2, []byte("needs: (not (= From To))"), 0o644)
	os.WriteFile(good, []byte("good"), 0o644)
	// The stub reads the driver, finds the spec path and each loaded file,
	// and prints SB-MUTATE-RESULT lines: a hostile is accepted when the
	// premise named after "needs: " is absent from the spec.
	stub := filepath.Join(dir, "fake-shen")
	script := `#!/bin/sh
driver="$2"
spec=$(sed -n 's/^(load "\(.*spec.shen\)")$/\1/p' "$driver")
echo "SB-MUTATE-SPEC-LOADED"
grep -o 'SB-MUTATE-RESULT [0-9]* ~A~%" (trap-error (do (load "[^"]*")' "$driver" | while read -r line; do
  i=$(echo "$line" | awk '{print $2}')
  f=$(echo "$line" | sed 's/.*(load "\([^"]*\)").*/\1/')
  need=$(sed -n 's/^needs: //p' "$f")
  if [ -z "$need" ]; then echo "SB-MUTATE-RESULT $i accepted"; continue; fi
  if grep -qF "$need" "$spec"; then echo "SB-MUTATE-RESULT $i rejected"; else echo "SB-MUTATE-RESULT $i accepted"; fi
done
`
	os.WriteFile(stub, []byte(script), 0o755)
	mc := MutateConfig{Shen: stub, Hostile: []string{h1, h2}, Good: []string{good}, Jobs: 2}
	rep, err := runMutateSpec(spec, mc, os.Stderr)
	if err == nil {
		t.Fatal("expected failure: the element? premise has no hostile witness")
	}
	if rep.Killed != 2 || rep.Survived != 1 {
		t.Fatalf("killed=%d survived=%d, want 2/1 (%+v)", rep.Killed, rep.Survived, rep.Premises)
	}
	if !strings.Contains(err.Error(), `(element? From ["a" "b"])`) {
		t.Fatalf("error should name the surviving premise: %v", err)
	}
	if rep.Premises[0].Status != "killed" || rep.Premises[0].KilledBy[0] != relPath(h1) {
		t.Fatalf("premise 0: %+v", rep.Premises[0])
	}

	// Baseline violation: a "hostile" that the full spec accepts.
	h3 := filepath.Join(dir, "h3.shen")
	os.WriteFile(h3, []byte("needs: (missing premise)"), 0o644)
	mc.Hostile = []string{h1, h3}
	if _, err := runMutateSpec(spec, mc, os.Stderr); err == nil || !strings.Contains(err.Error(), "baseline failed") {
		t.Fatalf("expected baseline failure, got %v", err)
	}
}

func TestAnnotateDischargeWithMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	r := &DischargeReport{SchemaVersion: 1, Rules: []DischargeRule{{
		Name: "amount",
		Premises: []DischargePremise{
			{ID: "amount.field-x", Expression: "X : number"},
			{ID: "amount.verified-x-0", Expression: "(>= X  0) : verified"},
		},
	}}}
	if err := writeDischarge(path, r); err != nil {
		t.Fatal(err)
	}
	rep := &mutationReport{ShenRuntime: "/x/shen-erl", GeneratedAt: "now", Premises: []mutationResult{
		{Datatype: "amount", Premise: "(>= X 0)", Status: "killed", KilledBy: []string{"h.shen"}},
	}}
	n, err := annotateDischargeWithMutation(path, rep)
	if err != nil || n != 1 {
		t.Fatalf("annotated %d, err %v", n, err)
	}
	got, _ := loadDischarge(path)
	m := got.Rules[0].Premises[1].Mutation
	if m == nil || m.Status != "killed" || m.ShenRuntime != "shen-erl" || got.Rules[0].Premises[0].Mutation != nil {
		t.Fatalf("bad annotation: %+v", got.Rules[0].Premises)
	}
	// Reports without mutation evidence stay byte-identical (omitempty).
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), `"mutation"`) != 1 {
		t.Fatalf("mutation field should appear exactly once:\n%s", data)
	}
}
