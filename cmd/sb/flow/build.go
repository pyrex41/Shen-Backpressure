package flow

import (
	"sort"

	"github.com/pyrex41/Shen-Backpressure/cmd/sb/internal/scip"
)

// FromIndex converts a decoded SCIP index into flow facts.
//
// Three passes, all per-document:
//
//  1. Definitions. Every occurrence carrying the Definition role
//     becomes a (def …) fact. Its extent is the occurrence's
//     enclosing_range when the indexer emits one (scip-go does, for
//     functions and methods) and the name's own range otherwise.
//
//  2. Enclosing attribution. Every non-definition occurrence is
//     attributed to the innermost definition in the same document
//     whose extent contains it. "Innermost" is the containing
//     definition with the fewest lines, which resolves a method
//     nested in a type correctly without needing a nesting relation
//     from the indexer.
//
//  3. Call edges. A reference to a callable symbol (SCIP descriptor
//     suffix `().`) from inside a definition is a call edge. This is
//     the step that makes the analysis language-independent: the
//     indexer has already resolved aliases, receivers and imports, so
//     an aliased import and a direct one produce the same edge.
//
// Local symbols (SCIP's "local N" scheme) are dropped: they are
// document-scoped and carry no cross-file flow information.
func FromIndex(idx *scip.Index, indexer string) *FactSet {
	fs := &FactSet{Indexer: indexer}
	for _, doc := range idx.Documents {
		if fs.Language == "" {
			fs.Language = doc.Language
		}
		defs := documentDefs(doc)
		fs.Defs = append(fs.Defs, defs...)
		for _, occ := range doc.Occurrences {
			if occ.IsDefinition() || isLocal(occ.Symbol) || occ.Symbol == "" {
				continue
			}
			span, ok := scip.NormalizeRange(occ.Range)
			if !ok {
				continue
			}
			encl := innermostContaining(defs, span.StartLine, span.StartCol)
			fs.Refs = append(fs.Refs, Ref{
				Symbol:    occ.Symbol,
				File:      doc.RelativePath,
				Line:      span.StartLine,
				Col:       span.StartCol,
				Enclosing: encl,
			})
			if encl != "" && IsCallable(occ.Symbol) && encl != occ.Symbol {
				fs.Calls = append(fs.Calls, Call{Caller: encl, Callee: occ.Symbol})
			}
		}
	}
	fs.Calls = dedupeCalls(fs.Calls)
	fs.Sort()
	fs.Index()
	return fs
}

func isLocal(symbol string) bool {
	return len(symbol) >= 6 && symbol[:6] == "local "
}

// documentDefs collects the definition facts of one document.
func documentDefs(doc scip.Document) []Def {
	var out []Def
	for _, occ := range doc.Occurrences {
		if !occ.IsDefinition() || isLocal(occ.Symbol) || occ.Symbol == "" {
			continue
		}
		span, ok := scip.NormalizeRange(occ.Range)
		if !ok {
			continue
		}
		if encl, ok := scip.NormalizeRange(occ.EnclosingRange); ok {
			span = encl
		}
		out = append(out, Def{
			Symbol:    occ.Symbol,
			File:      doc.RelativePath,
			StartLine: span.StartLine,
			StartCol:  span.StartCol,
			EndLine:   span.EndLine,
			EndCol:    span.EndCol,
		})
	}
	return out
}

// innermostContaining returns the symbol of the smallest definition
// whose extent contains the position, or "" when none does.
func innermostContaining(defs []Def, line, col int32) string {
	best := ""
	var bestSpan int32 = -1
	for _, d := range defs {
		if !d.Contains(line, col) {
			continue
		}
		if bestSpan < 0 || d.lineSpan() < bestSpan {
			best, bestSpan = d.Symbol, d.lineSpan()
		}
	}
	return best
}

func dedupeCalls(calls []Call) []Call {
	seen := map[Call]bool{}
	out := calls[:0]
	for _, c := range calls {
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Caller != out[j].Caller {
			return out[i].Caller < out[j].Caller
		}
		return out[i].Callee < out[j].Callee
	})
	return out
}
