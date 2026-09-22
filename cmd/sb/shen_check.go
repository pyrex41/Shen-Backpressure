package main

// shen_check.go — W6.A/B. Gate 4, `tc +`, with a host and a prelude.
//
// Gate 4 has always been "load the spec into a Shen host with the
// typechecker on". Two things kept it from ever being that:
//
//  1. No host was resolved. The three copies of bin/shen-check.sh
//     looked for shen-sbcl and shen-scheme only, and wrote the flags
//     as `shen -q -e … -l …` — a form no Shen port accepts, since -q
//     belongs to the `eval` subcommand. ResolveShenHost
//     (shenhost.go) and ShenEvalArgs fix both.
//
//  2. No prelude was loaded. A spec's `(define …)` bodies call
//     shen-derive's evaluator intrinsics — `val`, the field
//     accessors, `scanl` — which are not Shen functions, so tc+
//     rejected the spec on sight. `shen-derive prelude` emits typed
//     declarations for exactly those, and this command loads them
//     around `(tc +)` in the order Shen requires.
//
// The result is a claim with a stated premise: the spec is well-typed
// *given the prelude's intrinsic signatures*. docs/TRUST-MODEL.md
// names the prelude as a TCB member for that reason.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PreludeDir is where the generated prelude lands: under .sb/ with the
// other per-run artifacts, so it is rewritten from the spec on every
// run and can never describe an older spec than the one being checked.
const PreludeDir = ".sb"

const (
	preludeDeclaresName = "prelude.declares.shen"
	preludeDefinesName  = "prelude.defines.shen"
)

// ShenCheckResult is what a tc+ run produced.
type ShenCheckResult struct {
	Host   *ShenHost
	Output string
	// Skipped is true when no host was available, so nothing ran.
	Skipped bool
	// Err is non-nil when the host reported a type error or failed.
	Err error
}

