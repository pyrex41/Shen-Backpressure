package main

// ash.go — Ash policy lowering of access-shaped rules.
//
// Target selection mirrors shen-cedar/shen-rego (policyspec.SelectTargets):
// explicit --ash-targets, else every conclusion ending in -access /
// -permit / -allow. For each target the emitter writes
//
//   <NS>.Policy.<Target>Check   Ash.Policy.SimpleCheck — the actor must BE
//                               the target's guard struct (so every premise
//                               held when it was built) and, when the proof
//                               chain carries a tenant-typed field, that
//                               tenant must equal the request's tenant.
//   <NS>.Policy.<Target>Filter  Ash.Policy.FilterCheck — emitted when the
//                               proof carries a resource-typed field; scopes
//                               rows to the resource the proof names.
//
// Unlike Cedar/Rego, nothing is re-derived from attributes: the policy
// asks for the proof value itself, so the Shen premises are discharged by
// the guard constructor, not re-implemented in policy code.

import (
	"fmt"
	"strings"

	ps "github.com/pyrex41/Shen-Backpressure/policyspec"
)

type ashOptions struct {
	Targets      string
	TenantType   string
	ResourceType string
}

// fieldPath finds the shortest accessor path from a type to a field of
// type want (breadth-first over guard fields).
func (st *SymbolTable) fieldPath(from, want string) []FieldInfo {
	type item struct {
		t    string
		path []FieldInfo
	}
	queue := []item{{t: from}}
	seen := map[string]bool{from: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		info := st.Lookup(cur.t)
		if info == nil || info.Category == "sumtype" || len(info.Fields) == 0 {
			continue
		}
		for _, f := range info.Fields {
			p := append(append([]FieldInfo{}, cur.path...), f)
			if f.ShenType == want {
				return p
			}
			if !seen[f.ShenType] {
				seen[f.ShenType] = true
				queue = append(queue, item{t: f.ShenType, path: p})
			}
		}
	}
	return nil
}

// accessorChain renders `proof |> Mod.a() |> Mod2.b() [|> W.val()]`.
func (st *SymbolTable) accessorChain(from string, path []FieldInfo) (string, string) {
	var parts []string
	cur := from
	var names []string
	for _, f := range path {
		parts = append(parts, fmt.Sprintf("%s.%s()", st.moduleFor(cur), toSnake(f.ShenName)))
		names = append(names, f.ShenName)
		cur = f.ShenType
	}
	if st.IsWrapper(cur) {
		parts = append(parts, st.moduleFor(cur)+".val()")
	}
	return "proof |> " + strings.Join(parts, " |> "), strings.Join(names, " → ")
}

