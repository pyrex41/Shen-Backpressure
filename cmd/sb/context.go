package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ContextOutput is the structured context for LLM harnesses.
type ContextOutput struct {
	Spec       SpecInfo       `json:"spec"`
	Guards     GuardsInfo     `json:"guards"`
	Gates      GatesInfo      `json:"gates"`
	Config     ConfigInfo     `json:"config"`
}

type SpecInfo struct {
	Path        string `json:"path"`
	SymbolTable string `json:"symbol_table"` // raw shengen --dry-run output
}

type GuardsInfo struct {
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

type GatesInfo struct {
	Failures []string `json:"failures,omitempty"` // from backpressure.log
}

type ConfigInfo struct {
	Lang  string `json:"lang"`
	Pkg   string `json:"pkg"`
	Build string `json:"build"`
	Test  string `json:"test"`
}

func cmdContext(args []string) {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	format := fs.String("format", "json", "output format: json or md")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb context — Emit structured context for LLM harnesses

Usage: sb context [flags]

Generates a structured summary of the project's formal verification state,
including spec types, guard type inventory, recent gate failures, and config.
Useful for injecting into LLM prompts via skill backtick syntax or piping.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb context: %v\n", err)
		os.Exit(1)
	}

	ctx := buildContext(cfg)

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(ctx)
	case "md":
		printContextMarkdown(ctx, cfg)
	default:
		fmt.Fprintf(os.Stderr, "sb context: unknown format %q (use json or md)\n", *format)
		os.Exit(1)
	}
}

func buildContext(cfg *Config) ContextOutput {
	ctx := ContextOutput{
		Config: ConfigInfo{
			Lang:  cfg.Lang,
			Pkg:   cfg.Pkg,
			Build: cfg.Build,
			Test:  cfg.Test,
		},
	}

	// Spec info
	ctx.Spec.Path = cfg.Spec
	ctx.Spec.SymbolTable = getSymbolTable(cfg)

	// Guard type files
	ctx.Guards.Path = cfg.Output
	ctx.Guards.Files = listGuardFiles(cfg)

	// Gate failures from backpressure log
	ctx.Gates.Failures = getRecentFailures()

	return ctx
}

func getSymbolTable(cfg *Config) string {
	shengen, err := FindShengen()
	if err != nil {
		return fmt.Sprintf("(shengen not found: %v)", err)
	}

	cmd := exec.Command(shengen, "--spec", cfg.Spec, "--pkg", cfg.Pkg, "--dry-run")
	stderr, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(shengen --dry-run failed: %v)", err)
	}
	return strings.TrimSpace(string(stderr))
}

func listGuardFiles(cfg *Config) []string {
	dir := cfg.Output
	// Use the directory containing the output file
	if idx := strings.LastIndex(dir, "/"); idx >= 0 {
		dir = dir[:idx]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files
}

func getRecentFailures() []string {
	logPaths := []string{
		"plans/backpressure.log",
	}

	for _, p := range logPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		// Extract the most recent failure block (last --- block)
		content := string(data)
		blocks := strings.Split(content, "---")
		if len(blocks) < 2 {
			continue
		}
		lastBlock := strings.TrimSpace(blocks[len(blocks)-1])
		if lastBlock == "" && len(blocks) >= 3 {
			lastBlock = strings.TrimSpace(blocks[len(blocks)-2])
		}
		if lastBlock == "" {
			continue
		}

		var failures []string
		for _, line := range strings.Split(lastBlock, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "FAIL") {
				failures = append(failures, line)
			}
		}
		return failures
	}
	return nil
}

func printContextMarkdown(ctx ContextOutput, cfg *Config) {
	fmt.Printf("# Shen-Backpressure Context\n\n")
	fmt.Printf("## Spec\n\n")
	fmt.Printf("- **Path**: `%s`\n", ctx.Spec.Path)
	fmt.Printf("- **Language**: %s\n", ctx.Config.Lang)
	fmt.Printf("- **Package**: %s\n\n", ctx.Config.Pkg)

	if ctx.Spec.SymbolTable != "" {
		fmt.Printf("### Symbol Table\n\n```\n%s\n```\n\n", ctx.Spec.SymbolTable)
	}

	fmt.Printf("## Guard Types\n\n")
	fmt.Printf("- **Output**: `%s`\n", ctx.Guards.Path)
	if len(ctx.Guards.Files) > 0 {
		fmt.Printf("- **Files**: %s\n", strings.Join(ctx.Guards.Files, ", "))
	}
	fmt.Println()

	fmt.Printf("## Gates\n\n")
	fmt.Printf("| Gate | Command |\n")
	fmt.Printf("|------|--------|\n")
	fmt.Printf("| 1. shengen | `sb gen` |\n")
	fmt.Printf("| 2. test | `%s` |\n", ctx.Config.Test)
	fmt.Printf("| 3. build | `%s` |\n", ctx.Config.Build)
	fmt.Printf("| 4. shen tc+ | `sb check` |\n")
	fmt.Printf("| 5. tcb audit | `sb audit` |\n\n")

	if len(ctx.Gates.Failures) > 0 {
		fmt.Printf("### Recent Failures\n\n")
		for _, f := range ctx.Gates.Failures {
			fmt.Printf("- %s\n", f)
		}
		fmt.Println()
	}
}
