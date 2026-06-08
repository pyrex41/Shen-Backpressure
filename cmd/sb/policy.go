package main

// sb policy — the Cedar + Decidable-Shen-fragment (native terminating) runtime policy emitter gate (sketch for middle lattice tier).
//
// For a [cedar] entry in sb.toml, `sb policy` invokes the shen-cedar
// emitter (discovered via FindShenCedar, modeled on FindShengen), passing
// --spec, --out-schema, --out-policies, --targets (from config).
//
// For a [rego] entry, invokes shen-rego (parallel emitter) with --spec,
// --out-rego (primary text), --targets. Text .rego is the canonical form
// (opa eval / conftest friendly).
//
// Supports --regen (write directly) vs. drift check (tempfile + bytes.Equal).
// When the real `cedar` / `opa` binary is present, also runs official
// validate (cedar validate / opa eval smoke) on the emitted artifacts.
//
// The gate(s) auto-registered in `sb gates` + context when the respective
// config sections are present. This is the n-way differential enabler.

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	ps "github.com/pyrex41/Shen-Backpressure/policyspec"
)

func cmdPolicy(args []string) {
	fs := flag.NewFlagSet("policy", flag.ExitOnError)
	regen := fs.Bool("regen", false, "write regenerated policy artifacts (Cedar JSON + Rego .rego) in place instead of diffing")
	verbose := fs.Bool("verbose", false, "print the emitter command(s) before running")
	decidable := fs.Bool("decidable", false, "run the decidable-Shen-fragment mode (certify + pure-shen-fragment-eval stub) instead of / in addition to Cedar (sketch tier)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "sb policy — Cedar (shen-cedar) + Rego (shen-rego) runtime policy emitters + drift + real (cedar/opa) validate\n\n"+
			"Usage: sb policy [flags]\n\n"+
			"For [cedar] in sb.toml: runs shen-cedar → schema/policies JSON; `cedar validate` when present.\n"+
			"For [rego] in sb.toml:  runs shen-rego → primary .rego text module; `opa eval` smoke when present.\n"+
			"Mirrors derive tempfile/bytes.Equal + --regen drift pattern. n-way differential harness extends across tiers.\n\n"+
			"Flags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb policy: %v\n", err)
		os.Exit(1)
	}
	hasCedar := cfg.Cedar.SchemaOut != "" || cfg.Cedar.PoliciesOut != ""
	hasRego := cfg.Rego.ModuleOut != ""
	if !hasCedar && !hasRego && !*decidable && len(cfg.DecidableShen.Targets) == 0 {
		fmt.Fprintln(os.Stderr, "sb policy: no [cedar]/[rego] outputs and no --decidable / [decidable-shen] — nothing to do")
		return
	}

	spec := cfg.Spec
	if spec == "" {
		spec = "specs/core.shen"
	}
	if _, err := os.Stat(spec); err != nil {
		fmt.Fprintf(os.Stderr, "sb policy: spec file not found at %s\n", spec)
		os.Exit(1)
	}

	// Sketch path for decidable-Shen-fragment tier (native but terminating).
	// For now we invoke shen-cedar (extended with --decidable mode for recognizer/cert)
	// and also run an in-process pure-shen-fragment-eval stub for differential.
	if *decidable || len(cfg.DecidableShen.Targets) > 0 {
		runDecidableShenFragmentMode(spec, cfg, *regen, *verbose)
		// If no Cedar/Rego outputs are configured, the decidable run is all there
		// is to do. Reuse the same hasCedar/hasRego predicates computed above so
		// this can't diverge from gate registration or the final guard.
		if !hasCedar && !hasRego {
			return
		}
	}

	absSpec, _ := filepath.Abs(spec)

	// Cedar path (if configured)
	if hasCedar {
		cedarBin, err := FindShenCedar()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb policy: %v\n", err)
			os.Exit(1)
		}

		absSchema, _ := filepath.Abs(cfg.Cedar.SchemaOut)
		absPolicies, _ := filepath.Abs(cfg.Cedar.PoliciesOut)

		targets := ""
		if len(cfg.Cedar.Targets) > 0 {
			targets = strings.Join(cfg.Cedar.Targets, ",")
		}

		fmt.Fprintf(os.Stderr, "sb policy: shen-cedar %s → schema:%s policies:%s (targets:%v)\n",
			spec, cfg.Cedar.SchemaOut, cfg.Cedar.PoliciesOut, cfg.Cedar.Targets)

		baseArgs := []string{
			"--spec", absSpec,
		}
		if targets != "" {
			baseArgs = append(baseArgs, "--targets", targets)
		}

		if *regen {
			runArgs := append([]string(nil), baseArgs...)
			runArgs = append(runArgs, "--out-schema", absSchema, "--out-policies", absPolicies)
			if *verbose {
				printCommand(cedarBin, runArgs)
			}
			if err := runInDir("", cedarBin, runArgs...); err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: %v\n", err)
				os.Exit(1)
			}
			// Real cedar validate when binary available (same pattern as cedar-verify).
			runRealCedarValidateIfAvailable(absSchema, absPolicies, cfg.Cedar.Targets, *verbose)
		} else {
			// Drift path for Cedar (temp + compare, two files).
			tmpSchema, err := os.CreateTemp("", "shen-cedar-schema-*.json")
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: tempfile schema: %v\n", err)
				os.Exit(1)
			}
			tmpSchemaPath := tmpSchema.Name()
			tmpSchema.Close()
			defer os.Remove(tmpSchemaPath)

			tmpPolicies, err := os.CreateTemp("", "shen-cedar-policies-*.json")
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: tempfile policies: %v\n", err)
				os.Exit(1)
			}
			tmpPoliciesPath := tmpPolicies.Name()
			tmpPolicies.Close()
			defer os.Remove(tmpPoliciesPath)

			runArgs := append([]string(nil), baseArgs...)
			runArgs = append(runArgs, "--out-schema", tmpSchemaPath, "--out-policies", tmpPoliciesPath)
			if *verbose {
				printCommand(cedarBin, runArgs)
			}
			if err := runInDir("", cedarBin, runArgs...); err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: %v\n", err)
				os.Exit(1)
			}

			drifted := 0
			// Schema drift
			got, err := os.ReadFile(tmpSchemaPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: read regen schema: %v\n", err)
				os.Exit(1)
			}
			want, err := os.ReadFile(absSchema)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: read committed schema %s: %v\n  run `sb policy --regen` to create it.\n", absSchema, err)
				os.Exit(1)
			}
			if !bytes.Equal(got, want) {
				drifted++
				fmt.Fprintf(os.Stderr, "  DRIFT: %s differs from regenerated output.\n", cfg.Cedar.SchemaOut)
				diffOut, _ := exec.Command("diff", "-u", absSchema, tmpSchemaPath).CombinedOutput()
				fmt.Fprintln(os.Stderr, string(diffOut))
			}
			// Policies drift
			got, err = os.ReadFile(tmpPoliciesPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: read regen policies: %v\n", err)
				os.Exit(1)
			}
			want, err = os.ReadFile(absPolicies)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: read committed policies %s: %v\n  run `sb policy --regen` to create it.\n", absPolicies, err)
				os.Exit(1)
			}
			if !bytes.Equal(got, want) {
				drifted++
				fmt.Fprintf(os.Stderr, "  DRIFT: %s differs from regenerated output.\n", cfg.Cedar.PoliciesOut)
				diffOut, _ := exec.Command("diff", "-u", absPolicies, tmpPoliciesPath).CombinedOutput()
				fmt.Fprintln(os.Stderr, string(diffOut))
			}
			if drifted > 0 {
				fmt.Fprintf(os.Stderr, "sb policy: %d Cedar file(s) stale. Run `sb policy --regen` to update.\n", drifted)
				os.Exit(1)
			}
			runRealCedarValidateIfAvailable(absSchema, absPolicies, cfg.Cedar.Targets, *verbose)
		}
	}

	// Rego path (if [rego] configured) — parallel, text .rego primary.
	if hasRego {
		regoBin, err := FindShenRego()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb policy: %v\n", err)
			os.Exit(1)
		}

		absModule, _ := filepath.Abs(cfg.Rego.ModuleOut)

		targets := ""
		if len(cfg.Rego.Targets) > 0 {
			targets = strings.Join(cfg.Rego.Targets, ",")
		}

		fmt.Fprintf(os.Stderr, "sb policy: shen-rego %s → rego:%s (targets:%v)\n",
			spec, cfg.Rego.ModuleOut, cfg.Rego.Targets)

		baseArgs := []string{
			"--spec", absSpec,
		}
		if targets != "" {
			baseArgs = append(baseArgs, "--targets", targets)
		}
		if cfg.Rego.Package != "" {
			baseArgs = append(baseArgs, "--package", cfg.Rego.Package)
		}

		if *regen {
			runArgs := append([]string(nil), baseArgs...)
			runArgs = append(runArgs, "--out-rego", absModule)
			if *verbose {
				printCommand(regoBin, runArgs)
			}
			if err := runInDir("", regoBin, runArgs...); err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: %v\n", err)
				os.Exit(1)
			}
			runRealOpaValidateIfAvailable(absModule, cfg.Rego.Targets, cfg.Rego.Package, *verbose)
		} else {
			// Drift check for the single primary .rego text file.
			tmpRego, err := os.CreateTemp("", "shen-rego-*.rego")
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: tempfile rego: %v\n", err)
				os.Exit(1)
			}
			tmpRegoPath := tmpRego.Name()
			tmpRego.Close()
			defer os.Remove(tmpRegoPath)

			runArgs := append([]string(nil), baseArgs...)
			runArgs = append(runArgs, "--out-rego", tmpRegoPath)
			if *verbose {
				printCommand(regoBin, runArgs)
			}
			if err := runInDir("", regoBin, runArgs...); err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: %v\n", err)
				os.Exit(1)
			}

			got, err := os.ReadFile(tmpRegoPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: read regen rego: %v\n", err)
				os.Exit(1)
			}
			want, err := os.ReadFile(absModule)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sb policy: read committed rego %s: %v\n  run `sb policy --regen` to create it.\n", absModule, err)
				os.Exit(1)
			}
			if !bytes.Equal(got, want) {
				fmt.Fprintf(os.Stderr, "  DRIFT: %s differs from regenerated output.\n", cfg.Rego.ModuleOut)
				diffOut, _ := exec.Command("diff", "-u", absModule, tmpRegoPath).CombinedOutput()
				fmt.Fprintln(os.Stderr, string(diffOut))
				fmt.Fprintf(os.Stderr, "sb policy: Rego module stale. Run `sb policy --regen` to update.\n")
				os.Exit(1)
			}
			runRealOpaValidateIfAvailable(absModule, cfg.Rego.Targets, cfg.Rego.Package, *verbose)
		}
	}

	// If only decidable sketch was requested and no Cedar/Rego outs, we already returned above.
	if !hasCedar && !hasRego {
		return
	}
}

