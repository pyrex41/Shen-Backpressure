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
	"time"
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
	// Per-spec discharge reports accumulated during the first pass;
	// after `go test` runs we patch counter-examples in and emit the
	// final aggregated project report.
	var partialReports []*DischargeReport
	// Temp report files we asked shen-derive to write — cleaned up
	// at the end of this command.
	var reportTempFiles []string
	defer func() {
		for _, p := range reportTempFiles {
			os.Remove(p)
		}
	}()

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
		// Per-spec discharge report tempfile. Aggregated into
		// .sb/discharge_report.json after `go test` runs.
		reportTmp, err := os.CreateTemp("", "sb-discharge-*.json")
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb derive: report tempfile: %v\n", err)
			os.Exit(1)
		}
		reportTmpPath := reportTmp.Name()
		reportTmp.Close()
		reportTempFiles = append(reportTempFiles, reportTmpPath)

		// Resolve guard file (impl-relative) so shen-derive can write
		// code_references into the report. Best-effort: if the file
		// is missing the report just omits code_references.
		absGuardFile := ""
		if cfg.Output != "" {
			absGuardFile, _ = filepath.Abs(cfg.Output)
		}

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
				"--report-out", reportTmpPath,
			}
			if absGuardFile != "" {
				runArgs = append(runArgs, "--guard-file", absGuardFile)
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
			if pr, err := loadDischarge(reportTmpPath); err == nil && pr != nil {
				normaliseDischargePaths(pr, spec)
				partialReports = append(partialReports, pr)
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

		// Pick up the per-spec discharge report if shen-derive
		// produced one. TS specs don't write reports yet — that's
		// fine, the aggregated report just won't cover them.
		if pr, err := loadDischarge(reportTmpPath); err == nil && pr != nil {
			normaliseDischargePaths(pr, spec)
			partialReports = append(partialReports, pr)
		}
	}

	if drifted > 0 {
		fmt.Fprintf(os.Stderr, "sb derive: %d file(s) stale. Run `sb derive --regen` to update.\n", drifted)
		os.Exit(1)
	}

	// In --regen and --skip-test modes the spec ≡ impl oracle does
	// not actually run. We still emit a discharge report so users
	// can inspect the structural classification, but runtime-sampled
	// premises are downgraded to "unproven" (with an explicit
	// rationale) so the report cannot claim sampled equivalence
	// passed when zero samples were checked.
	if *regen {
		if len(partialReports) > 0 {
			finalizeDischargeReport(partialReports, cfg, nil, false, "report generated with --regen; spec ≡ impl oracle did not run")
		}
		return
	}
	if *skipTest {
		if len(partialReports) > 0 {
			finalizeDischargeReport(partialReports, cfg, nil, false, "report generated with --skip-test; spec ≡ impl oracle did not run")
		}
		return
	}

	// Second pass: run tests on the regenerated files. We capture
	// combined stdout/stderr so failures can be parsed into
	// counter-examples for the discharge report.
	goPkgs := make([]string, 0, len(goImplPkgs))
	for p := range goImplPkgs {
		goPkgs = append(goPkgs, p+"/...")
	}
	sort.Strings(goPkgs)
	var combinedTestOutput bytes.Buffer
	testFailed := false
	for _, p := range goPkgs {
		if *verbose {
			printDeriveCommand("", "go", []string{"test", "-v", p})
		}
		out, err := runCaptured("", "go", "test", "-v", p)
		combinedTestOutput.Write(out)
		// Echo to user so they see the same output as before.
		os.Stderr.Write(out)
		if err != nil {
			testFailed = true
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
			testFailed = true
		}
	}

	// Emit the project-level discharge report whether tests passed
	// or failed; the report's whole point is to surface failures
	// alongside successes.
	if len(partialReports) > 0 {
		failures := parseGoTestFailures(combinedTestOutput.String())
		finalizeDischargeReport(partialReports, cfg, failures, true, "")
	}

	if testFailed {
		os.Exit(1)
	}
}

