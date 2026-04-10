package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// GateKind identifies whether a gate runs a shell command or a derive verification.
type GateKind string

const (
	GateKindCommand GateKind = "command"
	GateKindDerive  GateKind = "derive"
)

// GateDef is a manifest-defined gate entry from [[gates]] in sb.toml.
type GateDef struct {
	Name          string   `toml:"name"`
	Kind          GateKind `toml:"kind"`
	Run           string   `toml:"run"`
	ParallelGroup string   `toml:"parallel_group"`
}

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

	// Manifest-defined gates (new [[gates]] format). When non-nil the gate
	// engine uses these instead of synthesising from Gen/Build/Test/Check/Audit.
	Gates []GateDef

	// Derive config (gate 6 — spec-equivalence verification)
	DeriveDir   string       // path to shen-derive module (default ../../shen-derive)
	DeriveSpecs []DeriveSpec // one entry per (define ...) to verify

	// Loop config
	Harness        string // LLM harness command (e.g. "claude -p")
	MaxIter        int    // max loop iterations
	HarnessTimeout string // per-harness-call timeout
	Prompt         string // path to main prompt file
	Plan           string // path to plan file
}

// HasManifestGates reports whether the config uses the new [[gates]] format.
func (c *Config) HasManifestGates() bool { return c.Gates != nil }

// DeriveSpec configures one shen-derive verify invocation.
type DeriveSpec struct {
	Lang     string // "go" (default) or "ts" — selects which shen-derive port runs
	Path     string // path to the .shen file holding the (define ...) block
	Func     string // Shen define name, e.g. "processable"
	ImplPkg  string // Go import path (go) or relative TS module path (ts) of the implementation
	ImplFunc string // implementation function name, e.g. "Processable"
	GuardPkg string // Go import path (go) or relative TS module path (ts) of the shengen guard module
	OutFile  string // path to the committed generated test file
	Seed     int64  // optional RNG seed; 0 = deterministic
}

// tomlDeriveSpec mirrors a [[derive.specs]] entry in sb.toml.
type tomlDeriveSpec struct {
	Lang     string `toml:"lang"`
	Path     string `toml:"path"`
	Func     string `toml:"func"`
	ImplPkg  string `toml:"impl_pkg"`
	ImplFunc string `toml:"impl_func"`
	GuardPkg string `toml:"guard_pkg"`
	OutFile  string `toml:"out_file"`
	Seed     int64  `toml:"seed"`
}

// tomlGateDef mirrors a [[gates]] entry in the new sb.toml format.
type tomlGateDef struct {
	Name          string `toml:"name"`
	Kind          string `toml:"kind"`
	Run           string `toml:"run"`
	ParallelGroup string `toml:"parallel_group"`
}

// tomlConfigNew is the new-format schema where `gates` is an array-of-tables
// and engine settings live under [engine].
type tomlConfigNew struct {
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
	Engine struct {
		Relaxed bool `toml:"relaxed"`
	} `toml:"engine"`
	Gates  []tomlGateDef `toml:"gates"`
	Derive struct {
		Dir   string           `toml:"dir"`
		Specs []tomlDeriveSpec `toml:"specs"`
	} `toml:"derive"`
	Loop struct {
		Harness string `toml:"harness"`
		MaxIter int    `toml:"max_iter"`
		Timeout string `toml:"timeout"`
		Prompt  string `toml:"prompt"`
		Plan    string `toml:"plan"`
	} `toml:"loop"`
}

