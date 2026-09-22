package symbolic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// DefaultDepth is the list-unrolling bound: every list-typed site is
// enumerated at each concrete length from 0 to DefaultDepth. Four
// matches the boundary pool's list lengths and is the standard bounded
// choice for this kind of analysis.
const DefaultDepth = 4

// DefaultMaxPaths caps the enumeration so a spec with many branches
// cannot make the gate unbounded. Hitting the cap is recorded as a
// warning, never silently.
const DefaultMaxPaths = 64

// defaultMaxShapes caps the list-shape product for the same reason.
const defaultMaxShapes = 64

// Feasibility is a per-path verdict.
type Feasibility int

const (
	// FeasUnknown means no solver was available, or the solver could
	// not decide. The path is neither claimed reachable nor dead.
	FeasUnknown Feasibility = iota
	// FeasFeasible means the solver produced a model: the path is
	// reachable and the model is a concrete witness.
	FeasFeasible
	// FeasDead means the path condition is unsatisfiable — a dead spec
	// branch, which is useful backpressure on the spec author.
	FeasDead
)

func (f Feasibility) String() string {
	switch f {
	case FeasFeasible:
		return "feasible"
	case FeasDead:
		return "dead"
	}
	return "unknown"
}

// Config drives Enumerate.
type Config struct {
	// Spec is the (define …) to enumerate. Its type signature is
	// required: without parameter types there is nothing to build
	// symbolic inputs from.
	Spec *specfile.Define
	// TypeTable resolves declared Shen types to their field structure
	// and their verified predicates.
	TypeTable *specfile.TypeTable
	// AllDefines lets the evaluator resolve calls between defines in
	// the same spec file.
	AllDefines []*specfile.Define

	// Depth is the list-unrolling bound. Zero means DefaultDepth.
	Depth int
	// MaxPaths caps the number of enumerated paths. Zero means
	// DefaultMaxPaths.
	MaxPaths int
	// Solver answers feasibility. Nil means "no solver": paths are
	// still enumerated and every one is reported FeasUnknown.
	Solver Solver
	// SplitBooleanResult, when the spec returns a boolean, splits each
	// structural path into a true-outcome and a false-outcome path.
	// A boolean spec with no `if` has exactly one structural path per
	// list shape, and its interesting partition is over the outcome —
	// so this is what makes "one sample per feasible path" mean
	// something for predicate-shaped specs. Defaults to true when the
	// return type is boolean.
	SplitBooleanResult *bool
}

// Path is one enumerated execution path of the spec.
type Path struct {
	// ID is the path's index in enumeration order; it is the <n> in
	// the `path:<n>` provenance tag.
	ID int
	// Shape records the concrete length chosen for each list site, in
	// the order the sites were encountered.
	Shape []int
	// Condition is the path condition: the conjuncts an input must
	// satisfy to take this path. Includes the verified predicates of
	// every constrained type reachable from the parameters, so a
	// decoded model never violates a guard constructor.
	Condition []Term
	// Outcome names the result branch for a boolean-split path:
	// "true", "false", or "" when the path was not split.
	Outcome string
	// Skeletons are the symbolic parameter values, one per parameter.
	Skeletons []SymVal
	// Feasible is the solver's verdict.
	Feasible Feasibility
	// Params is the decoded concrete input, one value per parameter.
	// Non-nil only when Feasible == FeasFeasible.
	Params []core.Value
	// Note carries a human-readable remark (a dead branch's reason, a
	// solver error).
	Note string
}

// Result is the whole enumeration.
type Result struct {
	Paths []Path
	// Total, Feasible, and Dead are the three counters the generated
	// test header and the discharge report carry.
	Total    int
	Feasible int
	Dead     int
	// SolverName is the solver that answered, or "" when none did.
	SolverName string
	// SolverAvailable reports whether a solver answered at all.
	SolverAvailable bool
	// Depth is the list-unrolling bound actually used.
	Depth int
	// Warnings records enumeration limits hit and paths abandoned
	// because they left the supported fragment.
	Warnings []string
}

