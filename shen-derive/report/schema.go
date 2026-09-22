// Package report defines the discharge-report schema and the
// classifier/emitter that produces it from a parsed Shen spec and a
// shen-derive verification harness.
//
// The discharge report is a structured artifact written on every
// gate run that distinguishes how each premise of each Shen rule was
// discharged: statically (via guard types), via runtime sampling
// (shen-derive's spec-equivalence pool), or unproven. It feeds two
// readers: the agent loop (terse Markdown via `sb context`) and a
// human auditor (long-form Markdown via `sb audit-report`).
//
// Schema is locked at SchemaVersion=1. Evolution must be additive.
// See thoughts/shared/research/2026-05-05-discharge-report-schema.md
// for the rationale.
package report

// SchemaVersion is the discharge report schema version. v0 of the
// feature emits version 1; future additive fields keep the same
// version. Renaming or removing fields requires a bump.
const SchemaVersion = 1

// Report is the top-level structure. Marshals to JSON in the field
// order declared here. JSON consumers should ignore fields they don't
// recognise so that additive changes don't break them.
type Report struct {
	SchemaVersion int        `json:"schema_version"`
	GeneratedAt   string     `json:"generated_at"`
	Spec          SpecInfo   `json:"spec"`
	Impl          ImplInfo   `json:"impl"`
	Tools         ToolsInfo  `json:"tools"`
	Rules         []Rule     `json:"rules"`
	Summary       Summary    `json:"summary"`

	// ---- W5 certificate block (additive, v1.x) -------------------
	// Toolchain records exactly which binaries produced this report,
	// so `sb verify-report` can say whether it is re-deriving with the
	// same tools or merely with compatible ones. omitempty keeps
	// pre-W5 reports byte-identical.
	Toolchain *Toolchain `json:"toolchain,omitempty"`
	// --------------------------------------------------------------

	Signature *Signature `json:"signature"` // null until `sb sign-report`
}

// ---- W5 certificate block (additive, v1.x) ----------------------

// Toolchain identifies the tools that produced a report. It is the
// reproducibility half of the certificate: `sb gen` output is a pure
// function of the spec bytes and the shengen binary, so recording the
// binary's hash pins the function. Every field is optional — a report
// produced without one of these tools simply omits it rather than
// claiming a version it does not know.
type Toolchain struct {
	// Go is the Go toolchain that built the binaries, e.g. "go1.24.7".
	Go string `json:"go,omitempty"`
	// GOOS / GOARCH matter because the generated code is
	// platform-independent but the binary hash is not.
	GOOS   string `json:"goos,omitempty"`
	GOARCH string `json:"goarch,omitempty"`
	// ShengenVersion and ShengenSHA256 identify the emitter. The hash
	// is over the binary bytes; with -trimpath it is stable across
	// checkout locations.
	ShengenVersion string `json:"shengen_version,omitempty"`
	ShengenSHA256  string `json:"shengen_sha256,omitempty"`
	// ShengenTSVersion is the TypeScript emitter's version, when the
	// project targets TypeScript.
	ShengenTSVersion string `json:"shengen_ts_version,omitempty"`
	// Z3Version is recorded only when a solver actually answered, so
	// an absent field means path-cover evidence was not prover-backed.
	Z3Version string `json:"z3_version,omitempty"`
	// Indexers are the SCIP indexers the flow gate used, in the order
	// they ran. Absent when no flow premise was evaluated.
	Indexers []ToolVersion `json:"indexers,omitempty"`
}

// ToolVersion is a named external tool and the version string it
// reported. Recorded verbatim: sb does not parse it.
type ToolVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// -----------------------------------------------------------------

// SpecInfo identifies the spec file(s) the report covers. v0 supports
// a single spec but the slice shape is reserved for multi-spec
// projects.
type SpecInfo struct {
	Files     []SpecFile `json:"files"`
	RuleCount int        `json:"rule_count"`
}

