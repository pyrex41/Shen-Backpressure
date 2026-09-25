package main

// mutate.go — `sb mutate-spec`: premise-mutation gate for Shen specs.
//
// A `(...) : verified` premise earns its keep only if some hostile program
// is rejected *because of it*. For every verified premise in the spec,
// mutate-spec drops that one premise, re-typechecks the mutant spec with
// (tc +) in a fresh Shen process, and loads every hostile file against it.
// The premise is KILLED when at least one hostile file now typechecks, and
// SURVIVES otherwise — i.e. no hostile program witnesses why the premise
// is there, so either the premise is redundant or the hostile corpus has
// a hole. The gate fails listing every surviving premise.
//
// Baseline sanity first: under the unmutated spec every hostile file must
// be REJECTED and every good file ACCEPTED.
//
// The check is language-agnostic: it only needs a Shen runtime that can
// run a script (shen-erl, shen-sbcl, shen-scheme, ShenScript, ...). A fresh
// process per mutant is required because Shen datatypes accumulate in the
// global environment.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// MutationReportPath is the sidecar evidence file mutate-spec writes.
const MutationReportPath = ".sb/mutation_report.json"

// verifiedIfPrelude lets `(if P X Y)` introduce `P : verified` in the
// then-branch. Without it, guarded constructions in good files cannot
// typecheck and the baseline would be meaningless.
const verifiedIfPrelude = `\* sb mutate-spec prelude: the if-introduction rule for verified facts. *\
(datatype verified-if
  P : boolean;
  P : verified >> X : A;
  Y : A;
  _______________________
  (if P X Y) : A;)
`

// MutateConfig mirrors [mutate] in sb.toml.
type MutateConfig struct {
	Shen    string   // Shen binary (default: $SB_SHEN, then shen-erl, shen-sbcl, shen on PATH)
	Args    []string // argv template; "{file}" is replaced by the driver path
	Prelude []string // files loaded before (tc +); empty = embedded verified-if prelude
	Hostile []string // globs of hostile .shen files (must be rejected)
	Good    []string // globs of good .shen files (must be accepted)
	Isolate string   // "mutant" (one process per mutant) or "hostile" (one per mutant×file)
	Timeout string   // per-process timeout (default 120s)
	Jobs    int      // parallel Shen processes (default NumCPU/2)

	MaxInferences int // per-file inference budget passed to (maxinferences N); 0 = runtime default
}

// Enabled reports whether [mutate] configures any hostile corpus.
func (m MutateConfig) Enabled() bool { return len(m.Hostile) > 0 }

type premiseSite struct {
	Datatype string
	Index    int // 0-based index among the block's verified premises
	Expr     string
	Line     int
	start    int
	end      int
}

type mutationResult struct {
	Datatype string   `json:"datatype"`
	Premise  string   `json:"premise"`
	Line     int      `json:"line"`
	Status   string   `json:"status"` // killed | survived | error
	KilledBy []string `json:"killed_by,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type mutationReport struct {
	SchemaVersion int              `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	Spec          string           `json:"spec"`
	ShenRuntime   string           `json:"shen_runtime"`
	Hostile       []string         `json:"hostile"`
	Good          []string         `json:"good"`
	Baseline      []fileVerdict    `json:"baseline"`
	Premises      []mutationResult `json:"premises"`
	Killed        int              `json:"killed"`
	Survived      int              `json:"survived"`
	Errors        int              `json:"errors"`
}

type fileVerdict struct {
	File     string `json:"file"`
	Expected string `json:"expected"`
	Got      string `json:"got"`
}

