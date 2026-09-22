package main

// forgery_test.go — the forgery gate's header parser, its classifier,
// and the driver it generates, exercised against small fixtures.
//
// The end-to-end checks (a file that really fails to compile, a
// forgery `sb flow` really catches) run against the examples; these
// tests cover the parts that decide *what* runs, which is where a
// silent mistake would be expensive: an expectation typo that skipped
// a corpus entry, or a driver that reported "ok" for a program that
// panicked.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeForgery drops a fixture into a temp corpus directory.
func writeForgery(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name+".go.bak")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseForgeryHeader(t *testing.T) {
	dir := t.TempDir()

	t.Run("full header", func(t *testing.T) {
		p := writeForgery(t, dir, "full", `// sb-forgery: expect runtime-panic
// sb-forgery-entry: ReadForged
//
// Forgery #3: take the zero value of a proof type and read it
// through an accessor.
//
// More prose that is not the description.
package demo

func ReadForged() string { return "" }
`)
		f, err := ParseForgeryHeader(p)
		if err != nil {
			t.Fatalf("ParseForgeryHeader: %v", err)
		}
		if f.Expect != ExpectRuntimePanic {
			t.Errorf("Expect = %q, want %q", f.Expect, ExpectRuntimePanic)
		}
		if f.Entry != "ReadForged" {
			t.Errorf("Entry = %q, want ReadForged", f.Entry)
		}
		if f.Name != "full" {
			t.Errorf("Name = %q, want full", f.Name)
		}
		// The description wraps across two comment lines and ends at
		// the first period — not at the first newline.
		want := "take the zero value of a proof type and read it through an accessor"
		if f.Description != want {
			t.Errorf("Description = %q, want %q", f.Description, want)
		}
	})

	t.Run("multi-word expectation survives", func(t *testing.T) {
		p := writeForgery(t, dir, "tcb", `// sb-forgery: expect succeeds (documented TCB limit)
// sb-forgery-entry: Forge
package demo
`)
		f, err := ParseForgeryHeader(p)
		if err != nil {
			t.Fatalf("ParseForgeryHeader: %v", err)
		}
		if f.Expect != ExpectSucceeds {
			t.Errorf("Expect = %q, want %q", f.Expect, ExpectSucceeds)
		}
	})

	t.Run("directives after package are ignored", func(t *testing.T) {
		// A file that quotes a directive in its body must not be able
		// to change its own declared outcome: the corpus's value
		// depends on the header being the header.
		p := writeForgery(t, dir, "quoted", `// sb-forgery: expect compile-error
package demo

// sb-forgery: expect succeeds (documented TCB limit)
func Forge() {}
`)
		f, err := ParseForgeryHeader(p)
		if err != nil {
			t.Fatalf("ParseForgeryHeader: %v", err)
		}
		if f.Expect != ExpectCompileError {
			t.Errorf("Expect = %q, want %q — a directive in the body must not win", f.Expect, ExpectCompileError)
		}
	})

	t.Run("missing expectation is an error", func(t *testing.T) {
		p := writeForgery(t, dir, "bare", "// Attempt #1: no declaration at all.\npackage demo\n")
		if _, err := ParseForgeryHeader(p); err == nil {
			t.Fatal("want an error for a forgery with no expectation line")
		} else if !strings.Contains(err.Error(), "must declare its expected outcome") {
			t.Errorf("error = %v, want it to say the outcome must be declared", err)
		}
	})

	t.Run("unknown expectation is an error", func(t *testing.T) {
		p := writeForgery(t, dir, "typo", "// sb-forgery: expect compiler-error\npackage demo\n")
		if _, err := ParseForgeryHeader(p); err == nil {
			t.Fatal("want an error for a misspelled expectation, not a silent skip")
		}
	})

	t.Run("runtime expectation without an entry is an error", func(t *testing.T) {
		p := writeForgery(t, dir, "noentry", "// sb-forgery: expect runtime-panic\npackage demo\n")
		if _, err := ParseForgeryHeader(p); err == nil {
			t.Fatal("want an error: a runtime expectation has nothing to run without an entry point")
		}
	})

	t.Run("derive-catch without a replaces line is an error", func(t *testing.T) {
		p := writeForgery(t, dir, "noreplace", "// sb-forgery: expect derive-catch\npackage demo\n")
		if _, err := ParseForgeryHeader(p); err == nil {
			t.Fatal("want an error: derive-catch has no file to stand in for")
		}
	})
}

func TestLoadForgeryCorpusIsSorted(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"03_c", "01_a", "02_b"} {
		writeForgery(t, dir, n, "// sb-forgery: expect compile-error\npackage demo\n")
	}
	// A stray non-corpus file must be ignored rather than parsed.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a forgery"), 0o644)

	got, err := LoadForgeryCorpus(dir)
	if err != nil {
		t.Fatalf("LoadForgeryCorpus: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d forgeries, want 3", len(got))
	}
	for i, want := range []string{"01_a", "02_b", "03_c"} {
		if got[i].Name != want {
			t.Errorf("forgery[%d] = %q, want %q (corpus order must be stable)", i, got[i].Name, want)
		}
	}
}

func TestLoadForgeryCorpusMissingDir(t *testing.T) {
	if _, err := LoadForgeryCorpus(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want an error for a missing corpus directory — a gate that silently checks nothing is worse than no gate")
	}
}

