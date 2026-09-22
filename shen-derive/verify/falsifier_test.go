package verify

// falsifier_test.go — the fourth sample source, end to end over a
// fixture spec: decoding, provenance, the re-evaluation that turns a
// model's *claim* into a committed sample, and the failure modes that
// have to warn rather than explode.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// falsifierConfig reuses the path-cover fixture spec: a numeric
// wrapper (`amount`), a string wrapper (`account-id`) and a composite
// (`transaction`), which between them exercise every decoding branch.
func falsifierConfig(t *testing.T, samplesPath string) *HarnessConfig {
	t.Helper()
	tmp := t_tempFile(pathCoverSpec)
	t.Cleanup(func() { t_removeFile(tmp) })

	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	def := sf.FindDefine("processable")
	if def == nil {
		t.Fatal("processable not found")
	}
	all := make([]*specfile.Define, len(sf.Defines))
	for i := range sf.Defines {
		all[i] = &sf.Defines[i]
	}
	return &HarnessConfig{
		Spec:             def,
		TypeTable:        specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard"),
		AllDefines:       all,
		ImplPkgPath:      "example.com/derived",
		ImplPkgName:      "derived",
		ImplFunc:         "Processable",
		TestPkgName:      "derived_test",
		FalsifierSamples: samplesPath,
	}
}

func writeSamples(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "falsifier-samples.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFalsifierSourceOffIsUnchanged(t *testing.T) {
	// The default must be byte-identical to a build with no falsifier
	// at all: a project that never runs `sb loop --falsify` sees no
	// change in its committed test file.
	h, err := BuildHarness(falsifierConfig(t, ""))
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if h.FalsifierSamplesAdded != 0 {
		t.Errorf("FalsifierSamplesAdded = %d, want 0", h.FalsifierSamplesAdded)
	}
	src, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if strings.Contains(src, "Falsifier") || strings.Contains(src, "falsify:") {
		t.Error("the generated file must not mention the falsifier when the source is off")
	}
}

func TestFalsifierMissingFileIsNotAnError(t *testing.T) {
	// Most falsifier runs find nothing, so the absence of the file is
	// the normal case, not a failure.
	cfg := falsifierConfig(t, filepath.Join(t.TempDir(), "absent.json"))
	h, err := BuildHarness(cfg)
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if h.FalsifierSamplesAdded != 0 {
		t.Errorf("FalsifierSamplesAdded = %d, want 0", h.FalsifierSamplesAdded)
	}
}

func TestFalsifierAddsFourthSampleSource(t *testing.T) {
	p := writeSamples(t, `{
	  "schema_version": 1,
	  "samples": [
	    {
	      "spec": "processable",
	      "note": "kills cmp-flip:internal/derived/processable.go:33:14",
	      "args": [5, [[2, "alice", "bob"], [3, "bob", "carol"]]]
	    },
	    {
	      "spec": "some-other-define",
	      "args": [0, []]
	    }
	  ]
	}`)
	poolOnly, err := BuildHarness(falsifierConfig(t, ""))
	if err != nil {
		t.Fatalf("BuildHarness (pool): %v", err)
	}
	h, err := BuildHarness(falsifierConfig(t, p))
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}

	// Only the entry naming this spec is taken.
	if h.FalsifierSamplesAdded != 1 {
		t.Fatalf("FalsifierSamplesAdded = %d, want 1 (the other entry names a different spec)", h.FalsifierSamplesAdded)
	}
	if len(h.Cases) != len(poolOnly.Cases)+1 {
		t.Fatalf("got %d cases, want %d — the falsifier source appends, it does not replace",
			len(h.Cases), len(poolOnly.Cases)+1)
	}
	if len(h.FalsifierWarnings) != 0 {
		t.Errorf("unexpected warnings: %v", h.FalsifierWarnings)
	}

	last := h.Cases[len(h.Cases)-1]
	if last.Provenance != "falsify:0" {
		t.Errorf("Provenance = %q, want falsify:0", last.Provenance)
	}
	if !strings.Contains(last.Note, "cmp-flip") {
		t.Errorf("Note = %q, want the falsifier's rationale to survive", last.Note)
	}

	src, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	for _, want := range []string{
		"// Falsifier: 1 sample(s)",
		"// provenance: falsify:0",
		"kills cmp-flip:",
		// Scalars go through the same mustXxx helpers as every other
		// source, so all four sources share one helper set.
		"mustAmount(5)",
		"mustTransaction(mustAmount(2), mustAccountId(\"alice\"), mustAccountId(\"bob\"))",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated file is missing %q:\n%s", want, src)
		}
	}
}

