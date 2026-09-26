package check

import "fmt"

// ExploreOptions bounds and configures an exploration.
type ExploreOptions struct {
	// MaxStates stops exploration once this many distinct states have
	// been found. Zero means no bound. A bounded run that stops early
	// reports Complete=false: it is evidence, not a check.
	MaxStates int
	// Deadlock reports a reachable state with no successors as a
	// violation, as TLC does by default. A spec whose terminal states are
	// intended should either turn this off or let those states stutter
	// (list themselves as a successor).
	Deadlock bool
}

// Violation is a failed check with the shortest trace that reaches it.
type Violation struct {
	// Kind is "invariant", "step-invariant", "deadlock" or "error".
	Kind string
	// Name is the invariant that failed; empty for deadlock.
	Name string
	// Err is the evaluation error when Kind is "error".
	Err error
	// Trace runs from an initial state to the failing state.
	Trace []Step
}

func (v *Violation) Error() string {
	switch v.Kind {
	case "deadlock":
		return "deadlock reached"
	case "error":
		return fmt.Sprintf("evaluation error: %v", v.Err)
	default:
		return fmt.Sprintf("%s %s violated", v.Kind, v.Name)
	}
}

// ExploreResult summarises an exploration.
type ExploreResult struct {
	States      int // distinct states found
	Transitions int // successor edges evaluated
	Depth       int // length of the longest shortest path from an initial state
	Complete    bool
	Violation   *Violation
}

// Explore enumerates the reachable states of m breadth-first. Because
// the search is breadth-first, a returned counterexample is a shortest
// one.
func (m *Machine) Explore(opts ExploreOptions) (*ExploreResult, error) {
	type node struct {
		step   Step
		parent int // index into nodes; -1 for initial states
		depth  int
	}
	var nodes []node
	seen := map[string]bool{}
	res := &ExploreResult{}

	trace := func(i int) []Step {
		var rev []Step
		for ; i >= 0; i = nodes[i].parent {
			rev = append(rev, nodes[i].step)
		}
		out := make([]Step, len(rev))
		for j := range rev {
			out[j] = rev[len(rev)-1-j]
		}
		return out
	}
	fail := func(kind, name string, err error, tr []Step) (*ExploreResult, error) {
		res.States = len(nodes)
		res.Violation = &Violation{Kind: kind, Name: name, Err: err, Trace: tr}
		return res, nil
	}
	// add records a newly found state and checks its state invariants.
	// It returns false when exploration should stop.
	add := func(st Step, parent, depth int) (bool, *ExploreResult, error) {
		k := key(st.State)
		if seen[k] {
			return true, nil, nil
		}
		seen[k] = true
		nodes = append(nodes, node{step: st, parent: parent, depth: depth})
		if depth > res.Depth {
			res.Depth = depth
		}
		name, err := m.brokenInvariant(st.State)
		if err != nil {
			r, e := fail("error", name, err, trace(len(nodes)-1))
			return false, r, e
		}
		if name != "" {
			r, e := fail("invariant", name, nil, trace(len(nodes)-1))
			return false, r, e
		}
		return true, nil, nil
	}

	inits, err := m.Initial()
	if err != nil {
		return nil, err
	}
	for _, s := range inits {
		if ok, r, e := add(Step{State: s}, -1, 0); !ok {
			return r, e
		}
	}

	for i := 0; i < len(nodes); i++ {
		if opts.MaxStates > 0 && len(nodes) >= opts.MaxStates {
			res.States = len(nodes)
			return res, nil
		}
		cur := nodes[i]
		succs, err := m.Successors(cur.step.State)
		if err != nil {
			return fail("error", "", err, trace(i))
		}
		if len(succs) == 0 && opts.Deadlock {
			return fail("deadlock", "", nil, trace(i))
		}
		for _, st := range succs {
			res.Transitions++
			name, err := m.brokenStepInvariant(cur.step.State, st.State)
			if err != nil || name != "" {
				kind := "step-invariant"
				if err != nil {
					kind = "error"
				}
				return fail(kind, name, err, append(trace(i), st))
			}
			if ok, r, e := add(st, i, cur.depth+1); !ok {
				return r, e
			}
		}
	}
	res.States = len(nodes)
	res.Complete = true
	return res, nil
}

// FormatTrace renders a trace one state per line, TLC style. start is
// the index of tr[0] in the full trace, for printing an excerpt.
func FormatTrace(tr []Step, start int) string {
	s := ""
	for i, st := range tr {
		action := st.Action
		if start+i == 0 {
			action = "<init>"
		} else if action == "" {
			action = "<next>"
		}
		s += fmt.Sprintf("  %3d  %-14s %s\n", start+i, action, st.State)
	}
	return s
}
