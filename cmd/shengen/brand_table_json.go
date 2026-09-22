package main

// brand_table_json.go — W5.4. `shengen --brand-table <file>` writes the
// inferred brand table as JSON.
//
// The human-readable table already goes to stderr (BrandTable.String),
// which is fine for a developer reading a codegen log and useless to a
// downstream tool. The discharge report needs to know, per Shen type,
// whether W1's brand inference pairs its premises — that is what
// justifies the `guard-brand-bound` discharge basis. Rather than
// re-implement the inference in shen-derive or sb, shengen emits the
// table it already computed and the report reads it.
//
// The JSON is sorted by Shen type name and contains no paths or
// timestamps, so it is a pure function of the spec bytes plus the
// shengen version — same reproducibility contract as the guards file.

import (
	"encoding/json"
	"os"
	"sort"
)

// BrandTableJSON is the wire format of --brand-table.
type BrandTableJSON struct {
	// SchemaVersion is this file's own version, independent of the
	// discharge report's.
	SchemaVersion int `json:"schema_version"`
	// Spec is the canonical (cwd-independent) spec path.
	Spec string `json:"spec"`
	// ShengenVersion is the emitter that inferred the table.
	ShengenVersion string `json:"shengen_version"`
	// Types is every entry, sorted by shen_name.
	Types []BrandTypeJSON `json:"types"`
}

// BrandTypeJSON is one type's inferred brand signature.
type BrandTypeJSON struct {
	ShenName string `json:"shen_name"`
	GoName   string `json:"go_name"`
	// Datatypes are the (datatype …) block names that produce this
	// type. Usually one, with the same name — but a block may conclude
	// in a differently-named type (payment's `balance-invariant`
	// concludes `balance-checked`), and the discharge report keys its
	// rules by the *block* name. Recording both lets a report look the
	// entry up by either.
	Datatypes []string `json:"datatypes,omitempty"`
	// Kind is "unbranded", "minted", "inherited", or "sum" — the same
	// vocabulary BrandTable.String prints.
	Kind string `json:"kind"`
	// Params are the phantom brand type parameters in declaration
	// order. Empty for an unbranded type.
	Params []string `json:"params,omitempty"`
	// Signature is the Go type signature with its brand parameters,
	// e.g. "SafeTransfer[B]". This is what a report's code reference
	// points a reader at.
	Signature string `json:"signature"`
	// PremiseArgs[i] are the brand arguments premise i carries. Two
	// premises sharing an argument is exactly the pairing constraint
	// W1 enforces: `NewSafeTransfer[B](tx Transaction[B], check
	// BalanceChecked[B])` cannot be called with a proof about another
	// transaction.
	PremiseArgs [][]string `json:"premise_args,omitempty"`
	// BoundPremises are the indices of the premises whose brand
	// arguments are shared — with the conclusion's own brand, or with
	// another premise's. Those are exactly the premises whose pairing
	// the Go compiler now enforces, and therefore the ones the
	// discharge report may record with the `guard-brand-bound` basis.
	//
	// `NewSafeTransfer[B](tx Transaction[B], check BalanceChecked[B])`
	// binds both premises: the brand cannot be satisfied by a proof
	// about a different transaction. `NewBalanceChecked[B](bal float64,
	// tx Transaction[B])` binds only premise 1 — the float is a value,
	// not evidence.
	BoundPremises []int `json:"bound_premises,omitempty"`
	// Bound reports whether any premise is brand-bound.
	Bound bool `json:"bound"`
}

