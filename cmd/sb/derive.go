package main

// sb derive — the spec-equivalence verification gate.
//
// For every [[derive.specs]] entry in sb.toml, `sb derive` invokes
// `shen-derive verify` (via `go run`, because shen-derive is a separate
// Go module) and diffs the regenerated test file against the committed
// copy. Drift is treated as failure. After all files pass the drift
// check, `go test` runs on each distinct implementation package.
//
// With --regen the subcommand writes the regenerated file over OutFile
// instead of diffing. This mirrors the payment example's Makefile
// pattern (shen-derive-verify vs shen-derive-regen) but lifts it into
// the sb CLI so every Shen-Backpressure project can opt in via
// sb.toml alone.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
)

func cmdDerive(args []string) {
	fs := flag.NewFlagSet("derive", flag.ExitOnError)
	regen := fs.Bool("regen", false, "write regenerated test files in place instead of diffing")
	skipTest := fs.Bool("skip-test", false, "skip `go test` on impl packages after the drift check")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb derive — Spec-equivalence verification gate

Usage: sb derive [flags]

For each [[derive.specs]] entry in sb.toml, regenerates the
shen-derive test file and fails if it differs from the committed
copy. Then runs `+"`go test`"+` on each referenced implementation package.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb derive: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.DeriveSpecs) == 0 {
		fmt.Fprintln(os.Stderr, "sb derive: no [[derive.specs]] entries in sb.toml — nothing to do")
		return
	}

	deriveDir := cfg.DeriveDir
	if deriveDir == "" {
		// Conventional location: the shen-derive module sits at
		// ../../shen-derive relative to an examples/<project> directory.
		// Fall back to that if not explicitly configured.
		deriveDir = "../../shen-derive"
	}
	absDeriveDir, err := filepath.Abs(deriveDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb derive: resolve derive dir: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stat(absDeriveDir); err != nil {
		fmt.Fprintf(os.Stderr, "sb derive: shen-derive not found at %s: %v\n", absDeriveDir, err)
		os.Exit(1)
	}

	// First pass: regenerate + diff (or overwrite in --regen mode).
	drifted := 0
	implPkgs := map[string]bool{}
	for i, spec := range cfg.DeriveSpecs {
		if err := validateDeriveSpec(spec); err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: spec[%d]: %v\n", i, err)
			os.Exit(1)
		}
		implPkgs[spec.ImplPkg] = true

		absSpec, _ := filepath.Abs(spec.Path)
		absOut, _ := filepath.Abs(spec.OutFile)

		regenArgs := []string{
			"run", ".",
			"verify", absSpec,
			"--func", spec.Func,
			"--impl-pkg", spec.ImplPkg,
			"--impl-func", spec.ImplFunc,
			"--import", spec.GuardPkg,
		}
		if spec.Seed != 0 {
			regenArgs = append(regenArgs, "--seed", strconv.FormatInt(spec.Seed, 10))
		}

		fmt.Fprintf(os.Stderr, "sb derive: [%s] %s → %s\n", spec.Func, spec.Path, spec.OutFile)

		if *regen {
			regenArgs = append(regenArgs, "--out", absOut)
			if err := runInDir(absDeriveDir, "go", regenArgs...); err != nil {
				fmt.Fprintf(os.Stderr, "sb derive: regen %s: %v\n", spec.Func, err)
				os.Exit(1)
			}
			continue
		}

		// Normal path: write to a tempfile, diff against the committed
		// output, and report drift.
		tmp, err := os.CreateTemp("", "shen-derive-*.go")
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: tempfile: %v\n", err)
			os.Exit(1)
		}
		tmpPath := tmp.Name()
		tmp.Close()
		defer os.Remove(tmpPath)

		regenArgs = append(regenArgs, "--out", tmpPath)
		if err := runInDir(absDeriveDir, "go", regenArgs...); err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: regen %s: %v\n", spec.Func, err)
			os.Exit(1)
		}

		got, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: read regen output: %v\n", err)
			os.Exit(1)
		}
		want, err := os.ReadFile(absOut)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: read committed file %s: %v\n"+
				"  run `sb derive --regen` to create it.\n", absOut, err)
			os.Exit(1)
		}
		if !bytes.Equal(got, want) {
			drifted++
			fmt.Fprintf(os.Stderr, "  DRIFT: %s differs from regenerated output.\n", spec.OutFile)
			diffOut, _ := exec.Command("diff", "-u", absOut, tmpPath).CombinedOutput()
			fmt.Fprintln(os.Stderr, string(diffOut))
		}
	}

	if drifted > 0 {
		fmt.Fprintf(os.Stderr, "sb derive: %d file(s) stale. Run `sb derive --regen` to update.\n", drifted)
		os.Exit(1)
	}

	if *regen || *skipTest {
		return
	}

	// Second pass: run `go test` on each implementation package. These
	// packages live in the project's own Go module (not shen-derive), so
	// we run the tests from the current working directory.
	pkgs := make([]string, 0, len(implPkgs))
	for p := range implPkgs {
		pkgs = append(pkgs, p+"/...")
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		if err := runInDir("", "go", "test", p); err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: go test %s failed: %v\n", p, err)
			os.Exit(1)
		}
	}
}

func validateDeriveSpec(s DeriveSpec) error {
	missing := []string{}
	if s.Path == "" {
		missing = append(missing, "path")
	}
	if s.Func == "" {
		missing = append(missing, "func")
	}
	if s.ImplPkg == "" {
		missing = append(missing, "impl_pkg")
	}
	if s.ImplFunc == "" {
		missing = append(missing, "impl_func")
	}
	if s.GuardPkg == "" {
		missing = append(missing, "guard_pkg")
	}
	if s.OutFile == "" {
		missing = append(missing, "out_file")
	}
	if len(missing) > 0 {
		return fmt.Errorf("[[derive.specs]] missing fields: %v", missing)
	}
	return nil
}

func runInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
