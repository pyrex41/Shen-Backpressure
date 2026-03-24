package main

import (
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

// ============================================================================
// Helpers
// ============================================================================

// parseFile_string is a test helper that parses a spec from a string
// rather than a file path.
func parseFile_string(spec string) ([]Datatype, error) {
	content := spec
	var types []Datatype
	for {
		idx := strings.Index(content, "(datatype ")
		if idx == -1 {
			break
		}
		content = content[idx:]
		depth, end := 0, -1
		for i, ch := range content {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
		if end == -1 {
			break
		}
		block := content[:end]
		content = content[end:]
		if dt := parseDatatype(block); dt != nil {
			types = append(types, *dt)
		}
	}
	return types, nil
}
