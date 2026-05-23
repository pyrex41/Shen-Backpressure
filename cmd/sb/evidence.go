package main

// evidence.go — `sb context --evidence` projects the locked v1
// discharge report into the smaller "mixed-evidence" view from design
// memo thoughts/shared/research/2026-05-05-feature-mixed-evidence-report.md.
//
// The discharge report already carries per-premise classification
// (static / runtime-sample / unproven). What it lacks is a one-screen
// agent-facing summary that answers: "across every rule, what kind of
// evidence backs each premise, and which rules still have unproven
// material?" This file builds that view.
//
// No schema changes: this is a read-only projection over the existing
// .sb/discharge_report.json. When the report is missing we say so
// clearly rather than fabricating zeros, because the absence of
// evidence is itself signal.

import (
	"fmt"
	"sort"
	"strings"
)

// EvidenceSummary is the JSON form of `sb context --evidence`.
// Counts at the top of each section are totals over every rule and
// every premise.
type EvidenceSummary struct {
	// Source is the path to the discharge report this view was
	// projected from, or empty if no report was available.
	Source string `json:"source,omitempty"`

	// Available reports whether a discharge report was found. When
	// false, all counts and Rules are zero/empty and the renderer
	// surfaces an explanatory message rather than misleading zeros.
	Available bool `json:"available"`

	// Totals over every rule's premises.
	PremisesStatic         int `json:"premises_static"`
	PremisesRuntimeSampled int `json:"premises_runtime_sampled"`
	PremisesUnproven       int `json:"premises_unproven"`
	PremisesTotal          int `json:"premises_total"`

	// Per-rule breakdown, sorted by rule name for stability.
	Rules []EvidenceRule `json:"rules,omitempty"`
}

// EvidenceRule is one rule's mixed-evidence row.
type EvidenceRule struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Static         int    `json:"static"`
	RuntimeSampled int    `json:"runtime_sampled"`
	Unproven       int    `json:"unproven"`
	Total          int    `json:"total"`
}

// BuildEvidenceSummary loads the discharge report at path and
// projects it into an EvidenceSummary. A missing or unreadable report
// produces a summary with Available=false; callers should render the
// "no report yet" message in that case rather than the empty table.
func BuildEvidenceSummary(path string) *EvidenceSummary {
	out := &EvidenceSummary{Source: path}
	r, err := loadDischarge(path)
	if err != nil || r == nil {
		return out
	}
	out.Available = true
	out.PremisesStatic = r.Summary.PremisesStatic
	out.PremisesRuntimeSampled = r.Summary.PremisesRuntimeSampled
	out.PremisesUnproven = r.Summary.PremisesUnproven
	out.PremisesTotal = r.Summary.PremisesTotal

	rows := make([]EvidenceRule, 0, len(r.Rules))
	for _, rule := range r.Rules {
		row := EvidenceRule{
			Name:   rule.Name,
			Kind:   rule.Kind,
			Status: rule.Status,
		}
		for _, p := range rule.Premises {
			row.Total++
			switch p.Discharge {
			case DischargeStatic:
				row.Static++
			case DischargeRuntimeSampled:
				row.RuntimeSampled++
			case DischargeUnproven:
				row.Unproven++
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	out.Rules = rows
	return out
}

// RenderEvidenceMarkdown emits the human-readable form: top-of-page
// totals, then a Markdown table of every rule with per-column
// premise counts. Designed for `sb context --evidence` output and PR
// comments. When the discharge report is absent we emit a one-line
// pointer instead of an empty table.
func RenderEvidenceMarkdown(path string) string {
	s := BuildEvidenceSummary(path)
	var b strings.Builder
	b.WriteString("## Mixed-Evidence Summary\n\n")

	if !s.Available {
		fmt.Fprintf(&b, "No discharge report found at `%s`.\n\n", path)
		b.WriteString("Run `sb gates` (or `sb derive`) to produce one. The mixed-evidence summary projects the per-premise classification recorded in the discharge report — without that artifact, the only honest answer is \"no evidence has been collected yet.\"\n")
		return b.String()
	}

	fmt.Fprintf(&b, "Source: `%s`\n\n", s.Source)
	fmt.Fprintf(&b, "**Totals:** %d premises — %d static, %d runtime-sampled, %d unproven\n\n",
		s.PremisesTotal, s.PremisesStatic, s.PremisesRuntimeSampled, s.PremisesUnproven)

	if len(s.Rules) == 0 {
		b.WriteString("(No rules to display.)\n")
		return b.String()
	}

	b.WriteString("| Rule | Kind | Status | Static | Runtime-sampled | Unproven | Total |\n")
	b.WriteString("|---|---|---|---:|---:|---:|---:|\n")
	for _, row := range s.Rules {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %d | %d | %d | %d |\n",
			row.Name, row.Kind, row.Status,
			row.Static, row.RuntimeSampled, row.Unproven, row.Total)
	}
	b.WriteString("\n")
	b.WriteString("Read this table as: which dimension of evidence covers each premise. Static premises are guarded by the target language's type system; runtime-sampled premises were checked against the Shen oracle on a deterministic boundary pool; unproven premises sit outside the verified boundary in this release.\n")
	return b.String()
}
