package check

import (
	"fmt"
	"path"
	"slices"
)

// Temporal lists the temporal properties to check on a complete graph.
// Each property names a (define P S -> boolean) state predicate.
//
// Liveness (Eventually, LeadsTo) is checked under the assumption TLA+
// specs usually write as WF_vars(Next): a behaviour never stutters
// forever in a state where some action is enabled. It ends only by
// stuttering in a state with no successors. WF and SF add fairness for
// individual actions on top of that.
type Temporal struct {
	// Eventually lists P for <>P: every behaviour reaches a P state.
	Eventually []string
	// LeadsTo lists P ~> Q: after any P state, a Q state follows.
	LeadsTo []LeadsTo
	// Possible lists P for the CTL property AG EF P: from every
	// reachable state, some behaviour can still reach a P state. This is
	// a branching-time property; TLA+ cannot state it.
	Possible []string
	// WF and SF give weak and strong fairness to actions, by label. A
	// label may be a glob ("* votes *"), and the constraint then applies
	// to the disjunction of the matching actions: SF on "* votes *" says
	// some vote happens, not that every computer votes.
	WF []string
	SF []string
}

// LeadsTo is the property P ~> Q.
type LeadsTo struct{ P, Q string }

func (t Temporal) empty() bool {
	return len(t.Eventually) == 0 && len(t.LeadsTo) == 0 && len(t.Possible) == 0
}

// fairness is one WF or SF constraint.
type fairness struct {
	pattern string
	strong  bool
}

func (f fairness) matches(action string) bool {
	ok, err := path.Match(f.pattern, action)
	return err == nil && ok
}

func (m *Machine) checkTemporal(g *graph, t Temporal) (*Violation, error) {
	if t.empty() {
		return nil, nil
	}
	preds := map[string][]bool{}
	pred := func(name string) ([]bool, error) {
		if vals, ok := preds[name]; ok {
			return vals, nil
		}
		d, err := m.find(name, 1)
		if err != nil {
			return nil, err
		}
		vals := make([]bool, len(g.step))
		for i, st := range g.step {
			if vals[i], err = m.holds(d, st.State); err != nil {
				return nil, err
			}
		}
		preds[name] = vals
		return vals, nil
	}
	var fair []fairness
	for _, p := range t.WF {
		fair = append(fair, fairness{pattern: p})
	}
	for _, p := range t.SF {
		fair = append(fair, fairness{pattern: p, strong: true})
	}

	for _, name := range t.Eventually {
		p, err := pred(name)
		if err != nil {
			return nil, err
		}
		var sources []int
		for i := range g.step {
			if g.parent[i] == -1 && !p[i] {
				sources = append(sources, i)
			}
		}
		if v := g.liveness(sources, p, fair); v != nil {
			v.Name = "<>" + name
			return v, nil
		}
	}
	for _, lt := range t.LeadsTo {
		p, err := pred(lt.P)
		if err != nil {
			return nil, err
		}
		q, err := pred(lt.Q)
		if err != nil {
			return nil, err
		}
		var sources []int
		for i := range g.step {
			if p[i] && !q[i] {
				sources = append(sources, i)
			}
		}
		if v := g.liveness(sources, q, fair); v != nil {
			v.Name = lt.P + " ~> " + lt.Q
			return v, nil
		}
	}
	for _, name := range t.Possible {
		p, err := pred(name)
		if err != nil {
			return nil, err
		}
		if i := g.cannotReach(p); i >= 0 {
			return &Violation{Kind: "possibility", Name: "AG EF " + name, Trace: g.pathTo(i)}, nil
		}
	}
	return nil, nil
}

