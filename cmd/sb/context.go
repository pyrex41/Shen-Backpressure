package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// ProjectContext is the top-level structured view of a Shen-Backpressure
// project, derived from the manifest and the Shen spec file. It is used by
// `sb context` for both JSON and Markdown output, and by the Ralph loop to
// hydrate LLM harness prompts.
type ProjectContext struct {
	Project      ProjectInfo       `json:"project"`
	Types        []TypeInfo        `json:"types"`
	Derive       *DeriveInfo       `json:"derive,omitempty"`
	Gates        []GateInfo        `json:"gates"`
	Backpressure *BackpressureInfo `json:"backpressure,omitempty"`
}

// ProjectInfo holds the project-level manifest fields.
type ProjectInfo struct {
	Lang        string `json:"lang"`
	Pkg         string `json:"pkg"`
	Spec        string `json:"spec"`
	GuardOutput string `json:"guard_output"`
	DBWrappers  string `json:"db_wrappers,omitempty"`
}

// TypeInfo describes a single guard type parsed from the Shen spec.
type TypeInfo struct {
	ShenName     string   `json:"shen_name"`
	TargetName   string   `json:"target_name"`
	Category     string   `json:"category"` // wrapper, constrained, composite, guarded
	Constructor  string   `json:"constructor"`
	Fields       []string `json:"fields,omitempty"`
	Verified     []string `json:"verified,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// DeriveInfo summarises configured shen-derive specs.
type DeriveInfo struct {
	Enabled bool             `json:"enabled"`
	Specs   []DeriveSpecInfo `json:"specs"`
}

// DeriveSpecInfo is a per-spec summary for the context output.
type DeriveSpecInfo struct {
	Lang     string `json:"lang"`
	Func     string `json:"func"`
	ImplFunc string `json:"impl_func"`
	OutFile  string `json:"out_file"`
}

// GateInfo mirrors a single gate from the manifest (or the synthesised legacy
// five-gate list) for context output.
type GateInfo struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Run           string `json:"run,omitempty"`
	ParallelGroup string `json:"parallel_group,omitempty"`
}

// BackpressureInfo summarises the latest entry in plans/backpressure.log if
// one exists. Only populated when the log is non-empty.
type BackpressureInfo struct {
	HasFailures   bool   `json:"has_failures"`
	LatestFailure string `json:"latest_failure,omitempty"`
	Summary       string `json:"summary,omitempty"`
}

// BuildContext assembles a ProjectContext from a loaded Config. It parses the
// Shen spec to extract guard types, normalises gates from the manifest (or
// synthesises them from the legacy command fields), and checks for recent
// backpressure log entries.
func BuildContext(cfg *Config) (*ProjectContext, error) {
	ctx := &ProjectContext{
		Project: ProjectInfo{
			Lang:        cfg.Lang,
			Pkg:         cfg.Pkg,
			Spec:        cfg.Spec,
			GuardOutput: cfg.Output,
			DBWrappers:  cfg.DBWrap,
		},
	}

	types, err := parseSpecTypes(cfg.Spec, cfg.Lang)
	if err != nil {
		// Spec parse failures are non-fatal for context output — we still
		// want to emit the rest of the manifest so users can diagnose.
		fmt.Fprintf(os.Stderr, "sb context: warning: parsing %s: %v\n", cfg.Spec, err)
	}
	ctx.Types = types

	ctx.Gates = buildGateInfos(cfg)

	if len(cfg.DeriveSpecs) > 0 {
		di := &DeriveInfo{Enabled: true}
		for _, s := range cfg.DeriveSpecs {
			di.Specs = append(di.Specs, DeriveSpecInfo{
				Lang:     s.Lang,
				Func:     s.Func,
				ImplFunc: s.ImplFunc,
				OutFile:  s.OutFile,
			})
		}
		ctx.Derive = di
	}

	if bp := readBackpressure("plans/backpressure.log"); bp != nil {
		ctx.Backpressure = bp
	}

	return ctx, nil
}

// buildGateInfos produces the public GateInfo list for the context. When the
// manifest uses [[gates]], each entry is copied directly; otherwise we
// synthesise the legacy five-gate pipeline from the command fields.
func buildGateInfos(cfg *Config) []GateInfo {
	var out []GateInfo
	if cfg.HasManifestGates() {
		for _, g := range cfg.Gates {
			out = append(out, GateInfo{
				Name:          g.Name,
				Kind:          string(g.Kind),
				Run:           g.Run,
				ParallelGroup: g.ParallelGroup,
			})
		}
	} else {
		testGroup, buildGroup := "", ""
		if cfg.Relaxed {
			testGroup, buildGroup = "build-test", "build-test"
		}
		out = []GateInfo{
			{Name: "shengen", Kind: "command", Run: cfg.Gen},
			{Name: "test", Kind: "command", Run: cfg.Test, ParallelGroup: testGroup},
			{Name: "build", Kind: "command", Run: cfg.Build, ParallelGroup: buildGroup},
			{Name: "shen-check", Kind: "command", Run: cfg.Check},
			{Name: "tcb-audit", Kind: "command", Run: cfg.Audit},
		}
	}
	if len(cfg.DeriveSpecs) > 0 {
		out = append(out, GateInfo{Name: "shen-derive", Kind: "derive"})
	}
	return out
}

// readBackpressure reads the backpressure log and extracts a summary of the
// latest failure, if any. Returns nil when the file is missing or empty.
func readBackpressure(path string) *BackpressureInfo {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	latest := lines[len(lines)-1]
	summary := latest
	if len(lines) > 5 {
		summary = strings.Join(lines[len(lines)-5:], "\n")
	}
	return &BackpressureInfo{
		HasFailures:   true,
		LatestFailure: latest,
		Summary:       summary,
	}
}

// RenderJSON returns the context as pretty-printed JSON.
func (ctx *ProjectContext) RenderJSON() ([]byte, error) {
	return json.MarshalIndent(ctx, "", "  ")
}

// RenderMarkdown returns a compact Markdown view intended for LLM prompt
// hydration. Sections are omitted when empty to keep the prompt small.
func (ctx *ProjectContext) RenderMarkdown() string {
	var b strings.Builder
	b.WriteString("## Project Context\n\n")
	fmt.Fprintf(&b, "**Language**: %s | **Package**: %s | **Spec**: %s\n",
		ctx.Project.Lang, ctx.Project.Pkg, ctx.Project.Spec)
	if ctx.Project.GuardOutput != "" {
		fmt.Fprintf(&b, "**Guard output**: %s\n", ctx.Project.GuardOutput)
	}
	if ctx.Project.DBWrappers != "" {
		fmt.Fprintf(&b, "**DB wrappers**: %s\n", ctx.Project.DBWrappers)
	}

	if len(ctx.Types) > 0 {
		b.WriteString("\n### Guard Types\n\n")
		b.WriteString("| Type | Category | Constructor |\n")
		b.WriteString("|------|----------|-------------|\n")
		for _, t := range ctx.Types {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", t.TargetName, t.Category, t.Constructor)
		}

		// Proof chain: topological chain of types via Dependencies.
		if chain := proofChain(ctx.Types); len(chain) > 1 {
			b.WriteString("\n**Proof Chain**: ")
			b.WriteString(strings.Join(chain, " -> "))
			b.WriteString("\n")
		}
	}

	if len(ctx.Gates) > 0 {
		b.WriteString("\n### Gates\n\n")
		for i, g := range ctx.Gates {
			line := fmt.Sprintf("%d. %s (%s)", i+1, g.Name, g.Kind)
			if g.ParallelGroup != "" {
				line += fmt.Sprintf(" [parallel: %s]", g.ParallelGroup)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	if ctx.Derive != nil && len(ctx.Derive.Specs) > 0 {
		b.WriteString("\n### Derive Coverage\n\n")
		for _, s := range ctx.Derive.Specs {
			fmt.Fprintf(&b, "- %s -> %s (%s)\n", s.Func, s.ImplFunc, s.OutFile)
		}
	}

	if ctx.Backpressure != nil && ctx.Backpressure.HasFailures {
		b.WriteString("\n### Backpressure\n\n")
		fmt.Fprintf(&b, "Latest failure: %s\n", ctx.Backpressure.LatestFailure)
	}

	return b.String()
}

// proofChain returns a best-effort ordering of types following the
// dependency edges. The ordering is stable: it walks types in source order
// and emits each exactly once, skipping ones that don't form a chain with
// their predecessor. For the common payment example this produces
// amount -> transaction -> balance-checked -> safe-transfer.
func proofChain(types []TypeInfo) []string {
	if len(types) == 0 {
		return nil
	}
	// Build a name->index for quick lookup.
	byName := make(map[string]int, len(types))
	for i, t := range types {
		byName[t.ShenName] = i
	}
	// Linear chain: include types that depend on the previous one, plus
	// roots at the start.
	var out []string
	for _, t := range types {
		if len(t.Dependencies) == 0 {
			out = append(out, t.TargetName)
			continue
		}
		// Only include if at least one dependency is already in the chain.
		for _, dep := range t.Dependencies {
			if _, ok := byName[dep]; ok {
				out = append(out, t.TargetName)
				break
			}
		}
	}
	return out
}

// cmdContext is the CLI entry point for `sb context`.
func cmdContext(args []string) {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	format := fs.String("format", "markdown", "output format: json or markdown")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb context — Emit project context from the manifest

Usage: sb context [flags]

Parses sb.toml and the Shen spec to produce a structured view of the
project: guard types, gate pipeline, derive coverage, and any recent
backpressure failures. Output is consumed by humans (markdown) or by
the Ralph loop for LLM prompt hydration (json).

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb context: %v\n", err)
		os.Exit(1)
	}

	ctx, err := BuildContext(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb context: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		data, err := ctx.RenderJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb context: rendering json: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	case "markdown", "md":
		fmt.Print(ctx.RenderMarkdown())
	default:
		fmt.Fprintf(os.Stderr, "sb context: unknown format %q (want json or markdown)\n", *format)
		os.Exit(1)
	}
}

// -----------------------------------------------------------------------------
// Minimal Shen spec parser
//
// Mirrors the approach in shen-derive/specfile/parse.go but is self-contained
// so the cmd/sb module doesn't pick up a dependency on shen-derive. It only
// extracts (datatype ...) blocks and classifies them into the four guard
// categories — wrapper, constrained, composite, guarded. (define ...) blocks
// are ignored here; derive coverage comes from the manifest instead.
// -----------------------------------------------------------------------------

// parseSpecTypes reads a .shen spec file and returns the guard types it
// declares, classified and with target-language constructor signatures.
// A missing spec file is treated as an empty type list, not an error.
func parseSpecTypes(path, lang string) ([]TypeInfo, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	content := stripShenComments(string(data))
	blocks := extractDatatypeBlocks(content)

	// First pass: classify each datatype and collect raw type info.
	var types []TypeInfo
	knownTypes := make(map[string]bool)
	for _, block := range blocks {
		ti := parseDatatypeBlock(block, lang)
		if ti == nil {
			continue
		}
		knownTypes[ti.ShenName] = true
		types = append(types, *ti)
	}

	// Filter each type's Dependencies down to the known-type set so we
	// don't surface spurious entries for primitive-like aliases.
	for i := range types {
		if len(types[i].Dependencies) == 0 {
			continue
		}
		filtered := types[i].Dependencies[:0]
		for _, d := range types[i].Dependencies {
			if knownTypes[d] {
				filtered = append(filtered, d)
			}
		}
		types[i].Dependencies = filtered
	}

	return types, nil
}

// stripShenComments removes \* ... *\ block comments and \\ ... line comments
// from a Shen source string, preserving string-literal contents.
func stripShenComments(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '"' {
			b.WriteByte(s[i])
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					b.WriteByte(s[i])
					b.WriteByte(s[i+1])
					i += 2
					continue
				}
				b.WriteByte(s[i])
				if s[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}
		if i+1 < len(s) && s[i] == '\\' && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*\\")
			if end == -1 {
				break
			}
			i += end + 4
			continue
		}
		if i+1 < len(s) && s[i] == '\\' && s[i+1] == '\\' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// extractDatatypeBlocks finds balanced-paren "(datatype ...)" blocks.
func extractDatatypeBlocks(content string) []string {
	const prefix = "(datatype "
	var out []string
	remaining := content
	for {
		idx := strings.Index(remaining, prefix)
		if idx == -1 {
			break
		}
		remaining = remaining[idx:]
		depth, end := 0, -1
		i := 0
		for i < len(remaining) {
			ch := remaining[i]
			if ch == '"' {
				i++
				for i < len(remaining) {
					if remaining[i] == '\\' && i+1 < len(remaining) {
						i += 2
						continue
					}
					if remaining[i] == '"' {
						i++
						break
					}
					i++
				}
				continue
			}
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
			i++
		}
		if end == -1 {
			break
		}
		out = append(out, remaining[:end])
		remaining = remaining[end:]
	}
	return out
}

// parseDatatypeBlock parses a single (datatype ...) block, classifies it, and
// builds a TypeInfo with the target-language constructor signature.
func parseDatatypeBlock(block, lang string) *TypeInfo {
	inner := strings.TrimPrefix(block, "(datatype ")
	nlIdx := strings.Index(inner, "\n")
	if nlIdx == -1 {
		return nil
	}
	shenName := strings.TrimSpace(inner[:nlIdx])
	body := strings.TrimRight(inner[nlIdx:], " \t\n)")

	// Find the first ==== separator and split premises from conclusion.
	lines := strings.Split(body, "\n")
	var premLines, concLines []string
	seenInf := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if len(t) >= 3 && (allRune(t, '=') || allRune(t, '_')) {
			seenInf = true
			continue
		}
		if !seenInf {
			premLines = append(premLines, t)
		} else {
			concLines = append(concLines, t)
		}
	}
	if len(concLines) == 0 {
		return nil
	}

	// Parse premises: value premises "X : type;" and verified premises
	// "(...) : verified;".
	type fieldPremise struct {
		name, typ string
	}
	var fields []fieldPremise
	var verified []string
	for _, raw := range premLines {
		line := strings.TrimSuffix(strings.TrimSpace(raw), ";")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ": verified") {
			v := strings.TrimSpace(strings.TrimSuffix(line, ": verified"))
			verified = append(verified, v)
			continue
		}
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) != 2 {
			continue
		}
		fields = append(fields, fieldPremise{
			name: strings.TrimSpace(parts[0]),
			typ:  strings.TrimSpace(parts[1]),
		})
	}
	if len(fields) == 0 {
		return nil
	}

	// Parse conclusion "[...] : typename" or "X : typename".
	concStr := strings.TrimSpace(strings.TrimSuffix(strings.Join(concLines, " "), ";"))
	if strings.Contains(concStr, ">>") {
		return nil // subtype/refinement rules (not a plain datatype)
	}
	cp := strings.SplitN(concStr, " : ", 2)
	if len(cp) != 2 {
		return nil
	}
	// Classify.
	category := classifyType(len(fields), len(verified))

	// Target language name.
	targetName := toTargetName(shenName, lang)

	// Constructor signature.
	var fieldNames []string
	var fieldTypes []string
	for _, f := range fields {
		fieldNames = append(fieldNames, f.name)
		fieldTypes = append(fieldTypes, f.typ)
	}
	ctor := buildConstructor(targetName, fieldTypes, len(verified) > 0, lang)

	// Dependencies: field types that look like other datatypes (i.e. not
	// primitive shen types).
	deps := collectDependencies(fieldTypes)

	return &TypeInfo{
		ShenName:     shenName,
		TargetName:   targetName,
		Category:     category,
		Constructor:  ctor,
		Fields:       fieldNames,
		Verified:     verified,
		Dependencies: deps,
	}
}

// classifyType maps (value-field count, verified-premise count) to one of the
// four guard categories.
func classifyType(nFields, nVerified int) string {
	switch {
	case nFields == 1 && nVerified == 0:
		return "wrapper"
	case nFields == 1 && nVerified > 0:
		return "constrained"
	case nFields > 1 && nVerified == 0:
		return "composite"
	default:
		return "guarded"
	}
}

// collectDependencies returns the subset of field types that look like other
// user-defined datatypes (kebab-case identifiers, not Shen primitives).
func collectDependencies(fieldTypes []string) []string {
	primitives := map[string]bool{
		"number": true, "string": true, "boolean": true, "symbol": true,
		"unit": true,
	}
	var out []string
	seen := make(map[string]bool)
	for _, ft := range fieldTypes {
		ft = strings.TrimSpace(ft)
		// Handle "(list T)" — extract T.
		if strings.HasPrefix(ft, "(list ") && strings.HasSuffix(ft, ")") {
			ft = strings.TrimSpace(ft[6 : len(ft)-1])
		}
		if ft == "" || primitives[ft] || strings.ContainsAny(ft, "() ") {
			continue
		}
		if seen[ft] {
			continue
		}
		seen[ft] = true
		out = append(out, ft)
	}
	return out
}

// toTargetName converts a kebab-case Shen name to the target language's
// conventional type name. Go uses PascalCase; TypeScript uses PascalCase too
// for type declarations.
func toTargetName(shenName, lang string) string {
	parts := strings.Split(shenName, "-")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}

// buildConstructor returns a human-readable constructor signature string.
// For constrained/guarded types (those with verified premises), the
// constructor returns an error alongside the value.
func buildConstructor(targetName string, fieldTypes []string, withError bool, lang string) string {
	var params []string
	for _, ft := range fieldTypes {
		params = append(params, targetType(ft, lang))
	}
	paramList := strings.Join(params, ", ")

	switch lang {
	case "ts":
		ret := targetName
		if withError {
			return fmt.Sprintf("new%s(%s): %s | Error", targetName, paramList, ret)
		}
		return fmt.Sprintf("new%s(%s): %s", targetName, paramList, ret)
	default: // go
		if withError {
			return fmt.Sprintf("New%s(%s) (%s, error)", targetName, paramList, targetName)
		}
		return fmt.Sprintf("New%s(%s) %s", targetName, paramList, targetName)
	}
}

// targetType maps a Shen type name to the target language's type name.
// Known user types are converted via toTargetName; primitives map to the
// language's native type; unknown forms fall back to the raw Shen text.
func targetType(shenType, lang string) string {
	shenType = strings.TrimSpace(shenType)
	// Handle "(list T)".
	if strings.HasPrefix(shenType, "(list ") && strings.HasSuffix(shenType, ")") {
		inner := strings.TrimSpace(shenType[6 : len(shenType)-1])
		innerT := targetType(inner, lang)
		switch lang {
		case "ts":
			return innerT + "[]"
		default:
			return "[]" + innerT
		}
	}
	switch shenType {
	case "number":
		if lang == "ts" {
			return "number"
		}
		return "float64"
	case "string":
		return "string"
	case "boolean":
		if lang == "ts" {
			return "boolean"
		}
		return "bool"
	}
	// User-defined type — convert kebab-case to target name.
	if strings.ContainsAny(shenType, "() ") {
		return shenType
	}
	return toTargetName(shenType, lang)
}

// allRune reports whether every rune in s equals r. Used to detect ===== and
// _____ separator lines.
func allRune(s string, r rune) bool {
	for _, c := range s {
		if c != r {
			return false
		}
	}
	return true
}