// writeCedarTextCompanions writes .cedarschema and .cedar text files next to the
// JSON outputs so `cedar validate` / `cedar authorize` can run on Cedar syntax.
// The companions are DERIVED from the emitted policies.json (not hand-frozen), so
// validation actually exercises the emitter's lowering — including the context
// attributes each policy references and the full precondition chain. The schema
// declares exactly those attributes (inferred Bool/Long/String) so a strict
// `cedar validate --deny-warnings` is meaningful rather than rubber-stamping.
func writeCedarTextCompanions(schemaOut, policiesOut string, targets []string) (textSchema, textPol string) {
	schemaDir := filepath.Dir(schemaOut)
	polDir := filepath.Dir(policiesOut)
	textSchema = filepath.Join(schemaDir, "schema.cedarschema")
	textPol = filepath.Join(polDir, "policies.cedar")

	ctxAttrs := map[string]string{} // attr name -> Cedar type
	polTxt := ""
	if raw, err := os.ReadFile(policiesOut); err == nil {
		var set struct {
			Policies []struct {
				PolicyID   string `json:"policy_id"`
				Effect     string `json:"effect"`
				Definition struct {
					Conditions []struct {
						Kind string         `json:"kind"`
						Body map[string]any `json:"body"`
					} `json:"conditions"`
				} `json:"definition"`
			} `json:"policies"`
		}
		if json.Unmarshal(raw, &set) == nil {
			for _, p := range set.Policies {
				var clauses []string
				for _, c := range p.Definition.Conditions {
					if c.Kind != "when" {
						continue
					}
					if s := cedarRenderBody(c.Body, ctxAttrs); s != "" {
						clauses = append(clauses, s)
					}
				}
				eff := p.Effect
				if eff == "" {
					eff = "permit"
				}
				body := "true"
				if len(clauses) > 0 {
					body = strings.Join(clauses, " &&\n    ")
				}
				polTxt += fmt.Sprintf("\n%s (\n    principal,\n    action,\n    resource\n) when {\n    %s\n};\n", eff, body)
			}
		}
	}

	// Declare exactly the context attributes the policies reference.
	var attrLines []string
	for name, typ := range ctxAttrs {
		attrLines = append(attrLines, fmt.Sprintf("    %s: %s", name, typ))
	}
	sort.Strings(attrLines)
	ctxBlock := "{}"
	if len(attrLines) > 0 {
		ctxBlock = "{\n" + strings.Join(attrLines, ",\n") + "\n  }"
	}
	schemaTxt := fmt.Sprintf(`entity User in [Tenant] = {
  "id"?: String
};
entity Tenant;
entity Resource;

action "read" appliesTo {
  principal: [User],
  resource: [Resource],
  context: %s
};
`, ctxBlock)

	_ = os.WriteFile(textSchema, []byte(schemaTxt), 0644)
	_ = os.WriteFile(textPol, []byte(polTxt), 0644)
	return
}

