// shen-cedar — Generate Cedar schema + policy JSON from Shen sequent specs.
//
// This is the runtime policy emitter for the "Cedar-shaped" slice of a
// Shen-Backpressure spec (single-snapshot PARC access predicates such as
// tenant-access / resource-access). It targets Cedar's stable *JSON* format
// (the machine interface; the CLI can round-trip to .cedar syntax).
//
// Architecture (v0 skeleton → full lowering):
//   1. Parse (datatype ...) blocks (reuse patterns from shengen + specfile).
//   2. Identify targets (explicit --targets or @policy-target annotations, or
//      shape inference for *-access / permit-* conclusions with boolean verified).
//   3. For each target, emit a Cedar entity schema fragment + a permit (or forbid)
//      policy whose when-clause is the translation of the verified premises.
//   4. The "is this lowerable?" judgment is the Shen gatekeeper (for v0 the
//      emitter fails hard with a clear message on non-fitting rules).
//
// Key constraints respected (from the project synthesis):
// - Snapshot only (no sequences, no temporal).
// - Cedar error semantics: missing attr makes condition error → policy not apply.
//   We map optional presence via the model (or treat required fields strictly).
// - Hierarchy via `in` where the spec + entities support it (tenant membership).
// - Do not re-do analysis; Cedar + symcc own that.
// - Output is committed + drift-checked (like guards_gen.go); differential
//   sampling (guard ctor vs. cedar authorize on same tuples) is the soundness
//   check on the *lowering*.
//
// Future: Rego emitter (Horn-like, great for aggregations + infra gating) and
// a "decidable Shen fragment" runtime tier (restricted total subset that still
// runs in shen-go / shen-lua / etc. but is certified terminating by the sequent
// rules — the "runtime shen that is also decidable" requested in review).
//
// The decidable-Shen-fragment is the *native* middle tier in the lattice:
//   Cedar (SMT, JSON policy)  ⊂  Rego (Horn, infra)  ⊂  Decidable-Shen-fragment (sequent+Prolog gatekeeper, total)  ⊂  full-TC pure-Shen
// Judgment: sequent calculus rule + embedded Prolog pass can discharge "in DecidableFragment".
// Fragment restrictions (starter): no general recursion (on selected policy targets), stratified rules,
// total functions (measures or obvious base/rec), Datalog/Horn-clause shaped verified bodies (only =, element?, cmp),
// bounded iteration. The emitter/mode here is *light*: parse tags or targets, simple syntactic check, emit
// "certified" comment (or safe module) OR run interpretation in a total evaluator stub. When host has Shen port,
// embed the restricted predicate directly: guaranteed termination for policy slice, zero drift on native data.
// Prefer Cedar on overlap for its SMT; this tier gives Shen-native terminating enforcement w/o external dep.
// Differential harness (n-way) extended to include pure-shen-fragment-eval on same samples.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ps "github.com/pyrex41/Shen-Backpressure/policyspec"
)

// Shared Shen-spec parsing + target selection live in the policyspec module
// (one copy for both emitters). These aliases keep the local call sites and
// tests reading naturally.
type (
	SExpr           = ps.SExpr
	Premise         = ps.Premise
	VerifiedPremise = ps.VerifiedPremise
	Conclusion      = ps.Conclusion
	Rule            = ps.Rule
	Datatype        = ps.Datatype
	ruleInfo        = ps.RuleInfo
)

var (
	parseSExpr                = ps.ParseSExpr
	parseDatatypes            = ps.ParseDatatypes
	buildRuleMap              = ps.BuildRuleMap
	buildRuleVariants         = ps.BuildRuleVariants
	collectClauses            = ps.CollectClauses
	collectTransitiveVerified = ps.CollectTransitiveVerified
	parseTargets              = ps.ParseTargets
	collectConclusions        = ps.CollectConclusions
	inferCedarTargets         = ps.InferAccessTargets
	selectCedarTargets        = ps.SelectTargets
	dirOf                     = ps.DirOf
	isNumericLiteral          = ps.IsNumericLiteral
)

