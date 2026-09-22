package symbolic

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// paymentSpec is the payment example's spec, inlined so the test does
// not depend on the examples tree.
const paymentSpec = `
(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)

(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx)))))
                     (val B0)
                     Txs)))
`

// classifySpec is a spec with a real `if` and a guard clause, so the
// enumerator has genuine control flow to fork on.
const classifySpec = `
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(define tier
  {amount --> number}
  A -> (if (> (val A) 100) 2 (if (> (val A) 10) 1 0)))
`

// contradictorySpec has a datatype whose verified premises cannot both
// hold, so it is uninhabited.
const contradictorySpec = `
(datatype impossible-amount
  X : number;
  (>= X 10) : verified;
  (< X 5) : verified;
  =========================
  X : impossible-amount;)
`

func parseSpec(t *testing.T, src string) *specfile.SpecFile {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "spec-*.shen")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatal(err)
	}
	f.Close()
	sf, err := specfile.ParseFile(f.Name())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return sf
}

func configFor(t *testing.T, src, fn string, solver Solver) *Config {
	t.Helper()
	sf := parseSpec(t, src)
	def := sf.FindDefine(fn)
	if def == nil {
		t.Fatalf("define %q not found", fn)
	}
	all := make([]*specfile.Define, len(sf.Defines))
	for i := range sf.Defines {
		all[i] = &sf.Defines[i]
	}
	return &Config{
		Spec:       def,
		TypeTable:  specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard"),
		AllDefines: all,
		Solver:     solver,
	}
}

// --- fake solver ---

// fakeSolver answers queries without any external binary. It is
// deliberately simple: it understands the fragment the enumerator emits
// for the test specs here (real variables compared against literals and
// against linear combinations) by brute-forcing a small candidate grid.
// Its purpose is to exercise the whole enumerate → check → decode path
// when z3 is not installed.
type fakeSolver struct {
	calls int
}

func (f *fakeSolver) Name() string { return "fake" }

func (f *fakeSolver) Check(q *Query) (*SolverResult, error) {
	f.calls++
	grid := []float64{0, 1, 2, -1, 5, 10, 100, 101, 2.5, -2.5, 50}
	// Try every assignment from the grid over the real vars; strings and
	// bools get a two-value domain. The query sizes in these tests are
	// tiny, so an exhaustive sweep is fine.
	var reals, strs, bools []*Var
	for _, v := range q.Vars {
		switch v.S {
		case SortReal:
			reals = append(reals, v)
		case SortString:
			strs = append(strs, v)
		default:
			bools = append(bools, v)
		}
	}
	if len(reals) > 6 {
		return &SolverResult{Status: StatusUnknown, Model: map[string]string{}}, nil
	}
	idx := make([]int, len(reals))
	for {
		env := map[string]core.Value{}
		for i, v := range reals {
			env[v.Name] = core.FloatVal(grid[idx[i]])
		}
		for _, v := range strs {
			env[v.Name] = core.StringVal("")
		}
		for _, v := range bools {
			env[v.Name] = core.BoolVal(false)
		}
		if ok, err := evalAll(q.Assertions, env); err == nil && ok {
			model := map[string]string{}
			for name, val := range env {
				switch x := val.(type) {
				case core.StringVal:
					model[name] = `"` + string(x) + `"`
				case core.BoolVal:
					if bool(x) {
						model[name] = "true"
					} else {
						model[name] = "false"
					}
				default:
					n, _ := core.AsNum(val)
					model[name] = formatReal(n)
				}
			}
			return &SolverResult{Status: StatusSat, Model: model}, nil
		}
		// odometer
		i := len(idx) - 1
		for i >= 0 {
			idx[i]++
			if idx[i] < len(grid) {
				break
			}
			idx[i] = 0
			i--
		}
		if i < 0 {
			break
		}
	}
	return &SolverResult{Status: StatusUnsat, Model: map[string]string{}}, nil
}

// evalAll interprets constraint terms against a concrete assignment.
func evalAll(ts []Term, env map[string]core.Value) (bool, error) {
	for _, t := range ts {
		v, err := evalTerm(t, env)
		if err != nil {
			return false, err
		}
		b, ok := v.(bool)
		if !ok || !b {
			return false, nil
		}
	}
	return true, nil
}

