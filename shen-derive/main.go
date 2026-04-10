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
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
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

	allDefines := make([]*specfile.Define, len(sf.Defines))
	for i := range sf.Defines {
		allDefines[i] = &sf.Defines[i]
	}

	cfg := &verify.HarnessConfig{
		Spec:        def,
		TypeTable:   tt,
		AllDefines:  allDefines,
		ImplPkgPath: *implPkgPath,
		ImplPkgName: *implPkgName,
		ImplFunc:    *implFunc,
		TestPkgName: *testPkg,
		MaxCases:    *maxCases,
		Seed:        *seed,
		RandomDraws: *randomDraws,
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
		return
	}
	if err := os.WriteFile(*out, []byte(source), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d cases)\n", *out, len(h.Cases))
}
