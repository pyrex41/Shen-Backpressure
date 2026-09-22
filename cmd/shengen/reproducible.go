package main

// reproducible.go — W5.1. `sb gen` output must be a pure function of
// the spec bytes and the shengen version, and nothing else. Two things
// used to leak into it:
//
//  1. The generated header embedded the spec path *as the caller typed
//     it*, so `shengen specs/core.shen` from examples/payment and
//     `shengen examples/payment/specs/core.shen` from the repo root
//     produced two different first lines for the same spec. That made
//     the drift check cwd-sensitive and the guards file unverifiable
//     from anywhere but the one directory the committer happened to
//     stand in.
//
//  2. Build-path metadata baked into the binary by the Go linker,
//     addressed by `-trimpath` in the Makefile rather than here.
//
// canonicalSpecPath fixes (1) by resolving the spec to an absolute
// path and then re-expressing it relative to the nearest enclosing
// project root — the directory holding sb.toml, go.mod, package.json,
// Cargo.toml, or .git. That anchor travels with the spec instead of
// with the caller, so the header is the same from every cwd and from
// every checkout location.

import (
	"os"
	"path/filepath"
)

// projectRootMarkers name the files whose presence identifies a
// project root. Ordered by specificity: a Shen-Backpressure manifest
// is the strongest signal, a VCS directory the weakest.
var projectRootMarkers = []string{
	"sb.toml",
	"go.mod",
	"package.json",
	"Cargo.toml",
	".git",
}

// canonicalSpecPath rewrites a spec path into the cwd-independent form
// recorded in the generated header: slash-separated and relative to
// the spec's nearest enclosing project root.
//
// Falls back to the file's base name when no root can be found (or the
// path cannot be resolved), which is still cwd-independent — the point
// is that the same spec file always yields the same string.
func canonicalSpecPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(filepath.Base(path))
	}
	abs = filepath.Clean(abs)
	root := findProjectRoot(filepath.Dir(abs))
	if root == "" {
		return filepath.ToSlash(filepath.Base(abs))
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return filepath.ToSlash(filepath.Base(abs))
	}
	return filepath.ToSlash(rel)
}

// findProjectRoot walks dir upwards looking for a project marker and
// returns the first directory that has one, or "" at the filesystem
// root. The spec directory itself is checked first; a spec that sits
// next to a go.mod is its own root.
func findProjectRoot(dir string) string {
	for {
		for _, marker := range projectRootMarkers {
			if _, err := os.Lstat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
