package main

// mutate.go — `sb mutate`: measure how strong the behavioral gate
// actually is.
//
// The discharge report says a premise is "runtime-sample" discharged
// and how many samples passed. That number is a measure of effort, not
// of strength: a hundred samples that all take the same branch are
// worth one. Mutation analysis measures strength directly. Break the
// implementation in a small, fixed set of ways; for each break, run
// *only* the committed spec test; count how many breaks the test
// notices. The ratio is the kill rate, and every survivor is a
// concrete, minimal counterexample to the claim that the gate covers
// the implementation — which is exactly the kind of feedback the
// Ralph loop is starved of.
//
// The operator set is deliberately small, for the reason the roadmap
// gives: large operator sets manufacture equivalent mutants, and an
// equivalent mutant is a false survivor that costs an author real time
// to dismiss. Five operators:
//
//	cmp-flip       flip one comparison operator
//	off-by-one     add one to a numeric literal
//	drop-conjunct  replace `A && B` with `A` (same for ||)
//	list-swap      swap head for last, and tail for init
//	zero-return    return the zero value before doing anything
//
// The last one is the cheapest and the most informative: a spec test
// that does not kill `return false` at the top of the function is
// testing nothing at all.
//
// Mutants that do not compile are *not* counted as caught. A compiler
// error is not evidence about the test suite, and folding it into the
// kill rate would let a score be inflated by writing mutants the
// language rejects. They are reported separately as `invalid`.

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultMutationTimeout bounds one mutant's test run. A mutant that
// makes the implementation loop forever is caught by this, and being
// caught by a timeout is still being caught: the gate noticed.
const DefaultMutationTimeout = 60 * time.Second

// Mutation operator names. These are part of a mutant's id, so they
// are stable identifiers, not display strings.
const (
	OpCmpFlip      = "cmp-flip"
	OpOffByOne     = "off-by-one"
	OpDropConjunct = "drop-conjunct"
	OpListSwap     = "list-swap"
	OpZeroReturn   = "zero-return"
)

// Mutant outcomes.
const (
	MutantCaught     = "caught"
	MutantSurvived   = "survived"
	MutantEquivalent = "equivalent"
	MutantInvalid    = "invalid"
)

// Mutant is one generated change to the implementation.
type Mutant struct {
	ID       string `json:"id"`       // operator:file:line:col — stable
	Operator string `json:"operator"` // one of the Op* constants
	File     string `json:"file"`     // project-relative
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Before   string `json:"before"`  // the original source fragment
	After    string `json:"after"`   // what the operator put there
	Outcome  string `json:"outcome"` // caught / survived / equivalent / invalid
	Detail   string `json:"detail,omitempty"`
	Spec     string `json:"spec"` // the (define …) whose test ran
	ImplFunc string `json:"impl_func"`

	// Source is the whole mutated file. It is how the mutant is
	// applied and is never serialised — a report carries the id and
	// the before/after fragments, not a copy of the tree.
	Source []byte `json:"-"`
}

// MutationScore is the additive `evidence.mutation_score` block. It is
// omitempty at every level above it, so a report from a project that
// never ran `sb mutate` is byte-identical to one from before W4.
type MutationScore struct {
	// Score is caught / (caught + survived). Equivalent and invalid
	// mutants are excluded from both halves: the first because no test
	// could kill them, the second because they are not evidence about
	// the test at all.
	Score      float64 `json:"score"`
	Caught     int     `json:"caught"`
	Survived   int     `json:"survived"`
	Equivalent int     `json:"equivalent"`
	Invalid    int     `json:"invalid"`
	Total      int     `json:"total"`
	Timeout    string  `json:"timeout"`
	MeasuredAt string  `json:"measured_at"`

	// Operators is the per-operator breakdown, sorted by operator name.
	Operators []MutationOperatorStat `json:"operators,omitempty"`

	// Specs is the per-spec breakdown, one entry per [[derive.specs]].
	Specs []MutationSpecStat `json:"specs,omitempty"`

	// Survivors lists every mutant the spec test did not kill, with
	// enough location to act on. These are the bits the sampler is not
	// returning.
	Survivors []Mutant `json:"survivors,omitempty"`

	// Gaps records target languages the operator set does not cover,
	// so a score is never read as covering more than it does.
	Gaps []string `json:"gaps,omitempty"`
}

