package main

// forgery.go — gate kind "forgery": run the corpus of programs that
// try to obtain a guard value without walking the proof chain, and
// check each one against the outcome it declares.
//
// The corpus started life as examples/multi-tenant-api/bypass_attempts/,
// a directory of `.go.bak` files whose outcomes were recorded in a doc
// comment and confirmed by hand. A recorded outcome is a claim about
// the trust model, and a claim nothing re-checks decays: W1 changed the
// shape of every guard type, and two of the eight attempts stopped
// compiling for a reason that had nothing to do with what they were
// demonstrating. W3 closed the same gap from the other side — attempts
// 04 and 08 named the flow gate as the thing that catches them, but
// only a human ever ran it.
//
// So the outcome moves out of the prose and into a header line the
// gate reads:
//
//	// sb-forgery: expect compile-error
//	// sb-forgery-entry: ReadForgedTenant
//	// sb-forgery-replaces: internal/derived/processable.go
//
// and `sb forgery` stages each file, runs the check its expectation
// names, and fails when any outcome differs from its declaration. A
// forgery that *succeeds* is a gate failure unless it declares
// `succeeds (documented TCB limit)` — the whole point of the corpus is
// that a new success is news.

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ForgeryHeaderPrefix introduces the expectation line.
const ForgeryHeaderPrefix = "// sb-forgery:"

// ForgeryEntryPrefix names the niladic exported function the runtime
// checks call. Required by every expectation that runs the program.
const ForgeryEntryPrefix = "// sb-forgery-entry:"

// ForgeryReplacesPrefix names the project-relative file a
// `derive-catch` forgery is staged over, replacing it for the duration
// of the check.
const ForgeryReplacesPrefix = "// sb-forgery-replaces:"

// Forgery expectations. Each one names exactly one check.
const (
	// ExpectCompileError — the target language's compiler refuses the
	// program. The strongest outcome: no binary exists to run.
	ExpectCompileError = "compile-error"

	// ExpectRuntimePanic — it compiles, and reading the forged value
	// panics. This is W1's witness (`mustBeMinted`), a runtime member
	// of the TCB.
	ExpectRuntimePanic = "runtime-panic"

	// ExpectRuntimeError — it compiles, and a generated constructor
	// refuses it by returning a non-nil error. This is a `verified`
	// premise lowered into the constructor body.
	ExpectRuntimeError = "runtime-error"

	// ExpectFlowViolation — it compiles and runs, and `sb flow` finds
	// it: the program reaches a sink with no proof on the path, or
	// references a constructor it may not.
	ExpectFlowViolation = "flow-violation"

	// ExpectGrepMissFlowCatch — the legacy regex gate passes it and
	// the flow gate fails it. The one expectation that asserts a
	// *difference* between two gates, which is why it names both.
	ExpectGrepMissFlowCatch = "grep-miss-flow-catch"

	// ExpectDeriveCatch — the forgery replaces an implementation file,
	// and the committed spec-equivalence test rejects it. This is the
	// behavioral gate, measured per-forgery rather than per-mutant.
	ExpectDeriveCatch = "derive-catch"

	// ExpectSucceeds — the forgery works. Declaring it is an
	// admission, recorded in the corpus and in the trust model, and
	// the gate holds the author to the wording so the phrase is
	// greppable.
	ExpectSucceeds = "succeeds (documented TCB limit)"
)

// forgeryExpectations is the closed set. A file naming anything else
// is a gate failure, not a skip: a typo in an expectation must not
// silently retire a corpus entry.
var forgeryExpectations = []string{
	ExpectCompileError,
	ExpectRuntimePanic,
	ExpectRuntimeError,
	ExpectFlowViolation,
	ExpectGrepMissFlowCatch,
	ExpectDeriveCatch,
	ExpectSucceeds,
}

// expectationNeedsEntry reports whether the expectation runs the
// staged program and therefore needs an entry point.
func expectationNeedsEntry(e string) bool {
	switch e {
	case ExpectRuntimePanic, ExpectRuntimeError, ExpectSucceeds:
		return true
	}
	return false
}

