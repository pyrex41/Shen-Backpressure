package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*
var templates embed.FS

//go:embed skilldata/*
var skilldata embed.FS

func cmdInit(args []string) {
	fset := flag.NewFlagSet("init", flag.ExitOnError)
	lang := fset.String("lang", "go", "target language: go or ts")
	pkg := fset.String("pkg", "shenguard", "guard type package name")
	withConfig := fset.Bool("config", false, "generate sb.toml config file")
	withMakefile := fset.Bool("makefile", false, "generate Makefile with gate targets")
	noSkills := fset.Bool("no-skills", false, "skip installing Claude Code skills and commands")
	fset.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb init — Scaffold a new Shen-backpressure project

Usage: sb init [flags]

Creates the directory structure, starter spec, shell scripts, and Claude Code
skills/commands needed for Shen-backpressure verification.

Does NOT run shengen — use "sb gen" next.

Flags:
`)
		fset.PrintDefaults()
	}
	fset.Parse(args)

	// Detect language if not explicitly set
	if !isFlagSet(fset, "lang") {
		if _, err := os.Stat("package.json"); err == nil {
			*lang = "ts"
		}
	}

	cfg := &Config{
		Lang:  *lang,
		Pkg:   *pkg,
		Spec:  "specs/core.shen",
		Check: "./bin/shen-check.sh",
	}
	switch cfg.Lang {
	case "go":
		cfg.Output = fmt.Sprintf("internal/%s/guards_gen.go", cfg.Pkg)
		cfg.Build = "go build ./..."
		cfg.Test = "go test ./..."
	case "ts":
		cfg.Output = fmt.Sprintf("src/%s/guards.ts", cfg.Pkg)
		cfg.Build = "npx tsc --noEmit"
		cfg.Test = "npm test"
	}

	fmt.Fprintln(os.Stderr, "Scaffolding Shen-backpressure project...")

	// Create directories
	dirs := []string{
		"specs",
		"bin",
		filepath.Dir(cfg.Output),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "sb init: creating %s: %v\n", d, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "  created %s/\n", d)
	}

	// Write starter spec
	writeTemplate("specs/core.shen", "templates/core.shen.tmpl", nil)

	// Write shell scripts
	writeEmbedded("bin/shen-check.sh", "templates/shen-check.sh", 0755)
	writeEmbedded("bin/shengen-codegen.sh", "templates/shengen-codegen.sh", 0755)
	writeEmbedded("bin/shenguard-audit.sh", "templates/shenguard-audit.sh", 0755)

	// Optional: sb.toml
	if *withConfig {
		writeTemplate("sb.toml", "templates/sb.toml.tmpl", cfg)
	}

	// Optional: Makefile
	if *withMakefile {
		writeTemplate("Makefile", "templates/Makefile.tmpl", cfg)
	}

	// Install skills and commands
	if !*noSkills {
		installSkills()
	}

	fmt.Fprintf(os.Stderr, `
Shen-backpressure scaffolded successfully.

Next steps:
  1. Edit specs/core.shen with your domain types
  2. Run "sb gen" to generate guard types
  3. Run "sb gates" to verify all five gates pass
`)
}

// installSkills copies the embedded skill bundle into .claude/
func installSkills() {
	fmt.Fprintln(os.Stderr, "\nInstalling Claude Code skills and commands...")

	// Walk the embedded skilldata filesystem and copy everything
	err := fs.WalkDir(skilldata, "skilldata", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Map skilldata/X to .claude/X
		relPath := strings.TrimPrefix(path, "skilldata/")
		if relPath == "" || relPath == "skilldata" {
			return nil
		}
		destPath := filepath.Join(".claude", relPath)

		if d.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("creating %s: %w", destPath, err)
			}
			return nil
		}

		// Read embedded file
		content, err := skilldata.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		// Map the directory structure:
		// skilldata/skills/shen-backpressure/SKILL.md -> .claude/skills/shen-backpressure/SKILL.md
		// skilldata/commands/init.md                  -> .claude/commands/sb/init.md
		// skilldata/AGENT_PROMPT.md                   -> .claude/commands/sb/AGENT_PROMPT.md

		// Commands go under .claude/commands/sb/ (namespaced)
		if strings.HasPrefix(relPath, "commands/") {
			destPath = filepath.Join(".claude", "commands", "sb", strings.TrimPrefix(relPath, "commands/"))
		}
		// AGENT_PROMPT.md goes alongside the commands
		if relPath == "AGENT_PROMPT.md" {
			destPath = filepath.Join(".claude", "commands", "sb", "AGENT_PROMPT.md")
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating parent for %s: %w", destPath, err)
		}

		// Skip if already exists (don't overwrite customizations)
		if _, err := os.Stat(destPath); err == nil {
			fmt.Fprintf(os.Stderr, "  skipped %s (already exists)\n", destPath)
			return nil
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}
		fmt.Fprintf(os.Stderr, "  wrote %s\n", destPath)
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "sb init: installing skills: %v\n", err)
		os.Exit(1)
	}
}

func writeTemplate(dest, tmplPath string, data any) {
	if _, err := os.Stat(dest); err == nil {
		fmt.Fprintf(os.Stderr, "  skipped %s (already exists)\n", dest)
		return
	}

	content, err := templates.ReadFile(tmplPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb init: reading template %s: %v\n", tmplPath, err)
		os.Exit(1)
	}

	if data != nil && (strings.Contains(string(content), "{{.") || strings.Contains(string(content), "{{$")) {
		tmpl, err := template.New(filepath.Base(tmplPath)).Parse(string(content))
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb init: parsing template %s: %v\n", tmplPath, err)
			os.Exit(1)
		}
		f, err := os.Create(dest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb init: creating %s: %v\n", dest, err)
			os.Exit(1)
		}
		defer f.Close()
		if err := tmpl.Execute(f, data); err != nil {
			fmt.Fprintf(os.Stderr, "sb init: executing template %s: %v\n", tmplPath, err)
			os.Exit(1)
		}
	} else {
		if err := os.WriteFile(dest, content, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "sb init: writing %s: %v\n", dest, err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "  wrote %s\n", dest)
}

func writeEmbedded(dest, tmplPath string, perm os.FileMode) {
	if _, err := os.Stat(dest); err == nil {
		fmt.Fprintf(os.Stderr, "  skipped %s (already exists)\n", dest)
		return
	}

	content, err := templates.ReadFile(tmplPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb init: reading %s: %v\n", tmplPath, err)
		os.Exit(1)
	}

	if err := os.WriteFile(dest, content, perm); err != nil {
		fmt.Fprintf(os.Stderr, "sb init: writing %s: %v\n", dest, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  wrote %s\n", dest)
}

func isFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
