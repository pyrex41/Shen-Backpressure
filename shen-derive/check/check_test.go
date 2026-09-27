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

func election(t *testing.T, consts map[string]string, n Names) *Machine {
	t.Helper()
	sf, err := specfile.ParseFile(filepath.Join("..", "..", "examples", "leader-election", "specs", "election.shen"))
	if err != nil {
		t.Fatal(err)
	}
	for name, expr := range consts {
		if err := Override(sf, name, expr); err != nil {
			t.Fatal(err)
		}
	}
	n.Init, n.Next = "init", "next"
	m, err := Load(sf, n)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// The election example reproduces the numbers in Reasonable's TLA+
// tutorial, and walks through liveness and fairness on top of it.
func TestElection(t *testing.T) {
	safety := Names{Invariants: []string{"one-leader"}}
	cases := []struct {
		name     string
		consts   map[string]string
		temporal Temporal
		states   int
		kind     string // expected violation kind; empty for none
		trace    int    // expected trace length, when kind is set
		loop     int
	}{
		{"safe", nil, Temporal{}, 38, "", 0, 0},
		{"double vote", map[string]string{"double-vote?": "true"}, Temporal{}, 190, "invariant", 7, 0},
		{"split vote", nil, Temporal{Eventually: []string{"has-leader"}}, 38, "liveness", 4, -1},
		{"no new election after a win", nil, Temporal{Possible: []string{"can-start"}}, 38, "possibility", 4, 0},
		{"timeouts livelock", map[string]string{"timeouts?": "true"},
			Temporal{Eventually: []string{"has-leader"}}, 38, "liveness", 4, 0},
		{"weak fairness is not enough", map[string]string{"timeouts?": "true"},
			Temporal{Eventually: []string{"has-leader"}, WF: []string{"* votes *"}}, 38, "liveness", 4, 0},
		{"strong fairness is", map[string]string{"timeouts?": "true"},
			Temporal{Eventually: []string{"has-leader"}, SF: []string{"* votes *"}}, 38, "", 0, 0},
		{"quorum typo passes safety", map[string]string{"timeouts?": "true", "quorum": "4"}, Temporal{}, 23, "", 0, 0},
		{"quorum typo fails liveness", map[string]string{"timeouts?": "true", "quorum": "4"},
			Temporal{Eventually: []string{"has-leader"}, SF: []string{"* votes *"}}, 23, "liveness", 4, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := election(t, c.consts, safety)
			res, err := m.Explore(ExploreOptions{Temporal: c.temporal})
			if err != nil {
				t.Fatal(err)
			}
			if res.States != c.states {
				t.Errorf("states: got %d, want %d", res.States, c.states)
			}
			v := res.Violation
			switch {
			case c.kind == "" && v != nil:
				t.Fatalf("unexpected %s\n%s", v.Error(), FormatViolation(v))
			case c.kind == "":
			case v == nil:
				t.Fatalf("want a %s violation", c.kind)
			case v.Kind != c.kind || len(v.Trace) != c.trace || (c.kind == "liveness" && v.Loop != c.loop):
				t.Fatalf("want %s with %d states (loop %d), got %s with %d (loop %d)\n%s",
					c.kind, c.trace, c.loop, v.Kind, len(v.Trace), v.Loop, FormatViolation(v))
			}
		})
	}
}

const ringSpec = `
\* A token moves round a ring of three; at 1 it may "skip" back to 0
   instead of advancing to 2. *\
(define init -> [0])
(define next
  0 -> [(@p "step" 1)]
  1 -> [(@p "advance" 2) (@p "skip" 0)]
  2 -> [(@p "step" 0)])
(define at-two S -> (= S 2))
(define at-one S -> (= S 1))
`

func TestLivenessFairness(t *testing.T) {
	m := load(t, ringSpec, Names{})
	for _, c := range []struct {
		name string
		tmp  Temporal
		ok   bool
	}{
		// 0 -> 1 -> 0 ... skips 2 forever.
		{"unfair", Temporal{Eventually: []string{"at-two"}}, false},
		// "advance" is disabled at 0, which the skip loop visits, so WF
		// is met without ever advancing.
		{"wf does not help", Temporal{Eventually: []string{"at-two"}, WF: []string{"advance"}}, false},
		// "advance" is enabled at every visit to 1, so SF forces it.
		{"sf does", Temporal{Eventually: []string{"at-two"}, SF: []string{"advance"}}, true},
		{"leads-to holds", Temporal{LeadsTo: []LeadsTo{{"at-two", "at-one"}}}, true},
		{"possible", Temporal{Possible: []string{"at-two"}}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, err := m.Explore(ExploreOptions{Temporal: c.tmp})
			if err != nil {
				t.Fatal(err)
			}
			if (res.Violation == nil) != c.ok {
				t.Fatalf("got %+v", res.Violation)
			}
			if v := res.Violation; v != nil && (v.Kind != "liveness" || v.Loop < 0) {
				t.Fatalf("want a liveness lasso, got\n%s", FormatViolation(v))
			}
		})
	}
}

func TestOverrideRejectsFunctions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.shen")
	os.WriteFile(path, []byte(counterSpec), 0o644)
	sf, err := specfile.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Override(sf, "next", "[]"); err == nil {
		t.Fatal("overriding a function should fail")
	}
	if err := Override(sf, "init", "[[5 5]]"); err != nil {
		t.Fatal(err)
	}
}
