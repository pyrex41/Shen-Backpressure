// shen-derive — Verification gate for Shen specs.
//
// You write a Shen spec (the obviously-correct definition). An LLM or human
// writes the Go implementation. shen-derive evaluates the spec on sampled
// inputs and generates a Go test that checks the implementation matches.
// The spec is the oracle.
//
// Subcommands:
//   parse    Parse a .shen file and pretty-print its structure
//   eval     Evaluate an s-expression
//   verify   Generate a spec-equivalence test for a Go implementation
//
// See plan: /Users/reuben/.claude/plans/snazzy-conjuring-spring.md

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/report"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/symbolic"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/verify"
)

const version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		runREPL()
		return
	}

	switch os.Args[1] {
	case "repl":
		runREPL()
	case "eval":
		cmdEval(os.Args[2:])
	case "parse":
		cmdParse(os.Args[2:])
	case "verify":
		cmdVerify(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("shen-derive %s\n", version)
	case "help", "--help", "-h":
		usage()
	default:
		expr := strings.Join(os.Args[1:], " ")
		evalAndPrint(expr)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `shen-derive — Verification gate for Shen specs (v%s)

Usage: shen-derive <command> [args]

Commands:
  repl                     Interactive s-expression evaluator (default)
  eval    <expr>           Evaluate an s-expression
  parse   <spec.shen>      Parse a .shen file and print its structure
  verify  <spec.shen>      Generate a spec-equivalence test (see "verify --help")
  version                  Print version

The verify command:
  shen-derive verify SPEC.shen \
    --func FUNC_NAME                   (required) which (define ...) block to use
    --impl-pkg IMPORT_PATH             (required) Go import path of the implementation
    --impl-func GO_FUNC_NAME           (required) name of the Go function to test
    --import IMPORT_PATH               (required) Go import path of the shengen package
    --import-alias ALIAS               default: "shenguard"
    --impl-pkg-name NAME               default: last segment of --impl-pkg
    --test-pkg NAME                    default: <impl-pkg-name>_test
    --out FILE                         default: stdout
    --max-cases N                      default: 24
    --path-cover                       add one sample per feasible spec path (needs z3 on PATH)
    --path-depth N                     list-unrolling depth for --path-cover (default 4)
    --vacuity                          check datatypes for inhabitation (default true)
`, version)
}

// --- REPL ---

func runREPL() {
	fmt.Fprintf(os.Stderr, "shen-derive %s — interactive evaluator\n", version)
	fmt.Fprintf(os.Stderr, "Type s-expressions to evaluate. Use :q to quit.\n\n")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "λ> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == ":q" || line == ":quit" {
			break
		}
		evalAndPrint(line)
	}
	fmt.Fprintln(os.Stderr)
}

// --- eval ---

func cmdEval(args []string) {
	if len(args) > 0 {
		evalAndPrint(strings.Join(args, " "))
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "\\\\") {
			continue
		}
		evalAndPrint(line)
	}
}

func evalAndPrint(input string) {
	sexpr, err := core.ParseSexpr(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		return
	}
	val, err := core.Eval(core.EmptyEnv(), sexpr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eval error: %v\n", err)
		return
	}
	fmt.Println(val.String())
}

// --- parse ---

func cmdParse(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shen-derive parse <spec.shen>")
		os.Exit(1)
	}
	sf, err := specfile.ParseFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Spec file: %s\n\n", sf.Path)

	fmt.Printf("Datatypes (%d):\n", len(sf.Datatypes))
	for _, dt := range sf.Datatypes {
		fmt.Printf("  %s (%d rule(s))\n", dt.Name, len(dt.Rules))
		for _, r := range dt.Rules {
			for _, p := range r.Premises {
				fmt.Printf("    %s : %s\n", p.VarName, p.TypeName)
			}
			for _, v := range r.Verified {
				fmt.Printf("    %s : verified\n", v.Raw)
			}
			if r.Conclusion.IsWrapped {
				fmt.Printf("    => %s : %s\n", r.Conclusion.Fields[0], r.Conclusion.TypeName)
			} else {
				fmt.Printf("    => [%s] : %s\n", strings.Join(r.Conclusion.Fields, " "), r.Conclusion.TypeName)
			}
		}
	}

	fmt.Printf("\nDefines (%d):\n", len(sf.Defines))
	for _, d := range sf.Defines {
		if len(d.TypeSig.ParamTypes) > 0 {
			sig := fmt.Sprintf("{%s --> %s}", strings.Join(d.TypeSig.ParamTypes, " --> "), d.TypeSig.ReturnType)
			fmt.Printf("  %s %s\n", d.Name, sig)
		} else {
			fmt.Printf("  %s (no type sig)\n", d.Name)
		}
		for ci, cl := range d.Clauses {
			patStrs := make([]string, len(cl.Patterns))
			for i, p := range cl.Patterns {
				patStrs[i] = core.PrettyPrintSexpr(p)
			}
			prefix := "    "
			if len(d.Clauses) > 1 {
				prefix = fmt.Sprintf("    [%d] ", ci)
			}
			guard := ""
			if cl.Guard != nil {
				guard = " where " + core.PrettyPrintSexpr(cl.Guard)
			}
			fmt.Printf("%s%s -> %s%s\n", prefix,
				strings.Join(patStrs, " "), core.PrettyPrintSexpr(cl.Body), guard)
		}
	}
}

