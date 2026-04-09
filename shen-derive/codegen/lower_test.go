package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// TestLowerFoldrAllNonNeg tests: foldr (\x acc -> if x >= 0 then acc else False) True
// Expected: a Go function that checks all elements are non-negative.
func TestLowerFoldrAllNonNeg(t *testing.T) {
	// foldr (\x acc -> if x >= 0 then acc else False) True
	stepFn := core.MkLam("x", &core.TInt{},
		core.MkLam("acc", &core.TBool{},
			core.MkIf(
				core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
				core.MkVar("acc"),
				core.MkBool(false),
			),
		),
	)

	term := core.MkApps(core.MkPrim(core.PrimFoldr), stepFn, core.MkBool(true))
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TBool{}}

	goCode, err := LowerToGo(term, ty, "AllNonNeg", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}
	t.Logf("Generated Go:\n%s", goCode)

	// Compile and test
	compileAndTest(t, goCode, `
package derived

import "testing"

func TestAllNonNeg(t *testing.T) {
	if !AllNonNeg([]int{1, 2, 3, 4, 5}) {
		t.Error("expected true for all positive")
	}
	if AllNonNeg([]int{1, -2, 3}) {
		t.Error("expected false when negative present")
	}
	if !AllNonNeg([]int{}) {
		t.Error("expected true for empty list")
	}
	if !AllNonNeg([]int{0}) {
		t.Error("expected true for zero")
	}
}
`)
}

// TestLowerFoldlSum tests: foldl (+) 0
// Expected: a Go function that sums a list.
func TestLowerFoldlSum(t *testing.T) {
	term := core.MkApps(core.MkPrim(core.PrimFoldl),
		core.MkPrim(core.PrimAdd),
		core.MkInt(0),
	)
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TInt{}}

	goCode, err := LowerToGo(term, ty, "Sum", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}
	t.Logf("Generated Go:\n%s", goCode)

	compileAndTest(t, goCode, `
package derived

import "testing"

func TestSum(t *testing.T) {
	if got := Sum([]int{1, 2, 3, 4, 5}); got != 15 {
		t.Errorf("Sum([1..5]) = %d, want 15", got)
	}
	if got := Sum([]int{}); got != 0 {
		t.Errorf("Sum([]) = %d, want 0", got)
	}
	if got := Sum([]int{-1, 2, -3}); got != -2 {
		t.Errorf("Sum([-1,2,-3]) = %d, want -2", got)
	}
}
`)
}

// TestLowerFilterPositive tests: filter (\x -> x > 0)
// Expected: a Go function that keeps only positive elements.
func TestLowerFilterPositive(t *testing.T) {
	term := core.MkApp(core.MkPrim(core.PrimFilter),
		core.MkLam("x", &core.TInt{},
			core.MkApps(core.MkPrim(core.PrimGt), core.MkVar("x"), core.MkInt(0)),
		),
	)
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TList{Elem: &core.TInt{}}}

	goCode, err := LowerToGo(term, ty, "FilterPositive", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}
	t.Logf("Generated Go:\n%s", goCode)

	compileAndTest(t, goCode, `
package derived

import (
	"testing"
	"reflect"
)

func TestFilterPositive(t *testing.T) {
	got := FilterPositive([]int{-2, -1, 0, 1, 2, 3})
	want := []int{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterPositive = %v, want %v", got, want)
	}
	got2 := FilterPositive([]int{-1, -2})
	if len(got2) != 0 {
		t.Errorf("FilterPositive(all neg) = %v, want []", got2)
	}
}
`)
}

