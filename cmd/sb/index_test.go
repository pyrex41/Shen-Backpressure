package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIndexersCoverConfiguredLanguages pins the language→indexer
// table. Adding a language to the flow path is adding an entry here
// and nothing else.
func TestIndexersCoverConfiguredLanguages(t *testing.T) {
	for _, lang := range []string{"go", "ts"} {
		spec, ok := indexers[lang]
		if !ok {
			t.Fatalf("no indexer for lang %q", lang)
		}
		if spec.Name == "" || spec.Install == "" || len(spec.Exts) == 0 {
			t.Errorf("%s: incomplete indexer spec %+v", lang, spec)
		}
		args := spec.Args(".sb/index.scip")
		found := false
		for _, a := range args {
			if a == ".sb/index.scip" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: args %v do not name the output path", lang, args)
		}
	}
}

// TestEnsureIndexUnknownLanguageDegrades is the honesty path: a
// project in a language with no indexer wired must not fail the gate,
// it must report that no evidence is available.
func TestEnsureIndexUnknownLanguageDegrades(t *testing.T) {
	out, err := ensureIndex(&Config{Lang: "cobol"}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Available {
		t.Error("an unknown language reported an available indexer")
	}
}

// TestTreeHashIsContentAddressed checks the cache key: same contents,
// same hash; changed contents, changed hash. Content and not mtime,
// because a Ralph loop rewrites files constantly and a mtime-keyed
// cache would miss a revert.
func TestTreeHashIsContentAddressed(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a\n")
	write("sub/b.go", "package b\n")
	write("notes.md", "ignored\n")

	h1, err := treeHash(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	h2, err := treeHash(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Error("tree hash is not deterministic")
	}

	// A file of an unwatched extension must not move the hash.
	write("notes.md", "still ignored, but longer\n")
	h3, _ := treeHash(dir, []string{".go"})
	if h3 != h1 {
		t.Error("an unwatched file changed the tree hash")
	}

	// A watched file must.
	write("sub/b.go", "package b\n\nfunc F() {}\n")
	h4, _ := treeHash(dir, []string{".go"})
	if h4 == h1 {
		t.Error("a watched file did not change the tree hash")
	}
}

// TestTreeHashSkipsGeneratedAndStagedTrees keeps the cache key from
// churning on .sb/ artifacts and keeps the .go.bak staging
// directories out of the hash entirely.
func TestTreeHashSkipsGeneratedAndStagedTrees(t *testing.T) {
	dir := t.TempDir()
	must := func(rel, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("a.go", "package a\n")
	base, err := treeHash(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, skipped := range []string{".sb/x.go", "node_modules/y.go", "vendor/z.go", ".git/w.go", "bypass_attempts/v.go"} {
		must(skipped, "package skipped\n")
	}
	after, err := treeHash(dir, []string{".go"})
	if err != nil {
		t.Fatal(err)
	}
	if after != base {
		t.Error("a skipped directory changed the tree hash")
	}
}

// TestWriteFileAtomic checks the artifact writer creates the parent
// directory and replaces an existing file.
func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "facts.shen")
	if err := writeFileAtomic(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two" {
		t.Errorf("content = %q, want two", got)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("left %d files behind, want 1 (no temp files)", len(entries))
	}
}