// Enumerate walks the spec body symbolically and returns one path per
// feasible execution, with a concrete witness attached to each.
//
// Enumeration never fails because of a missing solver: with cfg.Solver
// nil, every path comes back FeasUnknown and the caller falls back to
// its existing sampler.
func Enumerate(cfg *Config) (*Result, error) {
	if cfg == nil || cfg.Spec == nil {
		return nil, fmt.Errorf("nil spec")
	}
	if cfg.TypeTable == nil {
		return nil, fmt.Errorf("nil type table")
	}
	def := cfg.Spec
	if len(def.TypeSig.ParamTypes) == 0 {
		return nil, fmt.Errorf("spec %s: path enumeration requires a type signature", def.Name)
	}
	if len(def.ParamNames) != len(def.TypeSig.ParamTypes) {
		return nil, fmt.Errorf("spec %s: param count mismatch", def.Name)
	}

	depth := cfg.Depth
	if depth <= 0 {
		depth = DefaultDepth
	}
	maxPaths := cfg.MaxPaths
	if maxPaths <= 0 {
		maxPaths = DefaultMaxPaths
	}
	split := strings.TrimSpace(def.TypeSig.ReturnType) == "boolean"
	if cfg.SplitBooleanResult != nil {
		split = *cfg.SplitBooleanResult
	}

	res := &Result{Depth: depth}
	if cfg.Solver != nil {
		res.SolverName = cfg.Solver.Name()
	}

	base := baseSymEnv(cfg.TypeTable, cfg.AllDefines)

	for _, shape := range enumerateShapes(cfg, depth) {
		if len(res.Paths) >= maxPaths {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("path enumeration capped at %d paths", maxPaths))
			break
		}
		skeletons, typeConstraints, err := buildParams(cfg, shape)
		if err != nil {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("shape %v: %v", shape, err))
			continue
		}
		runs, warns := runDecisionTree(base, def, skeletons, maxPaths-len(res.Paths))
		res.Warnings = append(res.Warnings, warns...)
		for _, r := range runs {
			cond := append(append([]Term{}, typeConstraints...), r.pc...)
			outcomes := []struct {
				tag  string
				cond []Term
			}{{"", cond}}
			if split {
				if rb, ok := r.result.(*SBoolV); ok {
					if lit, isLit := BoolLit(rb.T); isLit {
						outcomes = []struct {
							tag  string
							cond []Term
						}{{fmt.Sprintf("%v", lit), cond}}
					} else {
						outcomes = []struct {
							tag  string
							cond []Term
						}{
							{"true", append(append([]Term{}, cond...), rb.T)},
							{"false", append(append([]Term{}, cond...), Not(rb.T))},
						}
					}
				}
			}
			for _, o := range outcomes {
				if len(res.Paths) >= maxPaths {
					res.Warnings = append(res.Warnings,
						fmt.Sprintf("path enumeration capped at %d paths", maxPaths))
					break
				}
				res.Paths = append(res.Paths, Path{
					ID:        len(res.Paths),
					Shape:     shape,
					Condition: o.cond,
					Outcome:   o.tag,
					Skeletons: skeletons,
				})
			}
		}
	}

	// Feasibility pass.
	for i := range res.Paths {
		p := &res.Paths[i]
		if cfg.Solver == nil {
			p.Feasible = FeasUnknown
			p.Note = "no SMT solver available; feasibility not established"
			continue
		}
		vars := queryVars(p)
		out, err := cfg.Solver.Check(&Query{Vars: vars, Assertions: p.Condition})
		if err != nil {
			p.Feasible = FeasUnknown
			p.Note = "solver error: " + err.Error()
			res.Warnings = append(res.Warnings, fmt.Sprintf("path %d: %v", p.ID, err))
			continue
		}
		res.SolverAvailable = true
		switch out.Status {
		case StatusUnsat:
			p.Feasible = FeasDead
			p.Note = "path condition is unsatisfiable (dead spec branch)"
		case StatusSat:
			model, derr := DecodeModel(vars, out.Model)
			if derr != nil {
				p.Feasible = FeasUnknown
				p.Note = "model decode failed: " + derr.Error()
				break
			}
			model = prettifyModel(cfg.Solver, vars, p.Condition, model)
			params := make([]core.Value, len(p.Skeletons))
			ok := true
			for j, sk := range p.Skeletons {
				cv, derr := DecodeSkeleton(sk, model)
				if derr != nil {
					p.Feasible = FeasUnknown
					p.Note = "model decode failed: " + derr.Error()
					ok = false
					break
				}
				params[j] = cv
			}
			if ok {
				p.Feasible = FeasFeasible
				p.Params = params
			}
		default:
			p.Feasible = FeasUnknown
			p.Note = "solver returned unknown"
		}
	}

	res.Total = len(res.Paths)
	for _, p := range res.Paths {
		switch p.Feasible {
		case FeasFeasible:
			res.Feasible++
		case FeasDead:
			res.Dead++
		}
	}
	return res, nil
}

