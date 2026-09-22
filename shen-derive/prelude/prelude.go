// Package prelude emits the typed Shen prelude that makes gate 4
// (`shen … eval -q -e '(tc +)' -l spec`) a statement about the spec
// that shen-derive actually evaluates.
//
// # Why a prelude exists at all
//
// A `(define …)` in a Shen-Backpressure spec is not plain Shen. Its
// body is evaluated by shen-derive's own evaluator
// (shen-derive/core/eval.go), which supplies three kinds of name that
// Shen's kernel does not:
//
//   - `val`, the wrapper destructor. The evaluator binds it to the
//     identity function, because a wrapper value is represented by its
//     base value at evaluation time (verify/harness.go, buildBaseEnv).
//   - one accessor per field of every composite datatype, bound to
//     "project index i out of the list", with the field's spec name
//     lower-cased.
//   - list combinators — `foldr`, `foldl`, `scanl`, `unfoldr`,
//     `compose` — and `!=`, none of which are in Shen's kernel.
//
// Without a prelude, tc+ on a spec that uses any of them fails with an
// undefined-symbol or type error. That is what had happened to
// examples/payment: `(define processable …)` uses `val`, the `amount`
// accessor and `scanl`, so gate 4 had never passed on that example
// since the define was added. The wrong fix is to loosen the spec or
// to stop running tc+. The right one is to say, in Shen, what these
// names mean — which is what this package emits.
//
// # What the prelude claims, and what it therefore costs
//
// The prelude is a TCB member. tc+ over spec-plus-prelude proves
// "the spec is well-typed **given these intrinsic signatures**", so a
// signature that does not match the evaluator would let a spec pass
// gate 4 while meaning something else to every other gate. Each
// signature below is therefore derived from the evaluator's code, and
// docs/TRUST-MODEL.md carries the same table for an auditor.
//
// # Two files, not one
//
// Shen's `declare` cannot run under `(tc +)` — its type argument is an
// ordinary list expression, which the typechecker rejects — and a
// `define`'s `{…}` signature is only recorded when the typechecker is
// on. So the prelude is emitted as two files with one job each:
//
//	prelude.declares.shen   `declare` forms; loaded BEFORE (tc +)
//	prelude.defines.shen    `define` forms; loaded AFTER (tc +)
//
// and the invocation is
//
//	shen eval -q -l prelude.declares.shen -e '(tc +)' \
//	     -l prelude.defines.shen -l specs/core.shen
//
// The split is not cosmetic: the declares deliberately are *not*
// typechecked (they are axioms) and the defines deliberately are (they
// are ordinary Shen the host verifies for us).
package prelude

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// Prelude is the emitted pair of files plus the notes that explain
// what could not be typed.
type Prelude struct {
	// Declares is the content of prelude.declares.shen.
	Declares string
	// Defines is the content of prelude.defines.shen.
	Defines string
	// Notes records every intrinsic the generator declined to type,
	// with the reason. They appear as Shen comments in Declares and
	// are returned here so a caller can print them.
	Notes []string
}

// intrinsicDef is one list combinator: the Shen text that defines it,
// and the helper definitions it needs.
type intrinsicDef struct {
	name string
	text string
}

// listIntrinsics are the evaluator's combinators that Shen's kernel
// does not have, each given a real Shen definition whose meaning
// matches core.execPrim exactly:
//
//	foldr f e xs   right fold, f applied element-first  (eval.go "foldr")
//	foldl f e xs   left fold, f applied accumulator-first (eval.go "foldl")
//	scanl f e xs   left fold keeping every prefix, INCLUDING e,
//	               so the result is one longer than xs (eval.go "scanl")
//	unfoldr f seed co-recursive build from (boolean * (elem * seed))
//	compose f g x  f (g x)
//	!=             the negation of Shen's =
//
// These are definitions, not axioms: the host typechecks them, so a
// mistake here is caught by gate 4 itself rather than trusted.
//
// One divergence is recorded rather than modelled: the evaluator caps
// `unfoldr` at 10000 iterations as a divergence guard. The Shen
// definition has no cap, which makes it the more permissive of the
// two; a spec that relies on the cap to terminate is a spec bug in
// either engine.
var listIntrinsics = []intrinsicDef{
	{"foldr", `(define foldr
  {(B --> (A --> A)) --> A --> (list B) --> A}
  _ Z [] -> Z
  F Z [X | Xs] -> (F X (foldr F Z Xs)))`},
	{"foldl", `(define foldl
  {(A --> (B --> A)) --> A --> (list B) --> A}
  _ Z [] -> Z
  F Z [X | Xs] -> (foldl F (F Z X) Xs))`},
	{"scanl", `(define scanl
  {(A --> (B --> A)) --> A --> (list B) --> (list A)}
  _ Z [] -> [Z]
  F Z [X | Xs] -> [Z | (scanl F (F Z X) Xs)])`},
	{"compose", `(define compose
  {(B --> C) --> (A --> B) --> A --> C}
  F G X -> (F (G X)))`},
	{"unfoldr", `(define unfoldr
  {(A --> (boolean * (B * A))) --> A --> (list B)}
  F Seed -> (shen-bp.unfoldr-step F (F Seed)))

(define shen-bp.unfoldr-step
  {(A --> (boolean * (B * A))) --> (boolean * (B * A)) --> (list B)}
  F (@p Go _) -> [] where (not Go)
  F (@p _ (@p X Next)) -> [X | (unfoldr F Next)])`},
	{"!=", `(define !=
  {A --> A --> boolean}
  X Y -> (not (= X Y)))`},
}

