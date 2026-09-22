package main

// falsify_test.go — the falsify phase's plumbing: prompt hydration,
// corpus ingestion, and sample counting.
//
// The phase itself calls a model, which these tests do not. What they
// cover is everything around that call — which is where a silent
// failure is expensive, because the phase runs unattended after a
// green build and nobody is watching its output.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packageDir is the working directory as it was when the test binary
// started. It is captured here rather than read back with os.Getwd
// inside each test, because another test in this package may have
// chdir'd into a directory that has since been removed, and Getwd
// then fails on a cwd that no longer exists.
var packageDir = func() string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	return d
}()

// inTempProject runs fn with the working directory set to a fresh
// temp project, restoring it afterwards.
func inTempProject(t *testing.T, fn func(dir string)) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(packageDir) })
	fn(dir)
}

const falsifyTemplate = `SPEC {{.SpecPath}}
{{.Spec}}
GUARDS {{.GuardsPath}}
{{.Guards}}
DIR {{.ForgeryDir}}
SAMPLES {{.SamplesPath}}
CORPUS
{{.Corpus}}
SURVIVORS
{{.Survivors}}
`

func TestBuildFalsifierPromptHydratesEverySlot(t *testing.T) {
	inTempProject(t, func(dir string) {
		os.MkdirAll("forgeries", 0o755)
		os.WriteFile("core.shen", []byte("(datatype amount)"), 0o644)
		os.WriteFile("guards.go", []byte("package shenguard // generated"), 0o644)
		os.WriteFile(filepath.Join("forgeries", "01_a.go.bak"), []byte(
			"// sb-forgery: expect compile-error\n//\n// Forgery #1: take the zero value of a proof.\npackage demo\n"), 0o644)

		cfg := &Config{Spec: "core.shen", Output: "guards.go", Forgery: ForgeryConfig{Dir: "forgeries"}}
		score := &MutationScore{
			Score: 0.5, Caught: 1, Survived: 1, Total: 2,
			Survivors: []Mutant{{ID: "cmp-flip:x.go:7:3", File: "x.go", Line: 7, Col: 3, Before: "<", After: "<="}},
		}
		got, err := BuildFalsifierPrompt(falsifyTemplate, cfg, score)
		if err != nil {
			t.Fatalf("BuildFalsifierPrompt: %v", err)
		}
		for _, want := range []string{
			"SPEC core.shen",
			"(datatype amount)",
			"GUARDS guards.go",
			"package shenguard // generated",
			"DIR forgeries",
			"SAMPLES " + FalsifierSamplesPath,
			"`01_a.go.bak`",
			"`compile-error`",
			"take the zero value of a proof",
			"cmp-flip:x.go:7:3",
			"1 survivor(s)",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("hydrated prompt is missing %q:\n%s", want, got)
			}
		}
		// Nothing may survive un-substituted: a literal {{ in the
		// prompt would reach the model as noise.
		if strings.Contains(got, "{{") {
			t.Errorf("prompt still carries an un-substituted action:\n%s", got)
		}
	})
}

func TestBuildFalsifierPromptNamesMissingFilesRatherThanGoingSilent(t *testing.T) {
	inTempProject(t, func(dir string) {
		cfg := &Config{Spec: "absent.shen", Output: "absent.go"}
		got, err := BuildFalsifierPrompt(falsifyTemplate, cfg, nil)
		if err != nil {
			t.Fatalf("BuildFalsifierPrompt: %v", err)
		}
		// An empty slot reads to a model as "there is nothing here",
		// which is a different and wrong claim from "I could not read
		// it".
		if !strings.Contains(got, "could not read absent.shen") {
			t.Errorf("a missing spec must be named in the slot:\n%s", got)
		}
		if !strings.Contains(got, "no corpus yet") {
			t.Errorf("a missing corpus must be named in the slot:\n%s", got)
		}
		if !strings.Contains(got, "no mutation run available") {
			t.Errorf("a missing mutation run must be named in the slot:\n%s", got)
		}
	})
}

func TestRenderSurvivorsExplainsAHundredPercent(t *testing.T) {
	// A clean sweep is the most dangerous thing to show a falsifier
	// without comment: it invites "nothing to do here", when what it
	// means is "nothing in this small operator set survived".
	out := renderSurvivorsForPrompt(&MutationScore{Score: 1, Caught: 3, Total: 3})
	if !strings.Contains(out, "None.") {
		t.Errorf("want the zero-survivor wording, got:\n%s", out)
	}
	if !strings.Contains(out, "not a reason to stop") {
		t.Errorf("a 100%% kill rate must not read as 'done':\n%s", out)
	}
}

