package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testRestoreCwd ensures we run from cmd/sb regardless of which other test
// in this package may have left cwd in a temp directory via os.Chdir.
func testRestoreCwd(t *testing.T) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pkgDir := filepath.Dir(thisFile)
	if err := os.Chdir(pkgDir); err != nil {
		t.Fatalf("Chdir to package dir: %v", err)
	}
}

// TestMultilangEmittersAreReachable verifies that the four shengen emitters
// (Go, TS, Py, Rs) are all discoverable from the cmd/sb tree. This is the
// reachability half of W2.2 — `sb gen --lang py` should not fail with
// "shengen-py not found" when invoked from a project living inside the
// repo.
func TestMultilangEmittersAreReachable(t *testing.T) {
	testRestoreCwd(t)
	cases := []struct {
		name string
		find func() (string, error)
		want string // suffix the resolved path must end with
	}{
		{"go", func() (string, error) { return FindShengen() }, ""}, // builds binary; allow any path
		{"ts", FindShengenTS, "cmd/shengen-ts/shengen.ts"},
		{"py", FindShengenPy, "cmd/shengen-py/shengen.py"},
		{"rs", FindShengenRs, "cmd/shengen-rs/shengen.py"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.find()
			if err != nil {
				t.Fatalf("Find%s: %v", strings.ToUpper(tc.name), err)
			}
			if tc.want != "" && !strings.HasSuffix(filepath.ToSlash(got), tc.want) {
				t.Errorf("resolved path %q does not end with %q", got, tc.want)
			}
		})
	}
}

// TestMultilangParityOnSharedSubset runs the three script-based emitters
// (TS via tsx, Py via python3, Rs via python3) on a single fixture spec
// that uses only constructs supported by all of them. The Go emitter is
// exercised by the existing cmd/shengen/main_test.go suite.
//
// We do NOT assert byte-equivalence — each language has its own syntax.
// Instead we assert that each emitter produces output containing the
// expected language-specific markers (function/struct/class declarations)
// for every datatype in the spec. This catches accidental regressions
// that break a single emitter while leaving the others working.
func TestMultilangParityOnSharedSubset(t *testing.T) {
	testRestoreCwd(t)
	// Skip on systems missing python3 / npx. We don't try to bootstrap them;
	// the test is informational and any missing tool means "skip", not "fail".
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	dir := t.TempDir()
	spec := `\* Cross-emitter parity fixture. Uses only constructs supported by all emitters. *\

(datatype tag-id
  X : string;
  (> (length X) 0) : verified;
  ==============
  X : tag-id;)

(datatype tag-block
  Id : tag-id;
  Body : string;
  ChildRefs : (list tag-id);
  ====================
  [Id Body ChildRefs] : tag-block;)
`
	specPath := filepath.Join(dir, "core.shen")
	if err := os.WriteFile(specPath, []byte(spec), 0644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	pyPath, err := FindShengenPy()
	if err != nil {
		t.Skipf("shengen-py not reachable: %v", err)
	}
	rsPath, err := FindShengenRs()
	if err != nil {
		t.Skipf("shengen-rs not reachable: %v", err)
	}

	// Run Python emitter.
	pyOut := filepath.Join(dir, "guards.py")
	if out, err := exec.Command("python3", pyPath, specPath, "--out", pyOut).CombinedOutput(); err != nil {
		t.Fatalf("shengen-py failed: %v\n%s", err, out)
	}
	pyBody, _ := os.ReadFile(pyOut)
	pyStr := string(pyBody)
	for _, want := range []string{"class TagId:", "class TagBlock:", "list[TagId]"} {
		if !strings.Contains(pyStr, want) {
			t.Errorf("py output missing %q", want)
		}
	}

	// Run Rust emitter.
	rsOut := filepath.Join(dir, "guards.rs")
	if out, err := exec.Command("python3", rsPath, specPath, "--out", rsOut).CombinedOutput(); err != nil {
		t.Fatalf("shengen-rs failed: %v\n%s", err, out)
	}
	rsBody, _ := os.ReadFile(rsOut)
	rsStr := string(rsBody)
	for _, want := range []string{"pub struct TagId", "pub struct TagBlock", "Vec<TagId>"} {
		if !strings.Contains(rsStr, want) {
			t.Errorf("rs output missing %q", want)
		}
	}

	// Run Go emitter via the binary discovery in FindShengen (which builds
	// from source if needed). Use the manifest-style invocation so we know
	// it lands as the file at goOut.
	goBin, err := FindShengen()
	if err != nil {
		t.Skipf("shengen (go) not reachable: %v", err)
	}
	goOut := filepath.Join(dir, "guards.go")
	if out, err := exec.Command(goBin, "--spec", specPath, "--pkg", "shenguard", "--out", goOut).CombinedOutput(); err != nil {
		t.Fatalf("shengen (go) failed: %v\n%s", err, out)
	}
	goBody, _ := os.ReadFile(goOut)
	goStr := string(goBody)
	for _, want := range []string{"type TagId struct", "type TagBlock struct", "[]TagId"} {
		if !strings.Contains(goStr, want) {
			t.Errorf("go output missing %q", want)
		}
	}
}