// TestLowerScanlRunningSum tests: scanl (+) 0
func TestLowerScanlRunningSum(t *testing.T) {
	term := core.MkApps(core.MkPrim(core.PrimScanl),
		core.MkPrim(core.PrimAdd),
		core.MkInt(0),
	)
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TList{Elem: &core.TInt{}}}

	goCode, err := LowerToGo(term, ty, "RunningSum", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}
	t.Logf("Generated Go:\n%s", goCode)

	compileAndTest(t, goCode, `
package derived

import (
	"testing"
	"reflect"
)

func TestRunningSum(t *testing.T) {
	got := RunningSum([]int{1, 2, 3})
	want := []int{0, 1, 3, 6}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunningSum([1,2,3]) = %v, want %v", got, want)
	}
	got2 := RunningSum([]int{})
	want2 := []int{0}
	if !reflect.DeepEqual(got2, want2) {
		t.Errorf("RunningSum([]) = %v, want %v", got2, want2)
	}
}
`)
}

// TestLowerMapSquare tests: map (\x -> x * x)
func TestLowerMapSquare(t *testing.T) {
	term := core.MkApp(core.MkPrim(core.PrimMap),
		core.MkLam("x", &core.TInt{},
			core.MkApps(core.MkPrim(core.PrimMul), core.MkVar("x"), core.MkVar("x")),
		),
	)
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TList{Elem: &core.TInt{}}}

	goCode, err := LowerToGo(term, ty, "Squares", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}
	t.Logf("Generated Go:\n%s", goCode)

	compileAndTest(t, goCode, `
package derived

import (
	"testing"
	"reflect"
)

func TestSquares(t *testing.T) {
	got := Squares([]int{1, 2, 3, 4})
	want := []int{1, 4, 9, 16}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Squares = %v, want %v", got, want)
	}
	got2 := Squares([]int{})
	if len(got2) != 0 {
		t.Errorf("Squares([]) = %v, want []", got2)
	}
}
`)
}

// TestLowerFoldrWithLambdaBody tests a foldr with if-then-else in the step function:
// foldr (\x acc -> if x > 0 then x + acc else acc) 0  (sum of positives)
func TestLowerFoldrSumPositives(t *testing.T) {
	stepFn := core.MkLam("x", &core.TInt{},
		core.MkLam("acc", &core.TInt{},
			core.MkIf(
				core.MkApps(core.MkPrim(core.PrimGt), core.MkVar("x"), core.MkInt(0)),
				core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkVar("acc")),
				core.MkVar("acc"),
			),
		),
	)

	term := core.MkApps(core.MkPrim(core.PrimFoldr), stepFn, core.MkInt(0))
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TInt{}}

	goCode, err := LowerToGo(term, ty, "SumPositives", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}
	t.Logf("Generated Go:\n%s", goCode)

	compileAndTest(t, goCode, `
package derived

import "testing"

func TestSumPositives(t *testing.T) {
	if got := SumPositives([]int{1, -2, 3, -4, 5}); got != 9 {
		t.Errorf("SumPositives = %d, want 9", got)
	}
	if got := SumPositives([]int{-1, -2}); got != 0 {
		t.Errorf("SumPositives(all neg) = %d, want 0", got)
	}
	if got := SumPositives([]int{}); got != 0 {
		t.Errorf("SumPositives([]) = %d, want 0", got)
	}
}
`)
}

func TestLowerBoolSingletonList(t *testing.T) {
	term := core.MkLam("x", &core.TBool{},
		core.MkList(core.MkVar("x")),
	)
	ty := &core.TFun{
		Param:  &core.TBool{},
		Result: &core.TList{Elem: &core.TBool{}},
	}

	goCode, err := LowerToGo(term, ty, "Singleton", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileAndTest(t, goCode, `
package derived

import (
	"reflect"
	"testing"
)

func TestSingleton(t *testing.T) {
	if got := Singleton(true); !reflect.DeepEqual(got, []bool{true}) {
		t.Fatalf("Singleton(true) = %v, want [true]", got)
	}
	if got := Singleton(false); !reflect.DeepEqual(got, []bool{false}) {
		t.Fatalf("Singleton(false) = %v, want [false]", got)
	}
}
`)
}

func TestLowerMapWithLet(t *testing.T) {
	term := core.MkApp(core.MkPrim(core.PrimMap),
		core.MkLam("x", &core.TInt{},
			core.MkLet("y",
				core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)),
				core.MkVar("y"),
			),
		),
	)
	ty := &core.TFun{
		Param:  &core.TList{Elem: &core.TInt{}},
		Result: &core.TList{Elem: &core.TInt{}},
	}

	goCode, err := LowerToGo(term, ty, "IncAll", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileAndTest(t, goCode, `
package derived

import (
	"reflect"
	"testing"
)

func TestIncAll(t *testing.T) {
	got := IncAll([]int{1, 2, 3})
	want := []int{2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IncAll = %v, want %v", got, want)
	}
}
`)
}

