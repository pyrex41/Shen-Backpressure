package main

// mutate_test.go — the five mutation operators, the mutant id, and the
// score arithmetic, over a small fixture.
//
// The part worth testing hardest is the id. A mutant id is what an
// author writes into sb.toml to mark a mutant equivalent, so it has to
// mean the same thing on the next run: it is computed against the
// *committed* file's positions, never against a partly-mutated tree.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mutateFixture = `package derived

func Processable(b0 float64, txs []float64) bool {
	balance := b0
	for _, tx := range txs {
		balance -= tx
		if balance < 0 {
			return false
		}
	}
	return true
}

func Both(a, b bool) bool { return a && b }

func Either(a, b bool) bool { return a || b }

func Head(xs []int) int { return xs[0] }

func Tail(xs []int) []int { return xs[1:] }
`

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "impl.go")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// mutantsByOperator groups a mutant set for assertions.
func mutantsByOperator(ms []Mutant) map[string][]Mutant {
	out := map[string][]Mutant{}
	for _, m := range ms {
		out[m.Operator] = append(out[m.Operator], m)
	}
	return out
}

func TestGenerateMutantsCoversEveryOperator(t *testing.T) {
	p := writeFixture(t, mutateFixture)
	ms, err := GenerateMutants(p)
	if err != nil {
		t.Fatalf("GenerateMutants: %v", err)
	}
	by := mutantsByOperator(ms)

	for _, op := range []string{OpCmpFlip, OpOffByOne, OpDropConjunct, OpListSwap, OpZeroReturn} {
		if len(by[op]) == 0 {
			t.Errorf("operator %s produced no mutants on the fixture", op)
		}
	}
	// Five functions, each with a result list the operator can build a
	// zero for, so five zero-return mutants.
	if got := len(by[OpZeroReturn]); got != 5 {
		t.Errorf("zero-return produced %d mutants, want one per function (5)", got)
	}
	// Two boolean operators, `&&` and `||`.
	if got := len(by[OpDropConjunct]); got != 2 {
		t.Errorf("drop-conjunct produced %d mutants, want 2", got)
	}
	// xs[0] and xs[1:].
	if got := len(by[OpListSwap]); got != 2 {
		t.Errorf("list-swap produced %d mutants, want 2", got)
	}
}

func TestMutantIDIsStableAndLocated(t *testing.T) {
	p := writeFixture(t, mutateFixture)
	first, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("mutant count changed between runs: %d then %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("mutant id is not stable: %q then %q", first[i].ID, second[i].ID)
		}
		// operator:file:line:col — four colon-separated parts, and the
		// author has to be able to paste it into sb.toml.
		if strings.Count(first[i].ID, ":") != 3 {
			t.Errorf("mutant id %q is not operator:file:line:col", first[i].ID)
		}
		if first[i].Line == 0 {
			t.Errorf("mutant %q has no line number to act on", first[i].ID)
		}
	}
}

func TestMutantSourcesCompileAndDiffer(t *testing.T) {
	p := writeFixture(t, mutateFixture)
	orig, _ := os.ReadFile(p)
	ms, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range ms {
		if len(m.Source) == 0 {
			t.Fatalf("%s: mutant carries no source", m.ID)
		}
		if string(m.Source) == string(orig) {
			t.Errorf("%s: mutant is byte-identical to the original — it would count as a survivor for free", m.ID)
		}
		if seen[string(m.Source)] {
			t.Errorf("%s: duplicate mutant body; one mutation per mutant means one body per mutant", m.ID)
		}
		seen[string(m.Source)] = true
	}
}

func TestCmpFlipRewritesOneOperator(t *testing.T) {
	p := writeFixture(t, "package p\n\nfunc F(a, b int) bool { return a < b }\n")
	ms, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	var flip *Mutant
	for i := range ms {
		if ms[i].Operator == OpCmpFlip {
			flip = &ms[i]
		}
	}
	if flip == nil {
		t.Fatal("no cmp-flip mutant")
	}
	if flip.Before != "<" || flip.After != "<=" {
		t.Errorf("cmp-flip recorded %q → %q, want < → <=", flip.Before, flip.After)
	}
	if !strings.Contains(string(flip.Source), "a <= b") {
		t.Errorf("mutated source does not carry the flip:\n%s", flip.Source)
	}
}

func TestDropConjunctKeepsTheLeftOperand(t *testing.T) {
	// `A && B` → `A && true` and `A || B` → `A || false`. Both mean A,
	// which is "drop the right operand" — and neither changes the
	// expression's shape, so no parent pointer is needed.
	p := writeFixture(t, "package p\n\nfunc And(a, b bool) bool { return a && b }\n\nfunc Or(a, b bool) bool { return a || b }\n")
	ms, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	var gotAnd, gotOr bool
	for _, m := range ms {
		if m.Operator != OpDropConjunct {
			continue
		}
		s := string(m.Source)
		if strings.Contains(s, "a && true") {
			gotAnd = true
		}
		if strings.Contains(s, "a || false") {
			gotOr = true
		}
	}
	if !gotAnd {
		t.Error("&& mutant should reduce to the left operand via `&& true`")
	}
	if !gotOr {
		t.Error("|| mutant should reduce to the left operand via `|| false`")
	}
}

func TestListSwapExchangesHeadForLastAndTailForInit(t *testing.T) {
	p := writeFixture(t, "package p\n\nfunc H(xs []int) int { return xs[0] }\n\nfunc T(xs []int) []int { return xs[1:] }\n")
	ms, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	var head, tail bool
	for _, m := range ms {
		if m.Operator != OpListSwap {
			continue
		}
		s := string(m.Source)
		if strings.Contains(s, "xs[len(xs)-1]") {
			head = true
		}
		if strings.Contains(s, "xs[:len(xs)-1]") {
			tail = true
		}
	}
	if !head {
		t.Error("head→last mutant missing: xs[0] should become xs[len(xs)-1]")
	}
	if !tail {
		t.Error("tail→init mutant missing: xs[1:] should become xs[:len(xs)-1]")
	}
}

func TestZeroReturnBuildsTheRightZero(t *testing.T) {
	p := writeFixture(t, `package p

