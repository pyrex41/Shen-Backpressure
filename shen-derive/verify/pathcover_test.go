package verify

import (
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/symbolic"
)

const pathCoverSpec = `
(datatype account-id
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

(define processable
  {amount --> (list transaction) --> boolean}
  B0 Txs -> (foldr (lambda X (lambda Acc (and (>= (val X) 0) Acc)))
              true
              (scanl (lambda B (lambda Tx (- (val B) (val (amount Tx)))))
                     (val B0)
                     Txs)))
`

// stubSolver answers every query "sat" with a fixed assignment, which
// is enough to exercise the harness wiring (sample construction, Go
// literal emission, header rendering) without a solver binary.
type stubSolver struct {
	status symbolic.Status
}

func (s *stubSolver) Name() string { return "stub" }

func (s *stubSolver) Check(q *symbolic.Query) (*symbolic.SolverResult, error) {
	if s.status == symbolic.StatusUnsat {
		return &symbolic.SolverResult{Status: symbolic.StatusUnsat}, nil
	}
	model := map[string]string{}
	for _, v := range q.Vars {
		switch v.S {
		case symbolic.SortReal:
			model[v.Name] = "0.0"
		case symbolic.SortString:
			model[v.Name] = `"a"`
		default:
			model[v.Name] = "false"
		}
	}
	return &symbolic.SolverResult{Status: symbolic.StatusSat, Model: model}, nil
}

func pathCoverConfig(t *testing.T, solver symbolic.Solver, pathCover bool) *HarnessConfig {
	t.Helper()
	tmp := t_tempFile(pathCoverSpec)
	t.Cleanup(func() { t_removeFile(tmp) })
	sf, err := specfile.ParseFile(tmp)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	def := sf.FindDefine("processable")
	if def == nil {
		t.Fatal("processable not found")
	}
	all := make([]*specfile.Define, len(sf.Defines))
	for i := range sf.Defines {
		all[i] = &sf.Defines[i]
	}
	return &HarnessConfig{
		Spec:        def,
		TypeTable:   specfile.BuildTypeTable(sf.Datatypes, "example.com/guards", "shenguard"),
		AllDefines:  all,
		ImplPkgPath: "example.com/derived",
		ImplPkgName: "derived",
		ImplFunc:    "Processable",
		TestPkgName: "derived_test",
		PathCover:   pathCover,
		PathDepth:   2,
		PathSolver:  solver,
	}
}

func TestPathCoverOffIsUnchanged(t *testing.T) {
	cfg := pathCoverConfig(t, nil, false)
	h, err := BuildHarness(cfg)
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if h.PathStats != nil {
		t.Fatal("PathStats must be nil when PathCover is off")
	}
	src, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if strings.Contains(src, "Path cover") {
		t.Fatal("header must not mention path cover when the feature is off")
	}
	if strings.Contains(src, "provenance") {
		t.Fatal("pool cases must not carry a provenance comment")
	}
	for _, c := range h.Cases {
		if !strings.HasPrefix(c.Name, "case_") {
			t.Fatalf("unexpected case name %q", c.Name)
		}
	}
}

func TestPathCoverAddsThirdSampleSource(t *testing.T) {
	poolOnly, err := BuildHarness(pathCoverConfig(t, nil, false))
	if err != nil {
		t.Fatalf("BuildHarness (pool): %v", err)
	}

	cfg := pathCoverConfig(t, &stubSolver{status: symbolic.StatusSat}, true)
	h, err := BuildHarness(cfg)
	if err != nil {
		t.Fatalf("BuildHarness (path): %v", err)
	}
	if h.PathStats == nil {
		t.Fatal("PathStats must be populated")
	}
	if h.PathStats.Total == 0 {
		t.Fatal("expected enumerated paths")
	}
	if !h.PathStats.SolverAvailable || h.PathStats.SolverName != "stub" {
		t.Fatalf("solver not recorded: %+v", h.PathStats)
	}
	if len(h.Cases) <= len(poolOnly.Cases) {
		t.Fatalf("path cover added no cases (%d vs %d)", len(h.Cases), len(poolOnly.Cases))
	}
	// Pool cases are unchanged and come first.
	for i := range poolOnly.Cases {
		if h.Cases[i].Name != poolOnly.Cases[i].Name || h.Cases[i].ExpectedGo != poolOnly.Cases[i].ExpectedGo {
			t.Fatalf("pool case %d changed: %+v vs %+v", i, h.Cases[i], poolOnly.Cases[i])
		}
	}
	nPath := 0
	for _, c := range h.Cases[len(poolOnly.Cases):] {
		if !strings.HasPrefix(c.Provenance, "path:") {
			t.Fatalf("case %q missing path provenance", c.Name)
		}
		if !strings.HasPrefix(c.Name, "path_") {
			t.Fatalf("path case name %q should start with path_", c.Name)
		}
		nPath++
	}
	if nPath != h.PathStats.SamplesAdded {
		t.Fatalf("%d path cases but SamplesAdded=%d", nPath, h.PathStats.SamplesAdded)
	}

	src, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	for _, want := range []string{
		"// Path cover: paths_total=",
		"paths_feasible=",
		"paths_dead=",
		"// Path cover solver: stub",
		"// provenance: path:",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("emitted source missing %q:\n%s", want, firstLines(src, 20))
		}
	}
	// Every path witness must be built through the guard constructors,
	// same as the pool's samples.
	if !strings.Contains(src, "mustAmount(") || !strings.Contains(src, "mustTransaction(") {
		t.Fatalf("path samples should use the mustXxx helpers:\n%s", src)
	}
}

func TestPathCoverDegradesWithoutSolver(t *testing.T) {
	// A solver that answers unsat for everything stands in for "no
	// feasible witness"; the harness must still emit the pool cases and
	// report the counters honestly.
	cfg := pathCoverConfig(t, &stubSolver{status: symbolic.StatusUnsat}, true)
	h, err := BuildHarness(cfg)
	if err != nil {
		t.Fatalf("BuildHarness: %v", err)
	}
	if h.PathStats.Feasible != 0 {
		t.Fatalf("expected no feasible paths, got %d", h.PathStats.Feasible)
	}
	if h.PathStats.Dead != h.PathStats.Total {
		t.Fatalf("expected every path dead, got %d/%d", h.PathStats.Dead, h.PathStats.Total)
	}
	if len(h.Cases) == 0 {
		t.Fatal("the boundary pool must still supply cases")
	}
	src, err := h.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if !strings.Contains(src, "Path cover degraded:") {
		t.Fatalf("header should explain the degradation:\n%s", firstLines(src, 15))
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
