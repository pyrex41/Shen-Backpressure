package prelude

// prelude_host_test.go — W6.B acceptance. Runs the emitted prelude
// through a real Shen host's typechecker over each example spec.
//
// Unlike the pre-W6 Shen-host test in cmd/sb/flow, this one is not
// behind an opt-in environment variable: it runs whenever a host is
// resolvable and skips with a reason when none is. An opt-in gate on a
// test whose whole purpose is to prove a claim about a host means the
// claim is never checked, which is exactly what happened to gate 4.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// findShenHost mirrors cmd/sb's ResolveShenHost. The two modules cannot
// import each other, so the order is duplicated and pinned by this
// comment: $SHEN, then shen-sbcl / shen-scheme / shen on PATH, then
// the repo's own bin/shen that `make shen-go` builds.
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
	if p := filepath.Join(repoRoot(t), "bin", "shen"); isExecutable(p) {
		return p
	}
	return ""
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// shen-derive/prelude/prelude_host_test.go → repo root
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// runTC loads the prelude and the given files into the host with the
// typechecker on, and returns the host's combined output.
//
// The argument shape is the contract the whole of W6 rests on: -q is a
// flag of the `eval` subcommand; the declares load before `(tc +)`
// because Shen's `declare` cannot run under the typechecker; the
// defines and the spec load after it so the host checks them.
func runTC(t *testing.T, host, declares, defines string, files ...string) (string, error) {
	t.Helper()
	args := []string{"eval", "-q", "-l", declares, "-e", "(tc +)", "-l", defines}
	for _, f := range files {
		args = append(args, "-l", f)
	}
	cmd := exec.Command(host, args...)
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		t.Fatalf("shen host did not finish in 120s\n%s", out)
	}
	return string(out), err
}

// writePrelude generates the prelude for a spec into a temp dir.
func writePrelude(t *testing.T, specPath string) (declares, defines string) {
	t.Helper()
	sf, err := specfile.ParseFile(specPath)
	if err != nil {
		t.Fatalf("parsing %s: %v", specPath, err)
	}
	p := Build(sf)
	dir := t.TempDir()
	declares = filepath.Join(dir, "prelude.declares.shen")
	defines = filepath.Join(dir, "prelude.defines.shen")
	if err := os.WriteFile(declares, []byte(p.Declares), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defines, []byte(p.Defines), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, n := range p.Notes {
		t.Logf("prelude gap: %s", n)
	}
	return declares, defines
}

// failureLine is the host's own words when it did not typecheck.
// Mirrors cmd/sb's shenTypeFailure: a non-zero exit is not the only
// failure mode, and "maximum inferences exceeded" — the sequent search
// giving up — is a failure to decide, not a pass.
func failureLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		low := strings.ToLower(l)
		if strings.HasPrefix(low, "error:") ||
			strings.Contains(low, "type error") ||
			strings.Contains(low, "maximum inferences exceeded") {
			return l
		}
	}
	return ""
}

// TestExampleSpecsTypecheckWithPrelude is the gate-4 claim, measured:
// every example spec plus its generated prelude passes `tc +` in a real
// Shen host.
func TestExampleSpecsTypecheckWithPrelude(t *testing.T) {
	host := findShenHost(t)
	if host == "" {
		t.Skip("no Shen host: set $SHEN, put shen-sbcl/shen-scheme/shen on PATH, or run `make shen-go`")
	}
	t.Logf("host: %s", host)

	root := repoRoot(t)
	for _, rel := range []string{
		"examples/payment/specs/core.shen",
		"examples/multi-tenant-api/specs/core.shen",
	} {
		t.Run(rel, func(t *testing.T) {
			spec := filepath.Join(root, rel)
			if _, err := os.Stat(spec); err != nil {
				t.Skipf("%s not present: %v", rel, err)
			}
			declares, defines := writePrelude(t, spec)
			out, err := runTC(t, host, declares, defines, spec)
			if err != nil {
				t.Fatalf("host exited non-zero: %v\n%s", err, out)
			}
			if msg := failureLine(out); msg != "" {
				t.Fatalf("tc+ rejected %s: %s\n%s", rel, msg, out)
			}
		})
	}
}

// TestIllTypedDefineStillFails is the other half of the claim, and the
// more important one: a prelude that made tc+ pass *everything* would
// be worse than no prelude. The fixture reuses payment's datatypes so
// the only difference from the passing case is one wrong type.
func TestIllTypedDefineStillFails(t *testing.T) {
	host := findShenHost(t)
	if host == "" {
		t.Skip("no Shen host: set $SHEN, put shen-sbcl/shen-scheme/shen on PATH, or run `make shen-go`")
	}

	spec := filepath.Join(repoRoot(t), "examples", "payment", "specs", "core.shen")
	if _, err := os.Stat(spec); err != nil {
		t.Skipf("payment spec not present: %v", err)
	}
	declares, defines := writePrelude(t, spec)

	// `(val (amount Tx))` is a number; a define that claims it is a
	// string is ill-typed however generous the prelude is. Note that
	// this is precisely the mistake the prelude's `val` signature is
	// there to catch: with `val` untyped, or typed [A --> B], this
	// would pass.
	bad := filepath.Join(t.TempDir(), "illtyped.shen")
	if err := os.WriteFile(bad, []byte(`(define w6-deliberately-ill-typed
  {transaction --> string}
  Tx -> (val (amount Tx)))
`), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runTC(t, host, declares, defines, spec, bad)
	msg := failureLine(out)
	if err == nil && msg == "" {
		t.Fatalf("tc+ accepted a define that returns a number where it declares a string;\n"+
			"the prelude's intrinsic signatures are too weak to be evidence\n%s", out)
	}
	t.Logf("tc+ rejected the ill-typed define as expected: %s", msg)
}
