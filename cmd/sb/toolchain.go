package main

// toolchain.go — W5.1. What produced this report.
//
// `sb gen` is a pure function of the spec bytes and the shengen
// binary. That claim is only checkable if the report says which
// binary, so every report records the emitter's sha256 alongside the
// versions of every other tool that contributed evidence: the Go
// toolchain, z3 when path-cover evidence is prover-backed, and each
// SCIP indexer when flow premises were evaluated.
//
// Everything here is best-effort. A tool that is absent is omitted
// rather than recorded as "unknown", because an absent field and a
// field claiming ignorance are different statements and only the first
// one is true.

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// DetectToolchain assembles the toolchain block for a report.
// shengenPath may be empty, in which case the emitter is looked up the
// same way `sb gen` looks it up.
func DetectToolchain(cfg *Config, shengenPath string) *DischargeToolchain {
	tc := &DischargeToolchain{
		Go:     runtime.Version(),
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}

	if shengenPath == "" {
		if p, err := FindShengen(); err == nil {
			shengenPath = p
		}
	}
	if shengenPath != "" {
		if sum, err := fileSHA256(shengenPath); err == nil {
			tc.ShengenSHA256 = sum
		}
		tc.ShengenVersion = firstLineOf(shengenPath, "--version")
	}

	// z3 only matters when a spec actually opted into path cover; a z3
	// on PATH that nothing consulted is not part of this report's
	// evidence.
	if cfg != nil && anySpecUsesPathCover(cfg) {
		if v := firstLineOf("z3", "--version"); v != "" {
			tc.Z3Version = v
		}
	}

	// W6 — the Shen host, whenever one is resolvable. Unlike z3 and
	// the indexers, it is not conditioned on a feature being switched
	// on: gate 4 is one of the fixed five, so every project's report
	// makes a tc+ claim and every report should say which typechecker
	// backed it. A report with no shen_host is one whose tc+ claim was
	// never checked by anything.
	if h := ResolveShenHost(cfg); h.Found() {
		tc.ShenHost = h.Name
		tc.ShenHostVersion = h.Version
	}

	// Indexers, likewise, only when a flow gate is configured.
	if cfg != nil && hasFlowGate(cfg) {
		for _, name := range []string{"scip-go", "scip-typescript"} {
			if _, err := exec.LookPath(name); err != nil {
				continue
			}
			if v := firstLineOf(name, "--version"); v != "" {
				tc.Indexers = append(tc.Indexers, DischargeToolVersion{Name: name, Version: v})
			}
		}
	}
	return tc
}

func anySpecUsesPathCover(cfg *Config) bool {
	for _, s := range cfg.DeriveSpecs {
		if s.PathCover {
			return true
		}
	}
	return false
}

func hasFlowGate(cfg *Config) bool {
	for _, g := range cfg.Gates {
		if g.Kind == GateKindFlow {
			return true
		}
	}
	return false
}

// firstLineOf runs `bin args...` and returns its first line of output,
// trimmed. Returns "" when the binary is missing or the call fails —
// this is identification, not a dependency.
func firstLineOf(bin string, args ...string) string {
	cmd := exec.Command(bin, args...)
	out, err := cmd.Output()
	if err != nil {
		// Some tools print their version to stderr, or exit non-zero
		// while still saying something useful.
		combined, cerr := exec.Command(bin, args...).CombinedOutput()
		if cerr != nil && len(combined) == 0 {
			return ""
		}
		out = combined
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

// fileSHA256 hashes a file's bytes.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ToolchainMismatch compares the toolchain a report records with the
// one verifying it now, and returns a human-readable list of
// differences. An empty result means "same tools".
//
// Differences are reported, never failed on: a report verified with a
// newer Go is not thereby wrong, and saying so is more useful than
// refusing. `sb verify-report` prints these as warnings and decides
// pass/fail on the re-derivation itself.
func ToolchainMismatch(recorded, current *DischargeToolchain) []string {
	if recorded == nil || current == nil {
		return nil
	}
	var out []string
	cmp := func(name, was, now string) {
		if was == "" || now == "" || was == now {
			return
		}
		out = append(out, name+": report says "+was+", this run has "+now)
	}
	cmp("go", recorded.Go, current.Go)
	cmp("goos", recorded.GOOS, current.GOOS)
	cmp("goarch", recorded.GOARCH, current.GOARCH)
	cmp("shengen version", recorded.ShengenVersion, current.ShengenVersion)
	cmp("shengen sha256", recorded.ShengenSHA256, current.ShengenSHA256)
	cmp("z3", recorded.Z3Version, current.Z3Version)
	cmp("shen host", recorded.ShenHost, current.ShenHost)
	cmp("shen host version", recorded.ShenHostVersion, current.ShenHostVersion)
	for _, was := range recorded.Indexers {
		for _, now := range current.Indexers {
			if was.Name == now.Name {
				cmp(was.Name, was.Version, now.Version)
			}
		}
	}
	return out
}