// BrandTableToJSON projects a BrandTable into its wire form.
func BrandTableToJSON(bt *BrandTable, types []Datatype, st *SymbolTable, specPath, shengenVersion string) *BrandTableJSON {
	blocks := datatypeBlocksByType(types, st)
	out := &BrandTableJSON{
		SchemaVersion:  1,
		Spec:           specPath,
		ShengenVersion: shengenVersion,
		Types:          []BrandTypeJSON{},
	}
	if bt == nil {
		return out
	}
	names := make([]string, 0, len(bt.Types))
	for n := range bt.Types {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		info := bt.Types[n]
		if info == nil {
			continue
		}
		entry := BrandTypeJSON{
			ShenName:    n,
			GoName:      info.GoName,
			Datatypes:   blocks[n],
			Kind:        brandKindLabel(info),
			Params:      info.Params,
			Signature:   brandSignature(info),
			PremiseArgs: normalisePremiseArgs(info.PremiseArgs),
		}
		entry.BoundPremises = brandBoundPremises(info)
		entry.Bound = len(entry.BoundPremises) > 0
		out.Types = append(out.Types, entry)
	}
	return out
}

// datatypeBlocksByType maps each generated type name to the
// (datatype …) blocks that conclude in it, in spec order.
func datatypeBlocksByType(types []Datatype, st *SymbolTable) map[string][]string {
	out := map[string][]string{}
	if st == nil {
		return out
	}
	for _, dt := range types {
		for _, gt := range classify(dt, st) {
			name := generatedShenName(dt, gt, st)
			if name == "" {
				continue
			}
			seen := false
			for _, existing := range out[name] {
				if existing == dt.Name {
					seen = true
					break
				}
			}
			if !seen {
				out[name] = append(out[name], dt.Name)
			}
		}
	}
	return out
}

// normalisePremiseArgs replaces nil inner slices with empty ones so an
// unbranded premise renders as [] rather than null. Keeps the JSON
// readable without changing its meaning.
func normalisePremiseArgs(in [][]string) [][]string {
	if len(in) == 0 {
		return nil
	}
	out := make([][]string, len(in))
	for i, args := range in {
		if args == nil {
			out[i] = []string{}
			continue
		}
		out[i] = args
	}
	return out
}

// brandKindLabel mirrors the vocabulary of BrandTable.String.
func brandKindLabel(info *BrandInfo) string {
	switch {
	case len(info.Params) == 0:
		return "unbranded"
	case info.IsSum:
		return "sum"
	case info.Minted:
		return "minted"
	default:
		return "inherited"
	}
}

// brandSignature renders the Go type with its brand parameters.
func brandSignature(info *BrandInfo) string {
	sig := info.GoName
	if len(info.Params) > 0 {
		sig += "["
		for i, p := range info.Params {
			if i > 0 {
				sig += ", "
			}
			sig += p
		}
		sig += "]"
	}
	return sig
}

// brandBoundPremises returns the indices of premises whose brand
// arguments are shared, and so are pinned by the type checker.
//
// A premise's brand argument is "shared" when it also appears in the
// conclusion's own parameter list (the premise must be evidence about
// the same subject as the conclusion) or in another premise's
// arguments (the two premises must be about each other). Either way,
// the compiler rejects a call that pairs evidence from two different
// chains, which is the property the `guard-brand-bound` discharge
// basis reports.
//
// A minted type binds nothing: it introduces a fresh brand that none
// of its premises carries, so there is no pairing to enforce.
func brandBoundPremises(info *BrandInfo) []int {
	if info == nil || len(info.PremiseArgs) == 0 {
		return nil
	}
	conclusion := map[string]bool{}
	if !info.Minted {
		for _, p := range info.Params {
			conclusion[p] = true
		}
	}
	// How many premises mention each brand.
	occurrences := map[string]int{}
	for _, args := range info.PremiseArgs {
		seen := map[string]bool{}
		for _, a := range args {
			if seen[a] {
				continue
			}
			seen[a] = true
			occurrences[a]++
		}
	}
	var out []int
	for i, args := range info.PremiseArgs {
		for _, a := range args {
			if conclusion[a] || occurrences[a] >= 2 {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

// WriteBrandTableJSON writes the table to path as indented JSON with a
// trailing newline.
func WriteBrandTableJSON(path string, t *BrandTableJSON) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
