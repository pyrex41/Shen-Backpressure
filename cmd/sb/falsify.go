package main

// falsify.go — `sb loop --falsify`: the second phase that runs after
// an iteration in which every gate passed.
//
// The main loop is CEGIS with a model as the synthesizer, and its
// progress per iteration is bounded by the bits the verifier returns.
// When every gate passes, the verifier returns exactly one bit —
// "green" — and the loop has nothing to work with. That is the moment
// this phase exists for. It inverts the question: instead of "make the
// gates pass", it asks "find one program or one input that shows a gate
// is weaker than it looks".
//
// The theory is the pairing from incorrectness logic. The main loop
// over-approximates and proves bugs *absent* within what its gates can
// see; the falsifier under-approximates and proves a bug *present*.
// Neither alone tells you how strong the gates are. Run together, the
// second one measures the first.
//
// Two things make this safe to wire into an automated loop:
//
//  1. Everything the falsifier produces is a *claim*, and every claim
//     is re-checked by machinery that does not trust it. A forgery
//     declares an expected outcome and `sb forgery` runs it; a sample
//     names an input and shen-derive derives the expectation from the
//     spec. A wrong claim becomes a failing gate or a passing sample,
//     never a false result.
//  2. Nothing here calls a model directly. The phase hydrates a prompt
//     and hands it to the same harness command the main loop uses, so
//     a project that has configured one harness has configured both.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// FalsifierPromptPath is the project-local prompt template. When it is
// absent the built-in copy from the embedded skill bundle is used, so
// `--falsify` works in a project that has never customised it.
const FalsifierPromptPath = "prompts/falsifier_prompt.md"

// FalsifierSamplesPath is where the falsifier appends inputs for
// shen-derive to pick up as a fourth sample source. It mirrors
// shen-derive's verify.DefaultFalsifierSamplesPath; the two modules do
// not import each other.
const FalsifierSamplesPath = ".sb/falsifier-samples.json"

// FalsifierPromptData is what the template is hydrated with.
type FalsifierPromptData struct {
	SpecPath    string
	Spec        string
	GuardsPath  string
	Guards      string
	ForgeryDir  string
	SamplesPath string
	Corpus      string
	Survivors   string
}

// BuildFalsifierPrompt hydrates the falsifier template. Every section
// is filled from what is on disk right now, because a prompt that
// describes a stale corpus invites the model to re-submit something
// the corpus already has.
//
// A missing spec or guards file is not fatal: the prompt says so in
// the slot instead, which is more useful to a model than an abort.
func BuildFalsifierPrompt(tmplSrc string, cfg *Config, score *MutationScore) (string, error) {
	corpusDir := cfg.Forgery.Dir
	if corpusDir == "" {
		corpusDir = DefaultForgeryDir
	}
	data := &FalsifierPromptData{
		SpecPath:    cfg.Spec,
		Spec:        readOrNote(cfg.Spec),
		GuardsPath:  cfg.Output,
		Guards:      readOrNote(cfg.Output),
		ForgeryDir:  corpusDir,
		SamplesPath: FalsifierSamplesPath,
		Corpus:      renderCorpusForPrompt(corpusDir),
		Survivors:   renderSurvivorsForPrompt(score),
	}
	tmpl, err := template.New("falsifier").Parse(tmplSrc)
	if err != nil {
		return "", fmt.Errorf("parsing the falsifier prompt template: %w", err)
	}
	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("hydrating the falsifier prompt: %w", err)
	}
	return b.String(), nil
}

// readOrNote returns the file's contents, or a note naming what was
// missing. Silence in a prompt slot reads to a model as "there is
// nothing here", which is a different and wrong claim.
func readOrNote(path string) string {
	if path == "" {
		return "(not configured for this project)"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(could not read %s: %v)", path, err)
	}
	return strings.TrimRight(string(data), "\n")
}

