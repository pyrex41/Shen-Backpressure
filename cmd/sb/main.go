// sb — Thin CLI for Shen-Backpressure projects.
//
// Scaffolds projects, runs shengen, executes verification gates, and
// launches Ralph loops (headless LLM harness). The intelligence lives
// in the skills (sb/ skill bundle) — this CLI just runs things.
//
// Subcommands:
//   init     Scaffold a new Shen-backpressure project (specs, scripts, skills)
//   gen      Run shengen to generate guard types from specs
//   gates    Run the five verification gates
//   loop     Launch a Ralph loop (headless LLM + five-gate verification)

package main

import (
	"fmt"
	"os"
)

const version = "0.2.0"

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
	fmt.Fprintf(os.Stderr, `sb — Shen-Backpressure CLI (v%s)

Usage: sb <command> [flags]

Commands:
  init      Scaffold a new Shen-backpressure project
  gen       Generate guard types from Shen specs
  gates     Run all five verification gates
  loop      Launch a Ralph loop (headless LLM + gates)
  version   Print version

Use "sb <command> --help" for more information about a command.
`, version)
}
