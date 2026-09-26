// Package check treats a Shen spec as a state machine in the style of a
// TLA+ spec and checks it two ways:
//
//   - Explore enumerates every reachable state of a finite instance
//     breadth-first (what TLC does), checking state invariants, step
//     invariants and deadlock, and returns a shortest counterexample
//     trace when one fails.
//   - ValidateTrace replays a trace recorded from a real implementation
//     and checks that every observed step is one the spec allows.
//
// A spec supplies three kinds of define:
//
//	(define init -> [S0 S1 ...])          initial states
//	(define next S -> [S' ...])           successor states of S
//	(define inv  S -> Bool)               state invariant ([]Inv)
//	(define step-inv S S' -> Bool)        step invariant ([][A]_vars)
//
// next may label a successor with the action that produced it by
// returning (@p "action" S') instead of S'. Labels show up in traces and
// can be recorded by an implementation so trace validation checks that
// the named action, not just some action, explains each step.
//
// States are ordinary evaluator values (lists, strings, numbers,
// booleans, tuples). Two states are the same state when they print the
// same, which is how the explorer deduplicates them.
package check

import (
	"fmt"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/verify"
)

// Machine is a spec bound to the defines that make it a state machine.
type Machine struct {
	spec           *specfile.SpecFile
	env            *core.Env
	init           *specfile.Define
	next           *specfile.Define
	invariants     []*specfile.Define
	stepInvariants []*specfile.Define
}

// Names says which defines in a spec play which role.
type Names struct {
	Init           string
	Next           string
	Invariants     []string
	StepInvariants []string
}

// Load binds a parsed spec to the named defines.
func Load(sf *specfile.SpecFile, n Names) (*Machine, error) {
	defs := make([]*specfile.Define, len(sf.Defines))
	for i := range sf.Defines {
		defs[i] = &sf.Defines[i]
	}
	tt := specfile.BuildTypeTable(sf.Datatypes, "", "")
	m := &Machine{spec: sf, env: verify.SpecEnv(tt, defs)}

	var err error
	if m.init, err = m.find(n.Init, 0); err != nil {
		return nil, err
	}
	if m.next, err = m.find(n.Next, 1); err != nil {
		return nil, err
	}
	for _, name := range n.Invariants {
		d, err := m.find(name, 1)
		if err != nil {
			return nil, err
		}
		m.invariants = append(m.invariants, d)
	}
	for _, name := range n.StepInvariants {
		d, err := m.find(name, 2)
		if err != nil {
			return nil, err
		}
		m.stepInvariants = append(m.stepInvariants, d)
	}
	return m, nil
}

// Override replaces the body of the nullary define name with expr, the
// way a TLC model config assigns CONSTANTS: the same spec can be checked
// at different sizes or with a feature switched on. It must be called
// before Load.
func Override(sf *specfile.SpecFile, name, expr string) error {
	d := sf.FindDefine(name)
	if d == nil {
		return fmt.Errorf("spec has no (define %s ...)", name)
	}
	if d.Arity() != 0 {
		return fmt.Errorf("define %s takes arguments; only constants (nullary defines) can be overridden", name)
	}
	body, err := core.ParseSexpr(expr)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	d.Clauses = []specfile.Clause{{Body: body}}
	return nil
}

// find returns the named define, checking its arity.
func (m *Machine) find(name string, arity int) (*specfile.Define, error) {
	d := m.spec.FindDefine(name)
	if d == nil {
		return nil, fmt.Errorf("spec has no (define %s ...)", name)
	}
	if d.Arity() != arity {
		return nil, fmt.Errorf("define %s takes %d arguments, expected %d", name, d.Arity(), arity)
	}
	return d, nil
}

// Step is one state in a trace and the action that produced it. The
// first step of a trace has an empty Action.
type Step struct {
	Action string
	State  core.Value
}

// Initial evaluates init.
func (m *Machine) Initial() ([]core.Value, error) {
	v, err := verify.EvalDefine(m.init, nil, m.env)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.init.Name, err)
	}
	lv, ok := v.(core.ListVal)
	if !ok {
		return nil, fmt.Errorf("%s must return a list of states, got %s", m.init.Name, v)
	}
	return lv, nil
}

// Successors evaluates next on s.
func (m *Machine) Successors(s core.Value) ([]Step, error) {
	v, err := verify.EvalDefine(m.next, []core.Value{s}, m.env)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", m.next.Name, s, err)
	}
	lv, ok := v.(core.ListVal)
	if !ok {
		return nil, fmt.Errorf("%s must return a list of states, got %s", m.next.Name, v)
	}
	out := make([]Step, len(lv))
	for i, e := range lv {
		out[i] = Step{State: e}
		if t, ok := e.(*core.TupleVal); ok {
			if label, ok := t.Fst.(core.StringVal); ok {
				out[i] = Step{Action: string(label), State: t.Snd}
			}
		}
	}
	return out, nil
}

// brokenInvariant returns the name of the first invariant s violates.
func (m *Machine) brokenInvariant(s core.Value) (string, error) {
	for _, d := range m.invariants {
		ok, err := m.holds(d, s)
		if err != nil || !ok {
			return d.Name, err
		}
	}
	return "", nil
}

// brokenStepInvariant returns the name of the first step invariant the
// step from s to t violates.
func (m *Machine) brokenStepInvariant(s, t core.Value) (string, error) {
	for _, d := range m.stepInvariants {
		ok, err := m.holds(d, s, t)
		if err != nil || !ok {
			return d.Name, err
		}
	}
	return "", nil
}

func (m *Machine) holds(d *specfile.Define, args ...core.Value) (bool, error) {
	v, err := verify.EvalDefine(d, args, m.env)
	if err != nil {
		return false, fmt.Errorf("%s: %w", d.Name, err)
	}
	b, ok := v.(core.BoolVal)
	if !ok {
		return false, fmt.Errorf("%s must return a boolean, got %s", d.Name, v)
	}
	return bool(b), nil
}

// key identifies a state for deduplication.
func key(s core.Value) string { return s.String() }