func cmdShenCheck(args []string) {
	fs := flag.NewFlagSet("shen-check", flag.ExitOnError)
	specFlag := fs.String("spec", "", "spec to check (default: [paths] spec from sb.toml)")
	noPrelude := fs.Bool("no-prelude", false, "do not generate or load the intrinsic prelude (tc+ over the bare spec)")
	timeout := fs.Duration("timeout", 60*time.Second, "kill the host after this long")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb shen-check — Gate 4: typecheck the spec in a Shen host

Usage: sb shen-check [flags]

Resolves a Shen host ($SHEN, then [shen] bin in sb.toml, then
shen-sbcl / shen-scheme / shen on PATH), asks shen-derive for the
typed prelude that declares the evaluator's intrinsics, and runs

  <host> eval -q -l %s/%s -e '(tc +)' \
         -l %s/%s -l <spec>

Exits 0 when the host typechecks the spec, 1 when it reports a type
error, and 2 when no host is available (nothing was checked — the
report records tc+ as UNVERIFIED in that case rather than passed).

Flags:
`, PreludeDir, preludeDeclaresName, PreludeDir, preludeDefinesName)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb shen-check: %v\n", err)
		os.Exit(1)
	}
	spec := *specFlag
	if spec == "" {
		spec = cfg.Spec
	}
	if spec == "" {
		fmt.Fprintln(os.Stderr, "sb shen-check: no spec configured ([paths] spec in sb.toml) and none given with --spec")
		os.Exit(1)
	}

	res := RunShenCheck(cfg, spec, *noPrelude, *timeout)
	if res.Skipped {
		fmt.Fprintf(os.Stderr, "sb shen-check: SKIP — %s\n", ShenInstallHint)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "Gate 4: Shen tc+ — %s (host: %s)\n", spec, res.Host)
	if res.Err != nil {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(res.Output))
		fmt.Fprintf(os.Stderr, "RESULT: FAIL (%v)\n", res.Err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "RESULT: PASS")
}

// RunShenCheck is the reusable half: `sb verify-report` calls it to
// re-run tc+ on a committed report's spec.
//
// A host's exit status is not the whole answer. Shen prints
// "ERROR: type error in rule N of f" and, for a spec whose types send
// the sequent search off a cliff, "maximum inferences exceeded" — the
// second of which is a failure to decide, not a pass. Both are treated
// as failures, and both are quoted verbatim so the next prompt gets
// the rule number.
func RunShenCheck(cfg *Config, spec string, noPrelude bool, timeout time.Duration) ShenCheckResult {
	host := ResolveShenHost(cfg)
	if !host.Found() {
		return ShenCheckResult{Skipped: true}
	}
	if _, err := os.Stat(spec); err != nil {
		return ShenCheckResult{Host: host, Err: fmt.Errorf("spec %s: %w", spec, err)}
	}

	var preFiles, files []string
	if !noPrelude {
		decl, def, err := ensurePrelude(cfg, spec)
		if err != nil {
			return ShenCheckResult{Host: host, Err: err}
		}
		if decl != "" {
			preFiles = append(preFiles, decl)
		}
		if def != "" {
			files = append(files, def)
		}
	}
	files = append(files, spec)

	args := ShenEvalArgs(preFiles, []string{"(tc +)"}, files)
	cmd := exec.Command(host.Path, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return ShenCheckResult{Host: host, Err: err}
	}
	go func() { done <- cmd.Wait() }()
	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		runErr = fmt.Errorf("host timed out after %s", timeout)
	}

	out := buf.String()
	res := ShenCheckResult{Host: host, Output: out, Err: runErr}
	if res.Err == nil {
		if msg := shenTypeFailure(out); msg != "" {
			res.Err = fmt.Errorf("%s", msg)
		}
	}
	return res
}

// shenTypeFailure returns the host's own words when its output says
// the check did not succeed, and "" when it did.
func shenTypeFailure(out string) string {
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		low := strings.ToLower(l)
		switch {
		case strings.HasPrefix(low, "error:"),
			strings.Contains(low, "type error"),
			strings.Contains(low, "maximum inferences exceeded"):
			return l
		}
	}
	return ""
}

// ensurePrelude regenerates the prelude for spec and returns the two
// file paths. Both are absolute, because the host is run from the
// project directory but the paths are also handed to `verify-report`,
// which may not be.
//
// The generator lives in shen-derive (it needs the spec parser and the
// type table), and is invoked the same way `sb derive` invokes it:
// `go run .` in the shen-derive module directory. A project with no
// shen-derive checkout gets no prelude rather than an error — tc+ over
// the bare spec is still a real check for a spec with no defines,
// which is most of them.
func ensurePrelude(cfg *Config, spec string) (declares, defines string, err error) {
	deriveDir := "../../shen-derive"
	if cfg != nil && cfg.DeriveDir != "" {
		deriveDir = cfg.DeriveDir
	}
	absDerive, err := filepath.Abs(deriveDir)
	if err != nil {
		return "", "", err
	}
	if _, statErr := os.Stat(absDerive); statErr != nil {
		fmt.Fprintf(os.Stderr, "sb shen-check: no shen-derive at %s; running tc+ over the bare spec\n", absDerive)
		return "", "", nil
	}
	absSpec, err := filepath.Abs(spec)
	if err != nil {
		return "", "", err
	}
	absOut, err := filepath.Abs(PreludeDir)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return "", "", err
	}

	cmd := exec.Command("go", "run", ".", "prelude",
		"--spec", absSpec, "--out-dir", absOut)
	cmd.Dir = absDerive
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return "", "", fmt.Errorf("shen-derive prelude: %v\n%s", runErr, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		// GAP lines belong on the gate's transcript: an intrinsic the
		// generator could not type is the reason a later tc+ failure
		// is not the spec author's fault.
		for _, line := range strings.Split(s, "\n") {
			if strings.Contains(line, "GAP:") {
				fmt.Fprintln(os.Stderr, line)
			}
		}
	}
	return filepath.Join(absOut, preludeDeclaresName),
		filepath.Join(absOut, preludeDefinesName), nil
}
