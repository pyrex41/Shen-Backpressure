package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func cmdLoop(args []string) {
	fs := flag.NewFlagSet("loop", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print the loop script without running it")
	falsify := fs.Bool("falsify", false, "after a passing iteration, run a second phase that looks for one forgery or one disagreeing input")
	falsifyOnly := fs.Bool("falsify-only", false, "run just the falsify phase once against the current tree, without looping")
	falsifyPrompt := fs.Bool("falsify-prompt", false, "print the hydrated falsifier prompt and exit; calls no harness")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb loop — Launch a Ralph loop

Usage: sb loop [flags]

Runs a headless LLM harness in a loop with five-gate verification.
Each iteration: run gates → if fail, inject errors into prompt → call
harness → repeat. Stops when all gates pass or max iterations reached.

With --falsify, a passing iteration is not the end. A second phase runs
the inverse of the main prompt: given the spec, the generated guards,
the forgery corpus and the mutation survivors, find ONE program that
obtains a guard value without its constructor, or ONE input where the
spec and the implementation disagree. Findings are written to the
forgery corpus (with a declared expectation the gate re-checks) or to
` + FalsifierSamplesPath + `, which shen-derive picks up as a fourth
sample source. The phase calls the same harness the main loop does.

The reasoning: when every gate passes, the verifier has returned
exactly one bit, and a loop whose progress is bounded by the verifier's
output has nothing to work with. The falsifier under-approximates —
looking for a witness that something IS wrong — which is the only way
to learn how strong the gates that just passed actually are.

Configuration via sb.toml [loop] or environment variables:
  RALPH_HARNESS          LLM command (default: "claude -p")
  RALPH_MAX_ITER         Max iterations (default: 10)
  RALPH_HARNESS_TIMEOUT  Per-call timeout (default: 10m)

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb loop: %v\n", err)
		os.Exit(1)
	}

	// The falsify-only paths do not need the main loop's prompt and
	// plan files, so they are handled before the prerequisite check.
	if *falsifyPrompt {
		tmplSrc, src, err := loadFalsifierTemplate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb loop: %v\n", err)
			os.Exit(1)
		}
		prompt, err := BuildFalsifierPrompt(tmplSrc, cfg, loadMutationScore())
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb loop: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "sb loop: falsifier prompt from %s\n", src)
		fmt.Print(prompt)
		return
	}
	if *falsifyOnly {
		runFalsifyPhase(cfg, 0)
		return
	}

	// Verify prerequisites
	for _, path := range []string{cfg.Spec, cfg.Prompt, cfg.Plan} {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "sb loop: missing %s — run 'sb init' first or create it\n", path)
			os.Exit(1)
		}
	}

	if *dryRun {
		fmt.Print(buildLoopScript(cfg))
		return
	}

	runLoop(cfg, *falsify)
}

func runLoop(cfg *Config, falsify bool) {
	fmt.Fprintf(os.Stderr, "Ralph loop: harness=%q max_iter=%d timeout=%s\n",
		cfg.Harness, cfg.MaxIter, cfg.HarnessTimeout)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n\n")

	backpressureLog := "plans/backpressure.log"

	for i := 1; i <= cfg.MaxIter; i++ {
		fmt.Fprintf(os.Stderr, "=== Iteration %d/%d ===\n", i, cfg.MaxIter)

		// Run gates first to check current state
		gateOutput, gatesPassed := runGatesForLoop(cfg)

		if gatesPassed {
			fmt.Fprintf(os.Stderr, "\nAll gates passed on iteration %d.\n", i)
			if !falsify {
				fmt.Fprintln(os.Stderr, "Done.")
				return
			}
			// A passing iteration is the point at which the verifier
			// returns its fewest bits — one, "green" — so it is
			// exactly when the falsifier is worth running. What it
			// finds becomes a permanent gate; what it fails to find is
			// weak evidence that the gates are doing their job.
			f := runFalsifyPhase(cfg, i)
			if !f.Any() {
				fmt.Fprintln(os.Stderr, "Done.")
				return
			}
			fmt.Fprintln(os.Stderr, "\nsb loop: the falsifier produced new material; continuing so the next iteration verifies it.")
			continue
		}

		// Write gate failures to backpressure log
		os.MkdirAll("plans", 0755)
		os.WriteFile(backpressureLog, []byte(fmt.Sprintf("--- Iteration %d ---\n%s\n", i, gateOutput)), 0644)

		// Build live project context (non-fatal on error)
		ctx, ctxErr := BuildContext(cfg)
		if ctxErr != nil {
			fmt.Fprintf(os.Stderr, "sb loop: context generation failed: %v (continuing without live context)\n", ctxErr)
			ctx = nil
		}

		// Build the prompt with live context and backpressure errors injected
		prompt := buildHarnessPrompt(cfg, ctx, gateOutput)

		// Call the harness
		fmt.Fprintf(os.Stderr, "\nCalling harness: %s\n", cfg.Harness)
		if err := callHarness(cfg, prompt); err != nil {
			fmt.Fprintf(os.Stderr, "Harness error: %v\n", err)
		}

		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprintf(os.Stderr, "Ralph loop: max iterations (%d) reached without all gates passing.\n", cfg.MaxIter)
	os.Exit(1)
}

func runGatesForLoop(cfg *Config) (string, bool) {
	gates := buildGateList(cfg)
	var output strings.Builder
	allPassed := true

	for _, g := range gates {
		r := runOneGate(g)
		logGate(r)
		if !r.passed {
			allPassed = false
			output.WriteString(fmt.Sprintf("FAIL [%s]\n%s\n", r.name, strings.TrimSpace(r.output)))
		}
	}

	return output.String(), allPassed
}

func buildHarnessPrompt(cfg *Config, ctx *ProjectContext, gateErrors string) string {
	// Read the main prompt file
	promptContent, err := os.ReadFile(cfg.Prompt)
	if err != nil {
		return gateErrors
	}

	// Read the plan
	planContent, _ := os.ReadFile(cfg.Plan)

	var prompt strings.Builder
	prompt.Write(promptContent)

	if ctx != nil {
		prompt.WriteString("\n\n## Live Project Context\n\n")
		prompt.WriteString(ctx.RenderMarkdown())
	}

	if len(planContent) > 0 {
		prompt.WriteString("\n\n## Current Plan\n\n")
		prompt.Write(planContent)
	}

	if gateErrors != "" {
		prompt.WriteString("\n\n## Backpressure Errors (fix these FIRST)\n\n```\n")
		prompt.WriteString(gateErrors)
		prompt.WriteString("```\n")
	}

	return prompt.String()
}

func callHarness(cfg *Config, prompt string) error {
	bin, args := SplitCommand(cfg.Harness)

	// Apply timeout using Go's context (works on all platforms, unlike GNU timeout)
	ctx := context.Background()
	if cfg.HarnessTimeout != "" {
		if dur, err := time.ParseDuration(cfg.HarnessTimeout); err == nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, dur)
			defer cancel()
		}
	}

	// Pipe prompt via stdin — this is how claude -p and most headless LLM tools work.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(prompt)

	return cmd.Run()
}

