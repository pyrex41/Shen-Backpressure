// sb — Engine/orchestrator CLI for Shen-Backpressure projects.
//
// Manages the project manifest (sb.toml), runs manifest-defined
// verification gates, and orchestrates Ralph loops (headless LLM
// harness with gate feedback). The manifest is the single source of
// truth for project structure, gate pipeline, and loop configuration.
//
// Subcommands:
//   init      Scaffold a new Shen-backpressure project (specs, scripts, skills)
//   gen       Run shengen to generate guard types from specs
//   gates     Run manifest-defined verification gates
//   derive    Run spec-equivalence verification
//   context   Emit project context from the manifest
//   loop      Launch a Ralph loop (headless LLM + gate verification)

package main

import (
	"fmt"
	"os"
)

const version = "0.3.0"

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
	case "derive":
		cmdDerive(os.Args[2:])
	case "context":
		cmdContext(os.Args[2:])
	case "loop":
		cmdLoop(os.Args[2:])
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
	fmt.Fprintf(os.Stderr, `sb — Shen-Backpressure engine/orchestrator (v%s)

Usage: sb <command> [flags]

Commands:
  init      Scaffold a new Shen-backpressure project
  gen       Generate guard types from Shen specs
  gates     Run manifest-defined verification gates
  derive    Run spec-equivalence verification
  context   Emit project context from the manifest
  loop      Launch a Ralph loop (headless LLM + gates)
  version   Print version

Use "sb <command> --help" for more information about a command.
`, version)
}
