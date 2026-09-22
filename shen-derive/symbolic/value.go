package symbolic

import (
	"fmt"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// SymVal is the symbolic counterpart of core.Value. Scalars carry a
// constraint Term; lists and tuples carry a concrete spine with
// symbolic leaves (see the bounded-unrolling note on the package doc).
type SymVal interface {
	symValNode()
	String() string
}

// SNum is a symbolic number. Every Shen `number` is modelled as an SMT
// Real: the spec subset mixes ints and floats freely (core.Eval promotes
// int op float to float), and LRA is decidable, so Real is both the
// faithful and the cheap choice. The decoder narrows a whole-valued
// model back to core.IntVal so generated Go literals stay readable.
type SNum struct {
	T        Term
	ShenType string // "number", or a wrapper/constrained type name
}

// SBoolV is a symbolic boolean.
type SBoolV struct {
	T        Term
	ShenType string
}

// SStrV is a symbolic string (or symbol).
type SStrV struct {
	T        Term
	ShenType string
}

// SListV is a list with a concrete length and symbolic elements. It
// doubles as the representation of a composite datatype, exactly as
// core.ListVal does in the concrete evaluator: a `transaction` is the
// three-element list [Amount From To].
type SListV struct {
	Elems    []SymVal
	ShenType string // "(list transaction)" or "transaction"
	ElemType string // element type for genuine list types; "" for composites
}

// STupleV is a symbolic (@p a b) pair.
type STupleV struct {
	Fst, Snd SymVal
}

// SClosureV is a symbolic lambda closure.
type SClosureV struct {
	Env   *SEnv
	Param string
	Body  core.Sexpr
}

// SPrimV is a partially applied built-in primitive.
type SPrimV struct {
	Op   string
	Args []SymVal
}

// SBuiltinV is a host (Go) function exposed to the symbolic evaluator —
// field accessors, `val`, and the curried references to sibling defines.
type SBuiltinV struct {
	Name string
	Fn   func(SymVal) (SymVal, error)
}

func (*SNum) symValNode()      {}
func (*SBoolV) symValNode()    {}
func (*SStrV) symValNode()     {}
func (*SListV) symValNode()    {}
func (*STupleV) symValNode()   {}
func (*SClosureV) symValNode() {}
func (*SPrimV) symValNode()    {}
func (*SBuiltinV) symValNode() {}

func (v *SNum) String() string   { return v.T.String() }
func (v *SBoolV) String() string { return v.T.String() }
func (v *SStrV) String() string  { return v.T.String() }
func (v *SListV) String() string {
	parts := make([]string, len(v.Elems))
	for i, e := range v.Elems {
		parts[i] = e.String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
func (v *STupleV) String() string   { return "(" + v.Fst.String() + ", " + v.Snd.String() + ")" }
func (v *SClosureV) String() string { return "<closure>" }
func (v *SPrimV) String() string    { return fmt.Sprintf("<prim:%s/%d>", v.Op, len(v.Args)) }
func (v *SBuiltinV) String() string { return fmt.Sprintf("<builtin:%s>", v.Name) }

// SEnv is a linked-list environment mapping names to symbolic values.
type SEnv struct {
	name   string
	val    SymVal
	parent *SEnv
}

// EmptySEnv returns the empty environment.
func EmptySEnv() *SEnv { return nil }

// Extend returns a new environment with name bound to val.
func (e *SEnv) Extend(name string, val SymVal) *SEnv {
	return &SEnv{name: name, val: val, parent: e}
}

// Lookup resolves a name in the environment.
func (e *SEnv) Lookup(name string) (SymVal, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.name == name {
			return cur.val, true
		}
	}
	return nil, false
}

// --- skeleton construction ---

// skeletonBuilder mints fresh variable names while walking a Shen type.
type skeletonBuilder struct {
	tt      *specfile.TypeTable
	n       int
	listLen func(path string) int // supplies a concrete length per list site
	// constraints collects the verified predicates of every constrained
	// wrapper encountered, instantiated at the freshly minted variable.
	// Without these, a model could hand back a negative `amount` and the
	// generated test's mustAmount() helper would panic.
	constraints []Term
}

func (b *skeletonBuilder) fresh(prefix string, s Sort) *Var {
	b.n++
	return NewVar(fmt.Sprintf("%s_%d", sanitizeVarName(prefix), b.n), s)
}

// sanitizeVarName maps a Shen identifier to a legal SMT-LIB2 simple
// symbol (letters, digits, and a small punctuation set).
func sanitizeVarName(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			out.WriteByte(c)
		default:
			out.WriteByte('_')
		}
	}
	r := out.String()
	if r == "" {
		return "v"
	}
	if r[0] >= '0' && r[0] <= '9' {
		return "v" + r
	}
	return r
}

