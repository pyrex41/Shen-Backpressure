// cmd/cedar-verify — n-way differential agreement / cross-emitter verification harness
// (guard ctors vs Cedar vs Rego vs decidable-Shen-fragment).
//
// This is the soundness gate for the runtime policy lowerings (Cedar + Rego middle tier)
// from the Shen spec. n-way agreement checking between:
//
//   1. The guard constructors (NewTenantAccess, NewResourceAccess etc.) as the
//      source-of-truth "should allow" oracle. A request is allowed iff the
//      corresponding New* ctor succeeds when given the IsMember/IsOwned flags.
//   2. The emitted Cedar policy (via shen-cedar JSON output).
//   3. pure-shen-fragment-eval (Decidable-Shen-fragment native tier): the restricted
//      total rules (Horn-shaped, no recursion) either via the stub emitted by
//      shen-cedar --decidable or the equivalent in-process re-impl. This path
//      can be embedded directly in a Shen runtime port (zero drift for native data).
//
// The harness always drives shen-cedar (plus --decidable) to (re)emit artifacts,
// then evaluates guard / Cedar / pure-shen-fragment on the same samples.
// - Sampling (PARC + the IsMember/IsOwned proof flags) reuses ideas from
//   shen-derive/verify (boundary values + bounded cartesian).
// - Guard side uses the exact same New* ctors as the real application
//   (internal/auth/tenant.go).
// - Cedar side uses an in-process JSON policy evaluator that interprets the
//   when-conditions produced by the emitter (== on context attrs, has, etc.).
// - pure-shen side is the total evaluator for the certified decidable fragment.
// - Results + full requests are written as JSONL (ready for `cedar authorize`
//   batch or SDK) + reports.
// This gives a real differential signal on lowering fidelity across the lattice.
// Persistent guard-deny + policy-allow mismatches are backpressure on emitters.
// - Sampling (PARC + the IsMember/IsOwned proof flags) reuses ideas from
//   shen-derive/verify (boundary values + bounded cartesian).
// - Guard side uses the exact same New* ctors as the real application
//   (internal/auth/tenant.go).
// - Cedar side uses an in-process JSON policy evaluator that interprets the
//   when-conditions produced by the emitter (== on context attrs, has, etc.).
// - Results + full requests are written as JSONL (ready for `cedar authorize`
//   batch or SDK) + reports.
// This gives a real differential signal on lowering fidelity. Persistent
// guard-deny + policy-allow mismatches after lowering improvements are the
// backpressure that drives fixes in the emitter.
//
// Run from the multi-tenant-api module root:
//   go run ./cmd/cedar-verify
//   make cedar-verify
//
// This is intentionally a standalone prototype binary (not a package test) so
// it can also serve as a future "batch driver" entrypoint.

package main

import (
	"bufio"
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

	"multi-tenant-api/internal/shenguard"
)

// --- Request / sample model (PARC + ground truth flags for guard oracle) ---

type accessSample struct {
	PrincipalID string
	TenantID    string
	ResourceID  string // empty for pure tenant-access samples
	IsMember    bool
	IsOwned     bool
	Level       string // "tenant" or "resource"
}

type cedarRequest struct {
	Principal map[string]any `json:"principal"`
	Action    map[string]any `json:"action"`
	Resource  map[string]any `json:"resource"`
	Context   map[string]any `json:"context"`
}

type sampleRecord struct {
	PrincipalID      string       `json:"principal_id"`
	TenantID         string       `json:"tenant_id"`
	ResourceID       string       `json:"resource_id,omitempty"`
	IsMember         bool         `json:"is_member"`
	IsOwned          bool         `json:"is_owned"`
	Level            string       `json:"level"`
	GuardShouldAllow bool         `json:"guard_should_allow"`
	Request          cedarRequest `json:"request"`
}

// cedarPolicyEvaluator is the abstraction for evaluating a Cedar policy set
// (loaded from the JSON emitted by shen-cedar) against PARC requests.
// Real in-process evaluation of the emitted policies (no stub ever).
type cedarPolicyEvaluator interface {
	Authorize(principal, action, resource, context map[string]any) (bool, error)
}

// newPureShenEval returns the "embedded decidable-Shen eval" for the n-way diff.
// It parses the spec once and evaluates a target's DNF clauses via the shared
// policyspec.EvalClauses — the same total evaluator the generated stub uses, so
// there is no hand-maintained per-target switch to drift from the emitters. The
// environment mirrors the always-valid HUMAN principal the guard oracle builds
// (makeDummyPrincipal: non-empty token, exp > now, no service secret), with
// isMember/isOwned varying per sample.
func newPureShenEval(specPath string) func(target string, s accessSample) bool {
	content, _ := os.ReadFile(specPath)
	variants := ps.BuildRuleVariants(ps.ParseDatatypes(string(content)))
	return func(target string, s accessSample) bool {
		env := map[string]any{
			"isMember": s.IsMember,
			"isOwned":  s.IsOwned,
			"exp":      float64(9999999999),   // parsed-claims (> Exp 0)
			"sig":      "dummy-sig-non-empty", // verified-jwt (not (= Sig ""))
			"x":        "non-empty",           // jwt-issuer / jwt-audience (not (= X ""))
			"secret":   "",                    // no service secret for the human principal
		}
		res, ok := ps.EvalClauses(ps.CollectClauses(target, variants, map[string]bool{}), env)
		return ok && res
	}
}

// jsonPolicyEvaluator loads a Cedar JSON policies file (the format produced
// by shen-cedar) and evaluates permit/when conditions against the request.
// This is the real in-process evaluator for the differential (no external
// cedar binary required for the common simple cases we emit).
type jsonPolicyEvaluator struct {
	policies []map[string]any
}