// queryVars collects every variable the query must declare: those in
// the path condition plus every skeleton leaf, so the model is total
// over the inputs even when a parameter is unconstrained.
func queryVars(p *Path) []*Var {
	seen := map[string]*Var{}
	for _, v := range FreeVars(p.Condition...) {
		seen[v.Name] = v
	}
	for _, sk := range p.Skeletons {
		for _, v := range skeletonVars(sk) {
			seen[v.Name] = v
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Var, 0, len(names))
	for _, n := range names {
		out = append(out, seen[n])
	}
	return out
}

func skeletonVars(sv SymVal) []*Var {
	switch v := sv.(type) {
	case *SNum:
		return FreeVars(v.T)
	case *SStrV:
		return FreeVars(v.T)
	case *SBoolV:
		return FreeVars(v.T)
	case *SListV:
		var out []*Var
		for _, e := range v.Elems {
			out = append(out, skeletonVars(e)...)
		}
		return out
	case *STupleV:
		return append(skeletonVars(v.Fst), skeletonVars(v.Snd)...)
	}
	return nil
}

// prettifyModel tries to replace the solver's model with an
// equivalent, more readable one: first all-integer, then two-decimal.
// Z3 is free to answer a linear constraint with 1/3; a committed test
// reads much better with 0 or 1 in it. A prettified candidate is only
// adopted when the solver confirms it still satisfies the path.
func prettifyModel(s Solver, vars []*Var, cond []Term, model map[string]core.Value) map[string]core.Value {
	if s == nil {
		return model
	}
	round := func(f float64, places int) float64 {
		p := 1.0
		for i := 0; i < places; i++ {
			p *= 10
		}
		r := float64(int64(f*p+copySign(0.5, f))) / p
		return r
	}
	for _, places := range []int{0, 2} {
		cand := map[string]core.Value{}
		changed := false
		pins := append([]Term{}, cond...)
		for _, v := range vars {
			cur, ok := model[v.Name]
			if !ok {
				continue
			}
			cand[v.Name] = cur
			if v.S != SortReal {
				continue
			}
			f, isNum := core.AsNum(cur)
			if !isNum {
				continue
			}
			r := round(f, places)
			if r != f {
				changed = true
			}
			cand[v.Name] = numberValue(r)
			eq, err := EqTerm(v, R(r))
			if err != nil {
				return model
			}
			pins = append(pins, eq)
		}
		if !changed {
			return model
		}
		out, err := s.Check(&Query{Vars: vars, Assertions: pins})
		if err != nil || out.Status != StatusSat {
			continue
		}
		return cand
	}
	return model
}

func copySign(mag, sign float64) float64 {
	if sign < 0 {
		return -mag
	}
	return mag
}

// --- list shape enumeration ---

// enumerateShapes discovers the spec's list sites and returns every
// combination of concrete lengths up to depth. Sites are discovered
// incrementally because a nested list site only exists once its
// enclosing list is non-empty.
func enumerateShapes(cfg *Config, depth int) [][]int {
	var complete [][]int
	seen := map[string]bool{}
	queue := [][]int{{}}
	for len(queue) > 0 && len(complete) < defaultMaxShapes {
		vec := queue[0]
		queue = queue[1:]
		used, needMore := probeShape(cfg, vec, depth)
		canon := vec
		if used < len(canon) {
			canon = canon[:used]
		}
		key := fmt.Sprint(canon)
		if !needMore {
			if !seen[key] {
				seen[key] = true
				complete = append(complete, canon)
			}
			continue
		}
		for l := 0; l <= depth; l++ {
			next := append(append([]int{}, vec...), l)
			queue = append(queue, next)
		}
	}
	if len(complete) == 0 {
		complete = [][]int{{}}
	}
	sort.Slice(complete, func(i, j int) bool {
		a, b := complete[i], complete[j]
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
	return complete
}

// probeShape builds the parameter skeletons with the given length
// vector and reports how many list sites were requested and whether
// the vector was too short.
func probeShape(cfg *Config, vec []int, depth int) (used int, needMore bool) {
	idx := 0
	short := false
	b := &skeletonBuilder{tt: cfg.TypeTable}
	b.listLen = func(string) int {
		i := idx
		idx++
		if i < len(vec) {
			return vec[i]
		}
		short = true
		return 0
	}
	for i, pt := range cfg.Spec.TypeSig.ParamTypes {
		if _, err := b.buildSkeleton(pt, paramVarPrefix(cfg.Spec, i)); err != nil {
			return idx, false
		}
	}
	return idx, short
}

// buildParams builds the parameter skeletons for one concrete shape and
// returns them alongside the type constraints they imply.
func buildParams(cfg *Config, shape []int) ([]SymVal, []Term, error) {
	idx := 0
	b := &skeletonBuilder{tt: cfg.TypeTable}
	b.listLen = func(string) int {
		i := idx
		idx++
		if i < len(shape) {
			return shape[i]
		}
		return 0
	}
	out := make([]SymVal, len(cfg.Spec.TypeSig.ParamTypes))
	for i, pt := range cfg.Spec.TypeSig.ParamTypes {
		sv, err := b.buildSkeleton(pt, paramVarPrefix(cfg.Spec, i))
		if err != nil {
			return nil, nil, err
		}
		out[i] = sv
	}
	return out, b.constraints, nil
}

func paramVarPrefix(def *specfile.Define, i int) string {
	if i < len(def.ParamNames) && def.ParamNames[i] != "" {
		return strings.ToLower(def.ParamNames[i])
	}
	return fmt.Sprintf("p%d", i)
}

// --- decision-tree search ---

type runResult struct {
	pc     []Term
	result SymVal
}

// runDecisionTree explores every decision sequence of the spec body for
// one fixed list shape, in the generational style described on `exec`.
func runDecisionTree(base *SEnv, def *specfile.Define, skeletons []SymVal, budget int) ([]runResult, []string) {
	var out []runResult
	var warns []string
	if budget <= 0 {
		return out, warns
	}
	queue := [][]bool{nil}
	for len(queue) > 0 && len(out) < budget {
		prefix := queue[0]
		queue = queue[1:]
		ex := &symExec{decisions: prefix, base: base, maxCallDepth: defaultMaxCallDepth}
		val, err := ex.evalDefine(def, skeletons)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: path abandoned: %v", def.Name, err))
			// The prefix that led here is a dead end for enumeration
			// purposes, but its siblings may still be explorable.
			for j := len(prefix); j < len(ex.taken); j++ {
				queue = append(queue, flipAt(ex.taken, j))
			}
			continue
		}
		out = append(out, runResult{pc: append([]Term{}, ex.pc...), result: val})
		for j := len(prefix); j < len(ex.taken); j++ {
			queue = append(queue, flipAt(ex.taken, j))
		}
	}
	return out, warns
}

// flipAt returns the prefix taken[:j] with decision j inverted.
func flipAt(taken []bool, j int) []bool {
	out := make([]bool, j+1)
	copy(out, taken[:j])
	out[j] = !taken[j]
	return out
}