// SpecFile records one spec file's path (repo-relative) and its
// SHA-256 hash. Hashes let stored reports be tied to exact spec bytes.
type SpecFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// ImplInfo describes the implementation under verification. git_commit
// may be null when the project is not in a git repo.
type ImplInfo struct {
	GitCommit       *string  `json:"git_commit"`
	GitDirty        *bool    `json:"git_dirty"`
	TargetLanguages []string `json:"target_languages"`
}

// ToolsInfo records which tools produced this report. Useful to
// reproduce a historical run with the same toolchain.
type ToolsInfo struct {
	SBVersion            string  `json:"sb_version"`
	ShenDeriveVersion    string  `json:"shen_derive_version"`
	ShengenVersion       string  `json:"shengen_version"`
	ShenRuntime          *string `json:"shen_runtime"`
	ShenRuntimeAvailable bool    `json:"shen_runtime_available"`
}

// Rule is one Shen (datatype …) or (define …) block, with its
// per-premise discharge classification and any counter-examples.
type Rule struct {
	Name                    string           `json:"name"`
	Kind                    string           `json:"kind"`
	SpecFile                string           `json:"spec_file"`
	SpecExcerpt             string           `json:"spec_excerpt"`
	HumanDescription        string           `json:"human_description"`
	HumanDescriptionSource  string           `json:"human_description_source"`
	Premises                []Premise        `json:"premises"`
	Status                  string           `json:"status"`
	// VacuityMessage explains a "vacuous" status in plain English:
	// which datatype is uninhabited and which premises contradict.
	// Additive (v1.x); omitted for every other status.
	VacuityMessage          string           `json:"vacuity_message,omitempty"`
	DischargedSinceCommit   *string          `json:"discharged_since_commit"`
	CounterExamples         []CounterExample `json:"counter_examples"`
}

// Status values for a rule.
const (
	StatusDischarged = "discharged"
	StatusViolated   = "violated"
	StatusUnproven   = "unproven"

	// StatusVacuous marks a rule whose datatype is uninhabited: the
	// conjunction of its verified premises is unsatisfiable, so no
	// value of the type can exist and every downstream claim resting
	// on it is empty. A vacuous rule fails the gate — an uninhabited
	// guard proves nothing. Additive (v1.x).
	StatusVacuous = "vacuous"
)

// HumanDescriptionSource values.
const (
	HumanDescriptionFromDoc           = ":doc"
	HumanDescriptionAutoGenerated     = "auto-generated"
)

// Premise is one obligation of a rule and how it's discharged.
//
// The runtime-via fields (RuntimeProfile, RuntimeChecker,
// EquivalenceTest, DBQueryExcerpt) are populated only for premises
// carrying a `:runtime-via` annotation. They are additive — premises
// discharged statically or by sampling omit them entirely, so reports
// for projects with no runtime-via are byte-identical to pre-runtime-via
// output. See docs/RUNTIME-VIA.md for the four profiles (A–D).
type Premise struct {
	ID             string   `json:"id"`
	Expression     string   `json:"expression"`
	Discharge      string   `json:"discharge"`
	DischargeBasis string   `json:"discharge_basis"`
	Rationale      string   `json:"rationale"`
	CodeReferences []string `json:"code_references,omitempty"`
	SamplesPassed  int      `json:"samples_passed"`
	SamplesFailed  int      `json:"samples_failed"`
	SampleSeed     *string  `json:"sample_seed"`

	// Path-cover counters (additive, v1.x). Populated only when
	// shen-derive's path sampler ran for this premise: PathsTotal is
	// every path the symbolic evaluator enumerated at the configured
	// unroll depth, PathsFeasible those the solver produced a witness
	// for (each one a committed test case), and PathsDead those whose
	// path condition is unsatisfiable. Total minus feasible minus dead
	// is the number the solver could not decide. Pointers so "zero
	// paths" stays distinguishable from "path cover did not run".
	PathsTotal    *int `json:"paths_total,omitempty"`
	PathsFeasible *int `json:"paths_feasible,omitempty"`
	PathsDead     *int `json:"paths_dead,omitempty"`

	// ---- W5 certificate block (additive, v1.x) -------------------

	// Precision names the strength of the evidence behind this
	// premise, on the total order in PrecisionRank. It is derived
	// from Discharge and DischargeBasis rather than supplied
	// independently, so it can never disagree with them; it exists
	// because "static vs sampled vs unproven" is the comparison a
	// reader actually wants and neither field alone expresses it.
	Precision string `json:"precision,omitempty"`

	// BrandSignature is the generic constructor signature that binds
	// this premise to its conclusion, e.g. "SafeTransfer[B]". Present
	// only on premises with the guard-brand-bound basis.
	BrandSignature string `json:"brand_signature,omitempty"`

	// --------------------------------------------------------------

	// RuntimeProfile is "A" | "B" | "C" | "D" for runtime-via premises,
	// "" otherwise. Rendered in the audit report as the human-facing
	// profile label (see RuntimeProfileLabel).
	RuntimeProfile string `json:"runtime_profile,omitempty"`
	// RuntimeChecker names the bound checker function for profiles
	// A, C, and D. Nil for profile B (the evaluator is the checker).
	RuntimeChecker *string `json:"runtime_checker,omitempty"`
	// EquivalenceTest is the relative path to the generated
	// sampled-equivalence test that pins the bespoke checker to the
	// spec predicate. Profile C only.
	EquivalenceTest *string `json:"equivalence_test,omitempty"`
	// DBQueryExcerpt is the SQL (or other query) the DB-attested
	// checker registered at gate time, surfaced so an auditor sees the
	// query without leaving the report. Profile D only.
	DBQueryExcerpt *string `json:"db_query_excerpt,omitempty"`
}