// --- verify ---

func cmdVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	funcName := fs.String("func", "", "(required) name of the (define ...) block in the spec")
	implPkgPath := fs.String("impl-pkg", "", "(required) Go import path of the implementation")
	implFunc := fs.String("impl-func", "", "(required) name of the Go function to test")
	importPath := fs.String("import", "", "(required) Go import path of the shengen package")
	importAlias := fs.String("import-alias", "shenguard", "import alias for the shengen package")
	implPkgName := fs.String("impl-pkg-name", "", "Go package name (default: last segment of --impl-pkg)")
	testPkg := fs.String("test-pkg", "", "package name for the generated test (default: <impl-pkg-name>_test)")
	out := fs.String("out", "", "output file (default: stdout)")
	maxCases := fs.Int("max-cases", 50, "maximum number of test cases")
	seed := fs.Int64("seed", 0, "RNG seed for random sampling (0 = deterministic boundary values only)")
	randomDraws := fs.Int("random-draws", 0, "number of random primitive draws per type when --seed != 0 (default 8)")
	reportOut := fs.String("report-out", "", "if non-empty, write a per-spec discharge report (JSON) to this path")
	guardFile := fs.String("guard-file", "", "path to shengen-emitted guards file (used to populate code_references in the discharge report)")
	pathCover := fs.Bool("path-cover", false, "add one concrete sample per feasible spec path (needs z3 on PATH; degrades to the sampler without it)")
	pathDepth := fs.Int("path-depth", 0, "list-unrolling depth for --path-cover (default 4)")
	pathMaxPaths := fs.Int("path-max-paths", 0, "cap on enumerated paths for --path-cover (default 64)")
	pathTimeoutMS := fs.Int("path-timeout-ms", 0, "per-query solver timeout in milliseconds for --path-cover (default 5000)")
	vacuity := fs.Bool("vacuity", true, "check every (datatype …) with verified premises for inhabitation; a vacuous datatype fails the gate")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: shen-derive verify <spec.shen> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fs.PrintDefaults()
	}

	if len(args) == 0 {
		fs.Usage()
		os.Exit(1)
	}
	specPath := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		os.Exit(1)
	}

	if *funcName == "" || *implPkgPath == "" || *implFunc == "" || *importPath == "" {
		fmt.Fprintln(os.Stderr, "error: --func, --impl-pkg, --impl-func, and --import are required")
		fs.Usage()
		os.Exit(1)
	}

	if *implPkgName == "" {
		*implPkgName = filepath.Base(*implPkgPath)
	}
	if *testPkg == "" {
		*testPkg = *implPkgName + "_test"
	}

	sf, err := specfile.ParseFile(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse spec: %v\n", err)
		os.Exit(1)
	}
	def := sf.FindDefine(*funcName)
	if def == nil {
		fmt.Fprintf(os.Stderr, "define %q not found in %s\n", *funcName, specPath)
		os.Exit(1)
	}

	tt := specfile.BuildTypeTable(sf.Datatypes, *importPath, *importAlias)

	// Vacuity pass, at spec load. An uninhabited datatype makes every
	// downstream claim empty, so it is worth knowing before a single
	// sample is generated. Without a solver the check is silently
	// skipped — the same clean degradation as path cover.
	var vacuousFindings []symbolic.VacuityFinding
	if *vacuity {
		solver, solverErr := symbolic.FindSolver(time.Duration(*pathTimeoutMS) * time.Millisecond)
		if solverErr == nil {
			findings, err := symbolic.CheckVacuity(tt, solver)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: vacuity check: %v\n", err)
			}
			for _, f := range findings {
				if f.Verdict == symbolic.VacuityVacuous {
					vacuousFindings = append(vacuousFindings, f)
					fmt.Fprintf(os.Stderr, "VACUOUS: %s\n", f.Message)
				}
			}
		}
	}

	allDefines := make([]*specfile.Define, len(sf.Defines))
	for i := range sf.Defines {
		allDefines[i] = &sf.Defines[i]
	}

	cfg := &verify.HarnessConfig{
		Spec:         def,
		TypeTable:    tt,
		AllDefines:   allDefines,
		ImplPkgPath:  *implPkgPath,
		ImplPkgName:  *implPkgName,
		ImplFunc:     *implFunc,
		TestPkgName:  *testPkg,
		MaxCases:     *maxCases,
		Seed:         *seed,
		RandomDraws:  *randomDraws,
		PathCover:    *pathCover,
		PathDepth:    *pathDepth,
		PathMaxPaths: *pathMaxPaths,
		PathTimeout:  time.Duration(*pathTimeoutMS) * time.Millisecond,
	}
	h, err := verify.BuildHarness(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build harness: %v\n", err)
		os.Exit(1)
	}
	source, err := h.Emit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "emit: %v\n", err)
		os.Exit(1)
	}

	if *out == "" {
		fmt.Print(source)
	} else {
		if err := os.WriteFile(*out, []byte(source), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d cases)\n", *out, len(h.Cases))
	}

	if h.PathStats != nil {
		ps := h.PathStats
		solver := ps.SolverName
		if !ps.SolverAvailable || solver == "" {
			solver = "none"
		}
		fmt.Fprintf(os.Stderr,
			"path cover: paths_total=%d paths_feasible=%d paths_dead=%d "+
				"(solver: %s, depth %d, %d sample(s) added)\n",
			ps.Total, ps.Feasible, ps.Dead, solver, ps.Depth, ps.SamplesAdded)
		if ps.DegradedReason != "" {
			fmt.Fprintf(os.Stderr, "path cover degraded: %s\n", ps.DegradedReason)
		}
		for _, w := range ps.Warnings {
			fmt.Fprintf(os.Stderr, "path cover warning: %s\n", w)
		}
	}

	if *reportOut != "" {
		seedLabel := "deterministic-default"
		if *seed != 0 {
			seedLabel = strconv.FormatInt(*seed, 10)
		}
		dischargeRules, err := report.ClassifyDatatypes(sf, *guardFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discharge classify: %v\n", err)
			os.Exit(1)
		}
		var pathInfo *report.PathCoverInfo
		if h.PathStats != nil {
			pathInfo = &report.PathCoverInfo{
				Total:           h.PathStats.Total,
				Feasible:        h.PathStats.Feasible,
				Dead:            h.PathStats.Dead,
				SolverName:      h.PathStats.SolverName,
				SolverAvailable: h.PathStats.SolverAvailable,
				Depth:           h.PathStats.Depth,
				SamplesAdded:    h.PathStats.SamplesAdded,
			}
		}
		dischargeRules = append(dischargeRules,
			report.ClassifyDefineWithPaths(specPath, def, len(h.Cases), seedLabel, *implFunc, pathInfo),
		)
		// An uninhabited datatype flips its rule to "vacuous" in the
		// report and fails the gate below.
		for _, f := range vacuousFindings {
			name := datatypeBlockFor(sf, f.Type)
			if !report.MarkVacuous(dischargeRules, name, f.Message) {
				fmt.Fprintf(os.Stderr, "warning: vacuous type %q has no matching report rule\n", f.Type)
			}
		}
		rep, err := report.Build(report.BuildOptions{
			SpecPath:          specPath,
			Now:               time.Now().UTC(),
			TargetLanguage:    "go",
			ShenDeriveVersion: version,
			Rules:             dischargeRules,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "discharge build: %v\n", err)
			os.Exit(1)
		}
		if err := rep.WriteFile(*reportOut); err != nil {
			fmt.Fprintf(os.Stderr, "discharge write: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote discharge report %s\n", *reportOut)
	}

	// The gate fails on a vacuous datatype. This is deliberately the
	// last thing the command does: the test file and the discharge
	// report are written first, so the operator (and `sb derive`) can
	// read the finding out of the artifact rather than only the log.
	if len(vacuousFindings) > 0 {
		fmt.Fprintf(os.Stderr,
			"\nshen-derive: %d uninhabited datatype(s) — failing. "+
				"An uninhabited guard type proves nothing: no program can construct a value of it, "+
				"so every rule that consumes one is empty.\n", len(vacuousFindings))
		os.Exit(1)
	}
}

// datatypeBlockFor maps a Shen type name (a datatype's conclusion) back
// to the name of the (datatype …) block that concludes it, which is the
// name the discharge report uses for the rule. They differ whenever the
// block is named for the invariant rather than for the type it builds
// (payment's `balance-invariant` concludes `balance-checked`).
func datatypeBlockFor(sf *specfile.SpecFile, typeName string) string {
	for _, dt := range sf.Datatypes {
		if dt.Name == typeName {
			return dt.Name
		}
		for _, r := range dt.Rules {
			if r.Conclusion.TypeName == typeName {
				return dt.Name
			}
		}
	}
	return typeName
}
