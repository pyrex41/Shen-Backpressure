package main

// prelude_cmd.go — W6.B. `shen-derive prelude` writes the typed Shen
// prelude for a spec, so gate 4 can run tc+ over a spec whose defines
// use shen-derive's evaluator intrinsics.
//
// See shen-derive/prelude/prelude.go for what is emitted and why, and
// docs/TRUST-MODEL.md for the trust the prelude carries.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/prelude"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// PreludeDeclaresName and PreludeDefinesName are the two halves of the
// prelude. The names are part of the contract with `sb shen-check`,
// which loads them in this order around `(tc +)`.
const (
	PreludeDeclaresName = "prelude.declares.shen"
	PreludeDefinesName  = "prelude.defines.shen"
)

func cmdPrelude(args []string) {
	fs := flag.NewFlagSet("prelude", flag.ExitOnError)
	outDir := fs.String("out-dir", ".", "directory to write "+PreludeDeclaresName+" and "+PreludeDefinesName+" into")
	quiet := fs.Bool("quiet", false, "do not print the paths written or the gaps found")
	specFlag := fs.String("spec", "", "path to the .shen spec (alternative to the positional argument)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `shen-derive prelude [--out-dir DIR] SPEC.shen

Writes the typed Shen prelude for SPEC: %s (loaded before
(tc +)) and %s (loaded after it). Run tc+ over the pair with

  shen eval -q -l DIR/%s -e '(tc +)' \
       -l DIR/%s -l SPEC.shen

Intrinsics the generator cannot type are reported as GAP comments in
the declares file and on stderr; exit status is still 0, because a gap
is a fact about the spec, not a failure of this command. The tc+ run
is where it becomes a failure.

Flags:
`, PreludeDeclaresName, PreludeDefinesName, PreludeDeclaresName, PreludeDefinesName)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	// Go's flag package stops at the first non-flag argument, so
	// `prelude SPEC --out-dir D` would silently ignore --out-dir.
	// --spec is the form callers should use; the positional stays for
	// hand use and is only read when --spec is absent.
	specPath := *specFlag
	if specPath == "" {
		if fs.NArg() < 1 {
			fs.Usage()
			os.Exit(2)
		}
		specPath = fs.Arg(0)
	}

	sf, err := specfile.ParseFile(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shen-derive prelude: %v\n", err)
		os.Exit(1)
	}

	p := prelude.Build(sf)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "shen-derive prelude: %v\n", err)
		os.Exit(1)
	}
	declPath := filepath.Join(*outDir, PreludeDeclaresName)
	defPath := filepath.Join(*outDir, PreludeDefinesName)
	if err := os.WriteFile(declPath, []byte(p.Declares), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "shen-derive prelude: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(defPath, []byte(p.Defines), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "shen-derive prelude: %v\n", err)
		os.Exit(1)
	}
	if !*quiet {
		fmt.Fprintf(os.Stderr, "shen-derive prelude: %s\nshen-derive prelude: %s\n", declPath, defPath)
		for _, n := range p.Notes {
			fmt.Fprintf(os.Stderr, "shen-derive prelude: GAP: %s\n", n)
		}
	}
}
