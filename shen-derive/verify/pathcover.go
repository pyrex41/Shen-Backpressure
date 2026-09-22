package verify

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/symbolic"
)

// PathStats summarises a path-cover run. It is what the generated test
// file's header and the discharge report's three optional counters are
// rendered from.
type PathStats struct {
	// Total, Feasible, and Dead are the path counters. Total minus
	// Feasible minus Dead is the number of paths whose feasibility the
	// solver could not establish.
	Total    int
	Feasible int
	Dead     int
	// SolverName is the solver that answered ("z3"), or "" when none
	// was available.
	SolverName string
	// SolverAvailable reports whether a solver actually answered. False
	// means the feature degraded: paths were enumerated, feasibility is
	// unknown, and only the boundary pool supplies evidence.
	SolverAvailable bool
	// Depth is the list-unrolling bound used.
	Depth int
	// SamplesAdded is how many path witnesses became test cases.
	SamplesAdded int
	// Warnings records enumeration limits and paths that left the
	// supported fragment. Surfaced to the operator, not to the file.
	Warnings []string
	// DegradedReason explains why path cover produced no samples, when
	// that is the case.
	DegradedReason string
}

// runPathCover enumerates the spec's paths and turns every feasible
// one into a sample set. It never returns an error for a missing or
// undecided solver: the caller proceeds with the boundary pool and the
// stats record what happened.
func runPathCover(cfg *HarnessConfig) ([][]Sample, *PathStats, error) {
	solver := cfg.PathSolver
	stats := &PathStats{}
	if solver == nil {
		s, err := symbolic.FindSolver(cfg.PathTimeout)
		if err != nil {
			if !errors.Is(err, symbolic.ErrSolverUnavailable) {
				return nil, stats, err
			}
			// Degrade cleanly: still enumerate so the header reports
			// how many paths exist, but claim nothing about which are
			// reachable.
			stats.DegradedReason = "no SMT solver on PATH (looked for z3); " +
				"feasibility unknown, falling back to the boundary pool"
		} else {
			solver = s
		}
	}

	res, err := symbolic.Enumerate(&symbolic.Config{
		Spec:       cfg.Spec,
		TypeTable:  cfg.TypeTable,
		AllDefines: cfg.AllDefines,
		Depth:      cfg.PathDepth,
		MaxPaths:   cfg.PathMaxPaths,
		Solver:     solver,
	})
	if err != nil {
		return nil, stats, err
	}

	stats.Total = res.Total
	stats.Feasible = res.Feasible
	stats.Dead = res.Dead
	stats.SolverName = res.SolverName
	stats.SolverAvailable = res.SolverAvailable
	stats.Depth = res.Depth
	stats.Warnings = res.Warnings

	var out [][]Sample
	for _, p := range res.Paths {
		if p.Feasible != symbolic.FeasFeasible || p.Params == nil {
			continue
		}
		row := make([]Sample, len(p.Params))
		ok := true
		for i := range p.Params {
			expr, err := goExprFromSkeleton(p.Skeletons[i], p.Params[i], cfg.TypeTable)
			if err != nil {
				stats.Warnings = append(stats.Warnings,
					fmt.Sprintf("path %d: cannot emit a Go literal for param %d: %v", p.ID, i, err))
				ok = false
				break
			}
			row[i] = Sample{
				Value:      p.Params[i],
				GoExpr:     expr,
				Provenance: fmt.Sprintf("path:%d", p.ID),
			}
		}
		if ok {
			out = append(out, row)
		}
	}
	stats.SamplesAdded = len(out)
	if stats.SamplesAdded == 0 && stats.DegradedReason == "" {
		stats.DegradedReason = "no feasible path produced a witness"
	}
	return out, stats, nil
}

// goExprFromSkeleton renders a decoded path witness as a Go source
// expression, using the same mustXxx helper convention as the boundary
// pool so the two sample sources share one set of helpers.
func goExprFromSkeleton(sk symbolic.SymVal, val core.Value, tt *specfile.TypeTable) (string, error) {
	switch s := sk.(type) {
	case *symbolic.SNum:
		return wrapScalar(s.ShenType, val, tt)
	case *symbolic.SStrV:
		return wrapScalar(s.ShenType, val, tt)
	case *symbolic.SBoolV:
		return wrapScalar(s.ShenType, val, tt)

	case *symbolic.SListV:
		lv, ok := val.(core.ListVal)
		if !ok {
			return "", fmt.Errorf("expected a list value for %s, got %T", s.ShenType, val)
		}
		if len(lv) != len(s.Elems) {
			return "", fmt.Errorf("witness length %d does not match skeleton length %d", len(lv), len(s.Elems))
		}
		parts := make([]string, len(lv))
		for i := range lv {
			p, err := goExprFromSkeleton(s.Elems[i], lv[i], tt)
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		if s.ElemType != "" {
			// A genuine (list T).
			return fmt.Sprintf("[]%s{%s}", tt.GoType(s.ElemType), strings.Join(parts, ", ")), nil
		}
		// A composite datatype, built through its guard constructor.
		entry, ok := tt.Entries[s.ShenType]
		if !ok {
			return "", fmt.Errorf("unknown composite type %q", s.ShenType)
		}
		return fmt.Sprintf("must%s(%s)", entry.GoName, strings.Join(parts, ", ")), nil
	}
	return "", fmt.Errorf("cannot emit a Go literal for symbolic value %s", sk.String())
}

// wrapScalar renders one scalar, wrapping it in the guard constructor
// helper when its Shen type is a wrapper or constrained type.
func wrapScalar(shenType string, val core.Value, tt *specfile.TypeTable) (string, error) {
	lit, err := primLiteral(val)
	if err != nil {
		return "", err
	}
	if entry, ok := tt.Entries[shenType]; ok {
		switch entry.Category {
		case specfile.CatWrapper, specfile.CatConstrained:
			return fmt.Sprintf("must%s(%s)", entry.GoName, lit), nil
		}
	}
	return lit, nil
}

func primLiteral(val core.Value) (string, error) {
	switch v := val.(type) {
	case core.IntVal:
		return fmt.Sprintf("%d", int64(v)), nil
	case core.FloatVal:
		return formatFloatLiteral(float64(v)), nil
	case core.StringVal:
		return fmt.Sprintf("%q", string(v)), nil
	case core.BoolVal:
		if bool(v) {
			return "true", nil
		}
		return "false", nil
	}
	return "", fmt.Errorf("no Go literal for %T", val)
}
