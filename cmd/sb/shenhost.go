package main

// shenhost.go — W6.A. One resolver for "which Shen host runs here".
//
// Before W6 the answer was spelled out four times: the repo-root
// bin/shen-check.sh, two per-example copies of it, and
// detectShenRuntimeHost in blame.go, which answered it by looking for
// the literal string "shen-sbcl". Each copy disagreed with the others
// about the flag form and about which binaries count, and the union of
// them was "no host in this container", so gate 4 was red everywhere
// and W5's second oracle never existed.
//
// The resolution order is the one the gate scripts documented and
// nothing implemented:
//
//	1. $SHEN            — an explicit path, which wins over everything
//	2. [shen] bin       — the project's own choice, from sb.toml
//	3. shen-sbcl        — shen-cl on SBCL, fastest startup
//	4. shen-scheme      — shen-scheme on Chez, fastest compute
//	5. shen             — any other port, which in this repository is
//	                      the Go port (make shen-go builds bin/shen)
//
// Steps 3–5 look on PATH; step 2 accepts a path relative to the
// project directory as well, because `[shen] bin = "../../bin/shen"`
// is how an example points at a host the repo built for itself.
//
// Every Shen port shares one CLI for this purpose:
//
//	shen eval -q -e '(tc +)' -l FILE ...
//
// Note that -q is a flag of the `eval` subcommand, not a top-level
// flag. The pre-W6 scripts wrote `shen -q -e … -l …`, which the Go
// port rejects outright; that alone would have kept gate 4 red even
// with a host installed.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ShenInstallHint is printed wherever a host is wanted and missing.
// It names the in-repo route first, because that one works offline
// after a single build and needs no package manager.
const ShenInstallHint = `no Shen host found. Either:
    make shen-go                # builds the Go port into bin/shen (needs Go 1.27; GOTOOLCHAIN=auto fetches it)
    export SHEN=/path/to/shen   # any Shen port's launcher
    brew tap Shen-Language/homebrew-shen && brew install shen-sbcl
  or set [shen] bin = "…" in sb.toml.`

// ShenHost is a resolved Shen host: which binary, how it was found,
// and what it says about itself.
type ShenHost struct {
	// Name is the host's identity for the report: the binary's base
	// name ("shen", "shen-sbcl", "shen-scheme").
	Name string
	// Path is the binary as it will be executed.
	Path string
	// Version is the first line of `<host> --version`, verbatim.
	Version string
	// Source says which step of the resolution order answered:
	// "SHEN", "sb.toml", or "PATH".
	Source string
}

// Found reports whether a host was resolved.
func (h *ShenHost) Found() bool { return h != nil && h.Path != "" }

// String renders the host for the toolchain block and gate output.
func (h *ShenHost) String() string {
	if !h.Found() {
		return "none"
	}
	if h.Version == "" {
		return h.Name
	}
	return h.Name + " (" + h.Version + ")"
}

// ResolveShenHost walks the resolution order above and returns the
// first host it finds, or nil. cfg may be nil, which skips step 2.
//
// A candidate counts as a host only if it is executable *and* answers
// `--version`: a `shen` on PATH that is a wrapper script pointing at a
// missing runtime is worse than no host, because it would make gate 4
// fail for a reason that has nothing to do with the spec.
func ResolveShenHost(cfg *Config) *ShenHost {
	type cand struct{ bin, source string }
	var cands []cand
	if v := strings.TrimSpace(os.Getenv("SHEN")); v != "" {
		cands = append(cands, cand{v, "SHEN"})
	}
	if cfg != nil && strings.TrimSpace(cfg.ShenBin) != "" {
		cands = append(cands, cand{strings.TrimSpace(cfg.ShenBin), "sb.toml"})
	}
	for _, n := range []string{"shen-sbcl", "shen-scheme", "shen"} {
		cands = append(cands, cand{n, "PATH"})
	}

	for _, c := range cands {
		path := resolveShenBinary(c.bin)
		if path == "" {
			continue
		}
		ver := firstLineOf(path, "--version")
		if ver == "" {
			continue
		}
		return &ShenHost{
			Name:    strings.TrimSuffix(filepath.Base(path), ".exe"),
			Path:    path,
			Version: ver,
			Source:  c.source,
		}
	}
	return nil
}

// resolveShenBinary turns a candidate into an executable path, or "".
// A name with no separator is looked up on PATH; anything else is
// treated as a path, relative ones against the working directory.
func resolveShenBinary(bin string) string {
	if strings.ContainsRune(bin, filepath.Separator) {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return ""
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			return ""
		}
		return abs
	}
	p, err := exec.LookPath(bin)
	if err != nil {
		return ""
	}
	return p
}

// ShenEvalArgs builds the argument vector for a type-checked load:
// `eval -q` (the -q belongs to the subcommand), the expressions in
// order, then every file in order. Both the caller's expressions and
// the files are evaluated left to right, so a prelude that must load
// *before* `(tc +)` is passed as a pre-file rather than as a file.
func ShenEvalArgs(preFiles []string, exprs []string, files []string) []string {
	args := []string{"eval", "-q"}
	for _, f := range preFiles {
		args = append(args, "-l", f)
	}
	for _, e := range exprs {
		args = append(args, "-e", e)
	}
	for _, f := range files {
		args = append(args, "-l", f)
	}
	return args
}

// printShenMissing writes the install hint to stderr under a caller's
// prefix, so every place that wants a host says the same thing.
func printShenMissing(prefix string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", prefix, ShenInstallHint)
}
