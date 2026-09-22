package main

// certificate_test.go — W5.2/W5.3. Tests for the canonicalization,
// the signature pair, and the pieces of `sb verify-report` that do not
// need a project on disk.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCanonicalIgnoresFormatting is the property the whole signing
// scheme rests on: two files that say the same thing canonicalise to
// the same bytes, however they are written.
func TestCanonicalIgnoresFormatting(t *testing.T) {
	pretty := []byte(`{
	  "schema_version": 1,
	  "rules": [ {"name": "a"}, {"name": "b"} ],
	  "summary": {"rule_count": 2}
	}`)
	compactReordered := []byte(`{"summary":{"rule_count":2},"rules":[{"name":"a"},{"name":"b"}],"schema_version":1}`)

	a, err := CanonicalReportBytes(pretty)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalReportBytes(compactReordered)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Errorf("formatting changed the canonical bytes:\n  %s\n  %s", a, b)
	}
	if !strings.HasPrefix(string(a), `{"rules":`) {
		t.Errorf("keys are not sorted: %s", a)
	}
}

// TestCanonicalArrayOrderIsSignificant — arrays are data, not
// key/value pairs, so reordering them must change the bytes.
func TestCanonicalArrayOrderIsSignificant(t *testing.T) {
	a, _ := CanonicalReportBytes([]byte(`{"rules":[{"name":"a"},{"name":"b"}]}`))
	b, _ := CanonicalReportBytes([]byte(`{"rules":[{"name":"b"},{"name":"a"}]}`))
	if string(a) == string(b) {
		t.Error("reordering the rules array must change the canonical bytes")
	}
}

// TestCanonicalExcludesSignature is why the signature is a fixed
// point: signing a document and then hashing it again must give the
// bytes that were signed.
func TestCanonicalExcludesSignature(t *testing.T) {
	unsigned := []byte(`{"schema_version":1,"signature":null}`)
	signed := []byte(`{"schema_version":1,"signature":{"algorithm":"ed25519","key_id":"k","value":"v"}}`)
	a, err := CanonicalReportBytes(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalReportBytes(signed)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Errorf("the signature member must not affect the canonical bytes:\n  %s\n  %s", a, b)
	}
}