func evalTerm(t Term, env map[string]core.Value) (any, error) {
	switch x := t.(type) {
	case *Var:
		v, ok := env[x.Name]
		if !ok {
			return nil, errUnknownVar
		}
		switch c := v.(type) {
		case core.StringVal:
			return string(c), nil
		case core.BoolVal:
			return bool(c), nil
		default:
			n, _ := core.AsNum(v)
			return n, nil
		}
	case *Lit:
		switch x.S {
		case SortBool:
			return x.B, nil
		case SortString:
			return x.Str, nil
		default:
			return x.R, nil
		}
	case *App:
		args := make([]any, len(x.Args))
		for i, a := range x.Args {
			v, err := evalTerm(a, env)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		return applyOp(x.Op, args)
	}
	return nil, errUnknownVar
}

func applyOp(op string, args []any) (any, error) {
	num := func(i int) float64 { f, _ := args[i].(float64); return f }
	bl := func(i int) bool { b, _ := args[i].(bool); return b }
	switch op {
	case "+":
		return num(0) + num(1), nil
	case "-":
		if len(args) == 1 {
			return -num(0), nil
		}
		return num(0) - num(1), nil
	case "*":
		return num(0) * num(1), nil
	case "/":
		if num(1) == 0 {
			return nil, errUnknownVar
		}
		return num(0) / num(1), nil
	case "<":
		return num(0) < num(1), nil
	case "<=":
		return num(0) <= num(1), nil
	case ">":
		return num(0) > num(1), nil
	case ">=":
		return num(0) >= num(1), nil
	case "not":
		return !bl(0), nil
	case "and":
		for i := range args {
			if !bl(i) {
				return false, nil
			}
		}
		return true, nil
	case "or":
		for i := range args {
			if bl(i) {
				return true, nil
			}
		}
		return false, nil
	case "=":
		return args[0] == args[1], nil
	case "str.len":
		s, _ := args[0].(string)
		return float64(len(s)), nil
	case "ite":
		if bl(0) {
			return args[1], nil
		}
		return args[2], nil
	}
	return nil, errUnknownVar
}

var errUnknownVar = errFake("unsupported term for the fake solver")

type errFake string

func (e errFake) Error() string { return string(e) }

// --- tests ---

func TestEnumerateWithoutSolverStillEnumerates(t *testing.T) {
	cfg := configFor(t, paymentSpec, "processable", nil)
	res, err := Enumerate(cfg)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if res.Total == 0 {
		t.Fatal("expected paths even without a solver")
	}
	if res.SolverAvailable {
		t.Fatal("SolverAvailable must be false with no solver")
	}
	for _, p := range res.Paths {
		if p.Feasible != FeasUnknown {
			t.Fatalf("path %d: want FeasUnknown, got %s", p.ID, p.Feasible)
		}
		if p.Params != nil {
			t.Fatalf("path %d: no solver must mean no witness", p.ID)
		}
	}
}

func TestEnumeratePaymentWithFakeSolver(t *testing.T) {
	cfg := configFor(t, paymentSpec, "processable", &fakeSolver{})
	cfg.Depth = 2
	res, err := Enumerate(cfg)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	// Shapes 0,1,2 × {true,false} outcomes = 6 paths; the empty-list
	// false outcome is dead because B0 : amount forces (>= B0 0).
	if res.Total != 6 {
		t.Fatalf("paths_total = %d, want 6 (%+v)", res.Total, res.Warnings)
	}
	if res.Dead != 1 {
		t.Fatalf("paths_dead = %d, want 1", res.Dead)
	}
	if res.Feasible != 5 {
		t.Fatalf("paths_feasible = %d, want 5", res.Feasible)
	}
	// Every feasible path must carry a witness whose amount fields
	// respect the constrained type's predicate.
	for _, p := range res.Paths {
		if p.Feasible != FeasFeasible {
			continue
		}
		if len(p.Params) != 2 {
			t.Fatalf("path %d: want 2 params, got %d", p.ID, len(p.Params))
		}
		b0, ok := core.AsNum(p.Params[0])
		if !ok || b0 < 0 {
			t.Fatalf("path %d: B0 = %v violates (>= X 0)", p.ID, p.Params[0])
		}
		txs, ok := p.Params[1].(core.ListVal)
		if !ok {
			t.Fatalf("path %d: Txs is %T, want a list", p.ID, p.Params[1])
		}
		if len(txs) != p.Shape[0] {
			t.Fatalf("path %d: list length %d, want shape %v", p.ID, len(txs), p.Shape)
		}
		for _, tx := range txs {
			fields := tx.(core.ListVal)
			amt, _ := core.AsNum(fields[0])
			if amt < 0 {
				t.Fatalf("path %d: transaction amount %v violates (>= X 0)", p.ID, amt)
			}
		}
	}
}

func TestEnumerateBranchingSpec(t *testing.T) {
	cfg := configFor(t, classifySpec, "tier", &fakeSolver{})
	res, err := Enumerate(cfg)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	// Three tiers → three paths, all reachable.
	if res.Total != 3 {
		t.Fatalf("paths_total = %d, want 3 (warnings %v)", res.Total, res.Warnings)
	}
	if res.Feasible != 3 {
		t.Fatalf("paths_feasible = %d, want 3", res.Feasible)
	}
	seen := map[float64]bool{}
	for _, p := range res.Paths {
		v, _ := core.AsNum(p.Params[0])
		seen[v] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected three distinct witnesses, got %v", seen)
	}
}

func TestVacuityDetectsContradiction(t *testing.T) {
	sf := parseSpec(t, contradictorySpec)
	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard")
	findings, err := CheckVacuity(tt, &fakeSolver{})
	if err != nil {
		t.Fatalf("CheckVacuity: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Verdict != VacuityVacuous {
		t.Fatalf("verdict = %s, want vacuous", f.Verdict)
	}
	if !AnyVacuous(findings) {
		t.Fatal("AnyVacuous must be true")
	}
	if !strings.Contains(f.Message, "uninhabited") {
		t.Fatalf("message should explain the problem, got %q", f.Message)
	}
}

func TestVacuityAcceptsInhabited(t *testing.T) {
	sf := parseSpec(t, paymentSpec)
	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard")
	findings, err := CheckVacuity(tt, &fakeSolver{})
	if err != nil {
		t.Fatalf("CheckVacuity: %v", err)
	}
	if AnyVacuous(findings) {
		t.Fatalf("payment spec must not be vacuous: %+v", findings)
	}
	if len(findings) != 1 || findings[0].Type != "amount" {
		t.Fatalf("want one finding for amount, got %+v", findings)
	}
}

func TestVacuityWithoutSolverIsUnknown(t *testing.T) {
	sf := parseSpec(t, contradictorySpec)
	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard")
	findings, err := CheckVacuity(tt, nil)
	if err != nil {
		t.Fatalf("CheckVacuity: %v", err)
	}
	if len(findings) != 1 || findings[0].Verdict != VacuityUnknown {
		t.Fatalf("want a single unknown finding, got %+v", findings)
	}
	if AnyVacuous(findings) {
		t.Fatal("no solver must never yield a vacuous verdict")
	}
}

func TestRenderScript(t *testing.T) {
	x := NewVar("x", SortReal)
	s := NewVar("s", SortString)
	c, err := Cmp(">=", x, R(0))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := StrLen(s)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Cmp(">", ln, R(1))
	if err != nil {
		t.Fatal(err)
	}
	script := RenderScript(&Query{Vars: []*Var{x, s}, Assertions: []Term{c, c2, B(true)}})
	for _, want := range []string{
		"(declare-const x Real)",
		"(declare-const s String)",
		"(assert (>= x 0.0))",
		"(assert (> (str.len s) 1.0))",
		"(check-sat)",
		"(get-value (x s))",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "(assert true)") {
		t.Fatalf("a `true` assertion should be dropped:\n%s", script)
	}
}

func TestParseSolverOutput(t *testing.T) {
	res, err := parseSolverOutput("sat\n((x 3.0)\n (y (- 2.5))\n (z (/ 1.0 4.0))\n (s \"ab\")\n (b true))\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Status != StatusSat {
		t.Fatalf("status = %s", res.Status)
	}
	vars := []*Var{
		NewVar("x", SortReal), NewVar("y", SortReal), NewVar("z", SortReal),
		NewVar("s", SortString), NewVar("b", SortBool),
	}
	model, err := DecodeModel(vars, res.Model)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if model["x"] != core.IntVal(3) {
		t.Fatalf("x = %v, want IntVal(3)", model["x"])
	}
	if model["y"] != core.FloatVal(-2.5) {
		t.Fatalf("y = %v, want FloatVal(-2.5)", model["y"])
	}
	if model["z"] != core.FloatVal(0.25) {
		t.Fatalf("z = %v, want FloatVal(0.25)", model["z"])
	}
	if model["s"] != core.StringVal("ab") {
		t.Fatalf("s = %v", model["s"])
	}
	if model["b"] != core.BoolVal(true) {
		t.Fatalf("b = %v", model["b"])
	}

	if res, err := parseSolverOutput("unsat\n"); err != nil || res.Status != StatusUnsat {
		t.Fatalf("unsat: %v %v", res, err)
	}
	if res, err := parseSolverOutput("unknown\n"); err != nil || res.Status != StatusUnknown {
		t.Fatalf("unknown: %v %v", res, err)
	}
}

func TestFindSolverDegradesCleanly(t *testing.T) {
	if _, err := exec.LookPath("z3"); err != nil {
		s, err := FindSolver(0)
		if s != nil || err != ErrSolverUnavailable {
			t.Fatalf("want ErrSolverUnavailable, got (%v, %v)", s, err)
		}
		t.Skip("z3 not on PATH")
	}
	s, err := FindSolver(0)
	if err != nil {
		t.Fatalf("FindSolver: %v", err)
	}
	if s.Name() != "z3" {
		t.Fatalf("name = %q", s.Name())
	}
}

// TestEnumeratePaymentWithZ3 is the end-to-end check against the real
// solver. It is skipped when z3 is not installed, which is the same
// degradation path the gate takes.
func TestEnumeratePaymentWithZ3(t *testing.T) {
	solver, err := FindSolver(0)
	if err != nil {
		t.Skip("z3 not on PATH")
	}
	cfg := configFor(t, paymentSpec, "processable", solver)
	res, err := Enumerate(cfg)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if res.SolverName != "z3" || !res.SolverAvailable {
		t.Fatalf("solver not recorded: %+v", res)
	}
	// Depth 4 → shapes 0..4, each split on the boolean outcome.
	if res.Total != 10 {
		t.Fatalf("paths_total = %d, want 10 (warnings %v)", res.Total, res.Warnings)
	}
	if res.Dead != 1 {
		t.Fatalf("paths_dead = %d, want 1 (the empty-list false outcome)", res.Dead)
	}
	if res.Feasible != 9 {
		t.Fatalf("paths_feasible = %d, want 9", res.Feasible)
	}
	// Cross-check every witness against the concrete evaluator: the
	// path's claimed outcome must be what core.Eval actually produces.
	for _, p := range res.Paths {
		if p.Feasible != FeasFeasible || p.Outcome == "" {
			continue
		}
		got := evalProcessable(t, cfg, p.Params)
		want := p.Outcome == "true"
		if got != want {
			t.Fatalf("path %d (outcome %s): witness %v evaluates to %v",
				p.ID, p.Outcome, p.Params, got)
		}
	}
}

func TestVacuityWithZ3(t *testing.T) {
	solver, err := FindSolver(0)
	if err != nil {
		t.Skip("z3 not on PATH")
	}
	sf := parseSpec(t, contradictorySpec)
	tt := specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard")
	findings, err := CheckVacuity(tt, solver)
	if err != nil {
		t.Fatalf("CheckVacuity: %v", err)
	}
	if !AnyVacuous(findings) {
		t.Fatalf("z3 should find the contradiction: %+v", findings)
	}
}

// evalProcessable runs the concrete evaluator on a decoded witness.
func evalProcessable(t *testing.T, cfg *Config, args []core.Value) bool {
	t.Helper()
	env := core.EmptyEnv().
		Extend("val", &core.BuiltinFn{Name: "val", Fn: func(v core.Value) (core.Value, error) { return v, nil }}).
		Extend("amount", &core.BuiltinFn{Name: "amount", Fn: func(v core.Value) (core.Value, error) {
			return v.(core.ListVal)[0], nil
		}})
	cl := cfg.Spec.Clauses[0]
	for i, name := range cfg.Spec.ParamNames {
		env = env.Extend(name, args[i])
	}
	_ = cl
	v, err := core.Eval(env, cfg.Spec.Clauses[0].Body)
	if err != nil {
		t.Fatalf("core.Eval: %v", err)
	}
	b, ok := v.(core.BoolVal)
	if !ok {
		t.Fatalf("core.Eval returned %T", v)
	}
	return bool(b)
}