type Guard struct{}

func B() bool                  { return true }
func S() string                { return "x" }
func N() int                   { return 1 }
func G() (Guard, error)        { return Guard{}, nil }
func P() *Guard                { return nil }
func Named() (ok bool, e error) { return true, nil }
`)
	ms, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"B":     "return false",
		"S":     `return ""`,
		"N":     "return 0",
		"G":     "return Guard{}, nil",
		"P":     "return nil",
		"Named": "return false, nil",
	}
	got := map[string]bool{}
	for _, m := range ms {
		if m.Operator != OpZeroReturn {
			continue
		}
		for fn, stmt := range want {
			if strings.Contains(m.Before, "func "+fn+" ") && m.After == stmt {
				got[fn] = true
			}
		}
	}
	for fn := range want {
		if !got[fn] {
			t.Errorf("no zero-return mutant for %s with body %q", fn, want[fn])
		}
	}
}

func TestZeroReturnSkipsVoidFunctions(t *testing.T) {
	p := writeFixture(t, "package p\n\nfunc Nothing() {}\n")
	ms, err := GenerateMutants(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.Operator == OpZeroReturn {
			t.Error("a function with no results has no zero value to return early; that mutant would be a no-op")
		}
	}
}

func TestIncrementLiteral(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"0", "1", true},
		{"41", "42", true},
		{"2.5", "3.5", true},
		{"0x1f", "", false},
		{"1e9", "", false},
		{"1_000", "", false},
	} {
		got, ok := incrementLiteral(tc.in)
		if ok != tc.ok {
			t.Errorf("incrementLiteral(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("incrementLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestKillRate(t *testing.T) {
	for _, tc := range []struct {
		caught, survived int
		want             float64
	}{
		{3, 1, 0.75},
		{4, 0, 1},
		{0, 2, 0},
		// Nothing live at all: there was nothing left to miss, so the
		// rate is 1 rather than a divide by zero.
		{0, 0, 1},
	} {
		if got := killRate(tc.caught, tc.survived); got != tc.want {
			t.Errorf("killRate(%d, %d) = %v, want %v", tc.caught, tc.survived, got, tc.want)
		}
	}
}

func TestIsGoBuildFailure(t *testing.T) {
	// A mutant the compiler rejects is not evidence about the test, so
	// it must not be counted as a kill.
	if !isGoBuildFailure("# example/p\n./impl.go:7:2: undefined: q\nFAIL\texample/p [build failed]\n") {
		t.Error("a [build failed] run must be classified invalid, not caught")
	}
	if isGoBuildFailure("--- FAIL: TestSpec_Processable (0.00s)\nFAIL\texample/p\t0.002s\n") {
		t.Error("a genuine test failure must be classified caught, not invalid")
	}
}

func TestStripSourceKeepsTheActionableFields(t *testing.T) {
	m := stripSource(Mutant{ID: "x:y:1:2", Before: "<", After: "<=", Source: []byte("package p")})
	if m.Source != nil {
		t.Error("a mutant in a report must not carry a copy of the mutated file")
	}
	if m.ID == "" || m.Before == "" || m.After == "" {
		t.Error("the id and the before/after fragments are what a reader acts on; they must survive")
	}
}

func TestMutationEquivalentListIsReadFromConfig(t *testing.T) {
	cfg := &Config{}
	applyMutation(cfg, tomlMutation{
		Equivalent: []string{"off-by-one:internal/derived/x.go:12:20"},
		Timeout:    "90s",
	})
	if len(cfg.Mutation.Equivalent) != 1 || cfg.Mutation.Equivalent[0] != "off-by-one:internal/derived/x.go:12:20" {
		t.Errorf("Equivalent = %v", cfg.Mutation.Equivalent)
	}
	if cfg.Mutation.Timeout != "90s" {
		t.Errorf("Timeout = %q, want 90s", cfg.Mutation.Timeout)
	}
}

func TestRenderGateStrengthOmittedWithoutMeasurement(t *testing.T) {
	var b strings.Builder
	renderGateStrength(&b, nil)
	renderGateStrength(&b, &DischargeEvidence{})
	if b.Len() != 0 {
		t.Errorf("an unmeasured gate must produce no section at all, not a table of zeros:\n%s", b.String())
	}
}

func TestRenderGateStrengthReportsScoreAndSurvivors(t *testing.T) {
	var b strings.Builder
	renderGateStrength(&b, &DischargeEvidence{
		MutationScore: &MutationScore{
			Score: 0.8, Caught: 4, Survived: 1, Equivalent: 2, Invalid: 1, Total: 8,
			Timeout: "60s", MeasuredAt: "2026-09-22T00:00:00Z",
			Operators: []MutationOperatorStat{{Operator: OpCmpFlip, Caught: 4, Survived: 1}},
			Specs:     []MutationSpecStat{{Spec: "processable", ImplFunc: "Processable", Test: "TestSpec_Processable", Score: 0.8}},
			Survivors: []Mutant{{ID: "cmp-flip:x.go:7:3", Before: "<", After: "<="}},
			Gaps:      []string{"typescript is unmeasured"},
		},
		Forgery: &ForgeryEvidence{Total: 9, AsDeclared: 9, Succeeding: 1, Corpus: "forgeries"},
	})
	out := b.String()
	for _, want := range []string{
		"## Gate Strength",
		"Mutation score — 80.0%",
		"cmp-flip:x.go:7:3",
		"1 survivor(s)",
		"typescript is unmeasured",
		"### Forgery corpus",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered section is missing %q:\n%s", want, out)
		}
	}
}