// TestCanonicalPreservesNumberDigits — a number must survive
// canonicalization exactly as written, with no float round trip.
func TestCanonicalPreservesNumberDigits(t *testing.T) {
	out, err := CanonicalReportBytes([]byte(`{"n":12345678901234567890,"f":1.50}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "12345678901234567890") {
		t.Errorf("large integer was perturbed: %s", out)
	}
	if !strings.Contains(string(out), "1.50") {
		t.Errorf("decimal digits were perturbed: %s", out)
	}
}

// TestSignAndVerifyRoundTrip covers the whole key-file path: generate,
// sign, verify.
func TestSignAndVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.json")
	if err := generateSigningKey(keyPath); err != nil {
		t.Fatal(err)
	}

	report := &DischargeReport{
		SchemaVersion: 1,
		GeneratedAt:   "2026-09-22T00:00:00Z",
		Rules:         []DischargeRule{{Name: "safe-transfer", Status: DischargeStatusDischarged}},
	}
	canonical, err := CanonicalReportBytesOf(report)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := ed25519Sign(canonical, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Canonicalization != CanonicalizationName || sig.Mode != SigModeKeyFile {
		t.Errorf("signature does not record its scheme: %+v", sig)
	}
	report.Signature = sig

	signed, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := VerifySignature(signed, sig, "", "")
	if err != nil {
		t.Fatalf("a freshly signed report must verify: %v", err)
	}
	if !strings.Contains(desc, "ed25519") {
		t.Errorf("verification description = %q", desc)
	}

	// One byte of a claim changes, and the signature must stop
	// verifying. This is the whole point.
	tampered := strings.Replace(string(signed), `"discharged"`, `"violated"`, 1)
	if tampered == string(signed) {
		t.Fatal("test setup: nothing was tampered")
	}
	if _, err := VerifySignature([]byte(tampered), sig, "", ""); err == nil {
		t.Error("a tampered report must not verify")
	}

	// Re-indenting must NOT break it.
	var generic map[string]any
	if err := json.Unmarshal(signed, &generic); err != nil {
		t.Fatal(err)
	}
	reindented, err := json.MarshalIndent(generic, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySignature(reindented, sig, "", ""); err != nil {
		t.Errorf("re-indenting must not break the signature: %v", err)
	}
}

func TestGenerateSigningKeyRefusesToOverwrite(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "key.json")
	if err := generateSigningKey(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := generateSigningKey(keyPath); err == nil {
		t.Error("overwriting an existing signing key must be refused")
	}
}

func TestVerifySignatureRejectsUnknownCanonicalization(t *testing.T) {
	sig := &DischargeSignature{
		Algorithm:        SigAlgorithmEd25519,
		Canonicalization: "some-future-scheme-v9",
	}
	_, err := VerifySignature([]byte(`{}`), sig, "", "")
	if err == nil || !strings.Contains(err.Error(), "canonicalization") {
		t.Errorf("want a canonicalization mismatch error, got %v", err)
	}
}

// ============================================================================
// verify-report checks
// ============================================================================

func TestVerifySpecHashesNamesEveryPremiseWhenTheSpecMoved(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "core.shen")
	if err := os.WriteFile(spec, []byte("(datatype foo)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &DischargeReport{
		Spec: DischargeSpec{Files: []DischargeSpecFile{{Path: spec, SHA256: "deadbeef"}}},
		Rules: []DischargeRule{{
			Name:     "foo",
			Premises: []DischargePremise{{ID: "foo.field-x"}, {ID: "foo.verified-y"}},
		}},
	}
	c := verifySpecHashes(r)
	if c.Status != VerifyFail {
		t.Fatalf("a changed spec must fail: %+v", c)
	}
	// Every premise rests on the spec, so every premise lost its basis.
	if len(c.LostBasis) != 2 {
		t.Errorf("want both premises named, got %v", c.LostBasis)
	}
}

func TestResolveCodeReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guards_gen.go")
	body := "package shenguard\n\ntype SafeTransfer[B brand] struct{}\n\nfunc NewSafeTransfer[B brand]() SafeTransfer[B] { return SafeTransfer[B]{} }\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := []string{path + ":3", path + ":NewSafeTransfer", path + ":SafeTransfer"}
	for _, ref := range ok {
		if err := resolveCodeReference(ref); err != nil {
			t.Errorf("%s should resolve: %v", ref, err)
		}
	}
	bad := []string{
		path + ":99",                  // past the end of the file
		path + ":NewTenantAccess",     // no such symbol
		filepath.Join(dir, "no.go:1"), // no such file
	}
	for _, ref := range bad {
		if err := resolveCodeReference(ref); err == nil {
			t.Errorf("%s should not resolve", ref)
		}
	}
}

// TestDeclaresSymbolDoesNotMatchAPrefix — NewSafeTransferExtra is not
// NewSafeTransfer.
func TestDeclaresSymbolDoesNotMatchAPrefix(t *testing.T) {
	src := []byte("func NewSafeTransferExtra() {}\n")
	if declaresSymbol(src, "NewSafeTransfer") {
		t.Error("a prefix match must not count as a declaration")
	}
	if !declaresSymbol(src, "NewSafeTransferExtra") {
		t.Error("the exact name must match")
	}
}

func TestPathCountersFromTestFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec_test.go")
	body := "// paths_total=10 paths_feasible=9 paths_dead=1 (solver: z3)\npackage derived\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := pathCountersFromTestFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.total != 10 || got.feasible != 9 || got.dead != 1 {
		t.Errorf("counters = %+v", got)
	}

	bare := filepath.Join(dir, "bare_test.go")
	if err := os.WriteFile(bare, []byte("package derived\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := pathCountersFromTestFile(bare); err == nil {
		t.Error("a file with no counters should be an error, not silent zeros")
	}
}

// ============================================================================
// precision and blame
// ============================================================================

func TestPrecisionForBasisMirrorsShenDerive(t *testing.T) {
	cases := []struct{ discharge, basis, want string }{
		{DischargeStatic, BasisGuardBrandBound, PrecisionStatic},
		{DischargeStatic, BasisGuardTypeAtBoundary, PrecisionStatic},
		{DischargeRuntimeSampled, BasisProverZ3PathCover, PrecisionPathCover},
		{DischargeRuntimeSampled, BasisShenDeriveSampled, PrecisionSampled},
		{DischargeRuntimeEvaluator, "runtime-via-evaluator", PrecisionRuntime},
		{DischargeStatic, DischargeBasisFlow, PrecisionStatic},
		// A flow premise the analysis refuted keeps the flow basis but
		// has no evidence at all.
		{DischargeUnproven, DischargeBasisFlow, PrecisionUnproven},
		{DischargeUnproven, DischargeBasisGrepFallback, PrecisionUnproven},
		{DischargeUnproven, BasisVacuousDatatype, PrecisionUnproven},
	}
	for _, c := range cases {
		if got := precisionForBasis(c.discharge, c.basis); got != c.want {
			t.Errorf("precisionForBasis(%q, %q) = %q, want %q", c.discharge, c.basis, got, c.want)
		}
	}
}

func TestWeakestPrecisionLeadsTheSummary(t *testing.T) {
	r := &DischargeReport{Rules: []DischargeRule{
		{Premises: []DischargePremise{{Precision: PrecisionStatic}}},
		{Premises: []DischargePremise{{Precision: PrecisionPathCover}, {Precision: PrecisionUnproven}}},
	}}
	if got := weakestPrecision(r); got != PrecisionUnproven {
		t.Errorf("weakestPrecision = %q, want unproven", got)
	}
}

func TestBlameForFollowsTheRoadmapOrder(t *testing.T) {
	cases := []struct {
		vacuous, runtimeVia, host bool
		blame, basis              string
	}{
		{true, false, false, BlameSpec, BlameBasisVacuous},
		{true, true, true, BlameSpec, BlameBasisVacuous},
		{false, true, true, BlameWrapper, BlameBasisRuntimeVia},
		{false, false, true, BlameImpl, BlameBasisEvaluatorAndHost},
		{false, false, false, BlameImpl, BlameBasisEvaluatorOnly},
	}
	for _, c := range cases {
		blame, basis := blameFor(c.vacuous, c.runtimeVia, c.host)
		if blame != c.blame || basis != c.basis {
			t.Errorf("blameFor(%v,%v,%v) = (%q,%q), want (%q,%q)",
				c.vacuous, c.runtimeVia, c.host, blame, basis, c.blame, c.basis)
		}
	}
}

// TestAssignBlameKeepsAnExistingAssignment — the flow gate blames its
// own violations at evaluation time, where it knows more.
func TestAssignBlameKeepsAnExistingAssignment(t *testing.T) {
	r := &DischargeReport{Rules: []DischargeRule{{
		Name:   "tenant-access-discipline",
		Status: DischargeStatusViolated,
		CounterExamples: []DischargeCounter{
			{CaseID: "flow_01", Blame: BlameImpl, BlameBasis: BlameBasisFlowAnalysis},
			{CaseID: "case_07"},
		},
	}}}
	assignBlame(r, false)
	if r.Rules[0].CounterExamples[0].BlameBasis != BlameBasisFlowAnalysis {
		t.Error("an existing blame basis must not be overwritten")
	}
	if r.Rules[0].CounterExamples[1].BlameBasis != BlameBasisEvaluatorOnly {
		t.Errorf("an unassigned counter-example must be blamed: %+v", r.Rules[0].CounterExamples[1])
	}
}

func TestBlameSummaryLine(t *testing.T) {
	r := &DischargeReport{Rules: []DischargeRule{{
		CounterExamples: []DischargeCounter{{Blame: BlameImpl, BlameBasis: BlameBasisEvaluatorOnly}},
	}}}
	line := blameSummaryLine(r)
	if !strings.HasPrefix(line, "BLAME: impl") {
		t.Errorf("summary should lead with the blamed party: %q", line)
	}
	if !strings.Contains(line, BlameBasisEvaluatorOnly) {
		t.Errorf("summary should name the basis: %q", line)
	}
	if blameSummaryLine(&DischargeReport{}) != "" {
		t.Error("a clean report has no blame line")
	}
}

// TestToolchainMismatch reports differences without inventing them.
func TestToolchainMismatch(t *testing.T) {
	a := &DischargeToolchain{Go: "go1.24.7", ShengenSHA256: "aaa"}
	b := &DischargeToolchain{Go: "go1.25.0", ShengenSHA256: "aaa"}
	diffs := ToolchainMismatch(a, b)
	if len(diffs) != 1 || !strings.Contains(diffs[0], "go1.25.0") {
		t.Errorf("want one go difference, got %v", diffs)
	}
	// An absent field on either side is not a difference: the report
	// does not know, which is not the same as disagreeing.
	if d := ToolchainMismatch(a, &DischargeToolchain{}); len(d) != 0 {
		t.Errorf("an empty toolchain should report no differences, got %v", d)
	}
	if d := ToolchainMismatch(nil, b); len(d) != 0 {
		t.Errorf("a nil recorded toolchain should report no differences, got %v", d)
	}
}
