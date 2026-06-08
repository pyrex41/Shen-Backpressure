# Multi-Tenant SaaS API — Implementation Plan

- [x] Set up SQLite database schema (users, tenants, tenant_memberships, resources, access_logs)
- [x] Implement JWT signing and validation (login endpoint, token parsing middleware)
- [x] Implement authentication proof construction (JWT → AuthenticatedUser with guard types)
- [x] Implement tenant membership lookup and TenantAccess proof construction
- [x] Implement resource ownership check and ResourceAccess proof construction
- [x] Implement HTTP handlers: POST /auth/login, GET /tenants/:id/resources, GET /tenants/:id/resources/:rid, POST /tenants/:id/resources
- [x] Build htmx admin dashboard (tenant list, user list, access logs, resource browser)
- [x] Integration tests for proof chain (cross-tenant access rejected, expired token rejected, non-member rejected, valid access accepted)

# Decidable-Shen-Fragment runtime tier sketch (parallel native-Shen middle tier: Cedar ⊂ Rego ⊂ Decidable-Shen-fragment ⊂ full-TC pure-Shen)
# Use Shen sequent calculus + embedded Prolog as gatekeeper. Restricted fragment: no general recursion, stratified/Horn-shaped bodies, total (bounded) rules.
# Judgment via @decidable-fragment annotations (dischargeable by tc+ / small Prolog later). Tiny emitter/mode certifies or runs in total evaluator stub.
# At runtime (shen-go etc): embed restricted predicate directly, guaranteed termination, zero translation drift for native data.
# Differential: n-way (guard ctors vs Cedar vs pure-shen-fragment-eval) on same samples. Single-home (Cedar wins overlap); this tier = Shen-native terminating enforcement w/o external dep.
- [x] Add @decidable-fragment annotations to tenant-access / resource-access in specs/core.shen (and core.tmpl)
- [x] Extend sb config/context/gates/policy for DecidableShenConfig + auto "shen-decidable" gate (sketch: certification + stub eval)
- [x] Tiny shen-decidable mode inside shen-cedar (or skeleton): fragment parser, recursion/allowed-form check, emit certified comment or total-eval stub
- [x] Extend cedar-verify differential harness with "pure-shen-fragment-eval" path (Go re-impl of restricted rules) + n-way agreement on samples
- [x] Wire [decidable-shen] in example sb.toml; update sb help, context markdown, all comments for lattice + "Shen as gatekeeper"
- [x] Run go build/test across cmd/sb, cmd/shen-cedar, example; keep sketch minimal (real total-eval/full lowering later)