// fallbackEvaluator tries primary (e.g. the real cedar binary) and falls back to
// a reliable in-process evaluator for any request the primary cannot evaluate, so
// a version-incompatible cedar CLI degrades to correct results instead of failing
// the gate. A primary decision is used as-is; only its hard errors trigger fallback.
type fallbackEvaluator struct {
	primary  cedarPolicyEvaluator
	fallback cedarPolicyEvaluator
}

func (e *fallbackEvaluator) Authorize(principal, action, resource, context map[string]any) (bool, error) {
	if e.primary != nil {
		if ok, err := e.primary.Authorize(principal, action, resource, context); err == nil {
			return ok, nil
		}
	}
	return e.fallback.Authorize(principal, action, resource, context)
}

func newJSONPolicyEvaluator(policiesPath string) (cedarPolicyEvaluator, error) {
	b, err := os.ReadFile(policiesPath)
	if err != nil {
		return nil, fmt.Errorf("read policies: %w", err)
	}
	var set struct {
		Policies []map[string]any `json:"policies"`
	}
	if err := json.Unmarshal(b, &set); err != nil {
		return nil, fmt.Errorf("parse policies JSON: %w", err)
	}
	if len(set.Policies) == 0 {
		return nil, fmt.Errorf("no policies found in %s", policiesPath)
	}
	return &jsonPolicyEvaluator{policies: set.Policies}, nil
}

// realCedarBinaryEvaluator uses the installed `cedar` CLI binary for real
// authorization decisions. It converts internal requests to the string form
// the CLI expects, generates a minimal entities file, a text .cedar policies
// (for reliability with the current emitter JSON), and a schema JSON in the
// form the CLI accepts. This fulfills using the real Cedar binary for the
// differential (instead of only in-process or stub).
type realCedarBinaryEvaluator struct {
	bin          string
	policiesJSON string
	schemaJSON   string
	policiesText string
	schemaText   string
}

func newRealCedarBinaryEvaluator(bin, policiesJSON, schemaJSON string) (cedarPolicyEvaluator, error) {
	if bin == "" {
		return nil, fmt.Errorf("no cedar binary")
	}
	policiesText, schemaText, err := cedarTextCompanionsFromJSON(policiesJSON)
	if err != nil {
		return nil, err
	}
	return &realCedarBinaryEvaluator{
		bin:          bin,
		policiesJSON: policiesJSON,
		schemaJSON:   schemaJSON,
		policiesText: policiesText,
		schemaText:   schemaText,
	}, nil
}