// cedarRenderBody renders a single emitted "when" body to Cedar text and records
// any context attributes it references (with an inferred type) into ctxAttrs.
// Handles the shapes shen-cedar emits: == / comparisons over context.<attr> and
// the `principal has id` fallback. Returns "" for shapes it can't render.
func cedarRenderBody(body map[string]any, ctxAttrs map[string]string) string {
	for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		m, ok := body[op].(map[string]any)
		if !ok {
			continue
		}
		left, lt := cedarRenderExpr(m["left"])
		right, rt := cedarRenderExpr(m["right"])
		if left == "" || right == "" {
			return ""
		}
		// Infer the context attr type from the literal it's compared against.
		// Equality/inequality infer from the operand; ordering comparisons are Long.
		typ := "String"
		if op != "==" && op != "!=" {
			typ = "Long"
		} else if rt == "Bool" || lt == "Bool" {
			typ = "Bool"
		} else if rt == "Long" || lt == "Long" {
			typ = "Long"
		}
		recordCtxAttr(m["left"], typ, ctxAttrs)
		recordCtxAttr(m["right"], typ, ctxAttrs)
		return left + " " + op + " " + right
	}
	if h, ok := body["has"].(map[string]any); ok {
		base, _ := cedarRenderExpr(h["left"])
		attr, _ := h["attr"].(string)
		if base != "" && attr != "" {
			return base + " has " + attr
		}
	}
	return ""
}

