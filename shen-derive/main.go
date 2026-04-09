// shen-derive — Calculational derivation tool for Shen-Backpressure.
//
// Derives efficient implementations from naive specifications by applying
// named algebraic laws from the Bird-Meertens (Squiggol) catalog. Each
// rewrite step emits side-condition proof obligations discharged by Shen.
//
// Subcommands:
//   repl      Interactive evaluator (default if no subcommand)
//   parse     Parse a spec file and pretty-print the AST
//   check     Type-check a spec
//   eval      Evaluate a spec on test inputs
//   laws      List available rewrite laws

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
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

Usage: shen-derive [command] [args]

Commands:
  repl      Interactive evaluator (default)
  eval      Evaluate an expression from stdin or argument
  parse     Parse and pretty-print an expression
  check     Type-check an expression
  version   Print version

Running with no arguments starts the REPL.
`, version)
}

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
			expr := strings.TrimPrefix(line, ":t ")
			showType(expr)
			continue
		}
		if strings.HasPrefix(line, ":p ") {
			expr := strings.TrimPrefix(line, ":p ")
			showParse(expr)
			continue
		}
		evalAndPrint(line)
	}
	fmt.Fprintln(os.Stderr)
}

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