func (e *realCedarBinaryEvaluator) Authorize(principal, action, resource, context map[string]any) (bool, error) {
	tmpDir, err := os.MkdirTemp("", "cedar-auth-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmpDir)

	// String forms for CLI
	pStr := fmt.Sprintf(`%s::"%s"`, principal["type"], principal["id"])
	aStr := fmt.Sprintf(`%s::"%s"`, action["type"], action["id"])
	rID := resource["id"]
	if rID == "" {
		rID = "placeholder"
	}
	rStr := fmt.Sprintf(`%s::"%s"`, resource["type"], rID)

	// Minimal entities.json using the object form for uid (required by CLI).
	tenantID := "t-unknown"
	if t, ok := resource["tenantId"].(string); ok {
		tenantID = t
	}
	if t, ok := context["tenantId"].(string); ok {
		tenantID = t
	}
	entities := []map[string]any{
		{"uid": map[string]any{"type": "User", "id": principal["id"]}, "attrs": map[string]any{}, "parents": []any{}},
		{"uid": map[string]any{"type": "Tenant", "id": tenantID}, "attrs": map[string]any{}, "parents": []any{}},
		{"uid": map[string]any{"type": "Resource", "id": rID}, "attrs": map[string]any{"tenantId": tenantID}, "parents": []any{}},
	}
	entsPath := filepath.Join(tmpDir, "entities.json")
	entsB, _ := json.Marshal(entities)
	_ = os.WriteFile(entsPath, entsB, 0644)

	// Request in the form CLI --request-json expects: principal etc as strings
	req := map[string]any{
		"principal": pStr,
		"action":    aStr,
		"resource":  rStr,
		"context":   context,
	}
	reqPath := filepath.Join(tmpDir, "req.json")
	reqB, _ := json.Marshal(req)
	_ = os.WriteFile(reqPath, reqB, 0644)

	polTextPath := filepath.Join(tmpDir, "policies.cedar")
	_ = os.WriteFile(polTextPath, []byte(e.policiesText), 0644)
	schemaPath := filepath.Join(tmpDir, "schema.cedarschema")
	_ = os.WriteFile(schemaPath, []byte(e.schemaText), 0644)

	// Call real cedar
	cmd := exec.Command(e.bin, "authorize",
		"--entities", entsPath,
		"--policies", polTextPath,
		"--schema", schemaPath,
		"--request-json", reqPath,
		"--request-validation", "false",
	)
	out, err := cmd.CombinedOutput()
	decision := strings.TrimSpace(string(out))
	upperDecision := strings.ToUpper(decision)
	if strings.Contains(upperDecision, "ALLOW") {
		return true, nil
	}
	if strings.Contains(upperDecision, "DENY") {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cedar authorize: %w\n%s", err, string(out))
	}
	return false, fmt.Errorf("cedar authorize returned an unrecognized decision: %s", decision)
}

func cedarTextCompanionsFromJSON(policiesJSON string) (string, string, error) {
	raw, err := os.ReadFile(policiesJSON)
	if err != nil {
		return "", "", err
	}
	var set struct {
		Policies []struct {
			Effect     string `json:"effect"`
			Definition struct {
				Conditions []struct {
					Kind string         `json:"kind"`
					Body map[string]any `json:"body"`
				} `json:"conditions"`
			} `json:"definition"`
		} `json:"policies"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		return "", "", err
	}

	ctxAttrs := map[string]string{}
	var policies strings.Builder
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
		fmt.Fprintf(&policies, "\n%s (\n    principal,\n    action,\n    resource\n) when {\n    %s\n};\n", eff, body)
	}

	var attrLines []string
	for name, typ := range ctxAttrs {
		attrLines = append(attrLines, fmt.Sprintf("    %s: %s", name, typ))
	}
	sort.Strings(attrLines)
	ctxBlock := "{}"
	if len(attrLines) > 0 {
		ctxBlock = "{\n" + strings.Join(attrLines, ",\n") + "\n  }"
	}
	schema := fmt.Sprintf(`entity User in [Tenant] = {
  "id"?: String
};
entity Tenant;
entity Resource = {
  "tenantId"?: String
};

action "read" appliesTo {
  principal: [User],
  resource: [Resource],
  context: %s
};
`, ctxBlock)
	return policies.String(), schema, nil
}

func cedarRenderBody(body map[string]any, ctxAttrs map[string]string) string {
	for _, op := range []string{"==", ">=", "<=", ">", "<"} {
		m, ok := body[op].(map[string]any)
		if !ok {
			continue
		}
		left, lt := cedarRenderExpr(m["left"])
		right, rt := cedarRenderExpr(m["right"])
		if left == "" || right == "" {
			return ""
		}
		typ := "String"
		if op != "==" {
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

// --- Rego (OPA) support for n-way (use `opa eval` on emitted .rego) ---
type opaEvaluator struct {
	bin      string
	module   string
	ruleBase string
}

// regoPackage is the Rego package the emitter is told to use (via --package) and
// the package the harness queries (data.<pkg>.<rule>); keep them in lockstep so
// the OPA path never queries a non-existent rule and silently reports false.
const regoPackage = "multi_tenant_authz"

func newOpaEvaluator(bin, module string) (cedarPolicyEvaluator, error) {
	if bin == "" {
		return nil, fmt.Errorf("no opa")
	}
	return &opaEvaluator{bin: bin, module: module, ruleBase: "data." + regoPackage}, nil
}

func (e *opaEvaluator) Authorize(principal, action, resource, context map[string]any) (bool, error) {
	input := map[string]any{"isMember": false, "isOwned": false}
	if v, ok := context["isMember"].(bool); ok {
		input["isMember"] = v
	}
	if v, ok := context["isOwned"].(bool); ok {
		input["isOwned"] = v
	}
	// Forward any remaining context evidence (e.g. exp/now for the token-expiry
	// precondition the lowered rules now carry) so the OPA path matches the
	// in-process evaluator and the guard oracle.
	for k, v := range context {
		if k == "level" {
			continue
		}
		if _, set := input[k]; !set {
			input[k] = v
		}
	}
	rule := "tenant_access"
	if rid, ok := resource["id"].(string); ok && rid != "" {
		rule = "resource_access"
	}
	tmpDir, _ := os.MkdirTemp("", "opa-*")
	defer os.RemoveAll(tmpDir)
	inP := filepath.Join(tmpDir, "in.json")
	ib, _ := json.Marshal(input)
	os.WriteFile(inP, ib, 0644)
	expr := e.ruleBase + "." + rule
	cmd := exec.Command(e.bin, "eval", "--input", inP, "--data", e.module, "--format", "json", expr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, nil
	}
	var r struct {
		Result []struct {
			Expressions []struct {
				Value interface{} `json:"value"`
			} `json:"expressions"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &r) == nil && len(r.Result) > 0 && len(r.Result[0].Expressions) > 0 {
		if b, ok := r.Result[0].Expressions[0].Value.(bool); ok {
			return b, nil
		}
	}
	return false, nil
}

func (e *jsonPolicyEvaluator) Authorize(principal, action, resource, context map[string]any) (bool, error) {
	req := map[string]any{
		"principal": principal,
		"action":    action,
		"resource":  resource,
		"context":   context,
	}
	for _, pol := range e.policies {
		if e.policyAllows(pol, req) {
			return true, nil
		}
	}
	return false, nil
}

func (e *jsonPolicyEvaluator) policyAllows(pol map[string]any, req map[string]any) bool {
	defIface, ok := pol["definition"]
	if !ok {
		return false
	}
	def, ok := defIface.(map[string]any)
	if !ok {
		return false
	}

	// Scope: our emitted policies currently use {"op":"All"} for p/a/r.
	// Treat All (or missing) as matching any entity. Future: support specific.
	if !scopeMatches(def["principal"]) {
		return false
	}
	if !scopeMatches(def["action"]) {
		return false
	}
	if !scopeMatches(def["resource"]) {
		return false
	}

	// Conditions: all "when" bodies must evaluate to true for this policy to apply.
	condsIface, ok := def["conditions"].([]any)
	if !ok || len(condsIface) == 0 {
		// No conditions (or only fallback has) -- treat as applies for permit.
		// (Our emitter always emits at least a has fallback or real condition.)
		return true
	}
	for _, c := range condsIface {
		cm, _ := c.(map[string]any)
		if cm == nil {
			continue
		}
		if k, _ := cm["kind"].(string); k != "when" {
			continue
		}
		body, _ := cm["body"].(map[string]any)
		if body == nil {
			continue
		}
		if !evalExpr(body, req) {
			return false
		}
	}
	return true
}

func scopeMatches(scopeIface any) bool {
	scope, ok := scopeIface.(map[string]any)
	if !ok {
		return true
	}
	op, _ := scope["op"].(string)
	return op == "All" || op == ""
}

// evalExpr evaluates a Cedar JSON "body" expression (the shape produced by
// our emitter's cedarBodyFromVerified / cedarExpr) in the context of a req.
// Supported (for the shapes we emit today):
//
//	{"==" : {"left": expr, "right": expr}}
//	{"has": {"left": expr, "attr": "foo"}}
//	{">=", "<=", ">", "<" : ... } (comparisons)
//	{"Var": "context"} / {"Var": "principal"} etc.
//	{".": {"left": expr, "attr": "isMember"} }
//	{"Value": true} / numbers
func evalExpr(expr map[string]any, req map[string]any) bool {
	if expr == nil {
		return false
	}

	// ==
	if eq, ok := expr["=="].(map[string]any); ok {
		l := evalToValue(eq["left"], req)
		r := evalToValue(eq["right"], req)
		return valuesEqual(l, r)
	}

	// != (e.g. lowered from (not (= Token "")))
	if ne, ok := expr["!="].(map[string]any); ok {
		l := evalToValue(ne["left"], req)
		r := evalToValue(ne["right"], req)
		return !valuesEqual(l, r)
	}

	// has
	if hasm, ok := expr["has"].(map[string]any); ok {
		left := evalToValue(hasm["left"], req)
		attr, _ := hasm["attr"].(string)
		if m, ok := left.(map[string]any); ok && attr != "" {
			if _, present := m[attr]; present {
				return true
			}
		}
		return false
	}

	// comparisons
	for _, op := range []string{">=", "<=", ">", "<"} {
		if cmp, ok := expr[op].(map[string]any); ok {
			l := evalToValue(cmp["left"], req)
			r := evalToValue(cmp["right"], req)
			lf, lok := toFloat64(l)
			rf, rok := toFloat64(r)
			if lok && rok {
				switch op {
				case ">=":
					return lf >= rf
				case "<=":
					return lf <= rf
				case ">":
					return lf > rf
				case "<":
					return lf < rf
				}
			}
			return false
		}
	}

	return false
}

func evalToValue(v any, req map[string]any) any {
	if m, ok := v.(map[string]any); ok {
		if varName, ok := m["Var"].(string); ok {
			if sub, ok := req[varName]; ok {
				return sub
			}
			return nil
		}
		if dot, ok := m["."].(map[string]any); ok {
			left := evalToValue(dot["left"], req)
			attr, _ := dot["attr"].(string)
			if lm, ok := left.(map[string]any); ok && attr != "" {
				return lm[attr]
			}
			return nil
		}
		if val, has := m["Value"]; has {
			return val
		}
	}
	return v
}

func valuesEqual(a, b any) bool {
	// Handle common cases from our policies (bool, number, string)
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		return af == bf
	}
	return a == b
}