// cedarRenderExpr renders an emitted expr node to Cedar text and returns a coarse
// type tag ("Bool"/"Long"/"") for literals to drive context-attr type inference.
func cedarRenderExpr(v any) (string, string) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", ""
	}
	if val, present := m["Value"]; present {
		switch x := val.(type) {
		case bool:
			if x {
				return "true", "Bool"
			}
			return "false", "Bool"
		case float64:
			return strconv.FormatFloat(x, 'g', -1, 64), "Long"
		case string:
			return `"` + x + `"`, "String"
		}
		return "", ""
	}
	if dot, present := m["."].(map[string]any); present {
		base, _ := cedarRenderExpr(dot["left"])
		attr, _ := dot["attr"].(string)
		if base != "" && attr != "" {
			return base + "." + attr, ""
		}
	}
	if varName, present := m["Var"].(string); present {
		return varName, ""
	}
	return "", ""
}

// recordCtxAttr notes a context.<attr> reference and its inferred type.
func recordCtxAttr(v any, typ string, ctxAttrs map[string]string) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	dot, ok := m["."].(map[string]any)
	if !ok {
		return
	}
	base, _ := dot["left"].(map[string]any)
	if vn, _ := base["Var"].(string); vn != "context" {
		return
	}
	if attr, _ := dot["attr"].(string); attr != "" {
		if _, seen := ctxAttrs[attr]; !seen || typ != "String" {
			ctxAttrs[attr] = typ
		}
	}
}