// Forgery is one corpus entry.
type Forgery struct {
	Path        string // path to the .go.bak, project-relative
	Name        string // basename without .go.bak
	Expect      string // one of forgeryExpectations
	Entry       string // exported niladic function, for runtime checks
	Replaces    string // project-relative file, for derive-catch
	Description string // first sentence of the leading doc comment
}

// ForgeryOutcome is what actually happened.
type ForgeryOutcome struct {
	Forgery Forgery
	Actual  string // the expectation the run matched, or a diagnostic
	Passed  bool   // Actual agrees with Forgery.Expect
	Detail  string // compiler message, panic text, violation line
	Elapsed time.Duration
}

// forgeryMarkerOutcome is the line the generated driver prints. Having
// the driver classify itself keeps the gate from having to guess at
// exit codes that a panic and an os.Exit both produce.
const forgeryMarkerOutcome = "SB-FORGERY-OUTCOME:"

func cmdForgery(args []string) {
	fs := flag.NewFlagSet("forgery", flag.ExitOnError)
	dir := fs.String("dir", "", "corpus directory (default: [forgery] dir in sb.toml, else forgeries)")
	grep := fs.String("grep", "", "legacy regex gate command, for grep-miss-flow-catch forgeries")
	markdown := fs.Bool("markdown", false, "render the corpus as a Markdown table instead of gate output")
	only := fs.String("only", "", "run just the forgeries whose name contains this substring")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb forgery — Run the forgery corpus and check each declared outcome

Usage: sb forgery [flags]

Stages every *.go.bak in the corpus directory into a scratch package
inside this module and runs the check its header declares:

  // sb-forgery: expect <expectation>

Expectations:
  compile-error          the compiler refuses the program
  runtime-panic          it compiles; reading the forged value panics
  runtime-error          it compiles; a constructor returns an error
  flow-violation         it compiles and runs; `+"`sb flow`"+` finds it
  grep-miss-flow-catch   the legacy regex passes it, `+"`sb flow`"+` fails it
  derive-catch           it replaces an impl file; the spec test rejects it
  succeeds (documented TCB limit)
                         it works, and the corpus says so on purpose

Runtime expectations also need an entry point:

  // sb-forgery-entry: SomeExportedNiladicFunc

and derive-catch needs the file it stands in for:

  // sb-forgery-replaces: internal/derived/processable.go

The gate fails when any outcome differs from its declaration. A
forgery that succeeds without declaring %q is a failure: a
new success is the finding the corpus exists to report.

Flags:
`, ExpectSucceeds)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb forgery: %v\n", err)
		os.Exit(1)
	}

	corpusDir := *dir
	if corpusDir == "" {
		corpusDir = cfg.Forgery.Dir
	}
	if corpusDir == "" {
		corpusDir = DefaultForgeryDir
	}
	grepCmd := *grep
	if grepCmd == "" {
		grepCmd = cfg.Forgery.Grep
	}
	if grepCmd == "" {
		grepCmd = flowGateRunField(cfg)
	}

	forgeries, err := LoadForgeryCorpus(corpusDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb forgery: %v\n", err)
		os.Exit(1)
	}
	if len(forgeries) == 0 {
		fmt.Fprintf(os.Stderr, "sb forgery: no *.go.bak in %s — nothing to check\n", corpusDir)
		return
	}
	if *only != "" {
		var kept []Forgery
		for _, f := range forgeries {
			if strings.Contains(f.Name, *only) {
				kept = append(kept, f)
			}
		}
		forgeries = kept
	}

	runner := &forgeryRunner{
		cfg:      cfg,
		stageDir: cfg.Forgery.Stage,
		grep:     grepCmd,
	}
	if runner.stageDir == "" {
		runner.stageDir = DefaultForgeryStage
	}
	defer runner.cleanup()

	outcomes := make([]ForgeryOutcome, 0, len(forgeries))
	for _, f := range forgeries {
		outcomes = append(outcomes, runner.run(f))
	}

	if *markdown {
		fmt.Print(RenderForgeryMarkdown(outcomes))
	}

	failed := 0
	for _, o := range outcomes {
		if !o.Passed {
			failed++
		}
		if !*markdown {
			status := "PASS"
			if !o.Passed {
				status = "FAIL"
			}
			fmt.Fprintf(os.Stderr, "%s  %-32s expect %-22s actual %s  %s\n",
				status, o.Forgery.Name, o.Forgery.Expect, o.Actual, o.Elapsed.Round(time.Millisecond))
			if !o.Passed && o.Detail != "" {
				fmt.Fprintf(os.Stderr, "      %s\n", firstLines(o.Detail, 6))
			}
		}
	}
	if !*markdown {
		fmt.Fprintf(os.Stderr, "\nsb forgery: %d/%d forgeries produced their declared outcome\n",
			len(outcomes)-failed, len(outcomes))
	}
	if err := mergeForgeryEvidence(outcomes, corpusDir); err != nil {
		fmt.Fprintf(os.Stderr, "sb forgery: warning: recording the corpus in %s: %v\n", DischargeReportPath, err)
	}
	if failed > 0 {
		os.Exit(1)
	}
}

// flowGateRunField digs the legacy grep out of the flow gate's `run`
// field, so a project that already configured it once does not have to
// repeat it under [forgery].
func flowGateRunField(cfg *Config) string {
	for _, g := range cfg.Gates {
		if g.Kind == GateKindFlow {
			return g.Run
		}
	}
	return ""
}

// LoadForgeryCorpus reads every *.go.bak in dir and parses its header.
// A file with no expectation line is an error rather than a skip: an
// unlabelled forgery is a claim nobody checks, which is the situation
// this gate exists to end.
func LoadForgeryCorpus(dir string) ([]Forgery, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("corpus directory %s does not exist", dir)
		}
		return nil, err
	}
	var out []Forgery
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go.bak") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		f, err := ParseForgeryHeader(p)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ParseForgeryHeader reads the sb-forgery directives out of path's
// leading comment block. Directives may appear anywhere in the leading
// comments; scanning stops at `package`, so a directive quoted in the
// body cannot change the file's declared outcome.
func ParseForgeryHeader(path string) (Forgery, error) {
	f := Forgery{
		Path: path,
		Name: strings.TrimSuffix(filepath.Base(path), ".go.bak"),
	}
	file, err := os.Open(path)
	if err != nil {
		return f, err
	}
	defer file.Close()

	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inDesc, descDone := false, false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			break
		}
		switch {
		case strings.HasPrefix(line, ForgeryHeaderPrefix):
			v := strings.TrimSpace(strings.TrimPrefix(line, ForgeryHeaderPrefix))
			f.Expect = strings.TrimSpace(strings.TrimPrefix(v, "expect"))
		case strings.HasPrefix(line, ForgeryEntryPrefix):
			f.Entry = strings.TrimSpace(strings.TrimPrefix(line, ForgeryEntryPrefix))
		case strings.HasPrefix(line, ForgeryReplacesPrefix):
			f.Replaces = strings.TrimSpace(strings.TrimPrefix(line, ForgeryReplacesPrefix))
		}
		// The description is the corpus entry's opening sentence,
		// which is wrapped across comment lines like the rest of the
		// file, so it is accumulated until the first period rather
		// than cut off at the first newline.
		if !descDone {
			switch {
			case inDesc && strings.HasPrefix(line, "//"):
				rest := strings.TrimSpace(strings.TrimPrefix(line, "//"))
				if rest == "" {
					descDone = true
					break
				}
				f.Description += " " + rest
			case strings.HasPrefix(line, "// Forgery"),
				strings.HasPrefix(line, "// Attempt"),
				strings.HasPrefix(line, "// Bug"):
				if i := strings.Index(line, ": "); i >= 0 {
					f.Description = strings.TrimSpace(line[i+2:])
					inDesc = true
				}
			}
			if inDesc {
				if i := strings.Index(f.Description, "."); i >= 0 {
					f.Description = f.Description[:i]
					descDone = true
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return f, err
	}

	if f.Expect == "" {
		return f, fmt.Errorf("%s: no %q line — every forgery must declare its expected outcome (one of %s)",
			path, ForgeryHeaderPrefix+" expect <outcome>", strings.Join(forgeryExpectations, ", "))
	}
	known := false
	for _, e := range forgeryExpectations {
		if f.Expect == e {
			known = true
			break
		}
	}
	if !known {
		return f, fmt.Errorf("%s: unknown expectation %q — must be one of %s",
			path, f.Expect, strings.Join(forgeryExpectations, ", "))
	}
	if expectationNeedsEntry(f.Expect) && f.Entry == "" {
		return f, fmt.Errorf("%s: expectation %q runs the program, so it needs a %q line",
			path, f.Expect, ForgeryEntryPrefix+" <ExportedNiladicFunc>")
	}
	if f.Expect == ExpectDeriveCatch && f.Replaces == "" {
		return f, fmt.Errorf("%s: expectation %q replaces an implementation file, so it needs a %q line",
			path, f.Expect, ForgeryReplacesPrefix+" <project-relative path>")
	}
	return f, nil
}

// forgeryRunner owns the scratch directories and restores the tree
// whatever happens. Every check runs inside the example module so the
// staged file can import the project's own guard package; nothing is
// ever written outside the scratch package and the one file a
// derive-catch forgery stands in for.
type forgeryRunner struct {
	cfg      *Config
	stageDir string
	grep     string

	// restore holds the original bytes of any file a derive-catch
	// forgery replaced, keyed by path.
	restore map[string][]byte
}

func (r *forgeryRunner) cleanup() {
	os.RemoveAll(r.stageDir)
	os.RemoveAll(r.driverDir())
	for p, data := range r.restore {
		os.WriteFile(p, data, 0o644)
	}
	r.restore = nil
}

func (r *forgeryRunner) driverDir() string { return r.stageDir + "_driver" }

func (r *forgeryRunner) run(f Forgery) ForgeryOutcome {
	start := time.Now()
	o := r.check(f)
	o.Forgery = f
	o.Elapsed = time.Since(start)
	o.Passed = o.Actual == f.Expect
	return o
}

func (r *forgeryRunner) check(f Forgery) ForgeryOutcome {
	if f.Expect == ExpectDeriveCatch {
		return r.checkDeriveCatch(f)
	}

	if err := r.stage(f); err != nil {
		return ForgeryOutcome{Actual: "stage-error", Detail: err.Error()}
	}
	defer os.RemoveAll(r.stageDir)

	buildOut, buildErr := runCaptured("", "go", "build", "./"+filepath.ToSlash(r.stageDir)+"/...")
	if buildErr != nil {
		return ForgeryOutcome{Actual: ExpectCompileError, Detail: string(buildOut)}
	}

	switch f.Expect {
	case ExpectFlowViolation, ExpectGrepMissFlowCatch:
		return r.checkFlow(f)
	case ExpectCompileError:
		// It compiled, so the declaration is already wrong. Say what
		// it does instead, which is the useful half of the report.
		return r.classifyRuntime(f)
	default:
		return r.classifyRuntime(f)
	}
}

// stage copies the forgery into the scratch package, renaming
// .go.bak → .go so the toolchain compiles it.
func (r *forgeryRunner) stage(f Forgery) error {
	os.RemoveAll(r.stageDir)
	if err := os.MkdirAll(r.stageDir, 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.stageDir, f.Name+".go"), data, 0o644)
}

// classifyRuntime builds a driver that calls the forgery's entry point
// and reports what the program actually does. Forgeries with no entry
// point are reported as "compiles" — which never equals an
// expectation, so a compile-error declaration on a file that compiles
// fails with a legible actual.
func (r *forgeryRunner) classifyRuntime(f Forgery) ForgeryOutcome {
	if f.Entry == "" {
		return ForgeryOutcome{Actual: "compiles", Detail: "no " + ForgeryEntryPrefix + " line, so the gate only built it"}
	}
	driver, err := r.writeDriver(f)
	if err != nil {
		return ForgeryOutcome{Actual: "driver-error", Detail: err.Error()}
	}
	defer os.RemoveAll(r.driverDir())

	out, runErr := runCaptured("", "go", "run", "./"+filepath.ToSlash(driver))
	text := string(out)
	switch {
	case strings.Contains(text, "panic:"):
		return ForgeryOutcome{Actual: ExpectRuntimePanic, Detail: text}
	case strings.Contains(text, forgeryMarkerOutcome+" error"):
		return ForgeryOutcome{Actual: ExpectRuntimeError, Detail: text}
	case strings.Contains(text, forgeryMarkerOutcome+" ok"):
		return ForgeryOutcome{Actual: ExpectSucceeds, Detail: text}
	case runErr != nil:
		return ForgeryOutcome{Actual: "run-error", Detail: text}
	default:
		return ForgeryOutcome{Actual: "no-outcome", Detail: text}
	}
}

// writeDriver generates a main package that imports the staged
// forgery and calls its entry point. The driver, not the gate,
// classifies the result: a panic reaches the gate as text, a non-nil
// error result reaches it as `error`, and anything else as `ok`. That
// keeps the three runtime expectations distinguishable without the
// gate having to interpret exit codes a panic and an os.Exit share.
func (r *forgeryRunner) writeDriver(f Forgery) (string, error) {
	modPath, err := goModulePath(".")
	if err != nil {
		return "", err
	}
	results, err := entryResultCount(filepath.Join(r.stageDir, f.Name+".go"), f.Entry)
	if err != nil {
		return "", err
	}

	var call string
	switch results {
	case 0:
		call = fmt.Sprintf("\ttarget.%s()\n\tfmt.Println(marker, \"ok\")\n", f.Entry)
	case 1:
		call = fmt.Sprintf(`	r0 := target.%s()
	if e, ok := any(r0).(error); ok && e != nil {
		fmt.Println(marker, "error", e)
		return
	}
	fmt.Println(marker, "ok", fmt.Sprint(r0))
`, f.Entry)
	case 2:
		call = fmt.Sprintf(`	r0, r1 := target.%s()
	if e, ok := any(r1).(error); ok && e != nil {
		fmt.Println(marker, "error", e)
		return
	}
	if e, ok := any(r0).(error); ok && e != nil {
		fmt.Println(marker, "error", e)
		return
	}
	fmt.Println(marker, "ok", fmt.Sprint(r0), fmt.Sprint(r1))
`, f.Entry)
	default:
		return "", fmt.Errorf("%s: entry %s returns %d results; the gate's driver supports 0, 1 or 2",
			f.Path, f.Entry, results)
	}

	src := fmt.Sprintf(`// Code generated by sb forgery. DO NOT EDIT.
package main

import (
	"fmt"

	target %q
)

const marker = %q

func main() {
%s}
`, modPath+"/"+filepath.ToSlash(r.stageDir), forgeryMarkerOutcome, call)

	dir := r.driverDir()
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
}

// checkFlow runs `sb flow` over the tree with the forgery staged, and
// for grep-miss-flow-catch also runs the legacy regex gate and
// requires it to *pass*. A forgery that both gates catch is not
// evidence that the flow gate sees more than a regex, so the gate
// refuses to record it as one.
func (r *forgeryRunner) checkFlow(f Forgery) ForgeryOutcome {
	if f.Expect == ExpectGrepMissFlowCatch {
		if r.grep == "" {
			return ForgeryOutcome{Actual: "no-grep-configured",
				Detail: "expectation " + ExpectGrepMissFlowCatch + " compares two gates, but no legacy regex command is configured; set [forgery] grep in sb.toml or give the flow gate a `run` field"}
		}
		bin, argv := SplitCommand(r.grep)
		grepOut, grepErr := runCaptured("", bin, argv...)
		if grepErr != nil {
			return ForgeryOutcome{Actual: "grep-catch",
				Detail: "the legacy regex gate caught this forgery, so it does not demonstrate a difference between the two gates:\n" + string(grepOut)}
		}
	}

	self, err := os.Executable()
	if err != nil {
		return ForgeryOutcome{Actual: "flow-error", Detail: err.Error()}
	}
	flowOut, flowErr := runCaptured("", self, "flow")
	if flowErr == nil {
		return ForgeryOutcome{Actual: "flow-clean",
			Detail: "`sb flow` reported no violation with this forgery staged:\n" + string(flowOut)}
	}
	detail := flowViolationLines(string(flowOut))
	if f.Expect == ExpectGrepMissFlowCatch {
		return ForgeryOutcome{Actual: ExpectGrepMissFlowCatch, Detail: detail}
	}
	return ForgeryOutcome{Actual: ExpectFlowViolation, Detail: detail}
}

// checkDeriveCatch stands the forgery in for a real implementation
// file and runs only the committed spec test. This is the behavioral
// gate measured one forgery at a time; `sb mutate` measures the same
// gate over a generated operator set.
func (r *forgeryRunner) checkDeriveCatch(f Forgery) ForgeryOutcome {
	target := f.Replaces
	orig, err := os.ReadFile(target)
	if err != nil {
		return ForgeryOutcome{Actual: "stage-error",
			Detail: fmt.Sprintf("cannot read the file this forgery replaces (%s): %v", target, err)}
	}
	if r.restore == nil {
		r.restore = map[string][]byte{}
	}
	r.restore[target] = orig
	defer func() {
		os.WriteFile(target, orig, 0o644)
		delete(r.restore, target)
	}()

	data, err := os.ReadFile(f.Path)
	if err != nil {
		return ForgeryOutcome{Actual: "stage-error", Detail: err.Error()}
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return ForgeryOutcome{Actual: "stage-error", Detail: err.Error()}
	}

	spec, ok := deriveSpecForImplFile(r.cfg, target)
	if !ok {
		return ForgeryOutcome{Actual: "no-derive-spec",
			Detail: fmt.Sprintf("no [[derive.specs]] entry names an impl package containing %s, so there is no committed spec test to run", target)}
	}
	out, testErr := runCaptured("", "go", "test", "-count=1",
		"-run", "^"+specTestName(spec)+"$", spec.ImplPkg)
	if testErr != nil {
		return ForgeryOutcome{Actual: ExpectDeriveCatch, Detail: string(out)}
	}
	return ForgeryOutcome{Actual: "derive-miss",
		Detail: fmt.Sprintf("%s passed with this forgery in place of %s — the committed samples do not distinguish it:\n%s",
			specTestName(spec), target, string(out))}
}

// specTestName is the test function shen-derive generates for a spec.
func specTestName(s DeriveSpec) string { return "TestSpec_" + s.ImplFunc }

// deriveSpecForImplFile finds the [[derive.specs]] entry whose impl
// package contains the given project-relative file. Matching is on the
// package's directory, which is the import path's tail.
func deriveSpecForImplFile(cfg *Config, file string) (DeriveSpec, bool) {
	dir := filepath.ToSlash(filepath.Dir(file))
	for _, s := range cfg.DeriveSpecs {
		if s.Lang != "" && s.Lang != "go" {
			continue
		}
		if strings.HasSuffix(filepath.ToSlash(s.ImplPkg), "/"+dir) ||
			filepath.ToSlash(s.ImplPkg) == dir {
			return s, true
		}
	}
	return DeriveSpec{}, false
}

// flowViolationLines keeps the FAIL blocks out of `sb flow`'s output
// and drops the PASS noise, so a failing gate's detail is the
// violation rather than the whole run.
func flowViolationLines(out string) string {
	var keep []string
	inFail := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "FAIL"):
			inFail = true
			keep = append(keep, line)
		case strings.HasPrefix(line, "PASS"), strings.HasPrefix(line, "VACUOUS"):
			inFail = false
		case inFail:
			keep = append(keep, strings.TrimSpace(line))
		}
	}
	if len(keep) == 0 {
		return strings.TrimSpace(out)
	}
	return strings.Join(keep, "\n")
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n      ")
}

// goModulePath reads the module path out of dir's go.mod.
func goModulePath(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("no module line in %s", filepath.Join(dir, "go.mod"))
}

// RenderForgeryMarkdown renders the corpus as the table
// bin/show-bypass-attempts.sh used to build by hand. The outcome
// column is now the *measured* one, so the table cannot drift from
// what the toolchain does.
func RenderForgeryMarkdown(outcomes []ForgeryOutcome) string {
	var b strings.Builder
	b.WriteString("# Forgery corpus\n\n")
	b.WriteString("Each row is a program that tries to obtain a guard value without walking the proof chain, or to skip a step of it. The expectation is declared in the file's own header (`// sb-forgery: expect …`) and the outcome column is what `sb forgery` measured when it staged the file and ran the check that expectation names — not a recorded claim.\n\n")
	b.WriteString("| # | File | Technique | Declared | Measured |\n")
	b.WriteString("|---|------|-----------|----------|----------|\n")
	for i, o := range outcomes {
		desc := o.Forgery.Description
		if desc == "" {
			desc = "(no description)"
		}
		measured := o.Actual
		if o.Passed {
			measured = "**" + measured + "**"
		} else {
			measured = "⚠️ " + measured + " (does not match)"
		}
		fmt.Fprintf(&b, "| %d | `%s.go.bak` | %s | `%s` | %s |\n",
			i+1, o.Forgery.Name, strings.ReplaceAll(desc, "|", "\\|"), o.Forgery.Expect, measured)
	}
	b.WriteString("\n**How to read this table.** `compile-error` is the strongest outcome: the compiler refuses to produce a binary. `runtime-error` is a `verified` premise lowered into a generated constructor. `runtime-panic` is W1's witness field, a runtime member of the TCB. `flow-violation` is the W3 gate over the resolved symbol graph, and `grep-miss-flow-catch` is the row that separates that gate from a regex — the legacy grep passes the file and the flow gate fails it. `derive-catch` is the behavioral gate: the forgery stands in for an implementation and the committed spec test rejects it. `succeeds (documented TCB limit)` is an admission, and it is in the corpus so that any *new* success shows up as a gate failure rather than as prose nobody re-reads.\n")
	return b.String()
}

