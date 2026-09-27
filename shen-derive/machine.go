package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/check"
	"github.com/pyrex41/Shen-Backpressure/shen-derive/specfile"
)

// consts holds the --const overrides collected by machineFlags.
var consts [][2]string

// machineFlags registers the flags shared by check and trace, which name
// the defines that make a spec a state machine.
func machineFlags(fs *flag.FlagSet) func() check.Names {
	initName := fs.String("init", "init", "nullary define returning the list of initial states")
	nextName := fs.String("next", "next", "define taking a state to its list of successors")
	inv := fs.String("inv", "", "comma-separated state invariants (state --> boolean)")
	stepInv := fs.String("step-inv", "", "comma-separated step invariants (state --> state --> boolean)")
	fs.Func("const", "`name=expr` replaces a constant (nullary define), like a TLC model's CONSTANTS; repeatable", func(v string) error {
		name, expr, ok := strings.Cut(v, "=")
		if !ok {
			return fmt.Errorf("want name=expr")
		}
		consts = append(consts, [2]string{strings.TrimSpace(name), expr})
		return nil
	})
	return func() check.Names {
		return check.Names{
			Init:           *initName,
			Next:           *nextName,
			Invariants:     splitList(*inv),
			StepInvariants: splitList(*stepInv),
		}
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadMachine parses args[0] as a spec and the remaining args as flags.
func loadMachine(fs *flag.FlagSet, names func() check.Names, args []string) *check.Machine {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fs.Usage()
		os.Exit(2)
	}
	fs.Parse(args[1:])
	sf, err := specfile.ParseFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(2)
	}
	for _, c := range consts {
		if err := check.Override(sf, c[0], c[1]); err != nil {
			fmt.Fprintf(os.Stderr, "--const: %v\n", err)
			os.Exit(2)
		}
	}
	m, err := check.Load(sf, names())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", args[0], err)
		os.Exit(2)
	}
	return m
}

// cmdCheck explores every reachable state of the spec, TLC style.
func cmdCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	names := machineFlags(fs)
	maxStates := fs.Int("max-states", 1000000, "stop after this many distinct states (0 = unbounded)")
	noDeadlock := fs.Bool("no-deadlock", false, "do not report states without successors")
	eventually := fs.String("eventually", "", "comma-separated P: every behaviour reaches a P state (<>P)")
	leadsTo := fs.String("leads-to", "", "comma-separated P:Q: every P state is followed by a Q state (P ~> Q)")
	possible := fs.String("possible", "", "comma-separated P: from every reachable state, P can still be reached (CTL AG EF P)")
	wf := fs.String("wf", "", "comma-separated action labels given weak fairness (a trailing * matches a prefix)")
	sf := fs.String("sf", "", "comma-separated action labels given strong fairness (a trailing * matches a prefix)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: shen-derive check <spec.shen> [flags]")
		fs.PrintDefaults()
	}
	m := loadMachine(fs, names, args)

	temporal := check.Temporal{
		Eventually: splitList(*eventually),
		Possible:   splitList(*possible),
		WF:         splitList(*wf),
		SF:         splitList(*sf),
	}
	for _, pq := range splitList(*leadsTo) {
		p, q, ok := strings.Cut(pq, ":")
		if !ok {
			fmt.Fprintf(os.Stderr, "--leads-to %q: want P:Q\n", pq)
			os.Exit(2)
		}
		temporal.LeadsTo = append(temporal.LeadsTo, check.LeadsTo{P: p, Q: q})
	}

	res, err := m.Explore(check.ExploreOptions{MaxStates: *maxStates, Deadlock: !*noDeadlock, Temporal: temporal})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("%d distinct states, %d transitions, depth %d\n", res.States, res.Transitions, res.Depth)
	if v := res.Violation; v != nil {
		fmt.Printf("FAIL: %s\n%s", v.Error(), check.FormatViolation(v))
		os.Exit(1)
	}
	if !res.Complete {
		fmt.Printf("INCOMPLETE: stopped at --max-states %d; no safety violation in the states seen, temporal properties not checked\n", *maxStates)
		os.Exit(3)
	}
	fmt.Println("OK: every reachable state satisfies the invariants and every temporal property holds")
}

// cmdTrace validates a trace recorded from an implementation.
func cmdTrace(args []string) {
	fs := flag.NewFlagSet("trace", flag.ExitOnError)
	names := machineFlags(fs)
	tracePath := fs.String("trace", "", "(required) JSONL trace: one state, or {\"action\":..,\"state\":..}, per line")
	fromAny := fs.Bool("from-any", false, "accept a trace that does not start in an initial state")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: shen-derive trace <spec.shen> --trace FILE.jsonl [flags]")
		fs.PrintDefaults()
	}
	m := loadMachine(fs, names, args)
	if *tracePath == "" {
		fs.Usage()
		os.Exit(2)
	}

	data, err := os.ReadFile(*tracePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	var tr []check.Step
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		st, err := check.StepFromJSON(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s:%d: %v\n", *tracePath, n, err)
			os.Exit(2)
		}
		tr = append(tr, st)
	}

	fail, err := m.ValidateTrace(tr, check.TraceOptions{FromAny: *fromAny})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if fail != nil {
		fmt.Printf("FAIL: %s\n", fail.Error())
		lo := max(0, fail.Index-3)
		fmt.Printf("recorded:\n%s", check.FormatTrace(tr[lo:fail.Index+1], lo))
		if len(fail.Allowed) > 0 {
			fmt.Println("spec allowed:")
			for _, s := range fail.Allowed {
				fmt.Printf("       %-14s %s\n", s.Action, s.State)
			}
		}
		os.Exit(1)
	}
	fmt.Printf("OK: %d-step trace is a behaviour of the spec\n", len(tr))
}