// Discharge values.
const (
	DischargeStatic         = "static"
	DischargeRuntimeSampled = "runtime-sample"
	DischargeUnproven       = "unproven"

	// Runtime-via discharge values (additive, v1). Each corresponds to
	// one of the four profiles in docs/RUNTIME-VIA.md.
	DischargeRuntimeAttested        = "runtime-attested"         // Profile A: bespoke checker, no oracle
	DischargeRuntimeEvaluator       = "runtime-evaluator"        // Profile B: evaluator-hosted from spec
	DischargeRuntimeAttestedSampled = "runtime-attested-sampled" // Profile C: bespoke + sampled-equivalence
	DischargeRuntimeAttestedDB      = "runtime-attested-db"      // Profile D: DB-attested
)

// DischargeBasis values used in v0.
const (
	BasisGuardTypeAtBoundary     = "guard-type-at-boundary"
	BasisGuardConstructorValidates = "guard-constructor-validates"
	BasisShenDeriveSampled         = "shen-derive-sampled"
	BasisNotDischarged             = "not-discharged"

	// BasisProverZ3PathCover is emitted when shen-derive's path
	// sampler ran with a solver: the evidence is one concrete case per
	// feasible path of the spec, plus the boundary pool. The schema
	// memo reserved the `prover-z3` family for exactly this.
	BasisProverZ3PathCover = "prover-z3-path-cover"

	// BasisVacuousDatatype is recorded on the premises of an
	// uninhabited rule.
	BasisVacuousDatatype = "vacuous-datatype"

	// BasisGuardBrandBound is emitted when W1's brand inference pairs
	// this premise with the rule's conclusion (or with a sibling
	// premise) through a shared phantom brand parameter. The claim is
	// narrower and stronger than guard-type-at-boundary: not merely
	// "a proof of the right type", but "a proof about this very
	// value". A caller cannot satisfy it with evidence minted for a
	// different subject, because the brand is an unexported type it
	// cannot name. Additive (v1.x).
	BasisGuardBrandBound = "guard-brand-bound"

	// Runtime-via discharge bases (additive, v1).
	BasisRuntimeViaWitness            = "runtime-via-witness"             // Profile A
	BasisRuntimeViaEvaluator          = "runtime-via-evaluator"          // Profile B
	BasisRuntimeViaSampledEquivalence = "runtime-via-sampled-equivalence" // Profile C
	BasisRuntimeViaDBAttested         = "runtime-via-db-attested"        // Profile D
)

// ---- W5 certificate block (additive, v1.x) ----------------------