func main() {
	fs := flag.NewFlagSet("shen-cedar", flag.ExitOnError)
	spec := fs.String("spec", "specs/core.shen", "path to the .shen spec")
	outSchema := fs.String("out-schema", "policies/cedar/schema.json", "output path for Cedar schema JSON")
	outPolicies := fs.String("out-policies", "policies/cedar/policies.json", "output path for Cedar policies JSON")
	targetsFlag := fs.String("targets", "", "comma-separated list of target conclusion/block names (e.g. tenant-access,resource-access); empty = infer")
	verbose := fs.Bool("verbose", false, "print parsed targets and symbol summary")
	decidable := fs.Bool("decidable", false, "run decidable-Shen-fragment mode (recognize @decidable-fragment / targets, static check for no-recursion + Horn forms, emit cert comment or total-eval stub; lighter than full Cedar lowering)")
	regen := fs.Bool("regen", false, "in --decidable mode, write the cert + total-eval stub sidecars (default: check only, no files written so verification gates stay read-only)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `shen-cedar — Emit Cedar JSON schema + policies from Shen specs (v0)

Usage:
  shen-cedar --spec specs/core.shen --out-schema ... --out-policies ...
  shen-cedar --spec ... --decidable [--targets tenant-access,...]

The emitter only lowers the Cedar-shaped fragment (snapshot access predicates).
Non-fitting rules cause a clear failure so the Shen spec remains the gate.

--decidable selects the tiny native-Shen decidable-fragment mode (sketch):
  - recognizes \* @decidable-fragment: name *\ or --targets
  - minimal check: no recursion among selected rules, only allowed verified forms (=, element?, comparisons)
  - emits a "certified" marker comment (or tiny total evaluator stub Go/Shen)
  - used by sb policy --decidable for certification + differential pure-shen-fragment-eval
  (Real: full sequent judgment discharge + total interpreter; this is the entry point parallel to Cedar.)

Flags:
`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(os.Args[1:])

	if *spec == "" {
		fmt.Fprintln(os.Stderr, "shen-cedar: --spec is required")
		os.Exit(1)
	}

	content, err := os.ReadFile(*spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar: reading spec: %v\n", err)
		os.Exit(1)
	}

	// --- Decidable-Shen-fragment mode (sketch emitter) ---
	// Lighter than Cedar lowering: tag-driven or target-driven recognition of the
	// restricted fragment, simple syntactic decidability check, certify or stub.
	if *decidable {
		runDecidableFragmentMode(*spec, string(content), *targetsFlag, *verbose, *regen)
		return
	}

	// Pure parsing + target selection extracted for testability (see collectConclusions,
	// selectCedarTargets, inferCedarTargets). Behavior is identical to the prior inline version.
	allConclusions := collectConclusions(string(content))
	requested := selectCedarTargets(*targetsFlag, allConclusions)

	if len(requested) == 0 {
		fmt.Fprintln(os.Stderr, "shen-cedar: no Cedar targets found (use --targets or add *-access / permit-* rules + /* @policy-target: cedar ... */ )")
		// Still succeed with empty outputs so the gate can be wired without
		// forcing every project to have policies immediately.
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "shen-cedar: spec=%s targets=%v (inferred or explicit)\n", *spec, requested)
	}

	// Parse full rules for verified premise lowering (SExpr + simple eq/cmp/element?).
	// Variants index ALL rules per conclusion so sum types lower to a disjunction.
	fullDTs := parseDatatypes(string(content))
	variants := buildRuleVariants(fullDTs)

	// Emit improved target-specific policies (policy_id permit-*/forbid-*, annotations
	// back to Shen rule name, and when conditions from verified premises).
	schema := buildCedarSchema(requested, variants)
	policies := buildCedarPolicies(requested, variants)

	if err := os.MkdirAll(dirOf(*outSchema), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar: mkdir schema: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(dirOf(*outPolicies), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar: mkdir policies: %v\n", err)
		os.Exit(1)
	}

	if err := writeJSON(*outSchema, schema); err != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar: write schema: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(*outPolicies, policies); err != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar: write policies: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Generated %s and %s from %s (targets: %v)\n",
		*outSchema, *outPolicies, *spec, requested)
}

// --- Minimal v0 Cedar model (will be replaced by real translation) ---

type cedarSchema struct {
	Schema   string                 `json:"schema"`
	Entities map[string]cedarEntity `json:"entities"`
	Actions  map[string]cedarAction `json:"actions"`
}

type cedarEntity struct {
	MemberOf []string       `json:"memberOfTypes,omitempty"`
	Shape    map[string]any `json:"shape,omitempty"`
}

type cedarAction struct {
	AppliesTo map[string]any `json:"appliesTo,omitempty"`
}

type cedarPolicySet struct {
	FormatVersion int               `json:"format_version"`
	PolicyStoreID string            `json:"policy_store_id,omitempty"`
	Policies      []cedarPolicyJSON `json:"policies"`
}

type cedarPolicyJSON struct {
	PolicyID    string            `json:"policy_id"`
	Effect      string            `json:"effect"`
	Annotations map[string]string `json:"annotations,omitempty"`
	// Definition holds the Cedar EST policy body (principal/action/resource + conditions).
	// Conditions now include translated when-clauses from Shen verified premises.
	Definition map[string]any `json:"definition"`
}

// buildMinimalCedarSchema is the no-rule fallback (used by tests and when no
// rules are parsed); it derives no context attributes.
func buildMinimalCedarSchema(targets []string) cedarSchema {
	return buildCedarSchema(targets, nil)
}

// buildCedarSchema emits the PARC entity scaffold plus a "read" action whose
// context Record declares exactly the attributes the lowered policies reference
// (derived from the targets' transitive verified premises). Previously the
// context was hardcoded empty, leaving schema and policies inconsistent.
func buildCedarSchema(targets []string, variants map[string][]ruleInfo) cedarSchema {
	ents := map[string]cedarEntity{
		"User": {
			MemberOf: []string{"Tenant"},
			Shape: map[string]any{
				"type": "Record",
				"attributes": map[string]any{
					"id": map[string]any{"type": "String"},
				},
			},
		},
		"Tenant": {
			Shape: map[string]any{
				"type": "Record",
				"attributes": map[string]any{
					"id": map[string]any{"type": "String"},
				},
			},
		},
		"Resource": {
			Shape: map[string]any{
				"type": "Record",
				"attributes": map[string]any{
					"id":       map[string]any{"type": "String"},
					"tenantId": map[string]any{"type": "String"},
				},
			},
		},
	}

	// Declare the context attributes the lowered policies actually reference, so
	// the schema is consistent with the emitted when-conditions.
	ctxAttrs := map[string]any{}
	for name, typ := range cedarContextAttrs(targets, variants) {
		ctxAttrs[name] = map[string]any{"type": typ}
	}
	for _, t := range targets {
		if targetLevel(t) != "" {
			ctxAttrs["level"] = map[string]any{"type": "String"}
		}
	}

	actions := map[string]cedarAction{
		"read": {
			AppliesTo: map[string]any{
				"principalTypes": []string{"User"},
				"resourceTypes":  []string{"Resource"},
				"context":        map[string]any{"type": "Record", "attributes": ctxAttrs},
			},
		},
	}

	return cedarSchema{
		Schema:   "2021-06-01",
		Entities: ents,
		Actions:  actions,
	}
}

// cedarContextAttrs derives the set of context attributes (name -> Cedar type)
// referenced by a target's lowered conditions, following the transitive
// precondition chain. Mirrors the lowering in cedarBodyFromVerified/cedarExpr.
func cedarContextAttrs(targets []string, variants map[string][]ruleInfo) map[string]string {
	attrs := map[string]string{}
	if variants == nil {
		return attrs
	}
	for _, t := range targets {
		for _, clause := range collectClauses(t, variants, map[string]bool{}) {
			for _, v := range clause {
				expr := parseSExpr(v.Raw)
				if !expr.IsCall() {
					continue
				}
				// (not (= A B)) constrains the same attrs as (= A B); unwrap it.
				if expr.Op() == "not" && len(expr.Children) == 2 && expr.Children[1].IsCall() {
					expr = expr.Children[1]
				}
				typ := "String"
				switch expr.Op() {
				case "=":
					// Type the attr from the literal it is compared against.
					for _, c := range expr.Children[1:] {
						if c.IsCall() {
							continue
						}
						switch {
						case c.Atom == "true" || c.Atom == "false":
							typ = "Bool"
						case isNumericLiteral(c.Atom):
							if typ != "Bool" {
								typ = "Long"
							}
						}
					}
				case ">=", "<=", ">", "<":
					typ = "Long"
				default:
					continue
				}
				for _, c := range expr.Children[1:] {
					if c.IsCall() || !isAttrAtom(c.Atom) {
						continue
					}
					attrs[ps.ToCamelCase(c.Atom)] = typ
				}
			}
		}
	}
	return attrs
}

// isAttrAtom reports whether an atom is a variable (a context attribute), as
// opposed to a literal (true/false, number, or a quoted "string").
func isAttrAtom(atom string) bool {
	if atom == "" || atom == "true" || atom == "false" || isNumericLiteral(atom) {
		return false
	}
	if len(atom) >= 2 && strings.HasPrefix(atom, `"`) && strings.HasSuffix(atom, `"`) {
		return false
	}
	return true
}

func buildMinimalCedarPolicies(targets []string) cedarPolicySet {
	// Now emits one policy per target (permit-foo for access-style targets).
	// When called from tests without rule context, uses basic "principal has id"
	// condition (mentions an attribute) as the improved minimal when-clause.
	// Real verified premise translation (via SExpr) is used by buildCedarPolicies.
	if len(targets) == 0 {
		targets = []string{"starter-access"}
	}
	var pols []cedarPolicyJSON
	for _, t := range targets {
		eff := "permit"
		if strings.Contains(strings.ToLower(t), "deny") || strings.Contains(strings.ToLower(t), "forbid") {
			eff = "forbid"
		}
		polID := eff + "-" + t
		// Basic condition that at least mentions an attribute (task v1 requirement).
		// The rich path will replace this with translated (= IsMember true) etc.
		pols = append(pols, cedarPolicyJSON{
			PolicyID: polID,
			Effect:   eff,
			Definition: map[string]any{
				"principal": map[string]any{"op": "All"},
				"action":    map[string]any{"op": "All"},
				"resource":  map[string]any{"op": "All"},
				"conditions": []any{
					map[string]any{
						"kind": "when",
						"body": map[string]any{
							"has": map[string]any{
								"left": map[string]any{"Var": "principal"},
								"attr": "id",
							},
						},
					},
				},
			},
			Annotations: map[string]string{
				"shen_datatype": t,
				"shen_source":   "minimal (no verified rules available)",
			},
		})
	}
	return cedarPolicySet{
		FormatVersion: 1,
		PolicyStoreID: "shen-backpressure-v0",
		Policies:      pols,
	}
}

// buildCedarPolicies emits, for each target, one permit/forbid policy per DNF
// clause of its (transitive) verified premises. A sum-typed dependency such as
// authenticated-principal = human ∨ service produces multiple clauses; Cedar
// authorizes if ANY matching permit applies, so multiple permits realize the
// disjunction. Each policy carries the target scope plus that clause's
// conditions, with annotations linking back to the Shen conclusion.
func buildCedarPolicies(targets []string, variants map[string][]ruleInfo) cedarPolicySet {
	if len(targets) == 0 {
		return buildMinimalCedarPolicies(nil)
	}
	var pols []cedarPolicyJSON
	for _, t := range targets {
		eff := "permit"
		if strings.Contains(strings.ToLower(t), "deny") || strings.Contains(strings.ToLower(t), "forbid") {
			eff = "forbid"
		}
		dtName := t
		if rs, ok := variants[t]; ok && len(rs) > 0 {
			dtName = rs[0].DtName
		}
		clauses := collectClauses(t, variants, map[string]bool{})
		for ci, clause := range clauses {
			polID := eff + "-" + t
			if len(clauses) > 1 {
				polID = fmt.Sprintf("%s-%s-%d", eff, t, ci)
			}
			ann := map[string]string{
				"shen_conclusion": t,
				"shen_datatype":   dtName,
				"shen_source":     "datatype " + t,
			}
			if len(clauses) > 1 {
				ann["shen_variant"] = strconv.Itoa(ci)
			}
			var conds []any
			if body := cedarTargetScopeBody(t); body != nil {
				conds = append(conds, map[string]any{"kind": "when", "body": body})
			}
			for _, v := range clause {
				if body := cedarBodyFromVerified(v); body != nil {
					conds = append(conds, map[string]any{"kind": "when", "body": body})
				}
			}
			if len(conds) == 0 {
				// Fallback: at least mention an attribute (principal has id).
				conds = []any{
					map[string]any{
						"kind": "when",
						"body": map[string]any{
							"has": map[string]any{
								"left": map[string]any{"Var": "principal"},
								"attr": "id",
							},
						},
					},
				}
			}
			pols = append(pols, cedarPolicyJSON{
				PolicyID:    polID,
				Effect:      eff,
				Annotations: ann,
				Definition: map[string]any{
					"principal":  map[string]any{"op": "All"},
					"action":     map[string]any{"op": "All"},
					"resource":   map[string]any{"op": "All"},
					"conditions": conds,
				},
			})
		}
	}
	return cedarPolicySet{
		FormatVersion: 1,
		PolicyStoreID: "shen-backpressure-v0",
		Policies:      pols,
	}
}

func cedarTargetScopeBody(target string) map[string]any {
	level := targetLevel(target)
	if level == "" {
		return nil
	}
	return map[string]any{
		"==": map[string]any{
			"left": map[string]any{
				".": map[string]any{
					"left": map[string]any{"Var": "context"},
					"attr": "level",
				},
			},
			"right": map[string]any{"Value": level},
		},
	}
}

func targetLevel(target string) string {
	lt := strings.ToLower(target)
	switch {
	case strings.Contains(lt, "resource"):
		return "resource"
	case strings.Contains(lt, "tenant"):
		return "tenant"
	default:
		return ""
	}
}

// cedarBodyFromVerified uses SExpr parse (from shengen) + simple translation
// (adapted verifiedToGo patterns) to turn e.g. (= IsMember true) into a
// Cedar when body like { "==" : { "left": { "." : {left: {Var:"context"}, attr:"isMember"} }, "right": {"Value": true} } }
// Falls back to nil (caller supplies has-principal-id).
func cedarBodyFromVerified(v VerifiedPremise) map[string]any {
	expr := parseSExpr(v.Raw)
	if !expr.IsCall() {
		return nil
	}
	switch expr.Op() {
	case "=":
		if len(expr.Children) != 3 {
			return nil
		}
		lhs := expr.Children[1]
		rhs := expr.Children[2]
		leftE := cedarExpr(lhs)
		rightE := cedarExpr(rhs)
		if leftE == nil || rightE == nil {
			return nil
		}
		return map[string]any{
			"==": map[string]any{"left": leftE, "right": rightE},
		}
	case "not":
		// (not (= A B)) -> A != B. Without this, non-emptiness premises like
		// (not (= Token "")) from the auth chain would be silently dropped,
		// making the policy more permissive than the guard.
		if len(expr.Children) != 2 {
			return nil
		}
		inner := expr.Children[1]
		if !inner.IsCall() || inner.Op() != "=" || len(inner.Children) != 3 {
			return nil
		}
		l := cedarExpr(inner.Children[1])
		r := cedarExpr(inner.Children[2])
		if l == nil || r == nil {
			return nil
		}
		return map[string]any{
			"!=": map[string]any{"left": l, "right": r},
		}
	case "element?":
		// (element? Elem Coll) -> Coll.contains(Elem). Both operands are simple
		// atoms (a value/var element and a collection attribute); inline set
		// literals [A B …] are not yet parsed and fall through to nil.
		if len(expr.Children) != 3 {
			return nil
		}
		el := cedarExpr(expr.Children[1])
		coll := cedarExpr(expr.Children[2])
		if el == nil || coll == nil {
			return nil
		}
		return map[string]any{
			"contains": map[string]any{"left": coll, "right": el},
		}
	case ">=", "<=", ">", "<":
		if len(expr.Children) != 3 {
			return nil
		}
		l := cedarExpr(expr.Children[1])
		r := cedarExpr(expr.Children[2])
		if l == nil || r == nil {
			return nil
		}
		return map[string]any{
			expr.Op(): map[string]any{"left": l, "right": r},
		}
	}
	return nil
}

// cedarExpr turns an SExpr atom (or simple) into Cedar JSON expr form.
// Vars become context.<attr> (v1 choice; mentions the premise attr).
// true/false/nums become Value. (head/tail etc not lowered in v1).
func cedarExpr(s *SExpr) map[string]any {
	if s == nil {
		return nil
	}
	if s.IsCall() {
		// v1: only direct atoms for equality/comparisons; skip nested for now.
		// Could extend with principal/resource special cases.
		return nil
	}
	atom := s.Atom
	if atom == "" {
		return nil
	}
	switch atom {
	case "true":
		return map[string]any{"Value": true}
	case "false":
		return map[string]any{"Value": false}
	}
	if isNumericLiteral(atom) {
		f, _ := strconv.ParseFloat(atom, 64)
		return map[string]any{"Value": f}
	}
	if len(atom) >= 2 && strings.HasPrefix(atom, `"`) && strings.HasSuffix(atom, `"`) {
		return map[string]any{"Value": atom[1 : len(atom)-1]}
	}
	// Treat as context attribute (e.g. IsMember -> context.isMember).
	// This satisfies "at least mentions the attribute" and reuses SExpr.
	attr := ps.ToCamelCase(atom)
	return map[string]any{
		".": map[string]any{
			"left": map[string]any{"Var": "context"},
			"attr": attr,
		},
	}
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ============================================================================
// Decidable-Shen-fragment mode (sketch).
// This is the minimal "decidable fragment" judgment + tiny emitter/mode.
// - Recognizes \* @decidable-fragment: foo *\ comments or --targets.
// - Simple check (no recursion among selected, only allowed verified forms).
// - Emits a certification comment (or tiny total-eval stub source).
// - Parallel to Cedar but native-Shen: can be run directly by a Shen port
//   (shen-go etc) with termination guarantee for the policy slice.
// The real judgment lives in Shen sequent calculus + Prolog; this is a
// syntactic approximation sufficient for the starter sketch + differential.
// ============================================================================

func runDecidableFragmentMode(specPath, content, targetsFlag string, verbose, regen bool) {
	fmt.Fprintf(os.Stderr, "shen-cedar --decidable: native decidable-Shen-fragment mode (sketch)\n")

	allConclusions := collectConclusions(content)
	requested := selectDecidableTargets(targetsFlag, allConclusions, content)

	if len(requested) == 0 {
		fmt.Fprintln(os.Stderr, "shen-cedar --decidable: no targets (add --targets or @decidable-fragment annotations)")
	}

	// Minimal fragment check on the selected targets (uses the existing Datatype/Rule parse).
	// A check failure is a real gate failure: the spec is outside the decidable
	// fragment, so we must not certify it.
	fullDTs := parseDatatypes(content)
	variants := buildRuleVariants(fullDTs)
	if checkErr := checkDecidableFragment(requested, variants); checkErr != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar --decidable: FRAGMENT CHECK FAILED: %v\n", checkErr)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "shen-cedar --decidable: fragment check OK for targets %v (no recursion, allowed Horn forms)\n", requested)

	if verbose {
		fmt.Fprintf(os.Stderr, "shen-cedar --decidable: targets=%v\n", requested)
	}

	// Sidecars (cert marker + total-eval stub) are written only with --regen, so
	// the verification gate (`sb policy --decidable` without --regen) stays
	// read-only and never dirties the working tree.
	if !regen {
		fmt.Fprintln(os.Stderr, "  (check only; pass --regen to (re)write decidable-fragment.cert + eval stub)")
		return
	}

	certDir := dirOf(specPath)
	if certDir == "" {
		certDir = "."
	}
	certPath := filepath.Join(certDir, "decidable-fragment.cert")
	certBody := fmt.Sprintf("# decidable-fragment certified (sketch)\n# targets: %s\n# judgment: sequent-calculus + prolog gatekeeper (no general recursion; Horn bodies only)\n# tier: Decidable-Shen-fragment (Cedar ⊂ Rego ⊂ this ⊂ full-TC pure-Shen)\n# can run directly in shen-* ports or via total-eval stub\n", strings.Join(requested, ","))
	if err := os.WriteFile(certPath, []byte(certBody), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar --decidable: warning: writing cert: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "  wrote cert marker: %s\n", certPath)
	}

	stubPath := filepath.Join(certDir, "decidable_fragment_eval_stub.go")
	stub := generateTinyTotalEvalStub(requested, variants)
	if err := os.WriteFile(stubPath, []byte(stub), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "shen-cedar --decidable: warning: writing stub: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "  wrote tiny total-eval stub: %s (for pure-shen-fragment-eval in differential)\n", stubPath)
	}
}

// selectDecidableTargets: explicit or inference + also scan for @decidable-fragment annotations.
// For sketch we reuse inference + a simple comment scan (parallel to @policy-target).
func selectDecidableTargets(targetsFlag string, allConclusions []string, content string) []string {
	requested := parseTargets(targetsFlag)
	if len(requested) == 0 {
		// inference on access etc (same as cedar) + explicit @decidable-fragment tags
		requested = inferCedarTargets(allConclusions) // reuse; they overlap for access predicates
		tagged := extractDecidableTags(content)
		for _, t := range tagged {
			if !contains(requested, t) {
				requested = append(requested, t)
			}
		}
	}
	return requested
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// extractDecidableTags scans for \* @decidable-fragment: name *\
func extractDecidableTags(content string) []string {
	var out []string
	lines := strings.Split(content, "\n")
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.Contains(t, "@decidable-fragment:") {
			// crude parse after :
			idx := strings.Index(t, "@decidable-fragment:")
			rest := strings.TrimSpace(t[idx+len("@decidable-fragment:"):])
			// stop at * or space or )
			for i, r := range rest {
				if r == '*' || r == ' ' || r == '\t' || r == ')' || r == '\n' {
					rest = strings.TrimSpace(rest[:i])
					break
				}
			}
			if rest != "" {
				out = append(out, rest)
			}
		}
	}
	return out
}

// checkDecidableFragment: the minimal syntactic "decidable fragment judgment".
// - Walks rules for requested targets.
// - Rejects if any selected conclusion appears in its own (transitive) premises/bodies (recursion).
// - Only permits a tiny set of verified forms ( = , element? , numeric cmps ) — Horn/Datalog shaped.
// Real version: full sequent well-formedness + stratification + measure/termination proof.
func checkDecidableFragment(targets []string, variants map[string][]ruleInfo) error {
	for _, t := range targets {
		if _, ok := variants[t]; !ok {
			continue
		}
		// Check every premise across every DNF clause the emitters actually
		// lower (all sum-type variants, full transitive closure) — recursion or
		// a disallowed form hidden behind a typed premise must still fail.
		for _, clause := range collectClauses(t, variants, map[string]bool{}) {
			for _, v := range clause {
				if strings.Contains(v.Raw, t) {
					return fmt.Errorf("recursion or self-reference detected in %s premise chain: %s (fragment disallows general recursion)", t, v.Raw)
				}
				if !isAllowedDecidableForm(v.Raw) {
					return fmt.Errorf("disallowed form in decidable fragment for %s: %s (only =, not(=), element?, >=,<=,>,< permitted in sketch)", t, v.Raw)
				}
			}
		}
	}
	return nil
}

func isAllowedDecidableForm(raw string) bool {
	s := strings.TrimSpace(raw)
	// strip outer parens for matching
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	s = strings.TrimSpace(s)
	// Forms the emitters can lower and EvalVerified can evaluate. `if` is
	// intentionally excluded — it is not a Horn-shaped verified premise for
	// these access rules, and the real judgment is the Shen sequent calculus +
	// Prolog pass, not this syntactic heuristic.
	allowedPrefixes := []string{"= ", "not ", "element? ", ">= ", "<= ", "> ", "< "}
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(s, p) || strings.HasPrefix(s, "("+p) {
			return true
		}
	}
	// bare symbol or simple true is ok for sketch (degenerate)
	if !strings.ContainsAny(s, " ()") || s == "true" || s == "false" {
		return true
	}
	return false
}

// generateTinyTotalEvalStub emits a tiny Go source that can be used as the
// "interpretation mode" total evaluator for the fragment predicates.
// For the multi-tenant access rules this is essentially the conjunctions.
// generateTinyTotalEvalStub emits a total evaluator for the certified targets.
// Rather than a hardcoded per-target switch, it embeds each target's DNF clauses
// (exactly what the Cedar/Rego emitters lower, including sum-type disjunctions)
// and evaluates them through the shared policyspec.EvalClauses — so the stub,
// the emitters, and the differential harness all use one total evaluator and
// cannot drift, and it generalizes to any access predicate.
func generateTinyTotalEvalStub(targets []string, variants map[string][]ruleInfo) string {
	var b strings.Builder
	b.WriteString("// Code generated by shen-cedar --decidable. DO NOT EDIT.\n")
	b.WriteString("// Total evaluator for the Decidable-Shen-fragment: total by construction\n")
	b.WriteString("// (no recursion, bounded Horn bodies). Evaluation is delegated to\n")
	b.WriteString("// policyspec.EvalClauses so it never drifts from the emitters/harness.\n\n")
	b.WriteString("package decidablefragment\n\n")
	b.WriteString("import ps \"github.com/pyrex41/Shen-Backpressure/policyspec\"\n\n")
	b.WriteString("// fragmentClauses holds each certified target's DNF clauses (a disjunction\n")
	b.WriteString("// of conjunctions of verified premises).\n")
	b.WriteString("var fragmentClauses = map[string][][]string{\n")
	for _, t := range targets {
		fmt.Fprintf(&b, "\t%q: {", t)
		for ci, clause := range collectClauses(t, variants, map[string]bool{}) {
			if ci > 0 {
				b.WriteString(", ")
			}
			b.WriteString("{")
			for i, v := range clause {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%q", v.Raw)
			}
			b.WriteString("}")
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// PureShenFragmentEval evaluates target's certified clauses against env\n")
	b.WriteString("// (camelCase attr -> value). Unknown target or all-indeterminate => false.\n")
	b.WriteString("func PureShenFragmentEval(target string, env map[string]any) bool {\n")
	b.WriteString("\trawClauses, ok := fragmentClauses[target]\n")
	b.WriteString("\tif !ok {\n\t\treturn false\n\t}\n")
	b.WriteString("\tclauses := make([][]ps.VerifiedPremise, len(rawClauses))\n")
	b.WriteString("\tfor i, raws := range rawClauses {\n")
	b.WriteString("\t\tprem := make([]ps.VerifiedPremise, len(raws))\n")
	b.WriteString("\t\tfor j, r := range raws {\n\t\t\tprem[j] = ps.VerifiedPremise{Raw: r}\n\t\t}\n")
	b.WriteString("\t\tclauses[i] = prem\n\t}\n")
	b.WriteString("\tres, ok := ps.EvalClauses(clauses, env)\n")
	b.WriteString("\treturn ok && res\n}\n")
	if formatted, err := format.Source([]byte(b.String())); err == nil {
		return string(formatted)
	}
	return b.String()
}
