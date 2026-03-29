package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdAudit(args []string) {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb audit — Gate 5: Verify shenguard package integrity

Usage: sb audit [flags]

Re-runs shengen and diffs output against the committed guards_gen.go.
Also checks for unexpected (hand-written) files in the shenguard package.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb audit: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Gate 5: TCB Audit — verifying shenguard package integrity")

	guardDir := filepath.Dir(cfg.Output)

	// Step 1: Check for unexpected files in shenguard directory
	if err := checkUnexpectedFiles(guardDir); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Check spec and output exist
	if _, err := os.Stat(cfg.Spec); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: spec file not found at %s\n", cfg.Spec)
		os.Exit(1)
	}
	if _, err := os.Stat(cfg.Output); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: generated file not found at %s\n", cfg.Output)
		os.Exit(1)
	}

	// Step 3: Regenerate to temp file and diff
	if err := diffRegenerated(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "PASS: shenguard package contains only generated code, output matches shengen")
}

func checkUnexpectedFiles(guardDir string) error {
	entries, err := os.ReadDir(guardDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // directory doesn't exist yet, nothing to check
		}
		return fmt.Errorf("reading %s: %w", guardDir, err)
	}

	allowed := map[string]bool{
		"guards_gen.go":    true,
		"db_scoped_gen.go": true,
	}

	var unexpected []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if !allowed[name] {
			unexpected = append(unexpected, name)
		}
	}

	if len(unexpected) > 0 {
		return fmt.Errorf("unexpected files in shenguard package: %s\nThe shenguard package must contain ONLY generated code.\nMove hand-written code to a separate package.", strings.Join(unexpected, " "))
	}
	return nil
}

func diffRegenerated(cfg *Config) error {
	shengen, err := FindShengen()
	if err != nil {
		return err
	}

	// Generate to temp file
	tmpFile, err := os.CreateTemp("", "shenguard-audit-*.go")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command(shengen, "--spec", cfg.Spec, "--pkg", cfg.Pkg, "--out", tmpPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("regenerating with shengen: %w", err)
	}

	// Diff
	diffCmd := exec.Command("diff", "-q", cfg.Output, tmpPath)
	if err := diffCmd.Run(); err != nil {
		// Files differ — show the diff
		detailCmd := exec.Command("diff", "-u", cfg.Output, tmpPath)
		detail, _ := detailCmd.Output()
		lines := strings.Split(string(detail), "\n")
		if len(lines) > 40 {
			lines = lines[:40]
		}

		return fmt.Errorf("%s does not match shengen output\n\nEither the spec changed without regenerating, or the file was manually edited.\nDiff:\n%s\n\nFix: sb gen", cfg.Output, strings.Join(lines, "\n"))
	}

	return nil
}