// Precision values, strongest first. The order is total: a report
// consumer can compare two premises by PrecisionRank and get a
// well-defined answer about which rests on stronger evidence.
const (
	// PrecisionStatic — the target language's type system refuses to
	// compile a violating program. No execution required.
	PrecisionStatic = "static"
	// PrecisionPathCover — a solver produced a concrete witness for
	// every feasible path of the spec at the configured unroll depth,
	// and the impl agreed on each. Bounded, but nothing inside the
	// bound went unexercised.
	PrecisionPathCover = "path-cover"
	// PrecisionSampled — the impl agreed with the spec on a
	// deterministic pool of inputs. Evidence, not proof.
	PrecisionSampled = "sampled"
	// PrecisionRuntime — the check happens in production, on the
	// value actually in hand. Strong about that value, silent about
	// every value the program never sees.
	PrecisionRuntime = "runtime"
	// PrecisionUnproven — nothing was established.
	PrecisionUnproven = "unproven"
)

// PrecisionOrder lists the precision values strongest to weakest.
var PrecisionOrder = []string{
	PrecisionStatic,
	PrecisionPathCover,
	PrecisionSampled,
	PrecisionRuntime,
	PrecisionUnproven,
}

// PrecisionRank returns a premise precision's position in the total
// order: 0 is strongest, larger is weaker. An unrecognised value
// ranks below everything known, so a report written by a newer tool
// degrades to "at least as weak as unproven" rather than to a panic.
func PrecisionRank(p string) int {
	for i, v := range PrecisionOrder {
		if v == p {
			return i
		}
	}
	return len(PrecisionOrder)
}

// Blame values. Every counter-example names exactly one responsible
// party, because the next question a reader has is "whose bug is
// this?" and an unattributed failure makes them re-derive the answer
// by hand.
const (
	// BlameSpec — the Shen spec is wrong or empty: a vacuous rule, a
	// dead branch, a predicate that does not mean what its author
	// intended.
	BlameSpec = "spec"
	// BlameImpl — the implementation disagrees with a spec that both
	// evaluators read the same way. The ordinary case.
	BlameImpl = "impl"
	// BlameWrapper — a :runtime-via checker (or the generated wrapper
	// around it) failed. The spec and the impl may both be right; the
	// composition is not.
	BlameWrapper = "wrapper"
	// BlameLowering — the spec's own Go evaluator and the Shen host
	// disagree about what the spec means. Neither the spec author nor
	// the implementer is at fault: the translation is.
	BlameLowering = "lowering"
)

// BlameBasis values explain how a blame assignment was reached, which
// matters because the rules differ in strength.
const (
	// BlameBasisEvaluatorAndHost — the spec's Go evaluator and a live
	// Shen host were both consulted and agreed, so a disagreement with
	// the impl is the impl's.
	BlameBasisEvaluatorAndHost = "evaluator-and-host"
	// BlameBasisEvaluatorOnly — no Shen host was available, so only
	// the Go evaluator spoke. `impl` is the assignment, but a lowering
	// bug in the evaluator would produce the same evidence, and this
	// basis says so out loud.
	BlameBasisEvaluatorOnly = "evaluator-only"
	// BlameBasisEvaluatorHostDisagree — the two oracles disagree; the
	// lowering is at fault whatever the impl did.
	BlameBasisEvaluatorHostDisagree = "evaluator-host-disagree"
	// BlameBasisRuntimeVia — the failure came from a :runtime-via
	// checker, which is wrapper code by construction.
	BlameBasisRuntimeVia = "runtime-via"
	// BlameBasisVacuous — an uninhabited datatype, which is a spec
	// defect by construction.
	BlameBasisVacuous = "vacuous-datatype"
)

// -----------------------------------------------------------------

// RuntimeProfileLabel maps the internal profile letter to the
// human-facing label used in the audit report. Empty profile ("")
// returns "".
func RuntimeProfileLabel(profile string) string {
	switch profile {
	case "A":
		return "bespoke checker"
	case "B":
		return "evaluator-hosted"
	case "C":
		return "bespoke checker + sampled equivalence"
	case "D":
		return "DB-attested"
	default:
		return ""
	}
}

