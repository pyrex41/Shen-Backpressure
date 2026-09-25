// shengen-ex — Generate Elixir guard modules (and Ash policy checks, and
// shen-derive style ExUnit tests) from Shen sequent-calculus specs.
//
//	shengen-ex --spec specs/core.shen --namespace MyApp.Shen \
//	    --out lib/my_app/shen/guards_gen.ex \
//	    --tracer-out shen/guard_tracer.ex \
//	    [--ash-out lib/my_app/shen/policy_gen.ex --ash-targets tenant-access,resource-access] \
//	    [--check]
//
//	shengen-ex derive --spec specs/core.shen --func member-of? \
//	    --namespace MyApp.Shen --impl-module MyApp.Authz --impl-func member_of? \
//	    --out test/derived/member_of_spec_test.exs [--stream-data]
//
// Output is deterministic (no timestamps, stable ordering) so committed
// files can be drift-checked with --check or by the tcb-audit gate.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "derive" {
		os.Exit(cmdDerive(os.Args[2:]))
	}
	os.Exit(cmdGen(os.Args[1:]))
}

type output struct {
	path    string
	content string
}

func cmdGen(args []string) int {
	fs := flag.NewFlagSet("shengen-ex", flag.ExitOnError)
	specFile := fs.String("spec", "", "Shen spec path (default specs/core.shen or first positional arg)")
	ns := fs.String("namespace", "", "Elixir namespace for generated modules, e.g. MyApp.Shen (required)")
	out := fs.String("out", "", "guard modules output file (default stdout)")
	tracerOut := fs.String("tracer-out", "", "write <NS>.GuardTracer to this file")
	ashOut := fs.String("ash-out", "", "write Ash policy checks for access rules to this file")
	ashTargets := fs.String("ash-targets", "", "comma-separated access conclusions (default: infer *-access/*-permit/*-allow)")
	tenantType := fs.String("tenant-type", "tenant-id", "Shen type naming the tenant in proof chains (Ash mode)")
	resourceType := fs.String("resource-type", "resource-id", "Shen type naming the resource in proof chains (Ash mode)")
	runtimeMod := fs.String("runtime-module", "", "module implementing :runtime-via checkers (default <NS>.Runtime)")
	allDefines := fs.Bool("all-defines", false, "emit every (define ...) into <NS>.Defines, not only those premises call")
	strict := fs.Bool("strict", false, "fail on premises that cannot be lowered instead of emitting fail-closed checks")
	check := fs.Bool("check", false, "do not write; exit 1 if any output file differs from what would be generated")
	dryRun := fs.Bool("dry-run", false, "print the symbol table (same format as shengen --dry-run) and exit")
	fs.Parse(args)

	path := "specs/core.shen"
	if *specFile != "" {
		path = *specFile
	} else if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shengen-ex: %v\n", err)
		return 1
	}
	types, defines, err := parseSpecContent(string(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "shengen-ex: %s: %v\n", path, err)
		return 1
	}
	st := newSymbolTable(*ns)
	st.Build(types)
	for i := range defines {
		st.Defines[defines[i].Name] = &defines[i]
	}
	if *dryRun {
		printSymbolTable(os.Stderr, types, st, path)
		return 0
	}
	if *ns == "" {
		fmt.Fprintln(os.Stderr, "shengen-ex: --namespace is required (e.g. --namespace MyApp.Shen)")
		return 2
	}
	opt := genOptions{SpecPath: path, Namespace: *ns, RuntimeModule: *runtimeMod, Strict: *strict, AllDefines: *allDefines}
	res, err := generateElixir(types, defines, st, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shengen-ex: %v\n", err)
		return 1
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "shengen-ex: warning: %s\n", w)
	}
	var outs []output
	outs = append(outs, output{*out, res.Code})
	if *tracerOut != "" {
		guardsFile := *out
		if guardsFile == "" {
			guardsFile = "the generated guards file"
		}
		outs = append(outs, output{*tracerOut, generateTracer(res, opt, guardsFile, *tracerOut)})
	}
	if *ashOut != "" {
		code, _, err := generateAsh(string(raw), st, opt, ashOptions{Targets: *ashTargets, TenantType: *tenantType, ResourceType: *resourceType})
		if err != nil {
			fmt.Fprintf(os.Stderr, "shengen-ex: ash: %v\n", err)
			return 1
		}
		outs = append(outs, output{*ashOut, code})
	}
	return writeOutputs(outs, *check, "shengen-ex --spec "+path)
}

// writeOutputs writes (or, with check, compares) every output. An empty
// path means stdout.
func writeOutputs(outs []output, check bool, regen string) int {
	drift := 0
	for _, o := range outs {
		if o.path == "" {
			if !check {
				fmt.Print(o.content)
			}
			continue
		}
		if check {
			have, err := os.ReadFile(o.path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "DRIFT: %s: %v\n", o.path, err)
				drift++
				continue
			}
			if !bytes.Equal(have, []byte(o.content)) {
				drift++
				fmt.Fprintf(os.Stderr, "DRIFT: %s differs from generated output\n", o.path)
				printDiff(o.path, o.content)
			}
			continue
		}
		if err := os.WriteFile(o.path, []byte(o.content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "shengen-ex: writing %s: %v\n", o.path, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "Generated %s\n", o.path)
	}
	if drift > 0 {
		fmt.Fprintf(os.Stderr, "shengen-ex: %d file(s) out of date; regenerate with %s\n", drift, regen)
		return 1
	}
	return 0
}

func printDiff(path, want string) {
	tmp, err := os.CreateTemp("", "shengen-ex-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(want)
	tmp.Close()
	d, _ := exec.Command("diff", "-u", path, tmp.Name()).CombinedOutput()
	lines := strings.Split(string(d), "\n")
	if len(lines) > 60 {
		lines = append(lines[:60], "...")
	}
	fmt.Fprintln(os.Stderr, strings.Join(lines, "\n"))
}
