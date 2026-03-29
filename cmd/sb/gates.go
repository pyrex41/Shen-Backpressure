package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type gate struct {
	name string
	cmd  string
	args []string
}

type gateResult struct {
	name    string
	passed  bool
	output  string
	elapsed time.Duration
}

func cmdGates(args []string) {
	fs := flag.NewFlagSet("gates", flag.ExitOnError)
	relaxed := fs.Bool("relaxed", false, "run test and build gates in parallel")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb gates — Run all five verification gates

Usage: sb gates [flags]

Runs the five-gate verification pipeline using the commands from sb.toml
or convention defaults:

  Gate 1: shengen       Regenerate guard types from spec
  Gate 2: test          Run tests against regenerated types
  Gate 3: build         Compile against regenerated types
  Gate 4: shen-check    Verify spec internal consistency
  Gate 5: tcb-audit     Verify shenguard package integrity

Each gate command is configurable via sb.toml [commands] or shell scripts
in bin/. The LLM adapts these per project via the skills.

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

	var results []gateResult
	if *relaxed {
		results = runGatesRelaxed(gates)
	} else {
		results = runGatesStrict(gates)
	}

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
	genBin, genArgs := SplitCommand(cfg.Gen)
	testBin, testArgs := SplitCommand(cfg.Test)
	buildBin, buildArgs := SplitCommand(cfg.Build)
	checkBin, checkArgs := SplitCommand(cfg.Check)
	auditBin, auditArgs := SplitCommand(cfg.Audit)

	return []gate{
		{name: "shengen", cmd: genBin, args: genArgs},
		{name: "test", cmd: testBin, args: testArgs},
		{name: "build", cmd: buildBin, args: buildArgs},
		{name: "shen-check", cmd: checkBin, args: checkArgs},
		{name: "tcb-audit", cmd: auditBin, args: auditArgs},
	}
}

func runOneGate(g gate) gateResult {
	start := time.Now()
	cmd := exec.Command(g.cmd, g.args...)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	return gateResult{
		name:    g.name,
		passed:  err == nil,
		output:  string(out),
		elapsed: elapsed,
	}
}

func runGatesStrict(gates []gate) []gateResult {
	results := make([]gateResult, 0, len(gates))
	for _, g := range gates {
		r := runOneGate(g)
		logGate(r)
		results = append(results, r)
	}
	return results
}

func runGatesRelaxed(gates []gate) []gateResult {
	results := make([]gateResult, len(gates))

	// Gate 0 (shengen) runs first — generates types needed by test and build
	results[0] = runOneGate(gates[0])
	logGate(results[0])

	// Gates 1 and 2 (test + build) run in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 1; i <= 2 && i < len(gates); i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := runOneGate(gates[idx])
			mu.Lock()
			results[idx] = r
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	for i := 1; i <= 2 && i < len(gates); i++ {
		logGate(results[i])
	}

	// Gates 3+ (shen-check, tcb-audit) run sequentially
	for i := 3; i < len(gates); i++ {
		results[i] = runOneGate(gates[i])
		logGate(results[i])
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