// CounterExample is a concrete witness of a rule violation.
type CounterExample struct {
	CaseID           string            `json:"case_id"`
	Input            map[string]string `json:"input"`
	SpecOutput       string            `json:"spec_output"`
	ImplOutput       string            `json:"impl_output"`
	ImplFunction     string            `json:"impl_function"`
	ImplFile         string            `json:"impl_file,omitempty"`
	ImplLineHint     *int              `json:"impl_line_hint"`
	FirstSeenCommit  *string           `json:"first_seen_commit"`
	Rationale        string            `json:"rationale"`

	// ---- W5 certificate block (additive, v1.x) -------------------
	// Blame names the responsible party (see the Blame* constants),
	// and BlameBasis records which rule assigned it. Both omitempty:
	// a pre-W5 report carries neither.
	Blame      string `json:"blame,omitempty"`
	BlameBasis string `json:"blame_basis,omitempty"`
	// --------------------------------------------------------------
}

// Summary rolls up rule and premise counts for the whole report.
// Fields are always present; counts may be zero.
type Summary struct {
	RuleCount              int `json:"rule_count"`
	RulesDischarged        int `json:"rules_discharged"`
	RulesViolated          int `json:"rules_violated"`
	RulesUnproven          int `json:"rules_unproven"`

	// RulesVacuous counts uninhabited rules (additive, v1.x).
	// omitempty keeps reports without vacuity byte-identical.
	RulesVacuous int `json:"rules_vacuous,omitempty"`
	PremisesTotal          int `json:"premises_total"`
	PremisesStatic         int `json:"premises_static"`
	PremisesRuntimeSampled int `json:"premises_runtime_sampled"`
	PremisesUnproven       int `json:"premises_unproven"`

	// Runtime-via premise counters (additive, v1). omitempty so reports
	// with no runtime-via premises stay byte-identical to pre-runtime-via
	// output.
	PremisesRuntimeAttested        int `json:"premises_runtime_attested,omitempty"`
	PremisesRuntimeEvaluator       int `json:"premises_runtime_evaluator,omitempty"`
	PremisesRuntimeAttestedSampled int `json:"premises_runtime_attested_sampled,omitempty"`
	PremisesRuntimeAttestedDB      int `json:"premises_runtime_attested_db,omitempty"`
}

// Signature is a detached signature over the report's canonical
// bytes. Written by `sb sign-report`; nil on an unsigned report.
//
// Canonical bytes are defined in cmd/sb/canonical.go and documented in
// docs/TRUST-MODEL.md: the report with its own `signature` field
// removed, marshalled as JSON with object keys sorted and no
// insignificant whitespace. Removing the field is what makes the
// signature a fixed point — the signer and the verifier hash the same
// bytes even though one of them is holding a signed document.
type Signature struct {
	// Algorithm is "ed25519" for the key-file mode, or "cosign" for
	// the keyless mode.
	Algorithm string `json:"algorithm"`
	// KeyID identifies the signer: the base64 public key for ed25519,
	// or the certificate identity for cosign.
	KeyID string `json:"key_id"`
	// Value is the base64 signature (ed25519) or bundle (cosign).
	Value string `json:"value"`

	// ---- W5 certificate block (additive, v1.x) -------------------
	// Canonicalization names the byte-derivation the signature covers,
	// so a future change to it cannot be mistaken for a bad signature.
	Canonicalization string `json:"canonicalization,omitempty"`
	// SignedAt is an RFC3339 timestamp. Advisory: it is inside the
	// signed bytes only in the sense that it is part of the signature
	// object, which is excluded from them, so treat it as a hint.
	SignedAt string `json:"signed_at,omitempty"`
	// Mode is "key-file" or "cosign-keyless".
	Mode string `json:"mode,omitempty"`
	// --------------------------------------------------------------
}

// CanonicalizationV1 is the only canonicalization this release
// produces: sorted keys, no whitespace, `signature` elided.
const CanonicalizationV1 = "sb-canonical-json-v1"
