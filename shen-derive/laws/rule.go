// Package laws implements the Bird-Meertens rewrite rule catalog for
// shen-derive. Rules operate on s-expressions — the same representation
// used by Shen itself.
//
// Patterns use metavariables (symbols starting with "?") to match arbitrary
// subexpressions. First-order matching binds metavariables to subtrees;
// substitution fills them into the RHS template.
package laws

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// Rule represents a named algebraic rewrite law.
type Rule struct {
	Name           string
	LHS            core.Sexpr
	RHS            core.Sexpr
	SideConditions []SideCondition
	Citation       string
}

// SideCondition represents a proof obligation: LHS = RHS for all values
// of the universally quantified free variables.
type SideCondition struct {
	Description string
	LHS         core.Sexpr
	RHS         core.Sexpr
}

// InstantiatedCondition is a side condition with metavariables filled in.
type InstantiatedCondition struct {
	Description string
	LHS         core.Sexpr
	RHS         core.Sexpr
}

// RewriteResult holds the output of applying a rewrite rule.
type RewriteResult struct {
	Original    core.Sexpr
	Rewritten   core.Sexpr
	RuleName    string
	Obligations []InstantiatedCondition
}

// --- Bindings ---

// Bindings maps metavariable names to the subtrees they matched.
type Bindings map[string]core.Sexpr

// --- Pattern Matching ---

// Match attempts first-order pattern matching on s-expressions.
// Metavariables (symbols starting with ?) match any subtree.
// Returns bindings on success, nil on failure.
func Match(pattern, term core.Sexpr) Bindings {
	bindings := make(Bindings)
	if matchSexpr(pattern, term, bindings) {
		return bindings
	}
	return nil
}

func matchSexpr(pattern, term core.Sexpr, b Bindings) bool {
	switch p := pattern.(type) {
	case *core.Atom:
		if name, ok := core.IsMetaVar(p); ok {
			// Metavariable: bind or check consistency
			if existing, ok := b[name]; ok {
				return existing.Equal(term)
			}
			b[name] = term
			return true
		}
		// Concrete atom: must match exactly
		return p.Equal(term)

	case *core.List:
		tl, ok := term.(*core.List)
		if !ok || len(p.Elems) != len(tl.Elems) {
			return false
		}
		for i := range p.Elems {
			if !matchSexpr(p.Elems[i], tl.Elems[i], b) {
				return false
			}
		}
		return true
	}

	return false
}

// --- Substitution ---

// Substitute replaces all metavariables in a template with their bindings.
func Substitute(template core.Sexpr, b Bindings) core.Sexpr {
	switch t := template.(type) {
	case *core.Atom:
		if name, ok := core.IsMetaVar(t); ok {
			if bound, found := b[name]; found {
				return core.DeepCopy(bound)
			}
		}
		return t

	case *core.List:
		elems := make([]core.Sexpr, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = Substitute(e, b)
		}
		return &core.List{Elems: elems}
	}
	return template
}

// --- Unresolved metavariable checking ---

func collectMetaVars(s core.Sexpr, acc map[string]bool) {
	switch s := s.(type) {
	case *core.Atom:
		if name, ok := core.IsMetaVar(s); ok {
			acc[name] = true
		}
	case *core.List:
		for _, e := range s.Elems {
			collectMetaVars(e, acc)
		}
	}
}

func ruleMentionsMetaVar(rule *Rule, name string) bool {
	acc := make(map[string]bool)
	collectMetaVars(rule.LHS, acc)
	collectMetaVars(rule.RHS, acc)
	for _, sc := range rule.SideConditions {
		collectMetaVars(sc.LHS, acc)
		collectMetaVars(sc.RHS, acc)
	}
	return acc[name]
}