// finalizeDischargeReport merges per-spec partial reports, fills in
// project-level metadata (git commit, history-derived
// `discharged_since_commit`), patches in counter-examples parsed from
// `go test` output, writes the canonical .sb/discharge_report.json,
// and rotates a copy into .sb/history/.
//
// testsRan indicates whether the spec ≡ impl oracle actually
// executed. When false (--regen / --skip-test), runtime-sampled
// premises are downgraded to "unproven" with the supplied rationale
// so the artifact never claims sampled equivalence passed when no
// samples were checked.
func finalizeDischargeReport(parts []*DischargeReport, cfg *Config, failures []parsedFailure, testsRan bool, skipReason string) {
	r := mergeDischargeReports(parts)
	r.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if r.Tools.SBVersion == "" {
		r.Tools.SBVersion = version
	}
	fillImplGit(r)
	if !testsRan {
		downgradeRuntimeSampledToUnproven(r, skipReason)
	}
	if len(failures) > 0 {
		applyCounterExamples(r, failures, cfg.DeriveSpecs)
	}
	// W3.2 — populate shen_runtime{,_available} when at least one spec
	// referenced by [[derive.specs]] uses a `:runtime-via` annotation.
	// The detection is purely textual (scans the spec file for the
	// Shen-comment marker), keeping this additive: projects with no
	// runtime-via continue to report `shen_runtime: null` exactly as
	// before. See thoughts/shared/research/2026-05-22-schema-v1-additions.md.
	if name := detectShenRuntime(cfg); name != "" {
		r.Tools.ShenRuntime = &name
		r.Tools.ShenRuntimeAvailable = true
	}
	computeDischargedSinceCommit(r)
	if err := writeDischarge(DischargeReportPath, r); err != nil {
		fmt.Fprintf(os.Stderr, "sb derive: write discharge report: %v\n", err)
		return
	}
	if err := rotateDischargeHistory(r, DischargeReportPath); err != nil {
		fmt.Fprintf(os.Stderr, "sb derive: rotate discharge history: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "sb derive: discharge report → %s (%d/%d rules discharged)\n",
		DischargeReportPath, r.Summary.RulesDischarged, r.Summary.RuleCount)
}

// detectShenRuntime scans every spec referenced by [[derive.specs]] for
// the `:runtime-via <name>` Shen-comment marker. If at least one
// occurrence is found, the returned string is the Shen runtime the
// project is composed with — currently always "shen-sbcl" because the
// only live-runtime backend wired up is shen-web-tools' SBCL host.
// Returns "" when no runtime-via annotation is present (the pure-static
// case, which is the vast majority of projects).
//
// The detection is intentionally textual to keep this function from
// pulling shengen's parser in. The marker's grammar is documented in
// docs/RUNTIME-VIA.md and pinned by cmd/shengen{,-ts} tests.
func detectShenRuntime(cfg *Config) string {
	if cfg == nil {
		return ""
	}
	seen := map[string]bool{}
	for _, s := range cfg.DeriveSpecs {
		if s.Path == "" || seen[s.Path] {
			continue
		}
		seen[s.Path] = true
		data, err := os.ReadFile(s.Path)
		if err != nil {
			continue
		}
		if bytes.Contains(data, []byte(":runtime-via ")) ||
			bytes.Contains(data, []byte("\\* :runtime-via")) {
			// "shen-sbcl" is the canonical name for the runtime the
			// only live-host demo uses today. When other hosts are
			// added (shen-cl, shen-scheme), this would be reported
			// via per-spec metadata; for the v1 prototype the name
			// is fixed.
			return "shen-sbcl"
		}
	}
	return ""
}

// runCaptured runs a command and returns its combined stdout/stderr
// alongside the run error. The error mirrors exec.Cmd.Run's
// semantics.
func runCaptured(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
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