func TestFalsifierAcceptsCompositesAsObjects(t *testing.T) {
	// A model writes this file, and the object form is the one it is
	// likely to reach for. Field names match case-insensitively
	// because the spec writes `Amount` and JSON writes `amount`.
	p := writeSamples(t, `{
	  "schema_version": 1,
	  "samples": [{
	    "spec": "processable",
	    "args": [1, [{"amount": 1, "from": "a", "to": "b"}]]
	  }]
	}`)
	h, err := BuildHarness(falsifierConfig(t, p))
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if h.FalsifierSamplesAdded != 1 {
		t.Fatalf("FalsifierSamplesAdded = %d, want 1; warnings: %v", h.FalsifierSamplesAdded, h.FalsifierWarnings)
	}
	src, _ := h.Emit()
	if !strings.Contains(src, `mustTransaction(mustAmount(1), mustAccountId("a"), mustAccountId("b"))`) {
		t.Errorf("object form did not decode into field order:\n%s", src)
	}
}

func TestFalsifierSampleIsReEvaluatedAgainstTheSpec(t *testing.T) {
	// The file carries an *input*, never an expected output: what the
	// falsifier proposes is a claim, and the spec's own evaluator is
	// what turns it into evidence. Here the running balance goes
	// negative, so the spec says false regardless of what any model
	// believed.
	p := writeSamples(t, `{
	  "schema_version": 1,
	  "samples": [{"spec": "processable", "args": [1, [[5, "a", "b"]]]}]
	}`)
	h, err := BuildHarness(falsifierConfig(t, p))
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	last := h.Cases[len(h.Cases)-1]
	if last.ExpectedGo != "false" {
		t.Errorf("want = %q, want false — 1 minus 5 is negative, and the spec decides that, not the file",
			last.ExpectedGo)
	}
}

func TestFalsifierMalformedEntryWarnsRatherThanFails(t *testing.T) {
	// One bad entry from a model must not take the whole gate down,
	// and the operator has to be told which entry it was.
	p := writeSamples(t, `{
	  "schema_version": 1,
	  "samples": [
	    {"spec": "processable", "args": [0]},
	    {"spec": "processable", "args": ["not-a-number", []]},
	    {"spec": "processable", "args": [2, []]}
	  ]
	}`)
	h, err := BuildHarness(falsifierConfig(t, p))
	if err != nil {
		t.Fatalf("BuildHarness must not fail on a malformed entry: %v", err)
	}
	if h.FalsifierSamplesAdded != 1 {
		t.Errorf("FalsifierSamplesAdded = %d, want 1 good entry out of 3", h.FalsifierSamplesAdded)
	}
	if len(h.FalsifierWarnings) != 2 {
		t.Fatalf("want 2 warnings, got %d: %v", len(h.FalsifierWarnings), h.FalsifierWarnings)
	}
	joined := strings.Join(h.FalsifierWarnings, "\n")
	if !strings.Contains(joined, "sample 0") || !strings.Contains(joined, "sample 1") {
		t.Errorf("warnings must name the offending entries:\n%s", joined)
	}
}

func TestFalsifierRejectsNewerSchema(t *testing.T) {
	p := writeSamples(t, `{"schema_version": 99, "samples": []}`)
	_, _, err := LoadFalsifierSamples(p, "processable", []string{"amount"}, nil)
	if err == nil {
		t.Fatal("a newer schema must be an error: reading it partially would make the gate quietly weaker")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("error = %v", err)
	}
}

func TestFalsifierDefaultPath(t *testing.T) {
	if DefaultFalsifierSamplesPath != ".sb/falsifier-samples.json" {
		t.Errorf("DefaultFalsifierSamplesPath = %q", DefaultFalsifierSamplesPath)
	}
}