// buildSkeleton produces a symbolic value of the given Shen type, with
// a fresh variable at every scalar leaf. `path` is a human-readable
// prefix used for variable names (e.g. "txs_0_amount").
func (b *skeletonBuilder) buildSkeleton(shenType, path string) (SymVal, error) {
	shenType = strings.TrimSpace(shenType)

	if elem := specfile.ElemType(shenType); elem != "" {
		n := b.listLen(path)
		elems := make([]SymVal, n)
		for i := 0; i < n; i++ {
			e, err := b.buildSkeleton(elem, fmt.Sprintf("%s_%d", path, i))
			if err != nil {
				return nil, err
			}
			elems[i] = e
		}
		return &SListV{Elems: elems, ShenType: shenType, ElemType: elem}, nil
	}

	switch shenType {
	case "number":
		return &SNum{T: b.fresh(path, SortReal), ShenType: shenType}, nil
	case "string", "symbol":
		return &SStrV{T: b.fresh(path, SortString), ShenType: shenType}, nil
	case "boolean":
		return &SBoolV{T: b.fresh(path, SortBool), ShenType: shenType}, nil
	}

	entry, ok := b.tt.Entries[shenType]
	if !ok {
		return nil, fmt.Errorf("unknown Shen type %q", shenType)
	}
	switch entry.Category {
	case specfile.CatWrapper, specfile.CatConstrained:
		inner, err := b.buildSkeleton(entry.ShenPrim, path)
		if err != nil {
			return nil, err
		}
		// Re-tag the leaf with the wrapper type so the Go-literal
		// emitter knows to wrap it in mustXxx(...).
		switch v := inner.(type) {
		case *SNum:
			v.ShenType = shenType
		case *SStrV:
			v.ShenType = shenType
		case *SBoolV:
			v.ShenType = shenType
		}
		if entry.Category == specfile.CatConstrained {
			cs, err := b.entryConstraints(entry, inner)
			if err != nil {
				return nil, err
			}
			b.constraints = append(b.constraints, cs...)
		}
		return inner, nil

	case specfile.CatComposite, specfile.CatGuarded:
		if len(entry.Fields) == 0 {
			return nil, fmt.Errorf("composite %q has no fields", shenType)
		}
		elems := make([]SymVal, len(entry.Fields))
		for i, f := range entry.Fields {
			e, err := b.buildSkeleton(f.ShenType, fmt.Sprintf("%s_%s", path, strings.ToLower(f.ShenName)))
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", f.ShenName, err)
			}
			elems[i] = e
		}
		sv := &SListV{Elems: elems, ShenType: shenType}
		if entry.Category == specfile.CatGuarded {
			cs, err := b.guardedConstraints(entry, sv)
			if err != nil {
				return nil, err
			}
			b.constraints = append(b.constraints, cs...)
		}
		return sv, nil

	default:
		return nil, fmt.Errorf("type %q (category %s) is not supported by the symbolic evaluator",
			shenType, entry.Category)
	}
}

// entryConstraints instantiates a constrained wrapper's verified
// predicates at the freshly built symbolic value.
func (b *skeletonBuilder) entryConstraints(entry *specfile.TypeEntry, at SymVal) ([]Term, error) {
	if entry.VarName == "" {
		return nil, nil
	}
	env := baseSymEnv(b.tt, nil).Extend(entry.VarName, at)
	return evalPredicates(entry.Verified, env)
}

// guardedConstraints instantiates a guarded composite's verified
// predicates with each premise variable bound to the matching field.
func (b *skeletonBuilder) guardedConstraints(entry *specfile.TypeEntry, at *SListV) ([]Term, error) {
	env := baseSymEnv(b.tt, nil)
	for i, f := range entry.Fields {
		if i < len(at.Elems) {
			env = env.Extend(f.ShenName, at.Elems[i])
		}
	}
	return evalPredicates(entry.Verified, env)
}

// evalPredicates symbolically evaluates each raw predicate to a single
// boolean term. A predicate that branches contributes the disjunction
// of its true-valued paths, which is the right reading for "this
// predicate holds".
func evalPredicates(raw []string, env *SEnv) ([]Term, error) {
	var out []Term
	for _, r := range raw {
		sexpr, err := core.ParseSexpr(r)
		if err != nil {
			return nil, fmt.Errorf("parse predicate %q: %w", r, err)
		}
		t, err := evalPredicateTerm(sexpr, env)
		if err != nil {
			return nil, fmt.Errorf("predicate %q: %w", r, err)
		}
		out = append(out, t)
	}
	return out, nil
}

// evalPredicateTerm evaluates one predicate to a boolean term. It uses
// the single-path evaluator: predicates in the spec subset are pure
// boolean expressions over their premise variables, so a fork here
// means the predicate is outside the supported fragment.
func evalPredicateTerm(sexpr core.Sexpr, env *SEnv) (Term, error) {
	ex := &symExec{maxCallDepth: defaultMaxCallDepth, base: env}
	v, err := ex.eval(env, sexpr)
	if err != nil {
		return nil, err
	}
	bv, ok := v.(*SBoolV)
	if !ok {
		return nil, fmt.Errorf("expected a boolean, got %s", v.String())
	}
	// Any conditions the evaluator collected on the way (from a folded
	// `if`, say) are part of what must hold for the predicate to mean
	// what it says.
	return And(append(append([]Term{}, ex.pc...), bv.T)...), nil
}
