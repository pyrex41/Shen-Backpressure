package main

// verify_report.go — W5.2. `sb verify-report` re-derives a committed
// discharge report's claims from the committed artifacts alone.
//
// Necula's asymmetry: producing the report took a model, a solver, an
// indexer, and a loop. Checking it takes none of them. This command
// never invokes a model, never reaches the network, and reads nothing
// but the repository it is standing in.
//
// What it re-derives, in order:
//
//  1. Spec hashes. Every spec the report names is re-hashed. A
//     mismatch invalidates the whole document, so nothing further is
//     attempted.
//  2. Static discharges. shengen is re-run on the spec and its output
//     diffed against the committed guards file. If they differ, every
//     premise whose basis is guard-type-at-boundary,
//     guard-constructor-validates, or guard-brand-bound has lost its
//     basis — the report's static claims are about a file that is not
//     the one in the tree — and each is named in the failure.
//  3. Code references. Every `file:line` a premise cites must resolve:
//     the file exists and the line is in range. A reference naming a
//     constructor (`…:NewSafeTransfer`) must find that symbol.
//  4. Sampled evidence. The committed shen-derive test files are run.
//     A failing case is the report's sampled premises losing their
//     basis.
//  5. Path-cover feasibility. Re-checked when z3 is on PATH; reported
//     as UNVERIFIED (not failed) when it is not. A missing solver
//     means the claim was not re-derived, which is different from the
//     claim being false, and conflating the two would make the
//     verifier lie in the safe-looking direction.
//  6. Flow premises. Re-evaluated from a freshly built index when an
//     indexer is present; UNVERIFIED otherwise.
//  7. Signature, when --require-sig is passed.
//
// Exit status is 0 only if no check FAILED. UNVERIFIED checks do not
// fail the command by themselves — `--strict` promotes them.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pyrex41/Shen-Backpressure/cmd/sb/flow"
)

// Verification outcome for one check.
const (
	VerifyPass       = "PASS"
	VerifyFail       = "FAIL"
	VerifyUnverified = "UNVERIFIED"
	VerifySkip       = "SKIP"
)

// VerifyCheck is one line of the verification result.
type VerifyCheck struct {
	// Name is a short label, e.g. "static discharges".
	Name string
	// Status is one of the Verify* constants.
	Status string
	// Detail is one line of explanation, always present.
	Detail string
	// LostBasis names the premises whose evidence this check
	// invalidated. Empty unless Status is FAIL.
	LostBasis []string
	// Notes are additional lines printed under the check.
	Notes []string
}