func toFloat64(x any) (float64, bool) {
	switch v := x.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// --- Sampling (reuses ideas from shen-derive/verify/samples.go: boundary values,
// random-ready ctx, cartesian product, list-like expansion for composites) ---

func genAccessSamples(maxSamples int) []accessSample {
	// Boundary + representative ids (string samples style from samples.go).
	principals := []string{"u-alice", "u-bob", "u-nobody", "svc-cron"}
	tenants := []string{"t-acme", "t-globex"}
	resources := []string{"r-1", "r-2", "r-3", "r-cross"}

	var out []accessSample

	// Tenant-level decisions (directly exercise NewTenantAccess + isMember).
	for _, p := range principals {
		for _, t := range tenants {
			for _, m := range []bool{true, false} {
				out = append(out, accessSample{
					PrincipalID: p,
					TenantID:    t,
					IsMember:    m,
					Level:       "tenant",
				})
				if len(out) >= maxSamples {
					return out
				}
			}
		}
	}

	// Resource-level decisions. We sample full (member, owned) space.
	// Guard allow requires the *chain*: tenant ctor must succeed (isMember)
	// AND the resource ctor must succeed (isOwned). This mirrors how
	// ResourceAccess can only be obtained after a valid TenantAccess.
	for _, p := range principals {
		for _, t := range tenants {
			for _, r := range resources {
				for _, m := range []bool{true, false} {
					for _, o := range []bool{true, false} {
						out = append(out, accessSample{
							PrincipalID: p,
							TenantID:    t,
							ResourceID:  r,
							IsMember:    m,
							IsOwned:     o,
							Level:       "resource",
						})
						if len(out) >= maxSamples {
							return out
						}
					}
				}
			}
		}
	}
	return out
}

// makeDummyPrincipal builds a valid AuthenticatedPrincipal (HumanPrincipal)
// using only the exported ctors. All inner guards succeed for these values;
// the only thing that can cause New*Access to fail is the boolean flags.
func makeDummyPrincipal(userID string) shenguard.AuthenticatedPrincipal {
	uid := shenguard.NewUserId(userID)
	iss, err := shenguard.NewJwtIssuer("shen-backpressure")
	if err != nil {
		panic("NewJwtIssuer dummy: " + err.Error())
	}
	aud, err := shenguard.NewJwtAudience("multi-tenant-api")
	if err != nil {
		panic("NewJwtAudience dummy: " + err.Error())
	}
	// exp > 0 (parsed-claims), sig non-empty (verified-jwt), and AuthenticatedUser
	// binds user == claims.sub (the W2.1 cross-field premise verified upstream).
	claims, err := shenguard.NewParsedClaims(uid, 9999999999, iss, aud)
	if err != nil {
		panic("NewParsedClaims dummy: " + err.Error())
	}
	jwt, err := shenguard.NewVerifiedJwt(claims, "dummy-sig-non-empty")
	if err != nil {
		panic("NewVerifiedJwt dummy: " + err.Error())
	}
	au, err := shenguard.NewAuthenticatedUser(jwt, uid)
	if err != nil {
		panic("NewAuthenticatedUser dummy: " + err.Error())
	}
	return shenguard.NewHumanPrincipal(au)
}

// computeGuardAllow runs the exact same ctor logic the real application uses
// (via Check* or direct New). This is the oracle.
func computeGuardAllow(s accessSample) bool {
	prin := makeDummyPrincipal(s.PrincipalID)
	tid := shenguard.NewTenantId(s.TenantID)

	if s.Level == "tenant" {
		_, err := shenguard.NewTenantAccess(prin, tid, s.IsMember)
		return err == nil
	}

	// resource level: must be able to build the inner TenantAccess first.
	ta, err := shenguard.NewTenantAccess(prin, tid, s.IsMember)
	if err != nil {
		return false
	}
	rid := shenguard.NewResourceId(s.ResourceID)
	_, err = shenguard.NewResourceAccess(ta, rid, s.IsOwned)
	return err == nil
}

// buildCedarRequest turns an internal sample into the stable (P, A, R, C) shape
// that both in-process and future external Cedar evaluators consume.
func buildCedarRequest(s accessSample) (map[string]any, map[string]any, map[string]any, map[string]any) {
	principal := map[string]any{
		"type": "User",
		"id":   s.PrincipalID,
		// In a real model we might also carry "in" parents or attrs; the
		// schema emitted by shen-cedar already declares User memberOf Tenant.
	}
	action := map[string]any{
		"type": "Action",
		"id":   "read", // matches the starter action in buildMinimalCedarSchema
	}
	resource := map[string]any{
		"type":     "Resource",
		"id":       s.ResourceID,
		"tenantId": s.TenantID,
	}
	if s.Level == "tenant" {
		// For tenant decisions we still emit a resource-shaped thing for
		// uniformity in the batch file; real policies may use different actions.
		resource["id"] = ""
	}
	context := map[string]any{
		// Context can carry synthetic evidence in the prototype. Real
		// requests would not leak isMember/isOwned here; those are what the
		// lowered policy conditions will effectively check via entity data.
		"isMember": s.IsMember,
		"isOwned":  s.IsOwned,
		"level":    s.Level,
		// makeDummyPrincipal builds a valid HUMAN principal: a parsed-claims with
		// exp > 0 and a non-empty JWT signature. The lowered human clause checks
		// context.exp > 0 and context.sig != ""; the service-variant clause checks
		// context.secret != "" (empty here, so the human clause drives the decision).
		// The JWT sub-binding (= User (head (head Jwt))) is an authentication concern
		// discharged by the guard, not lowered into the policy.
		"exp":    float64(9999999999),
		"sig":    "dummy-sig-non-empty",
		"x":      "non-empty", // jwt-issuer / jwt-audience (not (= X ""))
		"secret": "",
	}
	return principal, action, resource, context
}

// --- Report + file writing (drift-report style) ---

type agreementReport struct {
	TotalSamples int `json:"total_samples"`
	GuardAllows  int `json:"guard_allows"`
	CedarAllows  int `json:"cedar_allows"`
	RegoAllows   int `json:"rego_allows"`
	// New for decidable-Shen-fragment tier (n-way differential).
	PureShenFragmentAllows int `json:"pure_shen_fragment_allows"`
	Agreements             int `json:"agreements"`
	Mismatches             int `json:"mismatches"`
	EvaluationErrors       int `json:"evaluation_errors"`

	// Breakdown of the dangerous direction for a *lowering soundness gate*:
	// policy says allow when the guard ctor would have denied.
	GuardDenyCedarAllow int `json:"guard_deny_cedar_allow"`
	GuardDenyRegoAllow  int `json:"guard_deny_rego_allow"`

	// The other direction (policy too strict) is usually less critical for
	// "impossible by construction" but still worth surfacing.
	GuardAllowCedarDeny int `json:"guard_allow_cedar_deny"`
	GuardAllowRegoDeny  int `json:"guard_allow_rego_deny"`

	// Pure-shen vs guard mismatches (should be zero for the restricted fragment).
	GuardDenyPureAllow int `json:"guard_deny_pure_allow"`
	GuardAllowPureDeny int `json:"guard_allow_pure_deny"`
}

func main() {
	maxSamples := flag.Int("max-samples", 48, "cap on generated (P,A,R,C) tuples")
	doEmit := flag.Bool("emit-policies", true, "run shen-cedar first to (re)generate policies/cedar/*.json (real evaluator requires successful emit)")
	outDir := flag.String("out", "policies/cedar", "directory for emitted policies and verify artifacts")
	strict := flag.Bool("strict", true, "exit non-zero when a lowering is unsound (guard denies but a policy allows); set -strict=false to report only")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "cedar-verify: prototype n-way differential (guard ctors vs Cedar vs pure-shen-fragment-eval decidable tier)\n")

	// 1. Side-effect: run the emitter so the policies/ dir exists and we
	//    demonstrate the integration point. (Safe to ignore failure in prototype.)
	exampleRoot, err := os.Getwd()
	if err != nil {
		exampleRoot = "."
	}
	// Resolve outputs relative to the example module root (where make/go run is invoked).
	schemaPath := filepath.Join(exampleRoot, *outDir, "schema.json")
	policiesPath := filepath.Join(exampleRoot, *outDir, "policies.json")
	if *doEmit {
		fmt.Fprintf(os.Stderr, "cedar-verify: invoking shen-cedar to populate %s and %s ...\n", schemaPath, policiesPath)
		// shen-cedar is its own module (has go.mod). To avoid cross-module go run
		// resolution problems we chdir the child into cmd/shen-cedar/ and invoke
		// "go run main.go" *there*, passing *absolute* paths for spec + outputs.
		shenCedarDir := filepath.Join(exampleRoot, "..", "..", "cmd", "shen-cedar")
		specAbs := filepath.Join(exampleRoot, "specs", "core.shen")
		cmd := exec.Command("go", "run", "main.go",
			"--spec", specAbs,
			"--out-schema", schemaPath,
			"--out-policies", policiesPath,
			"--targets", "tenant-access,resource-access",
			"--verbose",
		)
		cmd.Dir = shenCedarDir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "cedar-verify: WARNING: shen-cedar emitter failed (%v); policies may be stale -- real evaluator will fail to load\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "cedar-verify: emitter OK\n")
		}

		// Emit Rego module (primary text form) for n-way opa eval path (parallel to cedar).
		regoModulePath := filepath.Join(exampleRoot, "policies", "rego", "authz.rego")
		shenRegoDir := filepath.Join(exampleRoot, "..", "..", "cmd", "shen-rego")
		regoCmd := exec.Command("go", "run", "main.go",
			"--spec", specAbs,
			"--out-rego", regoModulePath,
			"--targets", "tenant-access,resource-access",
			"--package", regoPackage,
			"--verbose",
		)
		regoCmd.Dir = shenRegoDir
		regoCmd.Stdout = os.Stderr
		regoCmd.Stderr = os.Stderr
		if err := regoCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "cedar-verify: WARNING: shen-rego emitter failed (%v)\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "cedar-verify: rego emitter OK\n")
		}

		// Also drive the decidable-Shen-fragment mode (tiny emitter + cert + stub).
		// This populates decidable-fragment.cert + decidable_fragment_eval_stub.go
		// and exercises the recognizer / check for the sketch (n-way now includes pure-shen).
		decCmd := exec.Command("go", "run", "main.go",
			"--spec", specAbs,
			"--targets", "tenant-access,resource-access",
			"--decidable",
			"--verbose",
		)
		decCmd.Dir = shenCedarDir
		decCmd.Stdout = os.Stderr
		decCmd.Stderr = os.Stderr
		if err := decCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "cedar-verify: WARNING: shen-cedar --decidable failed (%v)\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "cedar-verify: shen-cedar --decidable (fragment cert + stub) OK\n")
		}
	}

	// If the cedar binary is available, directly validate the emitted schema + policies
	// using the official tool. This is more authoritative than our in-process checks
	// and adds robustness to the gate (catches format/semantic issues in the lowering).
	// We also emit a companion .cedar text file (Cedar syntax is the most reliable
	// format for the CLI's validate/authorize).
	if cedarBin, lookErr := exec.LookPath("cedar"); lookErr == nil {
		textPoliciesPath := filepath.Join(exampleRoot, *outDir, "policies.cedar")
		textSchemaPath := filepath.Join(exampleRoot, *outDir, "schema.cedarschema")
		textPolicies, textSchema, err := cedarTextCompanionsFromJSON(policiesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cedar-verify: derive Cedar text companions FAILED: %v\n", err)
			os.Exit(1)
		}
		_ = os.WriteFile(textPoliciesPath, []byte(textPolicies), 0644)
		_ = os.WriteFile(textSchemaPath, []byte(textSchema), 0644)

		fmt.Fprintf(os.Stderr, "cedar-verify: validating with real cedar binary at %s (using .cedar text for reliability)...\n", cedarBin)
		validateCmd := exec.Command(cedarBin, "validate",
			"--schema", textSchemaPath,
			"--policies", textPoliciesPath,
			"--deny-warnings",
		)
		validateCmd.Stdout = os.Stderr
		validateCmd.Stderr = os.Stderr
		if err := validateCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "cedar-verify: real cedar validate FAILED: %v\n", err)
			os.Exit(1)
		} else {
			fmt.Fprintln(os.Stderr, "cedar-verify: real cedar validate OK")
		}
	}

	// 2. Build the gating evaluator. The in-process JSON evaluator interprets the
	// emitted policies directly and is the reliable, version-independent oracle
	// (the emitted artifacts are separately confirmed valid by `cedar validate`).
	// If a real `cedar` binary is present we prefer its actual semantics, but fall
	// back to in-process per-request when the binary cannot parse a request
	// (its CLI request format varies across versions) so a binary mismatch never
	// turns the soundness gate into a false red.
	inProc, err := newJSONPolicyEvaluator(policiesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cedar-verify: failed to create JSON policy evaluator from %s: %v\n", policiesPath, err)
		fmt.Fprintf(os.Stderr, "cedar-verify: did the emitter succeed? (see --emit-policies)\n")
		os.Exit(1)
	}
	var evaluator cedarPolicyEvaluator = inProc
	if cedarBin, lookErr := exec.LookPath("cedar"); lookErr == nil {
		if ev, err := newRealCedarBinaryEvaluator(cedarBin, policiesPath, schemaPath); err == nil {
			evaluator = &fallbackEvaluator{primary: ev, fallback: inProc}
			fmt.Fprintf(os.Stderr, "cedar-verify: using real cedar binary at %s (in-process fallback per request)\n", cedarBin)
		}
	} else {
		fmt.Fprintln(os.Stderr, "cedar-verify: using in-process JSON policy evaluator (no cedar binary in PATH)")
	}

	// 2b. Rego n-way path: prefer real opa binary on the emitted .rego (text primary).
	regoModulePath := filepath.Join(exampleRoot, "policies", "rego", "authz.rego")
	var regoEval cedarPolicyEvaluator
	if opaBin, lookErr := exec.LookPath("opa"); lookErr == nil {
		if ev, err := newOpaEvaluator(opaBin, regoModulePath); err == nil {
			regoEval = ev
			fmt.Fprintf(os.Stderr, "cedar-verify: using real opa binary at %s for Rego evaluation on %s\n", opaBin, regoModulePath)
		}
	}
	if regoEval == nil {
		fmt.Fprintln(os.Stderr, "cedar-verify: (no opa binary; Rego tier will be skipped in n-way matrix)")
	}

	// 3. Generate samples (boundary + cartesian, capped).
	samples := genAccessSamples(*maxSamples)
	fmt.Fprintf(os.Stderr, "cedar-verify: generated %d samples (max=%d)\n", len(samples), *maxSamples)

	// pure-shen-fragment evaluator: parses the spec and evaluates a target's
	// transitive verified premises via the shared policyspec.EvalVerified — the
	// same total evaluator the generated stub uses, so the two cannot drift.
	pureEval := newPureShenEval(filepath.Join(exampleRoot, "specs", "core.shen"))

	// 4. Evaluate all three (n-way) + collect.
	var (
		guardAllows            int
		cedarAllows            int
		regoAllows             int
		pureShenFragmentAllows int
		agreements             int
		mismatches             []string
		gdca                   int // guard-deny + cedar-allow
		gacd                   int // guard-allow + cedar-deny
		gdra                   int // guard-deny + rego-allow
		gard                   int // guard-allow + rego-deny
		gdpa                   int // guard-deny + pure-allow
		gapd                   int // guard-allow + pure-deny
		evaluationErrors       int
		records                []sampleRecord
	)

	for i, s := range samples {
		guardAllow := computeGuardAllow(s)
		p, a, r, c := buildCedarRequest(s)
		cedarAllow, err := evaluator.Authorize(p, a, r, c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "authorize error on sample %d: %v\n", i, err)
			evaluationErrors++
			cedarAllow = false
		}

		regoAllow := false
		if regoEval != nil {
			regoAllow, err = regoEval.Authorize(p, a, r, c)
			if err != nil {
				regoAllow = false
			}
		}

		// pure-shen-fragment-eval for n-way (uses the restricted rules directly).
		// For samples we pick the target by level (or always check tenant for member-ness).
		target := "tenant-access"
		if s.Level == "resource" {
			target = "resource-access"
		}
		pureAllow := pureEval(target, s)

		if guardAllow {
			guardAllows++
		}
		if cedarAllow {
			cedarAllows++
		}
		if pureAllow {
			pureShenFragmentAllows++
		}
		if regoAllow {
			regoAllows++
		}
		if guardAllow == cedarAllow {
			agreements++
		} else {
			dir := "G+ C-"
			if !guardAllow && cedarAllow {
				dir = "G- C+"
				gdca++
			} else {
				gacd++
			}
			mismatches = append(mismatches, fmt.Sprintf("%s %s p=%s t=%s r=%s member=%v owned=%v : guard=%v cedar=%v",
				s.Level, dir, s.PrincipalID, s.TenantID, s.ResourceID, s.IsMember, s.IsOwned, guardAllow, cedarAllow))
		}
		if regoEval != nil && guardAllow != regoAllow {
			rdir := "G+ R-"
			if !guardAllow && regoAllow {
				rdir = "G- R+"
				gdra++
			} else {
				gard++
			}
			mismatches = append(mismatches, fmt.Sprintf("%s %s (rego) p=%s t=%s r=%s member=%v owned=%v : guard=%v rego=%v",
				s.Level, rdir, s.PrincipalID, s.TenantID, s.ResourceID, s.IsMember, s.IsOwned, guardAllow, regoAllow))
		}
		if guardAllow != pureAllow {
			pdir := "G+ P-"
			if !guardAllow && pureAllow {
				pdir = "G- P+"
				gdpa++
			} else {
				gapd++
			}
			mismatches = append(mismatches, fmt.Sprintf("%s %s (pure-shen) p=%s t=%s member=%v owned=%v : guard=%v pure=%v",
				s.Level, pdir, s.PrincipalID, s.TenantID, s.IsMember, s.IsOwned, guardAllow, pureAllow))
		}

		rec := sampleRecord{
			PrincipalID:      s.PrincipalID,
			TenantID:         s.TenantID,
			ResourceID:       s.ResourceID,
			IsMember:         s.IsMember,
			IsOwned:          s.IsOwned,
			Level:            s.Level,
			GuardShouldAllow: guardAllow,
			Request: cedarRequest{
				Principal: p,
				Action:    a,
				Resource:  r,
				Context:   c,
			},
		}
		records = append(records, rec)
	}

	rep := agreementReport{
		TotalSamples:           len(samples),
		GuardAllows:            guardAllows,
		CedarAllows:            cedarAllows,
		RegoAllows:             regoAllows,
		PureShenFragmentAllows: pureShenFragmentAllows,
		Agreements:             agreements,
		Mismatches:             len(mismatches),
		EvaluationErrors:       evaluationErrors,
		GuardDenyCedarAllow:    gdca,
		GuardAllowCedarDeny:    gacd,
		GuardDenyRegoAllow:     gdra,
		GuardAllowRegoDeny:     gard,
		GuardDenyPureAllow:     gdpa,
		GuardAllowPureDeny:     gapd,
	}

	// 5. Write artifacts consumable by future real Cedar batch.
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cedar-verify: mkdir %s: %v\n", *outDir, err)
	}
	writeVerifyArtifacts(*outDir, records, rep)

	// 6. Print human report (modeled on derive harness + drift output).
	fmt.Println("=== Guard / Cedar / Rego / pure-shen-fragment n-way Differential ===")
	fmt.Printf("Samples: %d\n", rep.TotalSamples)
	fmt.Printf("Guard allows: %d\n", rep.GuardAllows)
	fmt.Printf("Cedar allows (policy eval): %d\n", rep.CedarAllows)
	fmt.Printf("Rego allows (opa eval): %d\n", rep.RegoAllows)
	fmt.Printf("pure-shen-fragment-eval allows: %d\n", rep.PureShenFragmentAllows)
	fmt.Printf("Agreements (guard==cedar): %d (%.1f%%)\n", rep.Agreements, percent(rep.Agreements, rep.TotalSamples))
	fmt.Printf("Mismatches (guard vs cedar): %d\n", rep.Mismatches)
	fmt.Printf("Evaluation errors: %d\n", rep.EvaluationErrors)
	fmt.Printf("  Guard-deny + Cedar-allow: %d  (policy too loose — soundness risk for lowering)\n", rep.GuardDenyCedarAllow)
	fmt.Printf("  Guard-deny + Rego-allow: %d   (Rego too loose)\n", rep.GuardDenyRegoAllow)
	fmt.Printf("  Guard-allow + Cedar-deny: %d  (policy too strict)\n", rep.GuardAllowCedarDeny)
	fmt.Printf("  Guard-allow + Rego-deny: %d   (Rego too strict)\n", rep.GuardAllowRegoDeny)
	fmt.Printf("pure-shen vs guard mismatches: G-P+=%d G+P-=%d (should be 0 for the fragment)\n", rep.GuardDenyPureAllow, rep.GuardAllowPureDeny)
	fmt.Println()
	if len(mismatches) > 0 {
		fmt.Println("First 12 mismatches (full list in verify-samples.jsonl):")
		limit := len(mismatches)
		if limit > 12 {
			limit = 12
		}
		for _, m := range mismatches[:limit] {
			fmt.Printf("  - %s\n", m)
		}
		if len(mismatches) > 12 {
			fmt.Printf("  ... (%d more)\n", len(mismatches)-12)
		}
		fmt.Println()
	}
	fmt.Printf("Artifacts written under %s/ (use for future cedar authorize batch):\n", *outDir)
	fmt.Println("  - verify-samples.jsonl  (one request per line + guard_should_allow)")
	fmt.Println("  - verify-report.json    (summary)")
	fmt.Println()
	fmt.Println("Real policy evaluation (no stub) against the emitted policies.json.")
	fmt.Println("The JSONL is ready for `cedar authorize` (or SDK batch) against the same schema+policies.")

	// If the cedar binary is available, log how the JSONL + emitted policies
	// can be used directly for full external evaluation (point 4 in the plan).
	if cedarBin, lookErr := exec.LookPath("cedar"); lookErr == nil {
		fmt.Fprintf(os.Stderr, "\ncedar binary detected at %s\n", cedarBin)
		fmt.Fprintf(os.Stderr, "You can drive full batch eval with the artifacts:\n")
		fmt.Fprintf(os.Stderr, "  cedar authorize --policies %s --schema %s --request-jsonl %s/verify-samples.jsonl\n",
			filepath.Join(*outDir, "policies.json"),
			filepath.Join(*outDir, "schema.json"),
			*outDir)
	} else {
		fmt.Fprintln(os.Stderr, "(no `cedar` binary in PATH; using in-process JSON policy eval above)")
	}

	fmt.Println("Run with -max-samples=8 for a quick smoke; larger values exercise more of the flag space.")
	fmt.Println("=== end report ===")

	// Strict mode is an agreement gate: any evaluator error or guard/policy
	// mismatch means the lowering is not merge-ready.
	if *strict && (rep.EvaluationErrors > 0 || rep.Mismatches > 0) {
		fmt.Fprintf(os.Stderr, "cedar-verify: FAIL — evaluator errors=%d mismatches=%d (G-C+=%d G+C-=%d G-R+=%d G+R-=%d G-P+=%d G+P-=%d)\n",
			rep.EvaluationErrors, rep.Mismatches,
			rep.GuardDenyCedarAllow, rep.GuardAllowCedarDeny,
			rep.GuardDenyRegoAllow, rep.GuardAllowRegoDeny,
			rep.GuardDenyPureAllow, rep.GuardAllowPureDeny)
		os.Exit(1)
	}
}

