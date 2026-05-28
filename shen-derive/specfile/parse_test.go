package specfile

import (
	"os"
	"path/filepath"
	"testing"
)

const paymentSpecPath = "../../examples/payment/specs/core.shen"

func TestParsePaymentSpec(t *testing.T) {
	sf, err := ParseFile(filepath.Clean(paymentSpecPath))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if got, want := len(sf.Datatypes), 6; got != want {
		t.Fatalf("datatypes: got %d, want %d", got, want)
	}

	wantNames := []string{"account-id", "amount", "transaction", "balance-invariant", "account-state", "safe-transfer"}
	for i, name := range wantNames {
		if sf.Datatypes[i].Name != name {
			t.Errorf("datatype[%d]: got %q, want %q", i, sf.Datatypes[i].Name, name)
		}
	}

	// Spot-check amount: single wrapped premise + one verified condition.
	amount := sf.FindDatatype("amount")
	if amount == nil || len(amount.Rules) != 1 {
		t.Fatalf("amount: expected 1 rule")
	}
	r := amount.Rules[0]
	if len(r.Premises) != 1 || r.Premises[0].TypeName != "number" {
		t.Errorf("amount premises: %+v", r.Premises)
	}
	if len(r.Verified) != 1 || r.Verified[0].Raw != "(>= X 0)" {
		t.Errorf("amount verified: %+v", r.Verified)
	}
	if !r.Conclusion.IsWrapped || r.Conclusion.TypeName != "amount" {
		t.Errorf("amount conclusion: %+v", r.Conclusion)
	}

	// Spot-check transaction: composite with three fields.
	tx := sf.FindDatatype("transaction")
	if tx == nil || len(tx.Rules) != 1 {
		t.Fatalf("transaction: expected 1 rule")
	}
	txr := tx.Rules[0]
	if txr.Conclusion.IsWrapped {
		t.Errorf("transaction should be composite, not wrapped")
	}
	if got := txr.Conclusion.Fields; len(got) != 3 || got[0] != "Amount" || got[1] != "From" || got[2] != "To" {
		t.Errorf("transaction fields: %v", got)
	}

	// The payment spec now includes a (define processable ...) block.
	if len(sf.Defines) != 1 {
		t.Fatalf("defines: got %d, want 1", len(sf.Defines))
	}
	proc := sf.FindDefine("processable")
	if proc == nil {
		t.Fatal("processable not found")
	}
	if got := proc.TypeSig.ParamTypes; len(got) != 2 || got[0] != "amount" || got[1] != "(list transaction)" {
		t.Errorf("processable param types: %v", got)
	}
	if proc.TypeSig.ReturnType != "boolean" {
		t.Errorf("processable return type: %q", proc.TypeSig.ReturnType)
	}
	if got := proc.ParamNames; len(got) != 2 || got[0] != "B0" || got[1] != "Txs" {
		t.Errorf("processable param names: %v", got)
	}
}

func TestParseTypeSig(t *testing.T) {
	cases := []struct {
		in         string
		wantParams []string
		wantRet    string
	}{
		{"{number --> number}", []string{"number"}, "number"},
		{"{amount --> (list transaction) --> boolean}", []string{"amount", "(list transaction)"}, "boolean"},
		{"{string --> string --> number --> boolean}", []string{"string", "string", "number"}, "boolean"},
	}
	for _, tc := range cases {
		sig, err := parseTypeSig(tc.in)
		if err != nil {
			t.Errorf("parseTypeSig(%q): %v", tc.in, err)
			continue
		}
		if len(sig.ParamTypes) != len(tc.wantParams) {
			t.Errorf("parseTypeSig(%q): params=%v, want %v", tc.in, sig.ParamTypes, tc.wantParams)
			continue
		}
		for i, p := range sig.ParamTypes {
			if p != tc.wantParams[i] {
				t.Errorf("parseTypeSig(%q): params[%d]=%q, want %q", tc.in, i, p, tc.wantParams[i])
			}
		}
		if sig.ReturnType != tc.wantRet {
			t.Errorf("parseTypeSig(%q): ret=%q, want %q", tc.in, sig.ReturnType, tc.wantRet)
		}
	}
}

