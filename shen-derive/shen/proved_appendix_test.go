package shen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/codegen"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/laws"
)

// TestProvedAppendixCoverage is a single-file test-only follow-up for shen-derive.
// It adds executable proved-appendix coverage for negate-sum and double-sum
// WITHOUT changing the fixed 20-target corpus (see corpus_test.go which we
// must not touch).
//
// For each case:
// - construct original foldr-fusion term + correct ?h witness
// - call RewriteStrict(FoldrFusion, supplemental ?h)
// - assert rewrite succeeds
// - compare original/rewritten evaluator outputs on representative inputs
// - lower rewritten term to Go
// - compile/run generated Go in temp module with focused assertions
//
// PROOF BOUNDARY IS EXPLICIT IN ALL TEST NAMES AND COMMENTS:
//
//	Only symbolic-polynomial discharge (from prove.go:dischargeSymbolic +
//	polynomial normalization) is treated as a sound proof. Other methods
//	(shen-tc+, empirical) remain diagnostic-only. This test verifies the
//	exact proof method and keeps the boundary crystal-clear.
func TestProvedAppendixCoverage(t *testing.T) {
	t.Run("negate-sum-proof-boundary", testProvedNegateSum)
	t.Run("double-sum-proof-boundary", testProvedDoubleSum)
}

func testProvedNegateSum(t *testing.T) {
	// PROOF BOUNDARY: negate . sum where sum = foldr (+) 0.
	// The side condition negate((x + y)) == h x (negate y) with h = \x z -> z - x
	// is discharged solely by symbolic-polynomial normalization in prove.go.
	// No claim is made for non-arithmetic cases.
	f := core.MkPrim(core.PrimNeg)
	g := core.MkPrim(core.PrimAdd)
	e := core.MkInt(0)
	h := core.MkLam("x", nil, core.MkLam("z", nil,
		core.MkApps(core.MkPrim(core.PrimSub), core.MkVar("z"), core.MkVar("x")),
	))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	rule := laws.FoldrFusion()
	extra := laws.Bindings{"?h": h}

	result, err := RewriteStrict(term, rule, laws.RootPath, extra)
	if err != nil {
		t.Fatalf("RewriteStrict failed for proved negate-sum: %v", err)
	}
	if len(result.Obligations) == 0 {
		t.Fatal("expected side condition obligation from foldr-fusion")
	}
	t.Logf("RewriteStrict succeeded: %s -> %s", core.PrettyPrint(term), core.PrettyPrint(result.Rewritten))

	// Explicitly re-discharge to verify PROOF BOUNDARY (symbolic-polynomial)
	cond := result.Obligations[0]
	dr := Discharge(cond)
	if !dr.Discharged || dr.Method != "symbolic-polynomial" {
		t.Fatalf("PROOF BOUNDARY FAILED: expected symbolic-polynomial proof, got method=%s discharged=%v err=%v output=%s",
			dr.Method, dr.Discharged, dr.Error, dr.Output)
	}
	t.Logf("PROOF BOUNDARY VERIFIED (symbolic-polynomial): %s", dr.Output)

	// Compare evaluator outputs on representative input
	input := core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3), core.MkInt(4))
	origApplied := core.MkApps(term, input)
	rewApplied := core.MkApps(result.Rewritten, input)

	origVal, err := core.Eval(core.EmptyEnv(), origApplied)
	if err != nil {
		t.Fatalf("original eval failed: %v", err)
	}
	rewVal, err := core.Eval(core.EmptyEnv(), rewApplied)
	if err != nil {
		t.Fatalf("rewritten eval failed: %v", err)
	}
	if origVal.String() != rewVal.String() {
		t.Errorf("evaluator mismatch: orig=%s rew=%s", origVal, rewVal)
	}
	if origVal.String() != "-10" {
		t.Errorf("expected -10, got %s", origVal)
	}
	t.Logf("Evaluator equivalence confirmed on [1,2,3,4]: both yield %s", origVal)

	// Lower rewritten term to Go
	listIntToInt := &core.TFun{
		Param:  &core.TList{Elem: &core.TInt{}},
		Result: &core.TInt{},
	}
	goCode, err := codegen.LowerToGo(result.Rewritten, listIntToInt, "NegateSumProved", "provedappendix")
	if err != nil {
		t.Fatalf("LowerToGo failed: %v", err)
	}
	if !strings.Contains(goCode, "func NegateSumProved") {
		t.Error("generated Go does not contain expected function name")
	}

	// Compile/run generated Go in temp module with focused assertions
	if err := runProvedAppendixGo(t, goCode, "NegateSumProved", []int{1, 2, 3, 4}, -10); err != nil {
		t.Fatalf("Go compile/run failed for negate-sum: %v", err)
	}
}

