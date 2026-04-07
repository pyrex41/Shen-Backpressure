package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	defaultHarness = "claude -p"
	defaultMaxIter = 10
	promptPath     = "prompts/main_prompt.md"
	planPath       = "plans/fix_plan.md"
	bpLogPath      = "plans/backpressure.log"
	bpMarker       = "## Backpressure Errors (from previous iteration)"
)

type gate struct {
	name string
	cmd  string
	args []string
}

type gateResult struct {
	name   string
	passed bool
	output string
}

var gates = []gate{
	{name: "shengen", cmd: "./bin/shengen-codegen.sh"},
	{name: "go-test", cmd: "go", args: []string{"test", "./..."}},
	{name: "go-build", cmd: "go", args: []string{"build", "./cmd/server"}},
	{name: "shen-typecheck", cmd: "./bin/shen-check.sh"},
}

func logf(format string, args ...any) {
	ts := time.Now().Format("15:04:05")
	fmt.Printf("%s [ralph] %s\n", ts, fmt.Sprintf(format, args...))
}

func validateTooling() error {
	type check struct {
		label string
		ok    bool
	}
	checks := []check{
		{"go", func() bool { _, err := exec.LookPath("go"); return err == nil }()},
		{"specs", func() bool { _, err := os.Stat("specs/core.shen"); return err == nil }()},
		{"shengen", func() bool { _, err := os.Stat("bin/shengen"); return err == nil }()},
		{"shen-check", func() bool { _, err := os.Stat("bin/shen-check.sh"); return err == nil }()},
	}

	parts := make([]string, len(checks))
	allOK := true
	for i, c := range checks {
		status := "OK"
		if !c.ok {
			status = "MISSING"
			allOK = false
		}
		parts[i] = fmt.Sprintf("%s=%s", c.label, status)
	}
	logf("Tooling validated: %s", strings.Join(parts, ", "))

	if !allOK {
		return fmt.Errorf("missing required tooling")
	}
	return nil
}

func buildPrompt(prevErrors []gateResult) (string, error) {
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("reading prompt: %w", err)
	}
	prompt := string(data)

	if len(prevErrors) == 0 {
		return prompt, nil
	}

	var errSection strings.Builder
	for _, r := range prevErrors {
		if !r.passed {
			fmt.Fprintf(&errSection, "\n### FAIL [%s]\n```\n%s\n```\n", r.name, strings.TrimSpace(r.output))
		}
	}

	if idx := strings.Index(prompt, bpMarker); idx != -1 {
		insertAt := idx + len(bpMarker)
		prompt = prompt[:insertAt] + "\n" + errSection.String() + prompt[insertAt:]
	} else {
		prompt += "\n" + bpMarker + "\n" + errSection.String()
	}

	return prompt, nil
}

func callLLM(harness, prompt string) error {
	cmd := exec.Command("sh", "-c", harness)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runOneGate(g gate) gateResult {
	cmd := exec.Command(g.cmd, g.args...)
	out, err := cmd.CombinedOutput()
	return gateResult{
		name:   g.name,
		passed: err == nil,
		output: string(out),
	}
}

func runGatesStrict() []gateResult {
	results := make([]gateResult, 0, len(gates))
	for _, g := range gates {
		r := runOneGate(g)
		status := "PASS"
		if !r.passed {
			status = "FAIL"
		}
		logf("%s [%s]", status, r.name)
		results = append(results, r)
	}
	return results
}

func runGatesRelaxed() []gateResult {
	results := make([]gateResult, len(gates))

	// Gate 0 (shengen) runs first — generates types needed by test and build
	results[0] = runOneGate(gates[0])
	status := "PASS"
	if !results[0].passed {
		status = "FAIL"
	}
	logf("%s [%s]", status, results[0].name)

	// Gates 1 and 2 (test + build) run in parallel
	var mu sync.Mutex
	var eg errgroup.Group
	for i := 1; i <= 2; i++ {
		i := i
		eg.Go(func() error {
			r := runOneGate(gates[i])
			mu.Lock()
			results[i] = r
			mu.Unlock()
			return nil
		})
	}
	eg.Wait()

	for i := 1; i <= 2; i++ {
		s := "PASS"
		if !results[i].passed {
			s = "FAIL"
		}
		logf("%s [%s]", s, results[i].name)
	}

	// Gate 3 (shen typecheck) runs last
	results[3] = runOneGate(gates[3])
	s := "PASS"
	if !results[3].passed {
		s = "FAIL"
	}
	logf("%s [%s]", s, results[3].name)

	return results
}

func allPassed(results []gateResult) bool {
	for _, r := range results {
		if !r.passed {
			return false
		}
	}
	return true
}

func failedGates(results []gateResult) []gateResult {
	var failed []gateResult
	for _, r := range results {
		if !r.passed {
			failed = append(failed, r)
		}
	}
	return failed
}

func appendBackpressureLog(results []gateResult) {
	f, err := os.OpenFile(bpLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logf("WARNING: could not open backpressure log: %v", err)
		return
	}
	defer f.Close()

	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "--- %s ---\n", ts)
	for _, r := range results {
		if !r.passed {
			fmt.Fprintf(f, "FAIL [%s]\n%s\n\n", r.name, strings.TrimSpace(r.output))
		}
	}
}

func hasPlanItems() bool {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "- [ ]") {
			return true
		}
	}
	return false
}

func main() {
	relaxed := flag.Bool("relaxed", false, "run go-test and go-build gates in parallel")
	flag.Parse()

	demo := os.Getenv("RALPH_DEMO") == "1"
	harness := os.Getenv("RALPH_HARNESS")
	if harness == "" {
		harness = defaultHarness
	}
	maxIter := defaultMaxIter
	if v := os.Getenv("RALPH_MAX_ITER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxIter = n
		}
	}

	mode := "strict"
	if *relaxed {
		mode = "relaxed"
	}
	logf("Starting Ralph-Shen loop (mode=%s)", mode)

	if err := validateTooling(); err != nil {
		logf("ERROR: %v", err)
		os.Exit(1)
	}

	var prevErrors []gateResult

	for i := 1; i <= maxIter; i++ {
		logf("=== Iteration %d ===", i)

		prompt, err := buildPrompt(prevErrors)
		if err != nil {
			logf("ERROR: %v", err)
			os.Exit(1)
		}

		if !demo {
			if err := callLLM(harness, prompt); err != nil {
				logf("WARNING: LLM harness returned error: %v", err)
			}
		}

		var results []gateResult
		if *relaxed {
			results = runGatesRelaxed()
		} else {
			results = runGatesStrict()
		}

		if allPassed(results) {
			logf("All gates passed on iteration %d", i)
			if len(prevErrors) == 0 && !hasPlanItems() {
				logf("Plan complete. Exiting.")
				os.Exit(0)
			}
			prevErrors = nil
			continue
		}

		prevErrors = failedGates(results)
		appendBackpressureLog(results)
		logf("Backpressure: %d gate(s) failed, feeding errors into next iteration", len(prevErrors))
	}

	if len(prevErrors) > 0 {
		logf("Max iterations (%d) reached with outstanding failures", maxIter)
		os.Exit(1)
	}
	logf("Loop completed after %d iterations", maxIter)
}
