package symbolic

import (
	"fmt"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// VacuityVerdict is the per-datatype outcome of the inhabitation check.
type VacuityVerdict int

const (
	// VacuityUnknown means no solver was available or the constraints
	// left the supported fragment. Not a failure.
	VacuityUnknown VacuityVerdict = iota
	// VacuityInhabited means the solver found a value satisfying every
	// verified premise.
	VacuityInhabited
	// VacuityVacuous means the conjunction of the verified premises is
	// unsatisfiable over the premise types: the datatype has no values,
	// so every downstream claim resting on it is empty.
	VacuityVacuous
)

func (v VacuityVerdict) String() string {
	switch v {
	case VacuityInhabited:
		return "inhabited"
	case VacuityVacuous:
		return "vacuous"
	}
	return "unknown"
}

// VacuityFinding is the result for one (datatype …) with verified
// premises.
type VacuityFinding struct {
	// Type is the Shen type name the datatype concludes.
	Type string
	// Verdict is the inhabitation verdict.
	Verdict VacuityVerdict
	// Predicates are the verified premises, as written in the spec.
	Predicates []string
	// Message is a readable explanation, suitable for printing to a
	// spec author and for the discharge report's rule message.
	Message string
}

// CheckVacuity asks, for every datatype with verified premises, whether
// the conjunction of those premises is satisfiable over the premise
// types. It uses the same constraint layer as path enumeration, so a
// spec the path sampler understands is a spec vacuity understands.
//
// Datatypes with no verified premises are skipped: an unconstrained
// wrapper or composite is inhabited by construction.
func CheckVacuity(tt *specfile.TypeTable, solver Solver) ([]VacuityFinding, error) {
	if tt == nil {
		return nil, fmt.Errorf("nil type table")
	}
	names := make([]string, 0, len(tt.Entries))
	for name := range tt.Entries {
		names = append(names, name)
	}
	// Deterministic order.
	sortStrings(names)

	var out []VacuityFinding
	for _, name := range names {
		entry := tt.Entries[name]
		if len(entry.Verified) == 0 {
			continue
		}
		switch entry.Category {
		case specfile.CatConstrained, specfile.CatGuarded:
		default:
			continue
		}
		f := VacuityFinding{Type: name, Predicates: append([]string{}, entry.Verified...)}

		b := &skeletonBuilder{tt: tt, listLen: func(string) int { return 1 }}
		sv, err := b.buildSkeleton(name, name)
		if err != nil {
			f.Verdict = VacuityUnknown
			f.Message = fmt.Sprintf(
				"inhabitation of %s could not be checked: %v", name, err)
			out = append(out, f)
			continue
		}
		// buildSkeleton already collected this entry's own predicates
		// (and those of every constrained type nested inside it) into
		// b.constraints, which is exactly the conjunction to test.
		if solver == nil {
			f.Verdict = VacuityUnknown
			f.Message = fmt.Sprintf(
				"inhabitation of %s not checked: no SMT solver available", name)
			out = append(out, f)
			continue
		}
		vars := queryVars(&Path{Condition: b.constraints, Skeletons: []SymVal{sv}})
		res, err := solver.Check(&Query{Vars: vars, Assertions: b.constraints})
		if err != nil {
			f.Verdict = VacuityUnknown
			f.Message = fmt.Sprintf("inhabitation of %s could not be checked: %v", name, err)
			out = append(out, f)
			continue
		}
		switch res.Status {
		case StatusUnsat:
			f.Verdict = VacuityVacuous
			f.Message = fmt.Sprintf(
				"datatype %s is uninhabited: no value can satisfy all of its verified premises "+
					"(%s). Every rule that consumes a %s is therefore empty — no program can "+
					"construct one, so the guard proves nothing. Relax or remove one of the premises.",
				name, strings.Join(entry.Verified, " ∧ "), name)
		case StatusSat:
			f.Verdict = VacuityInhabited
			f.Message = fmt.Sprintf("datatype %s is inhabited: the solver produced a value satisfying %s.",
				name, strings.Join(entry.Verified, " ∧ "))
		default:
			f.Verdict = VacuityUnknown
			f.Message = fmt.Sprintf("inhabitation of %s is undecided (solver returned unknown).", name)
		}
		out = append(out, f)
	}
	return out, nil
}

// AnyVacuous reports whether any finding is vacuous.
func AnyVacuous(fs []VacuityFinding) bool {
	for _, f := range fs {
		if f.Verdict == VacuityVacuous {
			return true
		}
	}
	return false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
