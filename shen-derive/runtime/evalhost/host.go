// Package evalhost exposes the shen-derive evaluator as a turnkey
// runtime host for `:runtime-via :eval` (profile B) premises.
//
// A plain `verified` premise is lowered by shengen into an inline
// check baked into the generated constructor. A `:runtime-via :eval`
// premise instead carries the spec predicate's s-expression text into
// the generated code, which calls Check here at construction time. The
// spec IS the runtime check: the same Shen expression the compiler
// would have lowered is evaluated directly by the embedded evaluator,
// with no separate SBCL process and no hand-written checker that could
// drift from the predicate.
//
// See docs/RUNTIME-VIA.md (profile B).
package evalhost

import (
	"context"
	"fmt"
	"sync"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// parsed predicate s-expressions are cached by source text — a generated
// constructor passes the same literal on every call.
var (
	cacheMu sync.RWMutex
	cache   = map[string]core.Sexpr{}
)

func parseCached(src string) (core.Sexpr, error) {
	cacheMu.RLock()
	s, ok := cache[src]
	cacheMu.RUnlock()
	if ok {
		return s, nil
	}
	s, err := core.ParseSexpr(src)
	if err != nil {
		return nil, fmt.Errorf("evalhost: parse predicate %q: %w", src, err)
	}
	cacheMu.Lock()
	cache[src] = s
	cacheMu.Unlock()
	return s, nil
}

// Check evaluates the Shen predicate s-expression src with varNames
// bound positionally to args, returning the boolean result. It is the
// profile-B runtime host: the same spec expression shengen would lower
// statically is instead evaluated here at construction time.
//
// ctx is accepted for signature uniformity with other runtime checkers
// and for future cancellation; pure-predicate evaluation does not
// consult it today.
//
// An error is returned (rather than a false result) when the inputs are
// malformed (arity mismatch, unsupported arg type), the predicate uses
// an evaluator primitive that is not yet supported, or the predicate
// does not evaluate to a boolean. Callers treat any error as a hard
// construction failure — the value must not be built.
func Check(ctx context.Context, src string, varNames []string, args ...any) (bool, error) {
	_ = ctx
	if len(varNames) != len(args) {
		return false, fmt.Errorf("evalhost: predicate %q has %d vars but %d args supplied", src, len(varNames), len(args))
	}
	sexpr, err := parseCached(src)
	if err != nil {
		return false, err
	}
	env := core.EmptyEnv()
	for i, name := range varNames {
		v, err := toValue(args[i])
		if err != nil {
			return false, fmt.Errorf("evalhost: arg %d (%s): %w", i, name, err)
		}
		env = env.Extend(name, v)
	}
	result, err := core.Eval(env, sexpr)
	if err != nil {
		return false, fmt.Errorf("evalhost: eval %q: %w", src, err)
	}
	b, ok := result.(core.BoolVal)
	if !ok {
		return false, fmt.Errorf("evalhost: predicate %q evaluated to %T, want boolean", src, result)
	}
	return bool(b), nil
}

// toValue converts a Go argument into the evaluator's Value domain.
// Supported: bool, string, and the numeric types shengen lowers Shen
// numbers to (int, int64, float64). Shen treats all numbers uniformly;
// the evaluator promotes int↔float as needed during comparison and
// arithmetic. A pre-built core.Value passes through unchanged.
func toValue(a any) (core.Value, error) {
	switch x := a.(type) {
	case bool:
		return core.BoolVal(x), nil
	case string:
		return core.StringVal(x), nil
	case int:
		return core.IntVal(int64(x)), nil
	case int64:
		return core.IntVal(x), nil
	case float64:
		return core.FloatVal(x), nil
	case float32:
		return core.FloatVal(float64(x)), nil
	case core.Value:
		return x, nil
	default:
		return nil, fmt.Errorf("unsupported arg type %T", a)
	}
}
