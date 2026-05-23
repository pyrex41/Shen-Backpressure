package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Parser Tests
// ============================================================================

func TestParseWrapper(t *testing.T) {
	spec := `(datatype account-id
  X : string;
  ==============
  X : account-id;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 1 {
		t.Fatalf("expected 1 datatype, got %d", len(types))
	}
	dt := types[0]
	if dt.Name != "account-id" {
		t.Errorf("expected name account-id, got %s", dt.Name)
	}
	if len(dt.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(dt.Rules))
	}
	r := dt.Rules[0]
	if len(r.Premises) != 1 || r.Premises[0].VarName != "X" || r.Premises[0].TypeName != "string" {
		t.Errorf("unexpected premises: %+v", r.Premises)
	}
	if !r.Conc.IsWrapped || r.Conc.TypeName != "account-id" {
		t.Errorf("unexpected conclusion: %+v", r.Conc)
	}
}

func TestParseConstrained(t *testing.T) {
	spec := `(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := types[0].Rules[0]
	if len(r.Verified) != 1 {
		t.Fatalf("expected 1 verified premise, got %d", len(r.Verified))
	}
	if r.Verified[0].Raw != "(>= X 0)" {
		t.Errorf("unexpected verified raw: %q", r.Verified[0].Raw)
	}
}

func TestParseComposite(t *testing.T) {
	spec := `(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := types[0].Rules[0]
	if r.Conc.IsWrapped {
		t.Error("composite should not be wrapped")
	}
	if len(r.Conc.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(r.Conc.Fields))
	}
	if r.Conc.Fields[0] != "Amount" || r.Conc.Fields[1] != "From" || r.Conc.Fields[2] != "To" {
		t.Errorf("unexpected fields: %v", r.Conc.Fields)
	}
}

func TestParseGuardedWithDifferentBlockName(t *testing.T) {
	spec := `(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	if types[0].Name != "balance-invariant" {
		t.Errorf("expected block name balance-invariant, got %s", types[0].Name)
	}
	r := types[0].Rules[0]
	if r.Conc.TypeName != "balance-checked" {
		t.Errorf("expected conclusion type balance-checked, got %s", r.Conc.TypeName)
	}
}

func TestParseSkipsAssumptionRules(t *testing.T) {
	spec := `(datatype amount
  X : string;
  ==============
  X : amount;

  X : amount >> X : string;
  ==============
  X : amount;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	// The >> rule should be skipped, leaving only the first rule
	if len(types[0].Rules) != 1 {
		t.Fatalf("expected 1 rule (>> skipped), got %d", len(types[0].Rules))
	}
}

func TestParseSideCondition(t *testing.T) {
	spec := `(datatype op-kind
  Op : string;
  if (element? Op [+ - * /])
  ==========================
  Op : op-kind;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := types[0].Rules[0]
	if len(r.Verified) != 1 {
		t.Fatalf("expected 1 verified from if-syntax, got %d", len(r.Verified))
	}
	if r.Verified[0].Raw != "(element? Op [+ - * /])" {
		t.Errorf("unexpected verified: %q", r.Verified[0].Raw)
	}
}

func TestParseMultipleBlocks(t *testing.T) {
	spec := `\* test spec *\
(datatype foo
  X : string;
  ===========
  X : foo;)

(datatype bar
  X : number;
  ===========
  X : bar;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 {
		t.Fatalf("expected 2 datatypes, got %d", len(types))
	}
	if types[0].Name != "foo" || types[1].Name != "bar" {
		t.Errorf("unexpected names: %s, %s", types[0].Name, types[1].Name)
	}
}

// ============================================================================
// Symbol Table Tests
// ============================================================================

func TestSymbolTableClassification(t *testing.T) {
	spec := `(datatype account-id
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

(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)`

	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)

	tests := []struct {
		name     string
		category string
	}{
		{"account-id", "wrapper"},
		{"amount", "constrained"},
		{"transaction", "composite"},
		{"balance-checked", "guarded"},
	}
	for _, tt := range tests {
		info := st.Lookup(tt.name)
		if info == nil {
			t.Errorf("type %s not found in symbol table", tt.name)
			continue
		}
		if info.Category != tt.category {
			t.Errorf("type %s: expected category %s, got %s", tt.name, tt.category, info.Category)
		}
	}
}

func TestSymbolTableFieldOrder(t *testing.T) {
	spec := `(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)`

	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)

	info := st.Lookup("transaction")
	if info == nil {
		t.Fatal("transaction not found")
	}
	if len(info.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(info.Fields))
	}
	expected := []struct{ name, typ string }{
		{"Amount", "amount"},
		{"From", "account-id"},
		{"To", "account-id"},
	}
	for i, exp := range expected {
		if info.Fields[i].ShenName != exp.name || info.Fields[i].ShenType != exp.typ {
			t.Errorf("field %d: expected %s:%s, got %s:%s",
				i, exp.name, exp.typ, info.Fields[i].ShenName, info.Fields[i].ShenType)
		}
	}
}

func TestSymbolTableAlias(t *testing.T) {
	spec := `(datatype unknown-profile
  Id : user-id;
  Email : email-addr;
  ==========================
  [Id Email] : unknown-profile;)

(datatype prompt-required
  Profile : unknown-profile;
  ==========================
  Profile : prompt-required;)`

	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)

	info := st.Lookup("prompt-required")
	if info == nil {
		t.Fatal("prompt-required not found")
	}
	if info.Category != "alias" {
		t.Errorf("expected alias, got %s", info.Category)
	}
	if info.WrappedType != "unknown-profile" {
		t.Errorf("expected wrapped type unknown-profile, got %s", info.WrappedType)
	}
}

// ============================================================================
// S-Expression Parser Tests
// ============================================================================

func TestParseSExprSimple(t *testing.T) {
	expr := parseSExpr("(>= X 10)")
	if !expr.IsCall() {
		t.Fatal("expected call")
	}
	if expr.Op() != ">=" {
		t.Errorf("expected op >=, got %s", expr.Op())
	}
	if len(expr.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(expr.Children))
	}
	if expr.Children[1].Atom != "X" {
		t.Errorf("expected X, got %s", expr.Children[1].Atom)
	}
	if expr.Children[2].Atom != "10" {
		t.Errorf("expected 10, got %s", expr.Children[2].Atom)
	}
}

func TestParseSExprNested(t *testing.T) {
	expr := parseSExpr("(= 0 (shen.mod X 10))")
	if expr.Op() != "=" {
		t.Errorf("expected op =, got %s", expr.Op())
	}
	inner := expr.Children[2]
	if !inner.IsCall() || inner.Op() != "shen.mod" {
		t.Errorf("expected nested shen.mod call, got %v", inner)
	}
}

func TestParseSExprAtom(t *testing.T) {
	expr := parseSExpr("hello")
	if !expr.IsAtom() {
		t.Error("expected atom")
	}
	if expr.Atom != "hello" {
		t.Errorf("expected hello, got %s", expr.Atom)
	}
}

// ============================================================================
// Resolver Tests
// ============================================================================

func buildPaymentSymbolTable() (*SymbolTable, []Datatype) {
	spec := `(datatype account-id
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

(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)`

	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)
	return st, types
}

func TestResolveHeadOnComposite(t *testing.T) {
	st, _ := buildPaymentSymbolTable()
	varMap := map[string]string{"Tx": "transaction"}

	expr := parseSExpr("(head Tx)")
	resolved, ok := st.resolveExpr(expr, varMap)
	if !ok {
		t.Fatal("failed to resolve (head Tx)")
	}
	// head of transaction = first field = Amount (camelCase in generated code)
	if resolved.GoCode != "tx.amount" {
		t.Errorf("expected tx.amount, got %s", resolved.GoCode)
	}
	if resolved.ShenType != "amount" {
		t.Errorf("expected type amount, got %s", resolved.ShenType)
	}
}

func TestResolveTailOnTwoFieldComposite(t *testing.T) {
	spec := `(datatype pair
  A : string;
  B : number;
  ===================
  [A B] : pair;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)

	varMap := map[string]string{"P": "pair"}
	expr := parseSExpr("(tail P)")
	resolved, ok := st.resolveExpr(expr, varMap)
	if !ok {
		t.Fatal("failed to resolve (tail P)")
	}
	// tail of 2-field composite = single remaining field B
	if resolved.GoCode != "p.b" {
		t.Errorf("expected p.b, got %s", resolved.GoCode)
	}
}

func TestResolveHeadTailChain(t *testing.T) {
	st, _ := buildPaymentSymbolTable()
	varMap := map[string]string{"Tx": "transaction"}

	// (head (tail Tx)) = second field = From
	expr := parseSExpr("(head (tail Tx))")
	resolved, ok := st.resolveExpr(expr, varMap)
	if !ok {
		t.Fatal("failed to resolve (head (tail Tx))")
	}
	if resolved.GoCode != "tx.from" {
		t.Errorf("expected tx.from, got %s", resolved.GoCode)
	}
}

func TestResolveBalanceInvariant(t *testing.T) {
	st, _ := buildPaymentSymbolTable()
	varMap := map[string]string{"Bal": "number", "Tx": "transaction"}

	v := VerifiedPremise{Raw: "(>= Bal (head Tx))"}
	goExpr, _ := st.verifiedToGo(v, varMap)
	// Should resolve to: bal >= tx.amount.Val()
	if goExpr != "bal >= tx.amount.Val()" {
		t.Errorf("expected 'bal >= tx.amount.Val()', got %q", goExpr)
	}
}

func TestResolveModulo(t *testing.T) {
	spec := `(datatype decade
  X : number;
  (= 0 (shen.mod X 10)) : verified;
  ===================================
  X : decade;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)

	varMap := map[string]string{"X": "number"}
	v := VerifiedPremise{Raw: "(= 0 (shen.mod X 10))"}
	goExpr, _ := st.verifiedToGo(v, varMap)
	if !strings.Contains(goExpr, "int(x) % 10") {
		t.Errorf("expected modulo expression, got %q", goExpr)
	}
}

func TestResolveLength(t *testing.T) {
	spec := `(datatype us-state
  X : string;
  (= 2 (length X)) : verified;
  =============================
  X : us-state;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)

	varMap := map[string]string{"X": "string"}
	v := VerifiedPremise{Raw: "(= 2 (length X))"}
	goExpr, _ := st.verifiedToGo(v, varMap)
	if !strings.Contains(goExpr, "len(x)") {
		t.Errorf("expected length expression, got %q", goExpr)
	}
}

// ============================================================================
// isNumericLiteral Tests
// ============================================================================

func TestIsNumericLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"0", true},
		{"42", true},
		{"-5", true},
		{"3.14", true},
		{"-0.5", true},
		{"", false},
		{"abc", false},
		{"--5", false},
		{"..", false},
		{"5.3.2", false},
		{"-", false},
		{".", false},
	}
	for _, tt := range tests {
		got := isNumericLiteral(tt.input)
		if got != tt.want {
			t.Errorf("isNumericLiteral(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestGenerateGoPaymentSpec(t *testing.T) {
	st, types := buildPaymentSymbolTable()
	output := generateGo(types, st, "payment", "specs/core.shen")

	// Verify header
	if !strings.Contains(output, "// Code generated by shengen from specs/core.shen. DO NOT EDIT.") {
		t.Error("missing or incorrect header")
	}

	// Verify wrapper type has unexported field
	if !strings.Contains(output, "type AccountId struct{ v string }") {
		t.Error("missing AccountId wrapper")
	}

	// Verify constrained type
	if !strings.Contains(output, "func NewAmount(x float64) (Amount, error)") {
		t.Error("missing Amount constrained constructor")
	}

	// Verify composite has unexported fields
	if strings.Contains(output, "\tAmount Amount") {
		t.Error("composite fields should be unexported (camelCase)")
	}
	if !strings.Contains(output, "\tamount Amount") {
		t.Error("missing unexported amount field in Transaction")
	}

	// Verify accessor methods are generated
	if !strings.Contains(output, "func (t Transaction) Amount() Amount") {
		t.Error("missing Amount() accessor on Transaction")
	}
	if !strings.Contains(output, "func (t Transaction) From() AccountId") {
		t.Error("missing From() accessor on Transaction")
	}

	// Verify guarded constructor uses unexported field access
	if !strings.Contains(output, "tx.amount.Val()") {
		t.Error("guarded constructor should use unexported field access tx.amount.Val()")
	}

	// Verify guarded type has accessor methods
	if !strings.Contains(output, "func (t BalanceChecked) Bal() float64") {
		t.Error("missing Bal() accessor on BalanceChecked")
	}
}

func TestGenerateGoWrapperOnlyNoFmt(t *testing.T) {
	spec := `(datatype name
  X : string;
  ===========
  X : name;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)

	output := generateGo(types, st, "test", "test.shen")

	if strings.Contains(output, `"fmt"`) {
		t.Error("wrapper-only spec should not import fmt")
	}
}

// TestGenerateGoNotEmptyStringHygiene checks the peephole + error-message
// fixes for the common `(not (= X ""))` premise. The lowered predicate is
// `!(x == "")`; the gate site must collapse the outer negation so the
// generated source reads `if (x == "")` instead of `if !(!(x == ""))`.
// The accompanying error message must also drop the "not: X must equal \"\""
// mechanical shape for "X must not be empty".
func TestGenerateGoNotEmptyStringHygiene(t *testing.T) {
	spec := `(datatype secret
  X : string;
  (not (= X "")) : verified;
  ==========================
  X : secret;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)
	output := generateGo(types, st, "test", "test.shen")

	if strings.Contains(output, "if !(!(x == \"\"))") {
		t.Errorf("double-negation peephole did not fire; expected the outer ! to be stripped.\nFull output:\n%s", output)
	}
	if !strings.Contains(output, "if (x == \"\") {") {
		t.Errorf("expected the violation branch to read `if (x == \"\")`.\nFull output:\n%s", output)
	}
	if strings.Contains(output, `"not: x must equal`) {
		t.Errorf("mechanical 'not: ... must equal \"\"' message leaked through.\nFull output:\n%s", output)
	}
	if !strings.Contains(output, `"x must not be empty: %v"`) {
		t.Errorf("expected friendlier 'must not be empty' error message.\nFull output:\n%s", output)
	}
}

// TestGenerateGoNotEqualsHygiene covers the (not (= X Y)) → "X must not
// equal Y" path where neither side is the empty-string literal.
func TestGenerateGoNotEqualsHygiene(t *testing.T) {
	spec := `(datatype distinct-pair
  A : string;
  B : string;
  (not (= A B)) : verified;
  =========================
  [A B] : distinct-pair;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)
	output := generateGo(types, st, "test", "test.shen")

	if strings.Contains(output, "if !(!(a == b))") {
		t.Errorf("double-negation peephole did not fire on (not (= A B)).\nFull output:\n%s", output)
	}
	if !strings.Contains(output, "if (a == b) {") {
		t.Errorf("expected the violation branch to read `if (a == b)`.\nFull output:\n%s", output)
	}
	if strings.Contains(output, `"not: a must equal b"`) {
		t.Errorf("mechanical 'not: a must equal b' message leaked through.\nFull output:\n%s", output)
	}
	if !strings.Contains(output, `"a must not equal b"`) {
		t.Errorf("expected friendlier 'a must not equal b' error message.\nFull output:\n%s", output)
	}
}

// TestGenerateGoPositiveEqUnchanged guards against the peephole over-firing.
// For `(= X "")` (no outer `not`), the lowered predicate is `x == ""` with
// no leading bang; the violation branch must still read `if !(x == "")`.
func TestGenerateGoPositiveEqUnchanged(t *testing.T) {
	spec := `(datatype empty-string
  X : string;
  (= X "") : verified;
  ====================
  X : empty-string;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)
	output := generateGo(types, st, "test", "test.shen")

	if !strings.Contains(output, "if !(x == \"\") {") {
		t.Errorf("positive (= X \"\") should still generate `if !(x == \"\")`.\nFull output:\n%s", output)
	}
}

// TestNegateGoExprPeephole covers stripOuterNotParen / negateGoExpr at the
// helper level, including the case where a leading `!(...)` closes before
// the end of the expression and the outer negation is NOT redundant.
func TestNegateGoExprPeephole(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"!(x == \"\")", "(x == \"\")"},
		{"!(a == b)", "(a == b)"},
		{"x >= 0", "!(x >= 0)"},
		{"foo(x)", "!(foo(x))"},
		{"!(a) && b", "!(!(a) && b)"}, // leading `!(...)` closes before end; do not strip
		{"!((a))", "((a))"},
	}
	for _, c := range cases {
		got := negateGoExpr(c.in)
		if got != c.want {
			t.Errorf("negateGoExpr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ============================================================================
// :runtime-via annotation tests (W3.2)
// ============================================================================

// TestParseRuntimeViaAnnotation verifies that the parser recognises the
// `: verified via <fname>` suffix and stores the checker name on the
// VerifiedPremise. Specs without the annotation must continue to parse
// unchanged (RuntimeVia stays "").
func TestParseRuntimeViaAnnotation(t *testing.T) {
	spec := `(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 1 {
		t.Fatalf("expected 1 datatype, got %d", len(types))
	}
	r := types[0].Rules[0]
	if len(r.Verified) != 1 {
		t.Fatalf("expected 1 verified premise, got %d", len(r.Verified))
	}
	if r.Verified[0].RuntimeVia != "shenEval" {
		t.Errorf("expected RuntimeVia=shenEval, got %q", r.Verified[0].RuntimeVia)
	}
	if r.Verified[0].Raw != "(> (length X) 0)" {
		t.Errorf("expected Raw to retain the predicate; got %q", r.Verified[0].Raw)
	}
}

// TestParseExistingVerifiedUnchanged is the back-compat guard. Specs
// without `via` must still parse as plain verified premises with
// RuntimeVia == "".
func TestParseExistingVerifiedUnchanged(t *testing.T) {
	spec := `(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)`
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := types[0].Rules[0]
	if r.Verified[0].RuntimeVia != "" {
		t.Errorf("expected RuntimeVia empty for plain `: verified`, got %q", r.Verified[0].RuntimeVia)
	}
}

// TestGenerateGoRuntimeViaConstructor checks the constructor emission
// for a constrained type whose verified premise is `:runtime-via
// <fname>`. The generated constructor must (a) take ctx as its first
// parameter, (b) call the named checker with the spec's datatype name +
// argument list, (c) NOT inline the original predicate, and (d) the
// package must declare a compile-time witness `var _ runtimeChecker =
// <fname>` so the build fails if <fname> is absent or wrong-shaped.
func TestGenerateGoRuntimeViaConstructor(t *testing.T) {
	spec := `(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)
	out := generateGo(types, st, "test", "test.shen")

	// (a) ctx parameter
	if !strings.Contains(out, "func NewQueryText(ctx context.Context, x string) (QueryText, error)") {
		t.Errorf("expected ctx-taking constructor signature.\nFull output:\n%s", out)
	}
	// (b) call into checker with predicate name + args
	if !strings.Contains(out, `shenEval(ctx, "query-text", x)`) {
		t.Errorf("expected checker call with predicate name + args.\nFull output:\n%s", out)
	}
	// (c) predicate not inlined
	if strings.Contains(out, "len(x) > 0") || strings.Contains(out, "x.length") {
		t.Errorf("predicate should NOT be inlined when :runtime-via is set.\nFull output:\n%s", out)
	}
	// (d) compile-time witness
	if !strings.Contains(out, "var _ runtimeChecker = shenEval") {
		t.Errorf("expected compile-time witness `var _ runtimeChecker = shenEval`.\nFull output:\n%s", out)
	}
	// runtimeChecker type alias is emitted exactly once.
	if !strings.Contains(out, "type runtimeChecker func(ctx context.Context, predicate string, args ...any) (bool, error)") {
		t.Errorf("expected runtimeChecker type alias.\nFull output:\n%s", out)
	}
	if !strings.Contains(out, `"context"`) {
		t.Errorf("expected context import.\nFull output:\n%s", out)
	}
	if !strings.Contains(out, `"errors"`) {
		t.Errorf("expected errors import.\nFull output:\n%s", out)
	}
}

// TestGenerateGoRuntimeViaGuarded checks the multi-field guarded path:
// runtime-via on a composite type should pass all camelCased field
// names as positional args.
func TestGenerateGoRuntimeViaGuarded(t *testing.T) {
	spec := `(datatype tag-rule
  Token : string;
  UserId : string;
  (= Token UserId) : verified; \* :runtime-via policyCheck *\
  ===========================================
  [Token UserId] : tag-rule;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)
	out := generateGo(types, st, "test", "test.shen")

	if !strings.Contains(out, "func NewTagRule(ctx context.Context, token string, userId string) (TagRule, error)") {
		t.Errorf("expected ctx-taking guarded constructor with all fields.\nFull output:\n%s", out)
	}
	if !strings.Contains(out, `policyCheck(ctx, "tag-rule", token, userId)`) {
		t.Errorf("expected checker call with all field args.\nFull output:\n%s", out)
	}
}

// TestGenerateGoRuntimeViaCompilePass verifies that the emitted code
// compiles when the named runtime checker IS defined in the package,
// and FAILS to compile when it isn't. This is the test the plan
// requires: the synthetic compile-pass must fail when the named
// function is absent.
func TestGenerateGoRuntimeViaCompilePass(t *testing.T) {
	spec := `(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)`
	types, _ := parseFile_string(spec)
	st := newSymbolTable()
	st.Build(types)
	out := generateGo(types, st, "guards", "test.shen")

	// Build a temp package with the generated code + a matching
	// shenEval stub, then run `go build`.
	if err := buildGoPackage(t, out, "guards", `package guards

import "context"

func shenEval(ctx context.Context, predicate string, args ...any) (bool, error) {
	return true, nil
}
`); err != nil {
		t.Fatalf("expected package with shenEval stub to compile; got: %v", err)
	}

	// Same generated code without the shenEval stub must FAIL to
	// compile — the compile-time witness `var _ runtimeChecker =
	// shenEval` is undefined.
	if err := buildGoPackage(t, out, "guards", ""); err == nil {
		t.Fatalf("expected build to FAIL when shenEval is missing; got nil error.\nGenerated code:\n%s", out)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// parseFile_string is a test helper that parses a spec from a string
// rather than a file path.
func parseFile_string(spec string) ([]Datatype, error) {
	var types []Datatype
	for _, block := range extractBlocks(spec, "(datatype ") {
		if dt := parseDatatype(block); dt != nil {
			types = append(types, *dt)
		}
	}
	return types, nil
}

// parseSpec_string parses both datatypes and defines from a string.
func parseSpec_string(spec string) ([]Datatype, []Define, error) {
	var types []Datatype
	for _, block := range extractBlocks(spec, "(datatype ") {
		if dt := parseDatatype(block); dt != nil {
			types = append(types, *dt)
		}
	}
	var defines []Define
	for _, block := range extractBlocks(spec, "(define ") {
		if def := parseDefine(block); def != nil {
			defines = append(defines, *def)
		}
	}
	return types, defines, nil
}

// buildGoPackage writes guardsSource (the shengen output) plus
// optional extraSource into a temp module and runs `go build ./...`.
// Returns the build error if any. Used by TestGenerateGoRuntimeViaCompilePass
// to verify the compile-time witness fires when the named checker is
// absent.
func buildGoPackage(t *testing.T, guardsSource, pkgName, extraSource string) error {
	t.Helper()
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, pkgName)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return err
	}
	gomod := fmt.Sprintf("module guardstest\n\ngo 1.21\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "guards_gen.go"), []byte(guardsSource), 0o644); err != nil {
		return err
	}
	if extraSource != "" {
		if err := os.WriteFile(filepath.Join(pkgDir, "extra.go"), []byte(extraSource), 0o644); err != nil {
			return err
		}
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}