// liveness looks for a fair behaviour that starts at one of sources and
// never visits a goal state. It returns that behaviour as a lasso, or nil
// if every fair behaviour from sources reaches a goal.
func (g *graph) liveness(sources []int, goal []bool, fair []fairness) *Violation {
	// Breadth-first search from sources through non-goal states. The
	// behaviours we are looking for stay inside this region.
	from := map[int]int{} // node -> predecessor in the region; sources map to -1
	root := map[int]int{} // node -> the source it was reached from
	var region []int
	for _, s := range sources {
		if _, ok := from[s]; !ok {
			from[s], root[s] = -1, s
			region = append(region, s)
		}
	}
	for i := 0; i < len(region); i++ {
		u := region[i]
		for _, e := range g.succ[u] {
			if _, ok := from[e.to]; !ok && !goal[e.to] {
				from[e.to], root[e.to] = u, root[u]
				region = append(region, e.to)
			}
		}
	}
	// prefix is a path from an initial state to node u through the region.
	prefix := func(u int) []Step {
		var mid []int
		for x := u; from[x] != -1; x = from[x] {
			mid = append(mid, x)
		}
		out := g.pathTo(root[u])
		for j := len(mid) - 1; j >= 0; j-- {
			out = append(out, g.stepVia(from[mid[j]], mid[j]))
		}
		return out
	}

	// A state with no successors ends a behaviour that stutters there
	// forever. No action is enabled, so it is fair.
	for _, u := range region {
		if len(g.succ[u]) == 0 {
			return &Violation{Kind: "liveness", Trace: prefix(u), Loop: -1}
		}
	}

	in := func(set map[int]bool) func(int) bool { return func(u int) bool { return set[u] } }
	inRegion := map[int]bool{}
	for _, u := range region {
		inRegion[u] = true
	}
	for _, c := range g.sccs(region, in(inRegion)) {
		comp := g.fairComponent(c, fair)
		if comp == nil {
			continue
		}
		// Enter at the component state found first, then go round.
		entry := -1
		for _, u := range region {
			if comp[u] {
				entry = u
				break
			}
		}
		tr := prefix(entry)
		loop := len(tr) - 1
		return &Violation{Kind: "liveness", Trace: append(tr, g.fairCycle(entry, comp, fair)...), Loop: loop}
	}
	return nil
}

// fairComponent returns a subset of the strongly connected component c
// in which a behaviour can cycle forever while respecting every fairness
// constraint, or nil if there is none.
func (g *graph) fairComponent(c []int, fair []fairness) map[int]bool {
	set := map[int]bool{}
	for _, u := range c {
		set[u] = true
	}
	if !g.cyclic(c, set) {
		return nil
	}
	for _, f := range fair {
		if g.takenWithin(set, f) {
			continue
		}
		var enabled, disabled []int
		for _, u := range c {
			if g.enables(u, f) {
				enabled = append(enabled, u)
			} else {
				disabled = append(disabled, u)
			}
		}
		if len(enabled) == 0 {
			continue // never enabled here, so never owed
		}
		if !f.strong {
			// WF is met by visiting a state where the action is
			// disabled; if there is none, no sub-cycle helps either.
			if len(disabled) == 0 {
				return nil
			}
			continue
		}
		// SF: an action enabled infinitely often must be taken, so a
		// fair cycle has to avoid every state that enables it.
		sub := map[int]bool{}
		for _, u := range disabled {
			sub[u] = true
		}
		for _, d := range g.sccs(disabled, func(u int) bool { return sub[u] }) {
			if r := g.fairComponent(d, fair); r != nil {
				return r
			}
		}
		return nil
	}
	return set
}

// fairCycle returns a cycle from entry back to entry inside comp that
// takes, or visits a state disabling, each fairness-constrained action.
// The returned steps exclude entry itself; the last step's successor is
// entry again.
func (g *graph) fairCycle(entry int, comp map[int]bool, fair []fairness) []Step {
	var out []Step
	at := entry
	walk := func(target int) {
		if target == at {
			return
		}
		path := g.pathWithin(at, target, comp)
		for j := 1; j < len(path); j++ {
			out = append(out, g.stepVia(path[j-1], path[j]))
		}
		at = target
	}
	moved := false
	for _, f := range fair {
		if u, v, ok := g.edgeWithin(comp, f); ok {
			walk(u)
			out = append(out, g.stepVia(u, v))
			at, moved = v, true
		} else {
			for _, u := range members(comp) {
				if !g.enables(u, f) {
					if u != at {
						walk(u)
						moved = true
					}
					break
				}
			}
		}
	}
	if !moved {
		// Take any step inside the component to start the loop.
		for _, e := range g.succ[entry] {
			if comp[e.to] {
				out = append(out, g.stepVia(entry, e.to))
				at = e.to
				break
			}
		}
	}
	walk(entry)
	// The last step lands on entry again; drop it, since the caller
	// prints "back to state <entry>" instead.
	return out[:len(out)-1]
}

