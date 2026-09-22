// Package scip is a minimal, dependency-free reader for the subset of
// the SCIP (SCIP Code Intelligence Protocol) index format that sb's
// flow gate needs.
//
// Why hand-written: cmd/sb's module graph has no protobuf runtime
// (see cmd/sb/go.mod), and pulling google.golang.org/protobuf plus the
// generated bindings from github.com/sourcegraph/scip only to read
// four fields would put a large dependency inside the verifier's own
// trust boundary. The wire format we depend on is the stable part of
// scip.proto and is decoded here directly:
//
//	message Index    { repeated Document documents = 2; }
//	message Document { string relative_path = 1; repeated Occurrence occurrences = 2;
//	                   string language = 4; }
//	message Occurrence { repeated int32 range = 1; string symbol = 2;
//	                     int32 symbol_roles = 3; repeated int32 enclosing_range = 7; }
//
// Unknown fields are skipped by wire type, so indexes produced by
// newer scip.proto revisions still decode. Anything we cannot skip is
// a hard error: a silently truncated index would make the flow gate
// report a discharge it did not prove.
package scip

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// SymbolRoleDefinition is the SymbolRole.Definition bit from scip.proto.
const SymbolRoleDefinition = 0x1

// Index is the decoded subset of a SCIP index.
type Index struct {
	Documents []Document
}

// Document is one indexed source file.
type Document struct {
	RelativePath string
	Language     string
	Occurrences  []Occurrence
}

// Occurrence is one symbol reference or definition inside a document.
type Occurrence struct {
	// Range is the SCIP range: either [startLine, startChar, endLine,
	// endChar] or the 3-element single-line form [line, startChar,
	// endChar]. Use NormalizeRange to get a Span.
	Range []int32
	// EnclosingRange is the definition's full extent (body included)
	// when the indexer emits it; nil otherwise.
	EnclosingRange []int32
	Symbol         string
	SymbolRoles    int32
}

// IsDefinition reports whether this occurrence is a definition site.
func (o Occurrence) IsDefinition() bool {
	return o.SymbolRoles&SymbolRoleDefinition != 0
}

// Span is a normalized, zero-based source range.
type Span struct {
	StartLine, StartCol, EndLine, EndCol int32
}

// Contains reports whether s contains the position (line, col).
func (s Span) Contains(line, col int32) bool {
	if line < s.StartLine || line > s.EndLine {
		return false
	}
	if line == s.StartLine && col < s.StartCol {
		return false
	}
	if line == s.EndLine && col > s.EndCol {
		return false
	}
	return true
}

// Lines returns the number of lines the span covers. Used to pick the
// innermost of two containing definitions.
func (s Span) Lines() int32 { return s.EndLine - s.StartLine }

// NormalizeRange converts a SCIP range (3- or 4-element) into a Span.
// A malformed range yields ok=false.
func NormalizeRange(r []int32) (Span, bool) {
	switch len(r) {
	case 3:
		return Span{StartLine: r[0], StartCol: r[1], EndLine: r[0], EndCol: r[2]}, true
	case 4:
		return Span{StartLine: r[0], StartCol: r[1], EndLine: r[2], EndCol: r[3]}, true
	default:
		return Span{}, false
	}
}

// Parse decodes the SCIP index in data.
func Parse(data []byte) (*Index, error) {
	idx := &Index{}
	d := &decoder{buf: data}
	for !d.done() {
		field, wire, err := d.key()
		if err != nil {
			return nil, err
		}
		if field == 2 && wire == wireBytes { // documents
			sub, err := d.bytesField()
			if err != nil {
				return nil, err
			}
			doc, err := parseDocument(sub)
			if err != nil {
				return nil, err
			}
			idx.Documents = append(idx.Documents, doc)
			continue
		}
		if err := d.skip(wire); err != nil {
			return nil, err
		}
	}
	return idx, nil
}

