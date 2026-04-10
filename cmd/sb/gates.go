package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type gate struct {
	name          string
	kind          GateKind
	cmd           string
	args          []string
	parallelGroup string
}

type gateResult struct {
	name     string
	kind     GateKind
	passed   bool
	exitCode int
	stdout   string
	stderr   string
	output   string // combined, kept for backward compat
	elapsed  time.Duration
}

func cmdGates(args []string) {
	fs := flag.NewFlagSet("gates", flag.ExitOnError)
	relaxed := fs.Bool("relaxed", false, "run test and build gates in parallel")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb gates — Run manifest-defined verification gates

Usage: sb gates [flags]

Runs the verification gate pipeline defined in sb.toml. Gates are
either defined explicitly via [[gates]] array-of-tables, or synthesised
from the legacy [commands] section:

  Gate 1: shengen       Regenerate guard types from spec
  Gate 2: test          Run tests against regenerated types
  Gate 3: build         Compile against regenerated types
  Gate 4: shen-check    Verify spec internal consistency
  Gate 5: tcb-audit     Verify shenguard package integrity

Gates with the same parallel_group run concurrently; all others run
sequentially.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb gates: %v\n", err)
		os.Exit(1)
	}

	if *relaxed || cfg.Relaxed {
		*relaxed = true
	}

	gates := buildGateList(cfg)

	// In legacy mode, -relaxed flag sets parallel groups on test+build.
	if *relaxed && !cfg.HasManifestGates() {
		for i := range gates {
			if gates[i].name == "test" || gates[i].name == "build" {
				gates[i].parallelGroup = "build-test"
			}
		}
	}

	results := runGateList(gates)

	// Summary
	fmt.Fprintln(os.Stderr)
	passed := 0
	for _, r := range results {
		status := "\033[32mPASS\033[0m"
		if !r.passed {
			status = "\033[31mFAIL\033[0m"
		} else {
			passed++
		}
		fmt.Fprintf(os.Stderr, "  %s  %-14s %s\n", status, r.name, r.elapsed.Round(time.Millisecond))
	}
	fmt.Fprintf(os.Stderr, "\n%d/%d gates passed\n", passed, len(results))

	if passed < len(results) {
		for _, r := range results {
			if !r.passed && r.output != "" {
				fmt.Fprintf(os.Stderr, "\n--- FAIL [%s] ---\n%s\n", r.name, strings.TrimSpace(r.output))
			}
		}
		os.Exit(1)
	}
}

func buildGateList(cfg *Config) []gate {
	var gates []gate

	if cfg.Gates != nil {
		// Manifest-defined gates: convert each GateDef to a gate.
		for _, gd := range cfg.Gates {
			bin, args := SplitCommand(gd.Run)
			gates = append(gates, gate{
				name:          gd.Name,
				kind:          gd.Kind,
				cmd:           bin,
				args:          args,
				parallelGroup: gd.ParallelGroup,
			})
		}
	} else {
		// Legacy mode: synthesise five gates from Gen/Build/Test/Check/Audit.
		genBin, genArgs := SplitCommand(cfg.Gen)
		testBin, testArgs := SplitCommand(cfg.Test)
		buildBin, buildArgs := SplitCommand(cfg.Build)
		checkBin, checkArgs := SplitCommand(cfg.Check)
		auditBin, auditArgs := SplitCommand(cfg.Audit)

		testGroup := ""
		buildGroup := ""
		if cfg.Relaxed {
			testGroup = "build-test"
			buildGroup = "build-test"
		}

		gates = []gate{
			{name: "shengen", kind: GateKindCommand, cmd: genBin, args: genArgs},
			{name: "test", kind: GateKindCommand, cmd: testBin, args: testArgs, parallelGroup: testGroup},
			{name: "build", kind: GateKindCommand, cmd: buildBin, args: buildArgs, parallelGroup: buildGroup},
			{name: "shen-check", kind: GateKindCommand, cmd: checkBin, args: checkArgs},
			{name: "tcb-audit", kind: GateKindCommand, cmd: auditBin, args: auditArgs},
		}
	}

	// Always append shen-derive gate if configured, regardless of format.
	if len(cfg.DeriveSpecs) > 0 {
		if self, err := os.Executable(); err == nil {
			gates = append(gates, gate{
				name: "shen-derive",
				kind: GateKindDerive,
				cmd:  self,
				args: []string{"derive"},
			})
		}
	}

	return gates
}

func runOneGate(g gate) gateResult {
	start := time.Now()
	cmd := exec.Command(g.cmd, g.args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	elapsed := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	return gateResult{
		name:     g.name,
		kind:     g.kind,
		passed:   err == nil,
		exitCode: exitCode,
		stdout:   stdout,
		stderr:   stderr,
		output:   stdout + stderr,
		elapsed:  elapsed,
	}
}

// runGateList executes gates in order. Consecutive gates sharing the same
// non-empty parallelGroup run concurrently; all others run sequentially.
func runGateList(gates []gate) []gateResult {
	results := make([]gateResult, len(gates))
	i := 0
	for i < len(gates) {
		pg := gates[i].parallelGroup
		if pg == "" {
			// Sequential gate.
			results[i] = runOneGate(gates[i])
			logGate(results[i])
			i++
			continue
		}
		// Collect consecutive gates with the same parallelGroup.
		j := i
		for j < len(gates) && gates[j].parallelGroup == pg {
			j++
		}
		// Run gates[i:j] concurrently.
		var wg sync.WaitGroup
		for idx := i; idx < j; idx++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				results[k] = runOneGate(gates[k])
			}(idx)
		}
		wg.Wait()
		for idx := i; idx < j; idx++ {
			logGate(results[idx])
		}
		i = j
	}
	return results
}

func logGate(r gateResult) {
	status := "\033[32mPASS\033[0m"
	if !r.passed {
		status = "\033[31mFAIL\033[0m"
	}
	fmt.Fprintf(os.Stderr, "%s [%s] %s\n", status, r.name, r.elapsed.Round(time.Millisecond))
}