func TestEntryResultCount(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte(`package demo

func None()                 {}
func One() string           { return "" }
func Two() (string, error)  { return "", nil }
func Named() (a, b, c int)  { return }
func Takes(x int) string    { return "" }
`), 0o644)

	for _, tc := range []struct {
		entry string
		want  int
	}{{"None", 0}, {"One", 1}, {"Two", 2}, {"Named", 3}} {
		got, err := entryResultCount(p, tc.entry)
		if err != nil {
			t.Fatalf("%s: %v", tc.entry, err)
		}
		if got != tc.want {
			t.Errorf("%s: got %d results, want %d", tc.entry, got, tc.want)
		}
	}

	if _, err := entryResultCount(p, "Takes"); err == nil {
		t.Error("want an error: the gate's driver cannot invent arguments, so an entry must be niladic")
	}
	if _, err := entryResultCount(p, "Absent"); err == nil {
		t.Error("want an error for an entry point that does not exist")
	}
}

func TestDeriveSpecForImplFile(t *testing.T) {
	cfg := &Config{DeriveSpecs: []DeriveSpec{
		{Lang: "go", Func: "processable", ImplFunc: "Processable", ImplPkg: "example.com/app/internal/derived"},
		{Lang: "ts", Func: "other", ImplFunc: "Other", ImplPkg: "src/other"},
	}}
	got, ok := deriveSpecForImplFile(cfg, "internal/derived/processable.go")
	if !ok {
		t.Fatal("want the derived spec to match its own impl file")
	}
	if got.ImplFunc != "Processable" {
		t.Errorf("matched %q, want Processable", got.ImplFunc)
	}
	if _, ok := deriveSpecForImplFile(cfg, "internal/elsewhere/x.go"); ok {
		t.Error("want no match for a file outside every impl package")
	}
}

func TestSpecTestName(t *testing.T) {
	if got := specTestName(DeriveSpec{ImplFunc: "Processable"}); got != "TestSpec_Processable" {
		t.Errorf("specTestName = %q, want TestSpec_Processable", got)
	}
}

func TestBuildForgeryEvidence(t *testing.T) {
	outcomes := []ForgeryOutcome{
		{Forgery: Forgery{Name: "a", Expect: ExpectCompileError}, Actual: ExpectCompileError, Passed: true},
		{Forgery: Forgery{Name: "b", Expect: ExpectSucceeds}, Actual: ExpectSucceeds, Passed: true},
		{Forgery: Forgery{Name: "c", Expect: ExpectCompileError}, Actual: ExpectSucceeds, Passed: false},
	}
	e := BuildForgeryEvidence(outcomes, "forgeries")
	if e.Total != 3 || e.AsDeclared != 2 {
		t.Errorf("Total=%d AsDeclared=%d, want 3 and 2", e.Total, e.AsDeclared)
	}
	// Both the declared TCB limit and the undeclared one count as
	// succeeding: the number a reader should look at is how many
	// forgeries actually work, not how many were expected to.
	if e.Succeeding != 2 {
		t.Errorf("Succeeding = %d, want 2", e.Succeeding)
	}
	if len(e.Mismatched) != 1 || !strings.Contains(e.Mismatched[0], "c:") {
		t.Errorf("Mismatched = %v, want the one entry that did not match", e.Mismatched)
	}
}

func TestFlowViolationLinesKeepsOnlyFailures(t *testing.T) {
	out := `PASS  constructor-only:X
      all good here
FAIL  must-pass-through:Y
      internal/h/f.go:71:19: reaches the sink with no proof
        shortest violating path: A -> B
PASS  constructor-only:Z
      also fine
`
	got := flowViolationLines(out)
	if !strings.Contains(got, "FAIL  must-pass-through:Y") {
		t.Errorf("want the FAIL block kept, got:\n%s", got)
	}
	if strings.Contains(got, "all good here") || strings.Contains(got, "also fine") {
		t.Errorf("want the PASS blocks dropped, got:\n%s", got)
	}
	if !strings.Contains(got, "shortest violating path") {
		t.Errorf("want the violation detail kept, got:\n%s", got)
	}
}

func TestRenderForgeryMarkdownFlagsMismatch(t *testing.T) {
	md := RenderForgeryMarkdown([]ForgeryOutcome{
		{Forgery: Forgery{Name: "ok", Expect: ExpectCompileError, Description: "a"}, Actual: ExpectCompileError, Passed: true},
		{Forgery: Forgery{Name: "bad", Expect: ExpectCompileError, Description: "b"}, Actual: ExpectSucceeds, Passed: false},
	})
	if !strings.Contains(md, "does not match") {
		t.Error("a mismatched row must be visibly flagged in the rendered table")
	}
	if strings.Count(md, "| `") < 2 {
		t.Errorf("want one table row per forgery, got:\n%s", md)
	}
}

func TestForgeryConfigDefaults(t *testing.T) {
	// A project that sets nothing under [forgery] still gets a working
	// gate; that is what lets a [[gates]] entry be the whole opt-in.
	if DefaultForgeryDir != "forgeries" {
		t.Errorf("DefaultForgeryDir = %q", DefaultForgeryDir)
	}
	if strings.HasPrefix(filepath.Base(DefaultForgeryStage), ".") ||
		strings.HasPrefix(filepath.Base(DefaultForgeryStage), "_") {
		t.Errorf("DefaultForgeryStage = %q — the Go toolchain skips dot and underscore directories, so the staged file would never be compiled", DefaultForgeryStage)
	}
}

func TestFlowGateRunFieldIsReusedAsTheGrep(t *testing.T) {
	cfg := &Config{Gates: []GateDef{
		{Name: "build", Kind: GateKindCommand, Run: "go build ./..."},
		{Name: "flow", Kind: GateKindFlow, Run: "./bin/audit.sh --grep-only"},
	}}
	if got := flowGateRunField(cfg); got != "./bin/audit.sh --grep-only" {
		t.Errorf("flowGateRunField = %q, want the flow gate's legacy grep", got)
	}
	if got := flowGateRunField(&Config{}); got != "" {
		t.Errorf("flowGateRunField = %q, want empty when no flow gate is configured", got)
	}
}
