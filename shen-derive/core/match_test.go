package core

import "testing"

// parsePattern is a shorthand that fails the test on parse error.
func parsePattern(t *testing.T, src string) Sexpr {
	t.Helper()
	s, err := ParseSexpr(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return s
}

func TestMatch_Wildcard(t *testing.T) {
	b, ok, err := Match(parsePattern(t, "_"), IntVal(42))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("expected match")
	}
	if len(b) != 0 {
		t.Errorf("expected no bindings, got %v", b)
	}
}

func TestMatch_VariableBinds(t *testing.T) {
	b, ok, err := Match(parsePattern(t, "X"), IntVal(42))
	if err != nil || !ok {
		t.Fatalf("match failed: ok=%v err=%v", ok, err)
	}
	if v, has := b["X"]; !has || v.String() != "42" {
		t.Errorf("expected X=42, got %v", b)
	}
}

func TestMatch_EmptyList(t *testing.T) {
	// [] parses as the symbol `nil`.
	b, ok, err := Match(parsePattern(t, "[]"), ListVal(nil))
	if err != nil || !ok {
		t.Fatalf("match: ok=%v err=%v", ok, err)
	}
	if len(b) != 0 {
		t.Errorf("expected no bindings, got %v", b)
	}

	// Non-empty list should NOT match `[]`.
	_, ok, err = Match(parsePattern(t, "[]"), ListVal{IntVal(1)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("expected mismatch for non-empty list against []")
	}
}

func TestMatch_ConsHeadTail(t *testing.T) {
	// [X | Xs] matches a non-empty list, binds head and tail.
	lst := ListVal{IntVal(1), IntVal(2), IntVal(3)}
	b, ok, err := Match(parsePattern(t, "[X | Xs]"), lst)
	if err != nil || !ok {
		t.Fatalf("match: ok=%v err=%v", ok, err)
	}
	if b["X"].String() != "1" {
		t.Errorf("X: got %v, want 1", b["X"])
	}
	xs, isList := b["Xs"].(ListVal)
	if !isList || len(xs) != 2 || xs[0].String() != "2" || xs[1].String() != "3" {
		t.Errorf("Xs: got %v, want [2 3]", b["Xs"])
	}

	// Empty list should NOT match `[X | Xs]`.
	_, ok, _ = Match(parsePattern(t, "[X | Xs]"), ListVal(nil))
	if ok {
		t.Error("empty list should not match cons pattern")
	}
}

func TestMatch_FixedLengthList(t *testing.T) {
	// [A B] desugars to (cons A (cons B nil)) — exactly a 2-element list.
	pat := parsePattern(t, "[A B]")

	b, ok, _ := Match(pat, ListVal{StringVal("x"), StringVal("y")})
	if !ok {
		t.Fatal("expected match for [x y]")
	}
	if b["A"].String() != "\"x\"" || b["B"].String() != "\"y\"" {
		t.Errorf("bindings: %v", b)
	}

	// 1-element list: should not match.
	_, ok, _ = Match(pat, ListVal{StringVal("x")})
	if ok {
		t.Error("1-element list matched [A B]")
	}

	// 3-element list: should not match (tail pattern `(cons B nil)` expects
	// a 1-element list, so matching against [y z] fails at nil).
	_, ok, _ = Match(pat, ListVal{StringVal("x"), StringVal("y"), StringVal("z")})
	if ok {
		t.Error("3-element list matched [A B]")
	}
}

func TestMatch_NestedPattern(t *testing.T) {
	// [[X Y] | Rest] — head is itself a 2-element list pattern.
	pat := parsePattern(t, "[[X Y] | Rest]")

	inner := ListVal{StringVal("a"), StringVal("b")}
	outer := ListVal{inner, IntVal(99)}
	b, ok, err := Match(pat, outer)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("expected match")
	}
	if b["X"].String() != "\"a\"" || b["Y"].String() != "\"b\"" {
		t.Errorf("X/Y bindings: %v", b)
	}
	rest, isList := b["Rest"].(ListVal)
	if !isList || len(rest) != 1 {
		t.Errorf("Rest: got %v, want 1-element list", b["Rest"])
	}

	// If the head element isn't a 2-element list, the nested match fails.
	bad := ListVal{ListVal{StringVal("a")}, IntVal(99)}
	_, ok, _ = Match(pat, bad)
	if ok {
		t.Error("nested match should have failed: head is 1-element")
	}
}

func TestMatch_BooleanLiteral(t *testing.T) {
	pat := parsePattern(t, "true")
	_, ok, _ := Match(pat, BoolVal(true))
	if !ok {
		t.Error("true should match BoolVal(true)")
	}
	_, ok, _ = Match(pat, BoolVal(false))
	if ok {
		t.Error("true should not match BoolVal(false)")
	}
}

func TestMatch_IntLiteral(t *testing.T) {
	pat := parsePattern(t, "42")
	_, ok, _ := Match(pat, IntVal(42))
	if !ok {
		t.Error("42 should match IntVal(42)")
	}
	_, ok, _ = Match(pat, IntVal(41))
	if ok {
		t.Error("42 should not match IntVal(41)")
	}
}

func TestMatch_TypeMismatch(t *testing.T) {
	// Cons pattern against a non-list: structural miss, no error.
	_, ok, err := Match(parsePattern(t, "[X | Xs]"), IntVal(1))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("cons pattern against int should not match")
	}
}
