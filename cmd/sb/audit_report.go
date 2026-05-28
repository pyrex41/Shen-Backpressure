package main

// audit_report.go — `sb audit-report` produces a long-form,
// audit-grade Markdown rendering of the latest discharge report (or
// of any historical report under .sb/history/). The output is
// intentionally written for a reader who has never seen
// Shen-Backpressure: plain-English rule descriptions, a "How to read
// this report" appendix, and no jargon outside the appendix.
//
// Wave 4 design: thoughts/shared/research/2026-05-05-discharge-report-schema.md.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func cmdAuditReport(args []string) {
	fs := flag.NewFlagSet("audit-report", flag.ExitOnError)
	format := fs.String("format", "markdown", "output format: markdown or json")
	in := fs.String("in", DischargeReportPath, "path to discharge report JSON (default: .sb/discharge_report.json)")
	out := fs.String("out", "", "output file (default: stdout)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb audit-report — Long-form audit-grade rendering of a discharge report

Usage: sb audit-report [flags]

Reads a discharge report (default: .sb/discharge_report.json, or a
historical copy under .sb/history/) and produces:

  --format=markdown   (default) a self-contained Markdown document
                      readable by a security reviewer or auditor who
                      has never seen this project.
  --format=json       a passthrough of the source JSON, useful for
                      piping into other tools.

The Markdown rendering covers: project + spec hash + git commit, tool
versions, per-rule sections (description, premises with discharge
classification, status, history), counter-examples for any violated
rules, and a "How to read this report" appendix.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	r, err := loadDischarge(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb audit-report: %v\n", err)
		os.Exit(1)
	}
	if r == nil {
		fmt.Fprintf(os.Stderr,
			"sb audit-report: no report found at %s. Run `sb gates` (or `sb derive`) first.\n",
			*in)
		os.Exit(1)
	}

	var rendered []byte
	switch *format {
	case "markdown", "md":
		rendered = []byte(renderAuditMarkdown(r, *in))
	case "json":
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb audit-report: %v\n", err)
			os.Exit(1)
		}
		rendered = b
	default:
		fmt.Fprintf(os.Stderr, "sb audit-report: unknown format %q (want markdown or json)\n", *format)
		os.Exit(2)
	}

	if *out == "" {
		os.Stdout.Write(rendered)
		if len(rendered) > 0 && rendered[len(rendered)-1] != '\n' {
			fmt.Println()
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "sb audit-report: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, rendered, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sb audit-report: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "sb audit-report: wrote %s\n", *out)
}

// renderAuditMarkdown produces the long-form Markdown rendering
// described above. Audience: an auditor or security reviewer who has
// not seen Shen-Backpressure before.
func renderAuditMarkdown(r *DischargeReport, sourcePath string) string {
	var b strings.Builder

	// Header.
	b.WriteString("# Discharge Report — Audit Rendering\n\n")
	fmt.Fprintf(&b, "Generated %s. Source artifact: `%s` (schema_version=%d).\n\n",
		r.GeneratedAt, sourcePath, r.SchemaVersion)

	if r.Impl.GitCommit != nil {
		commit := *r.Impl.GitCommit
		dirty := ""
		if r.Impl.GitDirty != nil && *r.Impl.GitDirty {
			dirty = " (working tree dirty)"
		}
		fmt.Fprintf(&b, "**Implementation commit:** `%s`%s\n", commit, dirty)
	} else {
		b.WriteString("**Implementation commit:** unavailable (project not in git)\n")
	}
	if len(r.Spec.Files) > 0 {
		b.WriteString("\n**Spec files:**\n\n")
		for _, sf := range r.Spec.Files {
			fmt.Fprintf(&b, "- `%s` (sha256 `%s`)\n", sf.Path, sf.SHA256)
		}
	}
	fmt.Fprintf(&b, "\n**Target languages:** %s\n", strings.Join(r.Impl.TargetLanguages, ", "))

	// Tool versions.
	b.WriteString("\n## Tool Versions\n\n")
	b.WriteString("| Tool | Version |\n|---|---|\n")
	fmt.Fprintf(&b, "| sb | %s |\n", emptyDash(r.Tools.SBVersion))
	fmt.Fprintf(&b, "| shen-derive | %s |\n", emptyDash(r.Tools.ShenDeriveVersion))
	fmt.Fprintf(&b, "| shengen | %s |\n", emptyDash(r.Tools.ShengenVersion))
	if r.Tools.ShenRuntime != nil {
		fmt.Fprintf(&b, "| shen runtime | %s |\n", *r.Tools.ShenRuntime)
	} else {
		fmt.Fprintf(&b, "| shen runtime | not detected |\n")
	}

	// Summary.
	b.WriteString("\n## Summary\n\n")
	s := r.Summary
	fmt.Fprintf(&b, "- **Rules:** %d total — %d discharged, %d violated, %d unproven\n",
		s.RuleCount, s.RulesDischarged, s.RulesViolated, s.RulesUnproven)
	if s.PremisesRuntimeEvaluator > 0 {
		fmt.Fprintf(&b, "- **Premises:** %d total — %d static, %d runtime-evaluator, %d runtime-sampled, %d unproven\n",
			s.PremisesTotal, s.PremisesStatic, s.PremisesRuntimeEvaluator, s.PremisesRuntimeSampled, s.PremisesUnproven)
	} else {
		fmt.Fprintf(&b, "- **Premises:** %d total — %d static, %d runtime-sampled, %d unproven\n",
			s.PremisesTotal, s.PremisesStatic, s.PremisesRuntimeSampled, s.PremisesUnproven)
	}

	if s.RulesViolated > 0 {
		b.WriteString("\n> :warning: **At least one rule is currently violated.** See per-rule sections below for counter-examples.\n")
	}

	// Per-rule sections, alphabetised for stability.
	rules := append([]DischargeRule(nil), r.Rules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })

	b.WriteString("\n## Rules\n\n")
	for _, rule := range rules {
		renderRuleSection(&b, rule)
	}

	// Appendix.
	b.WriteString("\n## How to Read This Report\n\n")
	b.WriteString(auditAppendix)

	return b.String()
}

