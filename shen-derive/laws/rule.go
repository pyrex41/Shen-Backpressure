// Package laws implements the Bird-Meertens rewrite rule catalog for
// shen-derive. Each rule is a named algebraic law with a LHS pattern,
// RHS pattern, side conditions, and a textbook citation.
//
// Patterns use metavariables (names starting with "?") to match arbitrary
// subterms. First-order matching binds metavariables to subterms;
// substitution fills them into the RHS.
package laws

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// Rule represents a named algebraic rewrite law.
type Rule struct {
	// Name is the rule identifier, e.g., "map-fusion".
	Name string

	// LHS is the pattern to match (contains metavariables like ?f, ?g).
	LHS core.Term

	// RHS is the replacement template (metavariables filled by matching).
	RHS core.Term

	// SideConditions are term equalities that must hold for the rewrite
	// to be valid. Each condition's metavariables are instantiated by
	// the match. Empty means the rule is unconditional.
	SideConditions []SideCondition

	// Citation is the textbook source for this law.
	Citation string
}

// SideCondition represents a proof obligation: LHS = RHS for all values
// of the universally quantified free variables.
type SideCondition struct {
	// Description is a human-readable statement of the condition.
	Description string

	// LHS and RHS of the equality to prove.
	LHS core.Term
	RHS core.Term
}

// InstantiatedCondition is a side condition with metavariables filled in.
type InstantiatedCondition struct {
	Description string
	LHS         core.Term
	RHS         core.Term
}

// RewriteResult holds the output of applying a rewrite rule.
type RewriteResult struct {
	// Original is the input term.
	Original core.Term

	// Rewritten is the output term after the rewrite.
	Rewritten core.Term

	// RuleName is which rule was applied.
	RuleName string

	// Obligations are the instantiated side conditions that must be
	// discharged for the rewrite to be valid.
	Obligations []InstantiatedCondition
}

// --- Metavariable helpers ---

// IsMetaVar returns true if a variable name is a metavariable (starts with '?').
func IsMetaVar(name string) bool {
	return len(name) > 0 && name[0] == '?'
}

// MetaVar creates a metavariable reference.
func MetaVar(name string) *core.Var {
	return &core.Var{Name: name}
}

// --- Pattern Matching ---

// Bindings maps metavariable names to the subterms they matched.
type Bindings map[string]core.Term

// Match attempts first-order pattern matching: does the pattern match the
// term, binding metavariables? Returns bindings on success, nil on failure.
func Match(pattern, term core.Term) Bindings {
	bindings := make(Bindings)
	if matchTerm(pattern, term, bindings) {
		return bindings
	}
	return nil
}

func matchTerm(pattern, term core.Term, b Bindings) bool {
	switch p := pattern.(type) {
	case *core.Var:
		if IsMetaVar(p.Name) {
			// Metavariable: bind or check consistency
			if existing, ok := b[p.Name]; ok {
				return termsEqual(existing, term)
			}
			b[p.Name] = term
			return true
		}
		// Concrete variable: must match exactly
		if v, ok := term.(*core.Var); ok {
			return p.Name == v.Name
		}
		return false

	case *core.Prim:
		if tp, ok := term.(*core.Prim); ok {
			return p.Op == tp.Op
		}
		return false

	case *core.Lit:
		if tl, ok := term.(*core.Lit); ok {
			return p.Kind == tl.Kind && p.IntVal == tl.IntVal &&
				p.BoolVal == tl.BoolVal && p.StrVal == tl.StrVal
		}
		return false

	case *core.App:
		if ta, ok := term.(*core.App); ok {
			return matchTerm(p.Func, ta.Func, b) && matchTerm(p.Arg, ta.Arg, b)
		}
		return false

	case *core.Lam:
		if tl, ok := term.(*core.Lam); ok {
			// For first-order matching, lambda params must match literally
			if p.Param != tl.Param {
				return false
			}
			return matchTerm(p.Body, tl.Body, b)
		}
		return false

	case *core.ListLit:
		if tl, ok := term.(*core.ListLit); ok {
			if len(p.Elems) != len(tl.Elems) {
				return false
			}
			for i := range p.Elems {
				if !matchTerm(p.Elems[i], tl.Elems[i], b) {
					return false
				}
			}
			return true
		}
		return false

	case *core.TupleLit:
		if tt, ok := term.(*core.TupleLit); ok {
			return matchTerm(p.Fst, tt.Fst, b) && matchTerm(p.Snd, tt.Snd, b)
		}
		return false

	case *core.IfExpr:
		if ti, ok := term.(*core.IfExpr); ok {
			return matchTerm(p.Cond, ti.Cond, b) &&
				matchTerm(p.Then, ti.Then, b) &&
				matchTerm(p.Else, ti.Else, b)
		}
		return false

	case *core.Let:
		if tl, ok := term.(*core.Let); ok {
			if p.Name != tl.Name {
				return false
			}
			return matchTerm(p.Bound, tl.Bound, b) &&
				matchTerm(p.Body, tl.Body, b)
		}
		return false
	}

	return false
}

