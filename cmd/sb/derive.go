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
	verbose := fs.Bool("verbose", false, "print each shen-derive and test command before running it")
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

	// Resolve the Go shen-derive module dir once (only needed if any
	// spec uses lang="go"). For TS specs, we invoke shen-derive-ts via
	// its source path inside the repo and don't need this.
	hasGo := false
	for _, s := range cfg.DeriveSpecs {
		if s.Lang == "go" {
			hasGo = true
			break
		}
	}
	var absDeriveDir string
	if hasGo {
		deriveDir := cfg.DeriveDir
		if deriveDir == "" {
			deriveDir = "../../shen-derive"
		}
		var err error
		absDeriveDir, err = filepath.Abs(deriveDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: resolve derive dir: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stat(absDeriveDir); err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: shen-derive not found at %s: %v\n", absDeriveDir, err)
			os.Exit(1)
		}
	}

	// For TS specs we invoke cmd/shen-derive-ts/shen-derive.ts from the
	// project directory via `npx tsx`. The path is repo-conventional:
	// ../../cmd/shen-derive-ts/shen-derive.ts relative to examples/<proj>.
	tsCLI := "../../cmd/shen-derive-ts/shen-derive.ts"
	absTSCLI, _ := filepath.Abs(tsCLI)

	// First pass: regenerate + diff (or overwrite in --regen mode).
	drifted := 0
	goImplPkgs := map[string]bool{}
	tsTestFiles := []string{}
	for i, spec := range cfg.DeriveSpecs {
		if err := validateDeriveSpec(spec); err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: spec[%d]: %v\n", i, err)
			os.Exit(1)
		}

		absSpec, _ := filepath.Abs(spec.Path)
		absOut, _ := filepath.Abs(spec.OutFile)

		fmt.Fprintf(os.Stderr, "sb derive: [%s/%s] %s → %s\n", spec.Lang, spec.Func, spec.Path, spec.OutFile)

		var (
			runCmd    string
			runArgs   []string
			runDir    string
			tempGlob  string
		)
		switch spec.Lang {
		case "go":
			goImplPkgs[spec.ImplPkg] = true
			runCmd = "go"
			runArgs = []string{
				"run", ".",
				"verify", absSpec,
				"--func", spec.Func,
				"--impl-pkg", spec.ImplPkg,
				"--impl-func", spec.ImplFunc,
				"--import", spec.GuardPkg,
			}
			runDir = absDeriveDir
			tempGlob = "shen-derive-*.go"
		case "ts":
			tsTestFiles = append(tsTestFiles, spec.OutFile)
			runCmd = "npx"
			runArgs = []string{
				"tsx", absTSCLI,
				"verify", absSpec,
				"--func", spec.Func,
				"--impl-module", spec.ImplPkg,
				"--impl-func", spec.ImplFunc,
				"--import", spec.GuardPkg,
			}
			runDir = "" // run in the project cwd
			tempGlob = "shen-derive-*.ts"
		default:
			fmt.Fprintf(os.Stderr, "sb derive: spec[%d]: unknown lang %q (want go or ts)\n", i, spec.Lang)
			os.Exit(1)
		}

		if spec.Seed != 0 {
			runArgs = append(runArgs, "--seed", strconv.FormatInt(spec.Seed, 10))
		}

		if *regen {
			runArgs = append(runArgs, "--out", absOut)
			if *verbose {
				printDeriveCommand(runDir, runCmd, runArgs)
			}
			if err := runInDir(runDir, runCmd, runArgs...); err != nil {
				fmt.Fprintf(os.Stderr, "sb derive: regen %s: %v\n", spec.Func, err)
				os.Exit(1)
			}
			continue
		}

		// Normal path: write to a tempfile, diff against the committed
		// output, and report drift.
		tmp, err := os.CreateTemp("", tempGlob)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: tempfile: %v\n", err)
			os.Exit(1)
		}
		tmpPath := tmp.Name()
		tmp.Close()
		defer os.Remove(tmpPath)

		runArgs = append(runArgs, "--out", tmpPath)
		if *verbose {
			printDeriveCommand(runDir, runCmd, runArgs)
		}
		if err := runInDir(runDir, runCmd, runArgs...); err != nil {
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

	// Second pass: run tests on the regenerated files.
	// Go specs → `go test ./impl-pkg/...` per distinct package.
	goPkgs := make([]string, 0, len(goImplPkgs))
	for p := range goImplPkgs {
		goPkgs = append(goPkgs, p+"/...")
	}
	sort.Strings(goPkgs)
	for _, p := range goPkgs {
		if *verbose {
			printDeriveCommand("", "go", []string{"test", p})
		}
		if err := runInDir("", "go", "test", p); err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: go test %s failed: %v\n", p, err)
			os.Exit(1)
		}
	}
	// TS specs → `node --import tsx --test <outfile>` per generated file.
	sort.Strings(tsTestFiles)
	for _, f := range tsTestFiles {
		tsArgs := []string{"--import", "tsx", "--test", f}
		if *verbose {
			printDeriveCommand("", "node", tsArgs)
		}
		if err := runInDir("", "node", tsArgs...); err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: node --test %s failed: %v\n", f, err)
			os.Exit(1)
		}
	}
}

// printDeriveCommand prints a shell-pastable representation of the
// command sb derive is about to run, including the working directory
// when it is non-empty.
func printDeriveCommand(dir, name string, args []string) {
	if dir != "" {
		fmt.Fprintf(os.Stderr, "+ (cd %s &&", dir)
	} else {
		fmt.Fprintf(os.Stderr, "+")
	}
	fmt.Fprintf(os.Stderr, " %s", name)
	for _, a := range args {
		fmt.Fprintf(os.Stderr, " %s", a)
	}
	if dir != "" {
		fmt.Fprintln(os.Stderr, ")")
	} else {
		fmt.Fprintln(os.Stderr)
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
