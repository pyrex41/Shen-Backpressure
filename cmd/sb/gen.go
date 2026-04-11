package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func cmdGen(args []string) {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	verbose := fs.Bool("verbose", false, "print the shengen command before running it")
	dryRun := fs.Bool("dry-run", false, "print the shengen command without executing it")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb gen — Generate guard types from Shen specs

Usage: sb gen [flags] [spec-file]

Runs shengen to generate guard types from the Shen spec file.
If no spec file is given, uses the configured or conventional path.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb gen: %v\n", err)
		os.Exit(1)
	}

	// Override spec from positional arg
	spec := cfg.Spec
	if fs.NArg() > 0 {
		spec = fs.Arg(0)
	}

	if _, err := os.Stat(spec); err != nil {
		fmt.Fprintf(os.Stderr, "sb gen: spec file not found at %s\n", spec)
		os.Exit(1)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.Output), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "sb gen: creating output dir: %v\n", err)
		os.Exit(1)
	}

	switch cfg.Lang {
	case "go":
		if err := runShengenGo(spec, cfg.Pkg, cfg.Output, cfg.DBWrap, *verbose, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "sb gen: %v\n", err)
			os.Exit(1)
		}
	case "ts":
		if err := runShengenTS(spec, cfg.Output, *verbose, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "sb gen: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "sb gen: unsupported language %q\n", cfg.Lang)
		os.Exit(1)
	}
}

func runShengenGo(spec, pkg, output, dbWrappers string, verbose, dryRun bool) error {
	shengen, err := FindShengen()
	if err != nil {
		return err
	}

	// Run shengen, capture stdout to output file
	args := []string{"--spec", spec, "--pkg", pkg, "--out", output}
	if dbWrappers != "" {
		args = append(args, "--db-wrappers", dbWrappers)
	}

	if verbose || dryRun {
		printCommand(shengen, args)
	}
	if dryRun {
		return nil
	}

	cmd := exec.Command(shengen, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shengen failed: %w", err)
	}

	// shengen --out already prints "Generated ..." to stderr
	return nil
}

func runShengenTS(spec, output string, verbose, dryRun bool) error {
	tsPath, err := FindShengenTS()
	if err != nil {
		return err
	}

	args := []string{"tsx", tsPath, spec, "--out", output}
	if verbose || dryRun {
		printCommand("npx", args)
	}
	if dryRun {
		return nil
	}

	cmd := exec.Command("npx", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shengen-ts failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Generated %s from %s\n", output, spec)
	return nil
}

// printCommand writes the exact command invocation to stderr so users
// can copy-paste it, reproducing the environment sb would use.
func printCommand(name string, args []string) {
	fmt.Fprintf(os.Stderr, "+ %s", name)
	for _, a := range args {
		fmt.Fprintf(os.Stderr, " %s", a)
	}
	fmt.Fprintln(os.Stderr)
}