// Build emits the prelude for one spec file.
//
// Nothing is emitted speculatively: an intrinsic appears only when
// some `(define …)` body in the spec actually mentions it. A prelude
// that declared names the spec never uses would put signatures in the
// TCB for no evidence in return, and would also be a standing
// invitation for the spec to start relying on one silently.
func Build(sf *specfile.SpecFile) *Prelude {
	tt := specfile.BuildTypeTable(sf.Datatypes, "", "")
	used := usedSymbols(sf)

	p := &Prelude{}
	var decls []string

	// --- val -------------------------------------------------------
	//
	// The evaluator's `val` is the identity function, so its honest
	// Shen type is "wrapper type in, base type out" for the wrapper
	// whose values the spec unwraps. Shen has no overloading, so one
	// `declare` can name exactly one such pair.
	//
	// The pair is chosen from the spec's *constrained* wrappers — the
	// single-field datatypes that carry a `verified` premise. That is
	// the only place `val` earns its keep: an unconstrained wrapper
	// (`(datatype user-id X : string; ==== X : user-id;)`) is a
	// transparent alias whose values already typecheck wherever the
	// base does, so the spec never needs to destructure one.
	//
	// With more than one constrained wrapper the choice would be
	// arbitrary, so the generator declares nothing and says so. Gate 4
	// then fails on the first `val` in such a spec, loudly, instead of
	// passing under a signature nobody chose.
	if used["val"] {
		pairs := constrainedWrapperPairs(tt)
		switch len(pairs) {
		case 0:
			p.Notes = append(p.Notes, "`val` is used by a (define …) but the spec has no constrained wrapper datatype to unwrap; no signature declared.")
		case 1:
			decls = append(decls, fmt.Sprintf("\\* val: the evaluator binds it to the identity function; %s is represented by its base %s.\n   See shen-derive/verify/harness.go (buildBaseEnv). *\\\n(declare val [%s --> %s])",
				pairs[0].wrapper, pairs[0].base, pairs[0].wrapper, pairs[0].base))
		default:
			names := make([]string, len(pairs))
			for i, pr := range pairs {
				names[i] = pr.wrapper + " --> " + pr.base
			}
			p.Notes = append(p.Notes, "`val` is used by a (define …) but the spec has "+
				fmt.Sprint(len(pairs))+" constrained wrappers ("+strings.Join(names, ", ")+
				"); Shen has no overloading, so no signature is declared. Give each unwrap its own name in the spec, or keep one constrained wrapper per spec file.")
		}
	}

	// --- field accessors -------------------------------------------
	//
	// Every composite datatype's field becomes an accessor, exactly as
	// buildBaseEnv registers it: the spec's field name, lower-cased,
	// projecting that field's declared type out of the composite.
	//
	// A name that two datatypes give different types is the same
	// overloading problem as `val`: the evaluator resolves it by
	// first-registration order, which is not a signature anyone would
	// want to trust, so it is reported instead of declared. A name two
	// datatypes agree on is declared once.
	for _, acc := range accessors(tt) {
		if !used[acc.name] {
			continue
		}
		if len(acc.types) > 1 {
			var conflicts []string
			for _, t := range acc.types {
				conflicts = append(conflicts, t.owner+" --> "+t.result)
			}
			p.Notes = append(p.Notes, "accessor `"+acc.name+"` has conflicting types across datatypes ("+
				strings.Join(conflicts, ", ")+"); no signature declared.")
			continue
		}
		t := acc.types[0]
		decls = append(decls, fmt.Sprintf("(declare %s [%s --> %s])", acc.name, t.owner, t.result))
	}

	// --- list combinators ------------------------------------------
	var defs []string
	for _, in := range listIntrinsics {
		if used[in.name] {
			defs = append(defs, in.text)
		}
	}

	p.Declares = renderFile(declaresHeader, sf.Path, decls, p.Notes)
	p.Defines = renderFile(definesHeader, sf.Path, defs, nil)
	return p
}