func TestLowerFoldrBoolList(t *testing.T) {
	stepFn := core.MkLam("x", &core.TInt{},
		core.MkLam("xs", &core.TList{Elem: &core.TBool{}},
			core.MkApps(core.MkPrim(core.PrimCons),
				core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
				core.MkVar("xs"),
			),
		),
	)
	term := core.MkApps(core.MkPrim(core.PrimFoldr), stepFn, core.MkPrim(core.PrimNil))
	ty := &core.TFun{
		Param:  &core.TList{Elem: &core.TInt{}},
		Result: &core.TList{Elem: &core.TBool{}},
	}

	goCode, err := LowerToGo(term, ty, "NonNegFlags", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileAndTest(t, goCode, `
package derived

import (
	"reflect"
	"testing"
)

func TestNonNegFlags(t *testing.T) {
	got := NonNegFlags([]int{-1, 0, 2})
	want := []bool{false, true, true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NonNegFlags = %v, want %v", got, want)
	}
}
`)
}

func TestLowerMapBoolIf(t *testing.T) {
	term := core.MkApp(core.MkPrim(core.PrimMap),
		core.MkLam("x", &core.TBool{},
			core.MkIf(core.MkVar("x"), core.MkVar("x"), core.MkBool(false)),
		),
	)
	ty := &core.TFun{
		Param:  &core.TList{Elem: &core.TBool{}},
		Result: &core.TList{Elem: &core.TBool{}},
	}

	goCode, err := LowerToGo(term, ty, "IdentityBools", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileAndTest(t, goCode, `
package derived

import (
	"reflect"
	"testing"
)

func TestIdentityBools(t *testing.T) {
	got := IdentityBools([]bool{true, false, true})
	want := []bool{true, false, true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IdentityBools = %v, want %v", got, want)
	}
}
`)
}

func TestLowerProjectedFoldlPayment(t *testing.T) {
	step := core.MkLam("state", nil,
		core.MkLam("tx", &core.TInt{},
			core.MkLet("next",
				core.MkApps(core.MkPrim(core.PrimSub),
					core.MkApp(core.MkPrim(core.PrimFst), core.MkVar("state")),
					core.MkVar("tx"),
				),
				core.MkTuple(
					core.MkVar("next"),
					core.MkApps(core.MkPrim(core.PrimAnd),
						core.MkApp(core.MkPrim(core.PrimSnd), core.MkVar("state")),
						core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("next"), core.MkInt(0)),
					),
				),
			),
		),
	)

	term := core.MkLam("b0", &core.TInt{},
		core.MkLam("txs", &core.TList{Elem: &core.TInt{}},
			core.MkApp(core.MkPrim(core.PrimSnd),
				core.MkApps(core.MkPrim(core.PrimFoldl),
					step,
					core.MkTuple(
						core.MkVar("b0"),
						core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("b0"), core.MkInt(0)),
					),
					core.MkVar("txs"),
				),
			),
		),
	)
	ty := &core.TFun{
		Param: &core.TInt{},
		Result: &core.TFun{
			Param:  &core.TList{Elem: &core.TInt{}},
			Result: &core.TBool{},
		},
	}

	goCode, err := LowerToGo(term, ty, "Processable", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileAndTest(t, goCode, `
package derived

import "testing"

func TestProcessable(t *testing.T) {
	if Processable(100, []int{30, 50, 40}) {
		t.Fatal("expected overspend case to fail")
	}
	if !Processable(100, []int{30, 20, 10}) {
		t.Fatal("expected non-negative balances to pass")
	}
	if Processable(-1, []int{}) {
		t.Fatal("expected negative initial balance to fail")
	}
	if Processable(5, []int{3, 2}) != true {
		t.Fatal("expected exact depletion without going negative to pass")
	}
}
`)
}

func TestLowerMapAfterFilter(t *testing.T) {
	term := core.MkLam("xs", &core.TList{Elem: &core.TInt{}},
		core.MkApps(core.MkPrim(core.PrimMap),
			core.MkLam("x", &core.TInt{},
				core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)),
			),
			core.MkApps(core.MkPrim(core.PrimFilter),
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimGt), core.MkVar("x"), core.MkInt(0)),
				),
				core.MkVar("xs"),
			),
		),
	)
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TList{Elem: &core.TInt{}}}

	goCode, err := LowerToGo(term, ty, "MapAfterFilter", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileAndTest(t, goCode, `
package derived

import (
	"reflect"
	"testing"
)

func TestMapAfterFilter(t *testing.T) {
	got := MapAfterFilter([]int{-2, 0, 1, 2, 3})
	want := []int{2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := MapAfterFilter([]int{-2, 0}); len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
}
`)
}

