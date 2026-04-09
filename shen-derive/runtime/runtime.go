// Package runtime provides helper types used by Go code generated
// by shen-derive's lowering pass.
package runtime

// Pair is a generic 2-tuple used to lower TupleLit nodes.
type Pair[A, B any] struct {
	Fst A
	Snd B
}