func TestParseDefine(t *testing.T) {
	block := `(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx))))) (val B0) Txs)))`

	def, err := parseDefine(block)
	if err != nil {
		t.Fatalf("parseDefine: %v", err)
	}
	if def.Name != "processable" {
		t.Errorf("name: %q", def.Name)
	}
	if got := def.TypeSig.ParamTypes; len(got) != 2 || got[0] != "amount" || got[1] != "(list transaction)" {
		t.Errorf("param types: %v", got)
	}
	if def.TypeSig.ReturnType != "boolean" {
		t.Errorf("return type: %q", def.TypeSig.ReturnType)
	}
	if got := def.ParamNames; len(got) != 2 || got[0] != "B0" || got[1] != "Txs" {
		t.Errorf("param names: %v", got)
	}
	if len(def.Clauses) != 1 {
		t.Fatalf("clauses: got %d, want 1", len(def.Clauses))
	}
	if def.Clauses[0].Body == nil {
		t.Fatal("clause body is nil")
	}
	if def.Clauses[0].Guard != nil {
		t.Error("unexpected guard on single-clause define")
	}
	if got := len(def.Clauses[0].Patterns); got != 2 {
		t.Errorf("clause patterns: got %d, want 2", got)
	}
}

func TestParseDefineMultiClauseWithGuards(t *testing.T) {
	block := `(define pair-in-list?
  _ _ [] -> false
  A B [[X Y] | Rest] -> true  where (and (= A X) (= B Y))
  A B [[X Y] | Rest] -> true  where (and (= A Y) (= B X))
  A B [_ | Rest] -> (pair-in-list? A B Rest))`

	def, err := parseDefine(block)
	if err != nil {
		t.Fatalf("parseDefine: %v", err)
	}
	if def.Name != "pair-in-list?" {
		t.Errorf("name: %q", def.Name)
	}
	if got := len(def.Clauses); got != 4 {
		t.Fatalf("clauses: got %d, want 4", got)
	}
	// Clause 0: `_ _ [] -> false`, no guard.
	if len(def.Clauses[0].Patterns) != 3 {
		t.Errorf("clause 0: %d patterns", len(def.Clauses[0].Patterns))
	}
	if def.Clauses[0].Guard != nil {
		t.Error("clause 0: unexpected guard")
	}
	// Clauses 1 and 2 both have where guards.
	if def.Clauses[1].Guard == nil {
		t.Error("clause 1: missing guard")
	}
	if def.Clauses[2].Guard == nil {
		t.Error("clause 2: missing guard")
	}
	// Clause 3 has a recursive call as its body, no guard.
	if def.Clauses[3].Guard != nil {
		t.Error("clause 3: unexpected guard")
	}
	// The define has no type signature — TypeSig fields should be empty.
	if len(def.TypeSig.ParamTypes) != 0 {
		t.Errorf("expected no type sig, got %v", def.TypeSig.ParamTypes)
	}
	// Arity should come from first clause pattern count.
	if def.Arity() != 3 {
		t.Errorf("arity: got %d, want 3", def.Arity())
	}
}

