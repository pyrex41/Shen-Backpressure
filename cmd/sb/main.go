// sb — CLI tool for Shen-Backpressure projects.
//
// Provides deterministic tooling for projects using Shen sequent-calculus
// type specs and shengen guard type generation. Complements the sb/ skill
// bundle which teaches LLMs how to think about guard types.
//
// Subcommands:
//   init     Scaffold a new Shen-backpressure project
//   gen      Run shengen to generate guard types from specs
//   gates    Run all five verification gates
//   audit    Run Gate 5 (TCB audit) only
//   check    Run Gate 4 (shen tc+) only
//   context  Emit structured context for LLM harnesses

package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "gen":
		cmdGen(os.Args[2:])
	case "gates":
		cmdGates(os.Args[2:])
	case "audit":
		cmdAudit(os.Args[2:])
	case "check":
		cmdCheck(os.Args[2:])
	case "context":
		cmdContext(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("sb %s\n", version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "sb: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `sb — Shen-Backpressure CLI (v%s)

Usage: sb <command> [flags]

Commands:
  init      Scaffold a new Shen-backpressure project
  gen       Generate guard types from Shen specs
  gates     Run all five verification gates
  audit     Run Gate 5 (TCB audit) only
  check     Run Gate 4 (shen tc+) only
  context   Emit structured context for LLM harnesses
  version   Print version

Use "sb <command> --help" for more information about a command.
`, version)
}
