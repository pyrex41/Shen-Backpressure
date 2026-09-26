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

// machineFlags registers the flags shared by check and trace, which name
// the defines that make a spec a state machine.
func machineFlags(fs *flag.FlagSet) func() check.Names {
	initName := fs.String("init", "init", "nullary define returning the list of initial states")
	nextName := fs.String("next", "next", "define taking a state to its list of successors")
	inv := fs.String("inv", "", "comma-separated state invariants (state --> boolean)")
	stepInv := fs.String("step-inv", "", "comma-separated step invariants (state --> state --> boolean)")
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
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: shen-derive check <spec.shen> [flags]")
		fs.PrintDefaults()
	}
	m := loadMachine(fs, names, args)

	res, err := m.Explore(check.ExploreOptions{MaxStates: *maxStates, Deadlock: !*noDeadlock})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("%d distinct states, %d transitions, depth %d\n", res.States, res.Transitions, res.Depth)
	if v := res.Violation; v != nil {
		fmt.Printf("FAIL: %s\n%s", v.Error(), check.FormatTrace(v.Trace, 0))
		os.Exit(1)
	}
	if !res.Complete {
		fmt.Printf("INCOMPLETE: stopped at --max-states %d; no violation in the states seen\n", *maxStates)
		os.Exit(3)
	}
	fmt.Println("OK: every reachable state satisfies the invariants")
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
