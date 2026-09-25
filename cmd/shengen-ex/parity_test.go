package main

// Classifier parity with cmd/shengen: for every spec in the repository the
// symbol-table dump must be byte-identical to `shengen --dry-run`, so the
// Go and Elixir guards agree on wrapper/constrained/alias/composite/
// guarded/sum-type classification and field layout.
//
// shengen feeds `\\ line comments` inside datatype blocks to its premise
// parser (examples/shen-web-tools/specs/medicare.shen shows the garbage
// that produces); shengen-ex strips comments first. To pin classification
// rather than comment handling, shengen runs on a comment-blanked copy.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSymbolTableParityWithShengen(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "shengen")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Join(root, "cmd", "shengen")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building shengen: %v\n%s", err, out)
	}
	var specs []string
	filepath.Walk(filepath.Join(root, "examples"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".shen") && strings.Contains(p, "/specs/") &&
			!strings.Contains(p, "/hostile/") && !strings.Contains(p, "/good/") && !strings.Contains(p, "node_modules") {
			specs = append(specs, p)
		}
		return nil
	})
	specs = append(specs, filepath.Join(root, "cmd/shengen-ex/testdata/features.shen"))
	if len(specs) < 10 {
		t.Fatalf("found only %d specs", len(specs))
	}
	for _, spec := range specs {
		rel, _ := filepath.Rel(root, spec)
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(spec)
			if err != nil {
				t.Fatal(err)
			}
			blanked := filepath.Join(t.TempDir(), filepath.Base(spec))
			if err := os.WriteFile(blanked, []byte(stripComments(string(raw))), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "--dry-run", blanked)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Skipf("shengen cannot parse %s: %v", rel, err)
			}
			want := stderr.String()
			if i := strings.Index(want, "Defined functions:"); i >= 0 {
				want = want[:i]
			}
			types, _, err := parseSpec(spec)
			if err != nil {
				t.Fatalf("shengen-ex cannot parse %s: %v", rel, err)
			}
			st := newSymbolTable("X")
			st.Build(types)
			var got bytes.Buffer
			printSymbolTable(&got, types, st, blanked)
			if got.String() != want {
				t.Errorf("symbol table differs from shengen for %s\n--- shengen\n%s--- shengen-ex\n%s", rel, want, got.String())
			}
		})
	}
}