func renderRuleSection(b *strings.Builder, rule DischargeRule) {
	statusBadge := "✅ Discharged"
	switch rule.Status {
	case DischargeStatusViolated:
		statusBadge = "❌ Violated"
	case DischargeStatusUnproven:
		statusBadge = "⚠️  Unproven"
	}
	fmt.Fprintf(b, "### `%s` — %s (%s)\n\n", rule.Name, rule.Kind, statusBadge)
	if rule.HumanDescription != "" {
		marker := ""
		if rule.HumanDescriptionSource == ":doc" {
			marker = " *(from spec :doc annotation)*"
		} else {
			marker = " *(auto-generated from rule structure; not reviewed by spec author)*"
		}
		fmt.Fprintf(b, "%s%s\n\n", rule.HumanDescription, marker)
	}
	if rule.SpecExcerpt != "" {
		b.WriteString("Spec:\n\n```shen\n")
		b.WriteString(strings.TrimRight(rule.SpecExcerpt, "\n"))
		b.WriteString("\n```\n\n")
	}
	if rule.DischargedSinceCommit != nil && *rule.DischargedSinceCommit != "" && rule.Status == DischargeStatusDischarged {
		fmt.Fprintf(b, "Continuously discharged since commit `%s`.\n\n", *rule.DischargedSinceCommit)
	}
	if len(rule.Premises) > 0 {
		b.WriteString("**Premises**\n\n")
		b.WriteString("| ID | Expression | Discharge | Basis | Rationale |\n")
		b.WriteString("|---|---|---|---|---|\n")
		for _, p := range rule.Premises {
			rationale := strings.ReplaceAll(p.Rationale, "|", "\\|")
			fmt.Fprintf(b, "| `%s` | `%s` | %s | %s | %s |\n",
				p.ID, escapeMarkdownInline(p.Expression),
				p.Discharge, p.DischargeBasis, rationale)
		}
		b.WriteString("\n")
		// Sample stats / code refs underneath the table.
		for _, p := range rule.Premises {
			if p.Discharge == DischargeRuntimeSampled {
				seed := "deterministic-default"
				if p.SampleSeed != nil {
					seed = *p.SampleSeed
				}
				fmt.Fprintf(b, "\n- `%s`: sampled %d cases (seed: %s); %d passed, %d failed.\n",
					p.ID, p.SamplesPassed+p.SamplesFailed, seed, p.SamplesPassed, p.SamplesFailed)
			}
			if len(p.CodeReferences) > 0 {
				fmt.Fprintf(b, "- `%s` code references: %s\n",
					p.ID, "`"+strings.Join(p.CodeReferences, "`, `")+"`")
			}
		}
		b.WriteString("\n")
	}
	if len(rule.CounterExamples) > 0 {
		b.WriteString("**Counter-examples**\n\n")
		for _, ce := range rule.CounterExamples {
			renderCounterExample(b, ce)
		}
	}
}

