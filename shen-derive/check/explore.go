package check

import (
	"fmt"
	"strings"
)

// ExploreOptions bounds and configures an exploration.
type ExploreOptions struct {
	// MaxStates stops exploration once this many distinct states have
	// been found. Zero means no bound. A bounded run that stops early
	// reports Complete=false: it is evidence, not a check. Temporal
	// properties are only checked on a complete graph.
	MaxStates int
	// Deadlock reports a reachable state with no successors as a
	// violation, as TLC does by default. A spec whose terminal states are
	// intended should either turn this off or let those states stutter
	// (list themselves as a successor).
	Deadlock bool

	// Temporal properties, checked once the reachable graph is complete.
	Temporal Temporal
}

// Violation is a failed check with a trace that demonstrates it.
type Violation struct {
	// Kind is "invariant", "step-invariant", "deadlock", "liveness",
	// "possibility" or "error".
	Kind string
	// Name is the property that failed; empty for deadlock.
	Name string
	// Err is the evaluation error when Kind is "error".
	Err error
	// Trace runs from an initial state to the failing state.
	Trace []Step
	// Loop describes how a liveness counterexample continues forever:
	// -1 means the last state stutters forever (nothing is enabled);
	// k >= 0 means the behaviour returns from the last state to
	// Trace[k] and repeats. Unused for other kinds.
	Loop int
}

func (v *Violation) Error() string {
	switch v.Kind {
	case "deadlock":
		return "deadlock reached"
	case "error":
		return fmt.Sprintf("evaluation error: %v", v.Err)
	case "possibility":
		return fmt.Sprintf("possibility %s violated: from the last state no behaviour ever reaches it", v.Name)
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

// edge is one successor of a state in the explored graph.
type edge struct {
	action string
	to     int
}

// graph is the reachable state graph built by Explore. Node i was found
// from parent[i] by action step[i].Action; initial states have parent -1.
// Nodes are numbered in breadth-first order, so following parents gives
// a shortest path from an initial state.
type graph struct {
	step   []Step
	parent []int
	depth  []int
	succ   [][]edge
	index  map[string]int
}

// pathTo returns the shortest path from an initial state to node i.
func (g *graph) pathTo(i int) []Step {
	var rev []Step
	for ; i >= 0; i = g.parent[i] {
		rev = append(rev, g.step[i])
	}
	out := make([]Step, len(rev))
	for j := range rev {
		out[j] = rev[len(rev)-1-j]
	}
	return out
}

// Explore enumerates the reachable states of m breadth-first, checking
// safety as it goes; a safety counterexample is therefore a shortest
// one. If the whole graph is explored without a safety violation, the
// temporal properties in opts are checked on it.
func (m *Machine) Explore(opts ExploreOptions) (*ExploreResult, error) {
	g := &graph{index: map[string]int{}}
	res := &ExploreResult{}

	fail := func(v *Violation) (*ExploreResult, error) {
		res.States = len(g.step)
		res.Violation = v
		return res, nil
	}
	// add returns the node for st, recording and checking it if new.
	add := func(st Step, parent int) (int, *Violation) {
		k := key(st.State)
		if i, ok := g.index[k]; ok {
			return i, nil
		}
		i := len(g.step)
		depth := 0
		if parent >= 0 {
			depth = g.depth[parent] + 1
		}
		g.index[k] = i
		g.step = append(g.step, st)
		g.parent = append(g.parent, parent)
		g.depth = append(g.depth, depth)
		g.succ = append(g.succ, nil)
		res.Depth = max(res.Depth, depth)
		name, err := m.brokenInvariant(st.State)
		if err != nil {
			return i, &Violation{Kind: "error", Name: name, Err: err, Trace: g.pathTo(i)}
		}
		if name != "" {
			return i, &Violation{Kind: "invariant", Name: name, Trace: g.pathTo(i)}
		}
		return i, nil
	}

	inits, err := m.Initial()
	if err != nil {
		return nil, err
	}
	for _, s := range inits {
		if _, v := add(Step{State: s}, -1); v != nil {
			return fail(v)
		}
	}

	for i := 0; i < len(g.step); i++ {
		if opts.MaxStates > 0 && len(g.step) >= opts.MaxStates {
			res.States = len(g.step)
			return res, nil
		}
		cur := g.step[i].State
		succs, err := m.Successors(cur)
		if err != nil {
			return fail(&Violation{Kind: "error", Err: err, Trace: g.pathTo(i)})
		}
		if len(succs) == 0 && opts.Deadlock {
			return fail(&Violation{Kind: "deadlock", Trace: g.pathTo(i)})
		}
		for _, st := range succs {
			res.Transitions++
			name, err := m.brokenStepInvariant(cur, st.State)
			if err != nil {
				return fail(&Violation{Kind: "error", Name: name, Err: err, Trace: append(g.pathTo(i), st)})
			}
			if name != "" {
				return fail(&Violation{Kind: "step-invariant", Name: name, Trace: append(g.pathTo(i), st)})
			}
			j, v := add(st, i)
			if v != nil {
				return fail(v)
			}
			g.succ[i] = append(g.succ[i], edge{action: st.Action, to: j})
		}
	}
	res.States = len(g.step)
	res.Complete = true

	v, err := m.checkTemporal(g, opts.Temporal)
	if err != nil {
		return nil, err
	}
	res.Violation = v
	return res, nil
}

// FormatTrace renders a trace one state per line, TLC style. start is
// the index of tr[0] in the full trace, for printing an excerpt.
func FormatTrace(tr []Step, start int) string {
	var b strings.Builder
	for i, st := range tr {
		action := st.Action
		if start+i == 0 {
			action = "<init>"
		} else if action == "" {
			action = "<next>"
		}
		fmt.Fprintf(&b, "  %3d  %-14s %s\n", start+i, action, st.State)
	}
	return b.String()
}

// FormatViolation renders a violation's trace, including how a liveness
// counterexample continues forever.
func FormatViolation(v *Violation) string {
	s := FormatTrace(v.Trace, 0)
	if v.Kind == "liveness" {
		if v.Loop < 0 {
			s += "       (stays here forever: no action is enabled)\n"
		} else {
			s += fmt.Sprintf("       (back to state %d, and repeats forever)\n", v.Loop)
		}
	}
	return s
}