// renderCorpusForPrompt lists the corpus with each entry's declared
// outcome and its opening sentence, so the model can see what has
// already been tried without being handed every file in full.
func renderCorpusForPrompt(dir string) string {
	forgeries, err := LoadForgeryCorpus(dir)
	if err != nil {
		return fmt.Sprintf("(no corpus yet at `%s`: %v — anything you write will be the first entry)", dir, err)
	}
	if len(forgeries) == 0 {
		return fmt.Sprintf("(`%s` is empty — anything you write will be the first entry)", dir)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d entr%s in `%s`:\n\n", len(forgeries), plural(len(forgeries), "y", "ies"), dir)
	b.WriteString("| File | Declares | Technique |\n|---|---|---|\n")
	for _, f := range forgeries {
		desc := f.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(&b, "| `%s.go.bak` | `%s` | %s |\n", f.Name, f.Expect, strings.ReplaceAll(desc, "|", "\\|"))
	}
	return b.String()
}

// renderSurvivorsForPrompt turns the mutation survivors into the
// falsifier's most concrete lead: each one names a line and a change
// the committed samples do not distinguish.
func renderSurvivorsForPrompt(score *MutationScore) string {
	if score == nil {
		return "(no mutation run available — run `sb mutate` to produce survivors, which are the sharpest lead this prompt can carry)"
	}
	if len(score.Survivors) == 0 {
		return fmt.Sprintf(
			"None. `sb mutate` produced %d mutant(s) and the committed spec test killed every live one (kill rate %.1f%%, %d marked equivalent, %d invalid).\n\n"+
				"That is not a reason to stop: the operator set is small on purpose, so a 100%% kill rate means \"nothing in this operator set survived\", not \"the samples are complete\". Look instead for a *shape* of input the operator set cannot express — a boundary the spec's text distinguishes, or a list length the pool never reaches.",
			score.Total, score.Score*100, score.Equivalent, score.Invalid)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d survivor(s) — each is a change to the implementation the committed spec test did NOT notice. Killing one is the most valuable thing you can do here.\n\n", len(score.Survivors))
	b.WriteString("| Mutant | Location | Change |\n|---|---|---|\n")
	survivors := append([]Mutant(nil), score.Survivors...)
	sort.SliceStable(survivors, func(i, j int) bool { return survivors[i].ID < survivors[j].ID })
	for _, m := range survivors {
		fmt.Fprintf(&b, "| `%s` | `%s:%d:%d` | `%s` → `%s` |\n",
			m.ID, m.File, m.Line, m.Col, m.Before, m.After)
	}
	b.WriteString("\nFor each: find an input on which the mutated implementation and the real one differ, and write it as an Option B sample. Note the mutant id in the sample's `note` so a later reader knows what deleting it would give up.\n")
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// FalsifierFindings is what one falsify phase produced, measured by
// looking at the tree rather than by believing the harness.
type FalsifierFindings struct {
	NewForgeries []string // corpus files that were not there before
	NewSamples   int      // entries added to the samples file
	CorpusErrors []string // files the corpus parser rejected
}

// Any reports whether the phase produced anything at all.
func (f *FalsifierFindings) Any() bool {
	return len(f.NewForgeries) > 0 || f.NewSamples > 0 || len(f.CorpusErrors) > 0
}

// snapshotFalsifierInputs records the corpus filenames and the sample
// count before the harness runs, so what it produced can be measured
// afterwards by difference. Measuring beats parsing the harness's
// prose about what it claims to have done.
func snapshotFalsifierInputs(corpusDir string) (map[string]bool, int) {
	names := map[string]bool{}
	if entries, err := os.ReadDir(corpusDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go.bak") {
				names[e.Name()] = true
			}
		}
	}
	return names, countFalsifierSamples(FalsifierSamplesPath)
}

// collectFalsifierFindings diffs the tree against the snapshot.
func collectFalsifierFindings(corpusDir string, before map[string]bool, samplesBefore int) *FalsifierFindings {
	out := &FalsifierFindings{}
	entries, err := os.ReadDir(corpusDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go.bak") || before[e.Name()] {
				continue
			}
			p := filepath.Join(corpusDir, e.Name())
			if _, err := ParseForgeryHeader(p); err != nil {
				// A new file with no usable header is a finding too:
				// the falsifier wrote something and did not declare an
				// outcome for it, and the operator needs to see that
				// rather than have the file silently ignored.
				out.CorpusErrors = append(out.CorpusErrors, err.Error())
				continue
			}
			out.NewForgeries = append(out.NewForgeries, p)
		}
	}
	sort.Strings(out.NewForgeries)
	if after := countFalsifierSamples(FalsifierSamplesPath); after > samplesBefore {
		out.NewSamples = after - samplesBefore
	}
	return out
}

