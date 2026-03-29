package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func cmdCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb check — Gate 4: Verify Shen spec consistency

Usage: sb check [flags]

Runs Shen's type checker (tc+) on the spec file to verify internal consistency.
Requires shen-sbcl to be installed.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb check: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Gate 4: shen tc+ — verifying spec consistency")

	if _, err := os.Stat(cfg.Spec); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: spec file not found at %s\n", cfg.Spec)
		os.Exit(1)
	}

	bin, cmdArgs := SplitCommand(cfg.Check)
	if len(cmdArgs) == 0 {
		// Default: pass spec as first arg
		cmdArgs = []string{cfg.Spec}
	}

	cmd := exec.Command(bin, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: shen tc+ failed\n")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "PASS: spec is internally consistent")
}
