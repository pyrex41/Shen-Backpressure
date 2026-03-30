package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds project configuration, loaded from sb.toml or detected by convention.
type Config struct {
	Lang    string // "go" or "ts"
	Pkg     string // guard type package name
	Spec    string // path to .shen spec file
	Output  string // path to generated guard types
	DBWrap  string // path to generated DB wrappers (optional)
	Gen     string // shengen command (gate 1)
	Build   string // build command (gate 3)
	Test    string // test command (gate 2)
	Check   string // shen tc+ command (gate 4)
	Audit   string // tcb audit command (gate 5)
	Relaxed bool   // run test+build in parallel

	// Loop config
	Harness        string // LLM harness command (e.g. "claude -p")
	MaxIter        int    // max loop iterations
	HarnessTimeout string // per-harness-call timeout
	Prompt         string // path to main prompt file
	Plan           string // path to plan file
}

// tomlConfig mirrors the sb.toml file structure.
type tomlConfig struct {
	Project struct {
		Lang string `toml:"lang"`
		Pkg  string `toml:"pkg"`
	} `toml:"project"`
	Paths struct {
		Spec       string `toml:"spec"`
		Output     string `toml:"output"`
		DBWrappers string `toml:"db_wrappers"`
	} `toml:"paths"`
	Commands struct {
		Gen       string `toml:"gen"`
		Build     string `toml:"build"`
		Test      string `toml:"test"`
		ShenCheck string `toml:"shen_check"`
		Audit     string `toml:"audit"`
	} `toml:"commands"`
	Gates struct {
		Relaxed bool `toml:"relaxed"`
	} `toml:"gates"`
	Loop struct {
		Harness string `toml:"harness"`
		MaxIter int    `toml:"max_iter"`
		Timeout string `toml:"timeout"`
		Prompt  string `toml:"prompt"`
		Plan    string `toml:"plan"`
	} `toml:"loop"`
}

// LoadConfig loads configuration from sb.toml if present, otherwise detects by convention.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Lang:           "go",
		Pkg:            "shenguard",
		Spec:           "specs/core.shen",
		Check:          "./bin/shen-check.sh",
		Audit:          "./bin/shenguard-audit.sh",
		Harness:        "claude -p",
		MaxIter:        10,
		HarnessTimeout: "10m",
		Prompt:         "prompts/main_prompt.md",
		Plan:           "plans/fix_plan.md",
	}

	// Try loading sb.toml
	if _, err := os.Stat("sb.toml"); err == nil {
		var tc tomlConfig
		if _, err := toml.DecodeFile("sb.toml", &tc); err != nil {
			return nil, fmt.Errorf("parsing sb.toml: %w", err)
		}
		if tc.Project.Lang != "" {
			cfg.Lang = tc.Project.Lang
		}
		if tc.Project.Pkg != "" {
			cfg.Pkg = tc.Project.Pkg
		}
		if tc.Paths.Spec != "" {
			cfg.Spec = tc.Paths.Spec
		}
		if tc.Paths.Output != "" {
			cfg.Output = tc.Paths.Output
		}
		if tc.Paths.DBWrappers != "" {
			cfg.DBWrap = tc.Paths.DBWrappers
		}
		if tc.Commands.Gen != "" {
			cfg.Gen = tc.Commands.Gen
		}
		if tc.Commands.Build != "" {
			cfg.Build = tc.Commands.Build
		}
		if tc.Commands.Test != "" {
			cfg.Test = tc.Commands.Test
		}
		if tc.Commands.ShenCheck != "" {
			cfg.Check = tc.Commands.ShenCheck
		}
		if tc.Commands.Audit != "" {
			cfg.Audit = tc.Commands.Audit
		}
		cfg.Relaxed = tc.Gates.Relaxed

		if tc.Loop.Harness != "" {
			cfg.Harness = tc.Loop.Harness
		}
		if tc.Loop.MaxIter > 0 {
			cfg.MaxIter = tc.Loop.MaxIter
		}
		if tc.Loop.Timeout != "" {
			cfg.HarnessTimeout = tc.Loop.Timeout
		}
		if tc.Loop.Prompt != "" {
			cfg.Prompt = tc.Loop.Prompt
		}
		if tc.Loop.Plan != "" {
			cfg.Plan = tc.Loop.Plan
		}
	}

	// Convention detection for unset fields
	if cfg.Lang == "" || cfg.Lang == "go" {
		if _, err := os.Stat("go.mod"); err == nil {
			cfg.Lang = "go"
		} else if _, err := os.Stat("package.json"); err == nil {
			cfg.Lang = "ts"
		}
	}

	if cfg.Output == "" {
		switch cfg.Lang {
		case "go":
			cfg.Output = fmt.Sprintf("internal/%s/guards_gen.go", cfg.Pkg)
		case "ts":
			cfg.Output = fmt.Sprintf("src/%s/guards.ts", cfg.Pkg)
		}
	}

	if cfg.Gen == "" {
		cfg.Gen = "./bin/shengen-codegen.sh"
	}

	if cfg.Build == "" {
		switch cfg.Lang {
		case "go":
			cfg.Build = "go build ./..."
		case "ts":
			cfg.Build = "npx tsc --noEmit"
		}
	}

	if cfg.Test == "" {
		switch cfg.Lang {
		case "go":
			cfg.Test = "go test ./..."
		case "ts":
			cfg.Test = "npm test"
		}
	}

	// Env var overrides for loop config
	if v := os.Getenv("RALPH_HARNESS"); v != "" {
		cfg.Harness = v
	}
	if v := os.Getenv("RALPH_MAX_ITER"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.MaxIter = n
		}
	}
	if v := os.Getenv("RALPH_HARNESS_TIMEOUT"); v != "" {
		cfg.HarnessTimeout = v
	}

	return cfg, nil
}

// FindShengen locates the shengen binary using the discovery chain:
// ./bin/shengen -> $SHENGEN_PATH -> $PATH -> build from cmd/shengen/main.go
func FindShengen() (string, error) {
	if _, err := os.Stat("bin/shengen"); err == nil {
		return "bin/shengen", nil
	}
	if p := os.Getenv("SHENGEN_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("shengen"); err == nil {
		return p, nil
	}

	// Try to build from source
	candidates := []string{
		"cmd/shengen/main.go",
		"../../cmd/shengen/main.go",
	}
	for _, src := range candidates {
		if _, err := os.Stat(src); err == nil {
			srcDir := filepath.Dir(src)
			outPath, _ := filepath.Abs("bin/shengen")
			fmt.Fprintf(os.Stderr, "Building shengen from %s...\n", srcDir)
			cmd := exec.Command("go", "build", "-o", outPath, ".")
			cmd.Dir = srcDir
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("building shengen from %s: %w", srcDir, err)
			}
			return outPath, nil
		}
	}

	return "", fmt.Errorf("shengen not found: check bin/shengen, $SHENGEN_PATH, $PATH, or cmd/shengen/main.go")
}

// FindShengenTS locates the TypeScript shengen.
func FindShengenTS() (string, error) {
	candidates := []string{
		"cmd/shengen-ts/shengen.ts",
		"../../cmd/shengen-ts/shengen.ts",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("shengen-ts not found")
}

// SplitCommand splits a shell command string into the binary and its arguments.
func SplitCommand(cmd string) (string, []string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