func TestLowerFilterAfterMap(t *testing.T) {
	term := core.MkLam("xs", &core.TList{Elem: &core.TInt{}},
		core.MkApps(core.MkPrim(core.PrimFilter),
			core.MkLam("x", &core.TInt{},
				core.MkApps(core.MkPrim(core.PrimGe), core.MkVar("x"), core.MkInt(0)),
			),
			core.MkApps(core.MkPrim(core.PrimMap),
				core.MkLam("x", &core.TInt{},
					core.MkApps(core.MkPrim(core.PrimSub), core.MkVar("x"), core.MkInt(1)),
				),
				core.MkVar("xs"),
			),
		),
	)
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TList{Elem: &core.TInt{}}}

	goCode, err := LowerToGo(term, ty, "FilterAfterMap", "derived")
	if err != nil {
		t.Fatalf("LowerToGo: %v", err)
	}

	compileAndTest(t, goCode, `
package derived

import (
	"reflect"
	"testing"
)

func TestFilterAfterMap(t *testing.T) {
	got := FilterAfterMap([]int{0, 1, 3})
	want := []int{0, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := FilterAfterMap([]int{-2, -1, 0}); len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
}
`)
}

func TestLowerRejectsUnsupportedNestedPipeline(t *testing.T) {
	term := core.MkLam("xs", &core.TList{Elem: &core.TInt{}},
		core.MkApps(core.MkPrim(core.PrimMap),
			core.MkLam("x", &core.TInt{},
				core.MkApps(core.MkPrim(core.PrimAdd), core.MkVar("x"), core.MkInt(1)),
			),
			core.MkApps(core.MkPrim(core.PrimScanl),
				core.MkPrim(core.PrimAdd),
				core.MkInt(0),
				core.MkVar("xs"),
			),
		),
	)
	ty := &core.TFun{Param: &core.TList{Elem: &core.TInt{}}, Result: &core.TList{Elem: &core.TInt{}}}

	_, err := LowerToGo(term, ty, "UnsupportedNested", "derived")
	if err == nil {
		t.Fatal("expected nested pipeline lowering to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported nested pipeline shape") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// compileAndTest writes Go source + test file to a temp dir, runs go test.
func compileAndTest(t *testing.T, srcCode, testCode string) {
	t.Helper()

	dir := t.TempDir()
	pkg := filepath.Join(dir, "derived")
	os.MkdirAll(pkg, 0755)

	// Write go.mod
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.24\n"), 0644)

	// Write source
	if err := os.WriteFile(filepath.Join(pkg, "derived.go"), []byte(srcCode), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	// Write test
	if err := os.WriteFile(filepath.Join(pkg, "derived_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatalf("write test: %v", err)
	}

	// Run go test
	cmd := exec.Command("go", "test", "-v", "./derived/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	t.Logf("go test output:\n%s", out)
	if err != nil {
		t.Fatalf("go test failed: %v", err)
	}
}
