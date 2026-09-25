package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates/*
var templates embed.FS

//go:embed skilldata/*
var skilldata embed.FS

func cmdInit(args []string) {
	fset := flag.NewFlagSet("init", flag.ExitOnError)
	lang := fset.String("lang", "go", "target language: go, ts or elixir")
	pkg := fset.String("pkg", "shenguard", "guard type package name (elixir: module namespace, default <App>.Shen from mix.exs)")
	withConfig := fset.Bool("config", false, "generate sb.toml config file")
	withMakefile := fset.Bool("makefile", false, "generate Makefile with gate targets")
	noSkills := fset.Bool("no-skills", false, "skip installing Claude Code skills and commands")
	templateDir := fset.String("template-dir", "", "directory with local template overrides (files here shadow embedded templates by basename)")
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
		} else if _, err := os.Stat("mix.exs"); err == nil {
			*lang = "elixir"
		}
	}

	if *lang == "elixir" {
		initElixir(*pkg, isFlagSet(fset, "pkg"), *withMakefile, *noSkills, *templateDir)
		return
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
	writeTemplate("specs/core.shen", "templates/core.shen.tmpl", nil, *templateDir)

	// Write shell scripts
	writeEmbedded("bin/shen-check.sh", "templates/shen-check.sh", 0755, *templateDir)
	writeEmbedded("bin/shengen-codegen.sh", "templates/shengen-codegen.sh", 0755, *templateDir)
	writeEmbedded("bin/shenguard-audit.sh", "templates/shenguard-audit.sh", 0755, *templateDir)

	// Optional: sb.toml
	if *withConfig {
		writeTemplate("sb.toml", "templates/sb.toml.tmpl", cfg, *templateDir)
	}

	// Optional: Makefile
	if *withMakefile {
		writeTemplate("Makefile", "templates/Makefile.tmpl", cfg, *templateDir)
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

// loadTemplateBytes returns the template content, preferring a local
// override file (overrideDir/<basename>) before falling back to the
// embedded FS. The returned source string is used for error messages
// and indicates where the content came from.
func loadTemplateBytes(tmplPath, overrideDir string) ([]byte, string, error) {
	if overrideDir != "" {
		local := filepath.Join(overrideDir, filepath.Base(tmplPath))
		if b, err := os.ReadFile(local); err == nil {
			return b, local, nil
		}
	}
	b, err := templates.ReadFile(tmplPath)
	if err != nil {
		return nil, tmplPath, err
	}
	return b, tmplPath, nil
}

func writeTemplate(dest, tmplPath string, data any, overrideDir string) {
	if _, err := os.Stat(dest); err == nil {
		fmt.Fprintf(os.Stderr, "  skipped %s (already exists)\n", dest)
		return
	}

	content, _, err := loadTemplateBytes(tmplPath, overrideDir)
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

func writeEmbedded(dest, tmplPath string, perm os.FileMode, overrideDir string) {
	if _, err := os.Stat(dest); err == nil {
		fmt.Fprintf(os.Stderr, "  skipped %s (already exists)\n", dest)
		return
	}

	content, _, err := loadTemplateBytes(tmplPath, overrideDir)
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

var mixAppRe = regexp.MustCompile(`app:\s*:([a-z0-9_]+)`)

// elixirApp reads the OTP app name from mix.exs ("my_app"), or "app".
func elixirApp() string {
	if data, err := os.ReadFile("mix.exs"); err == nil {
		if m := mixAppRe.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	return "app"
}

func camelizeApp(app string) string {
	var b strings.Builder
	for _, part := range strings.Split(app, "_") {
		if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

// initElixir scaffolds an Elixir (Mix) project: starter spec with its
// hostile/good corpus, the verified-if prelude, shen-erl check script and
// sb.toml. mix.exs is the user's file, so the tracer wiring is printed.
func initElixir(pkg string, pkgSet, withMakefile, noSkills bool, templateDir string) {
	app := elixirApp()
	if !pkgSet {
		pkg = camelizeApp(app) + ".Shen"
	}
	cfg := &Config{
		Lang:   "elixir",
		Pkg:    pkg,
		Spec:   "specs/core.shen",
		Output: fmt.Sprintf("lib/%s/shen/guards_gen.ex", app),
		Build:  "mix compile --warnings-as-errors",
		Test:   "mix test",
		Check:  "./bin/shen-check.sh",
	}
	fmt.Fprintln(os.Stderr, "Scaffolding Shen-backpressure project (elixir)...")
	for _, d := range []string{"specs/hostile", "specs/good", "bin", "shen", filepath.Dir(cfg.Output)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "sb init: creating %s: %v\n", d, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "  created %s/\n", d)
	}
	writeTemplate("specs/core.shen", "templates/elixir-core.shen.tmpl", nil, templateDir)
	writeEmbedded("specs/verified.shen", "templates/verified.shen", 0644, templateDir)
	writeEmbedded("specs/hostile/01_negative_amount.shen", "templates/elixir-hostile-negative-amount.shen", 0644, templateDir)
	writeEmbedded("specs/hostile/02_self_transfer.shen", "templates/elixir-hostile-self-transfer.shen", 0644, templateDir)
	writeEmbedded("specs/good/01_guarded.shen", "templates/elixir-good-guarded.shen", 0644, templateDir)
	writeEmbedded("bin/shen-check.sh", "templates/elixir-shen-check.sh", 0755, templateDir)
	writeTemplate("sb.toml", "templates/elixir-sb.toml.tmpl", cfg, templateDir)
	if withMakefile {
		writeTemplate("Makefile", "templates/elixir-Makefile.tmpl", cfg, templateDir)
	}
	if !noSkills {
		installSkills()
	}
	fmt.Fprintf(os.Stderr, `
Shen-backpressure scaffolded (elixir, namespace %s).

Wire the guard tracer into mix.exs (sb audit checks this):

    # top of mix.exs
    Code.require_file("shen/guard_tracer.ex", __DIR__)

    # in project/0
    elixirc_options: [tracers: [%s.GuardTracer]],

Next steps:
  1. Edit specs/core.shen; keep one hostile file per verified premise in specs/hostile/
  2. sb gen            # guards + tracer (needs Go to build shengen-ex, or bin/shengen-ex)
  3. sb gates          # gen, compile, test, shen tc+ (shen-erl), tcb audit, mutate-spec
     (shen-erl: https://github.com/pyrex41/shen-erl — set SHEN_ERL_ROOT)
`, cfg.Pkg, cfg.Pkg)
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
