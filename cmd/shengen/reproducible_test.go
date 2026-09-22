package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCanonicalSpecPathIsCwdIndependent is the regression test for the
// nondeterminism W5.1 closed: the same spec file reached by two
// different relative paths must canonicalise to one string.
func TestCanonicalSpecPathIsCwdIndependent(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "examples", "payment")
	specDir := filepath.Join(proj, "specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The project marker that anchors the canonical path.
	if err := os.WriteFile(filepath.Join(proj, "sb.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(specDir, "core.shen")
	if err := os.WriteFile(spec, []byte("(datatype foo)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	// From the project directory.
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	fromProject := canonicalSpecPath("specs/core.shen")

	// From two levels up, where the caller types a longer path.
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	fromRoot := canonicalSpecPath("examples/payment/specs/core.shen")

	if fromProject != fromRoot {
		t.Fatalf("canonical path is cwd-dependent: %q from project vs %q from root", fromProject, fromRoot)
	}
	if fromProject != "specs/core.shen" {
		t.Errorf("canonicalSpecPath = %q, want specs/core.shen", fromProject)
	}
}

// TestCanonicalSpecPathNoMarker falls back to the base name, which is
// still the same string from every cwd.
func TestCanonicalSpecPathNoMarker(t *testing.T) {
	// A path that cannot exist and has no project marker above it in
	// the temp dir still canonicalises deterministically. We can't
	// guarantee the absence of markers above t.TempDir() on every
	// machine, so assert only the invariant that matters: two spellings
	// of one path agree.
	dir := t.TempDir()
	spec := filepath.Join(dir, "core.shen")
	if err := os.WriteFile(spec, []byte("(datatype foo)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := canonicalSpecPath(spec)
	b := canonicalSpecPath(filepath.Join(dir, ".", "core.shen"))
	if a != b {
		t.Errorf("canonicalSpecPath disagrees on two spellings: %q vs %q", a, b)
	}
}

// TestBrandTableJSONBindsPairedPremises pins the `bound` field that the
// discharge report's guard-brand-bound basis rests on: safe-transfer's
// two premises share a brand, transaction's do not.
func TestBrandTableJSONBindsPairedPremises(t *testing.T) {
	bt := brandTableForFile(t, filepath.Join("..", "..", "examples", "payment", "specs", "core.shen"))
	out := BrandTableToJSON(bt, "specs/core.shen", "test")

	byName := map[string]BrandTypeJSON{}
	for _, e := range out.Types {
		byName[e.ShenName] = e
	}

	safe, ok := byName["safe-transfer"]
	if !ok {
		t.Fatalf("no safe-transfer entry; got %v", byName)
	}
	if !safe.Bound || len(safe.BoundPremises) != 2 {
		t.Errorf("safe-transfer should bind both premises: %+v", safe)
	}
	if safe.Signature != "SafeTransfer[B]" {
		t.Errorf("safe-transfer signature = %q, want SafeTransfer[B]", safe.Signature)
	}

	// balance-checked binds only its transaction premise; the bare
	// number it also takes is a value, not evidence.
	bc, ok := byName["balance-checked"]
	if !ok {
		t.Fatalf("no balance-checked entry; got %v", byName)
	}
	if len(bc.BoundPremises) != 1 || bc.BoundPremises[0] != 1 {
		t.Errorf("balance-checked should bind premise 1 only: %+v", bc)
	}

	// transaction mints a fresh brand, so none of its premises is
	// paired with anything.
	if tx, ok := byName["transaction"]; ok && tx.Bound {
		t.Errorf("transaction mints its own brand and binds nothing; got %+v", tx)
	}

	// Values stay out of it entirely.
	if a, ok := byName["amount"]; ok && a.Bound {
		t.Errorf("amount is a value, not evidence; got %+v", a)
	}

	// Round-trips as JSON.
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var back BrandTableJSON
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Types) != len(out.Types) {
		t.Errorf("round-trip lost entries: %d vs %d", len(back.Types), len(out.Types))
	}
}

// TestBrandTableJSONNilTable is the --no-brands case: no inference ran,
// so the file must still be valid JSON with an empty type list rather
// than a null.
func TestBrandTableJSONNilTable(t *testing.T) {
	out := BrandTableToJSON(nil, "specs/core.shen", "test")
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("empty output")
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	types, ok := back["types"].([]any)
	if !ok {
		t.Fatalf("types is not an array: %#v", back["types"])
	}
	if len(types) != 0 {
		t.Errorf("want empty types, got %d", len(types))
	}
}
