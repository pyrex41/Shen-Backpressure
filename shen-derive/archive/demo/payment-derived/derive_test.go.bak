package payment_derived

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/codegen"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/shen"
)

// apply b tx = b - tx
func mkApply() core.Term {
	return core.MkLam("b", &core.TInt{},
		core.MkLam("tx", &core.TInt{},
			core.MkApps(core.MkPrim(core.PrimSub), core.MkVar("b"), core.MkVar("tx")),
		),
	)
}

// geq0 x = x >= 0
func mkGeq0() core.Term {
	return core.MkLam("x", &core.TInt{},
		core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
	)
}

// all p xs = foldr (\x acc -> p x && acc) True xs
func mkAll(p core.Term) core.Term {
	return core.MkApps(core.MkPrim(core.PrimFoldr),
		core.MkLam("x", nil,
			core.MkLam("acc", nil,
				core.MkApps(core.MkPrim(core.PrimAnd),
					core.MkApp(p, core.MkVar("x")),
					core.MkVar("acc"),
				),
			),
		),
		core.MkBool(true),
	)
}

func mkProcessableSpec() core.Term {
	return core.MkLam("b0", &core.TInt{},
		core.MkLam("txs", &core.TList{Elem: &core.TInt{}},
			core.MkApp(mkAll(mkGeq0()),
				core.MkApps(core.MkPrim(core.PrimScanl),
					mkApply(),
					core.MkVar("b0"),
					core.MkVar("txs"),
				),
			),
		),
	)
}

type paymentCase struct {
	name string
	b0   int64
	txs  []int64
	want string
}

func evalProcessable(t *testing.T, fn core.Term, b0 int64, txs []int64) string {
	t.Helper()

	terms := make([]core.Term, len(txs))
	for i, tx := range txs {
		terms[i] = core.MkInt(tx)
	}

	val, err := core.Eval(core.EmptyEnv(),
		core.MkApps(fn, core.MkInt(b0), core.MkList(terms...)),
	)
	if err != nil {
		t.Fatalf("eval processable(%d, %v): %v", b0, txs, err)
	}
	return val.String()
}

func formatIntList(xs []int64) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprintf("%d", x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func renderTranscript(original, rewritten core.Term, goCode string, cases []paymentCase) string {
	var b strings.Builder

	b.WriteString("=== shen-derive: Payment Demo Derivation Transcript ===\n\n")
	b.WriteString("--- Step 0: Naive specification ---\n\n")
	b.WriteString("processable b0 txs = all (>= 0) (scanl apply b0 txs)\n")
	b.WriteString("where apply b tx = b - tx\n")
	b.WriteString("      all p xs = foldr (\\x acc -> p x && acc) True xs\n\n")
	b.WriteString("AST:\n")
	b.WriteString("  " + core.PrettyPrint(original) + "\n\n")

	b.WriteString("--- Step 1: Apply named rewrite ---\n\n")
	b.WriteString("Rule: all-scanl-fusion\n")
	b.WriteString("Before:\n")
	b.WriteString("  " + core.PrettyPrint(original) + "\n")
	b.WriteString("After:\n")
	b.WriteString("  " + core.PrettyPrint(rewritten) + "\n")
	b.WriteString("Obligations: none\n\n")

	b.WriteString("--- Step 2: Verify evaluator equivalence ---\n\n")
	for _, tc := range cases {
		b.WriteString(fmt.Sprintf("%s: processable %d %s = %s\n", tc.name, tc.b0, formatIntList(tc.txs), tc.want))
	}
	b.WriteString("\n")

	b.WriteString("--- Step 3: Lower rewritten term to Go ---\n\n")
	for _, line := range strings.Split(strings.TrimRight(goCode, "\n"), "\n") {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n")

	b.WriteString("--- Step 4: Artifact checks ---\n\n")
	b.WriteString("Generated Go matches checked-in artifact.\n")
	b.WriteString("Transcript matches checked-in artifact.\n")
	b.WriteString("Generated Go compiles and passes focused tests.\n")

	return b.String()
}

func normalizeArtifact(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}

func compileGeneratedProcessable(t *testing.T, goCode string) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module paymenttest\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "processable.go"), []byte(goCode), 0644); err != nil {
		t.Fatalf("write processable.go: %v", err)
	}

	testCode := `package payment_derived

import "testing"

func TestGeneratedProcessable(t *testing.T) {
	if Processable(100, []int{30, 50, 40}) {
		t.Fatal("expected overspend case to fail")
	}
	if !Processable(100, []int{30, 20, 10}) {
		t.Fatal("expected safe transactions to pass")
	}
	if Processable(-1, []int{}) {
		t.Fatal("expected negative initial balance to fail")
	}
}`
	if err := os.WriteFile(filepath.Join(dir, "processable_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatalf("write processable_test.go: %v", err)
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test failed: %v\n%s\nGenerated source:\n%s", err, out, goCode)
	}
}

func TestPaymentDerivation(t *testing.T) {
	spec := mkProcessableSpec()

	rewrittenResult, err := shen.RewriteStrict(spec, laws.AllScanlFusion(), laws.Path{0, 0}, nil)
	if err != nil {
		t.Fatalf("RewriteStrict: %v", err)
	}
	if len(rewrittenResult.Obligations) != 0 {
		t.Fatalf("expected unconditional rewrite, got %d obligations", len(rewrittenResult.Obligations))
	}

	cases := []paymentCase{
		{name: "overspend", b0: 100, txs: []int64{30, 50, 40}, want: "false"},
		{name: "safe", b0: 100, txs: []int64{30, 20, 10}, want: "true"},
		{name: "negative-initial", b0: -1, txs: nil, want: "false"},
	}
	for _, tc := range cases {
		specVal := evalProcessable(t, spec, tc.b0, tc.txs)
		rewrittenVal := evalProcessable(t, rewrittenResult.Rewritten, tc.b0, tc.txs)
		if specVal != tc.want {
			t.Fatalf("%s: spec = %s, want %s", tc.name, specVal, tc.want)
		}
		if rewrittenVal != tc.want {
			t.Fatalf("%s: rewritten = %s, want %s", tc.name, rewrittenVal, tc.want)
		}
	}

	ty, err := core.CheckTerm(rewrittenResult.Rewritten)
	if err != nil {
		t.Fatalf("CheckTerm rewritten: %v", err)
	}
	goCode, err := codegen.LowerToGo(rewrittenResult.Rewritten, ty, "Processable", "payment_derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileGeneratedProcessable(t, goCode)

	transcript := renderTranscript(spec, rewrittenResult.Rewritten, goCode, cases)

	expectedGo, err := os.ReadFile("processable.go")
	if err != nil {
		t.Fatalf("read processable.go: %v", err)
	}
	if normalizeArtifact(string(expectedGo)) != normalizeArtifact(goCode) {
		t.Fatalf("checked-in processable.go is stale\n--- expected ---\n%s\n--- actual ---\n%s", expectedGo, goCode)
	}

	expectedTranscript, err := os.ReadFile("derivation.txt")
	if err != nil {
		t.Fatalf("read derivation.txt: %v", err)
	}
	if normalizeArtifact(string(expectedTranscript)) != normalizeArtifact(transcript) {
		t.Fatalf("checked-in derivation.txt is stale\n--- expected ---\n%s\n--- actual ---\n%s", expectedTranscript, transcript)
	}
}
