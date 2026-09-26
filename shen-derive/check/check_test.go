package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

const counterSpec = `
(define init -> [[0 0]])

\* Two counters; the second may only catch up with the first. *\
(define next
  [A B] -> [(@p "inc-a" [(+ A 1) B]) (@p "inc-b" [A (+ B 1)])] where (and (< A 3) (< B A))
  [A B] -> [(@p "inc-a" [(+ A 1) B])] where (< A 3)
  [A B] -> [(@p "inc-b" [A (+ B 1)])] where (< B A)
  [A B] -> [])

(define b-behind [A B] -> (<= B A))
(define b-small  [A B] -> (< B 3))
(define monotone [A B] [C D] -> (and (<= A C) (<= B D)))
`

func load(t *testing.T, src string, n Names) *Machine {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.shen")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	sf, err := specfile.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n.Init == "" {
		n.Init = "init"
	}
	if n.Next == "" {
		n.Next = "next"
	}
	m, err := Load(sf, n)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestExploreComplete(t *testing.T) {
	m := load(t, counterSpec, Names{Invariants: []string{"b-behind"}, StepInvariants: []string{"monotone"}})
	res, err := m.Explore(ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// States are [A B] with 0 <= B <= A <= 3: 10 of them.
	if res.Violation != nil || !res.Complete || res.States != 10 || res.Depth != 6 {
		t.Fatalf("got %+v", res)
	}
}

func TestExploreShortestCounterexample(t *testing.T) {
	m := load(t, counterSpec, Names{Invariants: []string{"b-small"}})
	res, err := m.Explore(ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	v := res.Violation
	if v == nil || v.Kind != "invariant" || v.Name != "b-small" {
		t.Fatalf("want b-small violation, got %+v", res)
	}
	// Reaching B = 3 needs A = 3 too: six steps, seven states.
	if len(v.Trace) != 7 || v.Trace[6].State.String() != "[3, 3]" {
		t.Fatalf("want a 7-state trace ending in [3, 3], got\n%s", FormatTrace(v.Trace, 0))
	}
	if v.Trace[0].Action != "" || v.Trace[1].Action == "" {
		t.Fatalf("actions not recorded:\n%s", FormatTrace(v.Trace, 0))
	}
}

func TestExploreDeadlock(t *testing.T) {
	m := load(t, counterSpec, Names{})
	res, err := m.Explore(ExploreOptions{Deadlock: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Violation == nil || res.Violation.Kind != "deadlock" {
		t.Fatalf("want deadlock at [3, 3], got %+v", res)
	}
}

func TestExploreMaxStates(t *testing.T) {
	m := load(t, counterSpec, Names{})
	res, err := m.Explore(ExploreOptions{MaxStates: 4})
	if err != nil {
		t.Fatal(err)
	}
	if res.Complete || res.Violation != nil {
		t.Fatalf("want an incomplete run, got %+v", res)
	}
}

func steps(t *testing.T, lines ...string) []Step {
	t.Helper()
	var out []Step
	for _, l := range lines {
		st, err := StepFromJSON([]byte(l))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, st)
	}
	return out
}

func TestValidateTrace(t *testing.T) {
	m := load(t, counterSpec, Names{Invariants: []string{"b-behind"}})
	cases := []struct {
		name  string
		trace []string
		opts  TraceOptions
		want  string // substring of the failure; empty for a valid trace
	}{
		{"valid with stutter", []string{`[0,0]`, `[1,0]`, `[1,0]`, `{"action":"inc-b","state":[1,1]}`}, TraceOptions{}, ""},
		{"not initial", []string{`[1,0]`, `[2,0]`}, TraceOptions{}, "not an initial state"},
		{"from any", []string{`[1,0]`, `[2,0]`}, TraceOptions{FromAny: true}, ""},
		{"skipped step", []string{`[0,0]`, `[2,0]`}, TraceOptions{}, "no action takes"},
		{"wrong label", []string{`[0,0]`, `{"action":"inc-b","state":[1,0]}`}, TraceOptions{}, `action "inc-b"`},
		{"invariant", []string{`[0,1]`}, TraceOptions{FromAny: true}, "invariant b-behind"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fail, err := m.ValidateTrace(steps(t, c.trace...), c.opts)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case c.want == "" && fail != nil:
				t.Fatalf("unexpected failure: %v", fail)
			case c.want != "" && fail == nil:
				t.Fatalf("want failure containing %q", c.want)
			case c.want != "" && !strings.Contains(fail.Error(), c.want):
				t.Fatalf("want failure containing %q, got %v", c.want, fail)
			}
		})
	}
}

func TestStepFromJSON(t *testing.T) {
	st, err := StepFromJSON([]byte(`{"action":"a","state":["x",1,2.5,true,[]]}`))
	if err != nil {
		t.Fatal(err)
	}
	if st.Action != "a" || st.State.String() != `["x", 1, 2.5, true, []]` {
		t.Fatalf("got %q %s", st.Action, st.State)
	}
}

func TestMutexExample(t *testing.T) {
	for _, c := range []struct {
		file string
		ok   bool
	}{{"mutex.shen", true}, {"mutex-racy.shen", false}} {
		sf, err := specfile.ParseFile(filepath.Join("..", "..", "examples", "tla-mutex", "specs", c.file))
		if err != nil {
			t.Fatal(err)
		}
		m, err := Load(sf, Names{Init: "init", Next: "next", Invariants: []string{"mutex"}})
		if err != nil {
			t.Fatal(err)
		}
		res, err := m.Explore(ExploreOptions{Deadlock: true})
		if err != nil {
			t.Fatal(err)
		}
		if (res.Violation == nil) != c.ok {
			t.Fatalf("%s: got %+v", c.file, res)
		}
	}
}