func unresolvedMetaVarsInResult(result *RewriteResult) []string {
	acc := make(map[string]bool)
	collectMetaVars(result.Rewritten, acc)
	for _, ob := range result.Obligations {
		collectMetaVars(ob.LHS, acc)
		collectMetaVars(ob.RHS, acc)
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

// --- Path-based navigation ---

// Path identifies a position in an s-expression tree.
// Each element indexes into a List's Elems slice.
type Path []int

// RootPath is the path to the root of the expression.
var RootPath = Path{}

// AtPath returns the subexpression at the given path.
func AtPath(s core.Sexpr, path Path) (core.Sexpr, error) {
	cur := s
	for i, step := range path {
		l, ok := cur.(*core.List)
		if !ok {
			return nil, fmt.Errorf("at path step %d: expected list, got atom %v", i, cur)
		}
		if step < 0 || step >= len(l.Elems) {
			return nil, fmt.Errorf("at path step %d: index %d out of range (list has %d elements)", i, step, len(l.Elems))
		}
		cur = l.Elems[step]
	}
	return cur, nil
}

// ReplacePath returns a new s-expression with the subtree at path replaced.
func ReplacePath(s core.Sexpr, path Path, replacement core.Sexpr) (core.Sexpr, error) {
	if len(path) == 0 {
		return replacement, nil
	}
	l, ok := s.(*core.List)
	if !ok {
		return nil, fmt.Errorf("cannot navigate into atom %v", s)
	}
	step := path[0]
	if step < 0 || step >= len(l.Elems) {
		return nil, fmt.Errorf("index %d out of range (list has %d elements)", step, len(l.Elems))
	}

	newChild, err := ReplacePath(l.Elems[step], path[1:], replacement)
	if err != nil {
		return nil, err
	}

	elems := make([]core.Sexpr, len(l.Elems))
	copy(elems, l.Elems)
	elems[step] = newChild
	return &core.List{Elems: elems}, nil
}

// --- Rewrite engine ---

// Rewrite applies a named rule at the given path in the expression.
func Rewrite(term core.Sexpr, rule *Rule, path Path) (*RewriteResult, error) {
	target, err := AtPath(term, path)
	if err != nil {
		return nil, fmt.Errorf("rewrite %s: %w", rule.Name, err)
	}

	bindings := Match(rule.LHS, target)
	if bindings == nil {
		return nil, fmt.Errorf("rewrite %s: LHS pattern does not match term at path %v\n  pattern: %s\n  term:    %s",
			rule.Name, path, core.PrettyPrintSexpr(rule.LHS), core.PrettyPrintSexpr(target))
	}

	rewritten := Substitute(rule.RHS, bindings)
	newTerm, err := ReplacePath(term, path, rewritten)
	if err != nil {
		return nil, fmt.Errorf("rewrite %s: %w", rule.Name, err)
	}

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

// RewriteWithSupplementalBindings applies a rule with additional bindings
// for metavariables not bound by matching (e.g., ?h in foldr-fusion).
func RewriteWithSupplementalBindings(term core.Sexpr, rule *Rule, path Path, extra Bindings) (*RewriteResult, error) {
	target, err := AtPath(term, path)
	if err != nil {
		return nil, err
	}

	bindings := Match(rule.LHS, target)
	if bindings == nil {
		return nil, fmt.Errorf("rewrite %s: LHS pattern does not match at path %v\n  pattern: %s\n  term:    %s",
			rule.Name, path, core.PrettyPrintSexpr(rule.LHS), core.PrettyPrintSexpr(target))
	}

	for k, v := range extra {
		if _, isMeta := core.IsMetaVar(core.Sym(k)); !isMeta {
			return nil, fmt.Errorf("rewrite %s: supplemental binding %q is not a metavariable", rule.Name, k)
		}
		if _, ok := bindings[k]; ok {
			return nil, fmt.Errorf("rewrite %s: supplemental binding for %s would override an LHS match", rule.Name, k)
		}
		if !ruleMentionsMetaVar(rule, k) {
			return nil, fmt.Errorf("rewrite %s: supplemental binding %s is not used by the rule", rule.Name, k)
		}
		bindings[k] = v
	}

	rewritten := Substitute(rule.RHS, bindings)
	newTerm, err := ReplacePath(term, path, rewritten)
	if err != nil {
		return nil, err
	}

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