const declaresHeader = `\* ====================================================================
   GENERATED by shen-derive — do not edit.

   Typed declarations for the intrinsics shen-derive's evaluator
   implements for %s. Loaded BEFORE (tc +), because
   Shen's declare cannot run under the typechecker.

   These are AXIOMS, not proofs: tc+ over spec-plus-prelude proves the
   spec is well-typed *given* the signatures below. They are part of
   the trusted computing base — see docs/TRUST-MODEL.md.
   ==================================================================== *\
`

const definesHeader = `\* ====================================================================
   GENERATED by shen-derive — do not edit.

   Shen definitions of the list combinators shen-derive's evaluator
   implements for %s and Shen's kernel does not.
   Loaded AFTER (tc +), so the host typechecks them: unlike the
   declares, these are checked rather than trusted.
   ==================================================================== *\
`

// renderFile writes the header, then the gaps, then the forms.
//
// The header names the spec by its base name rather than the path it
// was generated from: the prelude lands in the project's .sb/, where
// the base name is unambiguous, and an absolute path would make the
// file differ between two checkouts of the same commit for no reason.
func renderFile(header, specPath string, forms, notes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, header, filepath.Base(specPath))
	for _, n := range notes {
		fmt.Fprintf(&b, "\n\\* GAP: %s *\\\n", n)
	}
	for _, f := range forms {
		b.WriteString("\n")
		b.WriteString(f)
		b.WriteString("\n")
	}
	return b.String()
}

type wrapperPair struct{ wrapper, base string }

// constrainedWrapperPairs returns the spec's constrained wrappers in a
// stable order.
func constrainedWrapperPairs(tt *specfile.TypeTable) []wrapperPair {
	var out []wrapperPair
	for _, e := range tt.Entries {
		if e.Category == specfile.CatConstrained && e.ShenPrim != "" {
			out = append(out, wrapperPair{e.ShenName, e.ShenPrim})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].wrapper < out[j].wrapper })
	return out
}

type accessorType struct{ owner, result string }

type accessor struct {
	name  string
	types []accessorType
}

// accessors enumerates the field accessors the evaluator registers,
// grouped by name, in a stable order. Types that agree are collapsed,
// so an accessor two datatypes spell the same way with the same
// meaning is one declaration rather than a conflict.
func accessors(tt *specfile.TypeTable) []accessor {
	byName := map[string]map[string]accessorType{}
	for _, e := range tt.Entries {
		for _, f := range e.Fields {
			if f.ShenType == "" || f.ShenType == "unknown" {
				continue
			}
			name := strings.ToLower(f.ShenName)
			if byName[name] == nil {
				byName[name] = map[string]accessorType{}
			}
			byName[name][e.ShenName+"->"+f.ShenType] = accessorType{e.ShenName, f.ShenType}
		}
	}
	var out []accessor
	for name, set := range byName {
		// Collapse entries that agree on the result type but differ in
		// the owner: they are one polymorphic projection the evaluator
		// applies to any composite, and declaring it at one owner
		// would be a lie about the others. Only a single owner is
		// declarable.
		byResult := map[string][]accessorType{}
		for _, t := range set {
			byResult[t.result] = append(byResult[t.result], t)
		}
		var types []accessorType
		for _, group := range byResult {
			sort.Slice(group, func(i, j int) bool { return group[i].owner < group[j].owner })
			types = append(types, group...)
		}
		sort.Slice(types, func(i, j int) bool {
			if types[i].owner != types[j].owner {
				return types[i].owner < types[j].owner
			}
			return types[i].result < types[j].result
		})
		out = append(out, accessor{name: name, types: types})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// usedSymbols collects every symbol mentioned in the head position of
// an application, or standing alone, anywhere in the spec's define
// bodies and guards. Over-approximating is harmless — a symbol that is
// a bound variable rather than an intrinsic simply never matches an
// intrinsic name — and under-approximating would silently drop a
// declaration the spec needs.
func usedSymbols(sf *specfile.SpecFile) map[string]bool {
	used := map[string]bool{}
	var walk func(core.Sexpr)
	walk = func(s core.Sexpr) {
		switch v := s.(type) {
		case *core.Atom:
			if v.Kind == core.AtomSymbol {
				used[v.Val] = true
			}
		case *core.List:
			for _, e := range v.Elems {
				walk(e)
			}
		}
	}
	for i := range sf.Defines {
		for _, c := range sf.Defines[i].Clauses {
			walk(c.Body)
			if c.Guard != nil {
				walk(c.Guard)
			}
		}
	}
	return used
}