func buildLoopScript(cfg *Config) string {
	bt := "`" // backtick — can't embed in Go raw strings
	return fmt.Sprintf("#!/bin/bash\n"+
		"# Ralph loop — generated by sb loop --dry-run\n"+
		"# Runs headless LLM with five-gate Shen backpressure verification\n"+
		"set -euo pipefail\n\n"+
		"HARNESS=\"${RALPH_HARNESS:-%s}\"\n"+
		"MAX_ITER=\"${RALPH_MAX_ITER:-%d}\"\n"+
		"TIMEOUT=\"${RALPH_HARNESS_TIMEOUT:-%s}\"\n"+
		"PROMPT=\"%s\"\n"+
		"PLAN=\"%s\"\n"+
		"BACKPRESSURE_LOG=\"plans/backpressure.log\"\n\n"+
		"for i in $(seq 1 \"$MAX_ITER\"); do\n"+
		"  echo \"=== Iteration $i/$MAX_ITER ===\"\n\n"+
		"  # Run five gates\n"+
		"  if sb gates 2>&1 | tee /tmp/ralph-gates.log; then\n"+
		"    echo \"All gates passed on iteration $i. Done.\"\n"+
		"    exit 0\n"+
		"  fi\n\n"+
		"  # Extract failures for backpressure injection\n"+
		"  grep -E \"^FAIL|^---\" /tmp/ralph-gates.log > \"$BACKPRESSURE_LOG\" 2>/dev/null || true\n\n"+
		"  # Capture live project context (non-fatal if unavailable)\n"+
		"  LIVE_CONTEXT=\"$(sb context --format markdown 2>/dev/null || echo '(context unavailable)')\"\n\n"+
		"  # Build prompt with live context and backpressure errors\n"+
		"  FULL_PROMPT=\"$(cat \"$PROMPT\")\n\n"+
		"## Live Project Context\n\n"+
		"$LIVE_CONTEXT\n\n"+
		"## Current Plan\n\n"+
		"$(cat \"$PLAN\" 2>/dev/null || echo '(no plan)')\n\n"+
		"## Backpressure Errors (fix these FIRST)\n\n"+
		"%s%s%s\n"+
		"$(cat \"$BACKPRESSURE_LOG\" 2>/dev/null || echo 'none')\n"+
		"%s%s%s\"\n\n"+
		"  # Call harness with timeout\n"+
		"  echo \"Calling harness: $HARNESS\"\n"+
		"  timeout \"$TIMEOUT\" $HARNESS <<< \"$FULL_PROMPT\" || echo \"Harness exited with $?\"\n\n"+
		"  echo\n"+
		"done\n\n"+
		"echo \"Max iterations ($MAX_ITER) reached without all gates passing.\"\n"+
		"exit 1\n",
		cfg.Harness, cfg.MaxIter, cfg.HarnessTimeout, cfg.Prompt, cfg.Plan,
		bt, bt, bt, bt, bt, bt)
}