func TestStripShenComments(t *testing.T) {
	in := `\* line comment *\
(datatype foo
  X : number;
  ============
  X : foo;)
\* another *\`
	out := stripShenComments(in)
	if containsAny(out, `\*`, `*\`) {
		t.Errorf("comments not stripped: %q", out)
	}
	if !containsAll(out, "datatype foo", "X : number") {
		t.Errorf("body lost: %q", out)
	}
}

func TestParseDefineRecursive(t *testing.T) {
	block := `(define fac
  {number --> number}
  0 -> 1
  N -> (* N (fac (- N 1))))`

	def, err := parseDefine(block)
	if err != nil {
		t.Fatalf("parseDefine: %v", err)
	}
	if def.Name != "fac" {
		t.Errorf("name: %q", def.Name)
	}
	if len(def.Clauses) != 2 {
		t.Fatalf("clauses: got %d, want 2", len(def.Clauses))
	}
	// Clause 0: literal 0 -> 1
	if def.Clauses[0].Guard != nil {
		t.Error("clause 0: unexpected guard")
	}
	// Clause 1: N -> (* N (fac (- N 1))) — recursive call in body
	if def.Clauses[1].Body == nil {
		t.Fatal("clause 1: body is nil")
	}
	if def.Clauses[1].Guard != nil {
		t.Error("clause 1: unexpected guard")
	}
}

func TestParseDefineUntypedMultiClause(t *testing.T) {
	// No type signature, three clauses with mixed guards
	block := `(define drug-clear-of-list?
  _ [] _ -> true
  Drug [Med | Meds] Pairs -> false where (pair-in-list? Drug Med Pairs)
  Drug [_ | Meds] Pairs -> (drug-clear-of-list? Drug Meds Pairs))`

	def, err := parseDefine(block)
	if err != nil {
		t.Fatalf("parseDefine: %v", err)
	}
	if def.Name != "drug-clear-of-list?" {
		t.Errorf("name: %q", def.Name)
	}
	if len(def.TypeSig.ParamTypes) != 0 {
		t.Errorf("expected no type sig, got %v", def.TypeSig.ParamTypes)
	}
	if len(def.Clauses) != 3 {
		t.Fatalf("clauses: got %d, want 3", len(def.Clauses))
	}
	if def.Arity() != 3 {
		t.Errorf("arity: got %d, want 3", def.Arity())
	}
	// Clause 0: no guard
	if def.Clauses[0].Guard != nil {
		t.Error("clause 0: unexpected guard")
	}
	// Clause 1: has where guard
	if def.Clauses[1].Guard == nil {
		t.Error("clause 1: missing guard")
	}
	// Clause 2: no guard, recursive
	if def.Clauses[2].Guard != nil {
		t.Error("clause 2: unexpected guard")
	}
}

func TestParseTypeSigComplex(t *testing.T) {
	cases := []struct {
		in         string
		wantParams []string
		wantRet    string
	}{
		// Nested list types
		{"{(list number) --> number --> boolean}",
			[]string{"(list number)", "number"}, "boolean"},
		// Nested list of lists
		{"{(list (list number)) --> number}",
			[]string{"(list (list number))"}, "number"},
		// Four params
		{"{string --> number --> boolean --> string --> number}",
			[]string{"string", "number", "boolean", "string"}, "number"},
	}
	for _, tc := range cases {
		sig, err := parseTypeSig(tc.in)
		if err != nil {
			t.Errorf("parseTypeSig(%q): %v", tc.in, err)
			continue
		}
		if len(sig.ParamTypes) != len(tc.wantParams) {
			t.Errorf("parseTypeSig(%q): params=%v, want %v", tc.in, sig.ParamTypes, tc.wantParams)
			continue
		}
		for i, p := range sig.ParamTypes {
			if p != tc.wantParams[i] {
				t.Errorf("parseTypeSig(%q): params[%d]=%q, want %q", tc.in, i, p, tc.wantParams[i])
			}
		}
		if sig.ReturnType != tc.wantRet {
			t.Errorf("parseTypeSig(%q): ret=%q, want %q", tc.in, sig.ReturnType, tc.wantRet)
		}
	}
}

func TestStripShenCommentsLineComments(t *testing.T) {
	in := `\\ this is a line comment
(define foo
  \\ comment in the middle
  X -> X)`
	out := stripShenComments(in)
	if containsAny(out, `\\`) {
		t.Errorf("line comments not stripped: %q", out)
	}
	if !containsAll(out, "define foo", "X -> X") {
		t.Errorf("body lost: %q", out)
	}
}

func TestStripShenCommentsMixed(t *testing.T) {
	in := `\* block comment *\
\\ line comment
(datatype amount
  X : number;
  \* inline block *\
  (>= X 0) : verified;
  =====================
  X : amount;)`
	out := stripShenComments(in)
	if containsAny(out, `\*`, `*\`, `\\`) {
		t.Errorf("comments not stripped: %q", out)
	}
	if !containsAll(out, "datatype amount", "X : number", ">= X 0") {
		t.Errorf("body lost: %q", out)
	}
}

func TestStripShenCommentsPreservesStrings(t *testing.T) {
	// String containing \\ should not be treated as a comment delimiter
	in := `(define foo X -> "hello \\\\ world")`
	out := stripShenComments(in)
	if !containsAll(out, `"hello \\\\ world"`) {
		t.Errorf("string was mangled: %q", out)
	}
}

func TestExtractBlocksMultipleDefines(t *testing.T) {
	content := `
(define foo
  X -> X)

(define bar
  Y -> (+ Y 1))
`
	blocks := extractBlocks(content, "(define ")
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if !containsAll(blocks[0], "define foo") {
		t.Errorf("block 0: %q", blocks[0])
	}
	if !containsAll(blocks[1], "define bar") {
		t.Errorf("block 1: %q", blocks[1])
	}
}

func TestExtractBlocksStringWithParens(t *testing.T) {
	// Parens inside string literals should not confuse the extractor.
	content := `(define greet
  X -> "(hello)")`
	blocks := extractBlocks(content, "(define ")
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if !containsAll(blocks[0], `"(hello)"`) {
		t.Errorf("block: %q", blocks[0])
	}
}

func TestParseMultipleDefinesInFile(t *testing.T) {
	content := `
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(define foo
  {number --> number}
  X -> (+ X 1))

(define bar
  {number --> number --> number}
  X Y -> (* X Y))
`
	// Write to temp file
	sf, err := parseContent(content)
	if err != nil {
		t.Fatalf("parseContent: %v", err)
	}
	if len(sf.Datatypes) != 1 {
		t.Errorf("datatypes: got %d, want 1", len(sf.Datatypes))
	}
	if len(sf.Defines) != 2 {
		t.Fatalf("defines: got %d, want 2", len(sf.Defines))
	}
	if sf.Defines[0].Name != "foo" {
		t.Errorf("define 0: %q", sf.Defines[0].Name)
	}
	if sf.Defines[1].Name != "bar" {
		t.Errorf("define 1: %q", sf.Defines[1].Name)
	}
}

func TestParseDosageSpec(t *testing.T) {
	// The dosage-calculator example was removed in commit eb3be68
	// (demo-readiness cleanup) but this test was left behind. Skip
	// rather than delete, in case the multi-clause-define fixture
	// returns; the parsing it exercises is also covered by inline
	// parseContent tests above.
	if _, err := os.Stat("../../examples/dosage-calculator/specs/core.shen"); os.IsNotExist(err) {
		t.Skip("dosage-calculator example removed in eb3be68; fixture no longer present")
	}
	sf, err := ParseFile(filepath.Clean("../../examples/dosage-calculator/specs/core.shen"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Should extract both defines: pair-in-list? and drug-clear-of-list?
	if len(sf.Defines) != 2 {
		t.Fatalf("defines: got %d, want 2", len(sf.Defines))
	}

	pairDef := sf.FindDefine("pair-in-list?")
	if pairDef == nil {
		t.Fatal("pair-in-list? not found")
	}
	if len(pairDef.Clauses) != 4 {
		t.Errorf("pair-in-list? clauses: got %d, want 4", len(pairDef.Clauses))
	}

	drugDef := sf.FindDefine("drug-clear-of-list?")
	if drugDef == nil {
		t.Fatal("drug-clear-of-list? not found")
	}
	if len(drugDef.Clauses) != 3 {
		t.Errorf("drug-clear-of-list? clauses: got %d, want 3", len(drugDef.Clauses))
	}
}

func TestParseDefineSingleAtomBody(t *testing.T) {
	// A clause whose body is a single atom (not parenthesized).
	block := `(define identity
  {number --> number}
  X -> X)`

	def, err := parseDefine(block)
	if err != nil {
		t.Fatalf("parseDefine: %v", err)
	}
	if len(def.Clauses) != 1 {
		t.Fatalf("clauses: got %d, want 1", len(def.Clauses))
	}
	if def.Clauses[0].Body == nil {
		t.Fatal("body is nil")
	}
}

func TestSplitPatterns(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"X Y Z", []string{"X", "Y", "Z"}},
		{"_ _ []", []string{"_", "_", "[]"}},
		{"A B [[X Y] | Rest]", []string{"A", "B", "[[X Y] | Rest]"}},
		{"Drug [Med | Meds] Pairs", []string{"Drug", "[Med | Meds]", "Pairs"}},
		{"(cons X Y) Z", []string{"(cons X Y)", "Z"}},
	}
	for _, tc := range cases {
		got := splitPatterns(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitPatterns(%q): got %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitPatterns(%q)[%d]: got %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestParseDocAnnotations(t *testing.T) {
	content := `
\* :doc "An Amount is a non-negative number representing money." *\
(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

\* :doc "Processable folds over a transaction list checking the running balance never goes negative." *\
(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> true)

\* unrelated comment *\
(datatype account-id
  X : string;
  ==============
  X : account-id;)
`
	dir := t.TempDir()
	specPath := filepath.Join(dir, "test.shen")
	if err := os.WriteFile(specPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	sf, err := ParseFile(specPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	amount := sf.FindDatatype("amount")
	if amount == nil {
		t.Fatal("amount datatype missing")
	}
	if want := "An Amount is a non-negative number representing money."; amount.Doc != want {
		t.Errorf("amount.Doc = %q, want %q", amount.Doc, want)
	}
	processable := sf.FindDefine("processable")
	if processable == nil {
		t.Fatal("processable define missing")
	}
	if want := "Processable folds over a transaction list checking the running balance never goes negative."; processable.Doc != want {
		t.Errorf("processable.Doc = %q, want %q", processable.Doc, want)
	}
	accountID := sf.FindDatatype("account-id")
	if accountID == nil {
		t.Fatal("account-id datatype missing")
	}
	if accountID.Doc != "" {
		t.Errorf("account-id.Doc = %q, want empty (only :doc annotations populate this field)", accountID.Doc)
	}
}

// TestExtractRuntimeViaMarkers pins parsing of the :runtime-via
// annotation grammar through the full parse path, including survival of
// the global comment-strip pass. Each datatype carries one verified
// premise; only some carry a marker.
func TestExtractRuntimeViaMarkers(t *testing.T) {
	src := `
\* :doc "A wrapper with a plain verified premise — no marker." *\
(datatype plain
  X : string;
  (not (= X "")) : verified;
  ==============
  X : plain;)

(datatype prof-a
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : prof-a;)

(datatype prof-b
  X : string;
  (> (length X) 0) : verified; \* :runtime-via :eval *\
  ==============
  X : prof-b;)

(datatype prof-c
  X : string;
  (> (length X) 0) : verified; \* :runtime-via checkThing :sampled *\
  ==============
  X : prof-c;)

(datatype prof-d
  X : string;
  (> (length X) 0) : verified; \* :runtime-via checkTenantMembership :requires-db *\
  ==============
  X : prof-d;)
`
	sf, err := parseContent(src)
	if err != nil {
		t.Fatalf("parseContent: %v", err)
	}

	marker := func(name string) *RuntimeViaMarker {
		dt := sf.FindDatatype(name)
		if dt == nil || len(dt.Rules) == 0 || len(dt.Rules[0].Verified) == 0 {
			t.Fatalf("%s: no verified premise parsed", name)
		}
		return dt.Rules[0].Verified[0].RuntimeVia
	}

	if m := marker("plain"); m != nil {
		t.Errorf("plain: marker = %+v, want nil", m)
	}
	if m := marker("prof-a"); m == nil || m.Profile() != "A" || m.Checker != "shenEval" {
		t.Errorf("prof-a: marker = %+v, want {Checker:shenEval} profile A", m)
	}
	if m := marker("prof-b"); m == nil || m.Profile() != "B" || !m.Eval {
		t.Errorf("prof-b: marker = %+v, want {Eval:true} profile B", m)
	}
	if m := marker("prof-c"); m == nil || m.Profile() != "C" || m.Checker != "checkThing" || !m.Sampled {
		t.Errorf("prof-c: marker = %+v, want {Checker:checkThing,Sampled} profile C", m)
	}
	if m := marker("prof-d"); m == nil || m.Profile() != "D" || m.Checker != "checkTenantMembership" || !m.RequiresDB {
		t.Errorf("prof-d: marker = %+v, want {Checker:checkTenantMembership,RequiresDB} profile D", m)
	}
}

// parseContent is a test helper that parses content without needing a file.
func parseContent(content string) (*SpecFile, error) {
	markers := extractRuntimeViaMarkers(content)
	content = stripShenComments(content)
	sf := &SpecFile{Path: "<test>"}
	for _, block := range extractBlocks(content, "(datatype ") {
		if dt, err := parseDatatype(block, markers); err != nil {
			return nil, err
		} else if dt != nil {
			sf.Datatypes = append(sf.Datatypes, *dt)
		}
	}
	for _, block := range extractBlocks(content, "(define ") {
		if def, err := parseDefine(block); err != nil {
			return nil, err
		} else if def != nil {
			sf.Defines = append(sf.Defines, *def)
		}
	}
	return sf, nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) < 0 {
			return false
		}
	}
	return true
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
