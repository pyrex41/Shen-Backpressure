// shen-derive — Calculational derivation tool for Shen-Backpressure.
//
// Derives efficient implementations from naive specifications by applying
// named algebraic laws from the Bird-Meertens (Squiggol) catalog. Each
// rewrite step emits side-condition obligations for validation/checking.
//
// Subcommands:
//   repl      Interactive evaluator (default if no subcommand)
//   parse     Parse a spec and pretty-print the AST
//   check     Type-check a spec
//   eval      Evaluate a spec on test inputs
//   rewrite   Apply a single rewrite rule
//   lower     Lower a term to Go
//   laws      List all available laws with citations

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/codegen"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/shen"
)

const version = "0.1.0"

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
	case "check":
		cmdCheck(os.Args[2:])
	case "rewrite":
		cmdRewrite(os.Args[2:])
	case "lower":
		cmdLower(os.Args[2:])
	case "laws":
		cmdLaws()
	case "version", "--version", "-v":
		fmt.Printf("shen-derive %s\n", version)
	case "help", "--help", "-h":
		usage()
	default:
		// Try to evaluate the entire argument as an expression
		expr := strings.Join(os.Args[1:], " ")
		evalAndPrint(expr)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `shen-derive — Calculational derivation tool (v%s)

Usage: shen-derive <command> [args]

Commands:
  repl                      Interactive evaluator (default)
  parse   <expr>            Parse and pretty-print an expression
  check   <expr>            Type-check an expression
  eval    <expr>            Evaluate an expression
  rewrite [--bind '?x=expr'] <expr> <rule>
                            Apply a named rewrite rule at root
  lower   <expr>            Lower a term to Go (prints to stdout)
  laws                      List all available rewrite laws
  version                   Print version

Running with no arguments starts the REPL.

REPL commands:
  :t <expr>    Show type
  :p <expr>    Show parsed AST
  :q           Quit
`, version)
}

// --- REPL ---

func runREPL() {
	fmt.Fprintf(os.Stderr, "shen-derive %s — interactive evaluator\n", version)
	fmt.Fprintf(os.Stderr, "Type expressions to evaluate. Use :q to quit, :t <expr> to show type.\n\n")

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
		if strings.HasPrefix(line, ":t ") {
			showType(strings.TrimPrefix(line, ":t "))
			continue
		}
		if strings.HasPrefix(line, ":p ") {
			showParse(strings.TrimPrefix(line, ":p "))
			continue
		}
		evalAndPrint(line)
	}
	fmt.Fprintln(os.Stderr)
}

// --- Subcommands ---

func cmdEval(args []string) {
	if len(args) > 0 {
		evalAndPrint(strings.Join(args, " "))
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		evalAndPrint(line)
	}
}

func cmdParse(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shen-derive parse <expression>")
		os.Exit(1)
	}
	input := strings.Join(args, " ")
	term, err := core.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(core.PrettyPrint(term))
}

func cmdCheck(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shen-derive check <expression>")
		os.Exit(1)
	}
	input := strings.Join(args, " ")
	term, err := core.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}
	ty, err := core.CheckTerm(term)
	if err != nil {
		fmt.Fprintf(os.Stderr, "type error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s\n", ty.String())
}

func cmdRewrite(args []string) {
	opts, err := parseRewriteArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rewrite error: %v\n\n", err)
		printRewriteUsage()
		os.Exit(1)
	}

	term, err := core.Parse(opts.Expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	rule := laws.LookupRule(opts.RuleName)
	if rule == nil {
		fmt.Fprintf(os.Stderr, "unknown rule: %q\n\n", opts.RuleName)
		printRewriteUsage()
		os.Exit(1)
	}

	result, err := shen.RewriteLazy(term, rule, laws.RootPath, opts.Extra)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rewrite error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Rule:    %s\n", result.RuleName)
	fmt.Printf("Before:  %s\n", core.PrettyPrint(result.Original))
	fmt.Printf("After:   %s\n", core.PrettyPrint(result.Rewritten))

	if len(result.Obligations) > 0 {
		fmt.Printf("Obligations (%d):\n", len(result.Obligations))
		for i, ob := range result.Obligations {
			fmt.Printf("  %d. %s\n", i+1, ob.Description)
			fmt.Printf("     %s = %s\n", core.PrettyPrint(ob.LHS), core.PrettyPrint(ob.RHS))
		}
	}
}