// MutationOperatorStat is one operator's row.
type MutationOperatorStat struct {
	Operator   string `json:"operator"`
	Caught     int    `json:"caught"`
	Survived   int    `json:"survived"`
	Equivalent int    `json:"equivalent"`
	Invalid    int    `json:"invalid"`
}

// MutationSpecStat is one spec's row.
type MutationSpecStat struct {
	Spec       string  `json:"spec"`
	ImplFunc   string  `json:"impl_func"`
	Test       string  `json:"test"`
	Score      float64 `json:"score"`
	Caught     int     `json:"caught"`
	Survived   int     `json:"survived"`
	Equivalent int     `json:"equivalent"`
	Invalid    int     `json:"invalid"`
}

func cmdMutate(args []string) {
	fs := flag.NewFlagSet("mutate", flag.ExitOnError)
	timeoutFlag := fs.String("timeout", "", "per-mutant test timeout (default: [derive.mutation] timeout, else 60s)")
	jsonOut := fs.String("json", "", "also write the full mutant list as JSON to this path")
	verbose := fs.Bool("verbose", false, "print every mutant, not just survivors")
	noReport := fs.Bool("no-report", false, "do not merge evidence.mutation_score into the discharge report")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb mutate — Measure the behavioral gate's kill rate

Usage: sb mutate [flags]

For each [[derive.specs]] entry, applies a fixed operator set to the
implementation function's package and, for every mutant, runs only the
committed spec test. Records caught / survived / equivalent, writes
`+"`evidence.mutation_score`"+` into %s, and prints the survivors.

Operators: %s, %s, %s, %s, %s.

A survivor is a change to the implementation the spec test did not
notice. Each one is either a missing sample — add a boundary or a path
witness that distinguishes it — or an equivalent mutant, which the
author marks in sb.toml:

  [derive.mutation]
  equivalent = [
    "off-by-one:internal/derived/x.go:12:20",  # bound is exclusive either way
  ]

Flags:
`, DischargeReportPath, OpCmpFlip, OpOffByOne, OpDropConjunct, OpListSwap, OpZeroReturn)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb mutate: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.DeriveSpecs) == 0 {
		fmt.Fprintln(os.Stderr, "sb mutate: no [[derive.specs]] entries in sb.toml — there is no behavioral gate to measure")
		return
	}

	timeout := DefaultMutationTimeout
	src := *timeoutFlag
	if src == "" {
		src = cfg.Mutation.Timeout
	}
	if src != "" {
		d, err := time.ParseDuration(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb mutate: bad timeout %q: %v\n", src, err)
			os.Exit(1)
		}
		timeout = d
	}

	equivalent := map[string]bool{}
	for _, id := range cfg.Mutation.Equivalent {
		equivalent[strings.TrimSpace(id)] = true
	}

	score, mutants, err := RunMutation(cfg, timeout, equivalent, *verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb mutate: %v\n", err)
		os.Exit(1)
	}

	printMutationScore(score)

	if *jsonOut != "" {
		data, _ := json.MarshalIndent(struct {
			Score   *MutationScore `json:"mutation_score"`
			Mutants []Mutant       `json:"mutants"`
		}{score, mutants}, "", "  ")
		if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "sb mutate: writing %s: %v\n", *jsonOut, err)
		} else {
			fmt.Fprintf(os.Stderr, "sb mutate: wrote %s\n", *jsonOut)
		}
	}

	if !*noReport {
		if err := mergeMutationScore(score); err != nil {
			fmt.Fprintf(os.Stderr, "sb mutate: warning: recording the score in %s: %v\n", DischargeReportPath, err)
		}
	}

	// `sb mutate` is a measurement, not a gate: a survivor is news,
	// not a build break, because the right response is usually a new
	// sample rather than a revert. Exit non-zero only when nothing
	// could be measured at all.
	if score.Total == 0 {
		fmt.Fprintln(os.Stderr, "sb mutate: the operator set produced no mutants — nothing was measured")
		os.Exit(1)
	}
}

// RunMutation generates and evaluates the mutant set for every Go
// [[derive.specs]] entry. It restores the tree after each mutant, and
// on any error, so an interrupted run does not leave a broken
// implementation behind.
func RunMutation(cfg *Config, timeout time.Duration, equivalent map[string]bool, verbose bool) (*MutationScore, []Mutant, error) {
	score := &MutationScore{
		Timeout:    timeout.String(),
		MeasuredAt: time.Now().UTC().Format(time.RFC3339),
	}
	var all []Mutant
	opStats := map[string]*MutationOperatorStat{}

	for _, spec := range cfg.DeriveSpecs {
		if spec.Lang != "" && spec.Lang != "go" {
			score.Gaps = append(score.Gaps, fmt.Sprintf(
				"spec %q targets %s; the operator set is implemented over go/ast only, so this spec's gate strength is unmeasured",
				spec.Func, spec.Lang))
			continue
		}
		files, err := implFilesFor(spec)
		if err != nil {
			return nil, nil, err
		}
		stat := MutationSpecStat{Spec: spec.Func, ImplFunc: spec.ImplFunc, Test: specTestName(spec)}

		for _, file := range files {
			mutants, err := GenerateMutants(file)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", file, err)
			}
			orig, err := os.ReadFile(file)
			if err != nil {
				return nil, nil, err
			}

			fmt.Fprintf(os.Stderr, "sb mutate: [%s] %s — %d mutants over %s\n",
				spec.Func, spec.ImplFunc, len(mutants), file)

			for i := range mutants {
				m := &mutants[i]
				m.Spec = spec.Func
				m.ImplFunc = spec.ImplFunc

				if equivalent[m.ID] {
					m.Outcome = MutantEquivalent
					m.Detail = "marked equivalent in sb.toml [derive.mutation]"
				} else {
					outcome, detail := evaluateMutant(file, m.Source, spec, timeout)
					m.Outcome, m.Detail = outcome, detail
				}
				// Always put the original back before moving on,
				// whatever happened.
				if err := os.WriteFile(file, orig, 0o644); err != nil {
					return nil, nil, fmt.Errorf("restoring %s: %w", file, err)
				}

				if verbose || m.Outcome == MutantSurvived {
					fmt.Fprintf(os.Stderr, "  %-10s %s  %s → %s\n", m.Outcome, m.ID, m.Before, m.After)
				}

				s, ok := opStats[m.Operator]
				if !ok {
					s = &MutationOperatorStat{Operator: m.Operator}
					opStats[m.Operator] = s
				}
				switch m.Outcome {
				case MutantCaught:
					s.Caught++
					stat.Caught++
					score.Caught++
				case MutantSurvived:
					s.Survived++
					stat.Survived++
					score.Survived++
					score.Survivors = append(score.Survivors, stripSource(*m))
				case MutantEquivalent:
					s.Equivalent++
					stat.Equivalent++
					score.Equivalent++
				case MutantInvalid:
					s.Invalid++
					stat.Invalid++
					score.Invalid++
				}
				all = append(all, stripSource(*m))
			}
		}
		stat.Score = killRate(stat.Caught, stat.Survived)
		score.Specs = append(score.Specs, stat)
	}

	score.Total = score.Caught + score.Survived + score.Equivalent + score.Invalid
	score.Score = killRate(score.Caught, score.Survived)
	for _, s := range opStats {
		score.Operators = append(score.Operators, *s)
	}
	sort.SliceStable(score.Operators, func(i, j int) bool { return score.Operators[i].Operator < score.Operators[j].Operator })
	return score, all, nil
}

// killRate is caught / (caught + survived). With no live mutants at
// all the rate is 1: there was nothing left to miss.
func killRate(caught, survived int) float64 {
	live := caught + survived
	if live == 0 {
		return 1
	}
	return float64(caught) / float64(live)
}

// stripSource drops the rendered mutant body before the mutant goes
// into a report. The body is the whole mutated file; the id, the
// location, and the before/after fragments are what a reader needs.
func stripSource(m Mutant) Mutant {
	m.Source = nil
	return m
}

// evaluateMutant writes the mutant over the implementation file, runs
// only the spec test, and classifies the result. The caller restores
// the original; this function never does, so a failure path cannot
// leave the decision about restoration ambiguous.
func evaluateMutant(file string, source []byte, spec DeriveSpec, timeout time.Duration) (string, string) {
	if err := os.WriteFile(file, source, 0o644); err != nil {
		return MutantInvalid, "writing the mutant: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-count=1",
		"-run", "^"+specTestName(spec)+"$", spec.ImplPkg)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()

	if ctx.Err() == context.DeadlineExceeded {
		// The mutant diverged and the gate noticed by not finishing.
		// That is a kill, and saying so is honest: the test's verdict
		// on this implementation is "not this one".
		return MutantCaught, "test timed out after " + timeout.String()
	}
	if err == nil {
		return MutantSurvived, out
	}
	// A mutant the compiler rejects says nothing about the test, so it
	// is excluded from the score rather than counted as a kill.
	if isGoBuildFailure(out) {
		return MutantInvalid, firstLines(out, 3)
	}
	return MutantCaught, firstLines(out, 4)
}

// isGoBuildFailure distinguishes "the mutated package does not
// compile" from "the test ran and failed". `go test` reports the
// former with a [build failed] / typecheck marker rather than a FAIL
// line for the test itself.
func isGoBuildFailure(out string) bool {
	return strings.Contains(out, "[build failed]") ||
		strings.Contains(out, "[setup failed]") ||
		strings.Contains(out, "typecheck]")
}

// implFilesFor lists the non-test Go files of the spec's
// implementation package, with the file declaring the implementation
// function first. The whole package is in scope, not just that one
// file: the spec test exercises the implementation through whatever
// helpers it calls, and a helper the test cannot distinguish is
// exactly the kind of gap the score exists to surface.
//
// `go list` resolves the import path to a directory, which keeps this
// working for any module layout rather than assuming the import path's
// tail is the directory on disk.
func implFilesFor(spec DeriveSpec) ([]string, error) {
	out, err := runCaptured("", "go", "list", "-f", "{{.Dir}}", spec.ImplPkg)
	if err != nil {
		return nil, fmt.Errorf("go list %s: %v: %s", spec.ImplPkg, err, strings.TrimSpace(string(out)))
	}
	dir := strings.TrimSpace(string(out))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	primary := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if rel, relErr := filepath.Rel(mustCwd(), p); relErr == nil {
			p = rel
		}
		if primary == "" && fileDeclares(p, spec.ImplFunc) {
			primary = p
			continue
		}
		files = append(files, p)
	}
	if primary == "" {
		return nil, fmt.Errorf("no file in %s declares func %s", dir, spec.ImplFunc)
	}
	return append([]string{primary}, files...), nil
}

// fileDeclares reports whether path declares a top-level function with
// the given name.
func fileDeclares(path, name string) bool {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return false
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name != nil && fn.Name.Name == name {
			return true
		}
	}
	return false
}

func mustCwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

// GenerateMutants parses the file and applies every operator at every
// site, one mutation per mutant. The mutants are returned in source
// order, each carrying the full mutated file in Source.
func GenerateMutants(path string) ([]Mutant, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Mutant

	// Each mutation re-parses from the original bytes, so the position
	// information in a mutant's id always refers to the *committed*
	// file. An id that shifted with each applied mutation would not be
	// stable enough to mark equivalent in sb.toml.
	sites, err := collectMutationSites(path, src)
	if err != nil {
		return nil, err
	}
	for _, s := range sites {
		mutated, err := applyMutantRewrite(path, src, s)
		if err != nil {
			return nil, err
		}
		out = append(out, Mutant{
			ID:       fmt.Sprintf("%s:%s:%d:%d", s.operator, filepath.ToSlash(path), s.line, s.col),
			Operator: s.operator,
			File:     filepath.ToSlash(path),
			Line:     s.line,
			Col:      s.col,
			Before:   s.before,
			After:    s.after,
			Source:   mutated,
		})
	}
	return out, nil
}

// mutationSite is one place an operator applies, identified by the
// position of the node it rewrites.
type mutationSite struct {
	operator string
	pos      token.Pos
	line     int
	col      int
	before   string
	after    string
	// kind distinguishes the rewrites that share a node type.
	kind string
}

// cmpFlips is the comparison-operator flip table. Each operator maps
// to exactly one counterpart, which keeps the mutant count linear in
// the number of comparisons instead of quadratic — the small-operator-
// set discipline the roadmap asks for.
var cmpFlips = map[token.Token]token.Token{
	token.EQL: token.NEQ,
	token.NEQ: token.EQL,
	token.LSS: token.LEQ,
	token.LEQ: token.LSS,
	token.GTR: token.GEQ,
	token.GEQ: token.GTR,
}

// collectMutationSites walks the file and records every site an
// operator applies to. Sites are collected from the whole file rather
// than one function, because the spec test exercises the
// implementation through whatever helpers it calls.
func collectMutationSites(path string, src []byte) ([]mutationSite, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var sites []mutationSite
	add := func(pos token.Pos, op, kind, before, after string) {
		p := fset.Position(pos)
		sites = append(sites, mutationSite{
			operator: op, pos: pos, line: p.Line, col: p.Column,
			before: before, after: after, kind: kind,
		})
	}

	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// zero-return: make the function answer before it does
		// anything. One mutant per function.
		if zero := zeroReturnStmt(fn); zero != "" {
			p := fset.Position(fn.Body.Lbrace)
			sites = append(sites, mutationSite{
				operator: OpZeroReturn, pos: fn.Body.Lbrace, line: p.Line, col: p.Column,
				before: "func " + fn.Name.Name + " body", after: zero, kind: "zero-return",
			})
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.BinaryExpr:
				if flip, ok := cmpFlips[e.Op]; ok {
					add(e.OpPos, OpCmpFlip, "cmp", e.Op.String(), flip.String())
				}
				if e.Op == token.LAND || e.Op == token.LOR {
					add(e.OpPos, OpDropConjunct, "drop", e.Op.String()+" <rhs>", "(rhs dropped)")
				}
			case *ast.BasicLit:
				if e.Kind == token.INT || e.Kind == token.FLOAT {
					if after, ok := incrementLiteral(e.Value); ok {
						add(e.ValuePos, OpOffByOne, "lit", e.Value, after)
					}
				}
			case *ast.IndexExpr:
				// head → last: xs[0] becomes xs[len(xs)-1].
				if lit, ok := e.Index.(*ast.BasicLit); ok && lit.Kind == token.INT && lit.Value == "0" {
					if name, ok := exprName(e.X); ok {
						add(e.Lbrack, OpListSwap, "head", name+"[0]", name+"[len("+name+")-1]")
					}
				}
			case *ast.SliceExpr:
				// tail → init: xs[1:] becomes xs[:len(xs)-1].
				if e.High == nil && e.Max == nil {
					if lit, ok := e.Low.(*ast.BasicLit); ok && lit.Kind == token.INT && lit.Value == "1" {
						if name, ok := exprName(e.X); ok {
							add(e.Lbrack, OpListSwap, "tail", name+"[1:]", name+"[:len("+name+")-1]")
						}
					}
				}
			}
			return true
		})
	}
	sort.SliceStable(sites, func(i, j int) bool {
		if sites[i].line != sites[j].line {
			return sites[i].line < sites[j].line
		}
		if sites[i].col != sites[j].col {
			return sites[i].col < sites[j].col
		}
		return sites[i].operator < sites[j].operator
	})
	return sites, nil
}

// exprName renders a simple identifier or selector so the rewritten
// `len(x)` names the same thing. Anything with side effects would be
// evaluated twice by the rewrite, so only side-effect-free forms
// qualify.
func exprName(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.SelectorExpr:
		if inner, ok := exprName(x.X); ok {
			return inner + "." + x.Sel.Name, true
		}
	}
	return "", false
}

// incrementLiteral adds one to an integer or float literal, preserving
// its written form. Hex, binary and exponent forms are skipped rather
// than guessed at.
func incrementLiteral(v string) (string, bool) {
	if strings.ContainsAny(v, "xXbBoOeEpP_") {
		return "", false
	}
	if strings.Contains(v, ".") {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return "", false
		}
		return strconv.FormatFloat(f+1, 'g', -1, 64), true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatInt(n+1, 10), true
}

// zeroReturnStmt renders `return <zero>, …` for the function's result
// list, or "" when the shape is one the operator cannot build a zero
// for (named results, or a type it cannot name).
func zeroReturnStmt(fn *ast.FuncDecl) string {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return ""
	}
	var parts []string
	for _, f := range fn.Type.Results.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		z, ok := zeroValueFor(f.Type)
		if !ok {
			return ""
		}
		for i := 0; i < n; i++ {
			parts = append(parts, z)
		}
	}
	return "return " + strings.Join(parts, ", ")
}

// zeroValueFor names the zero value of a result type as Go source.
// Composite guard types get `T{}` — which is itself the forgery W1's
// witness catches, so a zero-return mutant on a constructor is caught
// by the witness panic as well as by the spec test.
func zeroValueFor(t ast.Expr) (string, bool) {
	switch x := t.(type) {
	case *ast.Ident:
		switch x.Name {
		case "bool":
			return "false", true
		case "string":
			return `""`, true
		case "error", "any":
			return "nil", true
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
			"float32", "float64", "byte", "rune", "complex64", "complex128":
			return "0", true
		}
		return x.Name + "{}", true
	case *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.ChanType,
		*ast.FuncType, *ast.InterfaceType:
		return "nil", true
	case *ast.SelectorExpr:
		if pkg, ok := exprName(x.X); ok {
			if x.Sel.Name == "error" {
				return "nil", true
			}
			return pkg + "." + x.Sel.Name + "{}", true
		}
	case *ast.IndexExpr:
		// A generic instantiation such as Transaction[B].
		if name, ok := exprName(x.X); ok {
			if idx, ok := exprName(x.Index); ok {
				return name + "[" + idx + "]{}", true
			}
		}
	}
	return "", false
}

// applyMutation re-parses the original source and rewrites the one
// node the site names, then formats the result. Re-parsing per mutant
// is the simplest way to guarantee mutants are independent.
func applyMutantRewrite(path string, src []byte, s mutationSite) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	if s.operator == OpZeroReturn {
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Body.Lbrace != s.pos {
				continue
			}
			ret, err := parseReturnStmt(s.after)
			if err != nil {
				return nil, err
			}
			fn.Body.List = append([]ast.Stmt{ret}, fn.Body.List...)
			return formatFile(fset, file)
		}
		return nil, fmt.Errorf("zero-return site at %d:%d no longer resolves", s.line, s.col)
	}

	applied := false
	ast.Inspect(file, func(n ast.Node) bool {
		if applied {
			return false
		}
		switch e := n.(type) {
		case *ast.BinaryExpr:
			if e.OpPos != s.pos {
				return true
			}
			switch s.operator {
			case OpCmpFlip:
				if flip, ok := cmpFlips[e.Op]; ok {
					e.Op = flip
					applied = true
				}
			case OpDropConjunct:
				// Drop the right operand by replacing it with the
				// operator's neutral element: `A && B` → `A && true`
				// and `A || B` → `A || false`, both of which mean A.
				// Doing it this way needs no parent pointer, which is
				// what lets the whole rewrite stay inside go/ast with
				// no new module dependency.
				e.Y = neutralOperand(e.Op)
				applied = true
			}
		case *ast.IndexExpr:
			if e.Lbrack == s.pos && s.operator == OpListSwap && s.kind == "head" {
				e.Index = mustParseExpr("len(" + exprString(e.X) + ")-1")
				applied = true
			}
		case *ast.SliceExpr:
			if e.Lbrack == s.pos && s.operator == OpListSwap && s.kind == "tail" {
				e.High = mustParseExpr("len(" + exprString(e.X) + ")-1")
				e.Low = nil
				applied = true
			}
		case *ast.BasicLit:
			if e.ValuePos == s.pos && s.operator == OpOffByOne {
				e.Value = s.after
				applied = true
			}
		}
		return !applied
	})
	if !applied {
		return nil, fmt.Errorf("site %s at %d:%d no longer resolves", s.operator, s.line, s.col)
	}
	return formatFile(fset, file)
}

// neutralOperand is the identity element of a boolean operator:
// `true` for &&, `false` for ||. Substituting it for the right operand
// is exactly "drop that operand".
func neutralOperand(op token.Token) ast.Expr {
	if op == token.LOR {
		return ast.NewIdent("false")
	}
	return ast.NewIdent("true")
}

func exprString(e ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), e); err != nil {
		return ""
	}
	return buf.String()
}

func mustParseExpr(s string) ast.Expr {
	e, err := parser.ParseExpr(s)
	if err != nil {
		return ast.NewIdent("0")
	}
	return e
}

func parseReturnStmt(s string) (ast.Stmt, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "m.go", "package p\nfunc f() {\n"+s+"\n}\n", 0)
	if err != nil {
		return nil, err
	}
	fn := f.Decls[0].(*ast.FuncDecl)
	return fn.Body.List[0], nil
}

func formatFile(fset *token.FileSet, file *ast.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// printMutationScore renders the gate-facing summary.
func printMutationScore(s *MutationScore) {
	fmt.Fprintf(os.Stderr, "\nsb mutate: kill rate %.1f%% (%d caught / %d live)  —  %d equivalent, %d invalid, %d total\n",
		s.Score*100, s.Caught, s.Caught+s.Survived, s.Equivalent, s.Invalid, s.Total)
	if len(s.Operators) > 0 {
		fmt.Fprintln(os.Stderr, "\n  operator        caught  survived  equivalent  invalid")
		for _, o := range s.Operators {
			fmt.Fprintf(os.Stderr, "  %-14s  %6d  %8d  %10d  %7d\n",
				o.Operator, o.Caught, o.Survived, o.Equivalent, o.Invalid)
		}
	}
	if len(s.Survivors) > 0 {
		fmt.Fprintf(os.Stderr, "\n  %d survivor(s) — each is a change the spec test did not notice:\n", len(s.Survivors))
		for _, m := range s.Survivors {
			fmt.Fprintf(os.Stderr, "    %s\n      %s → %s\n", m.ID, m.Before, m.After)
		}
		fmt.Fprintln(os.Stderr, "\n  Kill a survivor by adding a sample that distinguishes it (a boundary\n"+
			"  value, or a path witness from path_cover), or mark it equivalent in\n"+
			"  sb.toml under [derive.mutation] with a one-line justification.")
	}
	for _, g := range s.Gaps {
		fmt.Fprintf(os.Stderr, "\n  GAP: %s\n", g)
	}
}

// mergeMutationScore writes the score into the discharge report's
// additive `evidence` block. The rest of the report is untouched, and
// schema_version does not move: `evidence` is omitempty, so a project
// that never runs `sb mutate` produces the same bytes it did before.
func mergeMutationScore(s *MutationScore) error {
	r, err := loadDischarge(DischargeReportPath)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("no discharge report at %s — run `sb derive` first so the score has a report to attach to", DischargeReportPath)
	}
	if r.Evidence == nil {
		r.Evidence = &DischargeEvidence{}
	}
	r.Evidence.MutationScore = s
	return writeDischarge(DischargeReportPath, r)
}
