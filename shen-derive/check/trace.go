package check

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// TraceOptions configures trace validation.
type TraceOptions struct {
	// FromAny accepts a trace that starts in any state satisfying the
	// invariants, not only an initial one: for traces sampled from a
	// long-running system rather than recorded from start-up.
	FromAny bool
}

// TraceFailure explains why a recorded trace is not a behaviour of the
// spec.
type TraceFailure struct {
	// Index is the position in the trace of the offending state.
	Index int
	// Reason says what was wrong with it.
	Reason string
	// Allowed lists what the spec permitted at that point: the initial
	// states for Index 0, otherwise the successors of the previous state.
	// Empty when the failure was an invariant.
	Allowed []Step
}

func (f *TraceFailure) Error() string {
	return fmt.Sprintf("trace step %d: %s", f.Index, f.Reason)
}

// ValidateTrace checks that tr is a behaviour of m: it starts in an
// initial state, every step is either a stutter (the state is unchanged)
// or a successor the spec allows, and every state and step satisfies the
// invariants. A step that carries an action label must be explained by a
// successor with that label. It returns nil when the trace is valid.
func (m *Machine) ValidateTrace(tr []Step, opts TraceOptions) (*TraceFailure, error) {
	if len(tr) == 0 {
		return nil, nil
	}
	if !opts.FromAny {
		inits, err := m.Initial()
		if err != nil {
			return nil, err
		}
		if !containsState(inits, tr[0].State) {
			allowed := make([]Step, len(inits))
			for i, s := range inits {
				allowed[i] = Step{State: s}
			}
			return &TraceFailure{Index: 0, Reason: fmt.Sprintf("%s is not an initial state", tr[0].State), Allowed: allowed}, nil
		}
	}
	for i, st := range tr {
		name, err := m.brokenInvariant(st.State)
		if err != nil {
			return nil, err
		}
		if name != "" {
			return &TraceFailure{Index: i, Reason: fmt.Sprintf("invariant %s violated by %s", name, st.State)}, nil
		}
		if i == 0 || key(st.State) == key(tr[i-1].State) {
			continue
		}
		prev := tr[i-1].State
		succs, err := m.Successors(prev)
		if err != nil {
			return nil, err
		}
		if !explains(succs, st) {
			reason := fmt.Sprintf("no action takes %s to %s", prev, st.State)
			if st.Action != "" {
				reason = fmt.Sprintf("action %q does not take %s to %s", st.Action, prev, st.State)
			}
			return &TraceFailure{Index: i, Reason: reason, Allowed: succs}, nil
		}
		name, err = m.brokenStepInvariant(prev, st.State)
		if err != nil {
			return nil, err
		}
		if name != "" {
			return &TraceFailure{Index: i, Reason: fmt.Sprintf("step invariant %s violated from %s to %s", name, prev, st.State)}, nil
		}
	}
	return nil, nil
}

func containsState(states []core.Value, s core.Value) bool {
	k := key(s)
	for _, t := range states {
		if key(t) == k {
			return true
		}
	}
	return false
}

func explains(succs []Step, st Step) bool {
	k := key(st.State)
	for _, s := range succs {
		if key(s.State) == k && (st.Action == "" || st.Action == s.Action) {
			return true
		}
	}
	return false
}

// StepFromJSON decodes one line of a JSONL trace. A line is either a bare
// state or an object {"action": "...", "state": ...}. JSON arrays become
// lists, whole numbers become integers.
func StepFromJSON(line []byte) (Step, error) {
	var raw any
	if err := json.Unmarshal(line, &raw); err != nil {
		return Step{}, err
	}
	if obj, ok := raw.(map[string]any); ok {
		sv, ok := obj["state"]
		if !ok {
			return Step{}, fmt.Errorf("trace object has no \"state\" field")
		}
		st, err := ValueFromJSON(sv)
		if err != nil {
			return Step{}, err
		}
		action, _ := obj["action"].(string)
		return Step{Action: action, State: st}, nil
	}
	st, err := ValueFromJSON(raw)
	return Step{State: st}, err
}

// ValueFromJSON converts a decoded JSON value to an evaluator value.
func ValueFromJSON(v any) (core.Value, error) {
	switch x := v.(type) {
	case bool:
		return core.BoolVal(x), nil
	case string:
		return core.StringVal(x), nil
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return core.IntVal(int64(x)), nil
		}
		return core.FloatVal(x), nil
	case []any:
		out := make(core.ListVal, len(x))
		for i, e := range x {
			ev, err := ValueFromJSON(e)
			if err != nil {
				return nil, err
			}
			out[i] = ev
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported JSON value in trace: %v", v)
}