// --- Substitution ---

// Substitute replaces all metavariables in a template with their bindings.
func Substitute(template core.Term, b Bindings) core.Term {
	return substTerm(template, b)
}

func substTerm(t core.Term, b Bindings) core.Term {
	switch t := t.(type) {
	case *core.Var:
		if IsMetaVar(t.Name) {
			if bound, ok := b[t.Name]; ok {
				return bound
			}
		}
		return t

	case *core.Prim:
		return t

	case *core.Lit:
		return t

	case *core.App:
		return &core.App{
			Func: substTerm(t.Func, b),
			Arg:  substTerm(t.Arg, b),
			P:    t.P,
		}

	case *core.Lam:
		return &core.Lam{
			Param:     t.Param,
			ParamType: t.ParamType,
			Body:      substTerm(t.Body, b),
			P:         t.P,
		}

	case *core.Let:
		return &core.Let{
			Name:  t.Name,
			Bound: substTerm(t.Bound, b),
			Body:  substTerm(t.Body, b),
			P:     t.P,
		}

	case *core.ListLit:
		elems := make([]core.Term, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = substTerm(e, b)
		}
		return &core.ListLit{Elems: elems, P: t.P}

	case *core.TupleLit:
		return &core.TupleLit{
			Fst: substTerm(t.Fst, b),
			Snd: substTerm(t.Snd, b),
			P:   t.P,
		}

	case *core.IfExpr:
		return &core.IfExpr{
			Cond: substTerm(t.Cond, b),
			Then: substTerm(t.Then, b),
			Else: substTerm(t.Else, b),
			P:    t.P,
		}
	}

	return t
}

// --- Term equality (structural, ignoring positions) ---

func termsEqual(a, b core.Term) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch a := a.(type) {
	case *core.Var:
		if bv, ok := b.(*core.Var); ok {
			return a.Name == bv.Name
		}
	case *core.Prim:
		if bp, ok := b.(*core.Prim); ok {
			return a.Op == bp.Op
		}
	case *core.Lit:
		if bl, ok := b.(*core.Lit); ok {
			return a.Kind == bl.Kind && a.IntVal == bl.IntVal &&
				a.BoolVal == bl.BoolVal && a.StrVal == bl.StrVal
		}
	case *core.App:
		if ba, ok := b.(*core.App); ok {
			return termsEqual(a.Func, ba.Func) && termsEqual(a.Arg, ba.Arg)
		}
	case *core.Lam:
		if bl, ok := b.(*core.Lam); ok {
			return a.Param == bl.Param && termsEqual(a.Body, bl.Body)
		}
	case *core.ListLit:
		if bl, ok := b.(*core.ListLit); ok {
			if len(a.Elems) != len(bl.Elems) {
				return false
			}
			for i := range a.Elems {
				if !termsEqual(a.Elems[i], bl.Elems[i]) {
					return false
				}
			}
			return true
		}
	case *core.TupleLit:
		if bt, ok := b.(*core.TupleLit); ok {
			return termsEqual(a.Fst, bt.Fst) && termsEqual(a.Snd, bt.Snd)
		}
	case *core.IfExpr:
		if bi, ok := b.(*core.IfExpr); ok {
			return termsEqual(a.Cond, bi.Cond) &&
				termsEqual(a.Then, bi.Then) &&
				termsEqual(a.Else, bi.Else)
		}
	case *core.Let:
		if bl, ok := b.(*core.Let); ok {
			return a.Name == bl.Name &&
				termsEqual(a.Bound, bl.Bound) &&
				termsEqual(a.Body, bl.Body)
		}
	}
	return false
}