// renderCounterExample emits one counter-example in the richer
// Markdown form designed for human auditors and AI agents alike.
// Compared with the previous inline-bullet rendering, this puts:
//   - the input as a multi-line key=value code block, so a reader can
//     copy the exact case into a debugger without backtick scraping;
//   - the spec-vs-impl outputs as side-by-side fenced blocks, so the
//     diff is visible to a five-second skim;
//   - the reproducer command in its own fenced block, so it can be
//     copy-pasted intact (no surrounding markdown noise to strip).
//
// Format intentionally stays single-language (no rendered HTML, no
// admonition extensions) so the same Markdown renders correctly on
// GitHub, in a PR comment, in `glow`, and in an agent harness's plain
// text view. See thoughts/shared/research/2026-05-05-feature-counterexample-traces.md
// for the design motivation.
func renderCounterExample(b *strings.Builder, ce DischargeCounter) {
	fmt.Fprintf(b, "#### Case `%s`\n\n", ce.CaseID)

	// Input as a multi-line code block — one key=value per line.
	// Sorted keys so identical input renders byte-identical across
	// runs (snapshot diff stability).
	if len(ce.Input) > 0 {
		keys := make([]string, 0, len(ce.Input))
		for k := range ce.Input {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("Input:\n\n```\n")
		for _, k := range keys {
			fmt.Fprintf(b, "%s = %s\n", k, ce.Input[k])
		}
		b.WriteString("```\n\n")
	}

	// Side-by-side spec-vs-impl. We render two fenced blocks back to
	// back; the reader's Markdown renderer is responsible for
	// horizontal placement when possible (GitHub doesn't have native
	// side-by-side, so stacked blocks are the lowest-common-denom).
	// The block language tags (`shen` / `<lang>`) give syntax
	// highlighters a hint without committing to a specific tool.
	b.WriteString("Spec output (Shen oracle):\n\n```shen\n")
	b.WriteString(strings.TrimRight(ce.SpecOutput, "\n"))
	b.WriteString("\n```\n\n")
	b.WriteString("Impl output (")
	if ce.ImplFunction != "" {
		fmt.Fprintf(b, "`%s`", ce.ImplFunction)
	} else {
		b.WriteString("target implementation")
	}
	b.WriteString("):\n\n```\n")
	b.WriteString(strings.TrimRight(ce.ImplOutput, "\n"))
	b.WriteString("\n```\n\n")

	if ce.ImplFile != "" {
		fmt.Fprintf(b, "Impl file: `%s`", ce.ImplFile)
		if ce.ImplLineHint != nil && *ce.ImplLineHint > 0 {
			fmt.Fprintf(b, ":%d", *ce.ImplLineHint)
		}
		b.WriteString("\n\n")
	}

	// Reproducer in its own fenced block so it's one shell-friendly
	// copy. We default to the Go test form because shen-derive's
	// generated tests are the dominant source of counter-examples
	// today; a future TS path would emit a node:test reproducer.
	if ce.ImplFunction != "" {
		b.WriteString("Reproduce:\n\n```sh\n")
		fmt.Fprintf(b, "go test -run 'TestSpec_%s/%s' -v\n", ce.ImplFunction, ce.CaseID)
		b.WriteString("```\n\n")
	}

	if ce.Rationale != "" {
		fmt.Fprintf(b, "> %s\n\n", ce.Rationale)
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// escapeMarkdownInline is a best-effort escape for content rendered
// inside a markdown table cell. We escape the table separator (`|`)
// and newlines (which would otherwise break a row); other markdown
// metacharacters (backticks, asterisks, brackets) survive and may
// distort cosmetic rendering when a spec expression contains them.
// Callers are expected not to render this content through HTML
// without further sanitisation.
func escapeMarkdownInline(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

const auditAppendix = `This report categorises every premise of every Shen rule by **how**
it was discharged in the implementation under verification.

- **Static** — the target language's type system (Go's static
  typing, applied to shengen's generated guard types) prevents the
  premise from being violated. A premise typed at the function
  boundary cannot be reached with a non-conforming value because the
  compiler refuses to build such a call site. ` + "`guard-type-at-boundary`" + ` and
  ` + "`guard-constructor-validates`" + ` are the two static bases this
  release emits.

- **Runtime-sampled** — shen-derive evaluates the Shen spec on a
  deterministic boundary pool (and, when seeded, additional random
  draws) and emits a Go test asserting that the implementation
  returns the same value on every sampled input. A "discharged"
  premise here means *every sampled case agreed*. This is sampled
  evidence, not an exhaustive proof.

- **Unproven** — the tool could not confidently classify the premise
  in this release. Treat the premise as outside the verified
  boundary until a future version of the tool can address it.

**What this report does not claim**

- It is not a SOC-2, ISO-27001, or any other compliance certification.
  It is a verification artifact that compliance and audit workflows
  may reference as evidence.
- It is not signed or attested. The ` + "`signature`" + ` field in the JSON is
  reserved for a future signing integration; in this release it is
  always null.
- It is not third-party verified. The classifications and rationales
  come from this tool's own analysis of the spec and the
  implementation.

**Reproducing this report**

The discharge report is produced as a side effect of every successful
` + "`sb gates`" + ` (or ` + "`sb derive`" + `) run. Run the gate pipeline against the
same spec and the same git commit recorded in this report and you
will get a byte-identical artifact (modulo the ` + "`generated_at`" + `
timestamp). Time-stamped copies accumulate under ` + "`.sb/history/`" + `.

For per-case input detail, open the generated test file referenced
in the spec's manifest and look for the matching ` + "`case_NN`" + ` entry.
`
