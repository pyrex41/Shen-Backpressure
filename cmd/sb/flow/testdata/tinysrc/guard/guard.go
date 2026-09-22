// Package guard stands in for a shengen-generated guard package: it
// exports a raw constructor that asserts a proof, which only the
// checked wrapper is allowed to call.
package guard

// Access is the proof value.
type Access struct{ ok bool }

// OK reports the asserted membership.
func (a Access) OK() bool { return a.ok }

// NewAccess is the raw constructor. Only Check may call it.
func NewAccess(ok bool) Access { return Access{ok: ok} }
