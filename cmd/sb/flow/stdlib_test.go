package flow

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stdlibPath locates sb/flow/stdlib.shen relative to this source file
// so the test works from any working directory.
func stdlibPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// cmd/sb/flow/stdlib_test.go → repo root
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(root, "sb", "flow", "stdlib.shen")
}

func readStdlib(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(stdlibPath(t))
	if err != nil {
		t.Fatalf("reading the Shen flow stdlib: %v", err)
	}
	return string(data)
}

// TestStdlibFormsAreBalanced is the parse-level check: every top-level
// form in the Shen stdlib closes, with Shen `\* … *\` comments and
// string literals accounted for. The Shen host is the only thing that
// can typecheck the file; this is what can be checked without one.
func TestStdlibFormsAreBalanced(t *testing.T) {
	src := readStdlib(t)
	forms := splitForms(src)
	if len(forms) < 20 {
		t.Fatalf("found only %d top-level forms; the stdlib should have far more", len(forms))
	}
	for _, f := range forms {
		depth := 0
		inString := false
		for i := 0; i < len(f); i++ {
			switch f[i] {
			case '"':
				inString = !inString
			case '(':
				if !inString {
					depth++
				}
			case ')':
				if !inString {
					depth--
				}
			}
			if depth < 0 {
				t.Fatalf("unbalanced form: %.60s…", f)
			}
		}
		if depth != 0 || inString {
			t.Fatalf("unterminated form or string: %.60s…", f)
		}
	}
}

// TestStdlibDefinesThePromisedPredicates pins the interface between
// the Shen engine and the Go engine. Both must answer the same
// queries, so if a predicate is renamed here, docs/FLOW.md's
// correspondence table and cmd/sb/flow are out of date too.
func TestStdlibDefinesThePromisedPredicates(t *testing.T) {
	src := readStdlib(t)
	for _, pred := range []string{
		"memberp", "flow-def", "flow-ref", "flow-call",
		"reaches", "reaches-acc", "fresh?",
		"unsanctioned-caller",
		"references-proof?", "source-def",
		"must-pass-through-violation", "unsanctioned-source", "path-to-sink",
	} {
		if !strings.Contains(src, "(defprolog "+pred+"\n") {
			t.Errorf("stdlib does not define the Prolog predicate %q", pred)
		}
	}
	for _, fn := range []string{
		"def", "ref", "call",
		"canonical", "matches?", "glob?", "callable?",
	} {
		if !strings.Contains(src, "(define "+fn+"\n") {
			t.Errorf("stdlib does not define the Shen function %q", fn)
		}
	}
}

// TestStdlibClausesAreTerminated checks the one piece of Shen Prolog
// syntax a reader gets wrong most often: every clause inside a
// defprolog ends with `;`.
func TestStdlibClausesAreTerminated(t *testing.T) {
	for _, form := range splitForms(readStdlib(t)) {
		if !strings.HasPrefix(form, "(defprolog ") {
			continue
		}
		body := strings.TrimSuffix(form, ")")
		// Drop the head line, then require a `;` for each `<--`.
		arrows := strings.Count(body, "<--")
		semis := strings.Count(body, ";")
		if arrows == 0 {
			t.Errorf("defprolog with no clauses: %.60s…", form)
		}
		if semis != arrows {
			t.Errorf("%.40s…: %d clauses but %d terminators", form, arrows, semis)
		}
	}
}

// TestStdlibAgainstShenHost runs the Shen engine for real. It is
// opt-in (SB_FLOW_SHEN=1) rather than automatic on the presence of a
// Shen host, because the primary engine's execution is not exercised
// in this repository's own CI and a silent enablement would turn a
// Shen-equipped clone red for reasons unrelated to that clone's
// change. docs/FLOW.md records this as an open gap.
func TestStdlibAgainstShenHost(t *testing.T) {
	if os.Getenv("SB_FLOW_SHEN") == "" {
		t.Skip("set SB_FLOW_SHEN=1 with shen-sbcl on PATH to exercise the Shen Prolog engine")
	}
	shen, err := exec.LookPath("shen-sbcl")
	if err != nil {
		t.Skip("shen-sbcl not on PATH")
	}

	dir := t.TempDir()
	factsFile := filepath.Join(dir, "facts.shen")
	facts := loadFixture(t)
	if err := os.WriteFile(factsFile, []byte(facts.WriteFacts()), 0o644); err != nil {
		t.Fatal(err)
	}

	// The allowlist and the constructor pattern are what sb generates
	// per premise before the query runs.
	queryFile := filepath.Join(dir, "query.shen")
	query := `
(define allowed-caller?
  Ctor Caller -> (matches? "app/Check" Caller))

(output "UNSANCTIONED: ~A~%"
        (prolog? (unsanctioned-caller "guard/NewAccess" Caller File Line)))
(output "VIOLATION: ~A~%"
        (prolog? (must-pass-through-violation "app/HandleList*" "app/Check" "Store#Query" Path)))
`
	if err := os.WriteFile(queryFile, []byte(query), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(shen, "-q",
		"-l", stdlibPath(t),
		"-l", factsFile,
		"-l", queryFile)
	out, err := cmd.CombinedOutput()
	t.Logf("shen output:\n%s", out)
	if err != nil {
		t.Fatalf("shen host failed: %v", err)
	}
	// Both queries must succeed: the fixture has exactly one
	// unsanctioned caller (ForgeAccess) and one violating path
	// (HandleListBad → readRows → Store#Query).
	for _, want := range []string{"UNSANCTIONED: true", "VIOLATION: true"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("shen engine did not report %q", want)
		}
	}
}