// countFalsifierSamples reads the samples file and returns how many
// entries it holds. A missing or unparseable file counts as zero: this
// is a difference measurement, and an unreadable file is not a finding
// about the falsifier.
func countFalsifierSamples(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var f struct {
		Samples []struct{} `json:"samples"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return 0
	}
	return len(f.Samples)
}

// runFalsifyPhase hydrates the prompt, calls the harness, and reports
// what changed on disk. It never fails the loop: the falsifier is an
// opportunistic search, and finding nothing is its ordinary outcome.
func runFalsifyPhase(cfg *Config, iteration int) *FalsifierFindings {
	corpusDir := cfg.Forgery.Dir
	if corpusDir == "" {
		corpusDir = DefaultForgeryDir
	}

	fmt.Fprintf(os.Stderr, "\n=== Falsify phase (after passing iteration %d) ===\n", iteration)

	// The survivors are the phase's sharpest input, so measure them
	// first rather than reusing whatever an earlier run left behind.
	score := loadMutationScore()
	if score == nil {
		fmt.Fprintln(os.Stderr, "sb loop: no mutation score on disk; run `sb mutate` to give the falsifier survivors to work from")
	}

	tmplSrc, src, err := loadFalsifierTemplate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb loop: %v\n", err)
		return &FalsifierFindings{}
	}
	prompt, err := BuildFalsifierPrompt(tmplSrc, cfg, score)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb loop: %v\n", err)
		return &FalsifierFindings{}
	}
	fmt.Fprintf(os.Stderr, "sb loop: falsifier prompt from %s (%d bytes)\n", src, len(prompt))

	before, samplesBefore := snapshotFalsifierInputs(corpusDir)

	fmt.Fprintf(os.Stderr, "sb loop: calling harness: %s\n", cfg.Harness)
	if err := callHarness(cfg, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "sb loop: falsifier harness error: %v\n", err)
	}

	findings := collectFalsifierFindings(corpusDir, before, samplesBefore)
	reportFalsifierFindings(findings, corpusDir)
	return findings
}

// reportFalsifierFindings prints what the phase produced and, when it
// produced a forgery, runs the corpus so the declaration is checked
// immediately rather than at the next `sb gates`.
func reportFalsifierFindings(f *FalsifierFindings, corpusDir string) {
	if !f.Any() {
		fmt.Fprintln(os.Stderr, "sb loop: the falsifier found nothing this round. That is its ordinary outcome, and it is weak evidence that the gates are doing their job — weak because the falsifier is a search, not a proof.")
		return
	}
	for _, p := range f.NewForgeries {
		fmt.Fprintf(os.Stderr, "sb loop: new forgery: %s\n", p)
	}
	for _, e := range f.CorpusErrors {
		fmt.Fprintf(os.Stderr, "sb loop: WARNING: a new corpus file is unusable: %s\n", e)
	}
	if f.NewSamples > 0 {
		fmt.Fprintf(os.Stderr, "sb loop: %d new falsifier sample(s) in %s — `sb derive --regen` will fold them into the committed spec test\n",
			f.NewSamples, FalsifierSamplesPath)
	}
	if len(f.NewForgeries) > 0 {
		fmt.Fprintln(os.Stderr, "sb loop: running the corpus so the new declaration is checked now")
		if self, err := os.Executable(); err == nil {
			out, runErr := runCaptured("", self, "forgery")
			os.Stderr.Write(out)
			if runErr != nil {
				fmt.Fprintln(os.Stderr, "sb loop: the corpus is RED. Either the new forgery's declared outcome was wrong, or it found a real hole. Both are findings; neither is a reason to edit the declaration until it goes green.")
			}
		}
	}
}

// loadFalsifierTemplate prefers the project's own copy and falls back
// to the embedded bundle, so `--falsify` works before a project has
// customised anything.
func loadFalsifierTemplate() (string, string, error) {
	if data, err := os.ReadFile(FalsifierPromptPath); err == nil {
		return string(data), FalsifierPromptPath, nil
	}
	data, err := skilldata.ReadFile("skilldata/FALSIFIER_PROMPT.md")
	if err != nil {
		return "", "", fmt.Errorf("no falsifier prompt: neither %s nor the embedded template is available (%w)", FalsifierPromptPath, err)
	}
	return string(data), "the embedded template (sb/FALSIFIER_PROMPT.md)", nil
}

// loadMutationScore reads the score the last `sb mutate` left in the
// discharge report. Missing is not an error: the phase says so and
// carries on with the rest of the prompt.
func loadMutationScore() *MutationScore {
	r, err := loadDischarge(DischargeReportPath)
	if err != nil || r == nil || r.Evidence == nil {
		return nil
	}
	return r.Evidence.MutationScore
}