// tomlConfigLegacy is the legacy schema where `gates` is a single table
// with a `relaxed` field.
type tomlConfigLegacy struct {
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
	Derive struct {
		Dir   string           `toml:"dir"`
		Specs []tomlDeriveSpec `toml:"specs"`
	} `toml:"derive"`
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

	// Try loading sb.toml — two-pass decode to handle both legacy ([gates]
	// table with relaxed field) and new ([[gates]] array-of-tables) formats.
	if _, err := os.Stat("sb.toml"); err == nil {
		var tcNew tomlConfigNew
		_, errNew := toml.DecodeFile("sb.toml", &tcNew)

		if errNew == nil && len(tcNew.Gates) > 0 {
			// New format: [[gates]] array-of-tables + [engine]
			applyProjectPaths(cfg, tcNew.Project.Lang, tcNew.Project.Pkg,
				tcNew.Paths.Spec, tcNew.Paths.Output, tcNew.Paths.DBWrappers)
			applyCommands(cfg, tcNew.Commands.Gen, tcNew.Commands.Build,
				tcNew.Commands.Test, tcNew.Commands.ShenCheck, tcNew.Commands.Audit)
			cfg.Relaxed = tcNew.Engine.Relaxed

			cfg.Gates = make([]GateDef, len(tcNew.Gates))
			for i, g := range tcNew.Gates {
				kind := GateKind(g.Kind)
				if kind == "" {
					kind = GateKindCommand
				}
				cfg.Gates[i] = GateDef{
					Name:          g.Name,
					Kind:          kind,
					Run:           g.Run,
					ParallelGroup: g.ParallelGroup,
				}
			}

			applyDerive(cfg, tcNew.Derive.Dir, tcNew.Derive.Specs)
			applyLoop(cfg, tcNew.Loop.Harness, tcNew.Loop.MaxIter,
				tcNew.Loop.Timeout, tcNew.Loop.Prompt, tcNew.Loop.Plan)
		} else {
			// Legacy format: [gates] table with relaxed bool
			var tcLegacy tomlConfigLegacy
			if _, err := toml.DecodeFile("sb.toml", &tcLegacy); err != nil {
				return nil, fmt.Errorf("parsing sb.toml: %w", err)
			}
			applyProjectPaths(cfg, tcLegacy.Project.Lang, tcLegacy.Project.Pkg,
				tcLegacy.Paths.Spec, tcLegacy.Paths.Output, tcLegacy.Paths.DBWrappers)
			applyCommands(cfg, tcLegacy.Commands.Gen, tcLegacy.Commands.Build,
				tcLegacy.Commands.Test, tcLegacy.Commands.ShenCheck, tcLegacy.Commands.Audit)
			cfg.Relaxed = tcLegacy.Gates.Relaxed

			applyDerive(cfg, tcLegacy.Derive.Dir, tcLegacy.Derive.Specs)
			applyLoop(cfg, tcLegacy.Loop.Harness, tcLegacy.Loop.MaxIter,
				tcLegacy.Loop.Timeout, tcLegacy.Loop.Prompt, tcLegacy.Loop.Plan)
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

// applyProjectPaths sets the project-level and paths fields from TOML values.
func applyProjectPaths(cfg *Config, lang, pkg, spec, output, dbWrap string) {
	if lang != "" {
		cfg.Lang = lang
	}
	if pkg != "" {
		cfg.Pkg = pkg
	}
	if spec != "" {
		cfg.Spec = spec
	}
	if output != "" {
		cfg.Output = output
	}
	if dbWrap != "" {
		cfg.DBWrap = dbWrap
	}
}

// applyCommands sets the gate command fields from TOML values.
func applyCommands(cfg *Config, gen, build, test, check, audit string) {
	if gen != "" {
		cfg.Gen = gen
	}
	if build != "" {
		cfg.Build = build
	}
	if test != "" {
		cfg.Test = test
	}
	if check != "" {
		cfg.Check = check
	}
	if audit != "" {
		cfg.Audit = audit
	}
}

// applyDerive sets the derive config from TOML values.
func applyDerive(cfg *Config, dir string, specs []tomlDeriveSpec) {
	if dir != "" {
		cfg.DeriveDir = dir
	}
	for _, s := range specs {
		lang := s.Lang
		if lang == "" {
			lang = "go"
		}
		cfg.DeriveSpecs = append(cfg.DeriveSpecs, DeriveSpec{
			Lang:     lang,
			Path:     s.Path,
			Func:     s.Func,
			ImplPkg:  s.ImplPkg,
			ImplFunc: s.ImplFunc,
			GuardPkg: s.GuardPkg,
			OutFile:  s.OutFile,
			Seed:     s.Seed,
		})
	}
}

// applyLoop sets the loop config from TOML values.
func applyLoop(cfg *Config, harness string, maxIter int, timeout, prompt, plan string) {
	if harness != "" {
		cfg.Harness = harness
	}
	if maxIter > 0 {
		cfg.MaxIter = maxIter
	}
	if timeout != "" {
		cfg.HarnessTimeout = timeout
	}
	if prompt != "" {
		cfg.Prompt = prompt
	}
	if plan != "" {
		cfg.Plan = plan
	}
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
