---
name: loop
description: Set up and run an autonomous AI coding loop with Shen sequent-calculus type backpressure
---

# Ralph-Shen Typed Loop

You are setting up and running a Ralph-Shen backpressure loop — an autonomous AI coding loop where every iteration must pass three gates before advancing:

1. **`go test ./...`** — Empirical correctness (specific test cases)
2. **`go build ./cmd/ralph`** — Compilation (syntax validity)
3. **`shen (tc +)`** — Formal proof (sequent-calculus types hold for ALL inputs)

If any gate fails, the errors are fed back into the LLM prompt as backpressure.

## Activation

Trigger this skill when the user says: "Ralph loop", "formal verification loop", "type-driven backpressure", "Shen backpressure loop", or asks to set up an autonomous coding loop with formal type checking.

## Workflow

### Step 1: Gather Requirements

Ask the user:
1. **What is your domain?** (e.g., payment processor, inventory system, state machine)
2. **What are your key invariants?** Describe in plain English the properties that must ALWAYS hold. Examples:
   - "Balance can never go negative"
   - "Every reachable state must have at least one valid transition"
   - "A resource can only be freed by its owner"
3. **Which LLM harness?** Options:
   - `claude -p` (Claude Code, default)
   - `cursor-agent -p` (Cursor)
   - `codex -p` (OpenAI Codex CLI)
   - `rho-cli run --prompt` (Rho)
   - Custom command

### Step 2: Generate Shen Type Specifications

Create `specs/core.shen` with sequent-calculus datatypes that formally encode the user's invariants.

Shen datatype rules use this form:
```shen
(datatype rule-name
  premise1;
  premise2;
  premise3;
  ============
  conclusion;)
```

**Template patterns:**

Non-negative value:
```shen
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)
```

Guarded operation (balance check):
```shen
(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)
```

Ownership check:
```shen
(datatype safe-free
  Alloc : allocated;
  Requester : process-id;
  (= (tail Alloc) Requester) : verified;
  =========================================
  [Alloc Requester] : safe-free;)
```

Non-empty constraint:
```shen
(datatype live-state
  S : state;
  Transitions : (list transition);
  (not (= Transitions [])) : verified;
  =====================================
  [S Transitions] : live-state;)
```

### Step 3: Generate the Go Orchestrator

Create `cmd/ralph/main.go` with this template:

```go
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
	{name: "go-test", cmd: "go", args: []string{"test", "./..."}},
	{name: "go-build", cmd: "go", args: []string{"build", "./cmd/ralph"}},
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
		{"shen", func() bool { _, err := os.Stat("bin/shen-check.sh"); return err == nil }()},
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
	return gateResult{name: g.name, passed: err == nil, output: string(out)}
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
	var mu sync.Mutex
	var eg errgroup.Group
	for i := 0; i < 2; i++ {
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
	for i := 0; i < 2; i++ {
		status := "PASS"
		if !results[i].passed {
			status = "FAIL"
		}
		logf("%s [%s]", status, results[i].name)
	}
	results[2] = runOneGate(gates[2])
	status := "PASS"
	if !results[2].passed {
		status = "FAIL"
	}
	logf("%s [%s]", status, results[2].name)
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
```

### Step 4: Generate the Shen Check Wrapper

Create `bin/shen-check.sh` (make it executable with `chmod +x`):

```bash
#!/bin/bash
# Wrapper to run Shen type check and exit cleanly.
# shen-go loops on "empty stream" after EOF instead of exiting,
# so we scan output and kill the process once we have our answer.

SHEN_BIN="${1:-./bin/shen}"
SPEC_FILE="${2:-specs/core.shen}"

if [ ! -f "$SHEN_BIN" ]; then
    echo "ERROR: shen binary not found at $SHEN_BIN"
    exit 1
fi

if [ ! -f "$SPEC_FILE" ]; then
    echo "ERROR: spec file not found at $SPEC_FILE"
    exit 1
fi

TMPOUT=$(mktemp)
printf '(load "%s")\n(tc +)\n' "$SPEC_FILE" | "$SHEN_BIN" > "$TMPOUT" 2>&1 &
SHEN_PID=$!

RESULT="unknown"
for i in $(seq 1 20); do
    sleep 0.5
    if grep -q "type error" "$TMPOUT" 2>/dev/null; then
        RESULT="type_error"
        break
    fi
    if grep -q "true" "$TMPOUT" 2>/dev/null; then
        RESULT="pass"
        break
    fi
    if ! kill -0 "$SHEN_PID" 2>/dev/null; then
        break
    fi
done

kill "$SHEN_PID" 2>/dev/null
wait "$SHEN_PID" 2>/dev/null

grep -v "empty stream" "$TMPOUT" | head -20
rm -f "$TMPOUT"

case "$RESULT" in
    pass)
        echo "RESULT: PASS"
        exit 0
        ;;
    type_error)
        echo "RESULT: FAIL (type error detected)"
        exit 1
        ;;
    *)
        echo "RESULT: FAIL (tc+ did not return true within 10s)"
        exit 1
        ;;
esac
```

### Step 5: Generate Supporting Files

Create `go.mod`:
```
module <project-name>

go 1.24

require golang.org/x/sync v0.12.0
```

Then run `go mod tidy`.

Create `Makefile`:
```makefile
.PHONY: all build test shen-check run run-relaxed demo clean

all: build test shen-check

build:
	go build -o ralph ./cmd/ralph

test:
	go test ./...

shen-check:
	@./bin/shen-check.sh

run: build
	./ralph

run-relaxed: build
	./ralph --relaxed

demo: build
	RALPH_DEMO=1 ./ralph

clean:
	rm -f ralph
	rm -f plans/backpressure.log
```

Create `prompts/main_prompt.md`:
```markdown
You are operating inside a Ralph loop with formal Shen backpressure.

## Context Files (read these every iteration)
- `specs/core.shen` — formal type definitions and sequent rules
- `plans/fix_plan.md` — current plan and progress
- Recent test/type errors (appended below by the orchestrator)

## Your Task
Implement ONE next item from `fix_plan.md` in Go AND strengthen Shen types in `specs/core.shen` so that `(tc +)` still passes.

## Rules
1. Never output placeholders. Full, compilable code only.
2. If you break any Shen proof or Go test, fix it before ending this response.
3. Every new behavior MUST have a corresponding Shen datatype or sequent rule.
4. Keep changes minimal and focused — one logical step per iteration.
5. If a Shen type error appears below, that is your TOP PRIORITY to fix.

## Backpressure Errors (from previous iteration)
<!-- The orchestrator appends errors here automatically -->
```

Create `plans/fix_plan.md` with the user's task list.

### Step 6: Install Shen-Go

If the user doesn't already have the shen binary:

```bash
git clone https://github.com/tiancaiamao/shen-go /tmp/shen-go
cd /tmp/shen-go && GOTOOLCHAIN=local make shen
cp shen <project>/bin/
```

### Step 7: Verify

Run `make all` to confirm all three gates pass. Report the results to the user.

If gates fail, fix the issues before declaring setup complete.

## Key Concepts

- **Backpressure**: When a gate fails, the error output is injected into the next LLM prompt, forcing the LLM to fix the issue before advancing.
- **Sequent calculus**: Shen's type system where rules have premises above a line and a conclusion below. If all premises hold, the conclusion follows.
- **Dual enforcement**: Invariants are stated formally in Shen AND enforced at runtime in Go. Shen catches logical bugs tests miss; tests catch runtime bugs Shen can't see.