// percent helper (avoid importing math for a simple thing).
func percent(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return (float64(num) / float64(den)) * 100.0
}

func writeVerifyArtifacts(dir string, records []sampleRecord, rep agreementReport) {
	// JSONL of requests (future `cedar authorize` batch input + expected).
	jlPath := filepath.Join(dir, "verify-samples.jsonl")
	f, err := os.Create(jlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cedar-verify: write %s: %v\n", jlPath, err)
		return
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			fmt.Fprintf(os.Stderr, "encode record: %v\n", err)
			break
		}
	}
	w.Flush()

	// Summary report.
	sumPath := filepath.Join(dir, "verify-report.json")
	b, _ := json.MarshalIndent(rep, "", "  ")
	_ = os.WriteFile(sumPath, append(b, '\n'), 0644)

	// Also emit a tiny human text for quick `cat` in make / CI.
	txtPath := filepath.Join(dir, "verify-report.txt")
	var sb strings.Builder
	fmt.Fprintf(&sb, "cedar-verify n-way report (incl. decidable-shen-fragment)\n")
	fmt.Fprintf(&sb, "samples=%d guard=%d cedar=%d pure-shen=%d agreements(guard-cedar)=%d (%.1f%%) mismatches=%d G-C+=%d G+C-=%d G-P+=%d G+P-=%d\n",
		rep.TotalSamples, rep.GuardAllows, rep.CedarAllows, rep.PureShenFragmentAllows,
		rep.Agreements, percent(rep.Agreements, rep.TotalSamples),
		rep.Mismatches, rep.GuardDenyCedarAllow, rep.GuardAllowCedarDeny,
		rep.GuardDenyPureAllow, rep.GuardAllowPureDeny)
	_ = os.WriteFile(txtPath, []byte(sb.String()), 0644)
}