func TestCollectFalsifierFindingsDiffsTheTree(t *testing.T) {
	inTempProject(t, func(dir string) {
		os.MkdirAll("forgeries", 0o755)
		existing := filepath.Join("forgeries", "01_old.go.bak")
		os.WriteFile(existing, []byte("// sb-forgery: expect compile-error\npackage demo\n"), 0o644)

		before, samplesBefore := snapshotFalsifierInputs("forgeries")
		if len(before) != 1 {
			t.Fatalf("snapshot saw %d file(s), want 1", len(before))
		}
		if samplesBefore != 0 {
			t.Fatalf("samplesBefore = %d, want 0", samplesBefore)
		}

		// Two new files: one well-formed, one with no declaration.
		os.WriteFile(filepath.Join("forgeries", "02_new.go.bak"),
			[]byte("// sb-forgery: expect runtime-panic\n// sb-forgery-entry: Read\npackage demo\n"), 0o644)
		os.WriteFile(filepath.Join("forgeries", "03_undeclared.go.bak"),
			[]byte("// Forgery #3: no declaration at all.\npackage demo\n"), 0o644)
		os.MkdirAll(filepath.Dir(FalsifierSamplesPath), 0o755)
		os.WriteFile(FalsifierSamplesPath,
			[]byte(`{"schema_version":1,"samples":[{"spec":"x","args":[1]},{"spec":"x","args":[2]}]}`), 0o644)

		f := collectFalsifierFindings("forgeries", before, samplesBefore)
		if len(f.NewForgeries) != 1 || !strings.HasSuffix(f.NewForgeries[0], "02_new.go.bak") {
			t.Errorf("NewForgeries = %v, want just the well-formed new file", f.NewForgeries)
		}
		// An undeclared new file is itself a finding: the falsifier
		// wrote something and declared no outcome for it, and that
		// must surface rather than be silently dropped.
		if len(f.CorpusErrors) != 1 {
			t.Errorf("CorpusErrors = %v, want the undeclared file reported", f.CorpusErrors)
		}
		if f.NewSamples != 2 {
			t.Errorf("NewSamples = %d, want 2", f.NewSamples)
		}
		if !f.Any() {
			t.Error("Any() must be true when the phase produced material")
		}
	})
}

func TestCollectFalsifierFindingsEmptyRun(t *testing.T) {
	inTempProject(t, func(dir string) {
		os.MkdirAll("forgeries", 0o755)
		os.WriteFile(filepath.Join("forgeries", "01.go.bak"),
			[]byte("// sb-forgery: expect compile-error\npackage demo\n"), 0o644)
		before, samplesBefore := snapshotFalsifierInputs("forgeries")
		f := collectFalsifierFindings("forgeries", before, samplesBefore)
		if f.Any() {
			t.Errorf("a run that changed nothing must report nothing: %+v", f)
		}
	})
}

func TestCountFalsifierSamples(t *testing.T) {
	inTempProject(t, func(dir string) {
		if got := countFalsifierSamples("nope.json"); got != 0 {
			t.Errorf("missing file counted %d, want 0", got)
		}
		os.WriteFile("bad.json", []byte("not json at all"), 0o644)
		if got := countFalsifierSamples("bad.json"); got != 0 {
			t.Errorf("unparseable file counted %d, want 0 — this is a difference measurement, not a validator", got)
		}
		os.WriteFile("ok.json", []byte(`{"schema_version":1,"samples":[{},{},{}]}`), 0o644)
		if got := countFalsifierSamples("ok.json"); got != 3 {
			t.Errorf("counted %d, want 3", got)
		}
	})
}

func TestEmbeddedFalsifierTemplateParses(t *testing.T) {
	// `--falsify` must work in a project that has never customised the
	// prompt, so the embedded copy has to be present and valid.
	inTempProject(t, func(dir string) {
		src, name, err := loadFalsifierTemplate()
		if err != nil {
			t.Fatalf("loadFalsifierTemplate: %v", err)
		}
		if !strings.Contains(name, "embedded") {
			t.Errorf("with no project copy, the template must come from the bundle; got %q", name)
		}
		got, err := BuildFalsifierPrompt(src, &Config{Spec: "core.shen"}, nil)
		if err != nil {
			t.Fatalf("the embedded template does not hydrate: %v", err)
		}
		for _, want := range []string{
			"grep-miss-flow-catch",
			"succeeds (documented TCB limit)",
			"schema_version",
			FalsifierSamplesPath,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("embedded prompt is missing %q", want)
			}
		}
		// Every expectation the gate accepts must appear in the
		// prompt, or the falsifier cannot declare it.
		for _, e := range forgeryExpectations {
			if !strings.Contains(got, e) {
				t.Errorf("the prompt does not offer the expectation %q, so the falsifier could never declare it", e)
			}
		}
	})
}

func TestProjectFalsifierTemplateWinsOverTheBundle(t *testing.T) {
	inTempProject(t, func(dir string) {
		os.MkdirAll(filepath.Dir(FalsifierPromptPath), 0o755)
		os.WriteFile(FalsifierPromptPath, []byte("local template for {{.SpecPath}}"), 0o644)
		src, name, err := loadFalsifierTemplate()
		if err != nil {
			t.Fatal(err)
		}
		if name != FalsifierPromptPath {
			t.Errorf("template came from %q, want the project's own copy", name)
		}
		if !strings.Contains(src, "local template") {
			t.Errorf("src = %q", src)
		}
	})
}

func TestFalsifierSamplesPathMatchesShenDerive(t *testing.T) {
	// cmd/sb and shen-derive are separate modules and cannot import
	// each other, so the path is duplicated. This pins the duplicate.
	if FalsifierSamplesPath != ".sb/falsifier-samples.json" {
		t.Errorf("FalsifierSamplesPath = %q; shen-derive's verify.DefaultFalsifierSamplesPath is .sb/falsifier-samples.json",
			FalsifierSamplesPath)
	}
}
