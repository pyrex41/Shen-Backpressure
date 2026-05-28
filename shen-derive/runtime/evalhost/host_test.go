package evalhost

import (
	"context"
	"strings"
	"testing"
)

func TestCheck_SingleFieldPredicate(t *testing.T) {
	// The payment `amount` premise: (>= X 0).
	cases := []struct {
		x    float64
		want bool
	}{
		{0, true},
		{100, true},
		{0.01, true},
		{-1, false},
		{-0.0001, false},
	}
	for _, c := range cases {
		got, err := Check(context.Background(), "(>= X 0)", []string{"X"}, c.x)
		if err != nil {
			t.Fatalf("(>= %v 0): unexpected error: %v", c.x, err)
		}
		if got != c.want {
			t.Errorf("(>= %v 0) = %v, want %v", c.x, got, c.want)
		}
	}
}

func TestCheck_CrossFieldEquality(t *testing.T) {
	// A relational predicate over two fields: (= A B).
	if ok, err := Check(context.Background(), "(= A B)", []string{"A", "B"}, "alice", "alice"); err != nil || !ok {
		t.Errorf("(= alice alice) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := Check(context.Background(), "(= A B)", []string{"A", "B"}, "alice", "bob"); err != nil || ok {
		t.Errorf("(= alice bob) = %v, %v; want false, nil", ok, err)
	}
}

func TestCheck_IntFloatPromotion(t *testing.T) {
	// Int arg compared against an int literal, and against a float
	// literal, both must work (the evaluator promotes uniformly).
	if ok, err := Check(context.Background(), "(<= X 100)", []string{"X"}, int(100)); err != nil || !ok {
		t.Errorf("(<= 100 100) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := Check(context.Background(), "(< X 100)", []string{"X"}, 100.0); err != nil || ok {
		t.Errorf("(< 100.0 100) = %v, %v; want false, nil", ok, err)
	}
}

func TestCheck_ArityMismatch(t *testing.T) {
	_, err := Check(context.Background(), "(>= X 0)", []string{"X", "Y"}, 1.0)
	if err == nil || !strings.Contains(err.Error(), "vars but") {
		t.Errorf("expected arity-mismatch error, got %v", err)
	}
}

func TestCheck_NonBooleanResult(t *testing.T) {
	// A predicate that evaluates to a number, not a boolean, is a
	// hard error — the caller must not construct the value.
	_, err := Check(context.Background(), "(+ X 1)", []string{"X"}, 1.0)
	if err == nil || !strings.Contains(err.Error(), "want boolean") {
		t.Errorf("expected non-boolean error, got %v", err)
	}
}

func TestCheck_UnsupportedArgType(t *testing.T) {
	_, err := Check(context.Background(), "(= A B)", []string{"A", "B"}, []int{1}, 2)
	if err == nil || !strings.Contains(err.Error(), "unsupported arg type") {
		t.Errorf("expected unsupported-arg-type error, got %v", err)
	}
}

// TestCheck_UnsupportedPrimitiveErrorsCleanly documents the current
// evaluator coverage boundary: predicates using a primitive the
// evaluator doesn't implement (e.g. `length`) fail with a clear error
// rather than silently passing. Profile B is only safe for predicates
// the evaluator can evaluate; richer predicates need profile A/C.
func TestCheck_UnsupportedPrimitiveErrorsCleanly(t *testing.T) {
	_, err := Check(context.Background(), "(> (length X) 0)", []string{"X"}, "abc")
	if err == nil {
		t.Error("expected an error for unsupported `length` primitive, got nil")
	}
}
