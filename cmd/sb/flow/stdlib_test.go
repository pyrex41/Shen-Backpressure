package flow

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
		"ctor-reference", "unsanctioned-caller",
		"source-def",
		"must-pass-through-violation", "unsanctioned-source", "path-to-sink",
	} {
		if !strings.Contains(src, "(defprolog "+pred+"\n") {
			t.Errorf("stdlib does not define the Prolog predicate %q", pred)
		}
	}
	// `calls`, not `call`: `call` is a Shen system function, so
	// `(define call …)` made this whole file unloadable and the Shen
	// engine had never run. Asserting the new name here is what keeps
	// the rename from being undone by a tidy-up.
	// defines-ctor-in-file? and references-proof? are Shen functions,
	// not Prolog predicates, because they are used under negation and
	// a `when` guard cannot call back into `prolog?` and see the
	// clause's bindings. The stdlib's own comment says why; asserting
	// the shape here keeps a future tidy-up from turning them back
	// into predicates and reintroducing the "mustString" abort.
	for _, fn := range []string{
		"def", "ref", "calls",
		"canonical", "matches?", "glob?", "callable?",
		"defines-ctor-in-file?", "references-proof?",
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

// findShenHost mirrors cmd/sb's ResolveShenHost. It is duplicated
// rather than imported because this is package flow, which must not
// depend on package main.
//
// Unlike the pre-W6 version, it looks for any port — shen-sbcl,
// shen-scheme, or a plain `shen`, including the repo's own bin/shen
// that `make shen-go` builds. Looking only for shen-sbcl was half of
// why the Shen engine was never exercised; the other half was the
// SB_FLOW_SHEN opt-in these tests used to sit behind.
func findShenHost(t *testing.T) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("SHEN")); v != "" {
		if p, err := exec.LookPath(v); err == nil {
			return p
		}
		if fi, err := os.Stat(v); err == nil && !fi.IsDir() {
			return v
		}
	}
	for _, n := range []string{"shen-sbcl", "shen-scheme", "shen"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	root := filepath.Join(filepath.Dir(stdlibPath(t)), "..", "..")
	if p := filepath.Join(root, "bin", "shen"); isExecutable(p) {
		return p
	}
	return ""
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// writeFixtureFacts renders the hand-written fact fixture to a file.
func writeFixtureFacts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "facts.shen")
	if err := os.WriteFile(path, []byte(loadFixture(t).WriteFacts()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestStdlibLoadsIntoShenHost is the check that would have caught the
// bug W6 fixed: the file loads at all. `(define call …)` made it
// unloadable — "call is a system function" — for four workstreams,
// while every document called it the primary engine.
//
// It runs whenever a host is resolvable, with no opt-in. An opt-in on
// a test whose only job is to execute something is a way of not
// executing it.
func TestStdlibLoadsIntoShenHost(t *testing.T) {
	host := findShenHost(t)
	if host == "" {
		t.Skip("no Shen host: set $SHEN, put shen-sbcl/shen-scheme/shen on PATH, or run `make shen-go`")
	}
	out, err := exec.Command(host, "eval", "-q", "-l", stdlibPath(t),
		"-e", `(output "STDLIB-LOADED~%")`).CombinedOutput()
	if err != nil {
		t.Fatalf("loading the Prolog stdlib into %s failed: %v\n%s", host, err, out)
	}
	if !strings.Contains(string(out), "STDLIB-LOADED") {
		t.Fatalf("the host did not finish loading the stdlib:\n%s", out)
	}
	for _, bad := range []string{"is a system function", "does not exist", "not a function"} {
		if strings.Contains(string(out), bad) {
			t.Fatalf("the host complained while loading the stdlib (%q):\n%s", bad, out)
		}
	}
}

// TestStdlibAgainstShenHost runs the Prolog rules for real over the
// hand-written fixture, through the same EvaluateShen path `sb flow`
// uses, and checks the verdicts against what the fixture was built to
// contain.
func TestStdlibAgainstShenHost(t *testing.T) {
	host := findShenHost(t)
	if host == "" {
		t.Skip("no Shen host: set $SHEN, put shen-sbcl/shen-scheme/shen on PATH, or run `make shen-go`")
	}
	factsFile := writeFixtureFacts(t)

	decls, err := ParseSpec("fixture.shen", `(flow tiny-discipline
  (constructor-only guard/NewAccess app/Check)
  (must-pass-through app/HandleList* app/Check Store#Query))`)
	if err != nil {
		t.Fatal(err)
	}

	verdicts, out, err := EvaluateShen(ShenHostRunner{
		Path:       host,
		StdlibPath: stdlibPath(t),
		Timeout:    120 * time.Second,
	}, factsFile, decls)
	t.Logf("shen output:\n%s", out)
	if err != nil {
		t.Fatalf("Shen engine: %v", err)
	}
	if len(verdicts) != 2 {
		t.Fatalf("want 2 verdicts, got %d: %+v", len(verdicts), verdicts)
	}
	// The fixture has exactly one unsanctioned caller (ForgeAccess)
	// and one violating path (HandleListBad → readRows → Store#Query),
	// so both premises range over something and both are violated.
	for _, v := range verdicts {
		if !v.Considered {
			t.Errorf("%s: the Shen engine found nothing to range over, so the fixture is not exercising the rule", v.PremiseID)
		}
		if !v.Violation {
			t.Errorf("%s: the Shen engine found no violation, but the fixture contains one", v.PremiseID)
		}
	}
}

// TestShenAndGoEnginesAgreeOnFixture is the claim docs/FLOW.md has
// always made and nothing checked: the Prolog rules and their Go
// transcription decide the same thing. `sb flow --engine both` runs
// this comparison on real facts; this runs it on the fixture, which
// has a violation of each premise kind and so is the case most likely
// to expose a difference.
func TestShenAndGoEnginesAgreeOnFixture(t *testing.T) {
	host := findShenHost(t)
	if host == "" {
		t.Skip("no Shen host: set $SHEN, put shen-sbcl/shen-scheme/shen on PATH, or run `make shen-go`")
	}
	factsFile := writeFixtureFacts(t)
	decls, err := ParseSpec("fixture.shen", `(flow tiny-discipline
  (constructor-only guard/NewAccess app/Check)
  (must-pass-through app/HandleList* app/Check Store#Query))`)
	if err != nil {
		t.Fatal(err)
	}

	goResults := Evaluate(loadFixture(t), decls)
	verdicts, out, err := EvaluateShen(ShenHostRunner{
		Path:       host,
		StdlibPath: stdlibPath(t),
		Timeout:    120 * time.Second,
	}, factsFile, decls)
	if err != nil {
		t.Fatalf("Shen engine: %v\n%s", err, out)
	}
	if d := CompareEngines(goResults, verdicts); len(d) > 0 {
		for _, dd := range d {
			t.Errorf("engines disagree: %s", dd)
		}
		t.Logf("shen output:\n%s", out)
	}
}
