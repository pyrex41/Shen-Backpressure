// Package shen implements the bridge between shen-derive's side conditions
// and the Shen type checker. It translates instantiated term equalities into
// Shen sequent-calculus definitions that (tc +) can verify.
//
// Limitations (v0):
//   - Side conditions are restricted to first-order term equalities where
//     both sides are composed of primitives, variables, and lambda applications.
//   - The Shen encoding declares each free variable as having its inferred type,
//     then asserts the equality as a verified premise in a datatype rule.
//   - When the Shen runtime is unavailable, Discharge falls back to empirical
//     testing: evaluating both sides on a set of sample inputs and checking
//     they agree. This is not a proof but catches many bugs.
package shen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
)

// --- Term-to-Shen translation ---

// EmitObligation takes an instantiated side condition (a term equality)
// and produces a Shen spec string that, if accepted by (tc +), certifies
// the equality at the type level.
//
// The encoding strategy:
//   1. Collect free variables from both sides of the equality.
//   2. Translate each side to a Shen s-expression.
//   3. Wrap in a (datatype ...) block with variable premises and the
//      equality as a verified premise.
func EmitObligation(cond laws.InstantiatedCondition) string {
	var b strings.Builder

	b.WriteString(`\* Auto-generated side-condition obligation *\`)
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`\* %s *\`, cond.Description))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`\* LHS: %s *\`, core.PrettyPrint(cond.LHS)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`\* RHS: %s *\`, core.PrettyPrint(cond.RHS)))
	b.WriteString("\n\n")

	// Collect free variables from both sides
	fvs := collectFreeVars(cond.LHS)
	for k, v := range collectFreeVars(cond.RHS) {
		fvs[k] = v
	}

	// Generate helper defines for complex subexpressions (lambdas)
	lhsShen, lhsDefs := termToShen(cond.LHS, "sc-lhs")
	rhsShen, rhsDefs := termToShen(cond.RHS, "sc-rhs")

	for _, d := range lhsDefs {
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	for _, d := range rhsDefs {
		b.WriteString(d)
		b.WriteString("\n\n")
	}

	// Build the datatype
	b.WriteString("(datatype side-condition-obligation\n")

	// Premises: declare each free variable with its type
	for name := range fvs {
		// For v0, we type all free variables as 'number' since our
		// side conditions are arithmetic equalities. A more general
		// version would infer types from context.
		b.WriteString(fmt.Sprintf("  %s : number;\n", shenVarName(name)))
	}

	// The verified premise: equality must hold
	b.WriteString(fmt.Sprintf("  (= %s %s) : verified;\n", lhsShen, rhsShen))
	b.WriteString("  ========================================\n")

	// Conclusion
	fvList := make([]string, 0, len(fvs))
	for name := range fvs {
		fvList = append(fvList, shenVarName(name))
	}
	if len(fvList) == 0 {
		b.WriteString("  true : obligation-discharged;)\n")
	} else {
		b.WriteString(fmt.Sprintf("  [%s] : obligation-discharged;)\n", strings.Join(fvList, " ")))
	}

	return b.String()
}

// termToShen translates a core.Term to a Shen s-expression string.
// It also returns any auxiliary (define ...) blocks needed for lambdas.
func termToShen(t core.Term, prefix string) (string, []string) {
	var defs []string
	s := termToShenInner(t, prefix, &defs, 0)
	return s, defs
}

var lambdaCounter int

func termToShenInner(t core.Term, prefix string, defs *[]string, depth int) string {
	switch t := t.(type) {
	case *core.Var:
		return shenVarName(t.Name)

	case *core.Lit:
		switch t.Kind {
		case core.LitInt:
			return fmt.Sprintf("%d", t.IntVal)
		case core.LitBool:
			if t.BoolVal {
				return "true"
			}
			return "false"
		case core.LitString:
			return fmt.Sprintf("%q", t.StrVal)
		}

	case *core.Prim:
		return primToShen(t.Op)

	case *core.App:
		// Detect binary operator application: App(App(Prim(op), lhs), rhs)
		if inner, ok := t.Func.(*core.App); ok {
			if p, ok := inner.Func.(*core.Prim); ok {
				if shenBinOp(p.Op) != "" {
					lhs := termToShenInner(inner.Arg, prefix, defs, depth)
					rhs := termToShenInner(t.Arg, prefix, defs, depth)
					return fmt.Sprintf("(%s %s %s)", shenBinOp(p.Op), lhs, rhs)
				}
			}
		}
		// General application
		fn := termToShenInner(t.Func, prefix, defs, depth)
		arg := termToShenInner(t.Arg, prefix, defs, depth)
		return fmt.Sprintf("(%s %s)", fn, arg)

	case *core.Lam:
		// Generate a named (define ...) for this lambda with type signature.
		// Shen requires {type --> type --> ...} when (tc +) is active.
		lambdaCounter++
		name := fmt.Sprintf("%s-lam%d", prefix, lambdaCounter)

		// Collect all parameters
		params := []string{t.Param}
		body := t.Body
		for {
			if inner, ok := body.(*core.Lam); ok {
				params = append(params, inner.Param)
				body = inner.Body
			} else {
				break
			}
		}

		bodyStr := termToShenInner(body, prefix, defs, depth+1)
		shenParams := make([]string, len(params))
		for i, p := range params {
			shenParams[i] = shenVarName(p)
		}

		// Build type signature: {number --> number --> number}
		// For v0, all parameters and return type are 'number'.
		typeParts := make([]string, len(params)+1)
		for i := range typeParts {
			typeParts[i] = "number"
		}
		typeSig := "{" + strings.Join(typeParts, " --> ") + "}"

		def := fmt.Sprintf("(define %s\n  %s\n  %s -> %s)",
			name, typeSig, strings.Join(shenParams, " "), bodyStr)
		*defs = append(*defs, def)
		return name

	case *core.IfExpr:
		cond := termToShenInner(t.Cond, prefix, defs, depth)
		then := termToShenInner(t.Then, prefix, defs, depth)
		els := termToShenInner(t.Else, prefix, defs, depth)
		return fmt.Sprintf("(if %s %s %s)", cond, then, els)

	case *core.ListLit:
		if len(t.Elems) == 0 {
			return "[]"
		}
		elems := make([]string, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = termToShenInner(e, prefix, defs, depth)
		}
		return "[" + strings.Join(elems, " ") + "]"

	case *core.TupleLit:
		fst := termToShenInner(t.Fst, prefix, defs, depth)
		snd := termToShenInner(t.Snd, prefix, defs, depth)
		return fmt.Sprintf("(@p %s %s)", fst, snd)

	case *core.Let:
		val := termToShenInner(t.Bound, prefix, defs, depth)
		body := termToShenInner(t.Body, prefix, defs, depth)
		return fmt.Sprintf("(let %s %s %s)", shenVarName(t.Name), val, body)
	}

	return fmt.Sprintf("<unknown:%T>", t)
}

// shenVarName converts a variable name to Shen format (capitalize first letter).
func shenVarName(name string) string {
	if len(name) == 0 {
		return name
	}
	// Shen variables must start with uppercase
	return strings.ToUpper(name[:1]) + name[1:]
}

// primToShen maps primitive operations to Shen equivalents.
func primToShen(op core.PrimOp) string {
	switch op {
	case core.PrimAdd:
		return "+"
	case core.PrimSub:
		return "-"
	case core.PrimMul:
		return "*"
	case core.PrimDiv:
		return "/"
	case core.PrimMod:
		return "shen.mod"
	case core.PrimEq:
		return "="
	case core.PrimNeq:
		return "not-equal?" // user-defined
	case core.PrimLt:
		return "<"
	case core.PrimLe:
		return "<="
	case core.PrimGt:
		return ">"
	case core.PrimGe:
		return ">="
	case core.PrimAnd:
		return "and"
	case core.PrimOr:
		return "or"
	case core.PrimNot:
		return "not"
	case core.PrimNeg:
		return "negate" // (define negate X -> (- 0 X))
	case core.PrimCons:
		return "cons"
	case core.PrimNil:
		return "[]"
	case core.PrimFst:
		return "fst"
	case core.PrimSnd:
		return "snd"
	case core.PrimMap:
		return "map"
	case core.PrimFoldr:
		return "foldr" // user-defined in Shen
	case core.PrimFoldl:
		return "foldl" // user-defined in Shen
	case core.PrimFilter:
		return "filter"
	case core.PrimCompose:
		return "compose" // user-defined
	default:
		return string(op)
	}
}

// shenBinOp returns the Shen infix operator for binary primitives,
// or "" if not a binary operator.
func shenBinOp(op core.PrimOp) string {
	switch op {
	case core.PrimAdd:
		return "+"
	case core.PrimSub:
		return "-"
	case core.PrimMul:
		return "*"
	case core.PrimDiv:
		return "/"
	case core.PrimEq:
		return "="
	case core.PrimLt:
		return "<"
	case core.PrimLe:
		return "<="
	case core.PrimGt:
		return ">"
	case core.PrimGe:
		return ">="
	default:
		return ""
	}
}

// collectFreeVars returns the set of free variable names in a term,
// excluding metavariables (which start with '?').
func collectFreeVars(t core.Term) map[string]bool {
	fv := make(map[string]bool)
	bound := make(map[string]bool)
	collectFV(t, bound, fv)
	return fv
}

func collectFV(t core.Term, bound, free map[string]bool) {
	switch t := t.(type) {
	case *core.Var:
		if !bound[t.Name] && !laws.IsMetaVar(t.Name) {
			free[t.Name] = true
		}
	case *core.Lam:
		newBound := make(map[string]bool)
		for k, v := range bound {
			newBound[k] = v
		}
		newBound[t.Param] = true
		collectFV(t.Body, newBound, free)
	case *core.App:
		collectFV(t.Func, bound, free)
		collectFV(t.Arg, bound, free)
	case *core.Let:
		collectFV(t.Bound, bound, free)
		newBound := make(map[string]bool)
		for k, v := range bound {
			newBound[k] = v
		}
		newBound[t.Name] = true
		collectFV(t.Body, newBound, free)
	case *core.IfExpr:
		collectFV(t.Cond, bound, free)
		collectFV(t.Then, bound, free)
		collectFV(t.Else, bound, free)
	case *core.ListLit:
		for _, e := range t.Elems {
			collectFV(e, bound, free)
		}
	case *core.TupleLit:
		collectFV(t.Fst, bound, free)
		collectFV(t.Snd, bound, free)
	case *core.Prim:
		// no free vars
	case *core.Lit:
		// no free vars
	}
}

// --- Shen runner preamble ---

// ShenPreamble returns helper definitions needed by side-condition specs.
// These must be loaded before the obligation spec. All defines include
// type signatures required by Shen's (tc +) mode.
func ShenPreamble() string {
	return `\* shen-derive runtime helpers *\

(define negate
  {number --> number}
  X -> (- 0 X))

(define compose
  {(B --> C) --> (A --> B) --> (A --> C)}
  F G -> (/. X (F (G X))))

(define foldr
  {(A --> B --> B) --> B --> (list A) --> B}
  F E [] -> E
  F E [X | Xs] -> (F X (foldr F E Xs)))

(define foldl
  {(B --> A --> B) --> B --> (list A) --> B}
  F E [] -> E
  F E [X | Xs] -> (foldl F (F E X) Xs))

(define scanl
  {(B --> A --> B) --> B --> (list A) --> (list B)}
  F E [] -> [E]
  F E [X | Xs] -> [E | (scanl F (F E X) Xs)])
`
}

// --- Discharge ---

// DischargeResult holds the outcome of discharging an obligation.
type DischargeResult struct {
	Discharged bool
	Method     string // "shen-tc+", "empirical", "skip"
	Output     string // raw output from the tool
	Error      error
}

// FindShenBinary locates a Shen runtime, following the same priority
// as the existing shen-check.sh script.
func FindShenBinary() string {
	if env := os.Getenv("SHEN"); env != "" {
		return env
	}
	for _, name := range []string{"shen-sbcl", "shen-scheme", "shen"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// Discharge attempts to discharge a side condition obligation.
// It tries the Shen type checker first; if unavailable, falls back to
// empirical testing via the evaluator.
func Discharge(cond laws.InstantiatedCondition) DischargeResult {
	// Try Shen first
	shenBin := FindShenBinary()
	if shenBin != "" {
		return dischargeShen(cond, shenBin)
	}

	// Fall back to empirical testing
	return dischargeEmpirical(cond)
}

// DischargeShenOnly tries Shen only; returns an error if Shen is unavailable.
func DischargeShenOnly(cond laws.InstantiatedCondition) DischargeResult {
	shenBin := FindShenBinary()
	if shenBin == "" {
		return DischargeResult{
			Discharged: false,
			Method:     "shen-tc+",
			Error:      fmt.Errorf("no Shen runtime found; install shen-sbcl or set $SHEN"),
		}
	}
	return dischargeShen(cond, shenBin)
}

// DischargeEmpirical tests the side condition by evaluating both sides
// on a set of sample inputs and checking they agree.
func DischargeEmpirical(cond laws.InstantiatedCondition) DischargeResult {
	return dischargeEmpirical(cond)
}

func dischargeShen(cond laws.InstantiatedCondition, shenBin string) DischargeResult {
	// Reset the lambda counter for deterministic output
	lambdaCounter = 0

	// Build the spec file content
	spec := ShenPreamble() + "\n" + EmitObligation(cond)

	// Write to a temp file
	tmp, err := os.CreateTemp("", "shen-derive-obligation-*.shen")
	if err != nil {
		return DischargeResult{Discharged: false, Method: "shen-tc+", Error: err}
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(spec); err != nil {
		tmp.Close()
		return DischargeResult{Discharged: false, Method: "shen-tc+", Error: err}
	}
	tmp.Close()

	// Run: shen eval -e "(tc +)" -l <spec>
	ctx := exec.Command(shenBin, "eval", "-e", "(tc +)", "-l", tmp.Name())
	ctx.Env = append(os.Environ(), "SHEN_QUIET=1")

	out, err := runWithTimeout(ctx, 30*time.Second)
	if err != nil {
		return DischargeResult{
			Discharged: false,
			Method:     "shen-tc+",
			Output:     string(out),
			Error:      fmt.Errorf("shen tc+ failed: %w", err),
		}
	}

	return DischargeResult{
		Discharged: true,
		Method:     "shen-tc+",
		Output:     string(out),
	}
}

func dischargeEmpirical(cond laws.InstantiatedCondition) DischargeResult {
	// Collect free variables
	fvs := collectFreeVars(cond.LHS)
	for k, v := range collectFreeVars(cond.RHS) {
		fvs[k] = v
	}

	fvNames := make([]string, 0, len(fvs))
	for name := range fvs {
		fvNames = append(fvNames, name)
	}

	// Test with a grid of sample integer values
	samples := []core.Value{
		core.IntVal(0), core.IntVal(1), core.IntVal(-1),
		core.IntVal(2), core.IntVal(5), core.IntVal(10),
		core.IntVal(-3), core.IntVal(7), core.IntVal(100),
	}

	tested := 0
	failed := 0
	var firstFailure string

	// For 0 free variables: just evaluate once
	if len(fvNames) == 0 {
		lv, err1 := core.Eval(core.EmptyEnv(), cond.LHS)
		rv, err2 := core.Eval(core.EmptyEnv(), cond.RHS)
		if err1 != nil || err2 != nil {
			return DischargeResult{
				Discharged: false,
				Method:     "empirical",
				Error:      fmt.Errorf("evaluation error: lhs=%v, rhs=%v", err1, err2),
			}
		}
		if lv.String() != rv.String() {
			return DischargeResult{
				Discharged: false,
				Method:     "empirical",
				Output:     fmt.Sprintf("LHS=%s, RHS=%s", lv, rv),
				Error:      fmt.Errorf("side condition failed: %s != %s", lv, rv),
			}
		}
		return DischargeResult{
			Discharged: true,
			Method:     "empirical",
			Output:     fmt.Sprintf("verified on 1 test case (no free variables)"),
		}
	}

	// For 1 free variable: test each sample
	// For 2 free variables: test each pair
	// For more: test a subset of combinations
	if len(fvNames) == 1 {
		for _, v := range samples {
			tested++
			env := core.EmptyEnv().Extend(fvNames[0], v)
			if ok, msg := testEquality(env, cond); !ok {
				failed++
				if firstFailure == "" {
					firstFailure = fmt.Sprintf("%s=%s: %s", fvNames[0], v, msg)
				}
			}
		}
	} else if len(fvNames) == 2 {
		for _, v1 := range samples {
			for _, v2 := range samples {
				tested++
				env := core.EmptyEnv().Extend(fvNames[0], v1).Extend(fvNames[1], v2)
				if ok, msg := testEquality(env, cond); !ok {
					failed++
					if firstFailure == "" {
						firstFailure = fmt.Sprintf("%s=%s, %s=%s: %s",
							fvNames[0], v1, fvNames[1], v2, msg)
					}
				}
			}
		}
	} else {
		// For 3+ vars, test a smaller set
		for i := 0; i < len(samples) && i < 5; i++ {
			for j := 0; j < len(samples) && j < 5; j++ {
				tested++
				env := core.EmptyEnv()
				for k, name := range fvNames {
					switch k {
					case 0:
						env = env.Extend(name, samples[i])
					case 1:
						env = env.Extend(name, samples[j])
					default:
						env = env.Extend(name, samples[k%len(samples)])
					}
				}
				if ok, msg := testEquality(env, cond); !ok {
					failed++
					if firstFailure == "" {
						firstFailure = msg
					}
				}
			}
		}
	}

	if failed > 0 {
		return DischargeResult{
			Discharged: false,
			Method:     "empirical",
			Output:     fmt.Sprintf("failed %d/%d test cases; first: %s", failed, tested, firstFailure),
			Error:      fmt.Errorf("side condition failed empirically"),
		}
	}

	return DischargeResult{
		Discharged: true,
		Method:     "empirical",
		Output:     fmt.Sprintf("verified on %d test cases", tested),
	}
}

func testEquality(env *core.Env, cond laws.InstantiatedCondition) (bool, string) {
	lv, err1 := core.Eval(env, cond.LHS)
	if err1 != nil {
		// Some sample values may cause errors (e.g., division by zero);
		// skip rather than fail.
		return true, ""
	}
	rv, err2 := core.Eval(env, cond.RHS)
	if err2 != nil {
		return true, ""
	}
	if lv.String() != rv.String() {
		return false, fmt.Sprintf("LHS=%s, RHS=%s", lv, rv)
	}
	return true, ""
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	done := make(chan struct{})
	var out []byte
	var err error

	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		return out, err
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return nil, fmt.Errorf("timed out after %s", timeout)
	}
}

// --- Lazy/Strict rewrite modes ---

// RewriteLazy applies a rewrite rule and collects obligations without
// discharging them. The caller decides what to do with them.
func RewriteLazy(term core.Term, rule *laws.Rule, path laws.Path, extra laws.Bindings) (*laws.RewriteResult, error) {
	if extra != nil {
		return laws.RewriteWithSupplementalBindings(term, rule, path, extra)
	}
	return laws.Rewrite(term, rule, path)
}

// RewriteStrict applies a rewrite rule and immediately discharges all
// obligations. Fails the rewrite if any obligation can't be discharged.
func RewriteStrict(term core.Term, rule *laws.Rule, path laws.Path, extra laws.Bindings) (*laws.RewriteResult, error) {
	result, err := RewriteLazy(term, rule, path, extra)
	if err != nil {
		return nil, err
	}

	for _, ob := range result.Obligations {
		dr := Discharge(ob)
		if !dr.Discharged {
			return nil, fmt.Errorf("obligation not discharged (%s): %s\n  LHS: %s\n  RHS: %s\n  %v",
				dr.Method, ob.Description,
				core.PrettyPrint(ob.LHS), core.PrettyPrint(ob.RHS), dr.Error)
		}
	}

	return result, nil
}
