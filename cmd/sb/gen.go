package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdGen(args []string) {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	verbose := fs.Bool("verbose", false, "print the shengen command before running it")
	dryRun := fs.Bool("dry-run", false, "print the shengen command without executing it")
	check := fs.Bool("check", false, "elixir only: do not write; fail if generated files drifted from the spec")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb gen — Generate guard types from Shen specs

Usage: sb gen [flags] [spec-file]

Runs the language-appropriate shengen emitter and writes guards to the
configured output path. The language is taken from sb.toml's
[project] lang field. Supported values:

    go      — full-featured Go emitter (cmd/shengen)
    ts      — full-featured TypeScript emitter (cmd/shengen-ts)
    py      — experimental Python emitter (cmd/shengen-py)
    rs      — experimental Rust emitter (cmd/shengen-rs)
    elixir  — Elixir emitter (cmd/shengen-ex): opaque structs + validating
              new/N, a compile tracer, and optional Ash policy checks
              ([elixir] tracer_out / ash_out / ash_targets in sb.toml)

The Python and Rust emitters cover datatypes, sum types, (list X)
parametric types, and a conservative subset of (define …) blocks.
Unsupported constructs produce explicit "unsupported" comments rather
than silently dropping behaviour.

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

	if *check && cfg.Lang != "elixir" {
		fmt.Fprintln(os.Stderr, "sb gen: --check is only implemented for lang = \"elixir\" (use bin/shenguard-audit.sh)")
		os.Exit(2)
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
	case "py":
		if err := runShengenPy(spec, cfg.Output, *verbose, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "sb gen: %v\n", err)
			os.Exit(1)
		}
	case "rs":
		if err := runShengenRs(spec, cfg.Output, *verbose, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "sb gen: %v\n", err)
			os.Exit(1)
		}
	case "elixir":
		if err := runShengenEx(cfg, spec, *verbose, *dryRun, *check); err != nil {
			fmt.Fprintf(os.Stderr, "sb gen: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "sb gen: unsupported language %q (expected go|ts|py|rs|elixir)\n", cfg.Lang)
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

func runShengenPy(spec, output string, verbose, dryRun bool) error {
	pyPath, err := FindShengenPy()
	if err != nil {
		return err
	}

	args := []string{pyPath, spec, "--out", output}
	if verbose || dryRun {
		printCommand("python3", args)
	}
	if dryRun {
		return nil
	}

	cmd := exec.Command("python3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shengen-py failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Generated %s from %s (experimental py emitter)\n", output, spec)
	return nil
}

func runShengenRs(spec, output string, verbose, dryRun bool) error {
	rsPath, err := FindShengenRs()
	if err != nil {
		return err
	}

	args := []string{rsPath, spec, "--out", output}
	if verbose || dryRun {
		printCommand("python3", args)
	}
	if dryRun {
		return nil
	}

	cmd := exec.Command("python3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shengen-rs failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Generated %s from %s (experimental rs emitter)\n", output, spec)
	return nil
}

// shengenExArgs builds the shengen-ex argv for the project config.
func shengenExArgs(cfg *Config, spec string) []string {
	args := []string{"--spec", spec, "--namespace", cfg.Pkg, "--out", cfg.Output}
	if cfg.Elixir.TracerOut != "" {
		args = append(args, "--tracer-out", cfg.Elixir.TracerOut)
	}
	if cfg.Elixir.AshOut != "" {
		args = append(args, "--ash-out", cfg.Elixir.AshOut)
		if len(cfg.Elixir.AshTargets) > 0 {
			args = append(args, "--ash-targets", strings.Join(cfg.Elixir.AshTargets, ","))
		}
	}
	if cfg.Elixir.RuntimeModule != "" {
		args = append(args, "--runtime-module", cfg.Elixir.RuntimeModule)
	}
	return args
}

func runShengenEx(cfg *Config, spec string, verbose, dryRun, check bool) error {
	bin, err := FindShengenEx()
	if err != nil {
		return err
	}
	args := shengenExArgs(cfg, spec)
	if check {
		args = append(args, "--check")
	}
	for _, p := range []string{cfg.Elixir.TracerOut, cfg.Elixir.AshOut} {
		if p != "" && !check {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
		}
	}
	if verbose || dryRun {
		printCommand(bin, args)
	}
	if dryRun {
		return nil
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shengen-ex failed: %w", err)
	}
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