// BuildForgeryEvidence summarises a run for the discharge report.
// Succeeding counts the forgeries that worked — the number a reader
// should look at first, because every one of them is a documented
// hole in the trust model rather than a closed one.
func BuildForgeryEvidence(outcomes []ForgeryOutcome, corpus string) *ForgeryEvidence {
	e := &ForgeryEvidence{
		Total:      len(outcomes),
		MeasuredAt: time.Now().UTC().Format(time.RFC3339),
		Corpus:     corpus,
	}
	for _, o := range outcomes {
		if o.Passed {
			e.AsDeclared++
		} else {
			e.Mismatched = append(e.Mismatched,
				fmt.Sprintf("%s: declared %s, measured %s", o.Forgery.Name, o.Forgery.Expect, o.Actual))
		}
		if o.Actual == ExpectSucceeds {
			e.Succeeding++
		}
	}
	return e
}

// mergeForgeryEvidence records the run in the discharge report's
// additive evidence block. A missing report is not an error: the
// forgery gate is useful on its own, and reporting is a bonus.
func mergeForgeryEvidence(outcomes []ForgeryOutcome, corpus string) error {
	r, err := loadDischarge(DischargeReportPath)
	if err != nil || r == nil {
		return err
	}
	if r.Evidence == nil {
		r.Evidence = &DischargeEvidence{}
	}
	r.Evidence.Forgery = BuildForgeryEvidence(outcomes, corpus)
	return writeDischarge(DischargeReportPath, r)
}

// entryResultCount parses the staged file and reports how many results
// the named entry function returns. The driver's shape depends on it,
// and reading it off the AST beats asking the author to declare it.
func entryResultCount(path, entry string) (int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return 0, err
	}
	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != entry {
			continue
		}
		if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
			return 0, fmt.Errorf("%s: entry %s takes parameters; a forgery entry point must be niladic so the gate can call it", path, entry)
		}
		if fn.Type.Results == nil {
			return 0, nil
		}
		n := 0
		for _, f := range fn.Type.Results.List {
			if len(f.Names) == 0 {
				n++
			} else {
				n += len(f.Names)
			}
		}
		return n, nil
	}
	return 0, fmt.Errorf("%s: no exported function %s to call as the entry point", path, entry)
}
