package main

// index.go — `sb index`: build the resolved symbol graph the flow gate
// reasons over.
//
// The step is: detect the project's language from sb.toml, run that
// language's SCIP indexer, decode index.scip, and write the flat fact
// file .sb/facts.shen. Everything downstream — the Shen Prolog rules
// and the Go engine alike — reads only that fact file, which is why
// the same (flow …) premise text runs unchanged against a Go tree and
// a TypeScript one.
//
// The indexer runs *after* the language's own type checker (it is
// built on it: scip-go loads packages with go/packages, scip-typescript
// with the TS compiler API). That is the whole reason a flow premise
// is stronger than a grep: aliasing, embedding, method values and
// re-exports are already resolved to a single symbol by the time we
// see them. It also means the indexer is in the TCB for flow premises;
// docs/TRUST-MODEL.md says so.
//
// Indexing is cached by tree hash under .sb/ so a Ralph iteration that
// did not touch source does not pay for it twice.

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/cmd/sb/flow"
	"github.com/pyrex41/Shen-Backpressure/cmd/sb/internal/scip"
)

// Artifact paths, all under the gitignored .sb/ directory.
const (
	// FactsPath is the fact file every flow engine reads.
	FactsPath = ".sb/facts.shen"
	// IndexSCIPPath is the raw index the indexer wrote.
	IndexSCIPPath = ".sb/index.scip"
	// IndexStampPath records the tree hash the facts were built from.
	IndexStampPath = ".sb/index.stamp"
)

// indexerSpec describes how to run one language's SCIP indexer.
type indexerSpec struct {
	// Name is the binary as it appears on PATH.
	Name string
	// Args are the arguments that make it write to out.
	Args func(out string) []string
	// Install is the one-line command that installs it, quoted in the
	// warning when it is missing.
	Install string
	// Exts are the source extensions whose contents feed the tree hash.
	Exts []string
}

// indexers maps a project language to its indexer. Adding a language
// is adding an entry here: nothing else in the flow path is
// language-specific.
var indexers = map[string]indexerSpec{
	"go": {
		Name:    "scip-go",
		Args:    func(out string) []string { return []string{"--output", out} },
		Install: "go install github.com/scip-code/scip-go/cmd/scip-go@latest",
		Exts:    []string{".go", ".mod"},
	},
	"ts": {
		Name:    "scip-typescript",
		Args:    func(out string) []string { return []string{"index", "--output", out} },
		Install: "npm i -g @sourcegraph/scip-typescript",
		Exts:    []string{".ts", ".tsx", ".json"},
	},
}

// IndexOutcome reports what `sb index` did, so the flow gate can be
// honest about which evidence it has.
type IndexOutcome struct {
	// Indexer is the tool that ran (or would have run).
	Indexer string
	// Available is false when no indexer for the language is on PATH.
	// The flow gate then falls back to the legacy grep and marks its
	// premises unproven.
	Available bool
	// Cached is true when a previous run's facts were reused.
	Cached bool
	// FactsPath is where the fact file landed.
	FactsPath string
	// Facts is the parsed fact set, nil when Available is false.
	Facts *flow.FactSet
	// InstallHint is the command that would make the indexer available.
	InstallHint string
}

func cmdIndex(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	force := fs.Bool("force", false, "re-run the indexer even when the cached facts are current")
	quiet := fs.Bool("quiet", false, "suppress progress output")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb index — Build the resolved symbol graph for flow premises

Usage: sb index [flags]

Detects the project language from sb.toml, runs that language's SCIP
indexer (scip-go for Go, scip-typescript for TypeScript), and writes
flat flow facts to %s:

  (def sym file start-line start-col end-line end-col)
  (ref sym file line col enclosing-def-sym)
  (calls caller-sym callee-sym)

Results are cached by tree hash; re-running without source changes is
a no-op. When no indexer is on PATH the command reports that and exits
0 — the flow gate degrades to the legacy grep rather than failing.

Flags:
`, FactsPath)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb index: %v\n", err)
		os.Exit(1)
	}
	out, err := ensureIndex(cfg, *force, !*quiet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb index: %v\n", err)
		os.Exit(1)
	}
	if !out.Available {
		fmt.Fprintf(os.Stderr, "sb index: no SCIP indexer for lang %q on PATH (install with: %s)\n",
			cfg.Lang, out.InstallHint)
		return
	}
	if *quiet {
		return
	}
	fmt.Fprintf(os.Stderr, "sb index: %s — %d defs, %d refs, %d calls → %s%s\n",
		out.Indexer, len(out.Facts.Defs), len(out.Facts.Refs), len(out.Facts.Calls),
		out.FactsPath, cachedSuffix(out.Cached))
}

func cachedSuffix(cached bool) string {
	if cached {
		return " (cached)"
	}
	return ""
}

// ensureIndex produces a current fact file, reusing the cached one
// when the tree hash still matches.
func ensureIndex(cfg *Config, force, verbose bool) (*IndexOutcome, error) {
	spec, ok := indexers[cfg.Lang]
	if !ok {
		return &IndexOutcome{Available: false, InstallHint: "no SCIP indexer is wired for this language"}, nil
	}
	out := &IndexOutcome{Indexer: spec.Name, FactsPath: FactsPath, InstallHint: spec.Install}

	bin, err := exec.LookPath(spec.Name)
	if err != nil {
		return out, nil
	}
	out.Available = true

	hash, err := treeHash(".", spec.Exts)
	if err != nil {
		return nil, fmt.Errorf("hashing tree: %w", err)
	}
	stamp := spec.Name + " " + hash

	if !force {
		if prev, err := os.ReadFile(IndexStampPath); err == nil && strings.TrimSpace(string(prev)) == stamp {
			if data, err := os.ReadFile(FactsPath); err == nil {
				facts, err := flow.ParseFacts(string(data))
				if err == nil {
					facts.Indexer = spec.Name
					out.Cached = true
					out.Facts = facts
					return out, nil
				}
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(FactsPath), 0o755); err != nil {
		return nil, err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "sb index: running %s…\n", spec.Name)
	}
	cmd := exec.Command(bin, spec.Args(IndexSCIPPath)...)
	if verbose {
		cmd.Stderr = os.Stderr
	}
	if combined, err := cmd.Output(); err != nil {
		return nil, fmt.Errorf("%s failed: %w\n%s", spec.Name, err, strings.TrimSpace(string(combined)))
	}

	raw, err := os.ReadFile(IndexSCIPPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", IndexSCIPPath, err)
	}
	idx, err := scip.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", IndexSCIPPath, err)
	}
	facts := flow.FromIndex(idx, spec.Name)
	if err := writeFileAtomic(FactsPath, []byte(facts.WriteFacts())); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(IndexStampPath, []byte(stamp+"\n")); err != nil {
		return nil, err
	}
	out.Facts = facts
	return out, nil
}

// treeHash is a content hash over every source file of interest,
// which is what decides whether the cached facts are still current.
// Content, not mtime: a Ralph loop rewrites files constantly and a
// mtime-keyed cache would miss a revert.
func treeHash(root string, exts []string) (string, error) {
	want := map[string]bool{}
	for _, e := range exts {
		want[e] = true
	}
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".sb", "node_modules", "vendor", "bypass_attempts", "forgeries":
				return filepath.SkipDir
			}
			return nil
		}
		if want[filepath.Ext(path)] {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s %d\n", p, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeFileAtomic writes data to path via a temp file and a rename.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sb-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