// TermsEqual is the exported version of structural term equality.
func TermsEqual(a, b core.Term) bool {
	return termsEqual(a, b)
}

func unresolvedMetaVarsInTerm(term core.Term, acc map[string]bool) {
	switch t := term.(type) {
	case *core.Var:
		if IsMetaVar(t.Name) {
			acc[t.Name] = true
		}
	case *core.App:
		unresolvedMetaVarsInTerm(t.Func, acc)
		unresolvedMetaVarsInTerm(t.Arg, acc)
	case *core.Lam:
		unresolvedMetaVarsInTerm(t.Body, acc)
	case *core.Let:
		unresolvedMetaVarsInTerm(t.Bound, acc)
		unresolvedMetaVarsInTerm(t.Body, acc)
	case *core.ListLit:
		for _, el := range t.Elems {
			unresolvedMetaVarsInTerm(el, acc)
		}
	case *core.TupleLit:
		unresolvedMetaVarsInTerm(t.Fst, acc)
		unresolvedMetaVarsInTerm(t.Snd, acc)
	case *core.IfExpr:
		unresolvedMetaVarsInTerm(t.Cond, acc)
		unresolvedMetaVarsInTerm(t.Then, acc)
		unresolvedMetaVarsInTerm(t.Else, acc)
	}
}

func ruleMentionsMetaVar(rule *Rule, name string) bool {
	acc := make(map[string]bool)
	unresolvedMetaVarsInTerm(rule.LHS, acc)
	unresolvedMetaVarsInTerm(rule.RHS, acc)
	for _, sc := range rule.SideConditions {
		unresolvedMetaVarsInTerm(sc.LHS, acc)
		unresolvedMetaVarsInTerm(sc.RHS, acc)
	}
	return acc[name]
}