func generateAsh(specContent string, st *SymbolTable, opt genOptions, ao ashOptions) (string, []string, error) {
	targets := ps.SelectTargets(ao.Targets, ps.CollectConclusions(stripComments(specContent)))
	if len(targets) == 0 {
		return "", nil, fmt.Errorf("no access-shaped targets found (pass --ash-targets)")
	}
	ns := opt.Namespace
	var b strings.Builder
	b.WriteString(header(opt.SpecPath, fmt.Sprintf(`# Ash policy checks lowered from the access rules of the Shen spec
# (targets: %s). Each check demands the Shen proof struct itself
# as the actor, so the premises were discharged by its constructor.`, strings.Join(targets, ", "))))
	b.WriteString("\n")

	fmt.Fprintf(&b, "defmodule %s.Policy do\n", ns)
	b.WriteString("  @moduledoc \"Helpers shared by the generated Ash policy checks.\"\n\n")
	b.WriteString("  @doc \"The tenant of the request being authorized, as a string (nil when none).\"\n")
	b.WriteString("  @spec request_tenant(term()) :: String.t() | nil\n")
	b.WriteString("  def request_tenant(%{subject: subject}) when is_map(subject) do\n")
	b.WriteString("    case Map.get(subject, :to_tenant) || Map.get(subject, :tenant) do\n      nil -> nil\n      tenant -> to_string(tenant)\n    end\n  end\n\n")
	b.WriteString("  def request_tenant(_context), do: nil\n\n")
	b.WriteString("  @doc \"True when the proof's tenant equals the request tenant. A request without a tenant never matches.\"\n")
	b.WriteString("  @spec tenant_matches?(String.t(), term()) :: boolean()\n")
	b.WriteString("  def tenant_matches?(proof_tenant, context) do\n    case request_tenant(context) do\n      nil -> false\n      tenant -> tenant == to_string(proof_tenant)\n    end\n  end\nend\n")

	var names []string
	for _, t := range targets {
		info := st.Lookup(t)
		if info == nil {
			return "", nil, fmt.Errorf("ash target %q is not a conclusion in the spec", t)
		}
		isProof := "is_struct(actor, " + st.moduleFor(t) + ")"
		if info.Category == "sumtype" {
			isProof = st.moduleFor(t) + ".member?(actor)"
		}
		tenantPath := st.fieldPath(t, ao.TenantType)
		resourcePath := st.fieldPath(t, ao.ResourceType)
		base := toPascalCase(t)
		checkMod := ns + ".Policy." + base + "Check"
		names = append(names, checkMod)

		fmt.Fprintf(&b, "\ndefmodule %s do\n", checkMod)
		fmt.Fprintf(&b, "  @moduledoc \"\"\"\n  Ash `SimpleCheck` for Shen `%s`.\n\n", t)
		fmt.Fprintf(&b, "  Passes only when the actor is a `%s` proof — built by its\n  constructor, so every premise of the rule held", st.moduleFor(t))
		if tenantPath != nil {
			_, human := st.accessorChain(t, tenantPath)
			fmt.Fprintf(&b, " — and the proof's\n  tenant (`%s`) equals the request tenant", human)
		}
		b.WriteString(".\n\n      policy action_type(:read) do\n")
		fmt.Fprintf(&b, "        authorize_if %s\n      end\n  \"\"\"\n", checkMod)
		b.WriteString("  use Ash.Policy.SimpleCheck\n\n")
		fmt.Fprintf(&b, "  @impl true\n  def describe(_opts), do: %s\n\n", elixirLiteralString(describeCheck(t, tenantPath != nil)))
		b.WriteString("  @impl true\n")
		if tenantPath != nil {
			fmt.Fprintf(&b, "  def match?(actor, context, _opts) do\n    %s and %s.Policy.tenant_matches?(tenant(actor), context)\n  end\n\n", isProof, ns)
			chain, human := st.accessorChain(t, tenantPath)
			fmt.Fprintf(&b, "  @doc \"Tenant the proof was issued for (Shen path: %s).\"\n", human)
			fmt.Fprintf(&b, "  @spec tenant(%s.t()) :: term()\n", st.moduleFor(t))
			fmt.Fprintf(&b, "  def tenant(proof), do: %s\n", chain)
		} else {
			fmt.Fprintf(&b, "  def match?(actor, _context, _opts), do: %s\n", isProof)
		}
		b.WriteString("end\n")

		if resourcePath != nil && info.Category != "sumtype" {
			filterMod := ns + ".Policy." + base + "Filter"
			names = append(names, filterMod)
			chain, human := st.accessorChain(t, resourcePath)
			fmt.Fprintf(&b, "\ndefmodule %s do\n", filterMod)
			fmt.Fprintf(&b, "  @moduledoc \"\"\"\n  Ash `FilterCheck` for Shen `%s`: scopes rows to the resource the\n", t)
			fmt.Fprintf(&b, "  actor's proof names (Shen path: `%s`). Option `:attribute` (default `:id`)\n  names the resource attribute to compare.\n\n", human)
			fmt.Fprintf(&b, "      policy action_type([:update, :destroy]) do\n        authorize_if {%s, attribute: :id}\n      end\n  \"\"\"\n", filterMod)
			b.WriteString("  use Ash.Policy.FilterCheck\n\n")
			fmt.Fprintf(&b, "  @impl true\n  def describe(opts), do: \"record #{Keyword.get(opts, :attribute, :id)} is the resource named by the actor's Shen %s proof\"\n\n", t)
			b.WriteString("  @impl true\n  def filter(actor, authorizer, opts) do\n    attribute = Keyword.get(opts, :attribute, :id)\n\n")
			cond := isProof
			if tenantPath != nil {
				cond += fmt.Sprintf(" and %s.Policy.tenant_matches?(%s.tenant(actor), authorizer)", ns, checkMod)
			}
			fmt.Fprintf(&b, "    if %s do\n      resource = resource(actor)\n      expr(^ref(attribute) == ^resource)\n    else\n      expr(false)\n    end\n  end\n\n", cond)
			fmt.Fprintf(&b, "  @doc \"Resource the proof was issued for (Shen path: %s).\"\n", human)
			fmt.Fprintf(&b, "  @spec resource(%s.t()) :: term()\n", st.moduleFor(t))
			fmt.Fprintf(&b, "  def resource(proof), do: %s\nend\n", chain)
		}
	}
	return b.String(), names, nil
}

func describeCheck(target string, tenant bool) string {
	if tenant {
		return "actor holds a Shen " + target + " proof for the request tenant"
	}
	return "actor holds a Shen " + target + " proof"
}
