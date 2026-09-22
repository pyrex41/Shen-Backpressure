package main

// canonical.go — W5.3. The byte string a discharge report's signature
// covers.
//
// A signature over "the file" would be useless: `sb audit-report
// --format=json` re-indents, a merge tool reorders keys, an editor
// strips a trailing newline, and the signature breaks without anything
// about the claims having changed. So the signature covers a canonical
// derivation of the report's *content*:
//
//  1. Parse the report as generic JSON. Whitespace and key order in the
//     file become irrelevant.
//  2. Delete the top-level `signature` member. This is what makes the
//     signature a fixed point: the signer computes the bytes of an
//     unsigned document, writes the signature into the document, and
//     the verifier — holding the signed document — recomputes exactly
//     the same bytes by removing it again.
//  3. Re-encode with object keys sorted by their UTF-8 code points,
//     no insignificant whitespace, and Go's json escaping (HTML
//     escaping disabled so `<`, `>` and `&` survive verbatim).
//
// Numbers are re-encoded from json.Number, i.e. exactly the digits the
// document contained, so a round trip through float64 cannot perturb
// them.
//
// The derivation is named `sb-canonical-json-v1` and that name is
// recorded inside every signature, so a future change to the rules is
// distinguishable from a bad signature.
//
// This is deliberately not RFC 8785 (JCS): JCS re-serialises numbers
// through the ECMAScript algorithm, which needs a float formatter we
// would have to hand-roll. Preserving the source digits is both
// simpler and stricter.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalizationName is the identifier recorded in a signature.
const CanonicalizationName = "sb-canonical-json-v1"

// SignatureField is the top-level member excluded from the canonical
// bytes.
const SignatureField = "signature"

// CanonicalReportBytes returns the canonical bytes for a discharge
// report given the raw file contents. The input need not be a report
// sb produced — any JSON object works — which matters because the
// verifier must be able to canonicalise a file it did not write.
func CanonicalReportBytes(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("parse report: %w", err)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("report is not a JSON object")
	}
	delete(obj, SignatureField)

	var b bytes.Buffer
	if err := writeCanonical(&b, obj); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// CanonicalReportBytesOf marshals r and canonicalises the result. Used
// by the signer, which holds a struct rather than a file.
func CanonicalReportBytesOf(r *DischargeReport) ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return CanonicalReportBytes(data)
}

// writeCanonical emits v in canonical form. Objects sort their keys;
// arrays keep their order (it is significant); scalars are encoded by
// encoding/json with HTML escaping off.
func writeCanonical(b *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonicalScalar(b, k); err != nil {
				return err
			}
			b.WriteByte(':')
			if err := writeCanonical(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	case []any:
		b.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, elem); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	case json.Number:
		// Exactly the digits the document carried.
		b.WriteString(t.String())
		return nil
	default:
		return writeCanonicalScalar(b, v)
	}
}

// writeCanonicalScalar encodes a string, bool, or null with HTML
// escaping disabled, and without the newline Encoder.Encode appends.
func writeCanonicalScalar(b *bytes.Buffer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	b.Write(bytes.TrimRight(buf.Bytes(), "\n"))
	return nil
}