func unresolvedMetaVarsInResult(result *RewriteResult) []string {
	acc := make(map[string]bool)
	unresolvedMetaVarsInTerm(result.Rewritten, acc)
	for _, ob := range result.Obligations {
		unresolvedMetaVarsInTerm(ob.LHS, acc)
		unresolvedMetaVarsInTerm(ob.RHS, acc)
	}
	if len(acc) == 0 {
		return nil
	}

	names := make([]string, 0, len(acc))
	for name := range acc {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateRewriteResult(ruleName string, result *RewriteResult) error {
	if unresolved := unresolvedMetaVarsInResult(result); len(unresolved) > 0 {
		return fmt.Errorf("rewrite %s: unresolved metavariables remain after substitution: %s",
			ruleName, strings.Join(unresolved, ", "))
	}
	return nil
}

// --- Location-based rewriting ---

// Path identifies a position in a term tree. Each element selects a child:
//   0 = Func of App, Body of Lam, Cond of If, Fst of Tuple, Bound of Let
//   1 = Arg of App, Then of If, Snd of Tuple, Body of Let
//   2 = Else of If
//   For ListLit: index of element
type Path []int

// RootPath is the path to the root of the term.
var RootPath = Path{}

// AtPath returns the subterm at the given path, or an error.
func AtPath(term core.Term, path Path) (core.Term, error) {
	cur := term
	for i, step := range path {
		child, err := nthChild(cur, step)
		if err != nil {
			return nil, fmt.Errorf("at path step %d: %w", i, err)
		}
		cur = child
	}
	return cur, nil
}

// ReplacePath returns a new term with the subterm at path replaced.
func ReplacePath(term core.Term, path Path, replacement core.Term) (core.Term, error) {
	if len(path) == 0 {
		return replacement, nil
	}
	return replaceAt(term, path, 0, replacement)
}

func nthChild(t core.Term, n int) (core.Term, error) {
	switch t := t.(type) {
	case *core.App:
		switch n {
		case 0:
			return t.Func, nil
		case 1:
			return t.Arg, nil
		}
	case *core.Lam:
		if n == 0 {
			return t.Body, nil
		}
	case *core.Let:
		switch n {
		case 0:
			return t.Bound, nil
		case 1:
			return t.Body, nil
		}
	case *core.IfExpr:
		switch n {
		case 0:
			return t.Cond, nil
		case 1:
			return t.Then, nil
		case 2:
			return t.Else, nil
		}
	case *core.ListLit:
		if n >= 0 && n < len(t.Elems) {
			return t.Elems[n], nil
		}
	case *core.TupleLit:
		switch n {
		case 0:
			return t.Fst, nil
		case 1:
			return t.Snd, nil
		}
	}
	return nil, fmt.Errorf("no child %d in %T", n, t)
}

func replaceAt(t core.Term, path Path, idx int, repl core.Term) (core.Term, error) {
	if idx == len(path) {
		return repl, nil
	}
	step := path[idx]

	switch t := t.(type) {
	case *core.App:
		switch step {
		case 0:
			newFunc, err := replaceAt(t.Func, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.App{Func: newFunc, Arg: t.Arg, P: t.P}, nil
		case 1:
			newArg, err := replaceAt(t.Arg, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.App{Func: t.Func, Arg: newArg, P: t.P}, nil
		}
	case *core.Lam:
		if step == 0 {
			newBody, err := replaceAt(t.Body, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.Lam{Param: t.Param, ParamType: t.ParamType, Body: newBody, P: t.P}, nil
		}
	case *core.Let:
		switch step {
		case 0:
			newBound, err := replaceAt(t.Bound, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.Let{Name: t.Name, Bound: newBound, Body: t.Body, P: t.P}, nil
		case 1:
			newBody, err := replaceAt(t.Body, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.Let{Name: t.Name, Bound: t.Bound, Body: newBody, P: t.P}, nil
		}
	case *core.IfExpr:
		switch step {
		case 0:
			newCond, err := replaceAt(t.Cond, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.IfExpr{Cond: newCond, Then: t.Then, Else: t.Else, P: t.P}, nil
		case 1:
			newThen, err := replaceAt(t.Then, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.IfExpr{Cond: t.Cond, Then: newThen, Else: t.Else, P: t.P}, nil
		case 2:
			newElse, err := replaceAt(t.Else, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.IfExpr{Cond: t.Cond, Then: t.Then, Else: newElse, P: t.P}, nil
		}
	case *core.ListLit:
		if step >= 0 && step < len(t.Elems) {
			newElem, err := replaceAt(t.Elems[step], path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			elems := make([]core.Term, len(t.Elems))
			copy(elems, t.Elems)
			elems[step] = newElem
			return &core.ListLit{Elems: elems, P: t.P}, nil
		}
	case *core.TupleLit:
		switch step {
		case 0:
			newFst, err := replaceAt(t.Fst, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.TupleLit{Fst: newFst, Snd: t.Snd, P: t.P}, nil
		case 1:
			newSnd, err := replaceAt(t.Snd, path, idx+1, repl)
			if err != nil {
				return nil, err
			}
			return &core.TupleLit{Fst: t.Fst, Snd: newSnd, P: t.P}, nil
		}
	}

	return nil, fmt.Errorf("cannot navigate step %d in %T", step, t)
}

// --- Rewrite engine ---

// Rewrite applies a named rule at the given path in the term.
// Returns the rewritten term and any instantiated side conditions.
func Rewrite(term core.Term, rule *Rule, path Path) (*RewriteResult, error) {
	// Navigate to the target subterm
	target, err := AtPath(term, path)
	if err != nil {
		return nil, fmt.Errorf("rewrite %s: %w", rule.Name, err)
	}

	// Match LHS pattern against target
	bindings := Match(rule.LHS, target)
	if bindings == nil {
		return nil, fmt.Errorf("rewrite %s: LHS pattern does not match term at path %v\n  pattern: %s\n  term:    %s",
			rule.Name, path, core.PrettyPrint(rule.LHS), core.PrettyPrint(target))
	}

	// Produce the rewritten subterm
	rewritten := Substitute(rule.RHS, bindings)

	// Replace in the original term
	newTerm, err := ReplacePath(term, path, rewritten)
	if err != nil {
		return nil, fmt.Errorf("rewrite %s: %w", rule.Name, err)
	}

	// Instantiate side conditions
	var obligations []InstantiatedCondition
	for _, sc := range rule.SideConditions {
		obligations = append(obligations, InstantiatedCondition{
			Description: sc.Description,
			LHS:         Substitute(sc.LHS, bindings),
			RHS:         Substitute(sc.RHS, bindings),
		})
	}

	result := &RewriteResult{
		Original:    term,
		Rewritten:   newTerm,
		RuleName:    rule.Name,
		Obligations: obligations,
	}
	if err := validateRewriteResult(rule.Name, result); err != nil {
		return nil, err
	}
	return result, nil
}
