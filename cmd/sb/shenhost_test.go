package main

// shenhost_test.go — W6.A. The one resolver, and the flag form that
// kept gate 4 red.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// findTestShenHost locates a host for the tests that need a live one.
// It deliberately does not go through ResolveShenHost, which reads
// sb.toml from the working directory: a test that used it would be
// testing its own fixture.
func findTestShenHost(t *testing.T) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("SHEN")); v != "" {
		if p, err := exec.LookPath(v); err == nil {
			return p
		}
		if fi, err := os.Stat(v); err == nil && !fi.IsDir() {
			abs, _ := filepath.Abs(v)
			return abs
		}
	}
	for _, n := range []string{"shen-sbcl", "shen-scheme", "shen"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	// cmd/sb → repo root → bin/shen, which `make shen-go` builds.
	abs, err := filepath.Abs(filepath.Join("..", "..", "bin", "shen"))
	if err != nil {
		return ""
	}
	if fi, statErr := os.Stat(abs); statErr == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
		return abs
	}
	return ""
}

// TestShenEvalArgsFlagForm pins the invocation the pre-W6 scripts got
// wrong. `-q` is a flag of the `eval` subcommand, not a top-level
// flag: `shen -q -e '(tc +)' -l spec` is rejected outright by the Go
// port, so gate 4 would have failed even with a host installed.
//
// Ordering is load-bearing too: a file passed before the expressions
// is loaded before them, which is how the prelude's `declare` forms
// get in before `(tc +)` turns the typechecker on.
func TestShenEvalArgsFlagForm(t *testing.T) {
	got := ShenEvalArgs([]string{"declares.shen"}, []string{"(tc +)"}, []string{"defines.shen", "spec.shen"})
	want := []string{"eval", "-q",
		"-l", "declares.shen",
		"-e", "(tc +)",
		"-l", "defines.shen",
		"-l", "spec.shen"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d: got %q, want %q (whole: %v)", i, got[i], want[i], got)
		}
	}
	if got[0] != "eval" || got[1] != "-q" {
		t.Error("-q must follow the eval subcommand, not precede it")
	}
}

// TestResolveShenHostPrefersEnv checks the first step of the order.
func TestResolveShenHostPrefersEnv(t *testing.T) {
	host := findTestShenHost(t)
	if host == "" {
		t.Skip("no Shen host available")
	}
	t.Setenv("SHEN", host)
	h := ResolveShenHost(&Config{ShenBin: "/definitely/not/here/shen"})
	if !h.Found() {
		t.Fatal("$SHEN did not resolve a host")
	}
	if h.Source != "SHEN" {
		t.Errorf("source: got %q, want SHEN", h.Source)
	}
	if h.Version == "" {
		t.Error("the resolver recorded no --version output; the toolchain block would have nothing to say")
	}
}

// TestResolveShenHostUsesConfigBin checks the second step, which is
// how an example points at the host `make shen-go` built without
// anybody exporting anything.
func TestResolveShenHostUsesConfigBin(t *testing.T) {
	host := findTestShenHost(t)
	if host == "" {
		t.Skip("no Shen host available")
	}
	t.Setenv("SHEN", "")
	h := ResolveShenHost(&Config{ShenBin: host})
	if !h.Found() {
		t.Fatal("[shen] bin did not resolve a host")
	}
	if h.Source != "sb.toml" {
		t.Errorf("source: got %q, want sb.toml", h.Source)
	}
}

// TestResolveShenHostRejectsANonAnsweringBinary: a candidate that
// exists but cannot answer --version is not a host. A wrapper script
// pointing at a missing runtime would otherwise make gate 4 fail for a
// reason that has nothing to do with the spec.
func TestResolveShenHostRejectsANonAnsweringBinary(t *testing.T) {
	t.Setenv("SHEN", "")
	dir := t.TempDir()
	fake := filepath.Join(dir, "shen")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := ResolveShenHost(&Config{ShenBin: fake})
	// It may still fall through to a real host on PATH; what must not
	// happen is the fake being accepted.
	if h.Found() && h.Path == fake {
		t.Error("a binary that answers nothing to --version was accepted as a host")
	}
}

// TestDetectShenRuntimeHostAsksAboutTheHost, not about the spec.
// Before W6 this function was `detectShenRuntime(cfg) == "shen-sbcl"`,
// which reports what the spec's `:runtime-via` markers say — so a
// project sitting next to a working host was told it had none, and
// W5's second oracle could never exist.
func TestDetectShenRuntimeHostAsksAboutTheHost(t *testing.T) {
	host := findTestShenHost(t)
	if host == "" {
		t.Skip("no Shen host available")
	}
	t.Setenv("SHEN", host)
	// A config with no derive specs at all: detectShenRuntime returns
	// "" for it, so the pre-W6 implementation would have said false.
	if !detectShenRuntimeHost(&Config{}) {
		t.Error("a resolvable host was not recognised as a second oracle")
	}
	t.Setenv("SHEN", "/definitely/not/here/shen")
	// With nothing on PATH either this is false; on a machine with a
	// system-wide shen it stays true, which is correct, so only assert
	// the negative when the environment really has no host.
	if _, err := exec.LookPath("shen"); err != nil {
		if _, err := exec.LookPath("shen-sbcl"); err != nil {
			if detectShenRuntimeHost(&Config{}) {
				t.Error("no host is available but one was reported")
			}
		}
	}
}
