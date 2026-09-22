package scip

import (
	"encoding/binary"
	"testing"
)

// TestRoundTrip builds an index with the test-support encoder and
// checks the reader recovers every field the flow gate depends on.
// This is the hand-constructed half of the fixture story: no protobuf
// toolchain is involved on either side.
func TestRoundTrip(t *testing.T) {
	want := &Index{Documents: []Document{{
		RelativePath: "internal/handlers/handlers.go",
		Language:     "go",
		Occurrences: []Occurrence{
			{
				Range:          []int32{88, 17, 88, 36},
				EnclosingRange: []int32{88, 0, 139, 1},
				Symbol:         "scip-go gomod m v `m/internal/handlers`/Server#handleListResources().",
				SymbolRoles:    SymbolRoleDefinition,
			},
			{
				Range:  []int32{99, 25, 42},
				Symbol: "scip-go gomod m v `m/internal/verified`/CheckTenantAccess().",
			},
		},
	}}}

	got, err := Parse(Encode(want))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.Documents) != 1 {
		t.Fatalf("got %d documents, want 1", len(got.Documents))
	}
	doc := got.Documents[0]
	if doc.RelativePath != want.Documents[0].RelativePath || doc.Language != "go" {
		t.Errorf("document header = %q/%q", doc.RelativePath, doc.Language)
	}
	if len(doc.Occurrences) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(doc.Occurrences))
	}

	def := doc.Occurrences[0]
	if !def.IsDefinition() {
		t.Error("definition role lost")
	}
	if def.Symbol != want.Documents[0].Occurrences[0].Symbol {
		t.Errorf("symbol = %q", def.Symbol)
	}
	span, ok := NormalizeRange(def.EnclosingRange)
	if !ok || span != (Span{StartLine: 88, StartCol: 0, EndLine: 139, EndCol: 1}) {
		t.Errorf("enclosing span = %+v ok=%v", span, ok)
	}

	ref := doc.Occurrences[1]
	if ref.IsDefinition() {
		t.Error("reference reported as a definition")
	}
	// The three-element single-line range form.
	span, ok = NormalizeRange(ref.Range)
	if !ok || span != (Span{StartLine: 99, StartCol: 25, EndLine: 99, EndCol: 42}) {
		t.Errorf("reference span = %+v ok=%v", span, ok)
	}
}

// TestUnknownFieldsAreSkipped is the forward-compatibility property:
// an index from a newer scip.proto must still decode.
func TestUnknownFieldsAreSkipped(t *testing.T) {
	occ := encodeOccurrence(Occurrence{
		Range:       []int32{1, 2, 3},
		Symbol:      "s m p v `x`/F().",
		SymbolRoles: SymbolRoleDefinition,
	})
	// Field 9 (not in our subset) as a varint, plus field 12 as bytes.
	occ = appendVarintKey(occ, 9, wireVarint)
	occ = binary.AppendUvarint(occ, 4242)
	occ = appendTagged(occ, 12, []byte("future"))

	doc := appendTagged(nil, 1, []byte("a.go"))
	doc = appendTagged(doc, 2, occ)
	// Document field 3 (symbols) and field 5 (text) are skipped.
	doc = appendTagged(doc, 3, []byte("ignored"))
	doc = appendTagged(doc, 5, []byte("package a"))

	idxBytes := appendTagged(nil, 1, []byte("metadata-ignored"))
	idxBytes = appendTagged(idxBytes, 2, doc)
	idxBytes = appendTagged(idxBytes, 3, []byte("external-symbols-ignored"))

	idx, err := Parse(idxBytes)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(idx.Documents) != 1 || len(idx.Documents[0].Occurrences) != 1 {
		t.Fatalf("got %+v", idx)
	}
	if idx.Documents[0].Occurrences[0].Symbol != "s m p v `x`/F()." {
		t.Errorf("symbol = %q", idx.Documents[0].Occurrences[0].Symbol)
	}
}

// TestTruncatedIndexIsAnError matters for the trust model: a
// half-decoded index would understate the reference set, and an
// understated reference set makes a flow premise look discharged when
// it is not. Failing loudly is the only safe behaviour.
func TestTruncatedIndexIsAnError(t *testing.T) {
	full := Encode(&Index{Documents: []Document{{
		RelativePath: "a.go",
		Occurrences:  []Occurrence{{Range: []int32{1, 2, 3}, Symbol: "s m p v `x`/F()."}},
	}}})
	for _, cut := range []int{1, len(full) / 2, len(full) - 1} {
		if _, err := Parse(full[:cut]); err == nil {
			t.Errorf("truncating to %d bytes decoded without error", cut)
		}
	}
}

func TestNormalizeRangeRejectsMalformed(t *testing.T) {
	for _, r := range [][]int32{nil, {1}, {1, 2}, {1, 2, 3, 4, 5}} {
		if _, ok := NormalizeRange(r); ok {
			t.Errorf("NormalizeRange(%v) accepted a malformed range", r)
		}
	}
}

func TestSpanContainsAndLines(t *testing.T) {
	s := Span{StartLine: 5, StartCol: 2, EndLine: 9, EndCol: 4}
	if !s.Contains(7, 0) || s.Contains(5, 1) || s.Contains(9, 5) || s.Contains(10, 0) {
		t.Error("Span.Contains is wrong at a boundary")
	}
	if s.Lines() != 4 {
		t.Errorf("Lines() = %d, want 4", s.Lines())
	}
}