func cmdMutateSpec(args []string) {
	fs := flag.NewFlagSet("mutate-spec", flag.ExitOnError)
	spec := fs.String("spec", "", "spec file (default: [paths] spec)")
	shen := fs.String("shen", "", "Shen binary (default: [mutate] shen, $SB_SHEN, shen-erl, shen-sbcl)")
	shenArgs := fs.String("args", "", "argv template for the Shen binary, space separated, {file} = driver (default by runtime)")
	hostile := fs.String("hostile", "", "comma-separated hostile globs (default: [mutate] hostile)")
	good := fs.String("good", "", "comma-separated good globs (default: [mutate] good)")
	prelude := fs.String("prelude", "", "comma-separated prelude files (default: [mutate] prelude or the built-in verified-if rule)")
	isolate := fs.String("isolate", "", "mutant | hostile (default: mutant; hostile for shen-sbcl)")
	jobs := fs.Int("jobs", 0, "parallel Shen processes")
	timeout := fs.String("timeout", "", "per-process timeout (default 120s)")
	maxInf := fs.Int("max-inferences", 0, "Shen inference budget per loaded file (default: runtime default)")
	jsonOut := fs.String("json", MutationReportPath, "write the mutation report here (empty to skip)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb mutate-spec — premise-mutation gate

Usage: sb mutate-spec [flags]

For every "(...) : verified" premise, drops it, re-typechecks the spec with
(tc +) in a fresh Shen process, and loads the hostile corpus. A premise is
killed when some hostile file starts typechecking; surviving premises have
no witness and fail the gate. Configure defaults under [mutate] in sb.toml.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb mutate-spec: %v\n", err)
		os.Exit(1)
	}
	mc := cfg.Mutate
	if *shen != "" {
		mc.Shen = *shen
	}
	if *shenArgs != "" {
		mc.Args = strings.Fields(*shenArgs)
	}
	if *hostile != "" {
		mc.Hostile = splitList(*hostile)
	}
	if *good != "" {
		mc.Good = splitList(*good)
	}
	if *prelude != "" {
		mc.Prelude = splitList(*prelude)
	}
	if *isolate != "" {
		mc.Isolate = *isolate
	}
	if *jobs > 0 {
		mc.Jobs = *jobs
	}
	if *timeout != "" {
		mc.Timeout = *timeout
	}
	if *maxInf > 0 {
		mc.MaxInferences = *maxInf
	}
	specPath := cfg.Spec
	if *spec != "" {
		specPath = *spec
	}

	rep, err := runMutateSpec(specPath, mc, os.Stderr)
	if rep != nil && *jsonOut != "" {
		if werr := writeJSONFile(*jsonOut, rep); werr != nil {
			fmt.Fprintf(os.Stderr, "sb mutate-spec: writing %s: %v\n", *jsonOut, werr)
		}
		if n, perr := annotateDischargeWithMutation(DischargeReportPath, rep); perr == nil && n > 0 {
			fmt.Fprintf(os.Stderr, "sb mutate-spec: attached mutation evidence to %d premise(s) in %s\n", n, DischargeReportPath)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb mutate-spec: %v\n", err)
		os.Exit(1)
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

// resolveShen picks the Shen binary and its argv template.
func resolveShen(mc MutateConfig) (bin string, argv []string, isolate string, err error) {
	bin = mc.Shen
	if bin == "" {
		bin = os.Getenv("SB_SHEN")
	}
	if bin == "" && os.Getenv("SHEN_ERL_ROOT") != "" {
		if p := filepath.Join(os.Getenv("SHEN_ERL_ROOT"), "bin", "shen-erl"); fileExists(p) {
			bin = p
		}
	}
	if bin == "" {
		for _, cand := range []string{"shen-erl", "shen-sbcl", "shen-scheme", "shen"} {
			if p, lerr := exec.LookPath(cand); lerr == nil {
				bin = p
				break
			}
		}
	}
	if bin == "" {
		return "", nil, "", errors.New("no Shen runtime found: set [mutate] shen, --shen, $SB_SHEN or $SHEN_ERL_ROOT (shen-erl, shen-sbcl, ShenScript, ...)")
	}
	if strings.HasPrefix(bin, "~/") {
		if home, herr := os.UserHomeDir(); herr == nil {
			bin = filepath.Join(home, bin[2:])
		}
	}
	base := filepath.Base(bin)
	argv = mc.Args
	isolate = mc.Isolate
	if len(argv) == 0 {
		switch {
		case strings.Contains(base, "sbcl"):
			// shen-cl: `-l file` loads and exits; `script` is not supported.
			argv = []string{"-l", "{file}"}
		default:
			// The portable launcher shared by the maintained ports
			// (shen-erl, shen-scheme, ShenScript's CLI, ...).
			argv = []string{"script", "{file}"}
		}
	}
	if isolate == "" {
		isolate = "mutant"
		if strings.Contains(base, "sbcl") {
			// shen-cl aborts the rest of a script after a trapped type error.
			isolate = "hostile"
		}
	}
	if isolate != "mutant" && isolate != "hostile" {
		return "", nil, "", fmt.Errorf("isolate must be mutant or hostile, got %q", isolate)
	}
	return bin, argv, isolate, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func expandGlobs(globs []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, g := range globs {
		matches, err := filepath.Glob(g)
		if err != nil {
			return nil, fmt.Errorf("bad glob %q: %w", g, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("glob %q matched no files", g)
		}
		sort.Strings(matches)
		for _, m := range matches {
			abs, _ := filepath.Abs(m)
			if !seen[abs] {
				seen[abs] = true
				out = append(out, abs)
			}
		}
	}
	return out, nil
}

// runMutateSpec executes the gate and returns the report. A non-nil error
// means the gate failed (baseline violation or surviving premises).
func runMutateSpec(specPath string, mc MutateConfig, log *os.File) (*mutationReport, error) {
	if !mc.Enabled() {
		return nil, errors.New("no hostile corpus configured ([mutate] hostile = [...] or --hostile)")
	}
	bin, argv, isolate, err := resolveShen(mc)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	hostiles, err := expandGlobs(mc.Hostile)
	if err != nil {
		return nil, err
	}
	var goods []string
	if len(mc.Good) > 0 {
		if goods, err = expandGlobs(mc.Good); err != nil {
			return nil, err
		}
	}
	timeout := 120 * time.Second
	if mc.Timeout != "" {
		if timeout, err = time.ParseDuration(mc.Timeout); err != nil {
			return nil, fmt.Errorf("bad timeout %q: %w", mc.Timeout, err)
		}
	}
	jobs := mc.Jobs
	if jobs <= 0 {
		jobs = runtime.NumCPU() / 2
		if jobs < 1 {
			jobs = 1
		}
	}

	work, err := os.MkdirTemp("", "sb-mutate-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	var preludes []string
	if len(mc.Prelude) == 0 {
		p := filepath.Join(work, "verified-if.shen")
		if err := os.WriteFile(p, []byte(verifiedIfPrelude), 0o644); err != nil {
			return nil, err
		}
		preludes = []string{p}
	} else {
		for _, p := range mc.Prelude {
			abs, _ := filepath.Abs(p)
			preludes = append(preludes, abs)
		}
	}

	r := &shenRunner{bin: bin, argv: argv, isolate: isolate, preludes: preludes, timeout: timeout, work: work,
		maxInferences: mc.MaxInferences}
	rep := &mutationReport{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Spec:          specPath,
		ShenRuntime:   bin,
	}
	for _, h := range hostiles {
		rep.Hostile = append(rep.Hostile, relPath(h))
	}
	for _, g := range goods {
		rep.Good = append(rep.Good, relPath(g))
	}

	sites := findVerifiedPremises(string(src))
	fmt.Fprintf(log, "sb mutate-spec: %s — %d verified premise(s), %d hostile, %d good file(s); runtime %s (isolate=%s, jobs=%d)\n",
		specPath, len(sites), len(hostiles), len(goods), filepath.Base(bin), isolate, jobs)

	// Baseline.
	all := append(append([]string{}, hostiles...), goods...)
	verdicts, err := r.run(string(src), all, "baseline")
	if err != nil {
		return rep, fmt.Errorf("baseline: %w", err)
	}
	var baseErrs []string
	for i, f := range all {
		want := "rejected"
		if i >= len(hostiles) {
			want = "accepted"
		}
		rep.Baseline = append(rep.Baseline, fileVerdict{File: relPath(f), Expected: want, Got: verdicts[i]})
		if verdicts[i] != want {
			baseErrs = append(baseErrs, fmt.Sprintf("%s is %s by the unmutated spec (expected %s)", relPath(f), verdicts[i], want))
		}
	}
	if len(baseErrs) > 0 {
		return rep, fmt.Errorf("baseline failed:\n  %s", strings.Join(baseErrs, "\n  "))
	}
	fmt.Fprintf(log, "  baseline: %d hostile rejected, %d good accepted\n", len(hostiles), len(goods))

	// Mutants.
	results := make([]mutationResult, len(sites))
	var wg sync.WaitGroup
	sem := make(chan struct{}, jobs)
	for i, s := range sites {
		wg.Add(1)
		go func(i int, s premiseSite) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := mutationResult{Datatype: s.Datatype, Premise: s.Expr, Line: s.Line}
			mutant := blankSpan(string(src), s.start, s.end)
			v, err := r.run(mutant, hostiles, fmt.Sprintf("m%03d", i))
			switch {
			case err != nil:
				res.Status, res.Error = "error", err.Error()
			default:
				for j, h := range hostiles {
					if v[j] == "accepted" {
						res.KilledBy = append(res.KilledBy, relPath(h))
					}
				}
				res.Status = "survived"
				if len(res.KilledBy) > 0 {
					res.Status = "killed"
				}
			}
			results[i] = res
		}(i, s)
	}
	wg.Wait()
	rep.Premises = results

	var survivors []string
	for _, res := range results {
		switch res.Status {
		case "killed":
			rep.Killed++
			fmt.Fprintf(log, "  KILLED    %s:%d %s  (by %s)\n", res.Datatype, res.Line, res.Premise, strings.Join(res.KilledBy, ", "))
		case "survived":
			rep.Survived++
			survivors = append(survivors, fmt.Sprintf("%s (datatype %s, %s:%d)", res.Premise, res.Datatype, specPath, res.Line))
			fmt.Fprintf(log, "  SURVIVED  %s:%d %s  (no hostile file typechecks without it)\n", res.Datatype, res.Line, res.Premise)
		default:
			rep.Errors++
			survivors = append(survivors, fmt.Sprintf("%s (datatype %s): %s", res.Premise, res.Datatype, res.Error))
			fmt.Fprintf(log, "  ERROR     %s:%d %s  %s\n", res.Datatype, res.Line, res.Premise, res.Error)
		}
	}
	fmt.Fprintf(log, "sb mutate-spec: %d killed, %d survived, %d error(s)\n", rep.Killed, rep.Survived, rep.Errors)
	if len(survivors) > 0 {
		return rep, fmt.Errorf("%d premise(s) without a hostile witness:\n  %s\nAdd a hostile .shen file that only typechecks without each premise, or delete the premise",
			len(survivors), strings.Join(survivors, "\n  "))
	}
	return rep, nil
}

func relPath(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return p
}

// ----------------------------------------------------------------------------
// Shen process driver
// ----------------------------------------------------------------------------

type shenRunner struct {
	bin      string
	argv     []string
	isolate  string
	preludes []string
	timeout  time.Duration
	work     string

	maxInferences int
}

var resultLine = regexp.MustCompile(`SB-MUTATE-RESULT (\d+) (accepted|rejected)`)

// run typechecks spec (with tc +) and reports, per file, whether loading it
// afterwards was accepted or rejected.
func (r *shenRunner) run(spec string, files []string, tag string) ([]string, error) {
	dir := filepath.Join(r.work, tag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	specFile := filepath.Join(dir, "spec.shen")
	if err := os.WriteFile(specFile, []byte(spec), 0o644); err != nil {
		return nil, err
	}
	out := make([]string, len(files))
	if r.isolate == "hostile" {
		for i := range files {
			v, err := r.runDriver(dir, fmt.Sprintf("driver-%d.shen", i), specFile, files, []int{i})
			if err != nil {
				return nil, err
			}
			out[i] = v[i]
		}
		return out, nil
	}
	idx := make([]int, len(files))
	for i := range idx {
		idx[i] = i
	}
	v, err := r.runDriver(dir, "driver.shen", specFile, files, idx)
	if err != nil {
		return nil, err
	}
	copy(out, v)
	return out, nil
}

func shenString(s string) string { return `"` + strings.ReplaceAll(s, `"`, `c#34;`) + `"` }

func (r *shenRunner) runDriver(dir, name, specFile string, files []string, which []int) ([]string, error) {
	var d strings.Builder
	for _, p := range r.preludes {
		fmt.Fprintf(&d, "(load %s)\n", shenString(p))
	}
	if r.maxInferences > 0 {
		fmt.Fprintf(&d, "(maxinferences %d)\n", r.maxInferences)
	}
	d.WriteString("(tc +)\n")
	fmt.Fprintf(&d, "(load %s)\n", shenString(specFile))
	d.WriteString("(output \"~%SB-MUTATE-SPEC-LOADED~%\")\n")
	for _, i := range which {
		// The inference counter is cumulative within a process and is not
		// reset when a load fails; a hostile file that exhausts the budget
		// would otherwise poison every later verdict. Reset it per file.
		d.WriteString("(trap-error (set shen.*infs* 0) (/. E skip))\n")
		fmt.Fprintf(&d, "(output \"~%%SB-MUTATE-RESULT %d ~A~%%\" (trap-error (do (load %s) accepted) (/. E rejected)))\n", i, shenString(files[i]))
	}
	driver := filepath.Join(dir, name)
	if err := os.WriteFile(driver, []byte(d.String()), 0o644); err != nil {
		return nil, err
	}
	args := make([]string, len(r.argv))
	for i, a := range r.argv {
		args[i] = strings.ReplaceAll(a, "{file}", driver)
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Dir = dir // crash dumps etc. stay in the scratch dir
	cmd.Stdin = nil
	outb, runErr := cmd.CombinedOutput()
	text := string(outb)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("shen timed out after %s", r.timeout)
	}
	if !strings.Contains(text, "SB-MUTATE-SPEC-LOADED") {
		return nil, fmt.Errorf("spec did not load under (tc +): %s", lastLines(text, 6))
	}
	got := make([]string, len(files))
	for _, m := range resultLine.FindAllStringSubmatch(text, -1) {
		var i int
		fmt.Sscanf(m[1], "%d", &i)
		if i >= 0 && i < len(got) {
			got[i] = m[2]
		}
	}
	for _, i := range which {
		if got[i] == "" {
			msg := "no verdict"
			if runErr != nil {
				msg += " (" + runErr.Error() + ")"
			}
			return nil, fmt.Errorf("%s for %s: %s", msg, relPath(files[i]), lastLines(text, 6))
		}
	}
	return got, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// ----------------------------------------------------------------------------
// Premise discovery
// ----------------------------------------------------------------------------

// blankComments replaces Shen comments with spaces, preserving offsets.
func blankComments(s string) string {
	b := []byte(s)
	inStr := false
	for i := 0; i < len(b); i++ {
		switch {
		case inStr:
			if b[i] == '"' {
				inStr = false
			}
		case b[i] == '"':
			inStr = true
		case b[i] == '\\' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(s[i+2:], `*\`)
			stop := len(b)
			if end >= 0 {
				stop = i + 2 + end + 2
			}
			for j := i; j < stop; j++ {
				if b[j] != '\n' {
					b[j] = ' '
				}
			}
			i = stop - 1
		case b[i] == '\\' && i+1 < len(b) && b[i+1] == '\\':
			for i < len(b) && b[i] != '\n' {
				b[i] = ' '
				i++
			}
		}
	}
	return string(b)
}

var verifiedRe = regexp.MustCompile(`:\s*verified\s*;`)

// findVerifiedPremises locates every `<expr> : verified;` inside a
// (datatype ...) block, with the byte span to blank for the mutant.
func findVerifiedPremises(src string) []premiseSite {
	s := blankComments(src)
	var out []premiseSite
	pos := 0
	for {
		i := strings.Index(s[pos:], "(datatype ")
		if i < 0 {
			break
		}
		start := pos + i
		depth, end := 0, -1
		inStr := false
		for j := start; j < len(s); j++ {
			c := s[j]
			if inStr {
				if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = j + 1
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			break
		}
		block := s[start:end]
		name := strings.Fields(strings.TrimPrefix(block, "(datatype "))
		dtName := ""
		if len(name) > 0 {
			dtName = strings.TrimSuffix(name[0], ")")
		}
		idx := 0
		for _, m := range verifiedRe.FindAllStringIndex(block, -1) {
			colon := start + m[0]
			semi := start + m[1] - 1
			e := colon - 1
			for e > start && (s[e] == ' ' || s[e] == '\t' || s[e] == '\n' || s[e] == '\r') {
				e--
			}
			b := e
			if s[e] == ')' {
				d := 0
				for b = e; b > start; b-- {
					if s[b] == ')' {
						d++
					} else if s[b] == '(' {
						d--
						if d == 0 {
							break
						}
					}
				}
			} else {
				for b > start && !strings.ContainsRune(" \t\n\r;()", rune(s[b-1])) {
					b--
				}
			}
			expr := strings.Join(strings.Fields(src[b:e+1]), " ")
			out = append(out, premiseSite{
				Datatype: dtName,
				Index:    idx,
				Expr:     expr,
				Line:     strings.Count(src[:b], "\n") + 1,
				start:    b,
				end:      semi + 1,
			})
			idx++
		}
		pos = end
	}
	return out
}

func blankSpan(s string, start, end int) string {
	b := []byte(s)
	for i := start; i < end && i < len(b); i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

// ----------------------------------------------------------------------------
// Report plumbing
// ----------------------------------------------------------------------------

func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// annotateDischargeWithMutation attaches per-premise mutation evidence to
// an existing discharge report (additive, omitempty field). Premises are
// matched by rule name (datatype block) and expression text. Returns how
// many premises were annotated; a missing report is not an error.
func annotateDischargeWithMutation(path string, rep *mutationReport) (int, error) {
	r, err := loadDischarge(path)
	if err != nil || r == nil {
		return 0, err
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	byKey := map[string]mutationResult{}
	for _, m := range rep.Premises {
		byKey[m.Datatype+"\x00"+norm(m.Premise+" : verified")] = m
	}
	n := 0
	for ri := range r.Rules {
		for pi := range r.Rules[ri].Premises {
			p := &r.Rules[ri].Premises[pi]
			m, ok := byKey[r.Rules[ri].Name+"\x00"+norm(p.Expression)]
			if !ok {
				continue
			}
			p.Mutation = &DischargeMutation{
				Status:      m.Status,
				KilledBy:    m.KilledBy,
				ShenRuntime: filepath.Base(rep.ShenRuntime),
				CheckedAt:   rep.GeneratedAt,
			}
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, writeDischarge(path, r)
}