func parseDocument(data []byte) (Document, error) {
	var doc Document
	d := &decoder{buf: data}
	for !d.done() {
		field, wire, err := d.key()
		if err != nil {
			return doc, err
		}
		switch {
		case field == 1 && wire == wireBytes: // relative_path
			s, err := d.stringField()
			if err != nil {
				return doc, err
			}
			doc.RelativePath = s
		case field == 2 && wire == wireBytes: // occurrences
			sub, err := d.bytesField()
			if err != nil {
				return doc, err
			}
			occ, err := parseOccurrence(sub)
			if err != nil {
				return doc, err
			}
			doc.Occurrences = append(doc.Occurrences, occ)
		case field == 4 && wire == wireBytes: // language
			s, err := d.stringField()
			if err != nil {
				return doc, err
			}
			doc.Language = s
		default:
			if err := d.skip(wire); err != nil {
				return doc, err
			}
		}
	}
	return doc, nil
}

func parseOccurrence(data []byte) (Occurrence, error) {
	var o Occurrence
	d := &decoder{buf: data}
	for !d.done() {
		field, wire, err := d.key()
		if err != nil {
			return o, err
		}
		switch {
		case field == 1: // range (packed or unpacked int32)
			vals, err := d.int32s(wire)
			if err != nil {
				return o, err
			}
			o.Range = append(o.Range, vals...)
		case field == 2 && wire == wireBytes: // symbol
			s, err := d.stringField()
			if err != nil {
				return o, err
			}
			o.Symbol = s
		case field == 3 && wire == wireVarint: // symbol_roles
			v, err := d.varint()
			if err != nil {
				return o, err
			}
			o.SymbolRoles = int32(v)
		case field == 7: // enclosing_range
			vals, err := d.int32s(wire)
			if err != nil {
				return o, err
			}
			o.EnclosingRange = append(o.EnclosingRange, vals...)
		default:
			if err := d.skip(wire); err != nil {
				return o, err
			}
		}
	}
	return o, nil
}

// --- wire decoding ---------------------------------------------------

const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireStart   = 3
	wireEnd     = 4
	wireFixed32 = 5
)

var errTruncated = errors.New("scip: truncated index")

type decoder struct {
	buf []byte
	pos int
}

func (d *decoder) done() bool { return d.pos >= len(d.buf) }

func (d *decoder) key() (field int, wire int, err error) {
	v, err := d.varint()
	if err != nil {
		return 0, 0, err
	}
	return int(v >> 3), int(v & 7), nil
}

func (d *decoder) varint() (uint64, error) {
	v, n := binary.Uvarint(d.buf[d.pos:])
	if n <= 0 {
		return 0, errTruncated
	}
	d.pos += n
	return v, nil
}

func (d *decoder) bytesField() ([]byte, error) {
	n, err := d.varint()
	if err != nil {
		return nil, err
	}
	if n > uint64(len(d.buf)-d.pos) {
		return nil, errTruncated
	}
	out := d.buf[d.pos : d.pos+int(n)]
	d.pos += int(n)
	return out, nil
}

func (d *decoder) stringField() (string, error) {
	b, err := d.bytesField()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// int32s reads either a packed length-delimited run of varints or a
// single unpacked varint, both of which proto3 allows for a
// `repeated int32`.
func (d *decoder) int32s(wire int) ([]int32, error) {
	switch wire {
	case wireVarint:
		v, err := d.varint()
		if err != nil {
			return nil, err
		}
		return []int32{int32(v)}, nil
	case wireBytes:
		b, err := d.bytesField()
		if err != nil {
			return nil, err
		}
		sub := &decoder{buf: b}
		var out []int32
		for !sub.done() {
			v, err := sub.varint()
			if err != nil {
				return nil, err
			}
			out = append(out, int32(v))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("scip: repeated int32 with wire type %d", wire)
	}
}

func (d *decoder) skip(wire int) error {
	switch wire {
	case wireVarint:
		_, err := d.varint()
		return err
	case wireFixed64:
		if len(d.buf)-d.pos < 8 {
			return errTruncated
		}
		d.pos += 8
		return nil
	case wireFixed32:
		if len(d.buf)-d.pos < 4 {
			return errTruncated
		}
		d.pos += 4
		return nil
	case wireBytes:
		_, err := d.bytesField()
		return err
	case wireStart, wireEnd:
		return errors.New("scip: unexpected group wire type")
	default:
		return fmt.Errorf("scip: unknown wire type %d", wire)
	}
}
