package scip

import "encoding/binary"

// Encode serialises idx back to the SCIP wire format. It writes only
// the fields Parse reads, which makes it exact for round-tripping
// hand-constructed fixtures and lossy for a real index.
//
// It exists so tests can build an index.scip fixture without the
// protobuf toolchain, and so a fixture can be regenerated from a
// decoded real index after trimming it down to a few documents.
func Encode(idx *Index) []byte {
	var out []byte
	for _, doc := range idx.Documents {
		out = appendTagged(out, 2, encodeDocument(doc))
	}
	return out
}

func encodeDocument(doc Document) []byte {
	var out []byte
	if doc.RelativePath != "" {
		out = appendTagged(out, 1, []byte(doc.RelativePath))
	}
	for _, occ := range doc.Occurrences {
		out = appendTagged(out, 2, encodeOccurrence(occ))
	}
	if doc.Language != "" {
		out = appendTagged(out, 4, []byte(doc.Language))
	}
	return out
}

func encodeOccurrence(o Occurrence) []byte {
	var out []byte
	if len(o.Range) > 0 {
		out = appendTagged(out, 1, packInt32s(o.Range))
	}
	if o.Symbol != "" {
		out = appendTagged(out, 2, []byte(o.Symbol))
	}
	if o.SymbolRoles != 0 {
		out = appendVarintKey(out, 3, wireVarint)
		out = binary.AppendUvarint(out, uint64(o.SymbolRoles))
	}
	if len(o.EnclosingRange) > 0 {
		out = appendTagged(out, 7, packInt32s(o.EnclosingRange))
	}
	return out
}

func packInt32s(vals []int32) []byte {
	var out []byte
	for _, v := range vals {
		out = binary.AppendUvarint(out, uint64(v))
	}
	return out
}

func appendVarintKey(dst []byte, field, wire int) []byte {
	return binary.AppendUvarint(dst, uint64(field)<<3|uint64(wire))
}

func appendTagged(dst []byte, field int, payload []byte) []byte {
	dst = appendVarintKey(dst, field, wireBytes)
	dst = binary.AppendUvarint(dst, uint64(len(payload)))
	return append(dst, payload...)
}