func cmdLower(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shen-derive lower <expression>")
		os.Exit(1)
	}
	input := strings.Join(args, " ")
	term, err := core.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}
	ty, err := core.CheckTerm(term)
	if err != nil {
		fmt.Fprintf(os.Stderr, "type error: %v\n", err)
		os.Exit(1)
	}
	goCode, err := codegen.LowerToGo(term, ty, "Derived", "derived")
	if err != nil {
		fmt.Fprintf(os.Stderr, "lower error: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(goCode)
}

func cmdLaws() {
	catalog := laws.Catalog()
	fmt.Printf("Available rewrite laws (%d):\n\n", len(catalog))
	for _, r := range catalog {
		fmt.Printf("  %s\n", r.Name)
		fmt.Printf("    LHS: %s\n", core.PrettyPrint(r.LHS))
		fmt.Printf("    RHS: %s\n", core.PrettyPrint(r.RHS))
		if len(r.SideConditions) > 0 {
			for _, sc := range r.SideConditions {
				fmt.Printf("    provided: %s\n", sc.Description)
			}
		} else {
			fmt.Printf("    (no side conditions)\n")
		}
		fmt.Printf("    cite: %s\n\n", r.Citation)
	}
}

// --- Helpers ---

func evalAndPrint(input string) {
	term, err := core.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		return
	}
	val, err := core.Eval(core.EmptyEnv(), term)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eval error: %v\n", err)
		return
	}
	fmt.Println(val.String())
}

func showType(input string) {
	term, err := core.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		return
	}
	ty, err := core.CheckTerm(term)
	if err != nil {
		fmt.Fprintf(os.Stderr, "type error: %v\n", err)
		return
	}
	fmt.Printf("%s :: %s\n", input, ty.String())
}

func showParse(input string) {
	term, err := core.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		return
	}
	fmt.Println(core.PrettyPrint(term))
}

type rewriteOptions struct {
	Expr     string
	RuleName string
	Extra    laws.Bindings
}

func parseRewriteArgs(args []string) (*rewriteOptions, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("usage: shen-derive rewrite [--bind '?x=expr'] <expression> <rule-name>")
	}

	var positional []string
	extra := make(laws.Bindings)

	for i := 0; i < len(args); i++ {
		if args[i] != "--bind" {
			positional = append(positional, args[i])
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("missing value after --bind")
		}
		name, term, err := parseRewriteBinding(args[i+1])
		if err != nil {
			return nil, err
		}
		if _, exists := extra[name]; exists {
			return nil, fmt.Errorf("duplicate binding for %s", name)
		}
		extra[name] = term
		i++
	}

	if len(positional) < 2 {
		return nil, fmt.Errorf("rewrite requires an expression and a rule name")
	}

	return &rewriteOptions{
		Expr:     strings.Join(positional[:len(positional)-1], " "),
		RuleName: positional[len(positional)-1],
		Extra:    extra,
	}, nil
}

func parseRewriteBinding(spec string) (string, core.Term, error) {
	name, expr, ok := strings.Cut(spec, "=")
	if !ok {
		return "", nil, fmt.Errorf("invalid binding %q: expected '?meta=<expr>'", spec)
	}
	name = strings.TrimSpace(name)
	expr = strings.TrimSpace(expr)
	if !laws.IsMetaVar(name) {
		return "", nil, fmt.Errorf("invalid binding %q: metavariable names must start with '?'", spec)
	}
	if expr == "" {
		return "", nil, fmt.Errorf("invalid binding %q: missing expression", spec)
	}

	term, err := core.Parse(expr)
	if err != nil {
		return "", nil, fmt.Errorf("binding %s parse error: %w", name, err)
	}
	if _, err := core.CheckTerm(term); err != nil {
		return "", nil, fmt.Errorf("binding %s type error: %w", name, err)
	}
	return name, term, nil
}

func printRewriteUsage() {
	fmt.Fprintln(os.Stderr, "usage: shen-derive rewrite [--bind '?x=expr'] <expression> <rule-name>")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  shen-derive rewrite 'map f . map g' map-fusion")
	fmt.Fprintln(os.Stderr, "  shen-derive rewrite --bind '?h=\\x z -> z - x' 'negate . foldr (+) 0' foldr-fusion")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Available rules:")
	for _, r := range laws.Catalog() {
		fmt.Fprintf(os.Stderr, "  %-20s %s\n", r.Name, r.Citation)
	}
}
