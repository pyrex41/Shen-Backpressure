package main

// gates_run.go — persistence of per-gate PASS/FAIL outcomes from the
// most recent `sb gates` run. The discharge report schema (v1, locked)
// captures derive-time evidence but says nothing about the wider gate
// pipeline (shengen, test, build, shen-check, tcb-audit). To give the
// agent-facing surface in `sb context` actionable feedback ("the last
// build gate failed in 3.2s"), we persist a small sidecar artifact at
// .sb/gates_last_run.json after every `sb gates` invocation.
//
// This file is intentionally NOT part of the v1 discharge schema. It
// is a separate, additive sibling that consumers can ignore entirely.
// Readers (currently: `sb context`) must treat its absence and any
// parse failure as "no information" — never as an error.
//
// Wire format is documented at
// thoughts/shared/research/2026-05-22-schema-v1-additions.md.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// GatesLastRunPath is where we persist the last-run summary. Sibling
// to .sb/discharge_report.json; gitignored along with the rest of .sb/.
const GatesLastRunPath = ".sb/gates_last_run.json"

// GatesLastRun is the wire-format struct. SchemaVersion is here so
// future additive changes (e.g. per-gate stderr excerpts) can be
// staged behind a version bump. Consumers tolerate older versions by
// reading only the fields they recognise.
type GatesLastRun struct {
	SchemaVersion int               `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	Gates         []GateLastResult  `json:"gates"`
}

// GateLastResult is one gate's outcome from the most recent run.
// DurationMs is recorded in milliseconds so the JSON round-trips
// cleanly without floating-point precision drift; renderers convert
// back to a time.Duration for display.
type GateLastResult struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// writeGatesLastRun persists the per-gate outcomes of a single `sb
// gates` invocation. Errors are non-fatal: this file is a hint for
// downstream UX, not a correctness boundary.
func writeGatesLastRun(results []gateResult) error {
	out := GatesLastRun{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Gates:         make([]GateLastResult, 0, len(results)),
	}
	for _, r := range results {
		out.Gates = append(out.Gates, GateLastResult{
			Name:       r.name,
			Passed:     r.passed,
			ExitCode:   r.exitCode,
			DurationMs: r.elapsed.Milliseconds(),
		})
	}
	if err := os.MkdirAll(filepath.Dir(GatesLastRunPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(GatesLastRunPath), ".gates-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, GatesLastRunPath)
}

// readGatesLastRun loads the persisted per-gate outcomes from path.
// Returns nil and no error when the file is missing, malformed, or
// otherwise unusable. Callers must never treat a nil return as an
// error — the contract is "no information, render as unknown".
func readGatesLastRun(path string) *GatesLastRun {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var r GatesLastRun
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	// Defensive: an empty Gates slice is fine to surface (means "no
	// gates ran"), but a wholly missing schema_version probably
	// indicates we're looking at an unrelated JSON file. Treat as
	// "no information".
	if r.SchemaVersion == 0 {
		return nil
	}
	return &r
}

// gateLastResultByName returns the named gate's last-run outcome from
// r, or nil if r is nil or the gate isn't present. Used by the context
// renderer to attach LastResult to each GateInfo defensively.
func gateLastResultByName(r *GatesLastRun, name string) *GateLastResult {
	if r == nil {
		return nil
	}
	for i := range r.Gates {
		if r.Gates[i].Name == name {
			return &r.Gates[i]
		}
	}
	return nil
}