func runRealCedarValidateIfAvailable(schemaOut, policiesOut string, targets []string, verbose bool) {
	cbin, err := FindCedar()
	if err != nil {
		return
	}
	textSchema, textPol := writeCedarTextCompanions(schemaOut, policiesOut, targets)
	if verbose {
		printCommand(cbin, []string{"validate", "--schema", textSchema, "--policies", textPol, "--deny-warnings"})
	}
	cmd := exec.Command(cbin, "validate", "--schema", textSchema, "--policies", textPol, "--deny-warnings")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  real cedar validate FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "  real cedar validate OK")
}

// -----------------------------------------------------------------------------
// Decidable-Shen-fragment sketch (tiny emitter/mode + pure eval stub for diff).
// This lives in sb for wiring; the recognizer/cert logic is also in shen-cedar
// --decidable (single-home binary). The judgment is the Shen sequent rules +
// Prolog gatekeeper; here we do a minimal syntactic check for the sketch.
// -----------------------------------------------------------------------------

func runDecidableShenFragmentMode(spec string, cfg *Config, regen, verbose bool) {
	fmt.Fprintf(os.Stderr, "sb policy --decidable: decidable-Shen-fragment tier (native terminating) on %s\n", spec)

	// 1. Invoke shen-cedar in decidable mode (the "tiny shen-decidable" recognizer / emitter).
	// It will parse @decidable-fragment (or targets), check fragment, emit cert comment or stub.
	cedarBin, err := FindShenCedar()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb policy --decidable: warning: shen-cedar not found (%v); using local stub only\n", err)
	} else {
		absSpec, _ := filepath.Abs(spec)
		runArgs := []string{"--spec", absSpec, "--decidable"}
		if regen {
			runArgs = append(runArgs, "--regen")
		}
		if len(cfg.DecidableShen.Targets) > 0 {
			runArgs = append(runArgs, "--targets", strings.Join(cfg.DecidableShen.Targets, ","))
		}
		if verbose {
			printCommand(cedarBin, runArgs)
		}
		if err := runInDir("", cedarBin, runArgs...); err != nil {
			// A non-zero exit means the fragment check failed (spec outside the
			// decidable fragment). That is a real gate failure — propagate it.
			fmt.Fprintf(os.Stderr, "sb policy --decidable: shen-cedar --decidable: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "  shen-cedar --decidable: fragment check OK")
	}

	// 2. The full n-way differential (guard ctors vs Cedar vs Rego vs
	// pure-shen-fragment-eval, over generated samples) lives in the example's
	// `make cedar-verify` harness, which fails on any unsound lowering. We do
	// not duplicate a hardcoded mini-oracle here — that only drifts from the
	// real harness and gives a false signal.
	if len(cfg.DecidableShen.Targets) > 0 {
		fmt.Fprintf(os.Stderr, "  fragment targets: %s — run `make cedar-verify` for the n-way differential\n",
			strings.Join(cfg.DecidableShen.Targets, ", "))
	}

	// 3. The cert + total-eval stub sidecars are written by `shen-cedar
	// --decidable --regen` (invoked above when regen is set); nothing further to do.
}

// (runInDir and printCommand are provided by derive.go in the same package.)

// runRealOpaValidateIfAvailable runs a smoke `opa eval` against the emitted
// primary .rego (text) when the opa binary is present. This is the Rego
// equivalent of runRealCedarValidateIfAvailable — gives official evaluator
// signal in the policy gate (and in cedar-verify / n-way harness).
func runRealOpaValidateIfAvailable(moduleOut string, targets []string, pkg string, verbose bool) {
	obin, err := FindOpa()
	if err != nil {
		return
	}
	if pkg == "" {
		pkg = "policy"
	}
	rules := []string{"allow"}
	if len(targets) > 0 {
		rules = rules[:0]
		for _, t := range targets {
			rules = append(rules, ps.ToSnakeIdent(t))
		}
	}

	// Positive input satisfies every precondition we lower (membership, token
	// non-emptiness + expiry, ownership, service secret); the negative flips
	// membership so every access rule must become false. Attribute names match
	// the emitter's camelCase lowering. The real n-way matrix is `make
	// cedar-verify`; this smoke just confirms each rule is defined and
	// discriminates on real input via the official evaluator.
	pos := `{"isMember": true, "isOwned": true, "exp": 9999999999, "sig": "s", "x": "s", "secret": "s"}`
	neg := `{"isMember": false, "isOwned": false, "exp": 9999999999, "sig": "s", "x": "s", "secret": "s"}`

	for _, rule := range rules {
		expr := "data." + pkg + "." + rule
		if v, err := opaEvalBool(obin, moduleOut, pos, expr, verbose); err != nil || !v {
			fmt.Fprintf(os.Stderr, "  real opa eval FAILED: %s should be true for satisfying input (undefined rule or attr mismatch); err=%v\n", expr, err)
			os.Exit(1)
		}
		if v, err := opaEvalBool(obin, moduleOut, neg, expr, verbose); err != nil || v {
			fmt.Fprintf(os.Stderr, "  real opa eval FAILED: %s should be false when membership is denied; err=%v\n", expr, err)
			os.Exit(1)
		}
	}
	fmt.Fprintf(os.Stderr, "  real opa eval OK (module %s, %d rule(s) discriminate on input)\n", moduleOut, len(rules))
}

// opaEvalBool runs `opa eval expr` with the given input JSON and returns the
// boolean value of the expression (false if undefined). opa exits 0 even when a
// rule is undefined, so reading the value is what makes the check real.
func opaEvalBool(obin, moduleOut, inputJSON, expr string, verbose bool) (bool, error) {
	tmpInput, err := os.CreateTemp("", "opa-input-*.json")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmpInput.Name())
	tmpInput.WriteString(inputJSON)
	tmpInput.Close()

	args := []string{"eval", "--format", "json", "--input", tmpInput.Name(), "--data", moduleOut, expr}
	if verbose {
		printCommand(obin, args)
	}
	out, err := exec.Command(obin, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%v: %s", err, out)
	}
	var parsed struct {
		Result []struct {
			Expressions []struct {
				Value any `json:"value"`
			} `json:"expressions"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &parsed) == nil && len(parsed.Result) > 0 && len(parsed.Result[0].Expressions) > 0 {
		if b, ok := parsed.Result[0].Expressions[0].Value.(bool); ok {
			return b, nil
		}
	}
	return false, nil
}
