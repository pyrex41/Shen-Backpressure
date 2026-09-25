package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestElixirConfigAndGates(t *testing.T) {
	testRestoreCwd(t)
	dir := t.TempDir()
	toml := `[project]
lang = "elixir"
pkg  = "Demo.Shen"

[elixir]
ash_out = "lib/demo/shen/policy_gen.ex"
ash_targets = ["tenant-access"]

[[gates]]
name = "shengen"
run  = "sb gen"

[mutate]
hostile = ["specs/hostile/*.shen"]
max_inferences = 5000000
`
	os.WriteFile(filepath.Join(dir, "sb.toml"), []byte(toml), 0o644)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer testRestoreCwd(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lang != "elixir" || cfg.Output != "lib/shen/guards_gen.ex" || cfg.Build != "mix compile --warnings-as-errors" || cfg.Test != "mix test" {
		t.Errorf("elixir defaults: %+v", cfg)
	}
	if cfg.Elixir.TracerOut != "shen/guard_tracer.ex" || cfg.Elixir.AshOut == "" || cfg.Mutate.MaxInferences != 5000000 {
		t.Errorf("[elixir]/[mutate]: %+v %+v", cfg.Elixir, cfg.Mutate)
	}
	args := strings.Join(shengenExArgs(cfg, cfg.Spec), " ")
	for _, want := range []string{"--namespace Demo.Shen", "--tracer-out shen/guard_tracer.ex", "--ash-out lib/demo/shen/policy_gen.ex", "--ash-targets tenant-access"} {
		if !strings.Contains(args, want) {
			t.Errorf("shengen-ex args %q missing %q", args, want)
		}
	}
	gates := buildGateList(cfg)
	self, _ := os.Executable()
	if gates[0].cmd != self || strings.Join(gates[0].args, " ") != "gen" {
		t.Errorf("`sb gen` gate should run this binary: %+v", gates[0])
	}
	if last := gates[len(gates)-1]; last.name != "mutate-spec" {
		t.Errorf("mutate-spec gate not appended: %+v", gates)
	}
}

func TestInitElixirScaffold(t *testing.T) {
	testRestoreCwd(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mix.exs"), []byte("def project, do: [app: :ledger_app, version: \"0.1.0\"]\n"), 0o644)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer testRestoreCwd(t)
	initElixir("shenguard", false, true, true, "")
	for _, f := range []string{"specs/core.shen", "specs/verified.shen", "specs/hostile/01_negative_amount.shen",
		"specs/hostile/02_self_transfer.shen", "specs/good/01_guarded.shen", "bin/shen-check.sh", "sb.toml", "Makefile"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	data, _ := os.ReadFile("sb.toml")
	for _, want := range []string{`pkg  = "LedgerApp.Shen"`, `output = "lib/ledger_app/shen/guards_gen.ex"`, `run  = "sb audit"`, "[mutate]"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("sb.toml missing %q", want)
		}
	}
	// The starter spec has exactly one hostile witness per verified premise.
	spec, _ := os.ReadFile("specs/core.shen")
	if n := len(findVerifiedPremises(string(spec))); n != 2 {
		t.Errorf("starter spec should have 2 verified premises, got %d", n)
	}
}

// TestAuditElixir runs the in-process audit on a synthetic project: a clean
// tree passes (drift check skipped by pointing at a matching generated
// file); hand-written namespace modules and raw __struct__ maps fail.
func TestAuditElixir(t *testing.T) {
	testRestoreCwd(t)
	bin, err := FindShengenEx()
	if err != nil {
		t.Skipf("shengen-ex unavailable: %v", err)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "specs"), 0o755)
	os.MkdirAll(filepath.Join(dir, "lib/demo/shen"), 0o755)
	os.WriteFile(filepath.Join(dir, "specs/core.shen"), []byte("(datatype user-id\n  X : string;\n  ==========\n  X : user-id;)\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "mix.exs"), []byte("Code.require_file(\"shen/guard_tracer.ex\", __DIR__)\n# elixirc_options: [tracers: [Demo.Shen.GuardTracer]]\n"), 0o644)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer testRestoreCwd(t)
	t.Setenv("SHENGEN_EX_PATH", bin)
	cfg := &Config{Lang: "elixir", Pkg: "Demo.Shen", Spec: "specs/core.shen", Output: "lib/demo/shen/guards_gen.ex",
		Elixir: ElixirConfig{TracerOut: "shen/guard_tracer.ex"}}
	os.MkdirAll("shen", 0o755)
	if err := runShengenEx(cfg, cfg.Spec, false, false, false); err != nil {
		t.Fatal(err)
	}
	if p := auditElixir(cfg); len(p) != 0 {
		t.Fatalf("clean project should pass, got %v", p)
	}
	os.WriteFile("lib/demo/evil.ex", []byte("defmodule Demo.Shen.Evil do\n  def f(t), do: %{__struct__: Demo.Shen.UserId, val: t}\nend\n"), 0o644)
	os.WriteFile("lib/demo/shen/extra.ex", []byte("defmodule Demo.Extra do\nend\n"), 0o644)
	p := strings.Join(auditElixir(cfg), "\n")
	for _, want := range []string{"generated namespace Demo.Shen", "raw __struct__ map", "extra.ex is not generated"} {
		if !strings.Contains(p, want) {
			t.Errorf("audit should report %q; got:\n%s", want, p)
		}
	}
	os.Remove("lib/demo/evil.ex")
	os.Remove("lib/demo/shen/extra.ex")
	os.WriteFile("specs/core.shen", []byte("(datatype user-id\n  X : number;\n  ==========\n  X : user-id;)\n"), 0o644)
	if p := strings.Join(auditElixir(cfg), "\n"); !strings.Contains(p, "drifted") {
		t.Errorf("spec change should be reported as drift, got %q", p)
	}
}

func TestShengenExReachable(t *testing.T) {
	testRestoreCwd(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	if _, err := FindShengenEx(); err != nil {
		t.Fatal(err)
	}
}