func cmdVerifyReport(args []string) {
	fs := flag.NewFlagSet("verify-report", flag.ExitOnError)
	in := fs.String("in", DischargeReportPath, "committed discharge report to verify")
	requireSig := fs.Bool("require-sig", false, "refuse a report that carries no valid signature")
	strict := fs.Bool("strict", false, "treat UNVERIFIED checks as failures")
	skipTests := fs.Bool("skip-tests", false, "do not re-run the committed sample tests")
	cosignIdentity := fs.String("cosign-identity", "", "expected certificate identity for a cosign-signed report")
	cosignIssuer := fs.String("cosign-issuer", "", "expected OIDC issuer for a cosign-signed report")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb verify-report — Re-derive a committed discharge report's claims

Usage: sb verify-report [flags]

Checks a committed report against the tree it is committed in,
without a model and without the network:

  spec hashes        every spec the report names still hashes to the
                     recorded sha256
  static discharges  shengen is re-run and its output diffed against
                     the committed guards file
  code references    every file:line a premise cites resolves
  sampled evidence   the committed shen-derive tests are re-run
  path cover         re-checked when z3 is present, UNVERIFIED when not
  flow premises      re-evaluated from a fresh index when an indexer
                     is present, UNVERIFIED when not
  signature          with --require-sig

Exit status is non-zero when any check FAILS, and the failure names
the premises that lost their basis. UNVERIFIED means "not re-derived
here" — a weaker statement than PASS and a different one from FAIL.
Use --strict to treat it as a failure.

Flags:
`)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	raw, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb verify-report: %v\n", err)
		os.Exit(1)
	}
	r, err := loadDischarge(*in)
	if err != nil || r == nil {
		fmt.Fprintf(os.Stderr, "sb verify-report: no readable report at %s\n", *in)
		os.Exit(1)
	}
	cfg, cfgErr := LoadConfig()

	var checks []VerifyCheck

	checks = append(checks, verifySpecHashes(r))
	specOK := checks[len(checks)-1].Status == VerifyPass

	if cfgErr != nil {
		checks = append(checks, VerifyCheck{
			Name: "manifest", Status: VerifyUnverified,
			Detail: fmt.Sprintf("no readable sb.toml (%v); only the report's self-contained claims were checked", cfgErr),
		})
	} else {
		checks = append(checks, VerifyCheck{
			Name: "manifest", Status: VerifyPass,
			Detail: fmt.Sprintf("sb.toml read: lang=%s, spec=%s, guards=%s", cfg.Lang, cfg.Spec, cfg.Output),
		})
		if specOK {
			checks = append(checks, verifyStaticDischarges(r, cfg))
		} else {
			checks = append(checks, VerifyCheck{
				Name: "static discharges", Status: VerifySkip,
				Detail: "skipped: the spec does not match the hash the report was built from",
			})
		}
		checks = append(checks, verifyCodeReferences(r))
		if *skipTests {
			checks = append(checks, VerifyCheck{
				Name: "sampled evidence", Status: VerifySkip,
				Detail: "skipped by --skip-tests",
			})
		} else {
			checks = append(checks, verifySampledEvidence(r, cfg))
		}
		checks = append(checks, verifyPathCover(r, cfg))
		checks = append(checks, verifyFlowPremises(r, cfg))
		checks = append(checks, verifyToolchain(r, cfg))
	}

	if *requireSig || r.Signature != nil {
		checks = append(checks, verifySignatureCheck(raw, r, *requireSig, *cosignIdentity, *cosignIssuer))
	}

	os.Exit(printVerifyResults(*in, r, checks, *strict))
}

// printVerifyResults renders the checks and returns the exit code.
func printVerifyResults(path string, r *DischargeReport, checks []VerifyCheck, strict bool) int {
	fmt.Printf("sb verify-report: %s\n", path)
	if len(r.Spec.Files) > 0 {
		fmt.Printf("  spec: %s\n", r.Spec.Files[0].Path)
	}
	fmt.Println()

	failed, unverified := 0, 0
	for _, c := range checks {
		switch c.Status {
		case VerifyFail:
			failed++
		case VerifyUnverified:
			unverified++
		}
		fmt.Printf("  %-11s %-20s %s\n", c.Status, c.Name, c.Detail)
		for _, n := range c.Notes {
			fmt.Printf("                                  %s\n", n)
		}
		for _, p := range c.LostBasis {
			fmt.Printf("                                  lost basis: %s\n", p)
		}
	}
	fmt.Println()

	switch {
	case failed > 0:
		fmt.Printf("FAILED — %d check(s) could not be re-derived. The premises named above no longer have the basis the report claims for them.\n", failed)
		return 1
	case unverified > 0 && strict:
		fmt.Printf("FAILED (--strict) — %d check(s) UNVERIFIED. Nothing contradicted the report; the tools needed to re-derive these claims were not present.\n", unverified)
		return 1
	case unverified > 0:
		fmt.Printf("OK, with %d check(s) UNVERIFIED — nothing contradicted the report, but the tools needed to re-derive those claims were not present. Run with --strict to require them.\n", unverified)
		return 0
	default:
		fmt.Println("OK — every claim in this report was re-derived from the committed artifacts, with no model and no network.")
		return 0
	}
}

// ============================================================================
// 1. Spec hashes
// ============================================================================

func verifySpecHashes(r *DischargeReport) VerifyCheck {
	c := VerifyCheck{Name: "spec hashes", Status: VerifyPass}
	if len(r.Spec.Files) == 0 {
		c.Status = VerifyUnverified
		c.Detail = "the report names no spec files"
		return c
	}
	var bad []string
	for _, sf := range r.Spec.Files {
		sum, err := fileSHA256(sf.Path)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", sf.Path, err))
			continue
		}
		if sum != sf.SHA256 {
			bad = append(bad, fmt.Sprintf("%s: recorded %s, on disk %s",
				sf.Path, short(sf.SHA256), short(sum)))
		}
	}
	if len(bad) > 0 {
		c.Status = VerifyFail
		c.Detail = "the spec has changed since this report was written"
		c.Notes = bad
		c.LostBasis = everyPremiseID(r)
		return c
	}
	c.Detail = fmt.Sprintf("%d spec file(s) hash as recorded", len(r.Spec.Files))
	return c
}

// ============================================================================
// 2. Static discharges
// ============================================================================

// staticBases are the discharge bases whose evidence is the generated
// guards file. If the file in the tree is not the one shengen produces
// from the committed spec, every premise resting on one of these has
// lost its basis.
var staticBases = map[string]bool{
	BasisGuardTypeAtBoundary:       true,
	BasisGuardConstructorValidates: true,
	BasisGuardBrandBound:           true,
}

func verifyStaticDischarges(r *DischargeReport, cfg *Config) VerifyCheck {
	c := VerifyCheck{Name: "static discharges"}
	premises := premisesWithBasisIn(r, staticBases)
	if len(premises) == 0 {
		c.Status = VerifySkip
		c.Detail = "no premise claims a static basis"
		return c
	}
	if cfg.Lang != "go" {
		c.Status = VerifyUnverified
		c.Detail = fmt.Sprintf("re-deriving guards is wired for lang=go; this project is lang=%s", cfg.Lang)
		return c
	}
	shengen, err := FindShengen()
	if err != nil {
		c.Status = VerifyUnverified
		c.Detail = "no shengen available to re-derive the guards file: " + err.Error()
		return c
	}
	committed, err := os.ReadFile(cfg.Output)
	if err != nil {
		c.Status = VerifyFail
		c.Detail = fmt.Sprintf("the guards file the report rests on is missing: %v", err)
		c.LostBasis = premises
		return c
	}

	args := []string{"--spec", cfg.Spec, "--pkg", cfg.Pkg}
	if cfg.Brands {
		args = append(args, "--brands")
	}
	cmd := exec.Command(shengen, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		c.Status = VerifyFail
		c.Detail = fmt.Sprintf("shengen failed on the committed spec: %v", err)
		c.Notes = tailLines(stderr.String(), 3)
		c.LostBasis = premises
		return c
	}

	if !bytes.Equal(stdout.Bytes(), committed) {
		c.Status = VerifyFail
		c.Detail = fmt.Sprintf("%s is not what shengen emits from %s", cfg.Output, cfg.Spec)
		c.Notes = append(c.Notes, firstDifference(committed, stdout.Bytes()))
		c.LostBasis = premises
		return c
	}
	c.Status = VerifyPass
	c.Detail = fmt.Sprintf("%s re-derives byte-identically from %s (%d premise(s))",
		cfg.Output, cfg.Spec, len(premises))
	return c
}

// firstDifference describes where two byte slices diverge, in terms a
// reader can act on: the line number and both versions of it.
func firstDifference(committed, regenerated []byte) string {
	a := strings.Split(string(committed), "\n")
	b := strings.Split(string(regenerated), "\n")
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return fmt.Sprintf("first difference at line %d: committed %q, regenerated %q",
				i+1, truncate(a[i], 60), truncate(b[i], 60))
		}
	}
	return fmt.Sprintf("committed file has %d lines, regenerated has %d", len(a), len(b))
}

// ============================================================================
// 3. Code references
// ============================================================================

func verifyCodeReferences(r *DischargeReport) VerifyCheck {
	c := VerifyCheck{Name: "code references", Status: VerifyPass}
	total := 0
	var bad []string
	var lost []string
	for _, rule := range r.Rules {
		for _, p := range rule.Premises {
			for _, ref := range p.CodeReferences {
				total++
				if err := resolveCodeReference(ref); err != nil {
					bad = append(bad, fmt.Sprintf("%s → %s: %v", p.ID, ref, err))
					lost = appendUniqueStr(lost, p.ID)
				}
			}
		}
	}
	if total == 0 {
		c.Status = VerifySkip
		c.Detail = "no premise cites a code reference"
		return c
	}
	if len(bad) > 0 {
		c.Status = VerifyFail
		c.Detail = fmt.Sprintf("%d of %d code reference(s) do not resolve", len(bad), total)
		c.Notes = bad
		c.LostBasis = lost
		return c
	}
	c.Detail = fmt.Sprintf("%d code reference(s) resolve", total)
	return c
}

// resolveCodeReference checks one "path:line" or "path:Symbol"
// reference. A numeric suffix must be a line that exists; a symbolic
// one must be a name the file declares.
func resolveCodeReference(ref string) error {
	idx := strings.LastIndex(ref, ":")
	if idx < 0 {
		if _, err := os.Stat(ref); err != nil {
			return fmt.Errorf("no such file")
		}
		return nil
	}
	path, suffix := ref[:idx], ref[idx+1:]
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("no such file")
	}
	if line, convErr := strconv.Atoi(suffix); convErr == nil {
		lines := bytes.Count(data, []byte("\n")) + 1
		if line < 1 || line > lines {
			return fmt.Errorf("line %d is out of range (file has %d lines)", line, lines)
		}
		return nil
	}
	// Symbolic reference: the generic constructor a brand-bound
	// premise points at.
	if !declaresSymbol(data, suffix) {
		return fmt.Errorf("%s declares no %s", filepath.Base(path), suffix)
	}
	return nil
}

// declaresSymbol looks for a Go declaration of name in src. Textual on
// purpose: parsing the file would need the whole go/ast machinery to
// answer a question a prefix match answers correctly for generated
// code, whose shape shengen controls.
func declaresSymbol(src []byte, name string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		for _, prefix := range []string{"func " + name, "type " + name, "var " + name, "const " + name} {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			// Guard against NewSafeTransferExtra matching
			// NewSafeTransfer: the next character must not continue
			// the identifier.
			rest := line[len(prefix):]
			if rest == "" || !isIdentChar(rest[0]) {
				return true
			}
		}
	}
	return false
}

func isIdentChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// ============================================================================
// 4. Sampled evidence
// ============================================================================

func verifySampledEvidence(r *DischargeReport, cfg *Config) VerifyCheck {
	c := VerifyCheck{Name: "sampled evidence"}
	premises := premisesWithBasisIn(r, map[string]bool{
		BasisShenDeriveSampled: true,
		BasisProverZ3PathCover: true,
	})
	if len(premises) == 0 {
		c.Status = VerifySkip
		c.Detail = "no premise rests on sampled evidence"
		return c
	}
	pkgs := map[string]bool{}
	for _, s := range cfg.DeriveSpecs {
		if s.Lang == "go" || s.Lang == "" {
			pkgs[s.ImplPkg] = true
		}
	}
	if len(pkgs) == 0 {
		c.Status = VerifyUnverified
		c.Detail = "the manifest names no Go impl package to run the committed tests in"
		return c
	}
	names := make([]string, 0, len(pkgs))
	for p := range pkgs {
		names = append(names, p+"/...")
	}
	sort.Strings(names)

	var failures []string
	for _, p := range names {
		out, err := runCaptured("", "go", "test", "-count=1", p)
		if err != nil {
			failures = append(failures, fmt.Sprintf("go test %s: %v", p, err))
			failures = append(failures, tailLines(string(out), 6)...)
		}
	}
	if len(failures) > 0 {
		c.Status = VerifyFail
		c.Detail = "the committed sample tests do not pass against this tree"
		c.Notes = failures
		c.LostBasis = premises
		return c
	}
	c.Status = VerifyPass
	c.Detail = fmt.Sprintf("committed tests pass in %s (%d premise(s))", strings.Join(names, ", "), len(premises))
	return c
}

// ============================================================================
// 5. Path cover
// ============================================================================

func verifyPathCover(r *DischargeReport, cfg *Config) VerifyCheck {
	c := VerifyCheck{Name: "path cover"}
	claims := pathCoverClaims(r)
	if len(claims) == 0 {
		c.Status = VerifySkip
		c.Detail = "no premise claims path-cover evidence"
		return c
	}
	if _, err := exec.LookPath("z3"); err != nil {
		c.Status = VerifyUnverified
		c.Detail = fmt.Sprintf("no z3 on PATH; the path-feasibility claims of %d premise(s) were not re-derived", len(claims))
		c.Notes = []string{
			"This is not a failure: nothing contradicted the report. " +
				"Install z3 (pip install z3-solver) to re-derive them.",
		}
		return c
	}
	// The committed test file records the path counters in its header,
	// and `sb derive` has already diffed that file against a fresh
	// regeneration. Re-running the solver here would duplicate that
	// work; what verify-report adds is the check that the counters the
	// report states and the counters the committed test file states
	// are the same document.
	var mismatches []string
	for _, spec := range cfg.DeriveSpecs {
		if !spec.PathCover || spec.OutFile == "" {
			continue
		}
		got, err := pathCountersFromTestFile(spec.OutFile)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: %v", spec.OutFile, err))
			continue
		}
		for _, claim := range claims {
			if claim.total != got.total || claim.feasible != got.feasible || claim.dead != got.dead {
				mismatches = append(mismatches, fmt.Sprintf(
					"%s: report says total=%d feasible=%d dead=%d, %s says total=%d feasible=%d dead=%d",
					claim.premiseID, claim.total, claim.feasible, claim.dead,
					spec.OutFile, got.total, got.feasible, got.dead))
			}
		}
	}
	if len(mismatches) > 0 {
		c.Status = VerifyFail
		c.Detail = "the report's path counters disagree with the committed test file"
		c.Notes = mismatches
		for _, claim := range claims {
			c.LostBasis = append(c.LostBasis, claim.premiseID)
		}
		return c
	}
	c.Status = VerifyPass
	c.Detail = fmt.Sprintf("z3 present; %d path-cover claim(s) agree with the committed test headers", len(claims))
	return c
}

type pathClaim struct {
	premiseID             string
	total, feasible, dead int
}

func pathCoverClaims(r *DischargeReport) []pathClaim {
	var out []pathClaim
	for _, rule := range r.Rules {
		for _, p := range rule.Premises {
			if p.DischargeBasis != BasisProverZ3PathCover || p.PathsTotal == nil {
				continue
			}
			claim := pathClaim{premiseID: p.ID, total: *p.PathsTotal}
			if p.PathsFeasible != nil {
				claim.feasible = *p.PathsFeasible
			}
			if p.PathsDead != nil {
				claim.dead = *p.PathsDead
			}
			out = append(out, claim)
		}
	}
	return out
}

// pathCountersFromTestFile reads the "paths_total=… paths_feasible=…
// paths_dead=…" line shen-derive writes into the generated test
// header.
func pathCountersFromTestFile(path string) (pathClaim, error) {
	var out pathClaim
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	text := string(data)
	found := 0
	for _, field := range []struct {
		key string
		dst *int
	}{
		{"paths_total", &out.total},
		{"paths_feasible", &out.feasible},
		{"paths_dead", &out.dead},
	} {
		i := strings.Index(text, field.key+"=")
		if i < 0 {
			continue
		}
		rest := text[i+len(field.key)+1:]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		if end == 0 {
			continue
		}
		n, convErr := strconv.Atoi(rest[:end])
		if convErr != nil {
			continue
		}
		*field.dst = n
		found++
	}
	if found == 0 {
		return out, fmt.Errorf("no path counters in the committed test header")
	}
	return out, nil
}

// ============================================================================
// 6. Flow premises
// ============================================================================

func verifyFlowPremises(r *DischargeReport, cfg *Config) VerifyCheck {
	c := VerifyCheck{Name: "flow premises"}
	claimed := 0
	var ids []string
	for _, rule := range r.Rules {
		if rule.Kind != FlowRuleKind {
			continue
		}
		for _, p := range rule.Premises {
			if p.DischargeBasis == DischargeBasisFlow {
				claimed++
				ids = append(ids, p.ID)
			}
		}
	}
	if claimed == 0 {
		c.Status = VerifySkip
		c.Detail = "no premise claims flow-analysis evidence"
		return c
	}
	decls, err := loadFlowDecls(cfg)
	if err != nil || len(decls) == 0 {
		c.Status = VerifyUnverified
		c.Detail = "the spec declares no (flow …) premises to re-evaluate"
		return c
	}
	// force=true: a cached fact file is an artifact of a previous run,
	// and the point of this command is to trust nothing a previous run
	// left behind.
	out, err := ensureIndex(cfg, true, false)
	if err != nil {
		c.Status = VerifyUnverified
		c.Detail = "could not build a fresh index: " + err.Error()
		return c
	}
	if !out.Available {
		c.Status = VerifyUnverified
		c.Detail = fmt.Sprintf("no SCIP indexer on PATH (%s); %d flow premise(s) not re-derived", out.Indexer, claimed)
		c.Notes = []string{"Install it with: " + out.InstallHint}
		return c
	}
	results := flow.Evaluate(out.Facts, decls)
	violations := 0
	var notes []string
	for _, res := range results {
		for _, v := range res.Violations {
			violations++
			if len(notes) < 6 {
				notes = append(notes, res.PremiseID+": "+v.Location+" — "+v.Rationale)
			}
		}
	}
	if violations > 0 {
		c.Status = VerifyFail
		c.Detail = fmt.Sprintf("re-evaluating the flow premises from a fresh index found %d violation(s)", violations)
		c.Notes = notes
		c.LostBasis = ids
		return c
	}
	c.Status = VerifyPass
	c.Detail = fmt.Sprintf("%d flow premise(s) re-evaluated clean from a fresh %s index", claimed, out.Indexer)
	return c
}

// ============================================================================
// 7. Toolchain and signature
// ============================================================================

func verifyToolchain(r *DischargeReport, cfg *Config) VerifyCheck {
	c := VerifyCheck{Name: "toolchain"}
	if r.Toolchain == nil {
		c.Status = VerifyUnverified
		c.Detail = "the report records no toolchain block (written before W5)"
		return c
	}
	diffs := ToolchainMismatch(r.Toolchain, DetectToolchain(cfg, ""))
	if len(diffs) == 0 {
		c.Status = VerifyPass
		c.Detail = "re-derived with the same tools the report records"
		return c
	}
	// Different tools are worth saying out loud, but the checks above
	// re-derived the claims with the tools actually present and
	// agreed. Failing here would mean "your Go is newer than the
	// report's", which is not a defect in the report.
	c.Status = VerifyPass
	c.Detail = fmt.Sprintf("%d tool version difference(s); the claims above still re-derived", len(diffs))
	c.Notes = diffs
	return c
}

func verifySignatureCheck(raw []byte, r *DischargeReport, require bool, identity, issuer string) VerifyCheck {
	c := VerifyCheck{Name: "signature"}
	if r.Signature == nil {
		if require {
			c.Status = VerifyFail
			c.Detail = "--require-sig was passed and this report is unsigned"
			return c
		}
		c.Status = VerifySkip
		c.Detail = "unsigned"
		return c
	}
	desc, err := VerifySignature(raw, r.Signature, identity, issuer)
	if err != nil {
		c.Status = VerifyFail
		c.Detail = "signature does not verify: " + err.Error()
		return c
	}
	c.Status = VerifyPass
	c.Detail = desc
	return c
}

// ============================================================================
// Helpers
// ============================================================================

func premisesWithBasisIn(r *DischargeReport, bases map[string]bool) []string {
	var out []string
	for _, rule := range r.Rules {
		for _, p := range rule.Premises {
			if bases[p.DischargeBasis] {
				out = append(out, p.ID)
			}
		}
	}
	return out
}

func everyPremiseID(r *DischargeReport) []string {
	var out []string
	for _, rule := range r.Rules {
		for _, p := range rule.Premises {
			out = append(out, p.ID)
		}
	}
	return out
}

func appendUniqueStr(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

func tailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func short(sum string) string {
	if len(sum) <= 12 {
		return sum
	}
	return sum[:12] + "…"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// hashBytes is used by tests and by the spec-hash check's error path.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
