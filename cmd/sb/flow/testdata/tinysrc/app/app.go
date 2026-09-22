// Package app is the fixture's application layer. It contains, on
// purpose, one sanctioned caller of the raw constructor, one
// unsanctioned caller reached through an aliased import, one handler
// that obtains the proof before the sink, and one that does not.
package app

import (
	g "tiny/guard"
)

// Store is the sink the flow premises protect.
type Store struct{}

// Query is the sink.
func (s Store) Query(q string) string { return q }

// Check is the sanctioned wrapper: the only caller of the raw
// constructor that a (constructor-only …) premise allows.
func Check(member bool) g.Access {
	return g.NewAccess(member)
}

// ForgeAccess is the unsanctioned caller. Note the aliased import: a
// grep for the unaliased package qualifier finds nothing here, while
// the resolved symbol graph shows the same constructor symbol.
func ForgeAccess() g.Access {
	return g.NewAccess(true)
}

// HandleListGood obtains the proof before reaching the sink, so the
// must-pass-through premise holds for it.
func HandleListGood(s Store, member bool) string {
	access := Check(member)
	if !access.OK() {
		return ""
	}
	return readRows(s)
}

// HandleListBad reaches the sink without obtaining the proof. It is
// the counterexample the must-pass-through premise reports, and the
// violating path runs through readRows.
func HandleListBad(s Store) string {
	return readRows(s)
}

// readRows is the intermediate hop, so the shortest violating path
// has more than one element.
func readRows(s Store) string {
	return s.Query("SELECT id FROM rows")
}
