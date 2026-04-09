package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
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