func testProvedDoubleSum(t *testing.T) {
	// PROOF BOUNDARY: double . sum where double = \n -> n*2.
	// The side condition (x+y)*2 == h x ((y*2)) with h = \x y -> (x*2)+y
	// is discharged solely by symbolic-polynomial normalization.
	// Explicit proof boundary maintained in test name and all comments.
	f := core.MkLam("n", nil, core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("n"), core.MkInt(2)))
	g := core.MkPrim(core.PrimAdd)
	e := core.MkInt(0)
	h := core.MkLam("x", nil, core.MkLam("y", nil,
		core.MkApps(core.MkPrim(core.PrimAdd),
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkInt(2)),
			core.MkVar("y"),
		),
	))

	term := core.MkApps(core.MkPrim(core.PrimCompose),
		f,
		core.MkApps(core.MkPrim(core.PrimFoldr), g, e),
	)

	rule := laws.FoldrFusion()
	extra := laws.Bindings{"?h": h}

	result, err := RewriteStrict(term, rule, laws.RootPath, extra)
	if err != nil {
		t.Fatalf("RewriteStrict failed for proved double-sum: %v", err)
	}
	if len(result.Obligations) == 0 {
		t.Fatal("expected side condition obligation from foldr-fusion")
	}
	t.Logf("RewriteStrict succeeded for double-sum: %s", core.PrettyPrint(result.Rewritten))

	// Explicit PROOF BOUNDARY verification
	cond := result.Obligations[0]
	dr := Discharge(cond)
	if !dr.Discharged || dr.Method != "symbolic-polynomial" {
		t.Fatalf("PROOF BOUNDARY FAILED for double-sum: expected symbolic-polynomial, got %s: %v", dr.Method, dr.Error)
	}
	t.Logf("PROOF BOUNDARY VERIFIED (symbolic-polynomial) for double-sum")

	// Compare evaluator outputs
	input := core.MkList(core.MkInt(1), core.MkInt(2), core.MkInt(3))
	origApplied := core.MkApps(term, input)
	rewApplied := core.MkApps(result.Rewritten, input)

	origVal, err := core.Eval(core.EmptyEnv(), origApplied)
	if err != nil {
		t.Fatalf("original eval: %v", err)
	}
	rewVal, err := core.Eval(core.EmptyEnv(), rewApplied)
	if err != nil {
		t.Fatalf("rewritten eval: %v", err)
	}
	if origVal.String() != rewVal.String() || origVal.String() != "12" {
		t.Errorf("evaluator mismatch or wrong result: %s", origVal)
	}
	t.Logf("Evaluator equivalence confirmed: both = %s", origVal)

	// Lower to Go
	listIntToInt := &core.TFun{
		Param:  &core.TList{Elem: &core.TInt{}},
		Result: &core.TInt{},
	}
	goCode, err := codegen.LowerToGo(result.Rewritten, listIntToInt, "DoubleSumProved", "provedappendix")
	if err != nil {
		t.Fatalf("LowerToGo failed: %v", err)
	}

	// Compile and run with local helper
	if err := runProvedAppendixGo(t, goCode, "DoubleSumProved", []int{1, 2, 3}, 12); err != nil {
		t.Fatalf("Go compile/run failed for double-sum: %v", err)
	}
}

// runProvedAppendixGo is a self-contained compile helper defined locally in
// this test-only file (as instructed). It writes the FULL generated Go code
// (including its own "package provedappendix" and imports) to appendix.go,
// then writes a separate _test.go with the focused Test func. This avoids
// duplicate package declarations. Runs `go test -run=TestFuncName -v`.
func runProvedAppendixGo(t *testing.T, goCode, funcName string, input []int, want int) error {
	tmpDir, err := os.MkdirTemp("", "shen-proved-appendix-"+funcName+"-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(`module provedappendix
go 1.24
`), 0644); err != nil {
		return err
	}

	// Write the FULL lowered code (it already contains "package provedappendix")
	if err := os.WriteFile(filepath.Join(tmpDir, "appendix.go"), []byte(goCode), 0644); err != nil {
		return err
	}

	inputStr := strings.Trim(strings.ReplaceAll(fmt.Sprintf("%v", input), " ", ", "), "[]")

	testCode := fmt.Sprintf(`package provedappendix

import "testing"

func Test%s(t *testing.T) {
	input := []int{%s}
	got := %s(input)
	if got != %d {
		t.Errorf("expected %d, got %%d", got)
	}
	t.Logf("proved-appendix %s(%%v) = %%d (OK)", input, got)
}
`, funcName, inputStr, funcName, want, want, funcName)

	testFile := filepath.Join(tmpDir, "appendix_test.go")
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		return err
	}

	cmd := exec.Command("go", "test", "-run", "Test"+funcName, "-v")
	cmd.Dir = tmpDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test failed for %s: %w\noutput:\n%s", funcName, err, out.String())
	}
	t.Logf("Successfully compiled+ran proved Go for %s (temp module cleaned up)", funcName)
	return nil
}