// members returns the nodes of set in ascending order, so traces are
// deterministic.
func members(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	slices.Sort(out)
	return out
}

// cannotReach returns the first reachable state from which no path
// reaches a goal state, or -1 if there is none.
func (g *graph) cannotReach(goal []bool) int {
	pred := make([][]int, len(g.step))
	for u, es := range g.succ {
		for _, e := range es {
			pred[e.to] = append(pred[e.to], u)
		}
	}
	can := make([]bool, len(g.step))
	var queue []int
	for i, ok := range goal {
		if ok {
			can[i] = true
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, u := range pred[v] {
			if !can[u] {
				can[u] = true
				queue = append(queue, u)
			}
		}
	}
	for i := range g.step {
		if !can[i] {
			return i
		}
	}
	return -1
}

func (g *graph) enables(u int, f fairness) bool {
	for _, e := range g.succ[u] {
		if f.matches(e.action) {
			return true
		}
	}
	return false
}

func (g *graph) edgeWithin(set map[int]bool, f fairness) (int, int, bool) {
	for _, u := range members(set) {
		for _, e := range g.succ[u] {
			if set[e.to] && f.matches(e.action) {
				return u, e.to, true
			}
		}
	}
	return 0, 0, false
}

func (g *graph) takenWithin(set map[int]bool, f fairness) bool {
	_, _, ok := g.edgeWithin(set, f)
	return ok
}

// cyclic reports whether the component c has an internal edge, which
// for a strongly connected component means it contains a cycle.
func (g *graph) cyclic(c []int, set map[int]bool) bool {
	for _, u := range c {
		for _, e := range g.succ[u] {
			if set[e.to] {
				return true
			}
		}
	}
	return false
}

// stepVia returns the step that goes from u to v, labelled with the
// first action that does so.
func (g *graph) stepVia(u, v int) Step {
	for _, e := range g.succ[u] {
		if e.to == v {
			return Step{Action: e.action, State: g.step[v].State}
		}
	}
	panic(fmt.Sprintf("check: no edge %d -> %d", u, v))
}

// pathWithin returns a shortest path of nodes from u to v (u != v) using
// only nodes in set.
func (g *graph) pathWithin(u, v int, set map[int]bool) []int {
	prev := map[int]int{}
	queue := []int{u}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		for _, e := range g.succ[x] {
			if !set[e.to] {
				continue
			}
			if _, seen := prev[e.to]; seen {
				continue
			}
			prev[e.to] = x
			if e.to == v {
				path := []int{v}
				for y := x; y != u; y = prev[y] {
					path = append(path, y)
				}
				path = append(path, u)
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path
			}
			queue = append(queue, e.to)
		}
	}
	panic(fmt.Sprintf("check: no path %d -> %d inside component", u, v))
}

// sccs returns the strongly connected components of the subgraph induced
// by nodes (Tarjan's algorithm, iterative).
func (g *graph) sccs(nodes []int, in func(int) bool) [][]int {
	index := map[int]int{}
	low := map[int]int{}
	onStack := map[int]bool{}
	var stack []int
	var out [][]int
	next := 0

	type frame struct{ u, i int }
	for _, start := range nodes {
		if _, ok := index[start]; ok {
			continue
		}
		call := []frame{{start, 0}}
		index[start], low[start] = next, next
		next++
		stack = append(stack, start)
		onStack[start] = true
		for len(call) > 0 {
			f := &call[len(call)-1]
			if f.i < len(g.succ[f.u]) {
				w := g.succ[f.u][f.i].to
				f.i++
				if !in(w) {
					continue
				}
				if _, ok := index[w]; !ok {
					index[w], low[w] = next, next
					next++
					stack = append(stack, w)
					onStack[w] = true
					call = append(call, frame{w, 0})
				} else if onStack[w] {
					low[f.u] = min(low[f.u], index[w])
				}
				continue
			}
			u := f.u
			call = call[:len(call)-1]
			if len(call) > 0 {
				p := call[len(call)-1].u
				low[p] = min(low[p], low[u])
			}
			if low[u] == index[u] {
				var comp []int
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp = append(comp, w)
					if w == u {
						break
					}
				}
				out = append(out, comp)
			}
		}
	}
	return out
}
